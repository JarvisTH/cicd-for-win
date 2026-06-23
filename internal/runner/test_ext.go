// test_ext.go — 扩展语言测试报告解析（Go、Python pytest）
package runner

import (
	"encoding/json"
	"strings"
	"fmt"
	"time"
)

// ===================== Go test JSON 解析 =====================

// goTestJSON 映射 go test -json 的单行输出。
type goTestJSON struct {
	Time    string `json:"Time"`
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// runGoTest 执行 go test 并解析 JSON 输出。
func runGoTest(projectPath string, start time.Time) (Result, *TestReport, error) {
	fmt.Fprintf(logWriter, "[Go] 开始 go test...\n")
	result := runCommandWithTimeout(projectPath, "go", "test", "./...", "-json")
	output := result.Stdout + "\n" + result.Stderr

	report := TestReport{RawLog: strings.TrimSpace(output)}
	var currentPackage string
	var currentTest string

	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		var event goTestJSON
		if err := json.Unmarshal([]byte(line), &event); err != nil { continue }

		switch event.Action {
		case "pass":
			if event.Test != "" { report.Passed++; report.Total++ }
		case "fail":
			if event.Test != "" { report.Failed++; report.Total++
				report.Failures = append(report.Failures, TestFailure{
					Suite: event.Package, Test: event.Test, Message: "test failed",
				})
			}
		case "skip":
			if event.Test != "" { report.Skipped++ }
		}
		_ = currentPackage
		_ = currentTest
	}
	if report.Total == 0 {
		report.Total = report.Passed + report.Failed + report.Skipped
	}

	status := "pass"
	if result.ExitCode != 0 && report.Failed > 0 { status = "fail" }

	return Result{Status: status, Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds())}, &report, nil
}

// ===================== pytest JSON 解析 =====================

// pytestJSON 映射 pytest --json-report 的输出结构。
type pytestJSON struct {
	Created  float64 `json:"created"`
	Duration float64 `json:"duration"`
	Exitcode int     `json:"exitcode"`
	Root     string  `json:"root"`
	Tests    []struct {
		NodeID string `json:"nodeid"`
		Outcome string `json:"outcome"`
		Call    struct {
			Longrepr string `json:"longrepr"`
		} `json:"call"`
	} `json:"tests"`
	Summary struct {
		Passed int `json:"passed"`
		Failed int `json:"failed"`
		Skipped int `json:"skipped"`
		Total   int `json:"total"`
	} `json:"summary"`
}

// runPythonTest 执行 pytest 并解析 JSON 报告。
func runPythonTest(projectPath string, start time.Time) (Result, *TestReport, error) {
	fmt.Fprintf(logWriter, "[Python] 开始 pytest...\n")
	result := runCommandWithTimeout(projectPath, "python", "-m", "pytest", "--json-report", "--json-report-file=-")
	output := result.Stdout + "\n" + result.Stderr

	var report TestReport
	report.RawLog = strings.TrimSpace(output)

	// 尝试解析 JSON 报告（pytest 输出到最后）
	var py pytestJSON
	lines := strings.Split(result.Stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if json.Unmarshal([]byte(lines[i]), &py) == nil && py.Summary.Total > 0 {
			report.Total = py.Summary.Total
			report.Passed = py.Summary.Passed
			report.Failed = py.Summary.Failed
			report.Skipped = py.Summary.Skipped
			for _, t := range py.Tests {
				if t.Outcome == "failed" {
					suite := t.NodeID
					if idx := strings.LastIndex(t.NodeID, "::"); idx >= 0 {
						suite = t.NodeID[:idx]
					}
					report.Failures = append(report.Failures, TestFailure{
						Suite: suite, Test: t.NodeID, Message: t.Call.Longrepr,
					})
				}
			}
			break
		}
	}

	status := "pass"
	if py.Exitcode != 0 { status = "fail" }
	return Result{Status: status, Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds())}, &report, nil
}
