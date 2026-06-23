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

func TestRunCommand_WithWorkDir(t *testing.T) {
	// 使用 cmd /c echo 测试基本执行
	result := runCommand(context.Background(), ".", "cmd.exe", "/c", "echo hello")
	if result.ExitCode != 0 {
		t.Errorf("ExitCode 应为 0, 得到 %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "hello" {
		t.Errorf("Stdout 应为 'hello', 得到 %q", result.Stdout)
	}
}

func TestRunCommand_ExitCode(t *testing.T) {
	result := runCommand(context.Background(), ".", "cmd.exe", "/c", "exit 42")
	if result.ExitCode != 42 {
		t.Errorf("ExitCode 应为 42, 得到 %d", result.ExitCode)
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
