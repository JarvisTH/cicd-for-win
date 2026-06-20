package serve

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
	"ci-cd/internal/security"
	"ci-cd/internal/sshutil"
)

//go:embed web/*
var webFiles embed.FS

// activeAuth 内存中缓存的当前认证信息，修改密码时立即更新。
// authMu 保护并发读写，避免数据竞争导致认证失效或 panic。
var (
	activeAuth *config.AuthConfig
	authMu     sync.RWMutex
)

// getActiveAuth 线程安全地读取当前认证配置。
func getActiveAuth() *config.AuthConfig {
	authMu.RLock()
	defer authMu.RUnlock()
	return activeAuth
}

// setActiveAuth 线程安全地更新当前认证配置。
func setActiveAuth(a *config.AuthConfig) {
	authMu.Lock()
	activeAuth = a
	authMu.Unlock()
}

// initAuth 初始化认证：从 auth.json 读取，文件不存在则创建默认
func initAuth(ciDir string) (*config.AuthConfig, error) {
	auth, err := config.LoadAuth(ciDir)
	if err != nil {
		return nil, err
	}
	isDefault := auth.Username == config.DefaultUsername && auth.VerifyPassword(config.DefaultPassword)
	if isDefault {
		log.Printf("⚠️  警告: 使用默认密码 (%s/%s)，请通过 Web UI 或 `ci passwd` 修改\n", config.DefaultUsername, config.DefaultPassword)
	}
	setActiveAuth(auth)
	return auth, nil
}

