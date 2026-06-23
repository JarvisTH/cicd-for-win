package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	input := map[string]any{
		"host": "example.com",
		"auth": "key",
	}
	output := sanitizeDeploy(input)
	if output["host"] != "example.com" {
		t.Error("字段应保留")
	}
	// 原始 map 未被修改（复制）
	if _, ok := input["password"]; ok {
		t.Error("原始 map 不应有 password 字段")
	}
}

func TestSanitizeDeploy_Empty(t *testing.T) {
	output := sanitizeDeploy(map[string]any{})
	if len(output) != 0 {
		t.Errorf("空输入应返回空 map, 得到 %v", output)
	}
}

func TestSanitizeDeploy_Nil(t *testing.T) {
	// 传入 nil map
	var input map[string]any = nil
	result := sanitizeDeploy(input)
	if result == nil {
		t.Error("nil 输入应返回非 nil 空 map")
	}
	if len(result) != 0 {
		t.Errorf("nil 输入应返回空 map, 得到 %v", result)
	}
}

// ===================== sanitizeServer =====================

func TestSanitizeServer(t *testing.T) {
	s := StandaloneServer{
		Name:     "test-server",
		Host:     "10.0.0.1",
		Port:     22,
		User:     "admin",
		AuthType: "password",
		Password: "supersecret",
		Note:     "production",
	}
	cleaned := sanitizeServer(s)
	if cleaned.Name != "test-server" {
		t.Error("Name 应保留")
	}
	if cleaned.Host != "10.0.0.1" {
		t.Error("Host 应保留")
	}
	if cleaned.Password != "" {
		t.Error("Password 应被清空")
	}
	// 原始对象不受影响
	if s.Password != "supersecret" {
		t.Error("原始对象不应被修改（值传递）")
	}
}

func TestSanitizeServer_EmptyPassword(t *testing.T) {
	s := StandaloneServer{Name: "test", Host: "x", Password: ""}
	cleaned := sanitizeServer(s)
	if cleaned.Name != "test" {
		t.Error("Name 应保留")
	}
	if cleaned.Password != "" {
		t.Error("空密码仍应为空")
	}
}

// ===================== buildCommandString =====================

func TestBuildCommandString_Simple(t *testing.T) {
	result := buildCommandString("git", []string{"status"})
	expected := "git status"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_MultipleArgs(t *testing.T) {
	result := buildCommandString("npm", []string{"run", "build", "--prod"})
	expected := "npm run build --prod"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_NoArgs(t *testing.T) {
	result := buildCommandString("ci", nil)
	expected := "ci"
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}

	result = buildCommandString("ci", []string{})
	if result != "ci" {
		t.Errorf("空参数列表: 期望 %q, 得到 %q", "ci", result)
	}
}

func TestBuildCommandString_ArgWithSpaces(t *testing.T) {
	result := buildCommandString("echo", []string{"hello world", "test"})
	expected := `echo "hello world" test`
	if result != expected {
		t.Errorf("期望 %q, 得到 %q", expected, result)
	}
}

func TestBuildCommandString_ArgWithQuotes(t *testing.T) {
	result := buildCommandString("echo", []string{`say "hi"`})
	expected := `echo "say \"hi\""`
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

	// 文件不存在
	if fileExists(filepath.Join(dir, "nonexistent.txt")) {
		t.Error("不存在的文件应返回 false")
	}

	// 创建文件
	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("content"), 0644)
	if !fileExists(path) {
		t.Error("存在的文件应返回 true")
	}

	// 目录也存在
	if !fileExists(dir) {
		t.Error("存在的目录应返回 true")
	}
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
	// 端口 0 有效（会在 normalize 阶段被修正为 22），排除在无效列表外
	tests := []int{-1, 65536, 99999}
	for _, port := range tests {
		d := &config.DeployConfig{
			Host:     "example.com",
			AuthType: "key",
			Port:     port,
		}
		err := validateAndEncryptDeploy(t.TempDir(), d)
		if err == nil {
			t.Errorf("端口 %d 应报错", port)
		}
	}
}

func TestValidateAndEncryptDeploy_ValidPort(t *testing.T) {
	tests := []int{1, 22, 1024, 65535}
	for _, port := range tests {
		d := &config.DeployConfig{
			Host:     "example.com",
			AuthType: "key",
			Port:     port,
		}
		err := validateAndEncryptDeploy(t.TempDir(), d)
		if err != nil {
			t.Errorf("端口 %d 不应报错: %v", port, err)
		}
	}
}

