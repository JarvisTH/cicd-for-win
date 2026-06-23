package serve

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
	"ci-cd/internal/sshutil"
)

func pushHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	proj := loadProjectByName(projectName)
	if proj == nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "未找到项目: " + projectName})
		return
	}
	proj.CiDir = ciDir

	err := runner.RunPushInternal(*proj)
	if err == nil {
		respondJSON(w, 200, map[string]string{"status": "pass", "message": "推送成功"})
	} else {
		respondJSON(w, 200, map[string]string{"status": "fail", "message": err.Error()})
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
		respondJSON(w, 200, map[string]string{"status": "error", "message": "缺少主机或用户名"})
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
		respondJSON(w, 200, map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	deploy := &config.DeployConfig{
		Host: host, Port: port, User: user,
		AuthType: authType, IdentityFile: keyFile,
	}
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	// 测试连接缩短超时
	sshCfg.Timeout = 5 * time.Second

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshCfg)
	if err != nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	client.Close()
	respondJSON(w, 200, map[string]string{"status": "ok", "message": "连接成功"})
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "upload"
	}
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	proj := loadProjectByName(projectName)
	if proj == nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "未找到项目: " + projectName})
		return
	}
	proj.CiDir = ciDir

	var result runner.Result
	var err error

	switch action {
	case "upload":
		result, err = runner.RunDeployInternal(*proj, ciDir)
	case "start", "stop", "status", "test":
		result, err = runner.RunDeployAction(*proj, ciDir, action)
	default:
		respondJSON(w, 200, map[string]string{"status": "error", "message": "未知部署操作: " + action})
		return
	}

	detail := ""
	if err != nil {
		detail = err.Error()
	} else {
		detail = result.ErrorLog
	}

	if err == nil && result.Status == "pass" {
		saveStepStatus(ciDir, runner.Result{
			Project:  projectName,
			Action:   "deploy",
			Status:   "pass",
			Duration: result.Duration,
		})
		respondJSON(w, 200, map[string]string{"status": "pass", "message": "部署成功", "detail": detail})
	} else {
		errMsg := detail
		if errMsg == "" {
			errMsg = "部署失败"
		}
		saveStepStatus(ciDir, runner.Result{
			Project:  projectName,
			Action:   "deploy",
			Status:   "fail",
			ErrorLog: errMsg,
		})
		respondJSON(w, 200, map[string]string{"status": "fail", "message": "部署失败", "detail": errMsg})
	}
}
