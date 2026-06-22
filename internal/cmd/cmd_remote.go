package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

// --- remote 子命令 ---

var CmdRemote = &cobra.Command{
	Use:   "remote <command>",
	Short: "远程服务器操作（文件管理 + 终端）",
}

var CmdRemoteLs = &cobra.Command{
	Use:   "ls <server>",
	Short: "列出远程服务器目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		source, _ := cmd.Flags().GetString("source")
		remotePath, _ := cmd.Flags().GetString("path")
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		entries, err := sftpClient.ReadDir(remotePath)
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		fmt.Printf("📁 %s:%s\n", name, remotePath)
		fmt.Println(strings.Repeat("─", 60))
		for _, e := range entries {
			mode := e.Mode().Perm().String()
			size := fmt.Sprintf("%10d", e.Size())
			if e.IsDir() {
				size = "       <DIR>"
			}
			modTime := e.ModTime().Format("2006-01-02 15:04")
			fmt.Printf("%s %s %s  %s\n", mode, size, modTime, e.Name())
		}
		return nil
	},
}

var CmdRemoteDownload = &cobra.Command{
	Use:   "download <server> --path <remote-path>",
	Short: "从远程服务器下载文件",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			return fmt.Errorf("打开远程文件失败: %w", err)
		}
		defer remoteFile.Close()

		localName := path.Base(remotePath)
		localFile, err := os.Create(localName)
		if err != nil {
			return fmt.Errorf("创建本地文件失败: %w", err)
		}
		defer localFile.Close()

		written, err := io.Copy(localFile, remoteFile)
		if err != nil {
			return fmt.Errorf("下载失败: %w", err)
		}
		fmt.Printf("✅ 已下载 %s (%d 字节) → %s\n", remotePath, written, localName)
		return nil
	},
}

var CmdRemoteUpload = &cobra.Command{
	Use:   "upload <server> --file <local-path> --path <remote-dir>",
	Short: "上传文件到远程服务器",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		localPath, _ := cmd.Flags().GetString("file")
		remoteDir, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if localPath == "" || remoteDir == "" {
			return fmt.Errorf("请指定 --file 和 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		localFile, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}
		defer localFile.Close()

		fileName := path.Base(localPath)
		targetPath := path.Join(remoteDir, fileName)
		remoteFile, err := sftpClient.Create(targetPath)
		if err != nil {
			return fmt.Errorf("创建远程文件失败: %w", err)
		}
		defer remoteFile.Close()

		written, err := io.Copy(remoteFile, localFile)
		if err != nil {
			return fmt.Errorf("上传失败: %w", err)
		}
		fmt.Printf("✅ 已上传 %s (%d 字节) → %s:%s\n", localPath, written, name, targetPath)
		return nil
	},
}

var CmdRemoteDelete = &cobra.Command{
	Use:   "delete <server> --path <remote-path>",
	Short: "删除远程服务器上的文件或目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		info, err := sftpClient.Stat(remotePath)
		if err != nil {
			return fmt.Errorf("路径不存在: %w", err)
		}
		if info.IsDir() {
			err = sftpClient.RemoveDirectory(remotePath)
		} else {
			err = sftpClient.Remove(remotePath)
		}
		if err != nil {
			return fmt.Errorf("删除失败: %w", err)
		}
		fmt.Printf("✅ 已删除 %s\n", remotePath)
		return nil
	},
}

var CmdRemoteMkdir = &cobra.Command{
	Use:   "mkdir <server> --path <remote-path>",
	Short: "在远程服务器上创建目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		remotePath, _ := cmd.Flags().GetString("path")
		source, _ := cmd.Flags().GetString("source")
		if remotePath == "" {
			return fmt.Errorf("请指定 --path")
		}
		deploy := resolveDeploy(name, source)
		if deploy == nil || deploy.Host == "" {
			return fmt.Errorf("服务器 %s 未配置部署信息", name)
		}
		client, err := dialSSH(deploy)
		if err != nil {
			return fmt.Errorf("SSH 连接失败: %w", err)
		}
		defer client.Close()
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return fmt.Errorf("SFTP 连接失败: %w", err)
		}
		defer sftpClient.Close()

		if err := sftpClient.MkdirAll(remotePath); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		fmt.Printf("✅ 已创建目录 %s\n", remotePath)
		return nil
	},
}
