package sshutil

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
)

// ===================== KnownHostsPath =====================

func TestKnownHostsPath(t *testing.T) {
	path := KnownHostsPath("/ci-cd")
	expected := filepath.Join("/ci-cd", ".known_hosts")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestKnownHostsPath_Relative(t *testing.T) {
	path := KnownHostsPath(".")
	expected := filepath.Join(".", ".known_hosts")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestKnownHostsPath_Empty(t *testing.T) {
	path := KnownHostsPath("")
	expected := filepath.Join("", ".known_hosts")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// ===================== appendKnownHost =====================

func TestAppendKnownHost_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, ".known_hosts")

	// Create a test key
	key := generateTestKey(t)

	err := appendKnownHost(khPath, "test-host", key)
	if err != nil {
		t.Fatalf("appendKnownHost failed: %v", err)
	}

	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	if len(data) == 0 {
		t.Error("known_hosts should not be empty after appending")
	}
}

func TestAppendKnownHost_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, ".known_hosts")

	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	err := appendKnownHost(khPath, "host1", key1)
	if err != nil {
		t.Fatalf("first append failed: %v", err)
	}
	err = appendKnownHost(khPath, "host2", key2)
	if err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	data, err := os.ReadFile(khPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}
}

// ===================== TOFUHostKeyCallback =====================

func TestTOFUHostKeyCallback_FirstTime(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, ".known_hosts")

	cb := TOFUHostKeyCallback(dir)
	key := generateTestKey(t)

	// First connection to unknown host should succeed (TOFU)
	err := cb("new-host", nil, key)
	if err != nil {
		t.Fatalf("first connection to unknown host should be accepted: %v", err)
	}

	// Host should now be in known_hosts
	if _, err := os.Stat(khPath); os.IsNotExist(err) {
		t.Error("known_hosts should have been created")
	}
}

// ===================== BuildSSHConfig =====================

func TestBuildSSHConfig_KeyAuth(t *testing.T) {
	dir := t.TempDir()

	// Create a test SSH key
	keyPath := filepath.Join(dir, "test_key")
	writeFile(t, keyPath, testPrivateKey)

	d := &config.DeployConfig{
		Host:         "example.com",
		Port:         22,
		User:         "testuser",
		AuthType:     "key",
		IdentityFile: keyPath,
	}

	cfg, err := BuildSSHConfig(d, dir)
	if err != nil {
		t.Fatalf("BuildSSHConfig failed: %v", err)
	}

	if cfg.User != "testuser" {
		t.Errorf("expected user testuser, got %s", cfg.User)
	}
	if cfg.Timeout == 0 {
		t.Error("timeout should be set")
	}
}

func TestBuildSSHConfig_KeyAuth_MissingFile(t *testing.T) {
	dir := t.TempDir()

	d := &config.DeployConfig{
		Host:         "example.com",
		Port:         22,
		User:         "testuser",
		AuthType:     "key",
		IdentityFile: filepath.Join(dir, "nonexistent_key"),
	}

	_, err := BuildSSHConfig(d, dir)
	if err == nil {
		t.Fatal("missing key file should return error")
	}
}

func TestBuildSSHConfig_PasswordAuth_Encrypted(t *testing.T) {
	dir := t.TempDir()

	// Create encryption key
	key, err := security.LoadOrCreateKey(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateKey failed: %v", err)
	}

	// Encrypt a password
	encPassword, err := security.EncryptPassword("my-secret-password", key)
	if err != nil {
		t.Fatalf("EncryptPassword failed: %v", err)
	}

	d := &config.DeployConfig{
		Host:     "example.com",
		Port:     22,
		User:     "testuser",
		AuthType: "password",
		Password: encPassword,
	}

	cfg, err := BuildSSHConfig(d, dir)
	if err != nil {
		t.Fatalf("BuildSSHConfig with encrypted password failed: %v", err)
	}

	if cfg.User != "testuser" {
		t.Errorf("expected user testuser, got %s", cfg.User)
	}
}

func TestBuildSSHConfig_NoAuth(t *testing.T) {
	dir := t.TempDir()

	d := &config.DeployConfig{
		Host: "example.com",
		Port: 22,
		User: "testuser",
	}

	cfg, err := BuildSSHConfig(d, dir)
	if err != nil {
		t.Fatalf("BuildSSHConfig with no auth should not error: %v", err)
	}

	if len(cfg.Auth) != 0 {
		t.Logf("auth methods: %d (agent fallback may add entries)", len(cfg.Auth))
	}
}

// ===================== Helpers =====================

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// generateTestKey generates a test SSH key pair for testing.
func generateTestKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	// Use the test private key to parse a public key
	signer, err := ssh.ParsePrivateKey([]byte(testPrivateKey))
	if err != nil {
		t.Fatalf("failed to parse test key: %v", err)
	}
	return signer.PublicKey()
}

// testPrivateKey is a pre-generated ED25519 private key for testing only.
const testPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACD8PywGeln50fHax/a9PrIJjShEeo2ukYIGG7c8WFfwZQAA
AIh8Nr4EfDa+BAAAAAtzc2gtZWQyNTUxOQAAACD8PywGeln50fHax/a9PrIJjShE
eo2ukYIGG7c8WFfwZQAAAEDPAg0HtMmiyD9Wb4G0eSCidcIXkvx4bLBjxiUx5UiZ
0/w/LAZ6WfnR8drH9r0+sgmNKER6ja6RggYbtzxYV/BlAAAAAAECAwQF
-----END OPENSSH PRIVATE KEY-----`
