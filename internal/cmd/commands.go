package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"ci-cd/internal/config"
	"ci-cd/internal/output"
	"ci-cd/internal/runner"
)

var jsonOutput bool

var CmdCheck = &cobra.Command{
	Use:   "check [project]",
	Short: "对项目执行代码检查（TypeScript 类型检查 + ESLint）",
	Long:  `对指定项目执行代码检查。不指定 project 则检查所有已启用项目。`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		projects := cfg.Filter(args)
		if len(projects) == 0 {
			return fmt.Errorf("没有找到匹配的项目")
		}
		results := []runner.Result{}
		for _, p := range projects {
			result, err := runner.RunCheck(p)
			results = append(results, result)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ❌ %v\n", p.Name, err)
			}
		}
		return output.Format(cmd, results, jsonOutput)
	},
}

var CmdBuild = &cobra.Command{
	Use:   "build [project]",
	Short: "对项目执行完整构建",
	Long:  `对指定项目执行完整构建。不指定 project 则构建所有项目。`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		projects := cfg.Filter(args)
		if len(projects) == 0 {
			return fmt.Errorf("没有找到匹配的项目")
		}
		results := []runner.Result{}
		for _, p := range projects {
			result, err := runner.RunBuild(p)
			results = append(results, result)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ❌ %v\n", p.Name, err)
			}
		}
		return output.Format(cmd, results, jsonOutput)
	},
}

var CmdTest = &cobra.Command{
	Use:   "test [project]",
	Short: "对项目执行单元测试",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		results := []runner.Result{}
		for _, p := range cfg.Filter(args) {
			result, err := runner.RunTest(p)
			results = append(results, result)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ❌ %v\n", p.Name, err)
			}
		}
		return output.Format(cmd, results, jsonOutput)
	},
}

var CmdPush = &cobra.Command{
	Use:   "push [project]",
	Short: "推送到所有 Git 远程仓库",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunPush(p)
		}
		return nil
	},
}

var CmdDeploy = &cobra.Command{
	Use:   "deploy [project]",
	Short: "将项目部署到远程服务器",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		target, _ := cmd.Flags().GetString("target")
		for _, p := range cfg.Filter(args) {
			runner.RunDeploy(p, target)
		}
		return nil
	},
}

var CmdHooks = &cobra.Command{
	Use:   "hooks [project]",
	Short: "安装 Git hooks 到项目",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunHooks(p)
		}
		return nil
	},
}

var CmdList = &cobra.Command{
	Use:   "list",
	Short: "列出所有项目及状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		for _, p := range cfg.Projects {
			status := "⚪"
			if !p.Enabled {
				status = "🔘 禁用"
			}
			fmt.Printf("%s %-25s %s\n", status, p.Name, p.Path)
		}
		return nil
	},
}

var CmdStatus = &cobra.Command{
	Use:   "status [project]",
	Short: "查看项目当前状态",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("projects.json")
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunStatus(p)
		}
		return nil
	},
}

var CmdDescribe = &cobra.Command{
	Use:   "describe",
	Short: "输出工具 Schema（供 LLM/AI Agent 发现）",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		schema := runner.GenerateSchema(format)
		fmt.Println(schema)
		return nil
	},
}

var CmdPasswd = &cobra.Command{
	Use:   "passwd [username] [password]",
	Short: "修改或重置 Web UI 登录密码",
	Long: `修改 Web UI 的 Basic Auth 登录密码。

不传参数时重置为默认密码 (admin/123456)。
示例:
  ci passwd                   重置为默认 admin/123456
  ci passwd admin myNewPass   修改密码
`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("无法获取 ci.exe 路径: %w", err)
		}
		ciDir := filepath.Dir(exe)

		username := config.DefaultUsername
		password := config.DefaultPassword

		if len(args) >= 1 {
			username = args[0]
		}
		if len(args) >= 2 {
			password = args[1]
		}

		if len(password) < 6 {
			return fmt.Errorf("密码长度不能少于 6 位")
		}

		auth := config.NewAuthConfig(username, password)
		if err := config.SaveAuth(ciDir, auth); err != nil {
			return fmt.Errorf("保存密码失败: %w", err)
		}

		fmt.Printf("✅ 密码已更新 — 用户名: %s  密码: %s\n", username, password)
		fmt.Printf("   文件: %s\n", filepath.Join(ciDir, config.AuthFileName))
		if password == config.DefaultPassword {
			fmt.Println("⚠️  警告: 正在使用默认密码，建议通过 `ci passwd <用户名> <密码>` 修改")
		}
		return nil
	},
}

