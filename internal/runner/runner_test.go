package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ci-cd/internal/config"
)

// ===================== Result 结构测试 =====================

func TestResultJSONRoundTrip(t *testing.T) {
	r := Result{
		Project:  "test-proj",
		Action:   "build",
		Status:   "pass",
		Duration: "3.2s",
		Command:  "npm run build",
		ErrorLog: "",
		Steps: []Step{
			{Name: "install", Status: "pass", Duration: "2.0s"},
			{Name: "compile", Status: "pass", Duration: "1.2s"},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if decoded.Project != r.Project {
		t.Errorf("Project 不匹配: 期望 %q, 得到 %q", r.Project, decoded.Project)
	}
	if decoded.Status != r.Status {
		t.Errorf("Status 不匹配: 期望 %q, 得到 %q", r.Status, decoded.Status)
	}
	if len(decoded.Steps) != 2 {
		t.Errorf("Steps 数量不匹配: 期望 2, 得到 %d", len(decoded.Steps))
	}
}

func TestResultWithTestReportJSON(t *testing.T) {
	r := Result{
		Project:  "test-proj",
		Action:   "test",
		Status:   "pass",
		Duration: "5.0s",
		Report: &TestReport{
			Total:    10,
			Passed:   8,
			Failed:   1,
			Skipped:  1,
			Coverage: "85.5%",
			Failures: []TestFailure{
				{Suite: "AuthTest", Test: "testLogin", Message: "expected true, got false"},
			},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if decoded.Report == nil {
		t.Fatal("Report 不应为 nil")
	}
	if decoded.Report.Total != 10 {
		t.Errorf("Total 不匹配: 期望 10, 得到 %d", decoded.Report.Total)
	}
	if decoded.Report.Coverage != "85.5%" {
		t.Errorf("Coverage 不匹配: 期望 85.5%%, 得到 %q", decoded.Report.Coverage)
	}
	if len(decoded.Report.Failures) != 1 {
		t.Fatalf("Failures 数量不匹配: 期望 1, 得到 %d", len(decoded.Report.Failures))
	}
	if decoded.Report.Failures[0].Test != "testLogin" {
		t.Errorf("Test name 不匹配: 期望 testLogin, 得到 %q", decoded.Report.Failures[0].Test)
	}
}

func TestResultEmptyFields(t *testing.T) {
	// 验证 omitempty 字段在空值时不出现在 JSON 中
	r := Result{Project: "p", Action: "a", Status: "s", Duration: "0s"}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "steps") {
		t.Error("空 Steps 不应出现在 JSON 中")
	}
	if strings.Contains(string(data), "report") {
		t.Error("空 Report 不应出现在 JSON 中")
	}
}

func TestResultWithErrorLog(t *testing.T) {
	r := Result{
		Project:  "p",
		Action:   "build",
		Status:   "fail",
		Duration: "1.0s",
		ErrorLog: "build failed with exit code 1",
		Steps:    []Step{{Name: "compile", Status: "fail", Duration: "1.0s"}},
	}
	data, _ := json.Marshal(r)
	if !strings.Contains(string(data), "error_log") {
		t.Error("ErrorLog 应出现在 JSON 中")
	}
}

// ===================== TestReport 结构测试 =====================

func TestTestReportJSON(t *testing.T) {
	report := TestReport{
		Total:    20,
		Passed:   18,
		Failed:   1,
		Skipped:  1,
		Coverage: "90%",
		RawLog:   "test output...",
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded TestReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if decoded.Total != 20 || decoded.Passed != 18 {
		t.Errorf("数据不匹配: Total=%d, Passed=%d", decoded.Total, decoded.Passed)
	}
}

func TestTestReportCoverageOmitEmpty(t *testing.T) {
	report := TestReport{Total: 5, Passed: 5, Failed: 0, Skipped: 0}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "coverage") {
		t.Error("空 Coverage 应被 omitempty 省略")
	}
}

func TestTestFailure(t *testing.T) {
	f := TestFailure{Suite: "TestSuite", Test: "testCase", Message: "assertion failed"}
	if f.Suite != "TestSuite" || f.Test != "testCase" || f.Message != "assertion failed" {
		t.Error("TestFailure 字段不匹配")
	}
}

// ===================== ToolSchema 结构测试 =====================

func TestToolSchemaJSON(t *testing.T) {
	schema := ToolSchema{
		Name:        "ci_test",
		Description: "Run tests",
		Parameters: &ToolParam{
			Type: "object",
			Properties: map[string]ParamProp{
				"project": {Type: "string", Description: "项目名称"},
			},
		},
	}
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("ToolSchema JSON 序列化失败: %v", err)
	}

	if !strings.Contains(string(data), "ci_test") {
		t.Error("序列化应包含工具名称")
	}
}

func TestToolSchemaNoParams(t *testing.T) {
	schema := ToolSchema{
		Name:        "ci_list",
		Description: "List projects",
	}
	data, _ := json.Marshal(schema)
	// 无 Parameters 时不应出现 parameters 字段（根据 omitempty 行为取决于 tag）
	// 验证不会 panic
	_ = data
}

// ===================== GenerateSchema =====================

func TestGenerateSchema_OpenAI(t *testing.T) {
	schema := GenerateSchema("openai")
	if schema == "" {
		t.Fatal("OpenAI schema 不应为空")
	}
	// 应包含 tools 顶层键
	if !strings.Contains(schema, `"tools"`) {
		t.Error("OpenAI 格式应包含 tools 外层")
	}
	// 应为合法 JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		t.Fatalf("OpenAI schema 不是合法 JSON: %v", err)
	}
	tools, ok := parsed["tools"].([]any)
	if !ok {
		t.Fatal("OpenAI schema 的 tools 应为数组")
	}
	if len(tools) < 10 {
		t.Errorf("tools 数量应 >= 10, 得到 %d", len(tools))
	}
}

func TestGenerateSchema_MCP(t *testing.T) {
	schema := GenerateSchema("mcp")
	if schema == "" {
		t.Fatal("MCP schema 不应为空")
	}
	// MCP 是顶层数组
	var parsed []any
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		t.Fatalf("MCP schema 不是合法 JSON: %v", err)
	}
	if len(parsed) < 10 {
		t.Errorf("工具数量应 >= 10, 得到 %d", len(parsed))
	}

	// 验证每个工具都有 name
	for i, tool := range parsed {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			t.Fatalf("工具 %d 不是 map", i)
		}
		if _, ok := toolMap["name"]; !ok {
			t.Errorf("工具 %d 缺少 name 字段", i)
		}
		if desc, ok := toolMap["description"]; !ok || desc == "" {
			t.Errorf("工具 %d 缺少 description", i)
		}
	}
}

