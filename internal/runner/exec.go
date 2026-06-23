// exec.go — 命令执行辅助函数，替代 PowerShell 的 Invoke-CmdSafe。
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ExecResult 保存命令执行的结果。
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandRunner 接口允许在测试中替换命令执行行为。
type CommandRunner interface {
	Run(ctx context.Context, workDir, name string, args ...string) ExecResult
}

// defaultCmdRunner 包级默认执行器，所有 run* 函数通过它执行。测试时可替换为 Mock。
var defaultCmdRunner CommandRunner = &osCommandRunner{}

// osCommandRunner 使用 os/exec 实际执行命令。
type osCommandRunner struct{}

func (r *osCommandRunner) Run(ctx context.Context, workDir, name string, args ...string) ExecResult {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResult{
				ExitCode: -1,
				Stderr:   fmt.Sprintf("执行失败: %v", err),
			}
		}
	}

	return ExecResult{
		ExitCode: exitCode,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
	}
}

// runCommand 执行外部命令，返回结构化的执行结果。
func runCommand(ctx context.Context, workDir, name string, args ...string) ExecResult {
	return defaultCmdRunner.Run(ctx, workDir, name, args...)
}

// runCommandWithTimeout 带超时的命令执行。
func runCommandWithTimeout(workDir, name string, args ...string) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return runCommand(ctx, workDir, name, args...)
}

// runNpx 执行 npx 命令。
func runNpx(ctx context.Context, workDir string, args ...string) ExecResult {
	npxName := "npx"
	if _, err := exec.LookPath("npx"); err != nil {
		npxName = "npx.cmd"
	}
	return runCommand(ctx, workDir, npxName, args...)
}

// runNpxWithTimeout 带超时的 npx 执行。
func runNpxWithTimeout(workDir string, args ...string) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return runNpx(ctx, workDir, args...)
}

// runMvn 执行 Maven 命令。
func runMvn(ctx context.Context, workDir string, args ...string) ExecResult {
	mvnName := "mvn"
	if _, err := exec.LookPath("mvn"); err != nil {
		mvnName = "mvn.cmd"
	}
	return runCommand(ctx, workDir, mvnName, args...)
}

// runNpm 执行 npm 命令。
func runNpm(ctx context.Context, workDir string, args ...string) ExecResult {
	npmName := "npm"
	if _, err := exec.LookPath("npm"); err != nil {
		npmName = "npm.cmd"
	}
	return runCommand(ctx, workDir, npmName, args...)
}

// runGit 执行 git 命令。
func runGit(ctx context.Context, workDir string, args ...string) ExecResult {
	gitName := "git"
	if _, err := exec.LookPath("git"); err != nil {
		gitName = "git.exe"
	}
	return runCommand(ctx, workDir, gitName, args...)
}

// runGitWithTimeout 带超时的 git 执行。
func runGitWithTimeout(workDir string, args ...string) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return runGit(ctx, workDir, args...)
}

// ===================== Mock 辅助（供测试文件使用） =====================

// mockCmdRunner 记录调用并返回预设结果，供测试使用。
type mockCmdRunner struct {
	mu       sync.Mutex
	calls    []cmdCall
	results  map[string]func(args []string) ExecResult
	defaultFn func(name string, args []string) ExecResult
}

type cmdCall struct {
	WorkDir string
	Name    string
	Args    []string
}

func (m *mockCmdRunner) Run(ctx context.Context, workDir, name string, args ...string) ExecResult {
	m.mu.Lock()
	m.calls = append(m.calls, cmdCall{WorkDir: workDir, Name: name, Args: args})
	m.mu.Unlock()

	key := name
	if fn, ok := m.results[key]; ok {
		return fn(args)
	}
	if m.defaultFn != nil {
		return m.defaultFn(name, args)
	}
	return ExecResult{ExitCode: 0, Stdout: "mock ok"}
}

// setCmdMock 替换 defaultCmdRunner 为 mock，返回恢复函数。
func setCmdMock(m *mockCmdRunner) func() {
	old := defaultCmdRunner
	defaultCmdRunner = m
	return func() { defaultCmdRunner = old }
}
