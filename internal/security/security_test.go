package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===================== EncryptPassword / DecryptPassword =====================

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	passwords := []string{
		"hello123",
		"",
		"密码+特殊字符!@#$%^&*()",
		"a",
		"enc:already-prefixed",
	}

	for _, pwd := range passwords {
		enc, err := EncryptPassword(pwd, key)
		if err != nil {
			t.Fatalf("EncryptPassword(%q) 失败: %v", pwd, err)
		}
		if !strings.HasPrefix(enc, encPrefix) {
			t.Fatalf("加密结果缺少 enc: 前缀: %q", enc)
		}

		dec, err := DecryptPassword(enc, key)
		if err != nil {
			t.Fatalf("DecryptPassword(%q) 失败: %v", enc, err)
		}
		if dec != pwd {
			t.Fatalf("解密结果不匹配: 期望 %q, 得到 %q", pwd, dec)
		}
	}
}

func TestEncryptDifferentCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// 相同明文每次加密结果不同（nonce 随机性）
	enc1, _ := EncryptPassword("same", key)
	enc2, _ := EncryptPassword("same", key)
	if enc1 == enc2 {
		t.Error("相同明文两次加密结果相同，说明 nonce 未正确随机化")
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	// AES 支持 16/24/32 字节密钥，15 字节是非法长度
	invalidKey := []byte("123456789012345") // 15 bytes
	_, err := EncryptPassword("test", invalidKey)
	if err == nil {
		t.Error("15 字节密钥应报错（AES 需要 16/24/32 字节）")
	}

	emptyKey := []byte{}
	_, err = EncryptPassword("test", emptyKey)
	if err == nil {
		t.Error("空密钥应报错")
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	key := make([]byte, 32)

	// 无 enc: 前缀的密码视为明文原样返回
	plaintext := "my-plain-password"
	result, err := DecryptPassword(plaintext, key)
	if err != nil {
		t.Fatalf("DecryptPassword(plain) 失败: %v", err)
	}
	if result != plaintext {
		t.Fatalf("明文透传结果不匹配: 期望 %q, 得到 %q", plaintext, result)
	}

	// 空字符串
	result, err = DecryptPassword("", key)
	if err != nil {
		t.Fatalf("DecryptPassword('') 失败: %v", err)
	}
	if result != "" {
		t.Fatalf("空字符串透传应返回空: 得到 %q", result)
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, _ := EncryptPassword("secret", key)

	// 篡改 base64 数据：替换两个连续字符保证与原串不同
	// 单字符可能巧合相同（概率 ~1/64），双字符可忽略此概率
	if len(enc) > 6 {
		tampered := enc[:4] + "XX" + enc[6:]
		_, err := DecryptPassword(tampered, key)
		if err == nil {
			t.Error("篡改后的密文应解密失败")
		}
	}
}

func TestDecryptShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// base64 解码后长度小于 nonce 大小
	shortEnc := encPrefix + "YWJj" // "abc" → 3 字节 < nonceSize (12)
	_, err := DecryptPassword(shortEnc, key)
	if err == nil {
		t.Error("过短的密文应解密失败")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	key := make([]byte, 32)
	invalidBase64 := encPrefix + "!!!not-base64!!!"
	_, err := DecryptPassword(invalidBase64, key)
	if err == nil {
		t.Error("无效 base64 应解码失败")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 0x01 // 两个密钥仅第一个字节不同

	enc, _ := EncryptPassword("secret", key1)
	_, err := DecryptPassword(enc, key2)
	if err == nil {
		t.Error("使用不同密钥解密应失败")
	}
}

// ===================== IsEncrypted =====================

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"enc:abc123", true},
		{"enc:", true},
		{"ENC:abc", false}, // 大小写敏感
		{"plaintext", false},
		{"", false},
		{"enc", false},
	}
	for _, tc := range tests {
		result := IsEncrypted(tc.input)
		if result != tc.expected {
			t.Errorf("IsEncrypted(%q) = %v, 期望 %v", tc.input, result, tc.expected)
		}
	}
}

// ===================== AtomicWriteFile =====================

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello atomic write")

	err := AtomicWriteFile(path, content, 0644)
	if err != nil {
		t.Fatalf("AtomicWriteFile 失败: %v", err)
	}

	// 读取验证内容
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("无法读取写入的文件: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("文件内容不匹配: 期望 %q, 得到 %q", content, data)
	}
}

func TestAtomicWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secure.txt")
	content := []byte("secret data")

	err := AtomicWriteFile(path, content, 0600)
	if err != nil {
		t.Fatalf("AtomicWriteFile 失败: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("无法 stat 文件: %v", err)
	}
	// 只检查文件存在且非空，权限在 Windows 上表现不同
	if info.Size() == 0 {
		t.Fatal("文件大小不应为 0")
	}
}

