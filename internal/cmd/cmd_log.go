package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

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
		JsonOutput, _ := cmd.Flags().GetBool("json")

		if limit <= 0 {
			limit = 100
		}

		results := queryAuditLogs(date, level, keyword, limit)
		if JsonOutput {
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
		icon := map[string]string{"error": "❌", "warn": "⚠️", "info": "ℹ️"}
		for _, r := range results {
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
		JsonOutput, _ := cmd.Flags().GetBool("json")

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

		if JsonOutput {
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
