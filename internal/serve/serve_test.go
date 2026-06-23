package serve

import (
	"encoding/json"
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
			t.Errorf("有效步骤 ID %q 应返回 true", id)
		}
	}

	invalidIDs := []string{"", "deploy2", "Check", "BUILD", "lint", "all"}
	for _, id := range invalidIDs {
		if isValidStepID(id) {
			t.Errorf("无效步骤 ID %q 应返回 false", id)
		}
	}
}

// ===================== stepStatusDir =====================

func TestStepStatusDir(t *testing.T) {
	dir := stepStatusDir("/ci-cd")
	expected := filepath.Join("/ci-cd", "status")
	if dir != expected {
		t.Errorf("期望 %q, 得到 %q", expected, dir)
	}
}

func TestStepStatusDirRelative(t *testing.T) {
	dir := stepStatusDir(".")
	expected := filepath.Join(".", "status")
	if dir != expected {
		t.Errorf("期望 %q, 得到 %q", expected, dir)
	}
}

// ===================== serversFilePath =====================

func TestServersFilePath(t *testing.T) {
	path := serversFilePath("/ci-cd")
	expected := filepath.Join("/ci-cd", "servers.json")
	if path != expected {
		t.Errorf("期望 %q, 得到 %q", expected, path)
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
		t.Error("非密码字段应保留")
	}
	if output["user"] != "admin" {
		t.Error("非密码字段应保留")
	}
	if pwd, ok := output["password"]; !ok || pwd != "" {
		t.Errorf("密码字段应清空, 得到 %v", pwd)
	}
}

func TestSanitizeDeploy_NoPassword(t *testing.T) {
	input := map[string]any{"host": "example.com"}
	output := sanitizeDeploy(input)
	if output["host"] != "example.com" {
		t.Error("无密码字段时其余字段应保留")
	}
}

func TestSanitizeDeploy_Empty(t *testing.T) {
	output := sanitizeDeploy(map[string]any{})
	if len(output) != 0 {
		t.Errorf("空输入应返回空 map, 得到 %v", output)
	}
}

func TestSanitizeDeploy_Nil(t *testing.T) {
	output := sanitizeDeploy(nil)
	if len(output) != 0 {
		t.Errorf("nil 输入应返回空 map, 得到 %v", output)
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
		t.Error("密码应被清空")
	}
	if safe.Name != "test" {
		t.Error("名称应保留")
	}
}

func TestSanitizeServer_EmptyPassword(t *testing.T) {
	s := StandaloneServer{Name: "test", Password: ""}
	safe := sanitizeServer(s)
	if safe.Password != "" {
		t.Error("空密码应保持为空")
	}
}

// ===================== buildCommandString =====================

func TestBuildCommandString_Simple(t *testing.T) {
	result := buildCommandString("cmd", []string{"arg1", "arg2"})
	expected := "cmd arg1 arg2"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_MultipleArgs(t *testing.T) {
	result := buildCommandString("cmd", []string{"a", "b", "c"})
	expected := "cmd a b c"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_NoArgs(t *testing.T) {
	result := buildCommandString("cmd", nil)
	expected := "cmd"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}

	result = buildCommandString("cmd", []string{})
	expected = "cmd"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithSpaces(t *testing.T) {
	result := buildCommandString("cmd", []string{"arg with spaces"})
	expected := `cmd "arg with spaces"`
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithQuotes(t *testing.T) {
	result := buildCommandString("cmd", []string{`say "hello"`})
	expected := `cmd "say \"hello\""`
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithAmpersand(t *testing.T) {
	result := buildCommandString("cmd", []string{"a&b"})
	expected := `cmd "a&b"`
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

// ===================== fileExists =====================

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	if fileExists(filepath.Join(dir, "nonexistent.txt")) {
		t.Error("不存在的文件应返回 false")
	}

	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("content"), 0644)
	if !fileExists(path) {
		t.Error("存在的文件应返回 true")
	}

	if !fileExists(dir) {
		t.Error("存在的目录应返回 true")
	}
}

// ===================== logFilePath =====================

func TestLogFilePath(t *testing.T) {
	path := logFilePath("/ci-cd")
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(path, today) {
		t.Errorf("日志路径应包含今天日期 %s, 得到 %s", today, path)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Errorf("日志路径应以 .log 结尾, 得到 %s", path)
	}
}

// ===================== respondJSON =====================

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 200, map[string]string{"status": "ok"})

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("状态码应为 200, 得到 %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type 应为 application/json")
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body.status 应为 ok, 得到 %s", body["status"])
	}
}

func TestRespondJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, 400, map[string]string{"error": "bad request"})

	resp := w.Result()
	if resp.StatusCode != 400 {
		t.Errorf("状态码应为 400, 得到 %d", resp.StatusCode)
	}
}

