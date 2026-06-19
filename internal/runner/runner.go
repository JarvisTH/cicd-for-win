package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ci-cd/internal/config"
)

type Result struct {
	Project  string `json:"project"`
	Action   string `json:"action"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
	Steps    []Step `json:"steps,omitempty"`
	Report   *TestReport `json:"report,omitempty"`
}

type Step struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
}

type TestReport struct {
	Total    int           `json:"total"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Skipped  int           `json:"skipped"`
	Coverage string        `json:"coverage,omitempty"`
	Failures []TestFailure `json:"failures,omitempty"`
	RawLog   string        `json:"raw_log,omitempty"`
}

type TestFailure struct {
	Suite   string `json:"suite"`
	Test    string `json:"test"`
	Message string `json:"message"`
}

func runPowershell(project config.Project, script string, args ...string) (Result, error) {
	psScript := filepath.Join(project.CiDir, script)
	cmdArgs := []string{"-ExecutionPolicy", "Bypass", "-File", psScript}
	cmdArgs = append(cmdArgs, args...)
	cmdArgs = append(cmdArgs, "-Json")

	start := time.Now()
	cmd := exec.Command("powershell.exe", cmdArgs...)
	output, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		return Result{
			Project:  project.Name,
			Action:   script,
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", elapsed.Seconds()),
		}, err
	}

	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		result = Result{Project: project.Name, Action: script, Status: "pass", Duration: fmt.Sprintf("%.1fs", elapsed.Seconds())}
	}
	result.Project = project.Name
	result.Action = script
	return result, nil
}

func RunCheck(project config.Project) (Result, error) {
	return runPowershell(project, "ci-runner.ps1", "-Action", "check", "-ProjectPath", project.Path)
}

func RunBuild(project config.Project) (Result, error) {
	return runPowershell(project, "ci-runner.ps1", "-Action", "build", "-ProjectPath", project.Path)
}

func RunTest(project config.Project) (Result, error) {
	result, err := runPowershell(project, "ci-runner.ps1", "-Action", "test", "-ProjectPath", project.Path)
	// 如果有测试报告，保存到磁盘
	if err == nil && result.Report != nil {
		saveTestReport(project, result)
	}
	return result, err
}

// saveTestReport 将测试报告保存到 reports/{project}/{timestamp}.json
func saveTestReport(project config.Project, result Result) {
	reportsDir := filepath.Join(project.CiDir, "reports", project.Name)
	os.MkdirAll(reportsDir, 0755)

	now := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("test-%s.json", now)
	path := filepath.Join(reportsDir, filename)

	// 只保存 report 相关信息，不保存 action/duration 等运行时数据
	saveData := Result{
		Project:  project.Name,
		Action:   "test",
		Status:   result.Status,
		Duration: result.Duration,
		Report:   result.Report,
	}
	data, _ := json.MarshalIndent(saveData, "", "  ")
	os.WriteFile(path, data, 0644)

	// 清理旧报告：只保留最近 20 条
	cleanOldReports(reportsDir, 20)
}

// cleanOldReports 清理旧报告，只保留最近的 n 条
func cleanOldReports(dir string, keep int) {
	pattern := filepath.Join(dir, "test-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	if len(files) <= keep {
		return
	}
	// 文件已按字母序排列（时间戳在文件名中），删除最老的
	for i := 0; i < len(files)-keep; i++ {
		os.Remove(files[i])
	}
}

func RunHooks(project config.Project) error {
	hooksScript := filepath.Join(project.CiDir, "install-hooks.bat")
	cmd := exec.Command("cmd.exe", "/c", hooksScript)
	return cmd.Run()
}

func RunDeploy(project config.Project, target string) (Result, error) {
	deployScript := filepath.Join(project.CiDir, "cd-deploy.ps1")
	cmd := exec.Command("powershell.exe",
		"-ExecutionPolicy", "Bypass",
		"-File", deployScript,
		"-ProjectName", project.Name,
		"-Action", "upload",
	)
	start := time.Now()
	output, err := cmd.Output()
	elapsed := time.Since(start)
	if err != nil {
		return Result{Project: project.Name, Action: "deploy", Status: "fail", Duration: fmt.Sprintf("%.1fs", elapsed.Seconds())}, err
	}
	var result Result
	json.Unmarshal(output, &result)
	result.Project = project.Name
	result.Duration = fmt.Sprintf("%.1fs", elapsed.Seconds())
	return result, nil
}

func RunPush(project config.Project) error {
	pushScript := filepath.Join(project.CiDir, "ci-push.ps1")
	cmd := exec.Command("powershell.exe",
		"-ExecutionPolicy", "Bypass",
		"-File", pushScript,
		"-ProjectName", project.Name,
	)
	return cmd.Run()
}

