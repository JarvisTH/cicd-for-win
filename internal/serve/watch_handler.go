// watch_handler.go — 文件监听 Web API
package serve

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

// activeWatchers 管理正在运行的文件监听器
var (
	activeWatchers   = map[string]func(){} // projectName -> cancel
	activeWatchersMu sync.Mutex
)

// handleWatchStart 开始监听项目文件变更。
// GET /api/watch/start?project=xxx
func handleWatchStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")
	if projectName == "" {
		respondJSON(w, 200, map[string]string{"error": "缺少 project 参数"})
		return
	}

	activeWatchersMu.Lock()
	defer activeWatchersMu.Unlock()

	// 已存在监听，跳过
	if _, ok := activeWatchers[projectName]; ok {
		respondJSON(w, 200, map[string]string{"status": "ok", "message": "已经在监听中"})
		return
	}

	// 加载项目配置
	ciDir := findCiDir()
	cfg, err := config.Load(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		respondJSON(w, 200, map[string]string{"error": "读取项目配置失败"})
		return
	}

	var proj *config.Project
	for i, p := range cfg.Projects {
		if p.Name == projectName {
			proj = &cfg.Projects[i]
			break
		}
	}
	if proj == nil {
		respondJSON(w, 200, map[string]string{"error": "未找到项目"})
		return
	}

	// 解析规则状态
	ruleStates := make(map[string]bool)
	for _, r := range proj.Rules {
		ruleStates[r.ID] = r.Enabled
	}

	// 启动监听（goroutine）
	ctx, cancel := context.WithCancel(context.Background())
	go runner.WatchProject(proj.Path, runner.DetectProjectType(proj.Path), ruleStates, ciDir, ctx)

	activeWatchers[projectName] = cancel
	respondJSON(w, 200, map[string]string{"status": "ok", "message": "已开始监听 " + projectName})
}

// handleWatchStop 停止所有或指定项目的文件监听。
// GET /api/watch/stop?project=xxx
func handleWatchStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")

	activeWatchersMu.Lock()
	defer activeWatchersMu.Unlock()

	if projectName != "" {
		if cancel, ok := activeWatchers[projectName]; ok {
			cancel()
			delete(activeWatchers, projectName)
		}
		respondJSON(w, 200, map[string]string{"status": "ok", "message": "已停止监听 " + projectName})
		return
	}

	// 停止全部
	for name, cancel := range activeWatchers {
		cancel()
		delete(activeWatchers, name)
	}
	respondJSON(w, 200, map[string]string{"status": "ok", "message": "已停止全部监听"})
}

// handleWatchStatus 返回当前监听状态。
// GET /api/watch/status
func handleWatchStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	activeWatchersMu.Lock()
	defer activeWatchersMu.Unlock()

	projects := make([]string, 0, len(activeWatchers))
	for name := range activeWatchers {
		projects = append(projects, name)
	}
	respondJSON(w, 200, map[string]any{"watching": projects})
}
