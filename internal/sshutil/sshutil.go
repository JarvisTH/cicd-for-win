// Package sshutil 提供 SSH 连接的公共逻辑：known_hosts 主机密钥校验（TOFU 策略）
// 与 SSH 客户端配置构建，供 serve 与 cmd 包复用，消除重复实现。
package sshutil

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
)

const knownHostsFile = ".known_hosts"

// KnownHostsPath 返回 ci-cd 目录下的 known_hosts 文件路径。
func KnownHostsPath(ciDir string) string {
	return filepath.Join(ciDir, knownHostsFile)
}

// tofuHostKeyCallback 实现 trust-on-first-use 策略：
//   - 已知主机：严格校验，密钥不匹配则拒绝（防中间人攻击）
//   - 未知主机：自动接受并追加到 known_hosts 文件
//
// 这是本地内网工具的合理折中：首次使用便捷，后续安全。
var tofuMu sync.Mutex // 追加写入 known_hosts 时串行化，避免并发写损坏

func TOFUHostKeyCallback(ciDir string) ssh.HostKeyCallback {
	khPath := KnownHostsPath(ciDir)
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// 文件不存在时，knownhosts.New 会返回 error，视为首次
		if _, err := os.Stat(khPath); err != nil {
			return appendKnownHost(khPath, hostname, key)
		}
		cb, err := knownhosts.New(khPath)
		if err != nil {
			return appendKnownHost(khPath, hostname, key)
		}
		if err := cb(hostname, remote, key); err != nil {
			// 区分"未知主机"（Want 为空）与"密钥不匹配"（Want 非空）
			var kerr *knownhosts.KeyError
			if errors.As(err, &kerr) && len(kerr.Want) == 0 {
				return appendKnownHost(khPath, hostname, key)
			}
			// 密钥不匹配，拒绝连接
			return fmt.Errorf("主机 %s 密钥校验失败（可能遭受中间人攻击）: %w", hostname, err)
		}
		return nil
	}
}

// appendKnownHost 将主机公钥追加到 known_hosts 文件。
func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	tofuMu.Lock()
	defer tofuMu.Unlock()
	line := knownhosts.Line([]string{hostname}, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("打开 known_hosts 失败: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("写入 known_hosts 失败: %w", err)
	}
	return nil
}

// BuildSSHConfig 根据 DeployConfig 构建 SSH 客户端配置。
// 自动解密 projects.json/servers.json 中存储的加密密码。
// ciDir 用于定位密钥文件与 known_hosts。
func BuildSSHConfig(d *config.DeployConfig, ciDir string) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:            d.User,
		HostKeyCallback: TOFUHostKeyCallback(ciDir),
		Timeout:         10 * time.Second,
	}

	// 1. SSH 密钥认证
	if d.AuthType == "key" && d.IdentityFile != "" {
		keyData, err := os.ReadFile(d.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("读取密钥文件失败 %s: %w", d.IdentityFile, err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("解析密钥文件失败 %s: %w", d.IdentityFile, err)
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
		return cfg, nil
	}

	// 2. 密码认证（自动解密）
	if d.AuthType == "password" && d.Password != "" {
		key, err := security.LoadOrCreateKey(ciDir)
		if err != nil {
			return nil, err
		}
		plain, err := security.DecryptPassword(d.Password, key)
		if err != nil {
			return nil, fmt.Errorf("密码解密失败: %w", err)
		}
		cfg.Auth = append(cfg.Auth, ssh.Password(plain))
	}

	// 3. SSH Agent 兜底（Windows openssh-ssh-agent 命名管道 / Unix socket）
	if agentConn, err := dialAgent(); err == nil {
		agentClient := agent.NewClient(agentConn)
		cfg.Auth = append(cfg.Auth, ssh.PublicKeysCallback(agentClient.Signers))
	}
	return cfg, nil
}

// dialAgent 尝试连接本机 SSH Agent，返回连接。
func dialAgent() (net.Conn, error) {
	// Windows: openssh-ssh-agent 命名管道
	if conn, err := net.Dial("pipe", `\\.\pipe\openssh-ssh-agent`); err == nil {
		return conn, nil
	}
	// Unix: SSH_AUTH_SOCK
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		return net.Dial("unix", sock)
	}
	return nil, errors.New("无可用 SSH Agent")
}
