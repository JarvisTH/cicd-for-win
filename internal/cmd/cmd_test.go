package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"ci-cd/internal/config"
)

// ===================== cliServer / cliServerList 序列化 =====================

func TestCliServerJSON(t *testing.T) {
	s := cliServer{
		Name:         "test-server",
		Host:         "10.0.0.1",
		Port:         22,
		User:         "admin",
		AuthType:     "password",
		IdentityFile: "",
		Password:     "enc:abc",
		Note:         "测试服务器",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"name":"test-server"`) {
		t.Errorf("应包含 name 字段, 实际: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"password":"enc:abc"`) {
		t.Errorf("应包含 password 字段, 实际: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "identity_file") {
		t.Error("空 identity_file 应被 omitempty 省略")
	}

	var decoded cliServer
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if decoded.Name != "test-server" {
		t.Errorf("Name 不匹配: 期望 test-server, 得到 %q", decoded.Name)
	}
	if decoded.Password != "enc:abc" {
		t.Errorf("Password 不匹配: 期望 enc:abc, 得到 %q", decoded.Password)
	}
}

func TestCliServerListJSON(t *testing.T) {
	list := cliServerList{
		Servers: []cliServer{
			{Name: "server1", Host: "10.0.0.1"},
			{Name: "server2", Host: "10.0.0.2"},
		},
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded cliServerList
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if len(decoded.Servers) != 2 {
		t.Fatalf("Servers 数应为 2, 得到 %d", len(decoded.Servers))
	}
}

func TestCliServerList_Empty(t *testing.T) {
	list := cliServerList{Servers: []cliServer{}}
	data, _ := json.Marshal(list)
	if !strings.Contains(string(data), `"servers":[]`) {
		t.Error("空列表应序列化为 []")
	}
}

// ===================== migratePlaintextPasswords =====================

func TestMigratePlaintextPasswords_AlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()

	list := &cliServerList{
		Servers: []cliServer{
			{Name: "s1", Host: "h1", AuthType: "password", Password: "enc:existing"},
		},
	}

	migratePlaintextPasswords(dir, list)
	if list.Servers[0].Password != "enc:existing" {
		t.Errorf("已加密密码应保持不变, 得到 %q", list.Servers[0].Password)
	}
}

func TestMigratePlaintextPasswords_KeyAuth(t *testing.T) {
	dir := t.TempDir()

	list := &cliServerList{
		Servers: []cliServer{
			{Name: "s1", Host: "h1", AuthType: "key", Password: "plain-text"},
		},
	}

	migratePlaintextPasswords(dir, list)
	if list.Servers[0].Password != "plain-text" {
		t.Errorf("key 认证的明文密码应保持不变, 得到 %q", list.Servers[0].Password)
	}
}

func TestMigratePlaintextPasswords_EncryptsPassword(t *testing.T) {
	dir := t.TempDir()

	list := &cliServerList{
		Servers: []cliServer{
			{Name: "s1", Host: "h1", AuthType: "password", Password: "my-plain-password"},
		},
	}

	migratePlaintextPasswords(dir, list)
	if list.Servers[0].Password == "my-plain-password" {
		t.Error("明文密码应被加密")
	}
	if !strings.HasPrefix(list.Servers[0].Password, "enc:") {
		t.Errorf("加密后应以 enc: 开头, 得到 %q", list.Servers[0].Password)
	}
}

func TestMigratePlaintextPasswords_EmptyPassword(t *testing.T) {
	dir := t.TempDir()

	list := &cliServerList{
		Servers: []cliServer{
			{Name: "s1", Host: "h1", AuthType: "password", Password: ""},
		},
	}

	migratePlaintextPasswords(dir, list)
	if list.Servers[0].Password != "" {
		t.Errorf("空密码应保持不变, 得到 %q", list.Servers[0].Password)
	}
}

func TestMigratePlaintextPasswords_MultipleServers(t *testing.T) {
	dir := t.TempDir()

	list := &cliServerList{
		Servers: []cliServer{
			{Name: "s1", AuthType: "password", Password: "pwd1"},
			{Name: "s2", AuthType: "password", Password: "enc:already"},
			{Name: "s3", AuthType: "key", Password: "not-encrypted"},
			{Name: "s4", AuthType: "password", Password: "pwd4"},
		},
	}

	migratePlaintextPasswords(dir, list)

	if list.Servers[0].Password == "pwd1" || !strings.HasPrefix(list.Servers[0].Password, "enc:") {
		t.Errorf("s1 密码应加密, 得到 %q", list.Servers[0].Password)
	}
	if list.Servers[1].Password != "enc:already" {
		t.Errorf("s2 已加密应不变, 得到 %q", list.Servers[1].Password)
	}
	if list.Servers[2].Password != "not-encrypted" {
		t.Errorf("s3 key 认证应不变, 得到 %q", list.Servers[2].Password)
	}
	if list.Servers[3].Password == "pwd4" || !strings.HasPrefix(list.Servers[3].Password, "enc:") {
		t.Errorf("s4 密码应加密, 得到 %q", list.Servers[3].Password)
	}
}

// ===================== saveServerList / loadServerList =====================

func TestLoadServerList_FileNotFound(t *testing.T) {
	list := loadServerList()
	if list == nil {
		t.Fatal("loadServerList 应返回非 nil 的列表")
	}
	if list.Servers == nil {
		t.Error("Servers 应初始化为空切片")
	}
}

// ===================== buildProjectDetails =====================

func TestBuildProjectDetails_Empty(t *testing.T) {
	cfg := &config.Config{Projects: []config.Project{}}
	details := buildProjectDetails(cfg)
	if len(details) != 0 {
		t.Errorf("空配置应返回空列表, 得到 %d", len(details))
	}
}

func TestBuildProjectDetails_Filter(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{Name: "proj-a", Path: "/tmp/a", Enabled: true},
			{Name: "proj-b", Path: "/tmp/b", Enabled: false},
		},
	}
	details := buildProjectDetails(cfg)
	if len(details) != 2 {
		t.Fatalf("应返回 2 个项目, 得到 %d", len(details))
	}
	if !details[0].Enabled {
		t.Error("proj-a 应为启用")
	}
	if details[1].Enabled {
		t.Error("proj-b 应为禁用")
	}
}

