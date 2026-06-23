package runner

import (
	"testing"
	"time"
)

// ===================== runGoTest JSON parsing =====================

func TestParseGoTestJSON(t *testing.T) {
	// Simulate go test -json output
	output := `{"Time":"2026-06-23T12:00:00Z","Action":"pass","Package":"mypkg","Test":"TestFoo","Elapsed":0.01}
{"Time":"2026-06-23T12:00:00Z","Action":"pass","Package":"mypkg","Test":"TestBar","Elapsed":0.02}
{"Time":"2026-06-23T12:00:00Z","Action":"fail","Package":"mypkg","Test":"TestBaz","Elapsed":0.03}
{"Time":"2026-06-23T12:00:00Z","Action":"skip","Package":"mypkg","Test":"TestSkip","Elapsed":0}
{"Time":"2026-06-23T12:00:00Z","Action":"output","Package":"mypkg","Test":"TestBaz","Output":"FAIL: TestBaz"}
{"Time":"2026-06-23T12:00:00Z","Action":"fail","Package":"mypkg","Elapsed":0.5}`

	// Write output to a temp file
	dir := t.TempDir()
	writeFile(t, dir+"/go.mod", "module test\n")
	writeFile(t, dir+"/main_test.go", `package main; import "testing"; func TestFoo(t *testing.T){}; func TestBar(t *testing.T){}; func TestBaz(t *testing.T){}; func TestSkip(t *testing.T){};`)

	// Test with mock command runner
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"go": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stdout: output, Stderr: ""}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := runGoTest(dir, time.Now())
	if err != nil {
		t.Fatalf("runGoTest failed: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("expected fail status, got %s", result.Status)
	}
	if report.Total != 4 {
		t.Errorf("expected 4 tests total, got %d", report.Total)
	}
	if report.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", report.Failed)
	}
	if report.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", report.Skipped)
	}
}

func TestRunGoTest_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"go": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"Action":"pass","Package":"pkg","Test":"TestA","Elapsed":0.01}` + "\n" + `{"Action":"pass","Package":"pkg","Test":"TestB","Elapsed":0.02}`, Stderr: ""}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := runGoTest(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("runGoTest failed: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("expected pass status, got %s", result.Status)
	}
	if report.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", report.Passed)
	}
}

// ===================== runPythonTest (pytest) JSON parsing =====================

func TestParsePytestJSON(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"python": func(args []string) ExecResult {
				return ExecResult{ExitCode: 1, Stdout: `{"created":123,"duration":0.5,"exitcode":1,"root":"/tmp","tests":[{"nodeid":"test_foo.py::test_pass","outcome":"passed","call":{"longrepr":""}},{"nodeid":"test_bar.py::test_fail","outcome":"failed","call":{"longrepr":"AssertionError: expected True"}}],"summary":{"passed":1,"failed":1,"skipped":0,"total":2}}`, Stderr: ""}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := runPythonTest(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("runPythonTest failed: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("expected fail status, got %s", result.Status)
	}
	if report.Total != 2 {
		t.Errorf("expected 2 tests, got %d", report.Total)
	}
	if report.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", report.Failed)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(report.Failures))
	}
	if report.Failures[0].Test != "test_bar.py::test_fail" {
		t.Errorf("expected failure test name, got %s", report.Failures[0].Test)
	}
}

func TestRunPythonTest_AllPass(t *testing.T) {
	mock := &mockCmdRunner{
		results: map[string]func([]string) ExecResult{
			"python": func(args []string) ExecResult {
				return ExecResult{ExitCode: 0, Stdout: `{"created":123,"duration":0.3,"exitcode":0,"root":"/tmp","tests":[{"nodeid":"test_a.py::test1","outcome":"passed","call":{"longrepr":""}}],"summary":{"passed":1,"failed":0,"skipped":0,"total":1}}`, Stderr: ""}
			},
		},
	}
	defer setCmdMock(mock)()

	result, report, err := runPythonTest(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("runPythonTest failed: %v", err)
	}
	if result.Status != "pass" {
		t.Errorf("expected pass status, got %s", result.Status)
	}
	if report.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", report.Passed)
	}
}

// ===================== notify (basic coverage) =====================

func TestNotify_NoPanic(t *testing.T) {
	// Verify notify functions don't panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("notify should not panic: %v", r)
		}
	}()
	Notify("test title", "test message")
	NotifyPass("test-proj", "check", "1.0s")
	NotifyFail("test-proj", "build")
}

func TestEscapePS(t *testing.T) {
	result := escapePS("it's a test")
	if result != "it''s a test" {
		t.Errorf("expected it''s a test, got %s", result)
	}
}
