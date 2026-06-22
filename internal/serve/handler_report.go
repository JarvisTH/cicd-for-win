package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

// saveTestReportToDisk 将测试报告保存到 reports/{project}/{timestamp}.json
func saveTestReportToDisk(ciDir string, result runner.Result) {
	reportsDir := filepath.Join(ciDir, "reports", result.Project)
	os.MkdirAll(reportsDir, config.DirPermDefault)

	now := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("test-%s.json", now)
	path := filepath.Join(reportsDir, filename)

	data, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(path, data, config.FilePermDefault)

	// 清理旧报告，只保留最近 MaxReportsKeep 条
	pattern := filepath.Join(reportsDir, "test-*.json")
	files, _ := filepath.Glob(pattern)
	if len(files) > config.MaxReportsKeep {
		for i := 0; i < len(files)-config.MaxReportsKeep; i++ {
			os.Remove(files[i])
		}
	}
}

// latestReportHandler 返回指定项目的最新测试报告。
// 支持可选参数 id，指定后返回对应 ID 的历史报告而非最新。
func latestReportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		respondJSON(w, 200, map[string]string{"error": "缺少 project 参数"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200, map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)

	// 如果指定了 id，直接加载该报告文件
	if id := r.URL.Query().Get("id"); id != "" {
		reportPath := filepath.Join(reportsDir, id+".json")
		data, err := os.ReadFile(reportPath)
		if err != nil {
			respondJSON(w, 200, map[string]string{"error": "报告不存在"})
			return
		}
		var report runner.Result
		if err := json.Unmarshal(data, &report); err != nil {
			respondJSON(w, 200, map[string]string{"error": "解析报告失败"})
			return
		}
		respondJSON(w, 200, map[string]any{"report": report})
		return
	}

	// 无 id 时返回最新报告
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		respondJSON(w, 200, map[string]any{"report": nil})
		return
	}

	latest := files[len(files)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		respondJSON(w, 200, map[string]string{"error": "读取报告失败"})
		return
	}

	var report runner.Result
	if err := json.Unmarshal(data, &report); err != nil {
		respondJSON(w, 200, map[string]string{"error": "解析报告失败"})
		return
	}
	respondJSON(w, 200, map[string]any{"report": report})
}

// reportListHandler 返回指定项目的报告列表
func reportListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		respondJSON(w, 200,map[string]any{"reports": []any{}})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]any{"reports": []any{}})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		respondJSON(w, 200,map[string]any{"reports": []any{}})
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
	respondJSON(w, 200,map[string]any{"reports": items})
}

// reportDeleteHandler 删除指定项目的某条测试报告
func reportDeleteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		respondJSON(w, 200,map[string]string{"error": "Method not allowed"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}
	var body struct {
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" || body.ID == "" {
		respondJSON(w, 200,map[string]string{"error": "缺少 project 或 id 参数"})
		return
	}
	reportPath := filepath.Join(ciDir, "reports", body.Project, body.ID+".json")
	if err := os.Remove(reportPath); err != nil {
		respondJSON(w, 200,map[string]string{"error": "删除失败: " + err.Error()})
		return
	}
	respondJSON(w, 200,map[string]string{"status": "ok", "message": "报告已删除"})
}

// GET /api/report/all — 返回所有项目的测试报告列表（合并）
func handleAllReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]any{"reports": []any{}})
		return
	}

	reportsRoot := filepath.Join(ciDir, "reports")
	projectDirs, err := os.ReadDir(reportsRoot)
	if err != nil {
		respondJSON(w, 200,map[string]any{"reports": []any{}})
		return
	}

	type reportItem struct {
		Project   string `json:"project"`
		ID        string `json:"id"`
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
					Total    int     `json:"total"`
					Passed   int     `json:"passed"`
					Failed   int     `json:"failed"`
					Coverage string  `json:"coverage"`
				} `json:"report"`
			}
			if err := json.Unmarshal(data, &res); err != nil {
				continue
			}

			item := reportItem{
				Project:   pDir.Name(),
				ID:        id,
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
		return allReports[i].ID > allReports[j].ID
	})

	if allReports == nil {
		allReports = []reportItem{}
	}
	respondJSON(w, 200,map[string]any{"reports": allReports})
}
