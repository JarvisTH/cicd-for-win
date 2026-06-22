package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ===================== NewAuthConfig =====================

func TestNewAuthConfig(t *testing.T) {
	auth := NewAuthConfig("admin", "mypassword")
	if auth.Username != "admin" {
		t.Errorf("用户名应为 admin, 得到 %q", auth.Username)
	}
	if auth.Salt == "" {
		t.Error("Salt 不应为空")
	}
	if auth.Hash == "" {
		t.Error("Hash 不应为空")
	}
}

func TestNewAuthConfig_SaltUniqueness(t *testing.T) {
	auth1 := NewAuthConfig("user", "pass")
	auth2 := NewAuthConfig("user", "pass")

	if auth1.Salt == auth2.Salt {
		t.Error("两次生成的 Salt 应该不同")
	}
	if auth1.Hash == auth2.Hash {
		t.Error("不同 Salt 下相同密码的 Hash 应该不同")
	}
}

func TestNewAuthConfig_EmptyValues(t *testing.T) {
	auth := NewAuthConfig("", "")
	if auth.Username != "" {
		t.Errorf("空用户名应保留为空, 得到 %q", auth.Username)
	}
	if auth.Hash == "" {
		t.Error("空密码也应生成 Hash")
	}
}

// ===================== VerifyPassword =====================

func TestVerifyPassword_Correct(t *testing.T) {
	auth := NewAuthConfig("admin", "correct-password")
	if !auth.VerifyPassword("correct-password") {
		t.Error("正确密码应验证通过")
	}
}

func TestVerifyPassword_Wrong(t *testing.T) {
	auth := NewAuthConfig("admin", "correct-password")
	if auth.VerifyPassword("wrong-password") {
		t.Error("错误密码应验证失败")
	}
}

func TestVerifyPassword_Empty(t *testing.T) {
	auth := NewAuthConfig("admin", "")
	if !auth.VerifyPassword("") {
		t.Error("空密码应验证通过")
	}
	if auth.VerifyPassword("not-empty") {
		t.Error("非空密码不应通过空密码验证")
	}
}

func TestVerifyPassword_Default(t *testing.T) {
	auth := NewAuthConfig(DefaultUsername, DefaultPassword)
	if !auth.VerifyPassword(DefaultPassword) {
		t.Error("默认密码应验证通过")
	}
}

func TestVerifyPassword_CrossUser(t *testing.T) {
	_ = NewAuthConfig("user1", "secret1") // user1 仅用于创建，不验证
	auth2 := NewAuthConfig("user2", "secret2")

	// user1 的密码不应通过 user2 的验证
	if auth2.VerifyPassword("secret1") {
		t.Error("user2 不应验证 user1 的密码")
	}
}

// ===================== hashPassword =====================

func TestHashPassword_Deterministic(t *testing.T) {
	h1 := hashPassword("salt123", "mypassword")
	h2 := hashPassword("salt123", "mypassword")
	if h1 != h2 {
		t.Error("相同 salt+password 应生成相同 hash")
	}
}

func TestHashPassword_Different(t *testing.T) {
	h1 := hashPassword("salt1", "mypassword")
	h2 := hashPassword("salt2", "mypassword")
	if h1 == h2 {
		t.Error("不同 salt 应生成不同 hash")
	}

	h3 := hashPassword("salt1", "pass1")
	h4 := hashPassword("salt1", "pass2")
	if h3 == h4 {
		t.Error("不同密码应生成不同 hash")
	}
}

func TestHashPassword_Format(t *testing.T) {
	hash := hashPassword("test-salt", "test-pass")
	if len(hash) != 64 { // SHA256 hex = 64 字符
		t.Errorf("SHA256 hex 应为 64 字符, 得到 %d: %q", len(hash), hash)
	}
	// 应为合法 hex 字符
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("hash 包含非法字符: %c", c)
		}
	}
}

// ===================== generateSalt =====================