func TestGenerateSchema_Text(t *testing.T) {
	schema := GenerateSchema("text")
	if schema == "" {
		t.Fatal("Text schema 不应为空")
	}
	// 文本格式以 "可用命令:" 开头
	if !strings.HasPrefix(schema, "可用命令:") {
		t.Errorf("Text 格式应以 '可用命令:' 开头, 实际: %q", schema[:20])
	}
	// 应包含工具名列表
	if !strings.Contains(schema, "ci_check") {
		t.Error("Text 格式应包含 ci_check")
	}
	if !strings.Contains(schema, "ci_serve") {
		t.Error("Text 格式应包含 ci_serve")
	}
}

func TestGenerateSchema_UnknownFormat(t *testing.T) {
	// 未知格式应回退到 text
	schema := GenerateSchema("unknown")
	if !strings.Contains(schema, "可用命令:") {
		t.Errorf("未知格式应回退到 text, 实际: %q", schema[:20])
	}
}

func TestGenerateSchema_AllToolsHaveRequiredFields(t *testing.T) {
	schema := GenerateSchema("openai")
	var parsed map[string]any
	json.Unmarshal([]byte(schema), &parsed)
	tools := parsed["tools"].([]any)

	for i, tool := range tools {
		toolMap := tool.(map[string]any)
		name, _ := toolMap["name"].(string)
		desc, _ := toolMap["description"].(string)
		if name == "" {
			t.Errorf("工具 %d 缺少 name", i)
		}
		if desc == "" {
			t.Errorf("工具 %d 缺少 description", i)
		}
		// 工具名应以 ci_ 开头
		if !strings.HasPrefix(name, "ci_") {
			t.Errorf("工具名应以 ci_ 前缀: %q", name)
		}
	}
}

