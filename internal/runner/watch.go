// watch.go — 文件变更监听，自动触发代码检查。
package runner

import (
	"fmt"
	"time"
)

// WatchProject 监听项目源文件变更，自动执行检查。
// 每 2 秒轮询一次源文件修改时间，发现变更后自动触发 RunCheckInternal。
func WatchProject(projectPath string, projectType ProjectType, ruleStates map[string]bool, ciDir string) {
	lastMod := getLatestModTime(projectPath, projectType)
	fmt.Fprintf(logWriter, "👀 监听 %s (类型: %s)，文件变更后自动检查...\n", projectPath, projectType)
	fmt.Fprintf(logWriter, "   按 Ctrl+C 停止监听\n")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		currentMod := getLatestModTime(projectPath, projectType)
		if currentMod.IsZero() {
			continue
		}
		if currentMod.After(lastMod) {
			lastMod = currentMod
			fmt.Fprintf(logWriter, "\n📂 检测到文件变更，开始检查...\n")
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
