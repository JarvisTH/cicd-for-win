package serve

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"ci-cd/internal/runner"
)

// buildRunnerArgs 构建执行 ci-runner.ps1 的 PowerShell 参数列表。
func buildRunnerArgs(ciDir, projectPath, action, customCommand, customArgs string) []string {
	args := []string{"-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(ciDir, "ci-runner.ps1"),
		"-Action", action,
		"-ProjectPath", projectPath,
		"-Json"}
	if customCommand != "" {
		args = append(args, "-CustomCommand", customCommand)
	}
	if customArgs != "" {
		args = append(args, "-CustomArgs", customArgs)
	}
	return args
}

// execRunner 执行 PowerShell 脚本，解析 JSON 结果并持久化状态。
// 返回 result（成功时）和 errMsg（失败时），两者互斥。
func execRunner(ciDir, project, action string, args []string) (result runner.Result, errMsg string) {
	cmd := exec.Command("powershell.exe", args...)
	cmdStr := buildCommandString("powershell.exe", args)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	if jsonErr := json.Unmarshal([]byte(stdoutStr), &result); jsonErr == nil {
		result.Command = cmdStr
		if result.Status == "fail" && result.ErrorLog == "" && stderrStr != "" {
			result.ErrorLog = stderrStr
		}
		if action == "test" && result.Report != nil {
			saveTestReportToDisk(ciDir, result)
		}
		saveStepStatus(ciDir, result)
		return result, ""
	}

	errMsg = "脚本执行异常"
	if err != nil {
		errMsg = err.Error()
	}
	if stderrStr != "" {
		errMsg = stderrStr
	}
	saveStepStatus(ciDir, runner.Result{
		Project: project, Action: action, Status: "fail", ErrorLog: errMsg,
	})
	return runner.Result{}, errMsg
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

		customCommand, customArgs := findPipelineStepCommand(ciDir, project, action)
		projectPath := findProjectPath(ciDir, project)
		if projectPath == "" {
			projectPath = project
		}

		args := buildRunnerArgs(ciDir, projectPath, action, customCommand, customArgs)
		result, errMsg := execRunner(ciDir, project, action, args)
		if errMsg != "" {
			json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
			return
		}
		if data, mErr := json.Marshal(result); mErr == nil {
			w.Write(data)
		}
	}
}
