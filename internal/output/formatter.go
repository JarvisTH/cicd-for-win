package output

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"ci-cd/internal/runner"
)

func Format(cmd *cobra.Command, results []runner.Result, jsonOutput bool) error {
	if jsonOutput {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, r := range results {
			status := "✅"
			if r.Status != "pass" {
				status = "❌"
			}
			fmt.Printf("[%s] %s (%s)\n", r.Project, status, r.Duration)
			for _, s := range r.Steps {
				stepStatus := "✅"
				if s.Status != "pass" {
					stepStatus = "❌"
				}
				fmt.Printf("  %s %s (%s)\n", stepStatus, s.Name, s.Duration)
			}
			// 输出测试报告详情
			if r.Report != nil {
				report := r.Report
				fmt.Printf("  📊 测试报告: %d 总数, %d 通过, %d 失败, %d 跳过",
					report.Total, report.Passed, report.Failed, report.Skipped)
				if report.Coverage != "" {
					fmt.Printf(", 覆盖率: %s", report.Coverage)
				}
				fmt.Println()
				for _, f := range report.Failures {
					fmt.Printf("    ❌ [%s] %s: %s\n", f.Suite, f.Test, f.Message)
				}
			}
		}
	}
	return nil
}
