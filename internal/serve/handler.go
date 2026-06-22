package serve

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
)

//go:embed web/*
var webFiles embed.FS

// activeAuth 内存中缓存的当前认证信息，修改密码时立即更新。
// authMu 保护并发读写，避免数据竞争导致认证失效或 panic。
var (
	activeAuth *config.AuthConfig
	authMu     sync.RWMutex
)

// getActiveAuth 线程安全地读取当前认证配置。
func getActiveAuth() *config.AuthConfig {
	authMu.RLock()
	defer authMu.RUnlock()
	return activeAuth
}

// setActiveAuth 线程安全地更新当前认证配置。
func setActiveAuth(a *config.AuthConfig) {
	authMu.Lock()
	activeAuth = a
	authMu.Unlock()
}

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
	setActiveAuth(auth)
	return auth, nil
}

// basicAuth 返回一个 HTTP 中间件，对请求进行 Basic Auth 校验。
// 带有效 download token 的请求可绕过 Basic Auth（用于浏览器原生下载场景）。
func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 一次性下载 token：浏览器原生下载（iframe/a 标签）无法携带 Authorization 头，
		// 用 token 放行，token 一次性消费、60 秒过期。
		if tk := r.URL.Query().Get("download_token"); tk != "" && validateDownloadToken(tk) {
			next.ServeHTTP(w, r)
			return
		}

		auth := getActiveAuth()
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

// csrfProtect 对状态变更方法（POST/PUT/DELETE）要求自定义请求头，
// 浏览器跨站表单无法携带自定义头，从而阻止 CSRF。Basic Auth 下浏览器会自动
// 发送凭据，仅靠 Basic Auth 不足以防范 CSRF。
func csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Requested-With") == "" {
				http.Error(w, `{"error":"缺少 X-Requested-With 头，疑似 CSRF 攻击"}`, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func NewHandler(ciDir string) http.Handler {
	// 初始化认证
	if _, err := initAuth(ciDir); err != nil {
		log.Fatalf("初始化认证失败: %v", err)
	}

	// 启动 SSH 空闲连接回收，防止长期运行泄漏连接
	startSSHReaper()

	// 启动时迁移 projects.json 中的明文部署密码为加密存储
	migrateProjectDeployPasswords(ciDir)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", apiHandler("check"))
	mux.HandleFunc("/api/build", apiHandler("build"))
	mux.HandleFunc("/api/test", apiHandler("test"))
	mux.HandleFunc("/api/push", pushHandler)
	mux.HandleFunc("/api/status", apiHandler("status"))
	mux.HandleFunc("/api/projects", projectListHandler)
	mux.HandleFunc("/api/project", projectSaveHandler)
	mux.HandleFunc("/api/deploy", deployHandler)
	mux.HandleFunc("/api/deploy/test", deployTestHandler)
	mux.HandleFunc("/api/steps/status", stepStatusHandler)
	mux.HandleFunc("/api/steps/status/clear", stepStatusClearHandler)
	mux.HandleFunc("/api/auth/status", authStatusHandler)
	mux.HandleFunc("/api/auth/change-password", changePasswordHandler)
	mux.HandleFunc("/api/report/latest", latestReportHandler)
	mux.HandleFunc("/api/report/list", reportListHandler)
	mux.HandleFunc("/api/report/delete", reportDeleteHandler)

	// 审计日志
	mux.HandleFunc("/api/log/append", handleLogAppend)
	mux.HandleFunc("/api/log/query", handleLogQuery)
	mux.HandleFunc("/api/log/dates", handleLogDates)
	mux.HandleFunc("/api/log/delete", handleLogDelete)

	// 统一报告列表
	mux.HandleFunc("/api/report/all", handleAllReports)

	// 规则文件查看
	mux.HandleFunc("/api/rules/", handleViewRuleFile)

	// 本地目录浏览（用于选择项目路径）
	mux.HandleFunc("/api/project/detect", handleProjectDetect)
	mux.HandleFunc("/api/local/ls", handleLocalLs)

	// 远程管理 API
	mux.HandleFunc("/api/remote/projects", handleRemoteDeployTargets)
	mux.HandleFunc("/api/remote/servers", handleRemoteServers)
	mux.HandleFunc("/api/remote/server", handleRemoteServerDelete)
	mux.HandleFunc("/api/remote/term", handleRemoteTerm)
	mux.HandleFunc("/api/remote/ls", handleRemoteLs)
	mux.HandleFunc("/api/remote/download", handleRemoteDownload)
	mux.HandleFunc("/api/remote/download-token", handleDownloadToken)
	mux.HandleFunc("/api/remote/upload", handleRemoteUpload)
	mux.HandleFunc("/api/remote/delete", handleRemoteDelete)
	mux.HandleFunc("/api/remote/mkdir", handleRemoteMkdir)
	mux.HandleFunc("/api/remote/disconnect", handleRemoteDisconnect)

	subFS, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	return basicAuth(csrfProtect(mux))
}

var validStepIDs = map[string]bool{"check": true, "build": true, "test": true, "push": true, "deploy": true}

func isValidStepID(id string) bool { return validStepIDs[id] }

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

// buildCommandString 将命令名和参数拼接为人类可读的字符串（供前端日志展示）
func buildCommandString(name string, args []string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteString(" ")
		if strings.ContainsAny(a, " &\"'") {
			b.WriteString("\"")
			b.WriteString(strings.ReplaceAll(a, "\"", "\\\""))
			b.WriteString("\"")
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// respondJSON 统一 HTTP JSON 响应，自动设置 Content-Type。
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError 统一错误响应。
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// GET /api/rules?file=rules/eslint-vue.mjs — 读取规则文件内容
func handleViewRuleFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		fileName = strings.TrimPrefix(r.URL.Path, "/api/rules/")
	}
	if fileName == "" {
		http.Error(w, "缺少 file 参数", http.StatusBadRequest)
		return
	}

	if strings.Contains(fileName, "..") || strings.ContainsAny(fileName, `\/`) {
		http.Error(w, "非法路径", http.StatusBadRequest)
		return
	}
	fileName = strings.TrimPrefix(fileName, "rules/")

	ciDir := findCiDir()
	if ciDir == "" {
		http.Error(w, "找不到 ci-cd 目录", http.StatusInternalServerError)
		return
	}

	rulesDir := filepath.Join(ciDir, "rules")
	filePath := filepath.Join(rulesDir, fileName)
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