var CmdReport = &cobra.Command{
	Use:   "report [project]",
	Short: "查看项目最新测试报告",
	Long: `查看指定项目的最新测试报告。
示例:
  ci report pair-front         查看最新测试报告
  ci report pair-front --list  列出所有历史报告
  ci report pair-front --json  输出 JSON 格式
  ci report pair-front --delete test-20260619-095000  删除指定报告
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		listMode, _ := cmd.Flags().GetBool("list")
		jsonMode, _ := cmd.Flags().GetBool("json")
		deleteID, _ := cmd.Flags().GetString("delete")

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("无法获取 ci.exe 路径: %w", err)
		}
		ciDir := filepath.Dir(exe)
		reportsDir := filepath.Join(ciDir, "reports", projectName)

		// 删除模式
		if deleteID != "" {
			reportPath := filepath.Join(reportsDir, deleteID+".json")
			if err := os.Remove(reportPath); err != nil {
				return fmt.Errorf("删除失败: %w", err)
			}
			fmt.Printf("🗑️ 已删除报告: %s/%s\n", projectName, deleteID)
			return nil
		}

		if listMode {
			pattern := filepath.Join(reportsDir, "*.json")
			files, err := filepath.Glob(pattern)
			if err != nil || len(files) == 0 {
				fmt.Printf("📭 [%s] 无测试报告\n", projectName)
				return nil
			}
			fmt.Printf("📋 [%s] 历史报告:\n", projectName)
			for _, f := range files {
				name := filepath.Base(f)
				var res runner.Result
				if data, err := os.ReadFile(f); err == nil {
					json.Unmarshal(data, &res)
				}
				status := "✅"
				if res.Status != "pass" {
					status = "❌"
				}
				reportInfo := ""
				if res.Report != nil {
					reportInfo = fmt.Sprintf(" (%d/%d 通过, 覆盖率: %s)", res.Report.Passed, res.Report.Total, res.Report.Coverage)
				}
				id := name[:len(name)-5] // 去掉 .json
				fmt.Printf("  %s %-40s %s\n", status, id, reportInfo)
			}
			return nil
		}

		// 读取最新报告
		pattern := filepath.Join(reportsDir, "*.json")
		files, err := filepath.Glob(pattern)
		if err != nil || len(files) == 0 {
			fmt.Printf("📭 [%s] 无测试报告，请先执行 ci test %s\n", projectName, projectName)
			return nil
		}
		latest := files[len(files)-1]
		data, err := os.ReadFile(latest)
		if err != nil {
			return fmt.Errorf("读取报告失败: %w", err)
		}

		if jsonMode {
			fmt.Println(string(data))
			return nil
		}

		var res runner.Result
		json.Unmarshal(data, &res)
		fmt.Printf("📊 [%s] 测试报告\n", projectName)
		fmt.Printf("   状态:   ")
		if res.Status == "pass" {
			fmt.Print("✅ 通过\n")
		} else {
			fmt.Print("❌ 失败\n")
		}
		if res.Report != nil {
			r := res.Report
			fmt.Printf("   总数:   %d\n", r.Total)
			fmt.Printf("   通过:   %d\n", r.Passed)
			fmt.Printf("   失败:   %d\n", r.Failed)
			fmt.Printf("   跳过:   %d\n", r.Skipped)
			if r.Coverage != "" {
				fmt.Printf("   覆盖率: %s\n", r.Coverage)
			}
			if len(r.Failures) > 0 {
				fmt.Printf("   失败详情:\n")
				for _, f := range r.Failures {
					fmt.Printf("     ❌ [%s] %s\n", f.Suite, f.Test)
					fmt.Printf("        %s\n", f.Message)
				}
			}
		}
		return nil
	},
}

// CmdDoctor 环境诊断命令
var CmdDoctor = &cobra.Command{
	Use:   "doctor",
	Short: "诊断 CI/CD 环境状态",
	Long: `诊断当前 CI/CD 环境，检查工具链和项目配置完整性。
示例:
  ci doctor          输出诊断结果
  ci doctor --json   输出 JSON 格式
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")

		type checkItem struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Message string `json:"message"`
		}

		checks := []checkItem{}

		// 检查 Go
		if _, err := exec.LookPath("go"); err == nil {
			checks = append(checks, checkItem{Name: "Go", Status: "ok", Message: "已安装"})
		} else {
			checks = append(checks, checkItem{Name: "Go", Status: "warn", Message: "未找到"})
		}

		// 检查 Git
		if _, err := exec.LookPath("git.exe"); err == nil {
			checks = append(checks, checkItem{Name: "Git", Status: "ok", Message: "已安装"})
		} else {
			checks = append(checks, checkItem{Name: "Git", Status: "warn", Message: "未找到"})
		}

		// 检查 Node.js
		if _, err := exec.LookPath("node"); err == nil {
			checks = append(checks, checkItem{Name: "Node.js", Status: "ok", Message: "已安装"})
		} else {
			checks = append(checks, checkItem{Name: "Node.js", Status: "warn", Message: "未找到"})
		}

		// 检查 Java/Maven
		if _, err := exec.LookPath("mvn.cmd"); err == nil {
			checks = append(checks, checkItem{Name: "Maven", Status: "ok", Message: "已安装"})
		} else if _, err := exec.LookPath("java"); err == nil {
			checks = append(checks, checkItem{Name: "Java", Status: "ok", Message: "已安装（未找到 Maven）"})
		} else {
			checks = append(checks, checkItem{Name: "Java", Status: "warn", Message: "未找到"})
		}

		// 检查 ci-runner.ps1
		exe, _ := os.Executable()
		ciDir := filepath.Dir(exe)
		runnerPath := filepath.Join(ciDir, "ci-runner.ps1")
		if _, err := os.Stat(runnerPath); err == nil {
			checks = append(checks, checkItem{Name: "ci-runner.ps1", Status: "ok", Message: "存在"})
		} else {
			checks = append(checks, checkItem{Name: "ci-runner.ps1", Status: "error", Message: "缺失"})
		}

		// 检查 auth.json
		authPath := filepath.Join(ciDir, "auth.json")
		if _, err := os.Stat(authPath); err == nil {
			checks = append(checks, checkItem{Name: "auth.json", Status: "ok", Message: "存在"})
		} else {
			checks = append(checks, checkItem{Name: "auth.json", Status: "warn", Message: "未初始化"})
		}

		// 检查项目配置
		cfg, err := config.Load("projects.json")
		projectCount := 0
		enabledCount := 0
		deployReady := 0
		if err == nil {
			projectCount = len(cfg.Projects)
			for _, p := range cfg.Projects {
				if p.Enabled {
					enabledCount++
				}
				if p.Deploy != nil && p.Deploy.Host != "" {
					deployReady++
				}
			}
		}
		checks = append(checks, checkItem{Name: "项目配置", Status: "ok", Message: fmt.Sprintf("%d 个项目, %d 启用, %d 已配置部署", projectCount, enabledCount, deployReady)})

		if jsonMode {
			data, _ := json.MarshalIndent(checks, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		fmt.Println("🏥 CI/CD 环境诊断")
		fmt.Println(strings.Repeat("─", 50))
		for _, c := range checks {
			icon := "✅"
			if c.Status == "warn" {
				icon = "⚠️"
			} else if c.Status == "error" {
				icon = "❌"
			}
			fmt.Printf("  %s %-20s %s\n", icon, c.Name, c.Message)
		}
		fmt.Println(strings.Repeat("─", 50))
		hasError := false
		hasWarn := false
		for _, c := range checks {
			if c.Status == "error" {
				hasError = true
			}
			if c.Status == "warn" {
				hasWarn = true
			}
		}
		if hasError {
			fmt.Println("❌ 存在严重问题，请修复后重试")
		} else if hasWarn {
			fmt.Println("⚠️ 部分环境未完整安装，但核心功能可用")
		} else {
			fmt.Println("✅ 环境正常")
		}
		return nil
	},
}

