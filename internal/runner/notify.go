// notify.go — 跨平台系统通知
package runner

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Notify 发送系统通知。
// title: 通知标题
// message: 通知内容
func Notify(title, message string) {
	switch runtime.GOOS {
	case "windows":
		// Windows: 使用 PowerShell 弹窗
		ps := fmt.Sprintf(`[System.Windows.Forms.MessageBox]::Show('%s','%s')`,
			escapePS(message), escapePS(title))
		exec.Command("powershell", "-Command", ps).Start()
	case "darwin":
		// macOS: 使用 osascript
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)).Start()
	default:
		// Linux: 使用 notify-send
		exec.Command("notify-send", title, message).Start()
	}
}

// NotifyPass 发送通过通知。
func NotifyPass(project, action, duration string) {
	Notify(fmt.Sprintf("✅ %s %s 通过", project, action),
		fmt.Sprintf("耗时: %s", duration))
}

// NotifyFail 发送失败通知。
func NotifyFail(project, action string) {
	Notify(fmt.Sprintf("❌ %s %s 失败", project, action),
		"请查看错误详情")
}

func escapePS(s string) string {
	// PowerShell 单引号字符串中的特殊字符转义
	result := ""
	for _, c := range s {
		if c == '\'' {
			result += "''"
		} else {
			result += string(c)
		}
	}
	return result
}
