package serve

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
	"ci-cd/internal/sshutil"
)

// ========== 一次性下载 token 机制 ==========
// 用于让浏览器原生下载（iframe/a 标签）绕过 Basic Auth，
// 因为这些方式无法可靠携带 Authorization 头。

var (
	downloadTokens   = map[string]time.Time{} // token -> 过期时间
	downloadTokensMu sync.Mutex
)

const downloadTokenTTL = 60 * time.Second // token 有效期 60 秒

// generateDownloadToken 生成一次性下载 token，有效期 60 秒。
func generateDownloadToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	downloadTokensMu.Lock()
	// 顺便清理过期 token，防止内存累积
	now := time.Now()
	for k, exp := range downloadTokens {
		if now.After(exp) {
			delete(downloadTokens, k)
		}
	}
	downloadTokens[token] = time.Now().Add(downloadTokenTTL)
	downloadTokensMu.Unlock()
	return token
}

// validateDownloadToken 校验 token 是否有效，有效则消费（一次性）。
func validateDownloadToken(token string) bool {
	if token == "" {
		return false
	}
	downloadTokensMu.Lock()
	defer downloadTokensMu.Unlock()
	exp, ok := downloadTokens[token]
	if !ok || time.Now().After(exp) {
		delete(downloadTokens, token)
		return false
	}
	delete(downloadTokens, token) // 一次性消费
	return true
}

// cachedClient 包装 SSH 客户端与最后使用时间，用于空闲回收。
type cachedClient struct {
	client   *ssh.Client
	lastUsed time.Time
}

const sshIdleTimeout = 30 * time.Minute // 空闲超过 30 分钟的连接自动回收

var (
	sshClients = map[string]*cachedClient{}
	sshMu      sync.Mutex
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// startSSHReaper 启动后台 goroutine 定期回收空闲 SSH 连接，防止连接泄漏。
func startSSHReaper() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			reapIdleSSHClients()
		}
	}()
}

// reapIdleSSHClients 关闭并移除超过空闲超时的缓存连接。
func reapIdleSSHClients() {
	sshMu.Lock()
	defer sshMu.Unlock()
	now := time.Now()
	for key, c := range sshClients {
		if now.Sub(c.lastUsed) > sshIdleTimeout {
			// 心跳检测，确认是否真空闲
			if _, _, err := c.client.SendRequest("keepalive@golang.org", true, nil); err != nil {
				c.client.Close()
				delete(sshClients, key)
				log.Printf("🔌 回收空闲/失效 SSH 连接: %s\n", key)
			} else {
				c.lastUsed = now // 仍然活跃，刷新时间
			}
		}
	}
}

// CloseAllSSHClients 关闭所有缓存的 SSH 连接，用于进程优雅退出。
func CloseAllSSHClients() {
	sshMu.Lock()
	defer sshMu.Unlock()
	for key, c := range sshClients {
		c.client.Close()
		delete(sshClients, key)
	}
	log.Printf("🔌 已关闭所有 SSH 连接\n")
}

// getSSHClient 获取或创建项目的 SSH 连接（带缓存 + 心跳检测）。
// 使用 double-check 锁模式：持锁期间不做网络 IO，避免阻塞其他请求。
func getSSHClient(projectName string, deploy *config.DeployConfig) (*ssh.Client, error) {
	if deploy == nil || deploy.Host == "" {
		return nil, fmt.Errorf("项目 %s 未配置部署信息", projectName)
	}
	ciDir := findCiDir()

	// 第一次检查：命中缓存且连接活跃则直接返回
	sshMu.Lock()
	if c, ok := sshClients[projectName]; ok {
		if _, _, err := c.client.SendRequest("keepalive@golang.org", true, nil); err == nil {
			c.lastUsed = time.Now()
			sshMu.Unlock()
			return c.client, nil
		}
		// 连接已断开，清理
		c.client.Close()
		delete(sshClients, projectName)
	}
	sshMu.Unlock()

	// 不持锁地建立新连接（网络 IO 可能耗时）
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败 %s: %w", addr, err)
	}

	// 第二次检查：防止并发期间已有其他 goroutine 建立了连接
	sshMu.Lock()
	defer sshMu.Unlock()
	if c, ok := sshClients[projectName]; ok {
		// 已有新连接，关闭刚建立的冗余连接
		client.Close()
		c.lastUsed = time.Now()
		return c.client, nil
	}
	sshClients[projectName] = &cachedClient{client: client, lastUsed: time.Now()}
	return client, nil
}

