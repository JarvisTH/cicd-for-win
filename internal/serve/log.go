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
)

const (
	LogsDir        = "logs"
	maxLogAge      = 30 * 24 * time.Hour // 保留 30 天内的日志
	logCleanupProb = 20                   // 每次写入时概率执行清理（1/logCleanupProb）
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"`
}

func logFilePath(ciDir string) string {
	return filepath.Join(ciDir, LogsDir, time.Now().Format("2006-01-02")+".log")
}

func appendLog(ciDir, level, message, source string) {
	dir := filepath.Join(ciDir, LogsDir)
	os.MkdirAll(dir, config.DirPermDefault)
	filePath := logFilePath(ciDir)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, config.FilePermDefault)
	if err != nil {
		return
	}
	defer f.Close()

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Message:   message,
		Source:    source,
	}
	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	f.Write(data)

	// 概率性触发旧日志清理（避免每次写入都扫描）
	if time.Now().UnixNano()%logCleanupProb == 0 {
		cleanupOldLogs(dir)
	}
}

// cleanupOldLogs 清理超过 maxLogAge 的日志文件
func cleanupOldLogs(logDir string) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxLogAge {
			os.Remove(filepath.Join(logDir, e.Name()))
		}
	}
}

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
	var entry LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
		return
	}
	entry.Timestamp = time.Now().Format(time.RFC3339)
	appendLog(ciDir, entry.Level, entry.Message, entry.Source)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"logs": []any{}})
		return
	}
	date := r.URL.Query().Get("date")
	level := r.URL.Query().Get("level")
	keyword := r.URL.Query().Get("keyword")
	limitStr := r.URL.Query().Get("limit")

	limit := 200
	if limitStr != "" {
		if v, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || v != 1 {
			limit = 200
		}
	}

	logDir := filepath.Join(ciDir, LogsDir)
	filePath := filepath.Join(logDir, date+".log")
	if date == "" {
		filePath = logFilePath(ciDir)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"logs": []any{}})
		return
	}

	lines := strings.Split(string(data), "\n")
	var results []LogEntry
	for i := len(lines) - 1; i >= 0; i-- {
		if len(results) >= limit {
			break
		}
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry LogEntry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if level != "" && entry.Level != level {
			continue
		}
		if keyword != "" && !strings.Contains(entry.Message, keyword) {
			continue
		}
		results = append(results, entry)
	}
	if results == nil {
		results = []LogEntry{}
	}
	json.NewEncoder(w).Encode(map[string]any{"logs": results})
}

func handleLogDates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"dates": []any{}})
		return
	}
	logDir := filepath.Join(ciDir, LogsDir)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"dates": []any{}})
		return
	}
	var dates []string
	for _, e := range entries {
		if !e.IsDir() {
			name := e.Name()
			dates = append(dates, strings.TrimSuffix(name, ".log"))
		}
	}
	sort.Strings(dates)
	if dates == nil {
		dates = []string{}
	}
	json.NewEncoder(w).Encode(map[string]any{"dates": dates})
}

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
	filePath := filepath.Join(ciDir, LogsDir, body.Date+".log")
	if err := os.Remove(filePath); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "删除失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "日志已删除"})
}