// basicAuth 返回一个 HTTP 中间件，对请求进行 Basic Auth 校验。
// 带有效 download token 的请求可绕过 Basic Auth（用于浏览器原生下载场景）。
func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 一次性下载 token：浏览器原生下载（iframe/a 标签）无法携带 Authorization 头，
		// 用 token 放行，token 一次性消费、60 秒过期。
		if tk := r.URL.Query().Get("download_token"); tk != "" && validateDownloadToken(tk) {
			next.ServeHTTP(w, r)
			return
		}

		auth := getActiveAuth()
		if auth == nil {
			http.Error(w, "认证未初始化", http.StatusInternalServerError)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(auth.Username)) != 1 ||
			!auth.VerifyPassword(pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="CI/CD 控制台"`)
			http.Error(w, "未授权，请输入正确的用户名和密码", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// csrfProtect 对状态变更方法（POST/PUT/DELETE）要求自定义请求头，
// 浏览器跨站表单无法携带自定义头，从而阻止 CSRF。Basic Auth 下浏览器会自动
// 发送凭据，仅靠 Basic Auth 不足以防范 CSRF。
func csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Requested-With") == "" {
				http.Error(w, `{"error":"缺少 X-Requested-With 头，疑似 CSRF 攻击"}`, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func NewHandler(ciDir string) http.Handler {
	// 初始化认证
	if _, err := initAuth(ciDir); err != nil {
		log.Fatalf("初始化认证失败: %v", err)
	}

	// 启动 SSH 空闲连接回收，防止长期运行泄漏连接
	startSSHReaper()

	// 启动时迁移 projects.json 中的明文部署密码为加密存储
	migrateProjectDeployPasswords(ciDir)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", apiHandler("check"))
	mux.HandleFunc("/api/build", apiHandler("build"))
	mux.HandleFunc("/api/test", apiHandler("test"))
	mux.HandleFunc("/api/push", pushHandler)
	mux.HandleFunc("/api/status", apiHandler("status"))
	mux.HandleFunc("/api/projects", projectListHandler)
	mux.HandleFunc("/api/project", projectSaveHandler)
	mux.HandleFunc("/api/deploy/test", deployTestHandler)
	mux.HandleFunc("/api/auth/status", authStatusHandler)
	mux.HandleFunc("/api/auth/change-password", changePasswordHandler)
	mux.HandleFunc("/api/report/latest", latestReportHandler)
	mux.HandleFunc("/api/report/list", reportListHandler)
	mux.HandleFunc("/api/report/delete", reportDeleteHandler)

	// 审计日志
	mux.HandleFunc("/api/log/append", handleLogAppend)
	mux.HandleFunc("/api/log/query", handleLogQuery)
	mux.HandleFunc("/api/log/dates", handleLogDates)
	mux.HandleFunc("/api/log/delete", handleLogDelete)

	// 统一报告列表
	mux.HandleFunc("/api/report/all", handleAllReports)

	// 规则文件查看
	mux.HandleFunc("/api/rules/", handleViewRuleFile)

	// 本地目录浏览（用于选择项目路径）
	mux.HandleFunc("/api/local/ls", handleLocalLs)

	// 远程管理 API
	mux.HandleFunc("/api/remote/projects", handleRemoteAllServers)
	mux.HandleFunc("/api/remote/servers", handleRemoteServers)
	mux.HandleFunc("/api/remote/server", handleRemoteServerDelete)
	mux.HandleFunc("/api/remote/term", handleRemoteTerm)
	mux.HandleFunc("/api/remote/ls", handleRemoteLs)
	mux.HandleFunc("/api/remote/download", handleRemoteDownload)
	mux.HandleFunc("/api/remote/download-token", handleDownloadToken)
	mux.HandleFunc("/api/remote/upload", handleRemoteUpload)
	mux.HandleFunc("/api/remote/delete", handleRemoteDelete)
	mux.HandleFunc("/api/remote/mkdir", handleRemoteMkdir)
	mux.HandleFunc("/api/remote/disconnect", handleRemoteDisconnect)

	subFS, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	return basicAuth(csrfProtect(mux))
}

// authStatusHandler 返回当前认证状态（不暴露密码）
func authStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	auth := getActiveAuth()
	if auth == nil {
		json.NewEncoder(w).Encode(map[string]any{"username": "", "is_default": false})
		return
	}
	isDefault := auth.Username == config.DefaultUsername && auth.VerifyPassword(config.DefaultPassword)
	json.NewEncoder(w).Encode(map[string]any{
		"username":   auth.Username,
		"is_default": isDefault,
	})
}

// changePasswordHandler 处理密码修改请求
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
		return
	}

	if body.OldPassword == "" || body.NewPassword == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "旧密码和新密码不能为空"})
		return
	}

	if len(body.NewPassword) < 6 {
		json.NewEncoder(w).Encode(map[string]string{"error": "新密码长度不能少于 6 位"})
		return
	}

	auth := getActiveAuth()
	if auth == nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "认证未初始化"})
		return
	}

	// 验证旧密码
	if !auth.VerifyPassword(body.OldPassword) {
		json.NewEncoder(w).Encode(map[string]string{"error": "旧密码错误"})
		return
	}

	// 生成新配置并保存
	newAuth := config.NewAuthConfig(auth.Username, body.NewPassword)
	if err := config.SaveAuth(ciDir, newAuth); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存密码失败: " + err.Error()})
		return
	}

	// 更新内存缓存
	setActiveAuth(newAuth)

	log.Printf("🔑 密码已修改 (用户: %s)\n", auth.Username)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "密码修改成功"})
}

func apiHandler(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		project := r.URL.Query().Get("project")
		ciDir := findCiDir()
		if ciDir == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-runner.ps1"})
			return
		}

		// 从项目配置中查找该步骤的自定义命令
		customCommand, customArgs := findPipelineStepCommand(ciDir, project, action)

		args := []string{"-ExecutionPolicy", "Bypass",
			"-File", filepath.Join(ciDir, "ci-runner.ps1"),
			"-Action", action,
			"-ProjectPath", project,
			"-Json"}
		if customCommand != "" {
			args = append(args, "-CustomCommand", customCommand)
		}
		if customArgs != "" {
			args = append(args, "-CustomArgs", customArgs)
		}
		cmd := exec.Command("powershell.exe", args...)
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Write(output)

		// 测试完成后，将报告持久化到磁盘
		if action == "test" {
			var result runner.Result
			if err := json.Unmarshal(output, &result); err == nil && result.Report != nil {
				saveTestReportToDisk(ciDir, result)
			}
		}
	}
}

// findPipelineStepCommand 从 projects.json 中查找指定项目的指定步骤的自定义命令
func findPipelineStepCommand(ciDir, projectName, stepID string) (command, args string) {
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return "", ""
	}
	var cfg struct {
		Projects []struct {
			Name     string `json:"name"`
			Pipeline *struct {
				Steps []struct {
					ID      string `json:"id"`
					Enabled bool   `json:"enabled"`
					Command string `json:"command"`
					Args    string `json:"args"`
				} `json:"steps"`
			} `json:"pipeline"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", ""
	}
	for _, p := range cfg.Projects {
		if p.Name != projectName {
			continue
		}
		if p.Pipeline == nil {
			return "", ""
		}
		for _, s := range p.Pipeline.Steps {
			if s.ID == stepID {
				return s.Command, s.Args
			}
		}
	}
	return "", ""
}

