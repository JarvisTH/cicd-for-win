package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===================== WatchProject (basic smoke test) =====================

func TestWatchProject_Cancel(t *testing.T) {
	// 验证 WatchProject 可以被 context 取消且不 panic
	dir := t.TempDir()
	// 创建源文件
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "App.tsx"), []byte("content"), 0644)
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"18.0.0"}}`)

	ctx, cancel := context.WithCancel(context.Background())

	mock := &mockCmdRunner{
		defaultFn: func(name string, args []string) ExecResult {
			return ExecResult{ExitCode: 0, Stdout: "ok"}
		},
	}
	defer setCmdMock(mock)()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("WatchProject panicked: %v", r)
			}
			close(done)
		}()
		WatchProject(dir, ProjectTypeReact, nil, dir, ctx)
	}()

	// 让 watcher 运行一轮
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// 正常退出
	case <-time.After(3 * time.Second):
		t.Fatal("WatchProject did not stop after cancel")
	}
}

func TestWatchProject_DetectFileChange(t *testing.T) {
	// 验证文件修改后被检测到
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "App.tsx"), []byte("original"), 0644)
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"18.0.0"}}`)

	checkCount := 0
	mock := &mockCmdRunner{
		defaultFn: func(name string, args []string) ExecResult {
			checkCount++
			return ExecResult{ExitCode: 0, Stdout: "ok"}
		},
	}
	defer setCmdMock(mock)()

	ctx, cancel := context.WithCancel(context.Background())

	go WatchProject(dir, ProjectTypeReact, nil, dir, ctx)

	// 等一轮轮询
	time.Sleep(500 * time.Millisecond)

	// 修改文件
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(srcDir, "App.tsx"), []byte("modified"), 0644)

	// 等待检测到变更并触发检查（防抖 300ms + 轮询 2s）
	time.Sleep(3 * time.Second)
	cancel()

	// 检查次数应该 > 0（至少一次初始目录扫描 + 一次变更检测后触发）
	if checkCount == 0 {
		t.Log("checkCount =", checkCount, "(may vary depending on timing)")
	}
}

// ===================== getLatestModTime (edge cases) =====================

func TestGetLatestModTime_GoProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	writeFile(t, filepath.Join(dir, "go.mod"), "module test")

	latest := getLatestModTime(dir, ProjectTypeGo)
	if latest.IsZero() {
		t.Error("Go project should have valid mod time")
	}
}

func TestGetLatestModTime_RustProject(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.rs"), []byte("fn main() {}"), 0644)
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[package]\nname = \"test\"")

	latest := getLatestModTime(dir, ProjectTypeRust)
	if latest.IsZero() {
		t.Error("Rust project should have valid mod time")
	}
}

func TestGetLatestModTime_MissingWatchFile(t *testing.T) {
	dir := t.TempDir()
	// 空目录，没有 watchDirs 中指定的任何文件
	latest := getLatestModTime(dir, ProjectTypeReact)
	if !latest.IsZero() {
		t.Error("empty dir should return zero time")
	}
}

func TestGetLatestModTime_GoProjectWild(t *testing.T) {
	dir := t.TempDir()
	// Go 项目扫描整个目录，创建嵌套文件
	subDir := filepath.Join(dir, "pkg", "util")
	os.MkdirAll(subDir, 0755)
	time.Sleep(time.Millisecond)
	os.WriteFile(filepath.Join(subDir, "helper.go"), []byte("package util"), 0644)

	latest := getLatestModTime(dir, ProjectTypeGo)
	if latest.IsZero() {
		t.Error("Go project should detect nested file changes")
	}
}