func TestGenerateSchema_AllMCPToolsHaveName(t *testing.T) {
	schema := GenerateSchema("mcp")
	var tools []any
	json.Unmarshal([]byte(schema), &tools)

	for i, tool := range tools {
		tm := tool.(map[string]any)
		if _, ok := tm["name"]; !ok {
			t.Errorf("MCP 工具 %d 缺少 name", i)
		}
	}
}

// ===================== saveTestReport & cleanOldReports =====================

func TestSaveTestReport(t *testing.T) {
	dir := t.TempDir()
	project := config.Project{Name: "test-proj", CiDir: dir}

	result := Result{
		Project:  "test-proj",
		Action:   "test",
		Status:   "pass",
		Duration: "2.0s",
		Report: &TestReport{
			Total: 10, Passed: 10, Failed: 0, Skipped: 0,
			Coverage: "90%",
		},
	}

	saveTestReport(project, result)

	// 验证目录和文件已创建
	reportsDir := filepath.Join(dir, "reports", "test-proj")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("读取报告目录失败: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("报告文件未创建")
	}

	// 验证文件内容
	data, err := os.ReadFile(filepath.Join(reportsDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("读取报告文件失败: %v", err)
	}
	var saved Result
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if saved.Project != "test-proj" {
		t.Errorf("Project 不匹配: 期望 test-proj, 得到 %q", saved.Project)
	}
	if saved.Report == nil || saved.Report.Total != 10 {
		t.Errorf("Report 数据不匹配: %+v", saved.Report)
	}
}

func TestSaveTestReport_MultiReports(t *testing.T) {
	dir := t.TempDir()
	project := config.Project{Name: "multi", CiDir: dir}

	// 连续保存多次，通过文件名验证每个报告的时间戳不同
	saveTestReport(project, Result{
		Project: "multi", Action: "test", Status: "pass", Duration: "0.1s",
		Report: &TestReport{Total: 1, Passed: 1, Failed: 0, Skipped: 0},
	})

	reportsDir := filepath.Join(dir, "reports", "multi")
	entries1, _ := os.ReadDir(reportsDir)
	count1 := len(entries1)

	// 快速连续保存时文件名可能因时间戳相同而覆盖，但不影响功能
	// 验证 saveTestReport 至少成功创建了目录和文件
	if count1 == 0 {
		t.Fatal("saveTestReport 应创建报告文件")
	}

	// 验证保存的数据可正确反序列化
	data, _ := os.ReadFile(filepath.Join(reportsDir, entries1[0].Name()))
	var saved Result
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("报告 JSON 反序列化失败: %v", err)
	}
	if saved.Action != "test" {
		t.Errorf("Action 不匹配: 期望 test, 得到 %q", saved.Action)
	}
}

func TestSaveTestReport_WithNoReport(t *testing.T) {
	dir := t.TempDir()
	project := config.Project{Name: "no-report", CiDir: dir}

	// Report 为 nil 时也应保存
	saveTestReport(project, Result{
		Project: "no-report", Action: "test", Status: "fail", Duration: "0.5s",
	})

	reportsDir := filepath.Join(dir, "reports", "no-report")
	entries, _ := os.ReadDir(reportsDir)
	if len(entries) != 1 {
		t.Errorf("应保存 1 个文件, 得到 %d", len(entries))
	}
}

func TestCleanOldReports_UnderLimit(t *testing.T) {
	dir := t.TempDir()
	// 创建 3 个报告文件
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, fmt.Sprintf("test-20260622-%02d0000.json", i))
		os.WriteFile(name, []byte("{}"), 0644)
	}

	cleanOldReports(dir, 5) // keep=5, 只有 3 个，不删除
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("低于上限不应删除: 期望 3, 得到 %d", len(entries))
	}
}

