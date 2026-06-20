package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// LocalFileInfo 本地文件/目录信息
type LocalFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// handleLocalLs 列出本地目录内容，用于在 Web UI 中浏览选择项目路径。
//   GET /api/local/ls?path=
//   - path 为空：Windows 返回盘符列表；其它平台返回 "/" 根目录内容
//   - path 给定：返回该目录下的条目（目录在前，名称升序）
func handleLocalLs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dirPath := r.URL.Query().Get("path")

	// 无指定路径：返回盘符列表（Windows）或根目录内容
	if dirPath == "" {
		if runtime.GOOS == "windows" {
			drives := listWindowsDrives()
			json.NewEncoder(w).Encode(map[string]any{
				"path":   "",
				"drives": drives,
				"files":  []any{},
			})
			return
		}
		dirPath = "/"
	}

	// 规范化路径：补充盘符根的斜杠（如 "D:" -> "D:\\"），避免读成当前工作目录
	cleanPath := filepath.Clean(dirPath)
	if runtime.GOOS == "windows" && len(cleanPath) == 2 && cleanPath[1] == ':' {
		cleanPath += string(filepath.Separator)
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": "路径不存在或无法访问: " + err.Error()})
		return
	}
	if !info.IsDir() {
		json.NewEncoder(w).Encode(map[string]any{"error": "指定路径不是目录"})
		return
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		// 权限不足等情况：返回空列表而非报错，便于用户向上返回
		json.NewEncoder(w).Encode(map[string]any{
			"path":   cleanPath,
			"parent": localParent(cleanPath),
			"files":  []any{},
		})
		return
	}

	var dirs, files []LocalFileInfo
	for _, entry := range entries {
		// 跳过隐藏文件/目录，减少噪音
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fi, err := entry.Info()
		modTime := ""
		var size int64
		if err == nil {
			modTime = fi.ModTime().Format("2006-01-02 15:04:05")
			size = fi.Size()
		}
		item := LocalFileInfo{
			Name:    entry.Name(),
			Size:    size,
			IsDir:   entry.IsDir(),
			ModTime: modTime,
		}
		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}
	// 目录在前，各自按名称升序
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })

	all := append(dirs, files...)
	json.NewEncoder(w).Encode(map[string]any{
		"path":   cleanPath,
		"parent": localParent(cleanPath),
		"drives": []string{},
		"files":  all,
	})
}

// localParent 返回上一级目录路径，用于"返回上级"导航
func localParent(p string) string {
	clean := filepath.Clean(p)
	parent := filepath.Dir(clean)
	// 已经在盘符根目录（如 "D:\"），返回空表示无上级
	if runtime.GOOS == "windows" {
		if len(clean) == 3 && clean[1] == ':' && clean[2] == '\\' {
			return ""
		}
		// filepath.Dir("D:\\") == "D:" ，需置空
		if len(parent) == 2 && parent[1] == ':' {
			return ""
		}
	} else if clean == "/" {
		return ""
	}
	if parent == clean {
		return ""
	}
	return parent
}

// listWindowsDrives 枚举 A-Z 可用盘符
func listWindowsDrives() []string {
	var drives []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			drives = append(drives, root)
		}
	}
	return drives
}
