package runner

import (
	"os"
	"path/filepath"
	"testing"

	"ci-cd/internal/config"
)

// ===================== RunTestInternal (mock 命令执行) =====================

func TestRunTestInternal_React_Vitest_AllPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"vitest":"1.0.0"}}`)
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"numTotalTestSuites":1,"numTotalTests":3,"numPassedTests":3,"numFailedTests":0,"numPendingTests":0,"testResults":[{"name":"test.js","assertionResults":[{"fullName":"test1","status":"passed"}]}]}`}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(dir, ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if report == nil {
		t.Fatal("report 不应为 nil")
	}
	if report.Total != 3 {
		t.Errorf("Total 应为 3, 得到 %d", report.Total)
	}
	if report.Passed != 3 {
		t.Errorf("Passed 应为 3, 得到 %d", report.Passed)
	}
}

func TestRunTestInternal_React_Vitest_WithFailures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"vitest":"1.0.0"}}`)
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stdout: `{"numTotalTestSuites":1,"numTotalTests":2,"numPassedTests":1,"numFailedTests":1,"numPendingTests":0,"testResults":[{"name":"test.js","assertionResults":[{"fullName":"pass-test","status":"passed"},{"fullName":"fail-test","status":"failed","failureMessages":["assert fail"]}]}]}`}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(dir, ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("有失败用例时 Status 应为 fail, 得到 %s", result.Status)
	}
	if report.Failed != 1 {
		t.Errorf("Failed 应为 1, 得到 %d", report.Failed)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("应有 1 个失败用例, 得到 %d", len(report.Failures))
	}
}

func TestRunTestInternal_Vue_Vitest_AllPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"vitest":"1.0.0"}}`)
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"numTotalTestSuites":1,"numTotalTests":1,"numPassedTests":1,"numFailedTests":0,"numPendingTests":0,"testResults":[]}`}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(dir, ProjectTypeVue)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if report.Passed != 1 {
		t.Errorf("Passed 应为 1, 得到 %d", report.Passed)
	}
}

func TestRunTestInternal_Maven_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "Tests run: 5"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(t.TempDir(), ProjectTypeMaven)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if report == nil {
		t.Fatal("report 不应为 nil")
	}
}

func TestRunTestInternal_Maven_Fail(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stderr: "Tests failed"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(t.TempDir(), ProjectTypeMaven)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("测试失败时 Status 应为 fail, 得到 %s", result.Status)
	}
	_ = report
}

func TestRunTestInternal_MavenMulti_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "Tests run: 10"}
			},
		},
	}
	defer setCmdMock(mock)()

	dir := t.TempDir()
	// 创建一个有 pom.xml 的子模块
	modDir := filepath.Join(dir, "module-a")
	os.MkdirAll(modDir, 0755)
	writeFile(t, filepath.Join(modDir, "pom.xml"), `<project><version>1.0</version></project>`)

	result, report, err := RunTestInternal(dir, ProjectTypeMavenMulti)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	_ = report
}

func TestRunTestInternal_Unknown_Skip(t *testing.T) {
	result, report, err := RunTestInternal(t.TempDir(), ProjectTypeUnknown)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "skip" {
		t.Errorf("未知类型 Status 应为 skip, 得到 %s", result.Status)
	}
	if report != nil {
		t.Error("未知类型 report 应为 nil")
	}
}

func TestRunTestInternal_React_Jest_AllPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"jest":"29.0.0"}}`)
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"numTotalTestSuites":1,"numTotalTests":4,"numPassedTests":4,"numFailedTests":0,"numPendingTests":0,"testResults":[{"name":"test.js","assertionResults":[{"fullName":"all pass","status":"passed"}]}]}`}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := RunTestInternal(dir, ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunTestInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if report.Passed != 4 {
		t.Errorf("Passed 应为 4, 得到 %d", report.Passed)
	}
}

// ===================== RunTest 公共 API =====================

func TestRunTest_ThroughGoExecutor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"18.0.0"}}`)
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"numTotalTestSuites":0,"numTotalTests":0,"numPassedTests":0,"numFailedTests":0,"numPendingTests":0,"testResults":[]}`}
			},
		},
	}
	defer setCmdMock(mock)()

	g := &GoExecutor{}
	proj := makeProject("go-test")
	proj.Path = dir
	result, err := g.Run(proj, "ci-runner.ps1", "-Action", "test", "-ProjectPath", dir)
	if err != nil {
		t.Fatalf("GoExecutor.Run(test) 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}

// ===================== RunPushInternal (mock 命令) =====================

func TestRunPushInternal_WithRemotes_Success(t *testing.T) {
	pushCallCount := 0
	mock := &mockCmdRunner{
		defaultFn: func(name string, args []string) ExecResult {
			if len(args) >= 1 && args[0] == "push" {
				pushCallCount++
				return ExecResult{ExitCode: 0, Stdout: "push ok"}
			}
			if len(args) >= 1 && args[0] == "remote" {
				return ExecResult{ExitCode: 0, Stdout: "origin\tgit@github.com:test/repo.git (fetch)"}
			}
			if len(args) >= 1 && args[0] == "rev-parse" {
				return ExecResult{ExitCode: 0, Stdout: "main"}
			}
			return ExecResult{ExitCode: 0, Stdout: "ok"}
		},
	}
	defer setCmdMock(mock)()

	proj := makeProject("push-with-remote")
	proj.Path = t.TempDir()
	proj.Remotes = []config.RemoteConfig{
		{Name: "origin", URL: "git@github.com:test/repo.git", Enabled: true},
	}
	proj.GitBranch = "main"

	err := RunPushInternal(proj)
	if err != nil {
		t.Errorf("推送应成功, 得到: %v", err)
	}
	if pushCallCount != 1 {
		t.Errorf("push 应被调用 1 次, 得到 %d", pushCallCount)
	}
}

func TestRunPushInternal_PushFails(t *testing.T) {
	mock := &mockCmdRunner{
		defaultFn: func(name string, args []string) ExecResult {
			if len(args) >= 1 && args[0] == "push" {
				return ExecResult{ExitCode: 1, Stderr: "push rejected"}
			}
			if len(args) >= 1 && args[0] == "remote" {
				return ExecResult{ExitCode: 0, Stdout: "origin\tgit@github.com:test/repo.git (fetch)"}
			}
			if len(args) >= 1 && args[0] == "rev-parse" {
				return ExecResult{ExitCode: 0, Stdout: "main"}
			}
			return ExecResult{ExitCode: 0, Stdout: "ok"}
		},
	}
	defer setCmdMock(mock)()

	proj := makeProject("push-fail")
	proj.Path = t.TempDir()
	proj.Remotes = []config.RemoteConfig{
		{Name: "origin", URL: "git@github.com:test/repo.git", Enabled: true},
	}
	proj.GitBranch = "main"

	err := RunPushInternal(proj)
	if err == nil {
		t.Error("push 失败时应返回错误")
	}
}
