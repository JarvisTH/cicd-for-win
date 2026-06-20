package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
	"ci-cd/internal/sshutil"
)

// ========== 独立服务器数据结构（与 serve 包一致） ==========

type cliServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	AuthType     string `json:"auth_type"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	Note         string `json:"note,omitempty"`
}

type cliServerList struct {
	Servers []cliServer `json:"servers"`
}

func loadServerList() *cliServerList {
	ciDir := ciDir()
	if ciDir == "" {
		return &cliServerList{Servers: []cliServer{}}
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "servers.json"))
	if err != nil {
		return &cliServerList{Servers: []cliServer{}}
	}
	var list cliServerList
	if err := json.Unmarshal(data, &list); err != nil {
		return &cliServerList{Servers: []cliServer{}}
	}
	if list.Servers == nil {
		list.Servers = []cliServer{}
	}
	// 自动迁移：检测到明文密码则加密回写
	migratePlaintextPasswords(ciDir, &list)
	return &list
}

// migratePlaintextPasswords 检测 servers.json 中的明文密码并加密回写，
// 确保磁盘上不存留明文密码。已加密（enc: 前缀）的跳过。
func migratePlaintextPasswords(ciDir string, list *cliServerList) {
	var changed bool
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	for i := range list.Servers {
		s := &list.Servers[i]
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			enc, err := security.EncryptPassword(s.Password, key)
			if err == nil {
				list.Servers[i].Password = enc
				changed = true
			}
		}
	}
	if changed {
		saveServerList(list) // 回写加密后的配置
	}
}

func saveServerList(list *cliServerList) error {
	ciDir := ciDir()
	if ciDir == "" {
		return fmt.Errorf("找不到 ci-cd 目录")
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	// 原子写入 + 0600 权限，防止崩溃损坏与其他用户读取
	return security.AtomicWriteFile(filepath.Join(ciDir, "servers.json"), data, 0600)
}

// ========== SSH 辅助（复用 internal/sshutil，含 known_hosts 校验与密码自动解密） ==========

func dialSSH(deploy *config.DeployConfig) (*ssh.Client, error) {
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir())
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	return ssh.Dial("tcp", addr, sshCfg)
}

// resolveDeploy 根据名称和来源(source: project/standalone) 返回部署配置
func resolveDeploy(name, source string) *config.DeployConfig {
	if source == "standalone" {
		list := loadServerList()
		for _, s := range list.Servers {
			if s.Name == name {
				return &config.DeployConfig{
					Host:         s.Host,
					Port:         s.Port,
					User:         s.User,
					AuthType:     s.AuthType,
					IdentityFile: s.IdentityFile,
					Password:     s.Password,
				}
			}
		}
		return nil
	}
	cfg, err := config.Load(filepath.Join(ciDir(), "projects.json"))
	if err != nil {
		return nil
	}
	for _, p := range cfg.Projects {
		if p.Name == name && p.Deploy != nil {
			return p.Deploy
		}
	}
	return nil
}

// ========== 日志辅助 ==========

func queryAuditLogs(date, level, keyword string, limit int) []map[string]string {
	ciDir := ciDir()
	if ciDir == "" {
		return nil
	}
	if date == "" {
		date = time.Now().Format("2006-01-02") // not imported yet
	}
	logPath := filepath.Join(ciDir, "logs", fmt.Sprintf("audit-%s.jsonl", date))
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []map[string]string
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry struct {
			Time    string `json:"time"`
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if level != "" && entry.Level != level {
			continue
		}
		if keyword != "" && !strings.Contains(entry.Message, keyword) {
			continue
		}
		results = append(results, map[string]string{
			"time":    entry.Time,
			"level":   entry.Level,
			"message": entry.Message,
		})
	}
	// 倒序
	sort.Slice(results, func(i, j int) bool {
		return results[i]["time"] > results[j]["time"]
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// ========== 命令定义 ==========

// --- remote 子命令 ---

var CmdRemote = &cobra.Command{
	Use:   "remote <command>",
	Short: "远程服务器操作（文件管理 + 终端）",
}

var CmdRemoteLs = &cobra.Command{
	Use:   "ls <server>",
	Short: "列出远程服务器目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		source, _ := cmd.Flags().GetString("source")
		remotePath, _ := cmd.Flags().GetString("path")
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		entries, err := sftpClient.ReadDir(remotePath)
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		fmt.Printf("📁 %s:%s\n", name, remotePath)
		fmt.Println(strings.Repeat("─", 60))
		for _, e := range entries {
			mode := e.Mode().Perm().String()
			size := fmt.Sprintf("%10d", e.Size())
			if e.IsDir() {
				size = "       <DIR>"
			}
			modTime := e.ModTime().Format("2006-01-02 15:04")
			fmt.Printf("%s %s %s  %s\n", mode, size, modTime, e.Name())
		}
		return nil
	},
}

var CmdRemoteDownload = &cobra.Command{
	Use:   "download <server> --path <remote-path>",
	Short: "从远程服务器下载文件",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			return fmt.Errorf("打开远程文件失败: %w", err)
		}
		defer remoteFile.Close()

		localName := path.Base(remotePath)
		localFile, err := os.Create(localName)
		if err != nil {
			return fmt.Errorf("创建本地文件失败: %w", err)
		}
		defer localFile.Close()

		written, err := io.Copy(localFile, remoteFile)
		if err != nil {
			return fmt.Errorf("下载失败: %w", err)
		}
		fmt.Printf("✅ 已下载 %s (%d 字节) → %s\n", remotePath, written, localName)
		return nil
	},
}

var CmdRemoteUpload = &cobra.Command{
	Use:   "upload <server> --file <local-path> --path <remote-dir>",
	Short: "上传文件到远程服务器",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		localPath, _ := cmd.Flags().GetString("file")
		remoteDir, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if localPath == "" || remoteDir == "" {
			return fmt.Errorf("请指定 --file 和 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		localFile, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}
		defer localFile.Close()

		fileName := path.Base(localPath)
		targetPath := path.Join(remoteDir, fileName)
		remoteFile, err := sftpClient.Create(targetPath)
		if err != nil {
			return fmt.Errorf("创建远程文件失败: %w", err)
		}
		defer remoteFile.Close()

		written, err := io.Copy(remoteFile, localFile)
		if err != nil {
			return fmt.Errorf("上传失败: %w", err)
		}
		fmt.Printf("✅ 已上传 %s (%d 字节) → %s:%s\n", localPath, written, name, targetPath)
		return nil
	},
}

var CmdRemoteDelete = &cobra.Command{
	Use:   "delete <server> --path <remote-path>",
	Short: "删除远程服务器上的文件或目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		info, err := sftpClient.Stat(remotePath)
		if err != nil {
			return fmt.Errorf("路径不存在: %w", err)
		}
		if info.IsDir() {
			err = sftpClient.RemoveDirectory(remotePath)
		} else {
			err = sftpClient.Remove(remotePath)
		}
		if err != nil {
			return fmt.Errorf("删除失败: %w", err)
		}
		fmt.Printf("✅ 已删除 %s\n", remotePath)
		return nil
	},
}

var CmdRemoteMkdir = &cobra.Command{
	Use:   "mkdir <server> --path <remote-path>",
	Short: "在远程服务器上创建目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		if err := sftpClient.MkdirAll(remotePath); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		fmt.Printf("✅ 已创建目录 %s\n", remotePath)
		return nil
	},
}

// --- server 子命令 ---

var CmdServer = &cobra.Command{
	Use:   "server <command>",
	Short: "管理独立服务器",
}

var CmdServerList = &cobra.Command{
	Use:   "list",
	Short: "列出所有独立服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		list := loadServerList()
		if len(list.Servers) == 0 {
			fmt.Println("📭 无独立服务器，请通过 Web UI 或 `ci server add` 添加")
			return nil
		}
		fmt.Println("🖥️  独立服务器列表")
		fmt.Println(strings.Repeat("─", 70))
		fmt.Printf("%-20s %-20s %-8s %s\n", "名称", "主机", "端口", "认证方式")
		fmt.Println(strings.Repeat("─", 70))
		for _, s := range list.Servers {
			fmt.Printf("%-20s %-20s %-8d %s\n", s.Name, s.Host, s.Port, s.AuthType)
		}
		return nil
	},
}

var CmdServerAdd = &cobra.Command{
	Use:   "add",
	Short: "添加独立服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		host, _ := cmd.Flags().GetString("host")
		user, _ := cmd.Flags().GetString("user")
		port, _ := cmd.Flags().GetInt("port")
		authType, _ := cmd.Flags().GetString("auth-type")
		keyPath, _ := cmd.Flags().GetString("key-path")
		password, _ := cmd.Flags().GetString("password")
		note, _ := cmd.Flags().GetString("note")

		if name == "" || host == "" || user == "" {
			return fmt.Errorf("--name、--host、--user 为必填")
		}
		if port == 0 {
			port = 22
		}
		// 密码加密存储（明文 → AES-GCM 密文）
		if authType == "password" && password != "" {
			key, err := security.LoadOrCreateKey(ciDir())
			if err != nil {
				return fmt.Errorf("初始化密钥失败: %w", err)
			}
			enc, err := security.EncryptPassword(password, key)
			if err != nil {
				return fmt.Errorf("密码加密失败: %w", err)
			}
			password = enc
		}
		list := loadServerList()
		for _, s := range list.Servers {
			if s.Name == name {
				return fmt.Errorf("服务器名称已存在: %s", name)
			}
		}
		list.Servers = append(list.Servers, cliServer{
			Name:         name,
			Host:         host,
			Port:         port,
			User:         user,
			AuthType:     authType,
			IdentityFile: keyPath,
			Password:     password,
			Note:         note,
		})
		if err := saveServerList(list); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		fmt.Printf("✅ 已添加服务器: %s (%s@%s:%d)\n", name, user, host, port)
		return nil
	},
}

var CmdServerDelete = &cobra.Command{
	Use:   "delete <name>",
	Short: "删除独立服务器",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		list := loadServerList()
		idx := -1
		for i, s := range list.Servers {
			if s.Name == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("服务器不存在: %s", name)
		}
		list.Servers = append(list.Servers[:idx], list.Servers[idx+1:]...)
		if err := saveServerList(list); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		fmt.Printf("🗑️ 已删除服务器: %s\n", name)
		return nil
	},
}

// --- log 子命令 ---

var CmdLog = &cobra.Command{
	Use:   "log <command>",
	Short: "审计日志操作",
}

var CmdLogQuery = &cobra.Command{
	Use:   "query",
	Short: "查询审计日志",
	RunE: func(cmd *cobra.Command, args []string) error {
		date, _ := cmd.Flags().GetString("date")
		level, _ := cmd.Flags().GetString("level")
		keyword, _ := cmd.Flags().GetString("keyword")
		limit, _ := cmd.Flags().GetInt("limit")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		if limit <= 0 {
			limit = 100
		}

		results := queryAuditLogs(date, level, keyword, limit)
		if jsonOutput {
			data, _ := json.MarshalIndent(results, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		if len(results) == 0 {
			fmt.Println("📭 无匹配日志")
			return nil
		}
		fmt.Printf("📋 审计日志 (%d 条)\n", len(results))
		fmt.Println(strings.Repeat("─", 80))
		for _, r := range results {
			icon := map[string]string{"error": "❌", "warn": "⚠️", "info": "ℹ️"}
			fmt.Printf("[%s] %s %s\n", r["time"], icon[r["level"]], r["message"])
		}
		return nil
	},
}

var CmdLogDates = &cobra.Command{
	Use:   "dates",
	Short: "列出有审计日志的日期",
	RunE: func(cmd *cobra.Command, args []string) error {
		ciDir := ciDir()
		if ciDir == "" {
			return fmt.Errorf("找不到 ci-cd 目录")
		}
		logsDir := filepath.Join(ciDir, "logs")
		entries, err := os.ReadDir(logsDir)
		if err != nil {
			return fmt.Errorf("读取日志目录失败: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".jsonl") {
				date := strings.TrimPrefix(name, "audit-")
				date = strings.TrimSuffix(date, ".jsonl")
				fmt.Println(date)
			}
		}
		return nil
	},
}

var CmdLogDelete = &cobra.Command{
	Use:   "delete --date <date>",
	Short: "删除指定日期的审计日志",
	RunE: func(cmd *cobra.Command, args []string) error {
		date, _ := cmd.Flags().GetString("date")
		if date == "" {
			return fmt.Errorf("请指定 --date (格式: 2026-06-19)")
		}
		ciDir := ciDir()
		if ciDir == "" {
			return fmt.Errorf("找不到 ci-cd 目录")
		}
		fpath := filepath.Join(ciDir, "logs", fmt.Sprintf("audit-%s.jsonl", date))
		if err := os.Remove(fpath); err != nil {
			return fmt.Errorf("删除失败: %w", err)
		}
		fmt.Printf("🗑️ 已删除 %s 的审计日志\n", date)
		return nil
	},
}

// --- report all 子命令 ---

var CmdReportAll = &cobra.Command{
	Use:   "all",
	Short: "列出所有项目的测试报告",
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword, _ := cmd.Flags().GetString("keyword")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		ciDir := ciDir()
		if ciDir == "" {
			return fmt.Errorf("找不到 ci-cd 目录")
		}
		reportsRoot := filepath.Join(ciDir, "reports")
		projectDirs, err := os.ReadDir(reportsRoot)
		if err != nil {
			return fmt.Errorf("读取报告目录失败: %w", err)
		}

		type reportItem struct {
			Project   string `json:"project"`
			Timestamp string `json:"timestamp"`
			Status    string `json:"status"`
			Total     int    `json:"total"`
			Passed    int    `json:"passed"`
			Failed    int    `json:"failed"`
			Coverage  string `json:"coverage,omitempty"`
		}
		var allReports []reportItem
		for _, pDir := range projectDirs {
			if !pDir.IsDir() {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(pDir.Name()), strings.ToLower(keyword)) {
				continue
			}
			pattern := filepath.Join(reportsRoot, pDir.Name(), "test-*.json")
			files, err := filepath.Glob(pattern)
			if err != nil {
				continue
			}
			for _, f := range files {
				name := filepath.Base(f)
				id := strings.TrimSuffix(name, ".json")
				ts := ""
				if len(id) > 5 {
					ts = id[5:]
				}
				data, err := os.ReadFile(f)
				if err != nil {
					continue
				}
				var res struct {
					Status string `json:"status"`
					Report *struct {
						Total    int    `json:"total"`
						Passed   int    `json:"passed"`
						Failed   int    `json:"failed"`
						Coverage string `json:"coverage"`
					} `json:"report"`
				}
				if err := json.Unmarshal(data, &res); err != nil {
					continue
				}
				item := reportItem{
					Project:   pDir.Name(),
					Timestamp: ts,
					Status:    res.Status,
				}
				if res.Report != nil {
					item.Total = res.Report.Total
					item.Passed = res.Report.Passed
					item.Failed = res.Report.Failed
					item.Coverage = res.Report.Coverage
				}
				allReports = append(allReports, item)
			}
		}
		sort.Slice(allReports, func(i, j int) bool {
			return allReports[i].Timestamp > allReports[j].Timestamp
		})

		if jsonOutput {
			data, _ := json.MarshalIndent(allReports, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		if len(allReports) == 0 {
			fmt.Println("📭 无测试报告")
			return nil
		}
		fmt.Printf("📊 测试报告 (%d 条)\n", len(allReports))
		fmt.Println(strings.Repeat("─", 90))
		fmt.Printf("%-20s %-5s %-6s %-6s %-6s %-6s %s\n", "项目", "状态", "总数", "通过", "失败", "覆盖率", "时间")
		fmt.Println(strings.Repeat("─", 90))
		for _, r := range allReports {
			status := "✅"
			if r.Status == "fail" {
				status = "❌"
			}
			fmt.Printf("%-20s %-5s %-6d %-6d %-6d %-6s %s\n", r.Project, status, r.Total, r.Passed, r.Failed, r.Coverage, r.Timestamp)
		}
		return nil
	},
}

// ========== 命令注册（在 init 中注册标志） ==========

func init() {
	// remote
	CmdRemote.AddCommand(CmdRemoteLs)
	CmdRemote.AddCommand(CmdRemoteDownload)
	CmdRemote.AddCommand(CmdRemoteUpload)
	CmdRemote.AddCommand(CmdRemoteDelete)
	CmdRemote.AddCommand(CmdRemoteMkdir)

	CmdRemoteLs.Flags().String("source", "project", "服务器来源: project/standalone")
	CmdRemoteLs.Flags().String("path", "/", "远程路径")
	CmdRemoteDownload.Flags().String("source", "project", "服务器来源: project/standalone")
	CmdRemoteDownload.Flags().String("path", "", "远程文件路径（必填）")
	CmdRemoteUpload.Flags().String("source", "project", "服务器来源: project/standalone")
	CmdRemoteUpload.Flags().String("file", "", "本地文件路径（必填）")
	CmdRemoteUpload.Flags().String("path", "", "远程目录路径（必填）")
	CmdRemoteDelete.Flags().String("source", "project", "服务器来源: project/standalone")
	CmdRemoteDelete.Flags().String("path", "", "远程路径（必填）")
	CmdRemoteMkdir.Flags().String("source", "project", "服务器来源: project/standalone")
	CmdRemoteMkdir.Flags().String("path", "", "远程路径（必填）")

	// server
	CmdServer.AddCommand(CmdServerList)
	CmdServer.AddCommand(CmdServerAdd)
	CmdServer.AddCommand(CmdServerDelete)

	CmdServerAdd.Flags().String("name", "", "服务器名称（必填）")
	CmdServerAdd.Flags().String("host", "", "主机地址（必填）")
	CmdServerAdd.Flags().String("user", "", "用户名（必填）")
	CmdServerAdd.Flags().Int("port", 22, "SSH 端口")
	CmdServerAdd.Flags().String("auth-type", "key", "认证方式: key/password")
	CmdServerAdd.Flags().String("key-path", "", "SSH 密钥路径")
	CmdServerAdd.Flags().String("password", "", "SSH 密码")
	CmdServerAdd.Flags().String("note", "", "备注")

	// log
	CmdLog.AddCommand(CmdLogQuery)
	CmdLog.AddCommand(CmdLogDates)
	CmdLog.AddCommand(CmdLogDelete)

	CmdLogQuery.Flags().String("date", "", "日期 (YYYY-MM-DD)，默认今天")
	CmdLogQuery.Flags().String("level", "", "级别过滤: info/warn/error")
	CmdLogQuery.Flags().String("keyword", "", "关键字搜索")
	CmdLogQuery.Flags().Int("limit", 100, "返回条数")
	CmdLogQuery.Flags().Bool("json", false, "JSON 格式输出")
	CmdLogDelete.Flags().String("date", "", "日期 (YYYY-MM-DD)（必填）")

	// report all
	CmdReport.AddCommand(CmdReportAll)
	CmdReportAll.Flags().String("keyword", "", "按项目名搜索")
	CmdReportAll.Flags().Bool("json", false, "JSON 格式输出")
}