// ========== API: 获取可远程管理的项目列表 ==========

func handleRemoteProjects(w http.ResponseWriter, r *http.Request) {
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
	var cfg struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}
	// 只返回有部署配置的项目
	var filtered []map[string]any
	for _, p := range cfg.Projects {
		if deploy, ok := p["deploy"].(map[string]any); ok {
			if host, _ := deploy["host"].(string); host != "" {
				// 脱敏，只返回连接所需信息（移除密码）
				name, _ := p["name"].(string)
				pType, _ := p["type"].(string)
				filtered = append(filtered, map[string]any{
					"name":   name,
					"type":   pType,
					"deploy": sanitizeDeploy(deploy),
				})
			}
		}
	}
	if filtered == nil {
		filtered = []map[string]any{}
	}
	json.NewEncoder(w).Encode(map[string]any{"projects": filtered})
}

// ========== 加载项目 ==========

func loadProjectByName(name string) *config.Project {
	ciDir := findCiDir()
	if ciDir == "" {
		return nil
	}
	cfg, err := config.Load(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return nil
	}
	for _, p := range cfg.Projects {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// resolveDeployConfig 根据 name + source 查找部署配置（支持项目部署和独立服务器）
func resolveDeployConfig(name, source string) *config.DeployConfig {
	if source == "standalone" {
		return loadStandaloneServerDeploy(name)
	}
	proj := loadProjectByName(name)
	if proj != nil {
		return proj.Deploy
	}
	return nil
}

// ========== WebSocket SSH 终端 ==========

func handleRemoteTerm(w http.ResponseWriter, r *http.Request) {
	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	if projectName == "" {
		http.Error(w, "缺少 project 参数", http.StatusBadRequest)
		return
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		http.Error(w, "项目不存在或未配置部署", http.StatusBadRequest)
		return
	}

	// 升级到 WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v\n", err)
		return
	}
	defer conn.Close()

	// 建立 SSH 连接
	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		conn.Close()
		return
	}
	defer client.Close()

	// 创建 SSH session
	session, err := client.NewSession()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": "创建 SSH session 失败: " + err.Error()})
		return
	}
	defer session.Close()

	// 请求伪终端 - 完整的交互式终端模式
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // 启用回显
		ssh.ECHOE:         1,     // 退格字符回显为 BS SP BS
		ssh.ECHOK:         1,     // 删除行后回显换行
		ssh.ECHONL:        0,     // 不回显换行符
		ssh.ICANON:        1,     // 规范模式（行缓冲）
		ssh.ISIG:          1,     // 启用信号字符（Ctrl+C 等）
		ssh.ICRNL:         1,     // 输入回车转换行
		ssh.OPOST:         1,     // 启用输出处理
		ssh.ONLCR:         1,     // 输出换行转回车换行
		ssh.TTY_OP_ISPEED: 14400, // 输入速率
		ssh.TTY_OP_OSPEED: 14400, // 输出速率
	}
	if err := session.RequestPty("xterm-256color", 40, 120, modes); err != nil {
		conn.WriteJSON(map[string]string{"error": "请求伪终端失败: " + err.Error()})
		return
	}

	// 获取 SSH session 的 stdin/stdout/stderr 管道
	stdin, err := session.StdinPipe()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}

	// 通知前端连接成功（goroutine 启动前发送）
	conn.WriteJSON(map[string]string{"type": "connected", "message": "SSH 终端已连接"})

	// 双向管道: WebSocket ↔ SSH
	var wg sync.WaitGroup
	wg.Add(2)

	// SSH stdout → WebSocket （先启动 goroutine，再启动 shell，避免输出丢失）
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// SSH stderr → WebSocket
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → SSH stdin
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				session.Close()
				return
			}
			// 处理 resize 消息
			if len(msg) > 6 && string(msg[:6]) == "resize" {
				parts := strings.Split(string(msg[6:]), "x")
				if len(parts) == 2 {
					w, _ := strconv.Atoi(parts[0])
					h, _ := strconv.Atoi(parts[1])
					if w > 0 && h > 0 {
						session.WindowChange(h, w)
					}
				}
				continue
			}
			stdin.Write(msg)
		}
	}()

	// 启动 shell（此时 goroutine 已在后台读 stdout）
	if err := session.Shell(); err != nil {
		conn.WriteJSON(map[string]string{"error": "启动 shell 失败: " + err.Error()})
		return
	}

	// 发送命令触发 shell 输出（延迟确保 goroutine 已开始读取）
	time.Sleep(200 * time.Millisecond)
	stdin.Write([]byte("\n"))

	wg.Wait()
}

