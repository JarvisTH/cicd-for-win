// test.go — 测试执行与报告解析逻辑，替代 ci-runner.ps1 的 Invoke-Test 函数。
package runner

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunTestInternal 对项目执行单元测试并解析测试报告。
// 对应 ci-runner.ps1 的 Invoke-Test 函数。
func RunTestInternal(projectPath string, projectType ProjectType, ciDir ...string) (Result, *TestReport, error) {
	start := time.Now()

	// 缓存检查（仅 test 不需要缓存，因为每次应重新运行以获取最新报告）
	// 但如果只想跳过无变更的测试，可以在此处添加 cacheHit 检查

	switch projectType {
	case ProjectTypeReact, ProjectTypeVue:
		return runFrontendTest(projectPath, projectType, start)
	case ProjectTypeMaven:
		return runMavenTest(projectPath, start)
	case ProjectTypeMavenMulti:
		return runMavenMultiTest(projectPath, start)
	default:
		fmt.Fprintf(logWriter, "未知项目类型: %s，跳过测试\n", projectType)
		return Result{
			Status:   "skip",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
		}, nil, nil
	}
}

// runFrontendTest 对前端项目执行测试，自动检测 Vitest 或 Jest。
func runFrontendTest(projectPath string, projectType ProjectType, start time.Time) (Result, *TestReport, error) {
	pkgFile := filepath.Join(projectPath, "package.json")
	pkgData, err := os.ReadFile(pkgFile)
	if err != nil {
		return failResult("无法读取 package.json", start), nil, fmt.Errorf("读取 package.json 失败: %w", err)
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	json.Unmarshal(pkgData, &pkg)

	hasDep := func(name string) bool {
		_, ok1 := pkg.Dependencies[name]
		_, ok2 := pkg.DevDependencies[name]
		return ok1 || ok2
	}

	isVitest := hasDep("vitest")
	isJest := hasDep("jest")

	fmt.Fprintf(logWriter, "[%s] 开始测试...\n", projectType)

	var report TestReport
	var output string
	exitCode := 0

	if isVitest {
		fmt.Fprintf(logWriter, "  检测到 Vitest\n")
		// 执行 vitest，输出 JSON 报告
		result := runNpxWithTimeout(projectPath, "vitest", "run", "--reporter=json")
		exitCode = result.ExitCode
		output = result.Stdout + "\n" + result.Stderr

		// 从输出中提取 JSON 报告（vitest --reporter=json 输出到最后一行）
		jsonStr := extractJSONFromOutput(result.Stdout)
		if jsonStr != "" {
			report = parseVitestJSON(jsonStr)
		}

		// 读取覆盖率
		covFile := filepath.Join(projectPath, "coverage", "coverage-summary.json")
		if covData, err := os.ReadFile(covFile); err == nil {
			report.Coverage = parseCoverageSummary(covData)
		}
	} else if isJest {
		fmt.Fprintf(logWriter, "  检测到 Jest\n")
		result := runNpxWithTimeout(projectPath, "jest", "--json", "--coverage")
		exitCode = result.ExitCode
		output = result.Stdout + "\n" + result.Stderr

		// 从输出中提取 JSON 报告
		jsonStr := extractJSONFromOutput(result.Stdout)
		if jsonStr != "" {
			report = parseJestJSON(jsonStr)
		}
	} else {
		// 兜底：直接跑 npm test
		result := runNpm(context.Background(), projectPath, "test")
		exitCode = result.ExitCode
		output = result.Stdout + "\n" + result.Stderr
	}

	report.RawLog = strings.TrimSpace(output)

	status := "pass"
	if exitCode != 0 && report.Failed > 0 {
		status = "fail"
	} else if exitCode != 0 {
		// 没有失败用例但退出码非零（如框架配置问题）
		status = "fail"
	}

	// 计算报告统计（如果解析成功）
	if report.Total > 0 && report.Passed == 0 && report.Failed == 0 {
		report.Passed = report.Total - report.Failed - report.Skipped
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
	}, &report, nil
}

// runMavenTest 对 Maven 项目执行测试，解析 Surefire XML 报告和 JaCoCo 覆盖率。
func runMavenTest(projectPath string, start time.Time) (Result, *TestReport, error) {
	fmt.Fprintf(logWriter, "[Maven] 开始 mvn test...\n")
	result := runMvn(context.Background(), projectPath, "test", "-Dmaven.test.failure.ignore=true")
	output := result.Stdout + "\n" + result.Stderr

	report := TestReport{
		RawLog: strings.TrimSpace(output),
	}

	// 解析 Surefire XML 报告
	surefireDir := filepath.Join(projectPath, "target", "surefire-reports")
	if entries, err := os.ReadDir(surefireDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), "TEST-") || !strings.HasSuffix(entry.Name(), ".xml") {
				continue
			}
			xmlData, err := os.ReadFile(filepath.Join(surefireDir, entry.Name()))
			if err != nil {
				continue
			}
			parseSurefireXML(xmlData, &report)
		}
	}

	// 解析 JaCoCo 覆盖率
	jacocoReport := filepath.Join(projectPath, "target", "site", "jacoco", "jacoco.xml")
	if xmlData, err := os.ReadFile(jacocoReport); err == nil {
		report.Coverage = parseJacocoXML(xmlData)
	}

	// 计算通过数
	if report.Total > 0 {
		report.Passed = report.Total - report.Failed - report.Skipped
	}

	status := "pass"
	if result.ExitCode != 0 && report.Failed > 0 {
		status = "fail"
	} else if result.ExitCode != 0 {
		status = "fail"
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
	}, &report, nil
}