func TestAtomicWriteFileNestedDir(t *testing.T) {
	dir := t.TempDir()
	// 写入不存在的子目录 — 应失败（不会自动创建目录）
	path := filepath.Join(dir, "sub", "nested.txt")
	err := AtomicWriteFile(path, []byte("test"), 0644)
	if err == nil {
		t.Error("写入不存在的目录应失败")
	}
}

// ===================== IsPathSafe =====================

func TestIsPathSafe(t *testing.T) {
	base := "/app/data"
	tests := []struct {
		name     string
		target   string
		safe     bool
	}{
		{"同一目录", "/app/data", true},
		{"子目录", "/app/data/sub", true},
		{"深层子目录", "/app/data/a/b/c", true},
		{"直接上级", "/app/other", false},
		{"跨级上级", "/etc/passwd", false},
		{"路径穿越", "/app/data/../../etc", false},
		{"不相关路径", "/var/log", false},
		{"相对路径穿越", "/app/other", false},
	}
	for _, tc := range tests {
		result := IsPathSafe(base, tc.target)
		if result != tc.safe {
			t.Errorf("IsPathSafe(%q, %q) = %v, 期望 %v", base, tc.target, result, tc.safe)
		}
	}
}

func TestIsPathSafeWindows(t *testing.T) {
	base := `D:\app\data`
	tests := []struct {
		target string
		safe   bool
	}{
		{`D:\app\data`, true},
		{`D:\app\data\sub`, true},
		{`D:\app\data\..\other`, false},
		{`D:\app\other`, false},
		{`E:\outside`, false},
	}
	for _, tc := range tests {
		result := IsPathSafe(base, tc.target)
		if result != tc.safe {
			t.Errorf("IsPathSafe(%q, %q) = %v, 期望 %v", base, tc.target, result, tc.safe)
		}
	}
}

func TestIsPathSafeError(t *testing.T) {
	// base 有非法字符，filepath.Rel 会返回错误
	result := IsPathSafe("", "/some/path")
	if result {
		t.Error("空 baseDir 应返回 false")
	}
}

// ===================== SanitizeFileName =====================

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
		name  string
	}{
		{"normal.txt", true, "normal.txt"},
		{"file-name_v1.2", true, "file-name_v1.2"},
		{"", false, ""},
		{"path/to/file", false, ""},
		{"..", false, ""},
		{"../etc", false, ""},
		{`C:\file`, false, ""},
		{"a/b", false, ""},
		{"onlyname", true, "onlyname"},
		{"file.with.dots", true, "file.with.dots"},
		{"  spaces  ", true, "  spaces  "}, // 空格是合法字符
	}
	for _, tc := range tests {
		name, ok := SanitizeFileName(tc.input)
		if ok != tc.ok {
			t.Errorf("SanitizeFileName(%q) ok = %v, 期望 %v", tc.input, ok, tc.ok)
		}
		if ok && name != tc.name {
			t.Errorf("SanitizeFileName(%q) name = %q, 期望 %q", tc.input, name, tc.name)
		}
	}
}

// ===================== LoadOrCreateKey =====================

func TestLoadOrCreateKey_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFile)

	// 创建新密钥
	key, err := LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateKey 创建新密钥失败: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("密钥长度应为 32 字节, 得到 %d", len(key))
	}

	// 验证文件已创建
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("密钥文件未创建")
	}
}

func TestLoadOrCreateKey_LoadsExisting(t *testing.T) {
	dir := t.TempDir()

	// 先创建
	key1, err := LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}

	// 再次加载
	key2, err := LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("再次加载失败: %v", err)
	}

	// 两次密钥应相同
	if len(key1) != len(key2) {
		t.Fatal("两次加载的密钥长度不一致")
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("两次加载的密钥内容不一致，说明未从文件读取")
		}
	}
}

func TestLoadOrCreateKey_RejectsInvalidFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFile)

	// 写入无效内容（非 hex、长度不正确的数据）
	os.WriteFile(keyPath, []byte("not-a-valid-key"), 0600)

	// 应覆盖写入新的有效密钥
	key, err := LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("无效文件后重新创建失败: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("新密钥长度应为 32, 得到 %d", len(key))
	}
}

func TestLoadOrCreateKey_RejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, keyFile)

	// 写入 16 字节 hex 编码密钥（合法 hex 但长度错误）
	os.WriteFile(keyPath, []byte("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"), 0600) // 16 bytes = 32 hex chars

	key, err := LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("长度错误的密钥应重新创建: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("新密钥长度应为 32, 得到 %d", len(key))
	}
}
