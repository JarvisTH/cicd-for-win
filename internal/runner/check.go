// check.go — 代码检查逻辑，替代 ci-runner.ps1 的 Invoke-Check 函数。
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// appendStep 创建并追加一个步骤，失败时捕获错误输出。
func appendStep(steps *[]Step, name string, stepStart time.Time, execResult ExecResult) string {
	status := "pass"
	errLog := ""
	if execResult.ExitCode != 0 {
		status = "fail"
		// 优先使用 stderr，fallback 到 stdout
		errLog = execResult.Stderr
		if errLog == "" {
			errLog = execResult.Stdout
		}
		// 限制错误日志长度，避免界面渲染卡顿
		if len(errLog) > 2000 {
			errLog = errLog[:2000] + "...（已截断）"
		}
	}
	*steps = append(*steps, Step{
		Name: name, Status: status,
		Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
		ErrorLog: errLog,
	})
	return status
}

// RunCheckInternal 对项目执行代码检查。
// 支持 React（tsc + eslint）、Vue（vue-tsc + eslint）、Maven（compile + checkstyle）。
// 对应 ci-runner.ps1 的 Invoke-Check 函数。
func RunCheckInternal(projectPath string, projectType ProjectType, ruleStates map[string]bool) (Result, error) {
	start := time.Now()
	var steps []Step
	allPassed := true
	var errLogs []string

	switch projectType {
	case ProjectTypeReact:
		if isRuleEnabled(ruleStates, "tsc") {
			fmt.Fprintf(logWriter, "[React] 开始 TypeScript 类型检查...\n")
			r := runNpxWithTimeout(projectPath, "tsc", "--noEmit")
			if appendStep(&steps, "tsc", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "tsc: "+r.Stderr)
			}
		}
		if isRuleEnabled(ruleStates, "eslint") {
			fmt.Fprintf(logWriter, "[React] 开始 ESLint 检查...\n")
			r := runNpxWithTimeout(projectPath, "eslint", "src/")
			if appendStep(&steps, "eslint", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "eslint: "+r.Stderr)
			}
		}

	case ProjectTypeVue:
		if isRuleEnabled(ruleStates, "tsc") {
			fmt.Fprintf(logWriter, "[Vue] 开始 vue-tsc 类型检查...\n")
			r := runNpxWithTimeout(projectPath, "vue-tsc", "--noEmit")
			if appendStep(&steps, "tsc", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "vue-tsc: "+r.Stderr)
			}
		}
		if isRuleEnabled(ruleStates, "eslint") {
			fmt.Fprintf(logWriter, "[Vue] 开始 ESLint 检查...\n")
			eslintConfig := filepath.Join(filepath.Dir(projectPath), "rules", "eslint-vue.mjs")
			r := runNpxWithTimeout(projectPath, "eslint", "-c", eslintConfig, "src/")
			if appendStep(&steps, "eslint", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "eslint: "+r.Stderr)
			}
		}

	case ProjectTypeMaven:
		if isRuleEnabled(ruleStates, "compile") {
			fmt.Fprintf(logWriter, "[Maven] 开始编译检查...\n")
			r := runMvn(context.Background(), projectPath, "compile", "-q")
			if appendStep(&steps, "compile", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "compile: "+r.Stderr)
			}
		}
		if isRuleEnabled(ruleStates, "checkstyle") {
			fmt.Fprintf(logWriter, "[Maven] 开始 Checkstyle 检查...\n")
			checkstyleConfig := filepath.Join(filepath.Dir(projectPath), "rules", "checkstyle.xml")
			r := runMvn(context.Background(), projectPath, "checkstyle:check", "-Dcheckstyle.config="+checkstyleConfig)
			if appendStep(&steps, "checkstyle", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "checkstyle: "+r.Stderr)
			}
		}

	case ProjectTypeMavenMulti:
		if isRuleEnabled(ruleStates, "compile") {
			fmt.Fprintf(logWriter, "[MavenMulti] 开始多模块编译检查...\n")
			r := runMvn(context.Background(), projectPath, "compile", "-q")
			if appendStep(&steps, "compile", time.Now(), r) == "fail" {
				allPassed = false
				errLogs = append(errLogs, "compile: "+r.Stderr)
			}
		}

	default:
		fmt.Fprintf(logWriter, "未知项目类型: %s，跳过代码检查\n", projectType)
	}

	duration := time.Since(start)
	status := "pass"
	if !allPassed {
		status = "fail"
	}

	// 将步骤错误汇总到 Result.ErrorLog（供前端 stepStatus 展示）
	errorLog := strings.Join(errLogs, "\n")
	if len(errorLog) > 5000 {
		errorLog = errorLog[:5000] + "...（已截断）"
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", duration.Seconds()),
		Steps:    steps,
		ErrorLog: errorLog,
	}, nil
}

// isRuleEnabled 判断规则是否启用。
// ruleStates 为空时所有规则默认启用（保持向后兼容）。
func isRuleEnabled(states map[string]bool, id string) bool {
	if len(states) == 0 {
		return true
	}
	enabled, ok := states[id]
	if !ok {
		return true // 未显式列出的规则默认启用
	}
	return enabled
}
