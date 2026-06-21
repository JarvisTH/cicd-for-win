package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Project struct {
	Name         string          `json:"name"`
	Path         string          `json:"path"`
	Enabled      bool            `json:"enabled"`
	Deploy       *DeployConfig   `json:"deploy,omitempty"`
	Pipeline     *PipelineConfig `json:"pipeline,omitempty"`
	Rules        []RuleState     `json:"rules,omitempty"`         // 代码检查规则的单条启用/禁用状态
	Remotes      []RemoteConfig  `json:"remotes,omitempty"`       // Git 远程仓库配置
	DeployTarget string          `json:"deployTarget,omitempty"`  // 部署目标（production/staging）
	GitBranch    string          `json:"gitBranch,omitempty"`     // 项目指定操作的 Git 分支
	CiDir        string          `json:"-"`                       // 自动填充
}

// RemoteConfig Git 远程仓库配置
type RemoteConfig struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// RuleState 描述单条代码检查规则的启用状态。
// ID 与 ci-runner.ps1 中识别的规则 id 对应（如 tsc/eslint/compile/checkstyle）。
type RuleState struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// IsRuleEnabled 返回指定规则是否启用。
// 项目未配置任何规则（Rules 为空）时，所有规则默认启用，保持向后兼容。
func (p *Project) IsRuleEnabled(id string) bool {
	if len(p.Rules) == 0 {
		return true
	}
	for _, r := range p.Rules {
		if r.ID == id {
			return r.Enabled
		}
	}
	return true // 未显式列出的规则默认启用
}

// PipelineConfig 定义项目的自定义流水线步骤
// Steps 为空时使用默认顺序: check → build → test → push → deploy
type PipelineConfig struct {
	Steps []PipelineStep `json:"steps,omitempty"`
}

// PipelineStep 流水线中的单个步骤
type PipelineStep struct {
	ID      string `json:"id"`       // check/build/test/push/deploy
	Enabled bool   `json:"enabled"`  // 是否启用
	Command string `json:"command,omitempty"` // 自定义命令（为空时使用默认命令）
	Args    string `json:"args,omitempty"`    // 自定义参数/额外参数
}

// StepDefaultCommands 每个步骤的默认命令描述（仅供参考，实际执行由 ci-runner.ps1 处理）
var StepDefaultCommands = map[string]string{
	"check":  "自动检测（tsc/eslint/mvn compile）",
	"build":  "自动检测（npm run build / mvn package）",
	"test":   "自动检测（jest/vitest/mvn test）",
	"push":   "git push（推送所有远程仓库）",
	"deploy": "SFTP 上传 + 远程重启",
}

// DefaultPipelineSteps 返回默认的流水线步骤配置
func DefaultPipelineSteps() []PipelineStep {
	return []PipelineStep{
		{ID: "check", Enabled: true},
		{ID: "build", Enabled: true},
		{ID: "test", Enabled: true},
		{ID: "push", Enabled: true},
		{ID: "deploy", Enabled: true},
	}
}

// GetEnabledSteps 返回项目启用的流水线步骤（按配置顺序）
// 如果项目没有 Pipeline 配置，返回默认步骤
func (p *Project) GetEnabledSteps() []string {
	defaults := DefaultPipelineSteps()
	if p.Pipeline == nil || len(p.Pipeline.Steps) == 0 {
		var result []string
		for _, s := range defaults {
			if s.Enabled {
				result = append(result, s.ID)
			}
		}
		return result
	}
	var result []string
	for _, s := range p.Pipeline.Steps {
		if s.Enabled {
			result = append(result, s.ID)
		}
	}
	return result
}

type DeployConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	RemoteDir    string `json:"remote_dir"`
	AuthType     string `json:"auth_type"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	StartCmd     string `json:"start_cmd,omitempty"`   // 自定义启动命令（为空则自动推断）
	StopCmd      string `json:"stop_cmd,omitempty"`    // 自定义停止命令（为空则自动推断）
	StatusCmd    string `json:"status_cmd,omitempty"`  // 自定义状态查询命令（为空则自动推断）
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
		// 规范化部署配置：端口默认 22，认证类型默认值
		if cfg.Projects[i].Deploy != nil {
			cfg.Projects[i].Deploy.normalize()
		}
	}
	return &cfg, nil
}

// normalize 规范化部署配置：补全端口默认值，校正认证类型。
func (d *DeployConfig) normalize() {
	if d.Port == 0 {
		d.Port = 22
	}
	if d.AuthType == "" {
		d.AuthType = "key"
	}
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
