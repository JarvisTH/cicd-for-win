// Package security 提供密码加解密、原子写入、路径校验等安全工具。
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	encPrefix = "enc:"    // 加密密码前缀，用于区分明文（向后兼容）
	keyFile   = ".secretkey" // 主密钥文件名
)

// LoadOrCreateKey 加载或创建 32 字节 AES 主密钥，文件权限 0600。
// 密钥以 hex 编码存盘，与密文分离，降低本机其他用户窃取后直接解密的风险。
func LoadOrCreateKey(ciDir string) ([]byte, error) {
	path := filepath.Join(ciDir, keyFile)
	if data, err := os.ReadFile(path); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(data))); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	// 创建新密钥
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成密钥失败: %w", err)
	}
	if err := AtomicWriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return key, nil
}

// EncryptPassword 用 AES-GCM 加密明文密码，返回 "enc:base64(nonce+ciphertext)"。
func EncryptPassword(plain string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword 解密 "enc:..." 格式密码；无前缀视为明文（向后兼容历史数据）。
func DecryptPassword(stored string, key []byte) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("密码 base64 解码失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文长度不足")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("密码解密失败: %w", err)
	}
	return string(plain), nil
}

// IsEncrypted 判断存储的密码是否为加密格式。
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, encPrefix)
}

// AtomicWriteFile 原子写入文件：先写临时文件再 rename，避免写一半崩溃损坏数据。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// IsPathSafe 检查 target 解析后是否位于 baseDir 之内（防路径穿越）。
// target 等于 baseDir 也视为安全。
func IsPathSafe(baseDir, target string) bool {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SanitizeFilename 校验文件名只含安全字符且不含路径分隔符/穿越符，返回清理后的名字。
// 用于限制只能访问指定目录下的具体文件。
func SanitizeFileName(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", false
	}
	return filepath.Clean(name), true
}
