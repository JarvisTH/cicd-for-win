package runner

import (
	"context"
	"testing"
)

// ===================== runCommand (with mock commands) =====================

func TestRunCommand_NotFound(t *testing.T) {
	result := runCommand(context.Background(), ".", "nonexistent-command-xyz", "arg1")
	if result.ExitCode != -1 {
		t.Errorf("不存在的命令 ExitCode 应为 -1, 得到 %d", result.ExitCode)
	}
	if result.Stderr == "" {
		t.Error("不存在的命令应有错误信息")
	}
}

func TestRunCommand_BasicExec(t *testing.T) {
	// 跨平台：使用 go version（所有平台都有）
	result := runCommand(context.Background(), ".", "go", "version")
	if result.ExitCode != 0 {
		t.Errorf("ExitCode 应为 0, 得到 %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if result.Stdout == "" {
		t.Errorf("Stdout 不应为空")
	}
}

// ===================== runGitWithTimeout (basic git) =====================

func TestRunGitWithTimeout_Version(t *testing.T) {
	result := runGitWithTimeout(".", "version")
	if result.ExitCode != 0 {
		t.Skip("git 不可用，跳过")
	}
	if result.Stdout == "" {
		t.Error("git version 应有输出")
	}
}

// ===================== runNpx =====================

func TestRunNpx_NoArgs(t *testing.T) {
	result := runNpxWithTimeout(".")
	if result.ExitCode != 0 {
		// npx 不带参数也可能报错，可以接受
		t.Logf("npx 无参数结果: exit=%d, stderr=%s", result.ExitCode, result.Stderr)
	}
}
