// build.go — 构建逻辑，替代 ci-runner.ps1 的 Invoke-Build 函数。
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunBuildInternal 对项目执行完整构建。
// 支持 React/Vue（npm run build）、Maven（mvn clean package -DskipTests）、MavenMulti（mvn clean install -DskipTests）。
// 对应 ci-runner.ps1 的 Invoke-Build 函数。
func RunBuildInternal(projectPath string, projectType ProjectType, ciDir ...string) (Result, error) {
	start := time.Now()
	var steps []Step

	// 缓存检查
	var projectName string
	if len(ciDir) > 0 && ciDir[0] != "" {
		projectName = filepath.Base(projectPath)
		if cache := cacheHit(ciDir[0], projectName, "build", projectType, projectPath); cache != nil {
			fmt.Fprintf(logWriter, cacheSummary(cache))
			return Result{Status: "pass", Duration: cache.Duration, Steps: []Step{{Name: "build", Status: "pass", Duration: cache.Duration}}}, nil
		}
	}

	switch projectType {
	case ProjectTypeReact, ProjectTypeVue:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[%s] 开始 npm run build...\n", projectType)
		r := runNpm(context.Background(), projectPath, "run", "build")
		if r.ExitCode != 0 {
			fmt.Fprintf(logWriter, "❌ npm run build 失败\n")
		} else if _, err := os.Stat(filepath.Join(projectPath, "dist", "index.html")); err != nil {
			fmt.Fprintf(logWriter, "⚠️ 构建产物 dist/index.html 不存在\n")
		} else {
			fmt.Fprintf(logWriter, "✅ 构建成功: dist/\n")
		}
		steps = append(steps, makeBuildStep("build", stepStart, r))

	case ProjectTypeMaven:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[Maven] 开始 mvn clean package...\n")
		r := runMvn(context.Background(), projectPath, "clean", "package", "-DskipTests", "-q")
		s := "pass"
		if r.ExitCode != 0 {
			s = "fail"
			fmt.Fprintf(logWriter, "❌ Maven 构建失败\n")
		} else {
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
		_ = s
		steps = append(steps, makeBuildStep("build", stepStart, r))

	case ProjectTypeMavenMulti:
		stepStart := time.Now()
		fmt.Fprintf(logWriter, "[MavenMulti] 开始 mvn clean install...\n")
		r := runMvn(context.Background(), projectPath, "clean", "install", "-DskipTests", "-q")
		if r.ExitCode != 0 {
			fmt.Fprintf(logWriter, "❌ 多模块构建失败\n")
		} else {
			fmt.Fprintf(logWriter, "✅ 全部模块构建成功\n")
		}
		steps = append(steps, makeBuildStep("build", stepStart, r))

	default:
		fmt.Fprintf(logWriter, "未知项目类型: %s，跳过构建\n", projectType)
		steps = append(steps, Step{Name: "build", Status: "skip", Duration: "0.0s"})
	}

	// 计算状态
	status := "pass"
	for _, s := range steps {
		if s.Status == "fail" {
			status = "fail"
			break
		}
	}

	// 收集错误日志
	var errParts []string
	for _, s := range steps {
		if s.ErrorLog != "" {
			errParts = append(errParts, s.Name+": "+s.ErrorLog)
		}
	}
	errorLog := strings.Join(errParts, "\n")
	if len(errorLog) > 5000 {
		errorLog = errorLog[:5000] + "...（已截断）"
	}

	// 保存缓存
	if projectName != "" {
		saveCache(ciDir[0], projectName, "build", &BuildCache{
			Project: projectName, Action: "build",
			Status:   status,
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			MaxModTime: getLatestModTime(projectPath, projectType),
		})
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
		Steps:    steps,
		ErrorLog: errorLog,
	}, nil
}

// makeBuildStep 构建一个 Step，失败时自动捕获命令 stderr。
func makeBuildStep(name string, stepStart time.Time, r ExecResult) Step {
	s := "pass"
	errLog := ""
	if r.ExitCode != 0 {
		s = "fail"
		errLog = r.Stderr
		if errLog == "" {
			errLog = r.Stdout
		}
		if len(errLog) > 2000 {
			errLog = errLog[:2000] + "...（已截断）"
		}
	}
	return Step{
		Name: name, Status: s,
		Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
		ErrorLog: errLog,
	}
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
