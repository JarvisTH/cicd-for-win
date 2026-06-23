package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestEnv creates a temp ci-cd directory with test fixtures.
// Returns the ciDir path and a cleanup function.
func setupTestEnv(t *testing.T) (ciDir string, cleanup func()) {
	t.Helper()
	ciDir = t.TempDir()

	// Create marker file so findCiDir can find this directory
	marker := filepath.Join(ciDir, "ci.exe")
	os.WriteFile(marker, []byte(""), 0644)

	// Create projects.json
	proj := `{
		"projects": [{
			"name": "test-proj",
			"path": "` + strings.ReplaceAll(ciDir, `\`, `\\`) + `",
			"type": "React",
			"enabled": true,
			"deploy": {"host": "10.0.0.1", "port": 22, "user": "deploy", "remote_dir": "/opt/app", "auth_type": "key", "identity_file": "` + strings.ReplaceAll(filepath.Join(ciDir, "id_rsa"), `\`, `\\`) + `"}
		}]
	}`
	os.WriteFile(filepath.Join(ciDir, "projects.json"), []byte(proj), 0644)

	// Create auth.json
	auth := `{"username":"admin","salt":"test-salt","hash":"test-hash"}`
	os.WriteFile(filepath.Join(ciDir, "auth.json"), []byte(auth), 0644)

	// Create rules dir
	os.MkdirAll(filepath.Join(ciDir, "rules"), 0755)
	os.WriteFile(filepath.Join(ciDir, "rules", "test-rule.mjs"), []byte("export default []"), 0644)

	// Create logs dir with today's log
	logDir := filepath.Join(ciDir, "logs")
	os.MkdirAll(logDir, 0755)
	today := time.Now().Format("2006-01-02")
	os.WriteFile(filepath.Join(logDir, today+".log"), []byte(`{"time":"2026-01-01T00:00:00Z","level":"info","message":"test"}`+"\n"), 0644)
	os.WriteFile(filepath.Join(logDir, "2026-06-20.log"), []byte(`{"time":"2026-06-20T00:00:00Z","level":"warn","message":"old"}`+"\n"), 0644)

	// Init auth
	initAuth(ciDir)

	// Override findCiDir for this test
	origFindCiDir := findCiDir
	findCiDir = func() string { return ciDir }

	cleanup = func() {
		findCiDir = origFindCiDir
	}
	return
}

// ===================== Integration: Rules =====================

func TestIntegration_ViewRuleFile_Valid(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Initiate activeAuth so basicAuth doesn't block
	// (auth was initialized in setupTestEnv)

	// Create a test handler - skip basicAuth for simplicity
	handler := http.HandlerFunc(handleViewRuleFile)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/rules?file=test-rule.mjs")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	_ = ciDir
}

// ===================== Integration: Log Dates =====================

func TestIntegration_LogDates(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := http.HandlerFunc(handleLogDates)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/log/dates")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string][]string
	json.NewDecoder(resp.Body).Decode(&result)
	dates := result["dates"]

	today := time.Now().Format("2006-01-02")
	foundToday := false
	foundOld := false
	for _, d := range dates {
		if d == today {
			foundToday = true
		}
		if d == "2026-06-20" {
			foundOld = true
		}
	}
	if !foundToday {
		t.Errorf("today's date %s should be in dates list", today)
	}
	if !foundOld {
		t.Error("2026-06-20 should be in dates list")
	}

	_ = ciDir
}

// ===================== Integration: Auth Status =====================

func TestIntegration_AuthStatus(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := http.HandlerFunc(authStatusHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/auth/status")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["username"] != "admin" {
		t.Errorf("expected username admin, got %v", result["username"])
	}

	_ = ciDir
}

// ===================== Integration: Log Append =====================

func TestIntegration_LogAppend(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := http.HandlerFunc(handleLogAppend)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	body := `{"level":"info","message":"integration test log","source":"test"}`
	resp, err := http.Post(ts.URL+"/api/log/append", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}

	_ = ciDir
}

// ===================== Integration: Step Status =====================

func TestIntegration_StepStatus(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Pre-populate a step status file
	statusDir := filepath.Join(ciDir, "status", "test-proj")
	os.MkdirAll(statusDir, 0755)
	os.WriteFile(filepath.Join(statusDir, "check.json"), []byte(`{"status":"pass"}`), 0644)

	handler := http.HandlerFunc(stepStatusHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/steps/status")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]map[string]map[string]stepStatusFile
	json.NewDecoder(resp.Body).Decode(&result)

	statuses := result["statuses"]
	if statuses == nil {
		t.Fatal("statuses should not be nil")
	}
	projSteps, ok := statuses["test-proj"]
	if !ok {
		t.Fatal("test-proj should have step status")
	}
	if projSteps["check"].Status != "pass" {
		t.Errorf("check status should be pass, got %s", projSteps["check"].Status)
	}

	_ = ciDir
}

// ===================== Integration: Project List =====================

func TestIntegration_ProjectList(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := http.HandlerFunc(projectListHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string][]map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	projects := result["projects"]
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0]["name"] != "test-proj" {
		t.Errorf("expected project name test-proj, got %v", projects[0]["name"])
	}

	_ = ciDir
}

// ===================== Integration: Report List =====================

func TestIntegration_ReportList(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Pre-populate a report file
	reportDir := filepath.Join(ciDir, "reports", "test-proj")
	os.MkdirAll(reportDir, 0755)
	report := `{"project":"test-proj","status":"pass","report":{"total":5,"passed":5,"failed":0}}`
	os.WriteFile(filepath.Join(reportDir, "test-20260623-120000.json"), []byte(report), 0644)

	handler := http.HandlerFunc(reportListHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/report/list?project=test-proj")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string][]map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	reports := result["reports"]
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	_ = ciDir
}

// ===================== Integration: All Reports =====================

func TestIntegration_AllReports(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	reportDir := filepath.Join(ciDir, "reports", "test-proj")
	os.MkdirAll(reportDir, 0755)
	report := `{"project":"test-proj","status":"pass","report":{"total":3,"passed":3,"failed":0}}`
	os.WriteFile(filepath.Join(reportDir, "test-20260623-120000.json"), []byte(report), 0644)

	handler := http.HandlerFunc(handleAllReports)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/report/all")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string][]map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	reports := result["reports"]
	if len(reports) < 1 {
		t.Fatal("should have at least 1 report")
	}

	_ = ciDir
}

// ===================== Integration: Step Status Clear =====================

func TestIntegration_StepStatusClear(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Pre-populate status
	statusDir := filepath.Join(ciDir, "status", "test-proj")
	os.MkdirAll(statusDir, 0755)
	os.WriteFile(filepath.Join(statusDir, "check.json"), []byte(`{"status":"pass"}`), 0644)

	handler := http.HandlerFunc(stepStatusClearHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Clear status for specific project
	resp, err := http.Post(ts.URL+"/api/steps/status/clear?project=test-proj", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify status is cleared
	statuses := loadStepStatuses(ciDir)
	if _, ok := statuses["test-proj"]; ok {
		t.Error("test-proj status should be cleared")
	}

	_ = ciDir
}

// ===================== Integration: Report Latest =====================

func TestIntegration_ReportLatest(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	reportDir := filepath.Join(ciDir, "reports", "test-proj")
	os.MkdirAll(reportDir, 0755)
	os.WriteFile(filepath.Join(reportDir, "test-20260623-120000.json"), []byte(`{"project":"test-proj","status":"pass","report":{"total":5,"passed":4,"failed":1}}`), 0644)

	handler := http.HandlerFunc(latestReportHandler)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/report/latest?project=test-proj")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["report"] == nil {
		t.Error("should have report data")
	}

	_ = ciDir
}

// ===================== Integration: Remote Servers =====================

func TestIntegration_RemoteServers_Empty(t *testing.T) {
	ciDir, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := http.HandlerFunc(handleRemoteServers)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/remote/servers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string][]map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["servers"] == nil {
		t.Error("servers should not be nil (should be empty array)")
	}

	_ = ciDir
}
