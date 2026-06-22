package main

import (
	"os"

	"github.com/spf13/cobra"
	"ci-cd/internal/cmd"
)

var rootCmd = &cobra.Command{
	Use:   "ci",
	Short: "本地 CI/CD 命令行工具",
	Long: `本地 CI/CD 工具链，覆盖完整 CI/CD 流程：
  代码检查 → 测试 → 构建 → 推送 → 部署 → 状态监控`,
}

func main() {
	rootCmd.PersistentFlags().BoolVarP(&cmd.JsonOutput, "json", "j", false, "以 JSON 格式输出")
	rootCmd.AddCommand(
		cmd.CmdCheck,
		cmd.CmdTest,
		cmd.CmdBuild,
		cmd.CmdPush,
		cmd.CmdDeploy,
		cmd.CmdHooks,
		cmd.CmdList,
		cmd.CmdStatus,
		cmd.CmdDescribe,
		cmd.CmdPasswd,
		cmd.CmdReport,
		cmd.CmdDoctor,
		cmd.CmdProjectList,
		cmd.CmdServe,
		cmd.CmdRemote,
		cmd.CmdServer,
		cmd.CmdLog,
		cmd.CmdLocal,
		cmd.CmdRules,
	)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
