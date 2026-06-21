package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
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

// handleProjectDetect 检测指定路径的项目类型和 Git 远程仓库。
//
//	GET /api/project/detect?path=xxx
//
// 返回 {type, isGit, remotes:[{name,url}]}，供前端在用户填写路径后自动展示可用的检查规则和导入远程仓库。
func handleProjectDetect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Query().Get("path")
	if path == "" {
		json.NewEncoder(w).Encode(map[string]any{"error": "缺少 path 参数"})
		return
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		json.NewEncoder(w).Encode(map[string]any{"error": "路径不存在或不是目录"})
		return
	}
	remotes, isGit := detectGitRemotes(path)
	branches, currentBranch := detectGitBranches(path)
	json.NewEncoder(w).Encode(map[string]any{
		"type":          detectProjectType(path),
		"isGit":         isGit,
		"remotes":       remotes,
		"branches":      branches,
		"currentBranch": currentBranch,
	})
}

// detectProjectType 检测项目类型，逻辑与 ci-runner.ps1 的 Get-ProjectType 保持一致。
func detectProjectType(path string) string {
	// package.json → 前端项目
	pkgFile := filepath.Join(path, "package.json")
	if data, err := os.ReadFile(pkgFile); err == nil {
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		_ = json.Unmarshal(data, &pkg)
		hasDep := func(name string) bool {
			_, ok1 := pkg.Dependencies[name]
			_, ok2 := pkg.DevDependencies[name]
			return ok1 || ok2
		}
		if hasDep("react") {
			return "React"
		}
		if hasDep("vue") || hasDep("vue-router") {
			return "Vue"
		}
		if hasDep("@angular/core") {
			return "Angular"
		}
		if hasDep("next") {
			return "Next"
		}
		return "Node"
	}
	// pom.xml → Maven
	pomFile := filepath.Join(path, "pom.xml")
	if data, err := os.ReadFile(pomFile); err == nil {
		content := string(data)
		if strings.Contains(content, "<modules>") || strings.Contains(content, "<packaging>pom</packaging>") {
			return "MavenMulti"
		}
		return "Maven"
	}
	if fileExists(filepath.Join(path, "build.gradle")) {
		return "Gradle"
	}
	if fileExists(filepath.Join(path, "Cargo.toml")) {
		return "Rust"
	}
	if fileExists(filepath.Join(path, "go.mod")) {
		return "Go"
	}
	return "Unknown"
}

// detectGitRemotes 执行 git remote -v 并解析为 [{name, url}] 列表（按 name 去重，取 fetch URL）。
// 第二个返回值表示该路径是否为 Git 仓库。
func detectGitRemotes(path string) ([]map[string]string, bool) {
	cmd := exec.Command("git.exe", "-C", path, "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		return nil, false // 非 Git 仓库或 git 不可用
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	seen := map[string]bool{}
	var remotes []map[string]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		url := fields[1]
		// git remote -v 每个远程输出两行（fetch + push），只取 fetch 行并按 name 去重
		if len(fields) >= 3 && fields[2] == "(fetch)" {
			if seen[name] {
				continue
			}
			seen[name] = true
			remotes = append(remotes, map[string]string{"name": name, "url": url})
		}
	}
	return remotes, true
}

// detectGitBranches 执行 git branch 列出所有本地分支及当前分支。
func detectGitBranches(path string) ([]string, string) {
	cmd := exec.Command("git.exe", "-C", path, "branch", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, ""
	}
	var branches []string
	currentBranch := ""
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 当前分支前有 * 标记
		if strings.HasPrefix(line, "* ") {
			currentBranch = strings.TrimPrefix(line, "* ")
			branches = append(branches, currentBranch)
		} else {
			branches = append(branches, line)
		}
	}
	return branches, currentBranch
}
