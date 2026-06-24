package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

// apiHandler 返回 HTTP handler，使用 Go 原生 runner 执行 CI/CD 操作。
func apiHandler(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		projectName := r.URL.Query().Get("project")
		ciDir := findCiDir()
		if ciDir == "" {
			respondJSON(w, 200, map[string]string{"error": "找不到 ci-cd 目录"})
			return
		}

		// 从 projects.json 加载项目配置
		cfg, loadErr := config.Load(filepath.Join(ciDir, "projects.json"))
		if loadErr != nil {
			respondJSON(w, 200, map[string]string{"error": "读取项目配置失败: " + loadErr.Error()})
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
			respondJSON(w, 200, map[string]string{"error": "未找到项目: " + projectName})
			return
		}
		proj.CiDir = ciDir

		// 处理自定义命令
		customCommand, customArgs := findPipelineStepCommand(ciDir, projectName, action)
		if customCommand != "" {
			// 重置当前及下游步骤状态为 pending
			resetDownstreamSteps(ciDir, projectName, action)
			// 先写入 running 状态，让前端轮询能实时看到执行中
			saveStepStatus(ciDir, runner.Result{Project: projectName, Action: action, Status: "running"})
			runCustomCommand(w, proj, action, customCommand, customArgs)
			return
		}

		// 重置当前及下游步骤状态为 pending
		resetDownstreamSteps(ciDir, projectName, action)
		// 先写入 running 状态，让前端轮询能实时看到执行中
		saveStepStatus(ciDir, runner.Result{Project: projectName, Action: action, Status: "running"})

		var result runner.Result
		var runErr error

		switch action {
		case "check":
			result, runErr = runner.RunCheck(*proj)
		case "build":
			result, runErr = runner.RunBuild(*proj)
		case "test":
			result, runErr = runner.RunTest(*proj)
		case "status":
			runner.RunStatus(*proj)
			result = runner.Result{
				Project:  projectName,
				Action:   "status",
				Status:   "pass",
				Duration: "0.0s",
			}
		default:
			respondJSON(w, 200, map[string]string{"error": "未知操作: " + action})
			return
		}

		result.Project = projectName
		result.Action = action
		result.Command = action

		// 保存步骤状态
		saveStepStatus(ciDir, result)

		if runErr != nil {
			saveStepStatus(ciDir, runner.Result{
				Project: projectName, Action: action, Status: "fail", ErrorLog: runErr.Error(),
			})
			respondJSON(w, 200, map[string]string{"error": runErr.Error()})
			return
		}

		if data, mErr := json.Marshal(result); mErr == nil {
			w.Write(data)
		}
	}
}

// runCustomCommand 执行用户自定义的流水线步骤命令。
func runCustomCommand(w http.ResponseWriter, proj *config.Project, action, command, args string) {
	start := time.Now()

	cmd := exec.Command(command)
	if args != "" {
		parts := strings.Fields(args)
		cmd = exec.Command(command, parts...)
	}
	if proj != nil && proj.Path != "" {
		cmd.Dir = proj.Path
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start)

	result := runner.Result{
		Project:  proj.Name,
		Action:   action,
		Duration: fmt.Sprintf("%.1fs", elapsed.Seconds()),
	}
	if err != nil {
		result.Status = "fail"
		result.ErrorLog = strings.TrimSpace(stderr.String())
		if result.ErrorLog == "" {
			result.ErrorLog = err.Error()
		}
	} else {
		result.Status = "pass"
	}

	saveStepStatus(proj.CiDir, result)
	if data, mErr := json.Marshal(result); mErr == nil {
		w.Write(data)
	}
}
