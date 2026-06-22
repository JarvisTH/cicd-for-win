package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"ci-cd/internal/config"
	"ci-cd/internal/output"
	"ci-cd/internal/runner"
)

// JsonOutput 是否以 JSON 格式输出。由 main.go 通过 -j/--json 标记设置。
var JsonOutput bool

var CmdCheck = &cobra.Command{
	Use:   "check [project]",
	Short: "对项目执行代码检查（TypeScript 类型检查 + ESLint）",
	Long:  `对指定项目执行代码检查。不指定 project 则检查所有已启用项目。`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		projects := cfg.Filter(args)
		if len(projects) == 0 {
			return fmt.Errorf("没有找到匹配的项目")
		}
		results := runParallel(projects, func(p config.Project) (runner.Result, error) {
			return runner.RunCheck(p)
		})
		return output.Format(cmd, results, JsonOutput)
	},
}

var CmdBuild = &cobra.Command{
	Use:   "build [project]",
	Short: "对项目执行完整构建",
	Long:  `对指定项目执行完整构建。不指定 project 则构建所有项目。`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		projects := cfg.Filter(args)
		if len(projects) == 0 {
			return fmt.Errorf("没有找到匹配的项目")
		}
		results := runParallel(projects, func(p config.Project) (runner.Result, error) {
			return runner.RunBuild(p)
		})
		return output.Format(cmd, results, JsonOutput)
	},
}

var CmdTest = &cobra.Command{
	Use:   "test [project]",
	Short: "对项目执行单元测试",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		results := runParallel(cfg.Filter(args), func(p config.Project) (runner.Result, error) {
			return runner.RunTest(p)
		})
		return output.Format(cmd, results, JsonOutput)
	},
}

var CmdPush = &cobra.Command{
	Use:   "push [project]",
	Short: "推送到所有 Git 远程仓库",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunPush(p)
		}
		return nil
	},
}

var CmdDeploy = &cobra.Command{
	Use:   "deploy [project]",
	Short: "将项目部署到远程服务器",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		target, _ := cmd.Flags().GetString("target")
		for _, p := range cfg.Filter(args) {
			runner.RunDeploy(p, target)
		}
		return nil
	},
}

var CmdHooks = &cobra.Command{
	Use:   "hooks [project]",
	Short: "安装 Git hooks 到项目",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunHooks(p)
		}
		return nil
	},
}

var CmdList = &cobra.Command{
	Use:   "list",
	Short: "列出所有项目及状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if JsonOutput {
			data, _ := json.MarshalIndent(cfg.Projects, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		for _, p := range cfg.Projects {
			status := "⚪"
			if !p.Enabled {
				status = "🔘 禁用"
			}
			fmt.Printf("%s %-25s %s\n", status, p.Name, p.Path)
		}
		return nil
	},
}

var CmdStatus = &cobra.Command{
	Use:   "status [project]",
	Short: "查看项目当前状态",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		for _, p := range cfg.Filter(args) {
			runner.RunStatus(p)
		}
		return nil
	},
}

var CmdDescribe = &cobra.Command{
	Use:   "describe",
	Short: "输出工具 Schema（供 LLM/AI Agent 发现）",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		schema := runner.GenerateSchema(format)
		fmt.Println(schema)
		return nil
	},
}

// runParallel 对多个项目并行执行 fn，返回结果集合。
func runParallel(projects []config.Project, fn func(config.Project) (runner.Result, error)) []runner.Result {
	var wg sync.WaitGroup
	results := make([]runner.Result, len(projects))
	for i, p := range projects {
		wg.Add(1)
		go func(idx int, proj config.Project) {
			defer wg.Done()
			result, err := fn(proj)
			results[idx] = result
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ❌ %v\n", proj.Name, err)
			}
		}(i, p)
	}
	wg.Wait()
	return results
}

// loadConfig 加载项目配置，封装重复的 config.Load("projects.json") + 错误处理。
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load("projects.json")
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func init() {
	CmdDeploy.Flags().String("target", "production", "部署目标（staging/production）")
	CmdDescribe.Flags().String("format", "openai", "输出格式: openai/mcp/text")
	CmdReport.Flags().Bool("list", false, "列出所有历史报告")
	CmdReport.Flags().Bool("json", false, "输出 JSON 格式")
	CmdReport.Flags().String("delete", "", "删除指定 ID 的报告")
	CmdDoctor.Flags().Bool("json", false, "输出 JSON 格式")
	CmdProjectList.Flags().Bool("json", false, "输出 JSON 格式")
}
