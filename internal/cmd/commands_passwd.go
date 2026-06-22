package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"ci-cd/internal/config"
)

var CmdPasswd = &cobra.Command{
	Use:   "passwd [username] [password]",
	Short: "修改或重置 Web UI 登录密码",
	Long: `修改 Web UI 的 Basic Auth 登录密码。

	不传参数时重置为默认密码 (admin/123456)。
	示例:
	  ci passwd                   重置为默认 admin/123456
	  ci passwd admin myNewPass   修改密码
	`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("无法获取 ci.exe 路径: %w", err)
		}
		ciDir := filepath.Dir(exe)

		username := config.DefaultUsername
		password := config.DefaultPassword

		if len(args) >= 1 {
			username = args[0]
		}
		if len(args) >= 2 {
			password = args[1]
		}

		if len(password) < 6 {
			return fmt.Errorf("密码长度不能少于 6 位")
		}

		auth := config.NewAuthConfig(username, password)
		if err := config.SaveAuth(ciDir, auth); err != nil {
			return fmt.Errorf("保存密码失败: %w", err)
		}

		fmt.Printf("✅ 密码已更新 — 用户名: %s  密码: %s\n", username, password)
		fmt.Printf("   文件: %s\n", filepath.Join(ciDir, config.AuthFileName))
		if password == config.DefaultPassword {
			fmt.Println("⚠️  警告: 正在使用默认密码，建议通过 `ci passwd <用户名> <密码>` 修改")
		}
		return nil
	},
}
