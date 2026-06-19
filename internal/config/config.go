package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Project struct {
	Name    string        `json:"name"`
	Path    string        `json:"path"`
	Enabled bool          `json:"enabled"`
	Deploy  *DeployConfig `json:"deploy,omitempty"`
	CiDir   string        `json:"-"` // 自动填充
}

type DeployConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	User      string `json:"user"`
	RemoteDir string `json:"remote_dir"`
	AuthType  string `json:"auth_type"`
}

type Config struct {
	Projects []Project `json:"projects"`
}

func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w\n  请执行 ci init wizard", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法获取 ci.exe 路径: %w", err)
	}
	ciDir := filepath.Dir(exePath)
	for i := range cfg.Projects {
		cfg.Projects[i].CiDir = ciDir
	}
	return &cfg, nil
}

func (c *Config) Filter(args []string) []Project {
	if len(args) == 0 {
		var enabled []Project
		for _, p := range c.Projects {
			if p.Enabled {
				enabled = append(enabled, p)
			}
		}
		return enabled
	}
	var matched []Project
	for _, name := range args {
		for _, p := range c.Projects {
			if p.Name == name && p.Enabled {
				matched = append(matched, p)
				break
			}
		}
	}
	return matched
}
