// exec.go — 命令执行辅助函数，替代 PowerShell 的 Invoke-CmdSafe。
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecResult 保存命令执行的结果。
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// runCommand 执行外部命令，返回结构化的执行结果。
// ctx 用于超时和取消；workDir 指定工作目录；name 为命令名；args 为参数列表。
// 对应 ci-runner.ps1 的 Invoke-CmdSafe 函数逻辑。
func runCommand(ctx context.Context, workDir, name string, args ...string) ExecResult {
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
			// 命令未找到、超时等错误
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

// runCommandWithTimeout 带超时的命令执行，超时自动取消。
func runCommandWithTimeout(workDir, name string, args ...string) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return runCommand(ctx, workDir, name, args...)
}

// runNpx 执行 npx 命令（兼容 Windows npx.cmd）。
func runNpx(ctx context.Context, workDir string, args ...string) ExecResult {
	// Windows 上 npx 可能名为 npx.cmd
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

// runMvn 执行 Maven 命令（兼容 Windows mvn.cmd）。
func runMvn(ctx context.Context, workDir string, args ...string) ExecResult {
	mvnName := "mvn"
	if _, err := exec.LookPath("mvn"); err != nil {
		mvnName = "mvn.cmd"
	}
	return runCommand(ctx, workDir, mvnName, args...)
}

// runNpm 执行 npm 命令（兼容 Windows npm.cmd）。
func runNpm(ctx context.Context, workDir string, args ...string) ExecResult {
	npmName := "npm"
	if _, err := exec.LookPath("npm"); err != nil {
		npmName = "npm.cmd"
	}
	return runCommand(ctx, workDir, npmName, args...)
}

// runGit 执行 git 命令（兼容 Windows git.exe）。
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