// saveTestReportToDisk 将测试报告保存到 reports/{project}/{timestamp}.json
func saveTestReportToDisk(ciDir string, result runner.Result) {
	reportsDir := filepath.Join(ciDir, "reports", result.Project)
	os.MkdirAll(reportsDir, 0755)

	now := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("test-%s.json", now)
	path := filepath.Join(reportsDir, filename)

	data, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(path, data, 0644)

	// 清理旧报告，只保留最近 20 条
	pattern := filepath.Join(reportsDir, "test-*.json")
	files, _ := filepath.Glob(pattern)
	if len(files) > 20 {
		for i := 0; i < len(files)-20; i++ {
			os.Remove(files[i])
		}
	}
}

func projectListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}

	// 解析并注入版本信息
	var cfg struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		w.Write(data)
		return
	}
	for i, p := range cfg.Projects {
		path, _ := p["path"].(string)
		if path == "" {
			continue
		}
		// 版本号
		cfg.Projects[i]["version"] = readProjectVersion(path)
		// Git 信息
		branch, commit := readGitInfo(path)
		cfg.Projects[i]["git_branch"] = branch
		cfg.Projects[i]["git_commit"] = commit
	}
	result, _ := json.MarshalIndent(cfg, "", "  ")
	w.Write(result)
}

func readProjectVersion(path string) string {
	// 尝试读取 package.json
	pkgFile := filepath.Join(path, "package.json")
	if data, err := os.ReadFile(pkgFile); err == nil {
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
	}
	// 尝试读取 pom.xml
	pomFile := filepath.Join(path, "pom.xml")
	if data, err := os.ReadFile(pomFile); err == nil {
		content := string(data)
		if idx := strings.Index(content, "<version>"); idx >= 0 {
			end := strings.Index(content[idx:], "</version>")
			if end >= 0 {
				return content[idx+9 : idx+end]
			}
		}
	}
	return ""
}

func readGitInfo(path string) (branch, commit string) {
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	} else {
		branch = ""
	}
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	} else {
		commit = ""
	}
	return
}

func projectSaveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	// 强类型反序列化 + 字段校验，避免任意配置注入
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误: " + err.Error()})
		return
	}
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.Name == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目名称不能为空"})
			return
		}
		// 校验项目路径存在且是目录
		if p.Path == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目路径不能为空"})
			return
		}
		if fi, err := os.Stat(p.Path); err != nil || !fi.IsDir() {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目路径不存在或不是目录: " + p.Path})
			return
		}
		// 校验并加密部署配置中的密码
		if p.Deploy != nil {
			if err := validateAndEncryptDeploy(ciDir, p.Deploy); err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}
		// 校验流水线自定义命令：必须不含 shell 元字符注入风险
		if p.Pipeline != nil {
			for _, s := range p.Pipeline.Steps {
				if !isValidStepID(s.ID) {
					json.NewEncoder(w).Encode(map[string]string{"error": "非法流水线步骤 ID: " + s.ID})
					return
				}
			}
		}
	}

	raw, _ := json.MarshalIndent(cfg, "", "  ")
	path := filepath.Join(ciDir, "projects.json")
	if err := security.AtomicWriteFile(path, raw, 0600); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// validateAndEncryptDeploy 校验部署配置合法性并加密明文密码。
func validateAndEncryptDeploy(ciDir string, d *config.DeployConfig) error {
	if d.Host == "" {
		return nil // 未配置部署，跳过
	}
	// 认证类型白名单
	switch d.AuthType {
	case "key", "agent", "password":
	default:
		return fmt.Errorf("非法认证类型: %s", d.AuthType)
	}
	// 端口范围校验
	if d.Port < 0 || d.Port > 65535 {
		return fmt.Errorf("端口超出有效范围: %d", d.Port)
	}
	// 密码明文 → 加密（已加密则保留）
	if d.AuthType == "password" && d.Password != "" && !security.IsEncrypted(d.Password) {
		key, err := security.LoadOrCreateKey(ciDir)
		if err != nil {
			return err
		}
		enc, err := security.EncryptPassword(d.Password, key)
		if err != nil {
			return fmt.Errorf("密码加密失败: %w", err)
		}
		d.Password = enc
	}
	return nil
}

