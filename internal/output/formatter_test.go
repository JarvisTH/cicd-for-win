package output

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"ci-cd/internal/runner"
)

// captureStdout 捕获函数执行期间的 stdout 输出
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = old
	return string(out)
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{Use: "test"}
}

func TestFormat_Text_Pass(t *testing.T) {
	results := []runner.Result{
		{
			Project:  "proj-a",
			Action:   "check",
			Status:   "pass",
			Duration: "2.5s",
			Steps: []runner.Step{
				{Name: "tsc", Status: "pass", Duration: "1.0s"},
				{Name: "eslint", Status: "pass", Duration: "1.5s"},
			},
		},
	}

	output := captureStdout(func() {
		Format(newTestCmd(), results, false)
	})

	if !strings.Contains(output, "✅") {
		t.Error("通过的步骤应显示 ✅")
	}
	if !strings.Contains(output, "proj-a") {
		t.Error("输出应包含项目名")
	}
	if !strings.Contains(output, "2.5s") {
		t.Error("输出应包含总耗时")
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Errorf("应有至少 3 行输出（标题+2 步骤）, 得到 %d", len(lines))
	}
}

func TestFormat_Text_Fail(t *testing.T) {
	results := []runner.Result{
		{
			Project:  "proj-b",
			Action:   "build",
			Status:   "fail",
			Duration: "0.5s",
			Steps: []runner.Step{
				{Name: "npm build", Status: "fail", Duration: "0.5s"},
			},
		},
	}

	output := captureStdout(func() {
		Format(newTestCmd(), results, false)
	})

	if !strings.Contains(output, "❌") {
		t.Error("失败的步骤应显示 ❌")
	}
	if !strings.Contains(output, "proj-b") {
		t.Error("输出应包含项目名")
	}
}

func TestFormat_Text_MixedResults(t *testing.T) {
	results := []runner.Result{
		{Project: "pass-proj", Action: "test", Status: "pass", Duration: "1.0s"},
		{Project: "fail-proj", Action: "test", Status: "fail", Duration: "0.3s"},
	}

	output := captureStdout(func() {
		Format(newTestCmd(), results, false)
	})

	if !strings.Contains(output, "[pass-proj] ✅") {
		t.Errorf("pass-proj 应显示 ✅, 输出: %q", output)
	}
	if !strings.Contains(output, "[fail-proj] ❌") {
		t.Errorf("fail-proj 应显示 ❌, 输出: %q", output)
	}
}

func TestFormat_Text_EmptyResults(t *testing.T) {
	output := captureStdout(func() {
		Format(newTestCmd(), []runner.Result{}, false)
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("空结果应无输出, 得到 %q", output)
	}
}

func TestFormat_Text_NoSteps(t *testing.T) {
	results := []runner.Result{
		{Project: "simple", Action: "push", Status: "pass", Duration: "0.1s"},
	}
	output := captureStdout(func() {
		Format(newTestCmd(), results, false)
	})
	if !strings.Contains(output, "simple") {
		t.Error("无 Steps 的结果也应输出项目名")
	}
}

func TestFormat_JSON(t *testing.T) {
	results := []runner.Result{
		{
			Project:  "json-proj",
			Action:   "check",
			Status:   "pass",
			Duration: "1.2s",
			Steps: []runner.Step{
				{Name: "lint", Status: "pass", Duration: "0.8s"},
			},
		},
	}

	output := captureStdout(func() {
		Format(newTestCmd(), results, true)
	})

	if !strings.Contains(output, `"project": "json-proj"`) {
		t.Error("JSON 输出应包含项目名")
	}
	if !strings.Contains(output, `"action": "check"`) {
		t.Error("JSON 输出应包含 action")
	}
	if !strings.Contains(output, `"status": "pass"`) {
		t.Error("JSON 输出应包含 status")
	}
	if !strings.Contains(output, `"steps"`) {
		t.Error("JSON 输出应包含 steps 数组")
	}
	// 应为合法 JSON 数组
	if output[0] != '[' {
		t.Errorf("JSON 输出应以 [ 开头（数组）, 实际: %c", output[0])
	}
}

func TestFormat_JSON_Empty(t *testing.T) {
	output := captureStdout(func() {
		Format(newTestCmd(), []runner.Result{}, true)
	})
	// 空数组的 JSON
	if !strings.Contains(output, "[]") {
		t.Errorf("空结果 JSON 应输出 [], 得到 %q", output)
	}
}

// TestFormat_ErrorLog 验证带有 ErrorLog 的结果在 Text 模式下正常显示
func TestFormat_ErrorLog(t *testing.T) {
	results := []runner.Result{
		{
			Project:  "err-proj",
			Action:   "build",
			Status:   "fail",
			Duration: "0.5s",
			ErrorLog: "error: command not found",
		},
	}
	// 不应 panic
	output := captureStdout(func() {
		Format(newTestCmd(), results, false)
	})
	if !strings.Contains(output, "❌") {
		t.Error("应有失败标记")
	}
	_ = output
}

func TestFormat_NilResults(t *testing.T) {
	// 传递 nil 不应 panic
	output := captureStdout(func() {
		Format(newTestCmd(), nil, false)
	})
	if strings.TrimSpace(output) != "" {
		t.Errorf("nil 结果应无输出, 得到 %q", output)
	}

	outputJSON := captureStdout(func() {
		Format(newTestCmd(), nil, true)
	})
	if !strings.Contains(outputJSON, "null") {
		t.Errorf("nil 结果 JSON 应输出 null, 得到 %q", outputJSON)
	}
}

func TestFormat_JSONValidSyntax(t *testing.T) {
	results := []runner.Result{
		{
			Project: "v", Action: "t", Status: "pass", Duration: "0s",
			Report: &runner.TestReport{
				Total: 5, Passed: 5, Failed: 0, Skipped: 0,
				Failures: []runner.TestFailure{
					{Suite: "suite1", Test: "test1", Message: "msg"},
				},
			},
		},
	}
	// 不应 panic
	output := captureStdout(func() {
		Format(newTestCmd(), results, true)
	})
	if !strings.Contains(output, `"report"`) {
		t.Error("JSON 应包含 report 字段")
	}
	_ = output
}
