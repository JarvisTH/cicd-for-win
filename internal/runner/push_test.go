package runner

import (
	"context"
	"testing"

	"ci-cd/internal/config"
)

// ===================== RunPush =====================

func TestRunPush_ThroughExecutor(t *testing.T) {
	mock := &mockExec{}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("push-test")
	err := RunPush(proj)
	if err != nil {
		t.Fatalf("RunPush 失败: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Errorf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
}

func TestRunPush_ErrorPropagation(t *testing.T) {
	mock := &mockExec{
		response: func(project config.Project, script string, args []string) (Result, error) {
			return Result{Status: "fail", ErrorLog: "push failed"}, nil
		},
	}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("push-fail")
	err := RunPush(proj)
	if err == nil {
		t.Error("push 返回 fail 时应返回错误")
	}
}

// ===================== RunPushInternal (git operations mocked) =====================

func TestRunPushInternal_NoRemotes(t *testing.T) {
	proj := config.Project{Name: "no-remote", Path: t.TempDir()}
	err := RunPushInternal(proj)
	if err != nil {
		t.Errorf("无远程仓库时应不报错, 得到: %v", err)
	}
}

// ===================== RunGit (basic smoke test) =====================

func TestRunGit_Smoke(t *testing.T) {
	result := runGit(context.Background(), t.TempDir(), "version")
	if result.ExitCode != 0 {
		t.Skip("git 不可用")
	}
}