// runMavenMultiTest 对多模块 Maven 项目执行测试，遍历各模块的 Surefire 报告。
func runMavenMultiTest(projectPath string, start time.Time) (Result, *TestReport, error) {
	fmt.Fprintf(logWriter, "[MavenMulti] 开始 mvn test（多模块）...\n")
	result := runMvn(context.Background(), projectPath, "test", "-Dmaven.test.failure.ignore=true")
	output := result.Stdout + "\n" + result.Stderr

	report := TestReport{
		RawLog: strings.TrimSpace(output),
	}

	// 遍历所有子模块的 surefire 报告
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return failResult("读取项目目录失败", start), nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		modPath := filepath.Join(projectPath, entry.Name())
		if _, err := os.Stat(filepath.Join(modPath, "pom.xml")); err != nil {
			continue // 没有 pom.xml 的不是 Maven 模块
		}
		surefireDir := filepath.Join(modPath, "target", "surefire-reports")
		if xmlEntries, err := os.ReadDir(surefireDir); err == nil {
			for _, xmlEntry := range xmlEntries {
				if xmlEntry.IsDir() || !strings.HasPrefix(xmlEntry.Name(), "TEST-") || !strings.HasSuffix(xmlEntry.Name(), ".xml") {
					continue
				}
				xmlData, err := os.ReadFile(filepath.Join(surefireDir, xmlEntry.Name()))
				if err != nil {
					continue
				}
				parseSurefireXML(xmlData, &report)
			}
		}
	}

	// 计算通过数
	if report.Total > 0 {
		report.Passed = report.Total - report.Failed - report.Skipped
	}

	status := "pass"
	if result.ExitCode != 0 && report.Failed > 0 {
		status = "fail"
	} else if result.ExitCode != 0 {
		status = "fail"
	}

	return Result{
		Status:   status,
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
	}, &report, nil
}

// ===================== 报告解析 =====================

