// handler_local.go — 本地操作（打开目录等）
package serve

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
)

// handleOpenDir 在本地文件管理器中打开指定目录。
// GET /api/local/open-dir?path=xxx
func handleOpenDir(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		respondJSON(w, 200, map[string]string{"error": "缺少 path 参数"})
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dirPath)
	case "darwin":
		cmd = exec.Command("open", dirPath)
	default:
		cmd = exec.Command("xdg-open", dirPath)
	}

	if err := cmd.Start(); err != nil {
		respondJSON(w, 200, map[string]string{"error": fmt.Sprintf("打开目录失败: %v", err)})
		return
	}

	respondJSON(w, 200, map[string]string{"status": "ok", "message": "已打开目录"})
}