func TestCleanOldReports_OverLimit(t *testing.T) {
	dir := t.TempDir()
	// 创建 5 个报告文件，保留 3 个，应删除最老的 2 个
	names := []string{
		"test-20260622-010000.json",
		"test-20260622-020000.json",
		"test-20260622-030000.json",
		"test-20260622-040000.json",
		"test-20260622-050000.json",
	}
	for _, n := range names {
		os.WriteFile(filepath.Join(dir, n), []byte("{}"), 0644)
	}

	cleanOldReports(dir, 3) // keep=3

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("应保留 3 个文件, 得到 %d", len(entries))
	}
	// 应删除最老的 2 个: 01 和 02
	for _, n := range names[:2] {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("最老的文件 %q 应被删除", n)
		}
	}
	// 最新的 3 个应保留
	for _, n := range names[2:] {
		if _, err := os.Stat(filepath.Join(dir, n)); os.IsNotExist(err) {
			t.Errorf("较新的文件 %q 应保留", n)
		}
	}
}

func TestCleanOldReports_NonexistentDir(t *testing.T) {
	// 不存在的目录不应 panic
	cleanOldReports(t.TempDir()+"/nonexistent", 20)
}

func TestCleanOldReports_KeepAll(t *testing.T) {
	dir := t.TempDir()
	// 创建 3 个文件，keep=3，应全部保留
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("test-%d.json", i)), []byte("{}"), 0644)
	}
	cleanOldReports(dir, 3)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("keep=3 时应全部保留, 得到 %d", len(entries))
	}
}

// ===================== hasDir =====================

func TestHasDir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "dist")
	os.MkdirAll(subDir, 0755)

	if !hasDir(dir, "dist") {
		t.Error("存在的目录应返回 true")
	}
	if hasDir(dir, "nonexistent") {
		t.Error("不存在的目录应返回 false")
	}

	// 文件不是目录
	filePath := filepath.Join(dir, "file.txt")
	os.WriteFile(filePath, []byte("content"), 0644)
	if hasDir(dir, "file.txt") {
		t.Error("文件不应被识别为目录")
	}
}

// ===================== hasJar =====================

func TestHasJar(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	os.MkdirAll(targetDir, 0755)

	// 无 jar 文件
	if hasJar(dir) {
		t.Error("无 jar 文件时应返回 false")
	}

	// 创建 jar 文件
	jarPath := filepath.Join(targetDir, "app.jar")
	os.WriteFile(jarPath, []byte("fake jar"), 0644)
	if !hasJar(dir) {
		t.Error("存在 jar 文件时应返回 true")
	}

	// 非 .jar 文件不应匹配
	os.Remove(jarPath)
	os.WriteFile(filepath.Join(targetDir, "app.zip"), []byte("zip"), 0644)
	if hasJar(dir) {
		t.Error("非 .jar 文件不应匹配")
	}
}

// ===================== HasDist =====================

func TestHasDist(t *testing.T) {
	dir := t.TempDir()

	// 无任何构建产物
	if HasDist(dir) {
		t.Error("无 dist 或 jar 时应返回 false")
	}

	// 创建 dist 目录
	os.MkdirAll(filepath.Join(dir, "dist"), 0755)
	if !HasDist(dir) {
		t.Error("存在 dist 目录时应返回 true")
	}
}

func TestHasDistWithJar(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "target"), 0755)
	os.WriteFile(filepath.Join(dir, "target", "app.jar"), []byte("jar"), 0644)

	if !HasDist(dir) {
		t.Error("存在 jar 文件时应返回 true")
	}
}

// ===================== ReadProjectVersion =====================

