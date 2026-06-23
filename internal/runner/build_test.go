package runner

import (
	"strings"
	"testing"
	"time"
)

// ===================== RunBuildInternal (mock 命令执行) =====================

func TestRunBuildInternal_Frontend_Success(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npm": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "build ok"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Errorf("应有 1 个步骤, 得到 %d", len(result.Steps))
	}
	if result.Steps[0].Name != "build" || result.Steps[0].Status != "pass" {
		t.Errorf("build 步骤应为 pass, 得到 %s", result.Steps[0].Status)
	}
}

func TestRunBuildInternal_Frontend_Fail(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npm": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stderr: "npm ERR! build failed"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("构建失败时 Status 应为 fail, 得到 %s", result.Status)
	}
}

func TestRunBuildInternal_Maven_Success(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "BUILD SUCCESS"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeMaven)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}

func TestRunBuildInternal_Maven_Fail(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stderr: "BUILD FAILURE"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeMaven)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("Status 应为 fail, 得到 %s", result.Status)
	}
}

func TestRunBuildInternal_MavenMulti_Success(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "BUILD SUCCESS"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeMavenMulti)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}

func TestRunBuildInternal_Unknown_Skip(t *testing.T) {
	result, err := RunBuildInternal(t.TempDir(), ProjectTypeUnknown)
	if err != nil {
		t.Fatalf("RunBuildInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("未知类型应返回 pass, 得到 %s", result.Status)
	}
}

// ===================== containsAny =====================

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s       string
		targets []string
		want    bool
	}{
		{"hello-world", []string{"hello"}, true},
		{"sources.jar", []string{"sources", "javadoc"}, true},
		{"app.jar", []string{"sources", "javadoc"}, false},
		{"", []string{"a"}, false},
		{"abc", nil, false},
	}
	for _, tc := range tests {
		got := containsAny(tc.s, tc.targets...)
		if got != tc.want {
			t.Errorf("containsAny(%q, %v) = %v, 期望 %v", tc.s, tc.targets, got, tc.want)
		}
	}
}

// ===================== makeBuildStep =====================

func TestMakeBuildStep_Success(t *testing.T) {
	r := ExecResult{ExitCode: 0, Stdout: "build ok"}
	step := makeBuildStep("build", time.Now(), r)
	if step.Status != "pass" {
		t.Errorf("status should be pass, got %s", step.Status)
	}
	if step.ErrorLog != "" {
		t.Errorf("success should have no error log, got %s", step.ErrorLog)
	}
}

func TestMakeBuildStep_FailWithStderr(t *testing.T) {
	r := ExecResult{ExitCode: 1, Stderr: "npm ERR! build failed"}
	step := makeBuildStep("build", time.Now(), r)
	if step.Status != "fail" {
		t.Errorf("status should be fail, got %s", step.Status)
	}
	if !strings.Contains(step.ErrorLog, "npm ERR!") {
		t.Errorf("should capture stderr, got %s", step.ErrorLog)
	}
}

func TestMakeBuildStep_FailWithStdoutFallback(t *testing.T) {
	r := ExecResult{ExitCode: 1, Stdout: "BUILD FAILURE", Stderr: ""}
	step := makeBuildStep("build", time.Now(), r)
	if step.Status != "fail" {
		t.Errorf("status should be fail, got %s", step.Status)
	}
	if !strings.Contains(step.ErrorLog, "BUILD FAILURE") {
		t.Errorf("should fallback to stdout, got %s", step.ErrorLog)
	}
}

func TestMakeBuildStep_TruncatesLongError(t *testing.T) {
	long := string(make([]byte, 3000))
	r := ExecResult{ExitCode: 1, Stderr: long}
	step := makeBuildStep("build", time.Now(), r)
	if len(step.ErrorLog) > 2100 {
		t.Errorf("long error should be truncated, length %d", len(step.ErrorLog))
	}
}

func TestMakeBuildStep_Duration(t *testing.T) {
	start := time.Now()
	time.Sleep(100 * time.Millisecond)
	r := ExecResult{ExitCode: 0}
	step := makeBuildStep("build", start, r)
	if step.Duration == "" || step.Duration == "0.0s" {
		t.Errorf("duration should be positive, got %s", step.Duration)
	}
}

// ===================== RunBuildInternal ErrorLog capture =====================

func TestRunBuildInternal_Frontend_FailCapturesError(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npm": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stderr: "npm ERR! build script failed\nError: command not found"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeReact)
	if err != nil {
		t.Fatalf("RunBuildInternal failed: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("status should be fail, got %s", result.Status)
	}
	if result.ErrorLog == "" {
		t.Error("ErrorLog should not be empty when build fails")
	}
	if !strings.Contains(result.ErrorLog, "npm ERR!") {
		t.Errorf("ErrorLog should contain command output, got %s", result.ErrorLog)
	}
}

func TestRunBuildInternal_Maven_FailCapturesError(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stderr: "BUILD FAILURE\nCompilation error"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunBuildInternal(t.TempDir(), ProjectTypeMaven)
	if err != nil {
		t.Fatalf("RunBuildInternal failed: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("status should be fail, got %s", result.Status)
	}
	if !strings.Contains(result.ErrorLog, "BUILD FAILURE") {
		t.Errorf("ErrorLog should contain maven output, got %s", result.ErrorLog)
	}
}

// ===================== RunBuild 公共 API =====================

func TestRunBuild_ThroughExecutor(t *testing.T) {
	mock := &mockExec{}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("test-build")
	result, err := RunBuild(proj)
	if err != nil {
		t.Fatalf("RunBuild 失败: %v", err)
	}
	if result.Action != "build" {
		t.Errorf("Action 应为 build, 得到 %s", result.Action)
	}
}