// ========== SFTP: 列出目录 ==========

type RemoteFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

func handleRemoteLs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	remotePath := r.URL.Query().Get("path")
	if remotePath == "" {
		remotePath = "/"
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		json.NewEncoder(w).Encode(map[string]any{"error": "项目不存在或未配置部署"})
		return
	}

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": "SFTP 连接失败: " + err.Error()})
		return
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir(remotePath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": "读取目录失败: " + err.Error()})
		return
	}

	files := make([]RemoteFileInfo, 0, len(entries))
	for _, entry := range entries {
		files = append(files, RemoteFileInfo{
			Name:    entry.Name(),
			Size:    entry.Size(),
			Mode:    entry.Mode().Perm().String(),
			IsDir:   entry.IsDir(),
			ModTime: entry.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	// 获取当前路径信息用于面包屑
	json.NewEncoder(w).Encode(map[string]any{
		"path":  remotePath,
		"files": files,
	})
}

// ========== SFTP: 下载文件 ==========

// handleDownloadToken 生成一次性下载 token（需 Basic Auth 认证）。
// 前端用此 token 构造 URL，让浏览器原生下载（iframe/a 标签）绕过 Basic Auth。
func handleDownloadToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	token := generateDownloadToken()
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func handleRemoteDownload(w http.ResponseWriter, r *http.Request) {
	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	remotePath := r.URL.Query().Get("path")

	if projectName == "" || remotePath == "" {
		http.Error(w, "缺少参数", http.StatusBadRequest)
		return
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		http.Error(w, "项目不存在或未配置部署", http.StatusBadRequest)
		return
	}

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		http.Error(w, "SFTP 连接失败", http.StatusInternalServerError)
		return
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		http.Error(w, "打开远程文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer remoteFile.Close()

	// 获取文件信息
	info, _ := remoteFile.Stat()
	fileName := path.Base(remotePath)

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.Header().Set("Content-Type", "application/octet-stream")
	if info != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}

	io.Copy(w, remoteFile)
}

// ========== SFTP: 上传文件 ==========

func handleRemoteUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	remotePath := r.URL.Query().Get("path")
	if projectName == "" || remotePath == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少参数"})
		return
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "项目不存在或未配置部署"})
		return
	}

	// 解析 multipart form
	r.ParseMultipartForm(100 << 20) // 100 MB max
	file, header, err := r.FormFile("file")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "读取上传文件失败: " + err.Error()})
		return
	}
	defer file.Close()

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "SFTP 连接失败"})
		return
	}
	defer sftpClient.Close()

	// 目标路径: remotePath + filename
	targetPath := path.Join(remotePath, header.Filename)

	// 创建远程文件
	remoteFile, err := sftpClient.Create(targetPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "创建远程文件失败: " + err.Error()})
		return
	}
	defer remoteFile.Close()

	written, err := io.Copy(remoteFile, file)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "上传失败: " + err.Error()})
		return
	}

	log.Printf("📤 [%s] 上传 %s (%d 字节) → %s\n", projectName, header.Filename, written, targetPath)
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"filename": header.Filename,
		"size":     written,
		"path":     targetPath,
	})
}