func RunStatus(project config.Project) {
	switch {
	case hasDir(project.Path, "dist"):
		fmt.Printf("[%s] ✅ dist/ 存在\n", project.Name)
	case hasJar(project.Path):
		fmt.Printf("[%s] ✅ target/*.jar 存在\n", project.Name)
	default:
		fmt.Printf("[%s] ⚪ 未构建\n", project.Name)
	}

	cmd := exec.Command("git.exe", "-C", project.Path, "status", "--porcelain")
	output, _ := cmd.Output()
	if len(output) > 0 {
		fmt.Printf("[%s] ⚠ 有未提交的变更\n", project.Name)
	} else {
		fmt.Printf("[%s] ✅ Git 工作区干净\n", project.Name)
	}
}

func hasDir(path, dir string) bool {
	info, err := os.Stat(filepath.Join(path, dir))
	return err == nil && info.IsDir()
}

func hasJar(path string) bool {
	matches, _ := filepath.Glob(filepath.Join(path, "target", "*.jar"))
	return len(matches) > 0
}

// HasDist 检查项目是否有构建产物（导出给 cmd 包使用）
func HasDist(path string) bool {
	return hasDir(path, "dist") || hasJar(path)
}

// ReadProjectVersion 读取项目的版本号（导出给 cmd 包使用）
func ReadProjectVersion(path string) string {
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

// ReadGitInfo 读取项目的 Git 分支和提交（导出给 cmd 包使用）
func ReadGitInfo(path string) (branch, commit string) {
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git.exe", "-C", path, "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	return
}

// ToolSchema 描述一个工具
type ToolSchema struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  *ToolParam `json:"parameters,omitempty"`
}

type ToolParam struct {
	Type       string              `json:"type"`
	Properties map[string]ParamProp `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type ParamProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func GenerateSchema(format string) string {
	tools := []ToolSchema{
		{
			Name: "ci_check", Description: "对指定项目执行代码检查（TypeScript 类型检查 + ESLint/Checkstyle）。不传 project 则检查所有已启用项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选，不传则检查所有项目）"},
				},
			},
		},
		{
			Name: "ci_test", Description: "对指定项目执行单元测试，自动检测 Jest/Vitest/Maven 测试框架，返回结构化报告（含通过数、失败数、覆盖率、失败详情）。不传 project 则测试所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选，不传则测试所有项目）"},
				},
			},
		},
		{
			Name: "ci_build", Description: "对指定项目执行完整构建（npm run build / mvn package）。不传 project 则构建所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选，不传则构建所有项目）"},
				},
			},
		},
		{
			Name: "ci_push", Description: "将指定项目的代码推送到所有已配置的 Git 远程仓库。不传 project 则推送所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选，不传则推送所有项目）"},
				},
			},
		},
		{
			Name: "ci_deploy", Description: "将指定项目的构建产物通过 SFTP 上传到远程服务器并启动服务。需要先配置部署信息。不传 project 则部署所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选，不传则部署所有项目）"},
				},
			},
		},
		{
			Name: "ci_hooks", Description: "安装 Git hooks 到指定项目（pre-commit / pre-push 钩子）。不传 project 则安装到所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选）"},
				},
			},
		},
		{
			Name: "ci_list", Description: "列出所有已配置的项目及启停状态。无需参数。",
		},
		{
			Name: "ci_status", Description: "查看指定项目的构建产物状态（dist/ 或 target/*.jar 是否存在）和 Git 工作区是否干净。不传 project 则查看所有项目。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（可选）"},
				},
			},
		},
		{
			Name: "ci_passwd", Description: "修改或重置 Web UI 登录密码。不传参数则重置为默认密码 admin/123456。需要服务器命令行权限。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"username": {Type: "string", Description: "用户名（可选，默认 admin）"},
					"password": {Type: "string", Description: "新密码，不少于 6 位（可选，默认 123456）"},
				},
			},
		},
		{
			Name: "ci_report", Description: "查看或删除指定项目的测试报告。查看最新报告、列出历史、或删除指定报告。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（必填）"},
					"delete":  {Type: "string", Description: "要删除的报告 ID（可选，通过 --list 可获取所有报告的 ID）"},
				},
				Required: []string{"project"},
			},
		},
		{
			Name: "ci_serve", Description: "启动 Web UI 服务器，在浏览器中管理 CI/CD 流程。默认端口 8080。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"port": {Type: "string", Description: "监听端口（可选，默认 8080）"},
				},
			},
		},
		{
			Name: "ci_doctor", Description: "诊断 CI/CD 环境状态，检查工具链（Go/Git/Node/Java/Maven）和配置文件完整性。",
		},
		{
			Name: "ci_project_list", Description: "列出所有项目的详细信息，包括路径、版本、Git 分支、部署目标和构建产物状态。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"json": {Type: "string", Description: "设为 true 则输出 JSON 格式"},
				},
			},
		},
	}
	switch format {
	case "openai":
		result := map[string]any{"tools": tools}
		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data)
	case "mcp":
		data, _ := json.MarshalIndent(tools, "", "  ")
		return string(data)
	default:
		output := "可用命令:\n"
		for _, t := range tools {
			output += fmt.Sprintf("  %s - %s\n", t.Name, t.Description)
		}
		return output
	}
}
