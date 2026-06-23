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

		handler := serve.NewHandler(ciDir())
		server := &http.Server{Addr: ":" + port, Handler: handler}

		if !noOpen {
			url := fmt.Sprintf("http://localhost:%s", port)
			fmt.Printf("🌐 打开 %s\n", url)
			openBrowser(url)
		}

		fmt.Printf("🚀 CI/CD Web UI 启动于 http://localhost:%s\n", port)
		fmt.Printf("🔑 默认用户名: admin  密码: 123456\n")
		fmt.Println("   (首次启动时自动创建 ci-cd/auth.json，请通过 Web UI 或 `ci passwd` 修改密码)")
		fmt.Println("按 Ctrl+C 停止服务器")

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		go func() {
			<-quit
			fmt.Println("\n🛑 关闭服务器...")
			// 优雅退出：先停 HTTP，再清理缓存的 SSH 连接，防止泄漏
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

// testCiDir 用于测试时覆盖 ciDir() 的返回值，避免依赖 os.Executable()。
var testCiDir string

// ciDir 返回 ci.exe 所在目录（即 ci-cd 根目录）
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

// openBrowser 跨平台打开 URL（Windows/Mac/Linux）
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
