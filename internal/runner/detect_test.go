package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===================== DetectProjectType =====================

func TestDetectProjectType_React(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"18.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeReact {
		t.Errorf("期望 React, 得到 %s", pt)
	}
}

func TestDetectProjectType_Vue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"vue":"3.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeVue {
		t.Errorf("期望 Vue, 得到 %s", pt)
	}
}

func TestDetectProjectType_VueRouter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"vue-router":"4.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeVue {
		t.Errorf("期望 Vue (via vue-router), 得到 %s", pt)
	}
}

func TestDetectProjectType_Angular(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"@angular/core":"15.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeAngular {
		t.Errorf("期望 Angular, 得到 %s", pt)
	}
}

func TestDetectProjectType_Next(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"next":"13.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeNext {
		t.Errorf("期望 Next, 得到 %s", pt)
	}
}

func TestDetectProjectType_NodePlain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"lodash":"4.0.0"}}`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeNode {
		t.Errorf("期望 Node, 得到 %s", pt)
	}
}

func TestDetectProjectType_Maven(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pom.xml"), `<project><groupId>com.test</groupId><artifactId>test</artifactId><version>1.0</version></project>`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeMaven {
		t.Errorf("期望 Maven, 得到 %s", pt)
	}
}

func TestDetectProjectType_MavenMulti(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pom.xml"), `<project><groupId>com.test</groupId><artifactId>parent</artifactId><version>1.0</version><packaging>pom</packaging><modules><module>mod-a</module></modules></project>`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeMavenMulti {
		t.Errorf("期望 MavenMulti, 得到 %s", pt)
	}
}

func TestDetectProjectType_Gradle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "build.gradle"), `apply plugin: 'java'`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeGradle {
		t.Errorf("期望 Gradle, 得到 %s", pt)
	}
}

func TestDetectProjectType_Rust(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), `[package]\nname = "test"`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeRust {
		t.Errorf("期望 Rust, 得到 %s", pt)
	}
}

func TestDetectProjectType_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module test`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeGo {
		t.Errorf("期望 Go, 得到 %s", pt)
	}
}

func TestDetectProjectType_Unknown(t *testing.T) {
	dir := t.TempDir()
	pt := DetectProjectType(dir)
	if pt != ProjectTypeUnknown {
		t.Errorf("期望 Unknown, 得到 %s", pt)
	}
}

func TestDetectProjectType_PackageJSONPriority(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"18.0.0"}}`)
	writeFile(t, filepath.Join(dir, "pom.xml"), `<project><groupId>com.test</groupId><artifactId>test</artifactId></project>`)
	pt := DetectProjectType(dir)
	if pt != ProjectTypeReact {
		t.Errorf("package.json 应优先于 pom.xml，得到 %s", pt)
	}
}

// ===================== detectFrontendType =====================

func TestDetectFrontendType_React(t *testing.T) {
	data := []byte(`{"dependencies":{"react":"18.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeReact {
		t.Errorf("期望 React, 得到 %s", pt)
	}
}