func TestValidateAndEncryptDeploy_EncryptsPlaintextPassword(t *testing.T) {
	dir := t.TempDir()
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "password",
		Port:     22,
		Password: "my-plain-password",
	}

	err := validateAndEncryptDeploy(dir, d)
	if err != nil {
		t.Fatalf("加密密码失败: %v", err)
	}
	if d.Password == "my-plain-password" {
		t.Error("明文密码应被加密")
	}
	if !strings.HasPrefix(d.Password, "enc:") {
		t.Errorf("加密密码应以 enc: 开头, 得到 %q", d.Password)
	}
}

func TestValidateAndEncryptDeploy_AlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	// 已加密的密码应保留
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "password",
		Port:     22,
		Password: "enc:abc123",
	}

	err := validateAndEncryptDeploy(dir, d)
	if err != nil {
		t.Fatalf("已加密密码不应报错: %v", err)
	}
	if d.Password != "enc:abc123" {
		t.Errorf("已加密密码应保持不变, 得到 %q", d.Password)
	}
}

func TestValidateAndEncryptDeploy_EmptyPassword(t *testing.T) {
	dir := t.TempDir()
	d := &config.DeployConfig{
		Host:     "example.com",
		AuthType: "password",
		Port:     22,
		Password: "",
	}

	err := validateAndEncryptDeploy(dir, d)
	if err != nil {
		t.Fatalf("空密码不应报错: %v", err)
	}
	if d.Password != "" {
		t.Errorf("空密码应保持不变, 得到 %q", d.Password)
	}
}

// ===================== saveStepStatus & loadStepStatuses =====================

func TestSaveStepStatus(t *testing.T) {
	dir := t.TempDir()

	result := runner.Result{
		Project: "test-proj",
		Action:  "check",
		Status:  "pass",
	}

	saveStepStatus(dir, result)

	statusDir := filepath.Join(dir, "status", "test-proj")
	if _, err := os.Stat(statusDir); os.IsNotExist(err) {
		t.Fatal("status 目录未创建")
	}

	statusFile := filepath.Join(statusDir, "check.json")
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		t.Fatal("状态文件未创建")
	}
}

func TestSaveStepStatus_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	// 空 Project 不应写文件
	saveStepStatus(dir, runner.Result{Project: "", Action: "check"})

	statusDir := filepath.Join(dir, "status")
	if entries, _ := os.ReadDir(statusDir); len(entries) > 0 {
		t.Error("空 Project 不应创建状态文件")
	}

	// 空 Action 也不应写文件
	saveStepStatus(dir, runner.Result{Project: "p", Action: ""})
	if entries, _ := os.ReadDir(statusDir); len(entries) > 0 {
		t.Error("空 Action 不应创建状态文件")
	}
}

func TestLoadStepStatuses(t *testing.T) {
	dir := t.TempDir()

	// 保存状态后再加载
	saveStepStatus(dir, runner.Result{
		Project: "proj-a", Action: "check", Status: "pass",
	})
	saveStepStatus(dir, runner.Result{
		Project: "proj-a", Action: "build", Status: "fail", ErrorLog: "build error",
	})
	saveStepStatus(dir, runner.Result{
		Project: "proj-b", Action: "test", Status: "pass",
	})

	statuses := loadStepStatuses(dir)
	if len(statuses) != 2 {
		t.Fatalf("应有 2 个项目的状态, 得到 %d", len(statuses))
	}

	// proj-a 应有 2 个步骤
	projA, ok := statuses["proj-a"]
	if !ok {
		t.Fatal("proj-a 的状态应存在")
	}
	if len(projA) != 2 {
		t.Fatalf("proj-a 应有 2 个步骤, 得到 %d", len(projA))
	}

	checkStatus, ok := projA["check"]
	if !ok {
		t.Fatal("proj-a 的 check 状态应存在")
	}
	if checkStatus.Status != "pass" {
		t.Errorf("check 状态应为 pass, 得到 %q", checkStatus.Status)
	}

	buildStatus, ok := projA["build"]
	if !ok {
		t.Fatal("proj-a 的 build 状态应存在")
	}
	if buildStatus.Status != "fail" {
		t.Errorf("build 状态应为 fail, 得到 %q", buildStatus.Status)
	}
	if buildStatus.ErrorLog != "build error" {
		t.Errorf("build ErrorLog 不匹配: 期望 %q, 得到 %q", "build error", buildStatus.ErrorLog)
	}

	// proj-b 应有 1 个步骤
	projB, ok := statuses["proj-b"]
	if !ok {
		t.Fatal("proj-b 的状态应存在")
	}
	if len(projB) != 1 {
		t.Fatalf("proj-b 应有 1 个步骤, 得到 %d", len(projB))
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

	// 创建非 JSON 文件在项目目录中
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

// ===================== (buildRunnerArgs 已迁移到 Go 原生实现，相关测试不再需要) =====================