func TestBuildProjectDetails_WithDeployHost(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{Name: "deploy-proj", Path: "/tmp/d", Enabled: true,
				Deploy: &config.DeployConfig{Host: "192.168.1.1"}},
		},
	}
	details := buildProjectDetails(cfg)
	if len(details) != 1 {
		t.Fatalf("应返回 1 个项目")
	}
	if details[0].DeployHost != "192.168.1.1" {
		t.Errorf("DeployHost 应为 192.168.1.1, 得到 %s", details[0].DeployHost)
	}
}

// ===================== printProjectList =====================

func TestPrintProjectList_Text(t *testing.T) {
	details := []projectDetail{
		{Name: "proj-a", Enabled: true, Version: "1.0", DeployHost: "host-a"},
		{Name: "proj-b", Enabled: false},
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printProjectList(details, false)

	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("printProjectList 失败: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "proj-a") {
		t.Error("输出应包含 proj-a")
	}
	if !strings.Contains(output, "✅") {
		t.Error("启用的项目应显示 ✅")
	}
	if !strings.Contains(output, "🔘") {
		t.Error("禁用的项目应显示 🔘")
	}
}

func TestPrintProjectList_JSON(t *testing.T) {
	details := []projectDetail{
		{Name: "json-proj", Enabled: true, Version: "2.0"},
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printProjectList(details, true)

	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("printProjectList(JSON) 失败: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"name": "json-proj"`) {
		t.Errorf("JSON 应包含项目名, 输出: %s", output)
	}
	if !strings.Contains(output, `"enabled": true`) {
		t.Error("JSON 应包含 enabled 字段")
	}
}

func TestPrintProjectList_Empty(t *testing.T) {
	err := printProjectList([]projectDetail{}, false)
	if err != nil {
		t.Errorf("空列表不应报错, 得到: %v", err)
	}
}

// ===================== printDoctorResults =====================

func TestPrintDoctorResults_Text(t *testing.T) {
	checks := []checkItem{
		{Name: "Go", Status: "ok", Message: "已安装"},
		{Name: "Node", Status: "warn", Message: "未找到"},
		{Name: "Runner", Status: "error", Message: "缺失"},
	}

	var buf bytes.Buffer
	err := printDoctorResults(&buf, checks, false)
	if err != nil {
		t.Fatalf("printDoctorResults 失败: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "✅") {
		t.Error("ok 状态应显示 ✅")
	}
	if !strings.Contains(output, "⚠️") {
		t.Error("warn 状态应显示 ⚠️")
	}
	if !strings.Contains(output, "❌") {
		t.Error("error 状态应显示 ❌")
	}
	if !strings.Contains(output, "存在严重问题") {
		t.Error("有 error 时应提示严重问题")
	}
}

func TestPrintDoctorResults_JSON(t *testing.T) {
	checks := []checkItem{
		{Name: "Go", Status: "ok", Message: "已安装"},
	}
	err := printDoctorResults(os.Stdout, checks, true)
	if err != nil {
		t.Fatalf("printDoctorResults(JSON) 失败: %v", err)
	}
}

func TestPrintDoctorResults_NoIssues(t *testing.T) {
	checks := []checkItem{
		{Name: "All", Status: "ok", Message: "正常"},
	}
	var buf bytes.Buffer
	err := printDoctorResults(&buf, checks, false)
	if err != nil {
		t.Fatalf("printDoctorResults 失败: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "环境正常") {
		t.Error("全部 ok 时应显示环境正常")
	}
}

func TestPrintDoctorResults_WarnOnly(t *testing.T) {
	checks := []checkItem{
		{Name: "Optional", Status: "warn", Message: "可选未安装"},
	}
	var buf bytes.Buffer
	err := printDoctorResults(&buf, checks, false)
	if err != nil {
		t.Fatalf("printDoctorResults 失败: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "部分环境未完整安装") {
		t.Error("有 warn 时应提示部分未安装")
	}
}

// ===================== runReport =====================

func TestRunReport_DeleteMissing(t *testing.T) {
	err := runReport("nonexistent-proj", false, false, "nonexistent-id")
	if err == nil {
		t.Log("删除不存在的报告返回 nil（预期可能的行为）")
	}
}

// ===================== projectDetail 序列化 =====================

func TestProjectDetailJSON(t *testing.T) {
	d := projectDetail{
		Name:    "test",
		Enabled: true,
		Version: "1.0",
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	if !strings.Contains(string(data), `"name":"test"`) {
		t.Error("JSON 应包含 name")
	}
}


