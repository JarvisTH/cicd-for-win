package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"ci-cd/internal/runner"
)

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
		return runReport(projectName, listMode, jsonMode, deleteID)
	},
}

// runReport 实现 report 命令的业务逻辑，供 RunE 调用和独立测试。
func runReport(projectName string, listMode, jsonMode bool, deleteID string) error {
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
			id := name[:len(name)-5]
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
}