func TestGenerateSalt_Format(t *testing.T) {
	salt := generateSalt()
	if salt == "" {
		t.Fatal("salt 不应为空")
	}
	if salt == "default-salt-fallback" {
		t.Error("不应触发 fallback")
	}
	// base64 编码的 16 字节应为 24 字符（含填充）
	if len(salt) < 20 {
		t.Errorf("salt 长度异常: %d", len(salt))
	}
}

func TestGenerateSalt_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		salt := generateSalt()
		if seen[salt] {
			t.Errorf("salt 重复: %q", salt)
		}
		seen[salt] = true
	}
}

// ===================== SaveAuth / LoadAuth =====================

func TestSaveAndLoadAuth(t *testing.T) {
	dir := t.TempDir()
	auth := NewAuthConfig("testuser", "testpass123")

	err := SaveAuth(dir, auth)
	if err != nil {
		t.Fatalf("SaveAuth 失败: %v", err)
	}

	// 验证文件已创建
	authPath := filepath.Join(dir, AuthFileName)
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		t.Fatal("auth.json 文件未创建")
	}

	loaded, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth 失败: %v", err)
	}
	if loaded.Username != "testuser" {
		t.Errorf("用户名不匹配: 期望 testuser, 得到 %q", loaded.Username)
	}
	if !loaded.VerifyPassword("testpass123") {
		t.Error("加载后的密码验证失败")
	}
}

func TestLoadAuth_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	// 文件不存在时应使用默认值创建
	auth, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth（文件不存在）应自动创建: %v", err)
	}
	if auth.Username != DefaultUsername {
		t.Errorf("默认用户名应为 %q, 得到 %q", DefaultUsername, auth.Username)
	}
	if !auth.VerifyPassword(DefaultPassword) {
		t.Error("默认密码应验证通过")
	}

	// 验证文件已创建
	authPath := filepath.Join(dir, AuthFileName)
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		t.Fatal("auth.json 应被自动创建")
	}
}

func TestLoadAuth_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, AuthFileName)
	// 写入无效 JSON
	os.WriteFile(authPath, []byte("{invalid json}"), 0644)

	// 应使用默认值覆盖
	auth, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth（无效文件）应自动修复: %v", err)
	}
	if auth.Username != DefaultUsername {
		t.Errorf("默认用户名应为 %q", DefaultUsername)
	}
}

func TestLoadAuth_EmptyFields(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, AuthFileName)
	// 写入有效 JSON 但字段为空
	os.WriteFile(authPath, []byte(`{"username":"","salt":"","hash":""}`), 0644)

	// 应触发重新初始化
	auth, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth（空字段）应自动修复: %v", err)
	}
	if auth.Username != DefaultUsername {
		t.Errorf("应使用默认值: 期望 %q, 得到 %q", DefaultUsername, auth.Username)
	}
}

func TestLoadAuth_MissingHash(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, AuthFileName)
	os.WriteFile(authPath, []byte(`{"username":"admin","salt":"abc123","hash":""}`), 0644)

	auth, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth（缺少 hash）应自动修复: %v", err)
	}
	// 应被重新初始化为默认值
	if !auth.VerifyPassword(DefaultPassword) {
		t.Error("应使用默认密码")
	}
}

func TestSaveAuth_UpdatesFile(t *testing.T) {
	dir := t.TempDir()

	// 保存一次
	auth1 := NewAuthConfig("user1", "pass1")
	SaveAuth(dir, auth1)

	// 再次保存不同内容
	auth2 := NewAuthConfig("user2", "pass2")
	SaveAuth(dir, auth2)

	// 加载应得到最后保存的内容
	loaded, err := LoadAuth(dir)
	if err != nil {
		t.Fatalf("LoadAuth 失败: %v", err)
	}
	if loaded.Username != "user2" {
		t.Errorf("应加载最后保存的用户名: 期望 user2, 得到 %q", loaded.Username)
	}
	if !loaded.VerifyPassword("pass2") {
		t.Error("应验证最后保存的密码")
	}
}

// ===================== authFilePath =====================

func TestAuthFilePath(t *testing.T) {
	path := authFilePath("/some/dir")
	expected := filepath.Join("/some/dir", AuthFileName)
	if path != expected {
		t.Errorf("authFilePath 期望 %q, 得到 %q", expected, path)
	}
}