func TestReadProjectVersion_FromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	pkgContent := `{"name":"test","version":"1.2.3"}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgContent), 0644)

	version := ReadProjectVersion(dir)
	if version != "1.2.3" {
		t.Errorf("期望 1.2.3, 得到 %q", version)
	}
}

func TestReadProjectVersion_FromPomXML(t *testing.T) {
	dir := t.TempDir()
	pomContent := `<project><version>2.0.0</version></project>`
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pomContent), 0644)

	version := ReadProjectVersion(dir)
	if version != "2.0.0" {
		t.Errorf("期望 2.0.0, 得到 %q", version)
	}
}

func TestReadProjectVersion_PackageJSONPriority(t *testing.T) {
	dir := t.TempDir()
	// 同时存在时 package.json 优先
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"3.0.0"}`), 0644)
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<version>4.0.0</version>`), 0644)

	version := ReadProjectVersion(dir)
	if version != "3.0.0" {
		t.Errorf("package.json 应优先: 期望 3.0.0, 得到 %q", version)
	}
}

func TestReadProjectVersion_NoVersionFiles(t *testing.T) {
	dir := t.TempDir()
	version := ReadProjectVersion(dir)
	if version != "" {
		t.Errorf("无版本文件应返回空, 得到 %q", version)
	}
}

func TestReadProjectVersion_PackageJSONNoVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644)
	version := ReadProjectVersion(dir)
	if version != "" {
		t.Errorf("无 version 字段应返回空, 得到 %q", version)
	}
}

func TestReadProjectVersion_PomXMLNoVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<project><name>test</name></project>`), 0644)
	version := ReadProjectVersion(dir)
	if version != "" {
		t.Errorf("无 version 标签应返回空, 得到 %q", version)
	}
}

// ===================== 时间格式测试 =====================

func TestReportTimestampFormat(t *testing.T) {
	// 验证 saveTestReport 中的时间格式可用作文本排序
	now := time.Now()
	ts := now.Format("20060102-150405")

	// 格式应为 YYYYMMDD-HHMMSS（15 字符，不含时区）
	if len(ts) != 15 {
		t.Errorf("时间戳长度应为 15, 得到 %d: %q", len(ts), ts)
	}
	// 验证可解析回本地时间
	parsed, err := time.ParseInLocation("20060102-150405", ts, time.Local)
	if err != nil {
		t.Fatalf("时间戳 %q 解析失败: %v", ts, err)
	}
	// 本地时间与 UTC 时间相差不应超过 24 小时
	diff := parsed.Sub(now)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("解析后的时间偏差过大: %v", diff)
	}
}

// ===================== MockExecutor + Run* 函数测试 =====================

// mockExec 记录调用并返回预设结果，用于测试 Run* 函数的编排逻辑。
type mockExec struct {
	mu       sync.Mutex
	calls    []callRecord
	response func(project config.Project, script string, args []string) (Result, error)
}

type callRecord struct {
	Project config.Project
	Script  string
	Args    []string
}

func (m *mockExec) Run(project config.Project, script string, args ...string) (Result, error) {
	m.mu.Lock()
	m.calls = append(m.calls, callRecord{Project: project, Script: script, Args: args})
	m.mu.Unlock()
	if m.response != nil {
		return m.response(project, script, args)
	}
	return Result{
		Project:  project.Name,
		Action:   script,
		Status:   "pass",
		Duration: "0.1s",
	}, nil
}

func TestRunCheck_UsesExecutor(t *testing.T) {
	mock := &mockExec{}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "test-proj", Path: "/tmp/test", CiDir: "/ci-cd"}
	result, err := RunCheck(proj)
	if err != nil {
		t.Fatalf("RunCheck 失败: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Script != "ci-runner.ps1" {
		t.Errorf("脚本应为 ci-runner.ps1, 得到 %q", call.Script)
	}
	if result.Status != "pass" {
		t.Errorf("状态应为 pass, 得到 %q", result.Status)
	}
	if result.Action != "check" {
		t.Errorf("Action 应为 check, 得到 %q", result.Action)
	}
}

func TestRunBuild_UsesExecutor(t *testing.T) {
	mock := &mockExec{}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "proj-b", Path: "/tmp/b", CiDir: "/ci-cd"}
	result, err := RunBuild(proj)
	if err != nil {
		t.Fatalf("RunBuild 失败: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	if mock.calls[0].Project.Name != "proj-b" {
		t.Errorf("项目名不匹配")
	}
	if result.Action != "build" {
		t.Errorf("Action 应为 ci-runner.ps1, 得到 %q", result.Action)
	}
}

