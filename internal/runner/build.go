// build.go — 构建逻辑，替代 ci-runner.ps1 的 Invoke-Build 函数。
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunBuildInternal 对项目执行完整构建。
// 支持 React/Vue（npm run build）、Maven（mvn clean package -DskipTests）、MavenMulti（mvn clean install -DskipTests）。
// 对应 ci-runner.ps1 的 Invoke-Build 函数。
func RunBuildInternal(projectPath string, projectType ProjectType) (Result, error) {
	start := time.Now()
	var steps []Step
	allPassed := true

	switch projectType {
	case ProjectTypeReact, ProjectTypeVue:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[%s] 开始 npm run build...\n", projectType)
		result := runNpm(context.Background(), projectPath, "run", "build")
		stepStatus := "pass"
		if result.ExitCode != 0 {
			stepStatus = "fail"
			allPassed = false
			fmt.Fprintf(logWriter, "❌ npm run build 失败\n")
		} else {
			// 验证构建产物
			if _, err := os.Stat(filepath.Join(projectPath, "dist", "index.html")); err != nil {
				fmt.Fprintf(logWriter, "⚠️ 构建产物 dist/index.html 不存在\n")
			} else {
				fmt.Fprintf(logWriter, "✅ 构建成功: dist/\n")
			}
		}
		steps = append(steps, Step{
			Name: "build", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
		})

	case ProjectTypeMaven:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[Maven] 开始 mvn clean package...\n")
		result := runMvn(context.Background(), projectPath, "clean", "package", "-DskipTests", "-q")
		stepStatus := "pass"
		if result.ExitCode != 0 {
			stepStatus = "fail"
			allPassed = false
			fmt.Fprintf(logWriter, "❌ Maven 构建失败\n")
		} else {
			// 验证构建产物
			matches, _ := filepath.Glob(filepath.Join(projectPath, "target", "*.jar"))
			hasJar := false
			for _, m := range matches {
				base := filepath.Base(m)
				if !containsAny(base, "sources", "javadoc", "original") {
					fmt.Fprintf(logWriter, "✅ 构建成功: %s\n", base)
					hasJar = true
					break
				}
			}
			if !hasJar {
				fmt.Fprintf(logWriter, "⚠️ 未找到 *.jar 产物\n")
			}
		}
		steps = append(steps, Step{
			Name: "build", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
		})

	case ProjectTypeMavenMulti:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[MavenMulti] 开始 mvn clean install...\n")
		result := runMvn(context.Background(), projectPath, "clean", "install", "-DskipTests", "-q")
		stepStatus := "pass"
		if result.ExitCode != 0 {
			stepStatus = "fail"
			allPassed = false
			fmt.Fprintf(logWriter, "❌ 多模块构建失败\n")
		} else {
			fmt.Fprintf(logWriter, "✅ 全部模块构建成功\n")
		}
		steps = append(steps, Step{
			Name: "build", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
		})

	default:
		fmt.Fprintf(logWriter, "未知项目类型: %s，跳过构建\n", projectType)
		steps = append(steps, Step{
			Name: "build", Status: "skip", Duration: "0.0s",
		})
	}

	duration := time.Since(start)
	status := "pass"
	if !allPassed {
		status = "fail"
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", duration.Seconds()),
		Steps:    steps,
	}, nil
}

// containsAny 检查 s 是否包含 targets 中的任意一个子串。
func containsAny(s string, targets ...string) bool {
	for _, t := range targets {
		for i := 0; i <= len(s)-len(t); i++ {
			if s[i:i+len(t)] == t {
				return true
			}
		}
	}
	return false
}
