package runner

import (
	"testing"
)

// ===================== RunCheckInternal (uses mockExecutor) =====================

func TestRunCheck_ThroughExecutor(t *testing.T) {
	mock := &mockExec{}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("test-check")
	result, err := RunCheck(proj)
	if err != nil {
		t.Fatalf("RunCheck 失败: %v", err)
	}
	if result.Action != "check" {
		t.Errorf("Action 应为 check, 得到 %s", result.Action)
	}
	if len(mock.calls) != 1 {
		t.Errorf("应调用 1 次 executor")
	}
}

func TestRunCheck_ParameterPassthrough(t *testing.T) {
	mock := &mockExec{}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("param-test")
	proj.Path = "/my/test/path"

	RunCheck(proj)

	if len(mock.calls) != 1 {
		t.Fatalf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Project.Path != "/my/test/path" {
		t.Errorf("Path 应透传, 得到 %s", call.Project.Path)
	}
}
