package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"ci-cd/internal/serve"
)

var CmdServe = &cobra.Command{
	Use:   "serve",
	Short: "启动 Web UI 服务器",
	Long:  `启动 Web UI 服务器，在浏览器中管理 CI/CD 流程。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		noOpen, _ := cmd.Flags().GetBool("no-open")
		isWin := runtime.GOOS == "windows"

		handler := serve.NewHandler(ciDir())
		server := &http.Server{Addr: ":" + port, Handler: handler}

		// Windows 下自动启动托盘，不打开浏览器
		if !noOpen {
			openBrowser(fmt.Sprintf("http://localhost:%s", port))
		}

		fmt.Printf("🚀 CI/CD Web UI 启动于 http://localhost:%s\n", port)
		fmt.Printf("🔑 默认用户名: admin  密码: 123456\n")

		if isWin {
			fmt.Println("🖥️  托盘图标已启动，右键系统托盘图标可操作")
			go serve.InitTray(port)
		} else {
			fmt.Println("   (首次启动时自动创建 ci-cd/auth.json，请通过 Web UI 或 `ci passwd` 修改密码)")
			fmt.Println("按 Ctrl+C 停止服务器")
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		go func() {
			<-quit
			fmt.Println("\n🛑 关闭服务器...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(ctx)
			serve.CloseAllSSHClients()
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	},
}

func init() {
	CmdServe.Flags().String("port", "8080", "监听端口")
	CmdServe.Flags().Bool("no-open", false, "不自动打开浏览器")
}

var testCiDir string

func ciDir() string {
	if testCiDir != "" {
		return testCiDir
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}