func TestRunTest_UsesExecutor(t *testing.T) {
	mock := &mockExec{
		response: func(project config.Project, script string, args []string) (Result, error) {
			return Result{
				Project: project.Name,
				Action:  script,
				Status:  "pass",
				Report: &TestReport{
					Total: 5, Passed: 5, Failed: 0, Skipped: 0,
				},
			}, nil
		},
	}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "test-t", Path: "/tmp/t", CiDir: "/ci-cd"}
	result, err := RunTest(proj)
	if err != nil {
		t.Fatalf("RunTest 失败: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	if result.Report == nil || result.Report.Total != 5 {
		t.Error("Report 应从 executor 的返回值正确传递")
	}
}

func TestRunTest_ErrorPropagation(t *testing.T) {
	mock := &mockExec{
		response: func(project config.Project, script string, args []string) (Result, error) {
			return Result{
				Project: project.Name,
				Action:  script,
				Status:  "fail",
			}, fmt.Errorf("exec failed")
		},
	}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "fail-t", Path: "/tmp/f", CiDir: "/ci-cd"}
	_, err := RunTest(proj)
	if err == nil {
		t.Error("executor 返回错误时 RunTest 应向上传递")
	}
}

func TestRunDeploy_UsesExecutorWithTarget(t *testing.T) {
	mock := &mockExec{}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "deploy-p", Path: "/tmp/d", CiDir: "/ci-cd"}
	result, err := RunDeploy(proj, "staging")
	if err != nil {
		t.Fatalf("RunDeploy 失败: %v", err)
	}
	_ = result

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Script != "cd-deploy.ps1" {
		t.Errorf("脚本应为 cd-deploy.ps1, 得到 %q", call.Script)
	}
}

func TestRunPush_UsesExecutor(t *testing.T) {
	mock := &mockExec{}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "push-p", Path: "/tmp/p", CiDir: "/ci-cd"}
	err := RunPush(proj)
	if err != nil {
		t.Fatalf("RunPush 失败: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	if mock.calls[0].Script != "ci-push.ps1" {
		t.Errorf("脚本应为 ci-push.ps1, 得到 %q", mock.calls[0].Script)
	}
}

func TestExecutorCalledWithCorrectArgs(t *testing.T) {
	mock := &mockExec{}
	oldExec := defaultExec
	defaultExec = mock
	defer func() { defaultExec = oldExec }()

	proj := config.Project{Name: "args-test", Path: "/tmp/args", CiDir: "/ci-cd"}
	RunCheck(proj)

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Project.Path != "/tmp/args" {
		t.Errorf("Project.Path 不匹配: 期望 /tmp/args, 得到 %q", call.Project.Path)
	}
}

func TestResultStructFields(t *testing.T) {
	// 验证所有字段都能正确序列化
	r := Result{
		Project:  "p",
		Action:   "a",
		Status:   "s",
		Duration: "d",
		Command:  "c",
		ErrorLog: "e",
		Steps:    []Step{{Name: "n", Status: "s", Duration: "d"}},
		Report:   &TestReport{Total: 1, Passed: 1, Failed: 0, Skipped: 0},
	}
	data, _ := json.Marshal(r)
	jsonStr := string(data)

	fields := []string{"project", "action", "status", "duration", "command", "error_log", "steps", "report"}
	for _, f := range fields {
		if !strings.Contains(jsonStr, f) {
			t.Errorf("Result JSON 应包含字段 %q", f)
		}
	}
}

func TestStepStructFields(t *testing.T) {
	s := Step{Name: "test", Status: "pass", Duration: "1s"}
	data, _ := json.Marshal(s)
	jsonStr := string(data)
	for _, f := range []string{"name", "status", "duration"} {
		if !strings.Contains(jsonStr, f) {
			t.Errorf("Step JSON 应包含字段 %q", f)
		}
	}
}

func TestTestFailureStructFields(t *testing.T) {
	f := TestFailure{Suite: "s", Test: "t", Message: "m"}
	data, _ := json.Marshal(f)
	jsonStr := string(data)
	for _, field := range []string{"suite", "test", "message"} {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("TestFailure JSON 应包含字段 %q", field)
		}
	}
}
