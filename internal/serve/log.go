package serve

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ci-cd/internal/security"
)

const LogsDir = "logs"

// LogEntry 单条审计日志
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Source  string `json:"source"`
}

// logFilePath 返回当天的日志文件路径 logs/audit-YYYY-MM-DD.jsonl
func logFilePath(ciDir string) string {
	date := time.Now().Format("2006-01-02")
	return filepath.Join(ciDir, LogsDir, fmt.Sprintf("audit-%s.jsonl", date))
}

// appendLog 追加一条日志到磁盘
func appendLog(ciDir, level, message, source string) {
	entry := LogEntry{
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Level:   level,
		Message: message,
		Source:  source,
	}
	data, _ := json.Marshal(entry)

	dir := filepath.Join(ciDir, LogsDir)
	os.MkdirAll(dir, 0755)

	f, err := os.OpenFile(logFilePath(ciDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("⚠️ 写审计日志失败: %v\n", err)
		return
	}
	defer f.Close()
	f.Write(data)
	f.WriteString("\n")
}

// POST /api/log/append — 前端 log() 调用此接口写审计日志
func handleLogAppend(w http.ResponseWriter, r *http.Request) {
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
		Message string `json:"message"`
		Level   string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
		return
	}
	if body.Message == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "消息不能为空"})
		return
	}
	if body.Level == "" {
		body.Level = "info"
	}
	appendLog(ciDir, body.Level, body.Message, "web")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /api/log/query?date=2026-06-19&level=error&keyword=xxx&limit=200 — 查询审计日志
func handleLogQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"logs": []any{}})
		return
	}

	date := r.URL.Query().Get("date")
	filterLevel := r.URL.Query().Get("level")
	keyword := r.URL.Query().Get("keyword")
	limitStr := r.URL.Query().Get("limit")

	limit := 200
	if limitStr != "" {
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || n != 1 || limit <= 0 {
			limit = 200
		}
	}

	// 如果不传 date，默认今天
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	fpath := filepath.Join(ciDir, LogsDir, fmt.Sprintf("audit-%s.jsonl", date))
	data, err := os.ReadFile(fpath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"logs": []any{}, "date": date})
		return
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entries []LogEntry
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		// 过滤级别
		if filterLevel != "" && e.Level != filterLevel {
			continue
		}
		// 过滤关键字
		if keyword != "" && !strings.Contains(e.Message, keyword) {
			continue
		}
		entries = append(entries, e)
	}

	// 倒序（最新的在前）
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Time > entries[j].Time
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	json.NewEncoder(w).Encode(map[string]any{
		"logs": entries,
		"date": date,
	})
}

// GET /api/log/dates — 列出所有有日志的日期
func handleLogDates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"dates": []any{}})
		return
	}

	dir := filepath.Join(ciDir, LogsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"dates": []any{}})
		return
	}

	var dates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".jsonl") {
			date := strings.TrimPrefix(name, "audit-")
			date = strings.TrimSuffix(date, ".jsonl")
			dates = append(dates, date)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	if dates == nil {
		dates = []string{}
	}
	json.NewEncoder(w).Encode(map[string]any{"dates": dates})
}

// POST /api/log/delete — 删除指定日期的审计日志文件
// body: {"date": "2026-06-19"}
func handleLogDelete(w http.ResponseWriter, r *http.Request) {
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
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Date == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 date 参数"})
		return
	}
	fpath := filepath.Join(ciDir, LogsDir, fmt.Sprintf("audit-%s.jsonl", body.Date))
	if err := os.Remove(fpath); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "删除失败: " + err.Error()})
		return
	}
	log.Printf("🗑️ 删除审计日志: %s\n", body.Date)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": fmt.Sprintf("已删除 %s 的日志", body.Date)})
}

// ========== 统一报告查看 ==========

// GET /api/rules?file=rules/eslint-vue.mjs — 读取规则文件内容
func handleViewRuleFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		// 兼容路径形式 /api/rules/eslint-vue.mjs
		fileName = strings.TrimPrefix(r.URL.Path, "/api/rules/")
	}
	if fileName == "" {
		http.Error(w, "缺少 file 参数", http.StatusBadRequest)
		return
	}

	// 安全检查：只允许读取 rules/ 目录下的文件，禁止路径穿越
	if strings.Contains(fileName, "..") || strings.ContainsAny(fileName, `\/`) {
		http.Error(w, "非法路径", http.StatusBadRequest)
		return
	}
	// 标准化路径
	fileName = strings.TrimPrefix(fileName, "rules/")

	ciDir := findCiDir()
	if ciDir == "" {
		http.Error(w, "找不到 ci-cd 目录", http.StatusInternalServerError)
		return
	}

	rulesDir := filepath.Join(ciDir, "rules")
	filePath := filepath.Join(rulesDir, fileName)
	// 二次校验：解析后路径必须在 rules 目录内（防 filepath.Join 清理后的穿越）
	if !security.IsPathSafe(rulesDir, filepath.Dir(filePath)) {
		http.Error(w, "非法路径: 禁止访问 rules 目录外文件", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "读取规则文件失败: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Write(data)
}

// GET /api/report/all — 返回所有项目的测试报告列表（合并）
func handleAllReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}

	reportsRoot := filepath.Join(ciDir, "reports")
	projectDirs, err := os.ReadDir(reportsRoot)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
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
				ts = id[5:] // 去掉 "test-"
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

	// 按时间倒序
	sort.Slice(allReports, func(i, j int) bool {
		return allReports[i].ID > allReports[j].ID
	})

	if allReports == nil {
		allReports = []reportItem{}
	}
	json.NewEncoder(w).Encode(map[string]any{"reports": allReports})
}
