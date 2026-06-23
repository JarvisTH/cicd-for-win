// Package runner 提供 CI/CD 流水线的核心执行逻辑。
//
// detect.go — 项目类型检测，从项目目录推断项目类型（React/Vue/Maven 等）。
// 与 ci-runner.ps1 的 Get-ProjectType 逻辑保持一致。
package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ProjectType 表示检测到的项目类型。
type ProjectType string

const (
	ProjectTypeReact      ProjectType = "React"
	ProjectTypeVue         ProjectType = "Vue"
	ProjectTypeAngular    ProjectType = "Angular"
	ProjectTypeNext       ProjectType = "Next"
	ProjectTypeNode       ProjectType = "Node"
	ProjectTypeMaven      ProjectType = "Maven"
	ProjectTypeMavenMulti ProjectType = "MavenMulti"
	ProjectTypeGradle     ProjectType = "Gradle"
	ProjectTypeRust       ProjectType = "Rust"
	ProjectTypeGo         ProjectType = "Go"
	ProjectTypeUnknown    ProjectType = "Unknown"
)

// DetectProjectType 从项目路径推断项目类型。
// 逻辑与 ci-runner.ps1 的 Get-ProjectType 保持一致。
func DetectProjectType(projectPath string) ProjectType {
	// package.json → 前端项目
	pkgFile := filepath.Join(projectPath, "package.json")
	if data, err := os.ReadFile(pkgFile); err == nil {
		return detectFrontendType(data)
	}

	// pom.xml → Maven
	pomFile := filepath.Join(projectPath, "pom.xml")
	if data, err := os.ReadFile(pomFile); err == nil {
		content := string(data)
		if strings.Contains(content, "<modules>") || strings.Contains(content, "<packaging>pom</packaging>") {
			return ProjectTypeMavenMulti
		}
		return ProjectTypeMaven
	}

	// build.gradle → Gradle
	if fileExists(filepath.Join(projectPath, "build.gradle")) {
		return ProjectTypeGradle
	}

	// Cargo.toml → Rust
	if fileExists(filepath.Join(projectPath, "Cargo.toml")) {
		return ProjectTypeRust
	}

	// go.mod → Go
	if fileExists(filepath.Join(projectPath, "go.mod")) {
		return ProjectTypeGo
	}

	return ProjectTypeUnknown
}

// detectFrontendType 解析 package.json 推断前端框架类型。
func detectFrontendType(data []byte) ProjectType {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ProjectTypeNode
	}

	hasDep := func(name string) bool {
		_, ok1 := pkg.Dependencies[name]
		_, ok2 := pkg.DevDependencies[name]
		return ok1 || ok2
	}

	switch {
	case hasDep("react"):
		return ProjectTypeReact
	case hasDep("vue") || hasDep("vue-router"):
		return ProjectTypeVue
	case hasDep("@angular/core"):
		return ProjectTypeAngular
	case hasDep("next"):
		return ProjectTypeNext
	default:
		return ProjectTypeNode
	}
}

// fileExists 检查文件或目录是否存在。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isFrontendType 判断项目类型是否为前端类型（React/Vue/Angular/Next/Node）。
func isFrontendType(t ProjectType) bool {
	switch t {
	case ProjectTypeReact, ProjectTypeVue, ProjectTypeAngular, ProjectTypeNext, ProjectTypeNode:
		return true
	default:
		return false
	}
}

// isMavenType 判断项目类型是否为 Maven 相关类型。
func isMavenType(t ProjectType) bool {
	return t == ProjectTypeMaven || t == ProjectTypeMavenMulti
}
