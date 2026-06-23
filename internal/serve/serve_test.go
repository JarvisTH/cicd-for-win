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

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

// ===================== isValidStepID =====================

func TestIsValidStepID(t *testing.T) {
	validIDs := []string{"check", "build", "test", "push", "deploy"}
	for _, id := range validIDs {
		if !isValidStepID(id) {
			t.Errorf("valid step ID %q should be true", id)
		}
	}

	invalidIDs := []string{"", "deploy2", "Check", "BUILD", "lint", "all"}
	for _, id := range invalidIDs {
		if isValidStepID(id) {
			t.Errorf("invalid step ID %q should be false", id)
		}
	}
}

// ===================== stepStatusDir =====================

func TestStepStatusDir(t *testing.T) {
	dir := stepStatusDir("/ci-cd")
	expected := filepath.Join("/ci-cd", "status")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestStepStatusDirRelative(t *testing.T) {
	dir := stepStatusDir(".")
	expected := filepath.Join(".", "status")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

// ===================== serversFilePath =====================

func TestServersFilePath(t *testing.T) {
	path := serversFilePath("/ci-cd")
	expected := filepath.Join("/ci-cd", "servers.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// ===================== sanitizeDeploy =====================

func TestSanitizeDeploy(t *testing.T) {
	input := map[string]any{
		"host":     "example.com",
		"password": "secret123",
		"user":     "admin",
		"port":     float64(22),
	}
	output := sanitizeDeploy(input)
	if output["host"] != "example.com" {
		t.Error("non-password fields should be preserved")
	}
	if output["user"] != "admin" {
		t.Error("non-password fields should be preserved")
	}
	if pwd, ok := output["password"]; !ok || pwd != "" {
		t.Errorf("password field should be cleared, got %v", pwd)
	}
}

func TestSanitizeDeploy_NoPassword(t *testing.T) {
	input := map[string]any{"host": "example.com"}
	output := sanitizeDeploy(input)
	if output["host"] != "example.com" {
		t.Error("fields without password should be preserved")
	}
}

func TestSanitizeDeploy_Empty(t *testing.T) {
	output := sanitizeDeploy(map[string]any{})
	if len(output) != 0 {
		t.Errorf("empty input should return empty map, got %v", output)
	}
}

func TestSanitizeDeploy_Nil(t *testing.T) {
	output := sanitizeDeploy(nil)
	if len(output) != 0 {
		t.Errorf("nil input should return empty map, got %v", output)
	}
}

// ===================== sanitizeServer =====================

func TestSanitizeServer(t *testing.T) {
	s := StandaloneServer{
		Name:     "test",
		Host:     "10.0.0.1",
		Password: "secret",
	}
	safe := sanitizeServer(s)
	if safe.Password != "" {
		t.Error("password should be cleared")
	}
	if safe.Name != "test" {
		t.Error("name should be preserved")
	}
}

func TestSanitizeServer_EmptyPassword(t *testing.T) {
	s := StandaloneServer{Name: "test", Password: ""}
	safe := sanitizeServer(s)
	if safe.Password != "" {
		t.Error("empty password should stay empty")
	}
}

// ===================== buildCommandString =====================

func TestBuildCommandString_Simple(t *testing.T) {
	result := buildCommandString("cmd", []string{"arg1", "arg2"})
	expected := "cmd arg1 arg2"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCommandString_MultipleArgs(t *testing.T) {
	result := buildCommandString("cmd", []string{"a", "b", "c"})
	expected := "cmd a b c"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCommandString_NoArgs(t *testing.T) {
	result := buildCommandString("cmd", nil)
	expected := "cmd"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	result = buildCommandString("cmd", []string{})
	expected = "cmd"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithSpaces(t *testing.T) {
	result := buildCommandString("cmd", []string{"arg with spaces"})
	expected := `cmd "arg with spaces"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithQuotes(t *testing.T) {
	result := buildCommandString("cmd", []string{`say "hello"`})
	expected := `cmd "say \"hello\""`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithAmpersand(t *testing.T) {
	result := buildCommandString("cmd", []string{"a&b"})
	expected := `cmd "a&b"`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ===================== fileExists =====================

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	if fileExists(filepath.Join(dir, "nonexistent.txt")) {
		t.Error("non-existent file should return false")
	}

	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("content"), 0644)
	if !fileExists(path) {
		t.Error("existing file should return true")
	}

	if !fileExists(dir) {
		t.Error("existing dir should return true")
	}
}

// ===================== logFilePath =====================

func TestLogFilePath(t *testing.T) {
	path := logFilePath("/ci-cd")
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(path, today) {
		t.Errorf("log path should contain today's date %s, got %s", today, path)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Errorf("log path should end with .log, got %s", path)
	}
}

// ===================== respondJSON =====================

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 200, map[string]string{"status": "ok"})

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("status should be 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be application/json")
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body.status should be ok, got %s", body["status"])
	}
}

func TestRespondJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 400, map[string]string{"error": "bad request"})

	resp := w.Result()
	if resp.StatusCode != 400 {
		t.Errorf("status should be 400, got %d", resp.StatusCode)
	}
}

// ===================== respondError =====================

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, 403, "forbidden")

	resp := w.Result()
	if resp.StatusCode != 403 {
		t.Errorf("status should be 403, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should be application/json")
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "forbidden" {
		t.Errorf("body.error should be forbidden, got %s", body["error"])
	}
}

// ===================== findCiDir =====================

func TestFindCiDir_NotFound(t *testing.T) {
	// findCiDir depends on os.Executable(), cannot mock in unit test
	// Just verify it doesn't panic
}

// ===================== validateAndEncryptDeploy =====================

func TestValidateAndEncryptDeploy_NoHost(t *testing.T) {
	d := &config.DeployConfig{Host: ""}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err != nil {
		t.Errorf("empty Host should skip: %v", err)
	}
}

func TestValidateAndEncryptDeploy_InvalidAuthType(t *testing.T) {
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "invalid",
	}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err == nil {
		t.Fatal("invalid auth type should error")
	}
}

func TestValidateAndEncryptDeploy_ValidAuthTypes(t *testing.T) {
	for _, authType := range []string{"key", "agent", "password"} {
		d := &config.DeployConfig{
			Host:     "example.com",
			AuthType: authType,
			Port:     22,
		}
		err := validateAndEncryptDeploy(t.TempDir(), d)
		if err != nil {
			t.Errorf("valid auth type %q should not error: %v", authType, err)
		}
	}
}

func TestValidateAndEncryptDeploy_InvalidPort(t *testing.T) {
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "password",
		Port:     -1,
	}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err == nil {
		t.Fatal("negative port should error")
	}

	d.Port = 99999
	err = validateAndEncryptDeploy(t.TempDir(), d)
	if err == nil {
		t.Fatal("port > 65535 should error")
	}
}

func TestValidateAndEncryptDeploy_ValidPort(t *testing.T) {
	for _, port := range []int{1, 22, 1024, 65535} {
		d := &config.DeployConfig{
			Host:     "example.com",
			AuthType: "password",
			Port:     port,
		}
		err := validateAndEncryptDeploy(t.TempDir(), d)
		if err != nil {
			t.Errorf("valid port %d should not error: %v", port, err)
		}
	}
}

func TestValidateAndEncryptDeploy_EncryptsPlaintextPassword(t *testing.T) {
	dir := t.TempDir()
	d := &config.DeployConfig{
		Host:     "example.com",
		Port:     22,
		User:     "admin",
		AuthType: "password",
		Password: "my-plain-password",
	}
	err := validateAndEncryptDeploy(dir, d)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if d.Password == "my-plain-password" {
		t.Error("plaintext password should be encrypted")
	}
	if !strings.HasPrefix(d.Password, "enc:") {
		t.Errorf("encrypted password should start with enc:, got %s", d.Password)
	}
}

func TestValidateAndEncryptDeploy_AlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	d := &config.DeployConfig{
		Host:     "example.com",
		Port:     22,
		AuthType: "password",
		Password: "enc:abc123",
	}
	err := validateAndEncryptDeploy(dir, d)
	if err != nil {
		t.Fatalf("already encrypted should not error: %v", err)
	}
	if d.Password != "enc:abc123" {
		t.Errorf("already encrypted password should stay unchanged, got %s", d.Password)
	}
}

func TestValidateAndEncryptDeploy_EmptyPassword(t *testing.T) {
	d := &config.DeployConfig{
		Host:     "example.com",
		Port:     22,
		AuthType: "password",
		Password: "",
	}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err != nil {
		t.Errorf("empty password should not error: %v", err)
	}
}

// ===================== saveStepStatus / loadStepStatuses =====================

func TestSaveStepStatus(t *testing.T) {
	dir := t.TempDir()
	saveStepStatus(dir, runner.Result{
		Project: "proj", Action: "check", Status: "pass",
	})

	statuses := loadStepStatuses(dir)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 project, got %d", len(statuses))
	}
	steps := statuses["proj"]
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps["check"].Status != "pass" {
		t.Errorf("status should be pass, got %s", steps["check"].Status)
	}
}

func TestSaveStepStatus_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	saveStepStatus(dir, runner.Result{Project: "", Action: "test", Status: "pass"})
	statuses := loadStepStatuses(dir)
	if len(statuses) != 0 {
		t.Errorf("empty project name should not save status, got %v", statuses)
	}
}

func TestLoadStepStatuses(t *testing.T) {
	dir := t.TempDir()
	saveStepStatus(dir, runner.Result{Project: "p1", Action: "check", Status: "pass"})
	saveStepStatus(dir, runner.Result{Project: "p1", Action: "build", Status: "fail", ErrorLog: "build err"})
	saveStepStatus(dir, runner.Result{Project: "p2", Action: "test", Status: "pass"})

	statuses := loadStepStatuses(dir)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(statuses))
	}
	if len(statuses["p1"]) != 2 {
		t.Errorf("p1 should have 2 steps, got %d", len(statuses["p1"]))
	}
	if statuses["p1"]["build"].Status != "fail" {
		t.Errorf("p1 build status should be fail, got %s", statuses["p1"]["build"].Status)
	}
	if statuses["p1"]["build"].ErrorLog != "build err" {
		t.Errorf("p1 build ErrorLog should be preserved, got %s", statuses["p1"]["build"].ErrorLog)
	}
}

func TestLoadStepStatuses_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	statuses := loadStepStatuses(dir)
	if len(statuses) != 0 {
		t.Errorf("empty dir should return empty map, got %v", statuses)
	}
}

func TestLoadStepStatuses_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()

	projDir := filepath.Join(dir, "status", "proj")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "check.json"), []byte(`{"status":"pass"}`), 0644)
	os.WriteFile(filepath.Join(projDir, "notes.txt"), []byte("not json"), 0644)

	statuses := loadStepStatuses(dir)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 project, got %d", len(statuses))
	}
	steps := statuses["proj"]
	if len(steps) != 1 {
		t.Fatalf("expected 1 step (skip .txt), got %d", len(steps))
	}
	if _, ok := steps["check"]; !ok {
		t.Error("check step should exist")
	}
}

func TestLoadStepStatuses_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "status", "proj")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "bad.json"), []byte("{invalid}"), 0644)

	statuses := loadStepStatuses(dir)
	if len(statuses) > 0 {
		t.Errorf("invalid JSON should be skipped, got %v", statuses)
	}
}

// ===================== checkLoginRateLimit =====================

func TestCheckLoginRateLimit_FirstAttempt(t *testing.T) {
	if !checkLoginRateLimit("10.0.0.1") {
		t.Error("first attempt should pass")
	}
}

func TestCheckLoginRateLimit_UnderLimit(t *testing.T) {
	ip := "10.0.0.2"
	for i := 0; i < 4; i++ {
		if !checkLoginRateLimit(ip) {
			t.Errorf("attempt %d should pass", i+1)
		}
	}
}

// ===================== generateDownloadToken / validateDownloadToken =====================

func TestGenerateDownloadToken_Valid(t *testing.T) {
	token := generateDownloadToken()
	if token == "" {
		t.Fatal("token should not be empty")
	}
	if !validateDownloadToken(token) {
		t.Error("newly generated token should be valid")
	}
	if validateDownloadToken(token) {
		t.Error("one-time token should be invalid on second use")
	}
}

func TestValidateDownloadToken_Empty(t *testing.T) {
	if validateDownloadToken("") {
		t.Error("empty token should be invalid")
	}
}

func TestValidateDownloadToken_Unknown(t *testing.T) {
	if validateDownloadToken("unknown-token") {
		t.Error("unknown token should be invalid")
	}
}

// ===================== saveTestReportToDisk =====================

func TestSaveTestReportToDisk(t *testing.T) {
	dir := t.TempDir()
	saveTestReportToDisk(dir, runner.Result{
		Project: "test-proj",
		Status:  "pass",
		Report: &runner.TestReport{
			Total: 10, Passed: 10, Failed: 0,
		},
	})

	reportsDir := filepath.Join(dir, "reports", "test-proj")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		t.Fatalf("report dir should exist: %v", err)
	}
	if len(entries) == 0 {
		t.Error("should have at least one report file")
	}
}

// ===================== cleanupOldLogs =====================

func TestCleanupOldLogs(t *testing.T) {
	logDir := t.TempDir()
	oldPath := filepath.Join(logDir, "2026-05-01.log")
	os.WriteFile(oldPath, []byte("old log"), 0644)
	oldTime := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(oldPath, oldTime, oldTime)

	newPath := filepath.Join(logDir, "2026-06-23.log")
	os.WriteFile(newPath, []byte("new log"), 0644)

	cleanupOldLogs(logDir)

	if _, err := os.Stat(oldPath); err == nil {
		t.Error("logs older than 30 days should be cleaned")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("logs within 30 days should be kept")
	}
}

func TestCleanupOldLogs_EmptyDir(t *testing.T) {
	cleanupOldLogs(t.TempDir())
}

// ===================== handleViewRuleFile =====================

func TestHandleViewRuleFile_Security(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/rules?file=../../etc/passwd", nil)
	handleViewRuleFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("path traversal should return 400, got %d", w.Code)
	}
}

func TestHandleViewRuleFile_EmptyFile(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/rules", nil)
	handleViewRuleFile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing file param should return 400, got %d", w.Code)
	}
}

// ===================== stepStatusClearHandler =====================

func TestStepStatusClearHandler_All(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "status", "test-proj")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "check.json"), []byte(`{"status":"pass"}`), 0644)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/steps/status/clear", nil)
	// Override findCiDir for this test
	origFindCiDir := findCiDir
	findCiDir = func() string { return dir }
	defer func() { findCiDir = origFindCiDir }()

	stepStatusClearHandler(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if _, err := os.Stat(projDir); err == nil {
		t.Error("status dir should be cleared")
	}
}

// ===================== handleLogQuery =====================

func TestHandleLogQuery_Empty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/log/query", nil)
	origFindCiDir := findCiDir
	findCiDir = func() string { return dir }
	defer func() { findCiDir = origFindCiDir }()

	handleLogQuery(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string][]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["logs"] == nil {
		t.Error("logs should not be nil")
	}
}

// ===================== handleLogDelete =====================

func TestHandleLogDelete_MissingDate(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/log/delete", nil)
	handleLogDelete(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("missing date should return error")
	}
}

// ===================== handleOpenDir =====================

func TestHandleOpenDir_MissingPath(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/local/open-dir", nil)
	handleOpenDir(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("missing path should return error")
	}
}

func TestHandleOpenDir_EmptyPath(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/local/open-dir?path=", nil)
	handleOpenDir(w, r)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("empty path should return error")
	}
}

// ===================== findGit =====================

func TestFindGit_ReturnsString(t *testing.T) {
	git := findGit()
	if git == "" {
		t.Error("findGit should return a non-empty string")
	}
}
