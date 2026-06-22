package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
	"ci-cd/internal/sshutil"
)

type cliServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	AuthType     string `json:"auth_type"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	Note         string `json:"note,omitempty"`
}

type cliServerList struct {
	Servers []cliServer `json:"servers"`
}

func loadServerList() *cliServerList {
	ciDir := ciDir()
	if ciDir == "" {
		return &cliServerList{Servers: []cliServer{}}
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "servers.json"))
	if err != nil {
		return &cliServerList{Servers: []cliServer{}}
	}
	var list cliServerList
	if err := json.Unmarshal(data, &list); err != nil {
		return &cliServerList{Servers: []cliServer{}}
	}
	if list.Servers == nil {
		list.Servers = []cliServer{}
	}
	migratePlaintextPasswords(ciDir, &list)
	return &list
}

func migratePlaintextPasswords(ciDir string, list *cliServerList) {
	var changed bool
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	for i := range list.Servers {
		s := &list.Servers[i]
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			enc, err := security.EncryptPassword(s.Password, key)
			if err == nil {
				list.Servers[i].Password = enc
				changed = true
			}
		}
	}
	if changed {
		saveServerList(list)
	}
}

func saveServerList(list *cliServerList) error {
	ciDir := ciDir()
	if ciDir == "" {
		return fmt.Errorf("找不到 ci-cd 目录")
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return security.AtomicWriteFile(filepath.Join(ciDir, "servers.json"), data, config.FilePermSecure)
}

func dialSSH(deploy *config.DeployConfig) (*ssh.Client, error) {
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir())
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	return ssh.Dial("tcp", addr, sshCfg)
}

func resolveDeploy(name, source string) *config.DeployConfig {
	if source == "standalone" {
		list := loadServerList()
		for _, s := range list.Servers {
			if s.Name == name {
				return &config.DeployConfig{
					Host: s.Host, Port: s.Port, User: s.User,
					AuthType: s.AuthType, IdentityFile: s.IdentityFile, Password: s.Password,
				}
			}
		}
		return nil
	}
	cfg, err := config.Load(filepath.Join(ciDir(), "projects.json"))
	if err != nil {
		return nil
	}
	for _, p := range cfg.Projects {
		if p.Name == name && p.Deploy != nil {
			return p.Deploy
		}
	}
	return nil
}

func queryAuditLogs(date, level, keyword string, limit int) []map[string]string {
	ciDir := ciDir()
	if ciDir == "" {
		return nil
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	logPath := filepath.Join(ciDir, "logs", fmt.Sprintf("audit-%s.jsonl", date))
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []map[string]string
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry struct {
			Time    string `json:"time"`
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if level != "" && entry.Level != level {
			continue
		}
		if keyword != "" && !strings.Contains(entry.Message, keyword) {
			continue
		}
		results = append(results, map[string]string{
			"time": entry.Time, "level": entry.Level, "message": entry.Message,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i]["time"] > results[j]["time"]
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}
