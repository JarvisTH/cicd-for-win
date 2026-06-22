package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/sshutil"
)

// ========== 一次性下载 token 机制 ==========
var (
	downloadTokens   = map[string]time.Time{}
	downloadTokensMu sync.Mutex
)

const downloadTokenTTL = 60 * time.Second

func generateDownloadToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	downloadTokensMu.Lock()
	now := time.Now()
	for k, exp := range downloadTokens {
		if now.After(exp) {
			delete(downloadTokens, k)
		}
	}
	downloadTokens[token] = time.Now().Add(downloadTokenTTL)
	downloadTokensMu.Unlock()
	return token
}

func validateDownloadToken(token string) bool {
	if token == "" {
		return false
	}
	downloadTokensMu.Lock()
	defer downloadTokensMu.Unlock()
	exp, ok := downloadTokens[token]
	if !ok || time.Now().After(exp) {
		delete(downloadTokens, token)
		return false
	}
	delete(downloadTokens, token)
	return true
}

// ========== SSH 连接池 ==========
type cachedClient struct {
	client   *ssh.Client
	lastUsed time.Time
}

const sshIdleTimeout = 30 * time.Minute

var (
	sshClients = map[string]*cachedClient{}
	sshMu      sync.Mutex
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func startSSHReaper() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			reapIdleSSHClients()
		}
	}()
}

func reapIdleSSHClients() {
	sshMu.Lock()
	defer sshMu.Unlock()
	now := time.Now()
	for key, c := range sshClients {
		if now.Sub(c.lastUsed) > sshIdleTimeout {
			if _, _, err := c.client.SendRequest("keepalive@golang.org", true, nil); err != nil {
				c.client.Close()
				delete(sshClients, key)
				log.Printf("🔌 回收空闲/失效 SSH 连接: %s\n", key)
			} else {
				c.lastUsed = now
			}
		}
	}
}

func CloseAllSSHClients() {
	sshMu.Lock()
	defer sshMu.Unlock()
	for key, c := range sshClients {
		c.client.Close()
		delete(sshClients, key)
	}
	log.Printf("🔌 已关闭所有 SSH 连接\n")
}

func getSSHClient(projectName string, deploy *config.DeployConfig) (*ssh.Client, error) {
	if deploy == nil || deploy.Host == "" {
		return nil, fmt.Errorf("项目 %s 未配置部署信息", projectName)
	}
	ciDir := findCiDir()

	sshMu.Lock()
	if c, ok := sshClients[projectName]; ok {
		if _, _, err := c.client.SendRequest("keepalive@golang.org", true, nil); err == nil {
			c.lastUsed = time.Now()
			sshMu.Unlock()
			return c.client, nil
		}
		c.client.Close()
		delete(sshClients, projectName)
	}
	sshMu.Unlock()

	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败 %s: %w", addr, err)
	}

	sshMu.Lock()
	defer sshMu.Unlock()
	if c, ok := sshClients[projectName]; ok {
		client.Close()
		c.lastUsed = time.Now()
		return c.client, nil
	}
	sshClients[projectName] = &cachedClient{client: client, lastUsed: time.Now()}
	return client, nil
}
