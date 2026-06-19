package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultUsername = "admin"
	DefaultPassword = "123456"
	AuthFileName    = "auth.json"
)

type AuthConfig struct {
	Username string `json:"username"`
	Salt     string `json:"salt"`
	Hash     string `json:"hash"`
}

// authFilePath 返回 auth.json 的完整路径
func authFilePath(ciDir string) string {
	return filepath.Join(ciDir, AuthFileName)
}

// LoadAuth 从 auth.json 读取认证信息；文件不存在则用默认值创建并保存
func LoadAuth(ciDir string) (*AuthConfig, error) {
	path := authFilePath(ciDir)
	data, err := os.ReadFile(path)
	if err == nil {
		var auth AuthConfig
		if err := json.Unmarshal(data, &auth); err == nil && auth.Username != "" && auth.Hash != "" {
			return &auth, nil
		}
	}

	// 文件不存在或格式错误，用默认值初始化
	auth := NewAuthConfig(DefaultUsername, DefaultPassword)
	if err := SaveAuth(ciDir, auth); err != nil {
		return nil, fmt.Errorf("初始化 auth.json 失败: %w", err)
	}
	return auth, nil
}

// SaveAuth 将认证信息写入 auth.json
func SaveAuth(ciDir string, auth *AuthConfig) error {
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(authFilePath(ciDir), data, 0644)
}

// NewAuthConfig 根据用户名和明文密码生成带盐的 AuthConfig
func NewAuthConfig(username, password string) *AuthConfig {
	salt := generateSalt()
	hash := hashPassword(salt, password)
	return &AuthConfig{
		Username: username,
		Salt:     salt,
		Hash:     hash,
	}
}

// VerifyPassword 验证明文密码是否匹配
func (a *AuthConfig) VerifyPassword(password string) bool {
	return a.Hash == hashPassword(a.Salt, password)
}

// hashPassword 使用 SHA256(salt + password) 计算密码哈希
func hashPassword(salt, password string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// generateSalt 生成 16 字节随机盐，返回 base64 编码
func generateSalt() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 极端情况：随机数生成失败，用固定值保底（理论上不会发生）
		return "default-salt-fallback"
	}
	return base64.StdEncoding.EncodeToString(b)
}
