package serve

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
	"ci-cd/internal/sshutil"
)

func pushHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(ciDir, "ci-push.ps1"),
		"-ProjectName", project)
	err := cmd.Run()
	if err == nil {
		respondJSON(w, 200,map[string]string{"status": "pass", "message": "推送成功"})
	} else {
		respondJSON(w, 200,map[string]string{"status": "fail", "message": err.Error()})
	}
}

func deployTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	host := r.URL.Query().Get("host")
	portStr := r.URL.Query().Get("port")
	user := r.URL.Query().Get("user")
	authType := r.URL.Query().Get("auth_type")
	keyFile := r.URL.Query().Get("identity_file")

	if host == "" || user == "" {
		respondJSON(w, 200,map[string]string{"status": "error", "message": "缺少主机或用户名"})
		return
	}
	port := 22
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p < 65536 {
			port = p
		}
	}

	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	deploy := &config.DeployConfig{
		Host: host, Port: port, User: user,
		AuthType: authType, IdentityFile: keyFile,
	}
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		respondJSON(w, 200,map[string]string{"status": "error", "message": err.Error()})
		return
	}
	// 测试连接缩短超时
	sshCfg.Timeout = 5 * time.Second

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshCfg)
	if err != nil {
		respondJSON(w, 200,map[string]string{"status": "error", "message": err.Error()})
		return
	}
	client.Close()
	respondJSON(w, 200,map[string]string{"status": "ok", "message": "连接成功"})
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "upload"
	}
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(ciDir, "cd-deploy.ps1"),
		"-ProjectName", project,
		"-Action", action,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	detail := ""
	if stderrStr != "" {
		detail = stderrStr
	} else if stdoutStr != "" {
		detail = stdoutStr
	} else if err != nil {
		detail = err.Error()
	}

	if err == nil {
		saveStepStatus(ciDir, runner.Result{
			Project:  project,
			Action:   "deploy",
			Status:   "pass",
			Duration: detail,
		})
		respondJSON(w, 200,map[string]string{"status": "pass", "message": "部署成功", "detail": detail})
	} else {
		saveStepStatus(ciDir, runner.Result{
			Project:  project,
			Action:   "deploy",
			Status:   "fail",
			ErrorLog: detail,
		})
		respondJSON(w, 200,map[string]string{"status": "fail", "message": "部署失败", "detail": detail})
	}
}