func TestDetectFrontendType_ReactDevDep(t *testing.T) {
	data := []byte(`{"devDependencies":{"react":"18.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeReact {
		t.Errorf("devDependencies 中的 react 也应识别, 得到 %s", pt)
	}
}

func TestDetectFrontendType_Vue(t *testing.T) {
	data := []byte(`{"dependencies":{"vue":"3.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeVue {
		t.Errorf("期望 Vue, 得到 %s", pt)
	}
}

func TestDetectFrontendType_VueRouter(t *testing.T) {
	data := []byte(`{"dependencies":{"vue-router":"4.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeVue {
		t.Errorf("期望 Vue (via vue-router), 得到 %s", pt)
	}
}

func TestDetectFrontendType_Angular(t *testing.T) {
	data := []byte(`{"dependencies":{"@angular/core":"15.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeAngular {
		t.Errorf("期望 Angular, 得到 %s", pt)
	}
}

func TestDetectFrontendType_Next(t *testing.T) {
	data := []byte(`{"dependencies":{"next":"13.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeNext {
		t.Errorf("期望 Next, 得到 %s", pt)
	}
}

func TestDetectFrontendType_Node(t *testing.T) {
	data := []byte(`{"dependencies":{"lodash":"4.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeNode {
		t.Errorf("期望 Node, 得到 %s", pt)
	}
}

func TestDetectFrontendType_EmptyDeps(t *testing.T) {
	data := []byte(`{}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeNode {
		t.Errorf("空依赖应返回 Node, 得到 %s", pt)
	}
}

func TestDetectFrontendType_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeNode {
		t.Errorf("无效 JSON 应返回 Node, 得到 %s", pt)
	}
}

func TestDetectFrontendType_ReactTakesPriority(t *testing.T) {
	data := []byte(`{"dependencies":{"react":"18.0.0","vue":"3.0.0"}}`)
	pt := detectFrontendType(data)
	if pt != ProjectTypeReact {
		t.Errorf("react 应优先于 vue, 得到 %s", pt)
	}
}

func TestDetectFrontendType_PriorityOrder(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected ProjectType
	}{
		{"react > others", `{"dependencies":{"@angular/core":"15","next":"13","vue":"3","react":"18"}}`, ProjectTypeReact},
		{"vue > others(no react)", `{"dependencies":{"@angular/core":"15","next":"13","vue":"3"}}`, ProjectTypeVue},
		{"angular > next", `{"dependencies":{"@angular/core":"15","next":"13"}}`, ProjectTypeAngular},
		{"next > node", `{"dependencies":{"next":"13"}}`, ProjectTypeNext},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pt := detectFrontendType([]byte(tc.json))
			if pt != tc.expected {
				t.Errorf("期望 %s, 得到 %s", tc.expected, pt)
			}
		})
	}
}

// ===================== isFrontendType / isMavenType =====================

func TestIsFrontendType(t *testing.T) {
	tests := []struct {
		pt   ProjectType
		want bool
	}{
		{ProjectTypeReact, true},
		{ProjectTypeVue, true},
		{ProjectTypeAngular, true},
		{ProjectTypeNext, true},
		{ProjectTypeNode, true},
		{ProjectTypeMaven, false},
		{ProjectTypeMavenMulti, false},
		{ProjectTypeGradle, false},
		{ProjectTypeRust, false},
		{ProjectTypeGo, false},
		{ProjectTypeUnknown, false},
	}
	for _, tc := range tests {
		got := isFrontendType(tc.pt)
		if got != tc.want {
			t.Errorf("isFrontendType(%s) = %v, 期望 %v", tc.pt, got, tc.want)
		}
	}
}

func TestIsMavenType(t *testing.T) {
	tests := []struct {
		pt   ProjectType
		want bool
	}{
		{ProjectTypeMaven, true},
		{ProjectTypeMavenMulti, true},
		{ProjectTypeReact, false},
		{ProjectTypeNode, false},
		{ProjectTypeUnknown, false},
	}
	for _, tc := range tests {
		got := isMavenType(tc.pt)
		if got != tc.want {
			t.Errorf("isMavenType(%s) = %v, 期望 %v", tc.pt, got, tc.want)
		}
	}
}

// ===================== fileExists =====================

func TestFileExists_True(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	writeFile(t, f, "hello")
	if !fileExists(f) {
		t.Error("存在的文件应返回 true")
	}
}

func TestFileExists_False(t *testing.T) {
	if fileExists("/nonexistent/path/xyz") {
		t.Error("不存在的文件应返回 false")
	}
}

func TestFileExists_Dir(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Error("存在的目录应返回 true")
	}
}

// ===================== isRuleEnabled =====================

func TestIsRuleEnabled_Empty(t *testing.T) {
	if !isRuleEnabled(nil, "tsc") {
		t.Error("空 states 时所有规则默认启用")
	}
	if !isRuleEnabled(map[string]bool{}, "tsc") {
		t.Error("空 map 时所有规则默认启用")
	}
}

func TestIsRuleEnabled_ExplicitlyEnabled(t *testing.T) {
	states := map[string]bool{"tsc": true}
	if !isRuleEnabled(states, "tsc") {
		t.Error("显式启用的规则应返回 true")
	}
}

func TestIsRuleEnabled_ExplicitlyDisabled(t *testing.T) {
	states := map[string]bool{"eslint": false}
	if isRuleEnabled(states, "eslint") {
		t.Error("显式禁用的规则应返回 false")
	}
}

func TestIsRuleEnabled_NotListed(t *testing.T) {
	states := map[string]bool{"tsc": true}
	if !isRuleEnabled(states, "eslint") {
		t.Error("未列出的规则应默认启用")
	}
}

// ===================== containsAny =====================

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s       string
		targets []string
		want    bool
	}{
		{"hello-world", []string{"hello"}, true},
		{"sources.jar", []string{"sources", "javadoc"}, true},
		{"app.jar", []string{"sources", "javadoc"}, false},
		{"", []string{"a"}, false},
		{"abc", nil, false},
	}
	for _, tc := range tests {
		got := containsAny(tc.s, tc.targets...)
		if got != tc.want {
			t.Errorf("containsAny(%q, %v) = %v, 期望 %v", tc.s, tc.targets, got, tc.want)
		}
	}
}

// ===================== failResult =====================

func TestFailResult(t *testing.T) {
	r := failResult("something went wrong", time.Now())
	if r.Status != "fail" {
		t.Errorf("Status 应为 fail, 得到 %s", r.Status)
	}
	if r.ErrorLog != "something went wrong" {
		t.Errorf("ErrorLog 不匹配")
	}
	if r.Duration == "" {
		t.Error("Duration 不应为空")
	}
}

// ===================== 辅助函数 =====================

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入文件 %s 失败: %v", path, err)
	}
}