// ===================== respondError =====================

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, 403, "forbidden")

	resp := w.Result()
	if resp.StatusCode != 403 {
		t.Errorf("状态码应为 403, 得到 %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type 应为 application/json")
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "forbidden" {
		t.Errorf("body.error 应为 forbidden, 得到 %s", body["error"])
	}
}

// ===================== findCiDir =====================

func TestFindCiDir_NotFound(t *testing.T) {
	// findCiDir 依赖 os.Executable()，无法在测试中直接 mock
	// 这里只验证它不会 panic
}

// ===================== validateAndEncryptDeploy =====================

func TestValidateAndEncryptDeploy_NoHost(t *testing.T) {
	d := &config.DeployConfig{Host: ""}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err != nil {
		t.Errorf("空 Host 应跳过: %v", err)
	}
}

func TestValidateAndEncryptDeploy_InvalidAuthType(t *testing.T) {
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "invalid",
	}
	err := validateAndEncryptDeploy(t.TempDir(), d)
	if err == nil {
		t.Fatal("非法认证类型应报错")
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
			t.Errorf("合法认证类型 %q 不应报错: %v", authType, err)
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
		t.Fatal("负端口应报错")
	}

	d.Port = 99999
	err = validateAndEncryptDeploy(t.TempDir(), d)
	if err == nil {
		t.Fatal("大于 65535 的端口应报错")
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
			t.Errorf("有效端口 %d 不应报错: %v", port, err)
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
		t.Fatalf("加密失败: %v", err)
	}
	if d.Password == "my-plain-password" {
		t.Error("明文密码应被加密")
	}
	if !strings.HasPrefix(d.Password, "enc:") {
		t.Errorf("加密后应以 enc: 开头, 得到 %s", d.Password)
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
		t.Fatalf("已加密密码不应报错: %v", err)
	}
	if d.Password != "enc:abc123" {
		t.Errorf("已加密密码应保持不变, 得到 %s", d.Password)
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
		t.Errorf("空密码不应报错: %v", err)
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
		t.Fatalf("应有 1 个项目, 得到 %d", len(statuses))
	}
	steps := statuses["proj"]
	if len(steps) != 1 {
		t.Fatalf("应有 1 个步骤, 得到 %d", len(steps))
	}
	if steps["check"].Status != "pass" {
		t.Errorf("状态应为 pass, 得到 %s", steps["check"].Status)
	}
}

func TestSaveStepStatus_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	saveStepStatus(dir, runner.Result{Project: "", Action: "test", Status: "pass"})
	statuses := loadStepStatuses(dir)
	if len(statuses) != 0 {
		t.Errorf("空项目名不应保存状态, 得到 %v", statuses)
	}
}

func TestLoadStepStatuses(t *testing.T) {
	dir := t.TempDir()
	saveStepStatus(dir, runner.Result{Project: "p1", Action: "check", Status: "pass"})
	saveStepStatus(dir, runner.Result{Project: "p1", Action: "build", Status: "fail", ErrorLog: "build err"})
	saveStepStatus(dir, runner.Result{Project: "p2", Action: "test", Status: "pass"})

	statuses := loadStepStatuses(dir)
	if len(statuses) != 2 {
		t.Fatalf("应有 2 个项目, 得到 %d", len(statuses))
	}
	if len(statuses["p1"]) != 2 {
		t.Errorf("p1 应有 2 个步骤, 得到 %d", len(statuses["p1"]))
	}
	if statuses["p1"]["build"].Status != "fail" {
		t.Errorf("p1 build 状态应为 fail, 得到 %s", statuses["p1"]["build"].Status)
	}
	if statuses["p1"]["build"].ErrorLog != "build err" {
		t.Errorf("p1 build ErrorLog 应保留, 得到 %s", statuses["p1"]["build"].ErrorLog)
	}
}

func TestLoadStepStatuses_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	statuses := loadStepStatuses(dir)
	if len(statuses) != 0 {
		t.Errorf("空目录应返回空 map, 得到 %v", statuses)
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
		t.Fatalf("应有 1 个项目, 得到 %d", len(statuses))
	}
	steps := statuses["proj"]
	if len(steps) != 1 {
		t.Fatalf("应有 1 个步骤（跳过 .txt）, 得到 %d", len(steps))
	}
	if _, ok := steps["check"]; !ok {
		t.Error("check 步骤应存在")
	}
}

func TestLoadStepStatuses_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "status", "proj")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "bad.json"), []byte("{invalid}"), 0644)

	statuses := loadStepStatuses(dir)
	if len(statuses) > 0 {
		t.Errorf("无效 JSON 应被跳过, 得到 %v", statuses)
	}
}
