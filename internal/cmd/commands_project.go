package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

type projectDetail struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type,omitempty"`
	Version     string `json:"version,omitempty"`
	GitBranch   string `json:"git_branch,omitempty"`
	GitCommit   string `json:"git_commit,omitempty"`
	DeployHost  string `json:"deploy_host,omitempty"`
	HasDist     bool   `json:"has_dist"`
	RemoteCount int    `json:"remote_count"`
}

// CmdProjectList 增强的项目列表
var CmdProjectList = &cobra.Command{
	Use:   "project list",
	Short: "列出所有项目的详细信息",
	Long: `列出所有项目的详细信息，包括路径、类型、构建状态、部署配置和 Git 信息。
	示例:
	  ci project list             列出所有项目详情
	  ci project list --json      输出 JSON 格式
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("读取项目配置失败: %w", err)
		}
		details := buildProjectDetails(cfg)
		return printProjectList(details, jsonMode)
	},
}

// buildProjectDetails 从配置构建项目详情列表。可单独测试。
func buildProjectDetails(cfg *config.Config) []projectDetail {
	var details []projectDetail
	for _, p := range cfg.Projects {
		d := projectDetail{
			Name:    p.Name,
			Path:    p.Path,
			Enabled: p.Enabled,
		}
		if p.Deploy != nil {
			d.DeployHost = p.Deploy.Host
		}
		if p.Enabled {
			d.HasDist = runner.HasDist(p.Path)
			d.Version = runner.ReadProjectVersion(p.Path)
			branch, commit := runner.ReadGitInfo(p.Path)
			d.GitBranch = branch
			d.GitCommit = commit
		}
		details = append(details, d)
	}
	return details
}

// printProjectList 输出项目列表。可单独测试。
func printProjectList(details []projectDetail, jsonMode bool) error {
	if jsonMode {
		data, _ := json.MarshalIndent(details, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%-20s %-8s %-12s %s\n", "项目", "状态", "版本", "部署目标")
	fmt.Println(strings.Repeat("─", 80))
	for _, d := range details {
		status := "🔘 禁用"
		if d.Enabled {
			status = "✅ 启用"
		}
		deployHost := d.DeployHost
		if deployHost == "" {
			deployHost = "-"
		}
		ver := d.Version
		if ver == "" {
			ver = "-"
		}
		fmt.Printf("%-20s %-8s %-12s %s\n", d.Name, status, ver, deployHost)
	}
	return nil
}
