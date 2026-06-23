package runner

import (
	"testing"
)

// ===================== RunBuildInternal (uses MockExecutor) =====================

// TestRunBuildInternal_UsesExec 验证 RunBuildInternal 通过执行外部命令完成任务。
// 由于它内部调用 runNpm/runMvn 等实际子进程，此处通过已有的 MockExecutor
// 验证公共 API RunBuild 能正确路由到 GoExecutor。

func TestRunBuildInternal_ContainsAny(t *testing.T) {
	if !containsAny("app-1.0-sources.jar", "sources", "javadoc") {
		t.Error("sources.jar 应被识别")
	}
	if !containsAny("app-1.0-javadoc.jar", "sources", "javadoc") {
		t.Error("javadoc.jar 应被识别")
	}
	if containsAny("app-1.0.jar", "sources", "javadoc") {
		t.Error("普通 jar 不应被识别为 sources 或 javadoc")
	}
}

// TestRunBuild_ThroughExecutor 验证 RunBuild 公共 API 通过 Executor 路由。
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
	if len(mock.calls) != 1 {
		t.Errorf("应调用 1 次 executor")
	}
}
