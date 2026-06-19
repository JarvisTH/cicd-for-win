package serve

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

//go:embed web/*
var webFiles embed.FS

// activeAuth 内存中缓存的当前认证信息，修改密码时立即更新
var activeAuth *config.AuthConfig

// initAuth 初始化认证：从 auth.json 读取，文件不存在则创建默认
func initAuth(ciDir string) (*config.AuthConfig, error) {
	auth, err := config.LoadAuth(ciDir)
	if err != nil {
		return nil, err
	}
	isDefault := auth.Username == config.DefaultUsername && auth.VerifyPassword(config.DefaultPassword)
	if isDefault {
		log.Printf("⚠️  警告: 使用默认密码 (%s/%s)，请通过 Web UI 或 `ci passwd` 修改\n", config.DefaultUsername, config.DefaultPassword)
	}
	activeAuth = auth
	return auth, nil
}

// basicAuth 返回一个 HTTP 中间件，对请求进行 Basic Auth 校验
func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := activeAuth
		if auth == nil {
			http.Error(w, "认证未初始化", http.StatusInternalServerError)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(auth.Username)) != 1 ||
			!auth.VerifyPassword(pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="CI/CD 控制台"`)
			http.Error(w, "未授权，请输入正确的用户名和密码", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func NewHandler(ciDir string) http.Handler {
	// 初始化认证
	if _, err := initAuth(ciDir); err != nil {
		log.Fatalf("初始化认证失败: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", apiHandler("check"))
	mux.HandleFunc("/api/build", apiHandler("build"))
	mux.HandleFunc("/api/test", apiHandler("test"))
	mux.HandleFunc("/api/push", pushHandler)
	mux.HandleFunc("/api/status", apiHandler("status"))
	mux.HandleFunc("/api/projects", projectListHandler)
	mux.HandleFunc("/api/project", projectSaveHandler)
	mux.HandleFunc("/api/deploy/test", deployTestHandler)
	mux.HandleFunc("/api/auth/status", authStatusHandler)
	mux.HandleFunc("/api/auth/change-password", changePasswordHandler)
	mux.HandleFunc("/api/report/latest", latestReportHandler)
	mux.HandleFunc("/api/report/list", reportListHandler)
	mux.HandleFunc("/api/report/delete", reportDeleteHandler)

	subFS, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	return basicAuth(mux)
}

// authStatusHandler 返回当前认证状态（不暴露密码）
func authStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	auth := activeAuth
	if auth == nil {
		json.NewEncoder(w).Encode(map[string]any{"username": "", "is_default": false})
		return
	}
	isDefault := auth.Username == config.DefaultUsername && auth.VerifyPassword(config.DefaultPassword)
	json.NewEncoder(w).Encode(map[string]any{
		"username":   auth.Username,
		"is_default": isDefault,
	})
}

// changePasswordHandler 处理密码修改请求
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
		return
	}

	if body.OldPassword == "" || body.NewPassword == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "旧密码和新密码不能为空"})
		return
	}

	if len(body.NewPassword) < 6 {
		json.NewEncoder(w).Encode(map[string]string{"error": "新密码长度不能少于 6 位"})
		return
	}

	auth := activeAuth
	if auth == nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "认证未初始化"})
		return
	}

	// 验证旧密码
	if !auth.VerifyPassword(body.OldPassword) {
		json.NewEncoder(w).Encode(map[string]string{"error": "旧密码错误"})
		return
	}

	// 生成新配置并保存
	newAuth := config.NewAuthConfig(auth.Username, body.NewPassword)
	if err := config.SaveAuth(ciDir, newAuth); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存密码失败: " + err.Error()})
		return
	}

	// 更新内存缓存
	activeAuth = newAuth

	log.Printf("🔑 密码已修改 (用户: %s)\n", auth.Username)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "密码修改成功"})
}

func apiHandler(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		project := r.URL.Query().Get("project")
		ciDir := findCiDir()
		if ciDir == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-runner.ps1"})
			return
		}
		cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass",
			"-File", filepath.Join(ciDir, "ci-runner.ps1"),
			"-Action", action,
			"-ProjectPath", project,
			"-Json")
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Write(output)

		// 测试完成后，将报告持久化到磁盘
		if action == "test" {
			var result runner.Result
			if err := json.Unmarshal(output, &result); err == nil && result.Report != nil {
				saveTestReportToDisk(ciDir, result)
			}
		}
	}
}

// saveTestReportToDisk 将测试报告保存到 reports/{project}/{timestamp}.json
func saveTestReportToDisk(ciDir string, result runner.Result) {
	reportsDir := filepath.Join(ciDir, "reports", result.Project)
	os.MkdirAll(reportsDir, 0755)

	now := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("test-%s.json", now)
	path := filepath.Join(reportsDir, filename)

	data, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile(path, data, 0644)

	// 清理旧报告，只保留最近 20 条
	pattern := filepath.Join(reportsDir, "test-*.json")
	files, _ := filepath.Glob(pattern)
	if len(files) > 20 {
		for i := 0; i < len(files)-20; i++ {
			os.Remove(files[i])
		}
	}
}

func projectListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}

	// 解析并注入版本信息
	var cfg struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		w.Write(data)
		return
	}
	for i, p := range cfg.Projects {
		path, _ := p["path"].(string)
		if path == "" {
			continue
		}
		// 版本号
		cfg.Projects[i]["version"] = readProjectVersion(path)
		// Git 信息
		branch, commit := readGitInfo(path)
		cfg.Projects[i]["git_branch"] = branch
		cfg.Projects[i]["git_commit"] = commit
	}
	result, _ := json.MarshalIndent(cfg, "", "  ")
	w.Write(result)
}

func readProjectVersion(path string) string {
	// 尝试读取 package.json
	pkgFile := filepath.Join(path, "package.json")
	if data, err := os.ReadFile(pkgFile); err == nil {
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
	}
	// 尝试读取 pom.xml
	pomFile := filepath.Join(path, "pom.xml")
	if data, err := os.ReadFile(pomFile); err == nil {
		content := string(data)
		if idx := strings.Index(content, "<version>"); idx >= 0 {
			end := strings.Index(content[idx:], "</version>")
			if end >= 0 {
				return content[idx+9 : idx+end]
			}
		}
	}
	return ""
}

func readGitInfo(path string) (branch, commit string) {
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	} else {
		branch = ""
	}
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	} else {
		commit = ""
	}
	return
}

func projectSaveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}
	var data any
	json.NewDecoder(r.Body).Decode(&data)
	raw, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(filepath.Join(ciDir, "projects.json"), raw, 0644)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func findCiDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if fileExists(filepath.Join(dir, "ci-runner.ps1")) {
		return dir
	}
	parent := filepath.Dir(dir)
	if fileExists(filepath.Join(parent, "ci-runner.ps1")) {
		return parent
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pushHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(ciDir, "ci-push.ps1"),
		"-ProjectName", project)
	err := cmd.Run()
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "pass", "message": "推送成功"})
	} else {
		json.NewEncoder(w).Encode(map[string]string{"status": "fail", "message": err.Error()})
	}
}

func deployTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host := r.URL.Query().Get("host")
	port := r.URL.Query().Get("port")
	user := r.URL.Query().Get("user")
	authType := r.URL.Query().Get("auth_type")
	keyFile := r.URL.Query().Get("identity_file")

	if host == "" || user == "" {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "缺少主机或用户名"})
		return
	}

	args := []string{
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-p", port,
	}
	if authType == "key" && keyFile != "" {
		args = append(args, "-i", keyFile)
	}
	args = append(args, user+"@"+host, "echo ok")

	cmd := exec.Command("ssh.exe", args...)
	err := cmd.Run()
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "连接成功"})
	} else {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
	}
}

// latestReportHandler 返回指定项目的最新测试报告
func latestReportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 project 参数"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		json.NewEncoder(w).Encode(map[string]any{"report": nil})
		return
	}

	latest := files[len(files)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "读取报告失败"})
		return
	}

	var report runner.Result
	if err := json.Unmarshal(data, &report); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "解析报告失败"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"report": report})
}

// reportListHandler 返回指定项目的报告列表
func reportListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	if project == "" {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
		return
	}

	reportsDir := filepath.Join(ciDir, "reports", project)
	pattern := filepath.Join(reportsDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"reports": []any{}})
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
	json.NewEncoder(w).Encode(map[string]any{"reports": items})
}

// reportDeleteHandler 删除指定项目的某条测试报告
func reportDeleteHandler(w http.ResponseWriter, r *http.Request) {
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
		Project string `json:"project"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" || body.ID == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 project 或 id 参数"})
		return
	}
	reportPath := filepath.Join(ciDir, "reports", body.Project, body.ID+".json")
	if err := os.Remove(reportPath); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "删除失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "报告已删除"})
}
