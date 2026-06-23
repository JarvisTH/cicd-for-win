package runner

import (
	"testing"
)

// ===================== parseVitestJSON =====================

func TestParseVitestJSON_AllPass(t *testing.T) {
	data := `{"numTotalTestSuites":2,"numTotalTests":5,"numPassedTests":5,"numFailedTests":0,"numPendingTests":0,"testResults":[{"name":"src/App.test.ts","assertionResults":[{"fullName":"App renders","status":"passed"}]}]}`
	report := parseVitestJSON(data)
	if report.Total != 5 {
		t.Errorf("Total 应为 5, 得到 %d", report.Total)
	}
	if report.Passed != 5 {
		t.Errorf("Passed 应为 5, 得到 %d", report.Passed)
	}
	if report.Failed != 0 {
		t.Errorf("Failed 应为 0, 得到 %d", report.Failed)
	}
	if len(report.Failures) != 0 {
		t.Errorf("不应有失败用例")
	}
}

func TestParseVitestJSON_WithFailures(t *testing.T) {
	data := `{"numTotalTestSuites":1,"numTotalTests":3,"numPassedTests":2,"numFailedTests":1,"numPendingTests":0,"testResults":[{"name":"src/App.test.ts","assertionResults":[{"fullName":"App renders","status":"passed"},{"fullName":"App fails","status":"failed","failureMessages":["Expected true to be false"]}]}]}`
	report := parseVitestJSON(data)
	if report.Total != 3 {
		t.Errorf("Total 应为 3, 得到 %d", report.Total)
	}
	if report.Passed != 2 {
		t.Errorf("Passed 应为 2, 得到 %d", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("Failed 应为 1, 得到 %d", report.Failed)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("应有 1 个失败用例, 得到 %d", len(report.Failures))
	}
	if report.Failures[0].Test != "App fails" {
		t.Errorf("失败测试名应为 'App fails', 得到 %s", report.Failures[0].Test)
	}
	if report.Failures[0].Message != "Expected true to be false" {
		t.Errorf("失败消息不匹配")
	}
}

func TestParseVitestJSON_MultipleSuites(t *testing.T) {
	data := `{"numTotalTestSuites":2,"numTotalTests":4,"numPassedTests":2,"numFailedTests":2,"numPendingTests":0,"testResults":[{"name":"suite/A.test.ts","assertionResults":[{"fullName":"A test1","status":"failed","failureMessages":["err1"]}]},{"name":"suite/B.test.ts","assertionResults":[{"fullName":"B test1","status":"failed","failureMessages":["err2"]}]}]}`
	report := parseVitestJSON(data)
	if report.Total != 4 {
		t.Errorf("Total 应为 4, 得到 %d", report.Total)
	}
	if report.Failed != 2 {
		t.Errorf("Failed 应为 2, 得到 %d", report.Failed)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("应有 2 个失败用例, 得到 %d", len(report.Failures))
	}
}

func TestParseVitestJSON_Empty(t *testing.T) {
	data := `{"numTotalTestSuites":0,"numTotalTests":0,"numPassedTests":0,"numFailedTests":0,"numPendingTests":0,"testResults":[]}`
	report := parseVitestJSON(data)
	if report.Total != 0 {
		t.Errorf("Total 应为 0, 得到 %d", report.Total)
	}
}

func TestParseVitestJSON_InvalidJSON(t *testing.T) {
	report := parseVitestJSON(`{invalid}`)
	// 无效 JSON 应返回空的 TestReport
	if report.Total != 0 {
		t.Errorf("Total 应为 0, 得到 %d", report.Total)
	}
}

// ===================== parseJestJSON =====================

func TestParseJestJSON_AllPass(t *testing.T) {
	data := `{"numTotalTestSuites":1,"numTotalTests":2,"numPassedTests":2,"numFailedTests":0,"numPendingTests":0,"testResults":[{"name":"test.js","assertionResults":[{"fullName":"test works","status":"passed"}]}]}`
	report := parseJestJSON(data)
	if report.Total != 2 {
		t.Errorf("Total 应为 2, 得到 %d", report.Total)
	}
	if report.Passed != 2 {
		t.Errorf("Passed 应为 2, 得到 %d", report.Passed)
	}
}

func TestParseJestJSON_WithFailures(t *testing.T) {
	data := `{"numTotalTestSuites":1,"numTotalTests":2,"numPassedTests":1,"numFailedTests":1,"numPendingTests":0,"testResults":[{"name":"test.js","assertionResults":[{"fullName":"test passes","status":"passed"},{"fullName":"test fails","status":"failed","failureMessages":["Error: assert failed"]}]}]}`
	report := parseJestJSON(data)
	if report.Failed != 1 {
		t.Errorf("Failed 应为 1, 得到 %d", report.Failed)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("应有 1 个失败用例")
	}
	if report.Failures[0].Message != "Error: assert failed" {
		t.Errorf("失败消息不匹配: %s", report.Failures[0].Message)
	}
}

func TestParseJestJSON_InvalidJSON(t *testing.T) {
	report := parseJestJSON(`bad`)
	if report.Total != 0 {
		t.Errorf("Total 应为 0, 得到 %d", report.Total)
	}
}

// ===================== parseSurefireXML =====================

func TestParseSurefireXML(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.AppTest" tests="3" failures="1" errors="1" skipped="1">
  <testcase name="testPass" classname="com.example.AppTest"/>
  <testcase name="testFail" classname="com.example.AppTest">
    <failure message="expected X but got Y"/>
  </testcase>
  <testcase name="testError" classname="com.example.AppTest">
    <error message="NullPointerException"/>
  </testcase>
</testsuite>`
	var report TestReport
	parseSurefireXML([]byte(xml), &report)

	if report.Total != 3 {
		t.Errorf("Total 应为 3, 得到 %d", report.Total)
	}
	if report.Failed != 2 { // failures(1) + errors(1)
		t.Errorf("Failed 应为 2, 得到 %d", report.Failed)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped 应为 1, 得到 %d", report.Skipped)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("应有 2 个失败详情, 得到 %d", len(report.Failures))
	}
	foundFailure := false
	foundError := false
	for _, f := range report.Failures {
		if f.Test == "testFail" && f.Message == "expected X but got Y" {
			foundFailure = true
		}
		if f.Test == "testError" && f.Message == "NullPointerException" {
			foundError = true
		}
	}
	if !foundFailure {
		t.Error("未找到 testFail 的 failure 记录")
	}
	if !foundError {
		t.Error("未找到 testError 的 error 记录")
	}
}

func TestParseSurefireXML_Empty(t *testing.T) {
	var report TestReport
	parseSurefireXML([]byte(`<testsuite name="empty" tests="0" failures="0" errors="0" skipped="0"/>`), &report)
	if report.Total != 0 {
		t.Error("空报告 Total 应为 0")
	}
}

func TestParseSurefireXML_Invalid(t *testing.T) {
	var report TestReport
	parseSurefireXML([]byte(`not xml`), &report)
	// 无效 XML 不应修改 report
	if report.Total != 0 {
		t.Error("无效 XML 不应修改报告")
	}
}

func TestParseSurefireXML_MergesMultiple(t *testing.T) {
	xml1 := `<testsuite name="Suite1" tests="2" failures="1" errors="0" skipped="0"><testcase name="t1"><failure message="f1"/></testcase></testsuite>`
	xml2 := `<testsuite name="Suite2" tests="3" failures="2" errors="0" skipped="1"><testcase name="t2"><failure message="f2"/></testcase></testsuite>`

	var report TestReport
	parseSurefireXML([]byte(xml1), &report)
	parseSurefireXML([]byte(xml2), &report)

	if report.Total != 5 {
		t.Errorf("Total 应为 5 (2+3), 得到 %d", report.Total)
	}
	if report.Failed != 3 {
		t.Errorf("Failed 应为 3 (1+2), 得到 %d", report.Failed)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped 应为 1, 得到 %d", report.Skipped)
	}
	if len(report.Failures) != 2 {
		t.Errorf("Failures 应为 2, 得到 %d", len(report.Failures))
	}
}

// ===================== parseJacocoXML =====================

func TestParseJacocoXML(t *testing.T) {
	xml := `<?xml version="1.0"?>
<report name="test">
  <counter type="LINE" covered="80" missed="20"/>
  <counter type="BRANCH" covered="10" missed="5"/>
</report>`
	cov := parseJacocoXML([]byte(xml))
	if cov != "80.0%" {
		t.Errorf("覆盖率应为 80.0%%, 得到 %s", cov)
	}
}

func TestParseJacocoXML_NoLineCounter(t *testing.T) {
	xml := `<?xml version="1.0"?><report name="test"><counter type="BRANCH" covered="10" missed="5"/></report>`
	cov := parseJacocoXML([]byte(xml))
	if cov != "" {
		t.Errorf("无 LINE counter 时应返回空字符串, 得到 %s", cov)
	}
}

func TestParseJacocoXML_Invalid(t *testing.T) {
	cov := parseJacocoXML([]byte(`bad xml`))
	if cov != "" {
		t.Errorf("无效 XML 应返回空字符串, 得到 %s", cov)
	}
}

func TestParseJacocoXML_ZeroTotal(t *testing.T) {
	xml := `<?xml version="1.0"?><report name="test"><counter type="LINE" covered="0" missed="0"/></report>`
	cov := parseJacocoXML([]byte(xml))
	if cov != "" {
		t.Errorf("总行数为 0 时应返回空字符串, 得到 %s", cov)
	}
}

// ===================== parseCoverageSummary =====================

func TestParseCoverageSummary(t *testing.T) {
	data := []byte(`{"total":{"lines":{"pct":85.5}}}`)
	cov := parseCoverageSummary(data)
	if cov != "85.5%" {
		t.Errorf("覆盖率应为 85.5%%, 得到 %s", cov)
	}
}

func TestParseCoverageSummary_InvalidJSON(t *testing.T) {
	cov := parseCoverageSummary([]byte(`bad`))
	if cov != "" {
		t.Errorf("无效 JSON 应返回空字符串, 得到 %s", cov)
	}
}

// ===================== extractJSONFromOutput =====================

func TestExtractJSONFromOutput_Found(t *testing.T) {
	output := `some log line
another line
{"numTotalTestSuites":1,"numTotalTests":5}`
	json := extractJSONFromOutput(output)
	if json != `{"numTotalTestSuites":1,"numTotalTests":5}` {
		t.Errorf("提取的 JSON 不匹配: %s", json)
	}
}

func TestExtractJSONFromOutput_NotFound(t *testing.T) {
	json := extractJSONFromOutput("just some output")
	if json != "" {
		t.Errorf("无 JSON 时应返回空字符串, 得到 %s", json)
	}
}

func TestExtractJSONFromOutput_Empty(t *testing.T) {
	json := extractJSONFromOutput("")
	if json != "" {
		t.Errorf("空输出应返回空字符串, 得到 %s", json)
	}
}

func TestExtractJSONFromOutput_LastLineWins(t *testing.T) {
	output := `{"numTotalTestSuites":1,"numTotalTests":3}
intermediate output
{"numTotalTestSuites":1,"numTotalTests":5}`
	json := extractJSONFromOutput(output)
	// 应取最后一行
	if json != `{"numTotalTestSuites":1,"numTotalTests":5}` {
		t.Errorf("应取最后的 JSON, 得到: %s", json)
	}
}

func TestExtractJSONFromOutput_WithBraceInContent(t *testing.T) {
	output := `{"numTotalTestSuites":1,"numTotalTests":1,"testResults":[{"name":"t","assertionResults":[{"status":"passed"}]}]}`
	json := extractJSONFromOutput(output)
	// 应提取完整的 JSON（含嵌套的 }
	if json != output {
		t.Errorf("提取的 JSON 应完整包含嵌套括号")
	}
}
