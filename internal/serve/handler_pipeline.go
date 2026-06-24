package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

// resetDownstreamSteps 重置指定步骤及其后续步骤的状态（删除状态文件，使其回到 pending）。
// 与前端 runAction 的重置逻辑保持一致。
func resetDownstreamSteps(ciDir, projectName, currentStep string) {
	defaultOrder := []string{"check", "build", "test", "push", "deploy"}
	idx := -1
	for i, s := range defaultOrder {
		if s == currentStep {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	// 删除当前步骤及之后所有步骤的状态文件
	for i := idx; i < len(defaultOrder); i++ {
		step := defaultOrder[i]
		path := filepath.Join(stepStatusDir(ciDir), projectName, step+".json")
		os.Remove(path)
	}
}

// getProjectEnabledSteps 返回项目启用的流水线步骤 ID 列表（保持配置顺序）。
// 无自定义流水线时返回默认顺序 [check, build, test, push, deploy]。
func getProjectEnabledSteps(proj *config.Project) []string {
	defaultOrder := []string{"check", "build", "test", "push", "deploy"}
	if proj == nil || proj.Pipeline == nil || len(proj.Pipeline.Steps) == 0 {
		return defaultOrder
	}
	var steps []string
	for _, s := range proj.Pipeline.Steps {
		if s.Enabled {
			steps = append(steps, s.ID)
		}
	}
	if len(steps) == 0 {
		return defaultOrder
	}
	return steps
}

// runPipelineStep 执行单个流水线步骤并返回结果，复用现有单步执行逻辑。
func runPipelineStep(proj *config.Project, ciDir, stepID string) (runner.Result, error) {
	// 优先使用自定义命令
	customCommand, customArgs := findPipelineStepCommand(ciDir, proj.Name, stepID)
	if customCommand != "" {
		return execPipelineCustomCommand(proj, stepID, customCommand, customArgs)
	}

	var result runner.Result
	var runErr error

	switch stepID {
	case "check":
		result, runErr = runner.RunCheck(*proj)
	case "build":
		result, runErr = runner.RunBuild(*proj)
	case "test":
		result, runErr = runner.RunTest(*proj)
	case "push":
		// RunPush 只返回 error，需手动构造 Result
		runErr = runner.RunPush(*proj)
		result = runner.Result{Project: proj.Name, Action: "push", Status: "pass"}
		if runErr != nil {
			result.Status = "fail"
			result.ErrorLog = runErr.Error()
		}
	case "deploy":
		target := "production"
		if proj.DeployTarget != "" {
			target = proj.DeployTarget
		}
		result, runErr = runner.RunDeploy(*proj, target)
	default:
		return runner.Result{}, fmt.Errorf("未知步骤: %s", stepID)
	}

	result.Project = proj.Name
	result.Action = stepID
	if result.Command == "" {
		result.Command = stepID
	}

	saveStepStatus(ciDir, result)
	if runErr != nil && result.Status != "fail" {
		failResult := runner.Result{
			Project: proj.Name, Action: stepID, Status: "fail", ErrorLog: runErr.Error(),
		}
		saveStepStatus(ciDir, failResult)
		return failResult, runErr
	}
	return result, nil
}

// execPipelineCustomCommand 执行用户自定义的流水线步骤命令（供流水线编排调用）。
func execPipelineCustomCommand(proj *config.Project, action, command, args string) (runner.Result, error) {
	start := time.Now()

	var cmd *exec.Cmd
	if args != "" {
		parts := strings.Fields(args)
		cmd = exec.Command(command, parts...)
	} else {
		cmd = exec.Command(command)
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
		errLog := strings.TrimSpace(stderr.String())
		if errLog == "" {
			errLog = err.Error()
		}
		result.ErrorLog = errLog
	} else {
		result.Status = "pass"
	}

	saveStepStatus(proj.CiDir, result)
	return result, nil
}

// pipelineRunHandler 执行单个项目的完整流水线（按步骤顺序，遇失败停止）。
func pipelineRunHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	projectName := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	cfg, err := config.Load(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "读取项目配置失败: " + err.Error()})
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
		respondJSON(w, 200, map[string]string{"status": "error", "message": "未找到项目: " + projectName})
		return
	}
	proj.CiDir = ciDir

	steps := getProjectEnabledSteps(proj)
	results := []runner.Result{}
	failed := false

	for _, step := range steps {
		// 先写入 running 状态，让前端轮询能实时看到执行中
		saveStepStatus(ciDir, runner.Result{
			Project: proj.Name, Action: step, Status: "running",
		})
		result, err := runPipelineStep(proj, ciDir, step)
		results = append(results, result)
		if err != nil || result.Status == "fail" {
			failed = true
			break
		}
	}

	status := "ok"
	if failed {
		status = "error"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"project": projectName,
		"results": results,
	})
}

// pipelineRunAllHandler 执行所有已启用项目的完整流水线。
func pipelineRunAllHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}

	cfg, err := config.Load(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		respondJSON(w, 200, map[string]string{"status": "error", "message": "读取项目配置失败: " + err.Error()})
		return
	}

	allResults := map[string][]runner.Result{}
	anyFailed := false

	for i := range cfg.Projects {
		proj := &cfg.Projects[i]
		if !proj.Enabled {
			continue
		}
		proj.CiDir = ciDir

		steps := getProjectEnabledSteps(proj)
		results := []runner.Result{}
		for _, step := range steps {
			// 先写入 running 状态，让前端轮询能实时看到执行中
			saveStepStatus(ciDir, runner.Result{
				Project: proj.Name, Action: step, Status: "running",
			})
			result, err := runPipelineStep(proj, ciDir, step)
			results = append(results, result)
			if err != nil || result.Status == "fail" {
				anyFailed = true
				break
			}
		}
		allResults[proj.Name] = results
	}

	status := "ok"
	if anyFailed {
		status = "error"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"results": allResults,
	})
}
