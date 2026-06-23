package runner

import (
	"testing"
)

// ===================== RunCheckInternal (mock 命令执行) =====================

func TestRunCheckInternal_React_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunCheckInternal(t.TempDir(), ProjectTypeReact, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Errorf("应有 2 个步骤 (tsc + eslint), 得到 %d", len(result.Steps))
	}
}

func TestRunCheckInternal_React_TscFails(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				if len(args) >= 1 && args[0] == "tsc" {
					return ExecResult{ExitCode: 1, Stderr: "tsc: error"}
				}
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunCheckInternal(t.TempDir(), ProjectTypeReact, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("tsc 失败时 Status 应为 fail, 得到 %s", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Errorf("应有 2 个步骤, 得到 %d", len(result.Steps))
	}
	if result.Steps[0].Name != "tsc" || result.Steps[0].Status != "fail" {
		t.Errorf("tsc 步骤应为 fail, 得到 status=%s", result.Steps[0].Status)
	}
}

func TestRunCheckInternal_Vue_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunCheckInternal(t.TempDir(), ProjectTypeVue, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}

func TestRunCheckInternal_Vue_EslintDisabled(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				if len(args) >= 1 && args[0] == "eslint" {
					t.Error("eslint 被禁用时不应被调用")
				}
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	ruleStates := map[string]bool{"eslint": false}
	result, err := RunCheckInternal(t.TempDir(), ProjectTypeVue, ruleStates)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Errorf("禁用 eslint 后应有 1 个步骤, 得到 %d", len(result.Steps))
	}
}

func TestRunCheckInternal_Maven_CheckstyleFails(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				for _, a := range args {
					if a == "checkstyle:check" {
						return ExecResult{ExitCode: 1, Stderr: "checkstyle error"}
					}
				}
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunCheckInternal(t.TempDir(), ProjectTypeMaven, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("checkstyle 失败时 Status 应为 fail, 得到 %s", result.Status)
	}
}

func TestRunCheckInternal_MavenMulti_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"mvn": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	result, err := RunCheckInternal(t.TempDir(), ProjectTypeMavenMulti, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}

func TestRunCheckInternal_Unknown_NoSteps(t *testing.T) {
	result, err := RunCheckInternal(t.TempDir(), ProjectTypeUnknown, nil)
	if err != nil {
		t.Fatalf("RunCheckInternal 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("未知类型应返回 pass, 得到 %s", result.Status)
	}
	if len(result.Steps) != 0 {
		t.Errorf("未知类型不应有步骤, 得到 %d", len(result.Steps))
	}
}

// ===================== RunCheck (通过 GoExecutor 公共 API) =====================

func TestRunCheck_GoExecutorRouted(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"npx": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: "OK"}
			},
		},
	}
	defer setCmdMock(mock)()

	g := &GoExecutor{}
	proj := makeProject("check-test")
	result, err := g.Run(proj, "ci-runner.ps1", "-Action", "check", "-ProjectPath", proj.Path)
	if err != nil {
		t.Fatalf("GoExecutor.Run(check) 失败: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("Status 应为 pass, 得到 %s", result.Status)
	}
}