// ========== SFTP: 删除文件/目录 ==========

func handleRemoteDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	remotePath := r.URL.Query().Get("path")
	if projectName == "" || remotePath == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少参数"})
		return
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "项目不存在或未配置部署"})
		return
	}

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "SFTP 连接失败"})
		return
	}
	defer sftpClient.Close()

	// 检查是文件还是目录
	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "路径不存在: " + err.Error()})
		return
	}

	if info.IsDir() {
		err = sftpClient.RemoveDirectory(remotePath)
	} else {
		err = sftpClient.Remove(remotePath)
	}

	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "删除失败: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "已删除"})
}

// ========== SFTP: 创建目录 ==========

func handleRemoteMkdir(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	projectName := r.URL.Query().Get("project")
	source := r.URL.Query().Get("source")
	remotePath := r.URL.Query().Get("path")
	if projectName == "" || remotePath == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少参数"})
		return
	}

	deploy := resolveDeployConfig(projectName, source)
	if deploy == nil || deploy.Host == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "项目不存在或未配置部署"})
		return
	}

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "SFTP 连接失败"})
		return
	}
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(remotePath); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "创建目录失败: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "目录已创建"})
}

// POST /api/remote/disconnect?project=xxx — 断开并清除缓存的 SSH 连接
func handleRemoteDisconnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}
	projectName := r.URL.Query().Get("project")
	if projectName == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 project 参数"})
		return
	}
	sshMu.Lock()
	if c, ok := sshClients[projectName]; ok {
		c.client.Close()
		delete(sshClients, projectName)
		log.Printf("🔌 已断开 SSH 连接: %s\n", projectName)
	}
	sshMu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "已断开"})
}

// ========== 独立服务器管理 (servers.json) ==========

type StandaloneServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	AuthType     string `json:"auth_type"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	Note         string `json:"note,omitempty"`
}

type ServerList struct {
	Servers []StandaloneServer `json:"servers"`
}

func serversFilePath(ciDir string) string {
	return filepath.Join(ciDir, "servers.json")
}

func loadServers(ciDir string) *ServerList {
	path := serversFilePath(ciDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ServerList{Servers: []StandaloneServer{}}
	}
	var list ServerList
	if err := json.Unmarshal(data, &list); err != nil {
		return &ServerList{Servers: []StandaloneServer{}}
	}
	if list.Servers == nil {
		list.Servers = []StandaloneServer{}
	}
	// 自动迁移：检测到明文密码则加密回写
	migrateServerPasswords(ciDir, &list)
	return &list
}

// migrateServerPasswords 检测 servers.json 中的明文密码并加密回写，
// 确保磁盘上不存留明文密码。
func migrateServerPasswords(ciDir string, list *ServerList) {
	var changed bool
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	for i := range list.Servers {
		s := &list.Servers[i]
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			enc, err := security.EncryptPassword(s.Password, key)
			if err == nil {
				list.Servers[i].Password = enc
				changed = true
			}
		}
	}
	if changed {
		saveServers(ciDir, list)
	}
}

// sanitizeDeploy 返回部署配置的脱敏副本（清空密码字段），用于 API 响应。
func sanitizeDeploy(deploy map[string]any) map[string]any {
	out := make(map[string]any, len(deploy))
	for k, v := range deploy {
		if k == "password" {
			out[k] = ""
			continue
		}
		out[k] = v
	}
	return out
}

// sanitizeServer 返回独立服务器的脱敏副本（清空密码）。
func sanitizeServer(s StandaloneServer) StandaloneServer {
	s.Password = ""
	return s
}

func saveServers(ciDir string, list *ServerList) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return security.AtomicWriteFile(serversFilePath(ciDir), data, 0600)
}

// GET  /api/remote/servers — 列出所有独立服务器
// POST /api/remote/servers — 添加一台独立服务器
func handleRemoteServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		return
	}

	if r.Method == "POST" {
		var s StandaloneServer
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
			return
		}
		if s.Name == "" || s.Host == "" || s.User == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "名称、主机、用户名不能为空"})
			return
		}
		if s.Port == 0 {
			s.Port = 22
		}
		// 加密明文密码
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			key, err := security.LoadOrCreateKey(ciDir)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": "初始化密钥失败: " + err.Error()})
				return
			}
			enc, err := security.EncryptPassword(s.Password, key)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": "密码加密失败: " + err.Error()})
				return
			}
			s.Password = enc
		}
		list := loadServers(ciDir)
		for _, existing := range list.Servers {
			if existing.Name == s.Name {
				json.NewEncoder(w).Encode(map[string]string{"error": "服务器名称已存在"})
				return
			}
		}
		list.Servers = append(list.Servers, s)
		if err := saveServers(ciDir, list); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
			return
		}
		log.Printf("🖥️ 添加服务器: %s (%s@%s)\n", s.Name, s.User, s.Host)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// GET 返回脱敏列表（不含密码）
	list := loadServers(ciDir)
	safe := make([]StandaloneServer, 0, len(list.Servers))
	for _, s := range list.Servers {
		safe = append(safe, sanitizeServer(s))
	}
	json.NewEncoder(w).Encode(map[string]any{"servers": safe})
}

// DELETE /api/remote/server?name=xxx — 删除独立服务器
func handleRemoteServerDelete(w http.ResponseWriter, r *http.Request) {
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
	name := r.URL.Query().Get("name")
	if name == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 name 参数"})
		return
	}
	list := loadServers(ciDir)
	idx := -1
	for i, s := range list.Servers {
		if s.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		json.NewEncoder(w).Encode(map[string]string{"error": "服务器不存在"})
		return
	}
	list.Servers = append(list.Servers[:idx], list.Servers[idx+1:]...)
	if err := saveServers(ciDir, list); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
		return
	}
	log.Printf("🗑️ 删除服务器: %s\n", name)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleRemoteAllServers 返回项目部署 + 独立服务器的合并列表
func handleRemoteAllServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		return
	}

	var items []map[string]any

	// 1. 从 projects.json 提取有部署配置的项目
	if data, err := os.ReadFile(filepath.Join(ciDir, "projects.json")); err == nil {
		var cfg struct {
			Projects []map[string]any `json:"projects"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			for _, p := range cfg.Projects {
				if deploy, ok := p["deploy"].(map[string]any); ok {
					if host, _ := deploy["host"].(string); host != "" {
						name, _ := p["name"].(string)
						pType, _ := p["type"].(string)
					items = append(items, map[string]any{
						"name":   "📋 " + name,
						"source": "project",
						"ref":    name,
						"type":   pType,
						"deploy": sanitizeDeploy(deploy),
					})
					}
				}
			}
		}
	}

	// 2. 从 servers.json 提取独立服务器（password 脱敏，不返回前端）
	sl := loadServers(ciDir)
	for _, s := range sl.Servers {
		items = append(items, map[string]any{
			"name":   "🖥️ " + s.Name,
			"source": "standalone",
			"ref":    s.Name,
			"type":   "Server",
			"deploy": map[string]any{
				"host":          s.Host,
				"port":          s.Port,
				"user":          s.User,
				"auth_type":     s.AuthType,
				"identity_file": s.IdentityFile,
				"password":      "",
			},
		})
	}

	if items == nil {
		items = []map[string]any{}
	}
	json.NewEncoder(w).Encode(map[string]any{"servers": items})
}

// loadStandaloneServerDeploy 通过名称查找独立服务器的部署配置
func loadStandaloneServerDeploy(name string) *config.DeployConfig {
	ciDir := findCiDir()
	if ciDir == "" {
		return nil
	}
	list := loadServers(ciDir)
	for _, s := range list.Servers {
		if s.Name == name {
			return &config.DeployConfig{
				Host:         s.Host,
				Port:         s.Port,
				User:         s.User,
				AuthType:     s.AuthType,
				IdentityFile: s.IdentityFile,
				Password:     s.Password,
			}
		}
	}
	return nil
}
