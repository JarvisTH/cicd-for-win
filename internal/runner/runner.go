// Package runner 提供 CI/CD 流水线的核心执行逻辑。
//
// runner.go — 包主入口，定义数据结构并暴露公共 API。
// Run* 函数现在直接使用 Go 实现，不再调用 PowerShell 脚本。
// 保留 Executor 接口用于测试 mock。
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ci-cd/internal/config"
)

// Result 表示一次 CI/CD 操作的执行结果。
type Result struct {
	Project  string       `json:"project"`
	Action   string       `json:"action"`
	Status   string       `json:"status"`
	Duration string       `json:"duration"`
	Command  string       `json:"command,omitempty"`   // 实际执行的命令（供前端日志展示）
	ErrorLog string       `json:"error_log,omitempty"`
	Steps    []Step       `json:"steps,omitempty"`
	Report   *TestReport  `json:"report,omitempty"`
}

// Step 表示流水线中的单个步骤。
type Step struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
	ErrorLog string `json:"error_log,omitempty"`
}

// TestReport 表示测试执行的结构化报告。
type TestReport struct {
	Total    int           `json:"total"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Skipped  int           `json:"skipped"`
	Coverage string        `json:"coverage,omitempty"`
	Failures []TestFailure `json:"failures,omitempty"`
	RawLog   string        `json:"raw_log,omitempty"`
}

// TestFailure 表示单个测试失败的详情。
type TestFailure struct {
	Suite   string `json:"suite"`
	Test    string `json:"test"`
	Message string `json:"message"`
}

// Executor 接口允许在测试中替换执行逻辑。
// Run 执行 script，传入项目信息和参数，返回 JSON 格式的 Result。
type Executor interface {
	Run(project config.Project, script string, args ...string) (Result, error)
}

// GoExecutor 使用 Go 原生实现执行 CI/CD 操作。
type GoExecutor struct{}

// Run 根据 script 名称路由到对应的 Go 实现。
func (e *GoExecutor) Run(project config.Project, script string, args ...string) (Result, error) {
	// 解析参数
	action := ""
	for i, a := range args {
		if a == "-Action" && i+1 < len(args) {
			action = args[i+1]
		}
	}

	switch script {
	case "ci-runner.ps1":
		return runCIRunnerAction(project, action, args)
	case "cd-deploy.ps1":
		return runDeployAction(project, action, args)
	case "ci-push.ps1":
		return runPushAction(project)
	default:
		return Result{}, fmt.Errorf("未知脚本: %s", script)
	}
}

// runCIRunnerAction 路由 ci-runner.ps1 的 action 到 Go 实现。
func runCIRunnerAction(project config.Project, action string, args []string) (Result, error) {
	projectPath := project.Path
	projectType := DetectProjectType(projectPath)

	// 解析 RuleStates 参数
	ruleStates := parseRuleStates(args)

	switch action {
	case "check":
		return RunCheckInternal(projectPath, projectType, ruleStates, project.CiDir)
	case "build":
		return RunBuildInternal(projectPath, projectType, project.CiDir)
	case "test":
		result, report, err := RunTestInternal(projectPath, projectType, project.CiDir)
		if err == nil && report != nil {
			result.Report = report
			saveTestReport(project, result)
		}
		result.Action = "test"
		result.Project = project.Name
		return result, err
	default:
		return Result{
			Project:  project.Name,
			Action:   action,
			Status:   "fail",
			Duration: "0.0s",
			ErrorLog: fmt.Sprintf("未知 action: %s", action),
		}, fmt.Errorf("未知 action: %s", action)
	}
}

// runDeployAction 路由 cd-deploy.ps1 的 action 到 Go 实现。
func runDeployAction(project config.Project, action string, args []string) (Result, error) {
	ciDir := project.CiDir

	switch action {
	case "upload":
		return RunDeployInternal(project, ciDir)
	case "start", "stop", "status", "test":
		return RunDeployAction(project, ciDir, action)
	default:
		return Result{
			Project:  project.Name,
			Action:   action,
			Status:   "fail",
			Duration: "0.0s",
			ErrorLog: fmt.Sprintf("未知部署 action: %s", action),
		}, fmt.Errorf("未知部署 action: %s", action)
	}
}

// runPushAction 执行 push 操作。
func runPushAction(project config.Project) (Result, error) {
	err := RunPushInternal(project)
	if err != nil {
		return Result{
			Project:  project.Name,
			Action:   "push",
			Status:   "fail",
			Duration: "0.0s",
			ErrorLog: err.Error(),
		}, err
	}
	return Result{
		Project:  project.Name,
		Action:   "push",
		Status:   "pass",
		Duration: "0.0s",
	}, nil
}

// parseRuleStates 从 args 中解析 RuleStates JSON 参数。
func parseRuleStates(args []string) map[string]bool {
	for i, a := range args {
		if a == "-RuleStates" && i+1 < len(args) {
			var states []struct {
				ID      string `json:"id"`
				Enabled bool   `json:"enabled"`
			}
			if err := json.Unmarshal([]byte(args[i+1]), &states); err == nil {
				result := make(map[string]bool)
				for _, s := range states {
					result[s.ID] = s.Enabled
				}
				return result
			}
		}
	}
	return nil
}

// defaultExec 包级默认执行器，Run* 函数通过它执行。测试时可替换为 MockExecutor。
var defaultExec Executor = &GoExecutor{}

// ResetForTest 重置包级全局变量到默认值，供测试使用。
func ResetForTest() {
	defaultExec = &GoExecutor{}
	defaultCmdRunner = &osCommandRunner{}
}

// RunCheck 对项目执行代码检查。
func RunCheck(project config.Project) (Result, error) {
	result, err := defaultExec.Run(project, "ci-runner.ps1", "-Action", "check", "-ProjectPath", project.Path)
	result.Project = project.Name
	result.Action = "check"
	return result, err
}

// RunBuild 对项目执行完整构建。
func RunBuild(project config.Project) (Result, error) {
	result, err := defaultExec.Run(project, "ci-runner.ps1", "-Action", "build", "-ProjectPath", project.Path)
	result.Project = project.Name
	result.Action = "build"
	return result, err
}

// RunTest 对项目执行单元测试。
func RunTest(project config.Project) (Result, error) {
	result, err := defaultExec.Run(project, "ci-runner.ps1", "-Action", "test", "-ProjectPath", project.Path)
	result.Project = project.Name
	result.Action = "test"
	return result, err
}

// RunPush 推送到所有 Git 远程仓库。
func RunPush(project config.Project) error {
	result, err := defaultExec.Run(project, "ci-push.ps1", "-ProjectName", project.Name)
	if err != nil {
		return err
	}
	if result.Status != "pass" {
		return fmt.Errorf(result.ErrorLog)
	}
	return nil
}

// RunDeploy 部署项目到远程服务器。
func RunDeploy(project config.Project, target string) (Result, error) {
	args := []string{"-ProjectName", project.Name, "-Action", "upload"}
	if target != "" {
		args = append(args, "-Target", target)
	}
	result, err := defaultExec.Run(project, "cd-deploy.ps1", args...)
	result.Project = project.Name
	result.Action = "deploy"
	return result, err
}

// RunHooks 安装 Git hooks 到项目。
func RunHooks(project config.Project) error {
	hooksScript := filepath.Join(project.CiDir, "install-hooks")
	// 跨平台：Windows 用 .bat，其他用 .sh
	if _, err := os.Stat(hooksScript + ".bat"); err == nil {
		return exec.Command("cmd.exe", "/c", hooksScript+".bat").Run()
	}
	if _, err := os.Stat(hooksScript + ".sh"); err == nil {
		return exec.Command("sh", hooksScript+".sh").Run()
	}
	return fmt.Errorf("找不到 install-hooks 脚本 (%s.bat / %s.sh)", hooksScript, hooksScript)
}

// RunStatus 查看项目当前状态。
func RunStatus(project config.Project) {
	switch {
	case hasDir(project.Path, "dist"):
		fmt.Printf("[%s] ✅ dist/ 存在\n", project.Name)
	case hasJar(project.Path):
		fmt.Printf("[%s] ✅ target/*.jar 存在\n", project.Name)
	default:
		fmt.Printf("[%s] ⚪ 未构建\n", project.Name)
	}

	ctx := context.Background()
	gitResult := runGit(ctx, project.Path, "status", "--porcelain")
	if gitResult.ExitCode == 0 && gitResult.Stdout != "" {
		fmt.Printf("[%s] ⚠ 有未提交的变更\n", project.Name)
	} else {
		fmt.Printf("[%s] ✅ Git 工作区干净\n", project.Name)
	}
}

// saveTestReport 将测试报告保存到 reports/{project}/{timestamp}.json。
func saveTestReport(project config.Project, result Result) {
	reportsDir := filepath.Join(project.CiDir, "reports", project.Name)
	os.MkdirAll(reportsDir, config.DirPermDefault)

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
	os.WriteFile(path, data, config.FilePermDefault)

	// 清理旧报告：只保留最近 20 条
	cleanOldReports(reportsDir, config.MaxReportsKeep)
}

// cleanOldReports 清理旧报告，只保留最近的 n 条。
// 按文件名（时间戳）排序，删除最老的，确保只保留最新的 keep 条。
func cleanOldReports(dir string, keep int) {
	pattern := filepath.Join(dir, "test-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	if len(files) <= keep {
		return
	}
	sort.Strings(files) // 文件名含时间戳，按字典序 = 时间序
	for i := 0; i < len(files)-keep; i++ {
		os.Remove(files[i])
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

// HasDist 检查项目是否有构建产物（导出给 cmd 包使用）。
func HasDist(path string) bool {
	return hasDir(path, "dist") || hasJar(path)
}

// ReadProjectVersion 读取项目的版本号。
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

// ReadGitInfo 读取项目的 Git 分支和提交信息。
func ReadGitInfo(path string) (branch, commit string) {
	ctx := context.Background()
	if out := runGit(ctx, path, "rev-parse", "--abbrev-ref", "HEAD"); out.ExitCode == 0 {
		branch = strings.TrimSpace(out.Stdout)
	}
	if out := runGit(ctx, path, "rev-parse", "--short", "HEAD"); out.ExitCode == 0 {
		commit = strings.TrimSpace(out.Stdout)
	}
	return
}

// ToolSchema 定义 AI Agent 可用的工具描述。
type ToolSchema struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  *ToolParam `json:"parameters,omitempty"`
}

// ToolParam 定义工具的参数描述。
type ToolParam struct {
	Type       string                `json:"type"`
	Properties map[string]ParamProp  `json:"properties"`
	Required   []string              `json:"required,omitempty"`
}

// ParamProp 定义单个参数的属性。
type ParamProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// GenerateSchema 生成 AI Agent 可用的工具 Schema。
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
					"target":  {Type: "string", Description: "部署目标（可选，production/staging，默认 production）"},
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
			Name: "ci_report", Description: "查看、列出或删除指定项目的测试报告。返回结构化报告（含通过数、失败数、覆盖率、失败详情）。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"project": {Type: "string", Description: "项目名称（必填）"},
					"list":    {Type: "string", Description: "设为 true 则列出所有历史报告 ID，而非查看最新（可选）"},
					"delete":  {Type: "string", Description: "要删除的报告 ID（可选，先 --list 获取 ID）"},
					"json":    {Type: "string", Description: "设为 true 则输出 JSON 格式（可选）"},
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
			Name: "ci_log", Description: "查询或删除审计日志。可指定日期、级别过滤、关键字搜索。不传参数查询今天全部日志。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"date":    {Type: "string", Description: "日志日期（可选，格式 YYYY-MM-DD，默认今天）"},
					"level":   {Type: "string", Description: "日志级别过滤（可选，info/warn/error）"},
					"keyword": {Type: "string", Description: "关键字搜索（可选）"},
					"delete":  {Type: "string", Description: "要删除的日期（可选，格式 YYYY-MM-DD）"},
					"limit":   {Type: "string", Description: "返回条数（可选，默认 100）"},
				},
			},
		},
		{
			Name: "ci_rules_list", Description: "列出可用的代码检查规则文件。",
		},
		{
			Name: "ci_rules_view", Description: "查看指定规则文件的内容。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"file": {Type: "string", Description: "规则文件名（必填，如 eslint-vue.mjs 或 checkstyle.xml）"},
				},
				Required: []string{"file"},
			},
		},
		{
			Name: "ci_server_list", Description: "列出所有已配置的独立服务器。",
		},
		{
			Name: "ci_server_add", Description: "添加一台独立服务器（用于远程文件管理和终端访问）。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"name":      {Type: "string", Description: "服务器名称（必填）"},
					"host":      {Type: "string", Description: "主机地址（必填，IP 或域名）"},
					"user":      {Type: "string", Description: "SSH 用户名（必填）"},
					"port":      {Type: "string", Description: "SSH 端口（可选，默认 22）"},
					"auth_type": {Type: "string", Description: "认证方式（可选，key/password，默认 key）"},
					"key_path":  {Type: "string", Description: "SSH 私钥路径（可选，auth_type=key 时使用）"},
					"password":  {Type: "string", Description: "SSH 密码（可选，auth_type=password 时使用）"},
					"note":      {Type: "string", Description: "备注说明（可选）"},
				},
				Required: []string{"name", "host", "user"},
			},
		},
		{
			Name: "ci_server_delete", Description: "删除一台独立服务器。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"name": {Type: "string", Description: "服务器名称（必填）"},
				},
				Required: []string{"name"},
			},
		},
		{
			Name: "ci_remote_ls", Description: "列出远程服务器上的目录内容。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"server": {Type: "string", Description: "服务器名称（必填，项目名或独立服务器名）"},
					"path":   {Type: "string", Description: "远程路径（可选，默认 /）"},
					"source": {Type: "string", Description: "服务器来源（可选，project/standalone，默认 project）"},
				},
				Required: []string{"server"},
			},
		},
		{
			Name: "ci_remote_download", Description: "从远程服务器下载文件到本地。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"server": {Type: "string", Description: "服务器名称（必填）"},
					"path":   {Type: "string", Description: "远程文件路径（必填）"},
					"source": {Type: "string", Description: "服务器来源（可选，project/standalone）"},
				},
				Required: []string{"server", "path"},
			},
		},
		{
			Name: "ci_remote_upload", Description: "上传本地文件到远程服务器。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"server": {Type: "string", Description: "服务器名称（必填）"},
					"file":   {Type: "string", Description: "本地文件路径（必填）"},
					"path":   {Type: "string", Description: "远程目标目录（必填）"},
					"source": {Type: "string", Description: "服务器来源（可选，project/standalone）"},
				},
				Required: []string{"server", "file", "path"},
			},
		},
		{
			Name: "ci_remote_delete", Description: "删除远程服务器上的文件或目录。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"server": {Type: "string", Description: "服务器名称（必填）"},
					"path":   {Type: "string", Description: "远程路径（必填）"},
					"source": {Type: "string", Description: "服务器来源（可选，project/standalone）"},
				},
				Required: []string{"server", "path"},
			},
		},
		{
			Name: "ci_remote_mkdir", Description: "在远程服务器上创建目录。",
			Parameters: &ToolParam{
				Type: "object",
				Properties: map[string]ParamProp{
					"server": {Type: "string", Description: "服务器名称（必填）"},
					"path":   {Type: "string", Description: "远程目录路径（必填）"},
					"source": {Type: "string", Description: "服务器来源（可选，project/standalone）"},
				},
				Required: []string{"server", "path"},
			},
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
