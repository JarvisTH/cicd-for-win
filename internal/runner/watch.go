// watch.go — 文件变更监听，自动触发代码检查。
package runner

import (
	"context"
	"fmt"
	"time"
)

// WatchProject 监听项目源文件变更，自动执行检查。
// 每 2 秒轮询一次源文件修改时间，发现变更后自动触发 RunCheckInternal。
// ctx 用于取消监听，为 nil 时使用 background context。
func WatchProject(projectPath string, projectType ProjectType, ruleStates map[string]bool, ciDir string, ctx ...context.Context) {
	watchCtx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		watchCtx = ctx[0]
	}
	lastMod := getLatestModTime(projectPath, projectType)
	fmt.Fprintf(logWriter, "👀 监听 %s (类型: %s)，文件变更后自动检查...\n", projectPath, projectType)
	fmt.Fprintf(logWriter, "   按 Ctrl+C 停止监听\n")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-watchCtx.Done():
			fmt.Fprintf(logWriter, "👀 监听已停止\n")
			return
		case <-ticker.C:
			currentMod := getLatestModTime(projectPath, projectType)
			if currentMod.IsZero() {
				continue
			}
			if currentMod.After(lastMod) {
				fmt.Fprintf(logWriter, "\n📂 检测到文件变更，等待写入完成...\n")
				time.Sleep(300 * time.Millisecond)
				afterDebounce := getLatestModTime(projectPath, projectType)
				if afterDebounce.IsZero() || !afterDebounce.After(lastMod) {
					fmt.Fprintf(logWriter, "  ⏳ 文件尚未稳定，等待下一次检测\n")
					continue
				}
				lastMod = afterDebounce
				fmt.Fprintf(logWriter, "  ✅ 文件已稳定，开始检查...\n")
				result, err := RunCheckInternal(projectPath, projectType, ruleStates, ciDir)
				if err != nil {
					fmt.Fprintf(logWriter, "❌ 检查失败: %v\n", err)
				} else if result.Status == "pass" {
					fmt.Fprintf(logWriter, "✅ 检查通过 (%s)\n", result.Duration)
				} else {
					fmt.Fprintf(logWriter, "❌ 检查未通过 (%s)\n", result.Duration)
				}
			}
		}
	}
}
