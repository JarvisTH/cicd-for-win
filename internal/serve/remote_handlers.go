package serve

import (
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
)

// ========== 远程项目列表 ==========

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
	var filtered []map[string]any
	for _, p := range cfg.Projects {
		if deploy, ok := p["deploy"].(map[string]any); ok {
			if host, _ := deploy["host"].(string); host != "" {
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

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v\n", err)
		return
	}
	defer conn.Close()

	client, err := getSSHClient(projectName, deploy)
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		conn.Close()
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": "创建 SSH session 失败: " + err.Error()})
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ECHOE:         1,
		ssh.ECHOK:         1,
		ssh.ECHONL:        0,
		ssh.ICANON:        1,
		ssh.ISIG:          1,
		ssh.ICRNL:         1,
		ssh.OPOST:         1,
		ssh.ONLCR:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 40, 120, modes); err != nil {
		conn.WriteJSON(map[string]string{"error": "请求伪终端失败: " + err.Error()})
		return
	}

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

	conn.WriteJSON(map[string]string{"type": "connected", "message": "SSH 终端已连接"})

	var wg sync.WaitGroup
	wg.Add(2)

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				session.Close()
				return
			}
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

	if err := session.Shell(); err != nil {
		conn.WriteJSON(map[string]string{"error": "启动 shell 失败: " + err.Error()})
		return
	}

	time.Sleep(200 * time.Millisecond)
	stdin.Write([]byte("\n"))

	wg.Wait()
}

// ========== SFTP: 文件操作 ==========

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

	json.NewEncoder(w).Encode(map[string]any{
		"path":  remotePath,
		"files": files,
	})
}

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

	info, _ := remoteFile.Stat()
	fileName := path.Base(remotePath)

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.Header().Set("Content-Type", "application/octet-stream")
	if info != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}

	io.Copy(w, remoteFile)
}

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

	r.ParseMultipartForm(100 << 20)
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

	targetPath := path.Join(remotePath, header.Filename)
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
		"status": "ok", "filename": header.Filename, "size": written, "path": targetPath,
	})
}

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
