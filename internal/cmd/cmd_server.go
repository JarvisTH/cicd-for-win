package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"ci-cd/internal/security"
)

// --- server 子命令 ---

var CmdServer = &cobra.Command{
	Use:   "server <command>",
	Short: "管理独立服务器",
}

var CmdServerList = &cobra.Command{
	Use:   "list",
	Short: "列出所有独立服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		list := loadServerList()
		if len(list.Servers) == 0 {
			fmt.Println("📭 无独立服务器，请通过 Web UI 或 `ci server add` 添加")
			return nil
		}
		fmt.Println("🖥️  独立服务器列表")
		fmt.Println(strings.Repeat("─", 70))
		fmt.Printf("%-20s %-20s %-8s %s\n", "名称", "主机", "端口", "认证方式")
		fmt.Println(strings.Repeat("─", 70))
		for _, s := range list.Servers {
			fmt.Printf("%-20s %-20s %-8d %s\n", s.Name, s.Host, s.Port, s.AuthType)
		}
		return nil
	},
}

var CmdServerAdd = &cobra.Command{
	Use:   "add",
	Short: "添加独立服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		host, _ := cmd.Flags().GetString("host")
		user, _ := cmd.Flags().GetString("user")
		port, _ := cmd.Flags().GetInt("port")
		authType, _ := cmd.Flags().GetString("auth-type")
		keyPath, _ := cmd.Flags().GetString("key-path")
		password, _ := cmd.Flags().GetString("password")
		note, _ := cmd.Flags().GetString("note")

		if name == "" || host == "" || user == "" {
			return fmt.Errorf("--name、--host、--user 为必填")
		}
		if port == 0 {
			port = 22
		}
		if authType == "password" && password != "" {
			key, err := security.LoadOrCreateKey(ciDir())
			if err != nil {
				return fmt.Errorf("初始化密钥失败: %w", err)
			}
			enc, err := security.EncryptPassword(password, key)
			if err != nil {
				return fmt.Errorf("密码加密失败: %w", err)
			}
			password = enc
		}
		list := loadServerList()
		for _, s := range list.Servers {
			if s.Name == name {
				return fmt.Errorf("服务器名称已存在: %s", name)
			}
		}
		list.Servers = append(list.Servers, cliServer{
			Name: name, Host: host, Port: port, User: user,
			AuthType: authType, IdentityFile: keyPath, Password: password, Note: note,
		})
		if err := saveServerList(list); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		fmt.Printf("✅ 已添加服务器: %s (%s@%s:%d)\n", name, user, host, port)
		return nil
	},
}

var CmdServerDelete = &cobra.Command{
	Use:   "delete <name>",
	Short: "删除独立服务器",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		list := loadServerList()
		idx := -1
		for i, s := range list.Servers {
			if s.Name == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("服务器不存在: %s", name)
		}
		list.Servers = append(list.Servers[:idx], list.Servers[idx+1:]...)
		if err := saveServerList(list); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
		fmt.Printf("🗑️ 已删除服务器: %s\n", name)
		return nil
	},
}
