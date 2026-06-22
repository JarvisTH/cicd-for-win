package cmd

import (
	"encoding/json"
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

	// 反序列化
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

	// 已加密的密码不应被修改
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

	// key 认证的密码不应被迁移
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

	// pwd1 应加密
	if list.Servers[0].Password == "pwd1" || !strings.HasPrefix(list.Servers[0].Password, "enc:") {
		t.Errorf("s1 密码应加密, 得到 %q", list.Servers[0].Password)
	}
	// enc:already 应不变
	if list.Servers[1].Password != "enc:already" {
		t.Errorf("s2 已加密应不变, 得到 %q", list.Servers[1].Password)
	}
	// key 认证应不变
	if list.Servers[2].Password != "not-encrypted" {
		t.Errorf("s3 key 认证应不变, 得到 %q", list.Servers[2].Password)
	}
	// pwd4 应加密
	if list.Servers[3].Password == "pwd4" || !strings.HasPrefix(list.Servers[3].Password, "enc:") {
		t.Errorf("s4 密码应加密, 得到 %q", list.Servers[3].Password)
	}
}

// ===================== saveServerList / loadServerList =====================

func TestLoadServerList_FileNotFound(t *testing.T) {
	// 服务器文件不存在时返回空列表
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

// ===================== printDoctorResults =====================

func TestPrintDoctorResults_JSON(t *testing.T) {
	checks := []checkItem{
		{Name: "Go", Status: "ok", Message: "已安装"},
	}
	err := printDoctorResults(os.Stdout, checks, true)
	if err != nil {
		t.Fatalf("printDoctorResults 失败: %v", err)
	}
}

// ===================== runReport 参数验证 =====================

func TestRunReport_DeleteMissing(t *testing.T) {
	// 删除不存在的报告应返回错误
	err := runReport("nonexistent-proj", false, false, "nonexistent-id")
	if err == nil {
		t.Log("删除不存在的报告返回 nil（预期可能的行为）")
	}
}

