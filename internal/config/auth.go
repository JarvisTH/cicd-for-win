package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultUsername = "admin"
	DefaultPassword = "123456"
	AuthFileName    = "auth.json"
)

type AuthConfig struct {
	Username string `json:"username"`
	Hash     string `json:"hash"`
	// Salt 保留用于向后兼容（旧 SHA256 格式），新生成的文件不再使用
	Salt string `json:"salt,omitempty"`
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

// NewAuthConfig 根据用户名和明文密码生成 AuthConfig（使用 bcrypt 加密）
func NewAuthConfig(username, password string) *AuthConfig {
	hash := hashPassword(password)
	return &AuthConfig{
		Username: username,
		Hash:     hash,
	}
}

// VerifyPassword 验证明文密码是否匹配（支持 bcrypt 和旧版 SHA256）
func (a *AuthConfig) VerifyPassword(password string) bool {
	// 尝试 bcrypt 验证
	if err := bcrypt.CompareHashAndPassword([]byte(a.Hash), []byte(password)); err == nil {
		return true
	}
	// 向后兼容：旧版 SHA256(salt + password)
	if a.Salt != "" {
		return a.Hash == oldHashPassword(a.Salt, password)
	}
	return false
}

// hashPassword 使用 bcrypt 计算密码哈希
func hashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return password // 极端情况，保底
	}
	return string(hash)
}

// oldHashPassword 旧版 SHA256 哈希（向后兼容）
func oldHashPassword(salt, password string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}