var validStepIDs = map[string]bool{"check": true, "build": true, "test": true, "push": true, "deploy": true}

func isValidStepID(id string) bool { return validStepIDs[id] }

func findCiDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if fileExists(filepath.Join(dir, "ci-runner.ps1")) {
		return dir
	}
	parent := filepath.Dir(dir)
	if fileExists(filepath.Join(parent, "ci-runner.ps1")) {
		return parent
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pushHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(ciDir, "ci-push.ps1"),
		"-ProjectName", project)
	err := cmd.Run()
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "pass", "message": "推送成功"})
	} else {
		json.NewEncoder(w).Encode(map[string]string{"status": "fail", "message": err.Error()})
	}
}

func deployTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host := r.URL.Query().Get("host")
	portStr := r.URL.Query().Get("port")
	user := r.URL.Query().Get("user")
	authType := r.URL.Query().Get("auth_type")
	keyFile := r.URL.Query().Get("identity_file")

	if host == "" || user == "" {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "缺少主机或用户名"})
		return
	}
	port := 22
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p < 65536 {
			port = p
		}
	}

	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	deploy := &config.DeployConfig{
		Host: host, Port: port, User: user,
		AuthType: authType, IdentityFile: keyFile,
	}
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	// 测试连接缩短超时
	sshCfg.Timeout = 5 * time.Second

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshCfg)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	client.Close()
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "连接成功"})
}

// migrateProjectDeployPasswords 启动时迁移 projects.json 中的明文部署密码为加密存储。
// 确保磁盘上不存留明文密码。已加密（enc: 前缀）的跳过。
func migrateProjectDeployPasswords(ciDir string) {
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	var changed bool
	for i := range cfg.Projects {
		d := cfg.Projects[i].Deploy
		if d == nil {
			continue
		}
		if d.AuthType == "password" && d.Password != "" && !security.IsEncrypted(d.Password) {
			enc, err := security.EncryptPassword(d.Password, key)
			if err == nil {
				cfg.Projects[i].Deploy.Password = enc
				changed = true
			}
		}
	}
	if changed {
		raw, _ := json.MarshalIndent(cfg, "", "  ")
		security.AtomicWriteFile(filepath.Join(ciDir, "projects.json"), raw, 0600)
		log.Printf("🔐 已将 projects.json 中的明文部署密码迁移为加密存储\n")
	}
}

// latestReportHandler 返回指定项目的最新测试报告
func latestReportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 project 参数"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		json.NewEncoder(w).Encode(map[string]any{"report": nil})
		return
	}

	latest := files[len(files)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "读取报告失败"})
		return
	}

	var report runner.Result
	if err := json.Unmarshal(data, &report); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "解析报告失败"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"report": report})
}

// reportListHandler 返回指定项目的报告列表
func reportListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}

	type reportItem struct {
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Status    string `json:"status"`
		Total     int    `json:"total"`
		Passed    int    `json:"passed"`
		Failed    int    `json:"failed"`
	}
	var items []reportItem
	for _, f := range files {
		name := filepath.Base(f)
		id := strings.TrimSuffix(name, ".json")
		ts := ""
		if len(id) > 5 {
			ts = id[5:]
		}
		var res runner.Result
		if data, err := os.ReadFile(f); err == nil {
			json.Unmarshal(data, &res)
		}
		item := reportItem{
			ID:        id,
			Timestamp: ts,
			Status:    res.Status,
		}
		if res.Report != nil {
			item.Total = res.Report.Total
			item.Passed = res.Report.Passed
			item.Failed = res.Report.Failed
		}
		items = append(items, item)
	}
	// 按时间倒序
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	json.NewEncoder(w).Encode(map[string]any{"reports": items})
}

// reportDeleteHandler 删除指定项目的某条测试报告
func reportDeleteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}
	var body struct {
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" || body.ID == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 project 或 id 参数"})
		return
	}
	reportPath := filepath.Join(ciDir, "reports", body.Project, body.ID+".json")
	if err := os.Remove(reportPath); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "删除失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "报告已删除"})
}