// vitestJSON 映射 Vitest --reporter=json 的输出结构。
type vitestJSON struct {
	NumTotalTests     int `json:"numTotalTests"`
	NumPassedTests    int `json:"numPassedTests"`
	NumFailedTests    int `json:"numFailedTests"`
	NumPendingTests   int `json:"numPendingTests"`
	TestResults       []struct {
		Name             string `json:"name"`
		AssertionResults []struct {
			FullName        string   `json:"fullName"`
			Status          string   `json:"status"`
			FailureMessages []string `json:"failureMessages"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

// parseVitestJSON 解析 Vitest JSON 报告。
func parseVitestJSON(data string) TestReport {
	var r vitestJSON
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return TestReport{}
	}

	report := TestReport{
		Total:   r.NumTotalTests,
		Passed:  r.NumPassedTests,
		Failed:  r.NumFailedTests,
		Skipped: r.NumPendingTests,
	}

	for _, suite := range r.TestResults {
		for _, t := range suite.AssertionResults {
			if t.Status == "failed" {
				suiteName := suite.Name
				if idx := strings.LastIndex(suiteName, "/"); idx >= 0 {
					suiteName = suiteName[idx+1:]
				}
				msg := ""
				if len(t.FailureMessages) > 0 {
					msg = strings.ReplaceAll(t.FailureMessages[0], "\n", " ")
				}
				report.Failures = append(report.Failures, TestFailure{
					Suite: suiteName, Test: t.FullName, Message: msg,
				})
			}
		}
	}

	return report
}

// jestJSON 映射 Jest --json 的输出结构。
type jestJSON struct {
	NumTotalTests  int `json:"numTotalTests"`
	NumPassedTests int `json:"numPassedTests"`
	NumFailedTests int `json:"numFailedTests"`
	NumPendingTests int `json:"numPendingTests"`
	TestResults    []struct {
		Name             string `json:"name"`
		AssertionResults []struct {
			FullName        string   `json:"fullName"`
			Status          string   `json:"status"`
			FailureMessages []string `json:"failureMessages"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

// parseJestJSON 解析 Jest JSON 报告。
func parseJestJSON(data string) TestReport {
	var r jestJSON
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return TestReport{}
	}

	report := TestReport{
		Total:   r.NumTotalTests,
		Passed:  r.NumPassedTests,
		Failed:  r.NumFailedTests,
		Skipped: r.NumPendingTests,
	}

	for _, suite := range r.TestResults {
		for _, t := range suite.AssertionResults {
			if t.Status == "failed" {
				suiteName := suite.Name
				if idx := strings.LastIndex(suiteName, "/"); idx >= 0 {
					suiteName = suiteName[idx+1:]
				}
				msg := ""
				if len(t.FailureMessages) > 0 {
					msg = strings.ReplaceAll(t.FailureMessages[0], "\n", " ")
				}
				report.Failures = append(report.Failures, TestFailure{
					Suite: suiteName, Test: t.FullName, Message: msg,
				})
			}
		}
	}

	return report
}

// surefireXML 映射 Maven Surefire XML 报告结构。
type surefireXML struct {
	XMLName   xml.Name `xml:"testsuite"`
	Name      string   `xml:"name,attr"`
	Tests     int      `xml:"tests,attr"`
	Failures  int      `xml:"failures,attr"`
	Errors    int      `xml:"errors,attr"`
	Skipped   int      `xml:"skipped,attr"`
	TestCases []struct {
		Name      string `xml:"name,attr"`
		ClassName string `xml:"classname,attr"`
		Failure   *struct {
			Message string `xml:"message,attr"`
		} `xml:"failure"`
		Error *struct {
			Message string `xml:"message,attr"`
		} `xml:"error"`
	} `xml:"testcase"`
}

// parseSurefireXML 解析单个 Surefire XML 报告并合并到 TestReport。
func parseSurefireXML(data []byte, report *TestReport) {
	var ts surefireXML
	if err := xml.Unmarshal(data, &ts); err != nil {
		return
	}
	report.Total += ts.Tests
	report.Failed += ts.Failures + ts.Errors
	report.Skipped += ts.Skipped

	for _, tc := range ts.TestCases {
		if tc.Failure != nil || tc.Error != nil {
			msg := ""
			if tc.Failure != nil {
				msg = tc.Failure.Message
			} else if tc.Error != nil {
				msg = tc.Error.Message
			}
			report.Failures = append(report.Failures, TestFailure{
				Suite: ts.Name, Test: tc.Name, Message: msg,
			})
		}
	}
}

// jacocoXML 映射 JaCoCo XML 报告结构。
type jacocoXML struct {
	XMLName xml.Name `xml:"report"`
	Counters []struct {
		Type    string `xml:"type,attr"`
		Covered int    `xml:"covered,attr"`
		Missed  int    `xml:"missed,attr"`
	} `xml:"counter"`
}

// parseJacocoXML 解析 JaCoCo XML 报告，返回覆盖率字符串。
func parseJacocoXML(data []byte) string {
	var r jacocoXML
	if err := xml.Unmarshal(data, &r); err != nil {
		return ""
	}
	for _, c := range r.Counters {
		if c.Type == "LINE" {
			total := c.Covered + c.Missed
			if total > 0 {
				pct := float64(c.Covered) / float64(total) * 100
				return fmt.Sprintf("%.1f%%", pct)
			}
		}
	}
	return ""
}

// coverageSummary 映射 Vitest coverage-summary.json 结构。
type coverageSummary struct {
	Total struct {
		Lines struct {
			Pct float64 `json:"pct"`
		} `json:"lines"`
	} `json:"total"`
}

// parseCoverageSummary 解析 Vitest 覆盖率摘要 JSON。
func parseCoverageSummary(data []byte) string {
	var s coverageSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return fmt.Sprintf("%.1f%%", s.Total.Lines.Pct)
}

// extractJSONFromOutput 从命令输出中提取 JSON 报告字符串。
// 查找以 {"numTotalTestSuites" 开头的 JSON 行。
func extractJSONFromOutput(output string) string {
	lines := strings.Split(output, "\n")
	// 从后往前查找 JSON 行
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, `{"numTotalTestSuites":`) {
			// 找到 JSON 结尾
			endIdx := strings.LastIndex(line, "}")
			if endIdx >= 0 {
				return line[:endIdx+1]
			}
			return line
		}
	}
	return ""
}

// failResult 返回一个快速失败的 Result。
func failResult(errMsg string, start time.Time) Result {
	return Result{
		Status:   "fail",
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
		ErrorLog: errMsg,
	}
}

var logWriter = os.Stderr
