package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"ci-cd/internal/config"
)

type checkItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
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
		return runDoctor(jsonMode)
	},
}

// runDoctor 实现 doctor 命令的业务逻辑，供 RunE 调用和独立测试。
func runDoctor(jsonMode bool) error {
	checks := runChecks()
	return printDoctorResults(os.Stdout, checks, jsonMode)
}

// runChecks 执行环境检查，返回检查项列表。可单独测试。
func runChecks() []checkItem {
	checks := []checkItem{}

	if _, err := exec.LookPath("go"); err == nil {
		checks = append(checks, checkItem{Name: "Go", Status: "ok", Message: "已安装"})
	} else {
		checks = append(checks, checkItem{Name: "Go", Status: "warn", Message: "未找到"})
	}

	if _, err := exec.LookPath("git.exe"); err == nil {
		checks = append(checks, checkItem{Name: "Git", Status: "ok", Message: "已安装"})
	} else {
		checks = append(checks, checkItem{Name: "Git", Status: "warn", Message: "未找到"})
	}

	if _, err := exec.LookPath("node"); err == nil {
		checks = append(checks, checkItem{Name: "Node.js", Status: "ok", Message: "已安装"})
	} else {
		checks = append(checks, checkItem{Name: "Node.js", Status: "warn", Message: "未找到"})
	}

	if _, err := exec.LookPath("mvn.cmd"); err == nil {
		checks = append(checks, checkItem{Name: "Maven", Status: "ok", Message: "已安装"})
	} else if _, err := exec.LookPath("java"); err == nil {
		checks = append(checks, checkItem{Name: "Java", Status: "ok", Message: "已安装（未找到 Maven）"})
	} else {
		checks = append(checks, checkItem{Name: "Java", Status: "warn", Message: "未找到"})
	}

	exe, _ := os.Executable()
	ciDir := filepath.Dir(exe)
	runnerPath := filepath.Join(ciDir, "ci-runner.ps1")
	if _, err := os.Stat(runnerPath); err == nil {
		checks = append(checks, checkItem{Name: "ci-runner.ps1", Status: "ok", Message: "存在"})
	} else {
		checks = append(checks, checkItem{Name: "ci-runner.ps1", Status: "error", Message: "缺失"})
	}

	authPath := filepath.Join(ciDir, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		checks = append(checks, checkItem{Name: "auth.json", Status: "ok", Message: "存在"})
	} else {
		checks = append(checks, checkItem{Name: "auth.json", Status: "warn", Message: "未初始化"})
	}

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

	return checks
}

// printDoctorResults 输出诊断结果到 w。可单独测试。
func printDoctorResults(w io.Writer, checks []checkItem, jsonMode bool) error {
	if jsonMode {
		data, _ := json.MarshalIndent(checks, "", "  ")
		fmt.Fprintln(w, string(data))
		return nil
	}

	fmt.Fprintln(w, "🏥 CI/CD 环境诊断")
	fmt.Fprintln(w, strings.Repeat("─", 50))
	for _, c := range checks {
		icon := "✅"
		if c.Status == "warn" {
			icon = "⚠️"
		} else if c.Status == "error" {
			icon = "❌"
		}
		fmt.Fprintf(w, "  %s %-20s %s\n", icon, c.Name, c.Message)
	}
	fmt.Fprintln(w, strings.Repeat("─", 50))
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
		fmt.Fprintln(w, "❌ 存在严重问题，请修复后重试")
	} else if hasWarn {
		fmt.Fprintln(w, "⚠️ 部分环境未完整安装，但核心功能可用")
	} else {
		fmt.Fprintln(w, "✅ 环境正常")
	}
	return nil
}