// CmdProjectList 增强的项目列表
var CmdProjectList = &cobra.Command{
	Use:   "project list",
	Short: "列出所有项目的详细信息",
	Long: `列出所有项目的详细信息，包括路径、类型、构建状态、部署配置和 Git 信息。
示例:
  ci project list             列出所有项目详情
  ci project list --json      输出 JSON 格式
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")
		cfg, err := config.Load("projects.json")
		if err != nil {
			return fmt.Errorf("读取项目配置失败: %w", err)
		}

		type projectDetail struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Enabled     bool   `json:"enabled"`
			Type        string `json:"type,omitempty"`
			Version     string `json:"version,omitempty"`
			GitBranch   string `json:"git_branch,omitempty"`
			GitCommit   string `json:"git_commit,omitempty"`
			DeployHost  string `json:"deploy_host,omitempty"`
			HasDist     bool   `json:"has_dist"`
			RemoteCount int    `json:"remote_count"`
		}

		var details []projectDetail
		for _, p := range cfg.Projects {
			d := projectDetail{
				Name:    p.Name,
				Path:    p.Path,
				Enabled: p.Enabled,
			}
			if p.Deploy != nil {
				d.DeployHost = p.Deploy.Host
			}
			if p.Enabled {
				d.HasDist = runner.HasDist(p.Path)
				d.Version = runner.ReadProjectVersion(p.Path)
				branch, commit := runner.ReadGitInfo(p.Path)
				d.GitBranch = branch
				d.GitCommit = commit
			}
			details = append(details, d)
		}

		if jsonMode {
			data, _ := json.MarshalIndent(details, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("%-20s %-8s %-12s %s\n", "项目", "状态", "版本", "部署目标")
		fmt.Println(strings.Repeat("─", 80))
		for _, d := range details {
			status := "🔘 禁用"
			if d.Enabled {
				status = "✅ 启用"
			}
			deployHost := d.DeployHost
			if deployHost == "" {
				deployHost = "-"
			}
			ver := d.Version
			if ver == "" {
				ver = "-"
			}
			fmt.Printf("%-20s %-8s %-12s %s\n", d.Name, status, ver, deployHost)
		}
		return nil
	},
}

func init() {
	CmdDeploy.Flags().String("target", "production", "部署目标（staging/production）")
	CmdDescribe.Flags().String("format", "openai", "输出格式: openai/mcp/text")
	CmdReport.Flags().Bool("list", false, "列出所有历史报告")
	CmdReport.Flags().Bool("json", false, "输出 JSON 格式")
	CmdReport.Flags().String("delete", "", "删除指定 ID 的报告")
	CmdDoctor.Flags().Bool("json", false, "输出 JSON 格式")
	CmdProjectList.Flags().Bool("json", false, "输出 JSON 格式")
}
