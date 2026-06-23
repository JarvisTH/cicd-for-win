// check.go — 代码检查逻辑，替代 ci-runner.ps1 的 Invoke-Check 函数。
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// RunCheckInternal 对项目执行代码检查。
// 支持 React（tsc + eslint）、Vue（vue-tsc + eslint）、Maven（compile + checkstyle）。
// 对应 ci-runner.ps1 的 Invoke-Check 函数。
func RunCheckInternal(projectPath string, projectType ProjectType, ruleStates map[string]bool) (Result, error) {
	start := time.Now()
	var steps []Step
	allPassed := true

	switch projectType {
	case ProjectTypeReact:
		if isRuleEnabled(ruleStates, "tsc") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[React] 开始 TypeScript 类型检查...\n")
			result := runNpxWithTimeout(projectPath, "tsc", "--noEmit")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ TypeScript 类型检查失败\n")
			}
			steps = append(steps, Step{
				Name: "tsc", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}
		if isRuleEnabled(ruleStates, "eslint") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[React] 开始 ESLint 检查...\n")
			result := runNpxWithTimeout(projectPath, "eslint", "src/")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ ESLint 检查失败\n")
			}
			steps = append(steps, Step{
				Name: "eslint", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}

	case ProjectTypeVue:
		if isRuleEnabled(ruleStates, "tsc") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[Vue] 开始 vue-tsc 类型检查...\n")
			result := runNpxWithTimeout(projectPath, "vue-tsc", "--noEmit")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ vue-tsc 类型检查失败\n")
			}
			steps = append(steps, Step{
				Name: "tsc", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}
		if isRuleEnabled(ruleStates, "eslint") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[Vue] 开始 ESLint 检查...\n")
			// Vue 项目使用 CI/CD 自有规则文件
			eslintConfig := filepath.Join(filepath.Dir(projectPath), "rules", "eslint-vue.mjs")
			result := runNpxWithTimeout(projectPath, "eslint", "-c", eslintConfig, "src/")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ ESLint 检查失败\n")
			}
			steps = append(steps, Step{
				Name: "eslint", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}

	case ProjectTypeMaven:
		if isRuleEnabled(ruleStates, "compile") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[Maven] 开始编译检查...\n")
			result := runMvn(context.Background(), projectPath, "compile", "-q")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ Maven 编译检查失败\n")
			}
			steps = append(steps, Step{
				Name: "compile", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}
		if isRuleEnabled(ruleStates, "checkstyle") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[Maven] 开始 Checkstyle 检查...\n")
			checkstyleConfig := filepath.Join(filepath.Dir(projectPath), "rules", "checkstyle.xml")
			result := runMvn(context.Background(), projectPath, "checkstyle:check", "-Dcheckstyle.config="+checkstyleConfig)
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ Checkstyle 检查失败\n")
			}
			steps = append(steps, Step{
				Name: "checkstyle", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}

	case ProjectTypeMavenMulti:
		if isRuleEnabled(ruleStates, "compile") {
			stepStart := time.Now()
			fmt.Fprintf(logWriter, "[MavenMulti] 开始多模块编译检查...\n")
			result := runMvn(context.Background(), projectPath, "compile", "-q")
			stepStatus := "pass"
			if result.ExitCode != 0 {
				stepStatus = "fail"
				allPassed = false
				fmt.Fprintf(logWriter, "❌ 多模块编译检查失败\n")
			}
			steps = append(steps, Step{
				Name: "compile", Status: stepStatus, Duration: fmt.Sprintf("%.1fs", time.Since(stepStart).Seconds()),
			})
		}

	default:
		fmt.Fprintf(logWriter, "未知项目类型: %s，跳过代码检查\n", projectType)
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
