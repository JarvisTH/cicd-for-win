// push.go — Git 推送逻辑，替代 ci-push.ps1 脚本。
package runner

import (
	"context"
	"fmt"
	"strings"

	"ci-cd/internal/config"
)

// RunPushInternal 将项目推送到所有已启用的 Git 远程仓库。
// 对应 ci-push.ps1 脚本的逻辑。
func RunPushInternal(project config.Project) error {
	if len(project.Remotes) == 0 {
		fmt.Fprintf(logWriter, "[%s] 未配置远程仓库，跳过推送\n", project.Name)
		return nil
	}

	// 确定推送的目标分支
	branch := project.GitBranch
	if branch == "" {
		// 未指定分支时使用当前检出分支
		result := runGitWithTimeout(project.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if result.ExitCode != 0 {
			return fmt.Errorf("[%s] 获取当前分支失败", project.Name)
		}
		branch = strings.TrimSpace(result.Stdout)
	}

	allSuccess := true
	for _, remote := range project.Remotes {
		if !remote.Enabled {
			continue
		}

		// 检查远程是否已存在
		remoteResult := runGit(context.Background(), project.Path, "remote", "-v")
		remoteExists := false
		if remoteResult.ExitCode == 0 {
			for _, line := range strings.Split(remoteResult.Stdout, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), remote.Name+"\t") {
					remoteExists = true
					break
				}
			}
		}

		if !remoteExists {
			addResult := runGit(context.Background(), project.Path, "remote", "add", remote.Name, remote.URL)
			if addResult.ExitCode != 0 {
				fmt.Fprintf(logWriter, "[%s] 添加远程 %s 失败: %s\n", project.Name, remote.Name, addResult.Stderr)
				allSuccess = false
				continue
			}
			fmt.Fprintf(logWriter, "[%s] 添加远程: %s → %s\n", project.Name, remote.Name, remote.URL)
		}

		fmt.Fprintf(logWriter, "[%s] 📤 推送到 %s...\n", project.Name, remote.Name)
		pushResult := runGitWithTimeout(project.Path, "push", remote.Name, branch)
		if pushResult.ExitCode == 0 {
			fmt.Fprintf(logWriter, "[%s] ✅ %s: 推送成功\n", project.Name, remote.Name)
		} else {
			fmt.Fprintf(logWriter, "[%s] ❌ %s: 推送失败 — %s\n", project.Name, remote.Name, pushResult.Stderr)
			allSuccess = false
		}
	}

	if !allSuccess {
		return fmt.Errorf("[%s] 部分远程推送失败", project.Name)
	}
	return nil
}
