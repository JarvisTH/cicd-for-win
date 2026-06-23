package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

var CmdWatch = &cobra.Command{
	Use:   "watch [project]",
	Short: "监听项目文件变更，自动执行代码检查",
	Long: `监听指定项目的源文件变更，检测到修改后自动执行代码检查。
	不指定 project 则监听所有已启用项目。`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		projects := cfg.Filter(args)
		if len(projects) == 0 {
			return fmt.Errorf("没有找到匹配的项目")
		}

		// 如果有多个项目，分别启动监听（goroutine 中）
		if len(projects) > 1 {
			fmt.Printf("👀 监听 %d 个项目，按 Ctrl+C 停止\n", len(projects))
		}
		for _, p := range projects {
			ciDir := filepath.Dir(".")
			projectType := runner.DetectProjectType(p.Path)
			go runner.WatchProject(p.Path, projectType, parseRuleStatesFromProject(p), ciDir)
		}
		// 阻塞主 goroutine
		select {}
	},
}

// parseRuleStatesFromProject 从项目配置解析规则状态。
func parseRuleStatesFromProject(p config.Project) map[string]bool {
	states := make(map[string]bool)
	for _, r := range p.Rules {
		states[r.ID] = r.Enabled
	}
	return states
}
