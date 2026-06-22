package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ===================== IsRuleEnabled =====================

func TestIsRuleEnabled_NoRules(t *testing.T) {
	p := &Project{Name: "test", Rules: nil}
	if !p.IsRuleEnabled("tsc") {
		t.Error("Rules 为 nil 时所有规则应默认启用")
	}
	if !p.IsRuleEnabled("eslint") {
		t.Error("Rules 为 nil 时任意规则应默认启用")
	}

	p2 := &Project{Name: "test", Rules: []RuleState{}}
	if !p2.IsRuleEnabled("checkstyle") {
		t.Error("Rules 为空切片时所有规则应默认启用")
	}
}

func TestIsRuleEnabled_RulesExplicit(t *testing.T) {
	p := &Project{
		Name: "test",
		Rules: []RuleState{
			{ID: "tsc", Enabled: true},
			{ID: "eslint", Enabled: false},
		},
	}

	if !p.IsRuleEnabled("tsc") {
		t.Error("tsc 已启用，应返回 true")
	}
	if p.IsRuleEnabled("eslint") {
		t.Error("eslint 已禁用，应返回 false")
	}
}

func TestIsRuleEnabled_NotListed(t *testing.T) {
	p := &Project{
		Name: "test",
		Rules: []RuleState{
			{ID: "tsc", Enabled: true},
		},
	}
	// 未显式列出的规则默认启用
	if !p.IsRuleEnabled("compile") {
		t.Error("未显式列出的规则应默认启用")
	}
	if !p.IsRuleEnabled("checkstyle") {
		t.Error("未显式列出的规则应默认启用")
	}
}

// ===================== DefaultPipelineSteps =====================

func TestDefaultPipelineSteps(t *testing.T) {
	steps := DefaultPipelineSteps()
	expectedIDs := []string{"check", "build", "test", "push", "deploy"}

	if len(steps) != len(expectedIDs) {
		t.Fatalf("默认步骤数应为 %d, 得到 %d", len(expectedIDs), len(steps))
	}
	for i, id := range expectedIDs {
		if steps[i].ID != id {
			t.Errorf("步骤 %d ID 应为 %q, 得到 %q", i, id, steps[i].ID)
		}
		if !steps[i].Enabled {
			t.Errorf("默认步骤 %q 应为启用状态", id)
		}
	}
}

// ===================== GetEnabledSteps =====================

func TestGetEnabledSteps_NoPipeline(t *testing.T) {
	p := &Project{Name: "test", Pipeline: nil}
	steps := p.GetEnabledSteps()
	expected := []string{"check", "build", "test", "push", "deploy"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("无 Pipeline 配置: 期望 %v, 得到 %v", expected, steps)
	}
}

func TestGetEnabledSteps_EmptyPipeline(t *testing.T) {
	p := &Project{Name: "test", Pipeline: &PipelineConfig{}}
	steps := p.GetEnabledSteps()
	expected := []string{"check", "build", "test", "push", "deploy"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("空 Pipeline 应返回默认步骤: 期望 %v, 得到 %v", expected, steps)
	}
}

func TestGetEnabledSteps_EmptySteps(t *testing.T) {
	p := &Project{
		Name:     "test",
		Pipeline: &PipelineConfig{Steps: []PipelineStep{}},
	}
	steps := p.GetEnabledSteps()
	expected := []string{"check", "build", "test", "push", "deploy"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("空 Steps 应返回默认步骤: 期望 %v, 得到 %v", expected, steps)
	}
}

func TestGetEnabledSteps_AllEnabled(t *testing.T) {
	p := &Project{
		Name: "test",
		Pipeline: &PipelineConfig{
			Steps: []PipelineStep{
				{ID: "check", Enabled: true},
				{ID: "build", Enabled: true},
				{ID: "test", Enabled: true},
				{ID: "push", Enabled: true},
				{ID: "deploy", Enabled: true},
			},
		},
	}
	steps := p.GetEnabledSteps()
	expected := []string{"check", "build", "test", "push", "deploy"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("期望 %v, 得到 %v", expected, steps)
	}
}

func TestGetEnabledSteps_PartialEnabled(t *testing.T) {
	p := &Project{
		Name: "test",
		Pipeline: &PipelineConfig{
			Steps: []PipelineStep{
				{ID: "check", Enabled: true},
				{ID: "build", Enabled: false},
				{ID: "test", Enabled: true},
				{ID: "push", Enabled: false},
				{ID: "deploy", Enabled: true},
			},
		},
	}
	steps := p.GetEnabledSteps()
	expected := []string{"check", "test", "deploy"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("期望 %v, 得到 %v", expected, steps)
	}
}

func TestGetEnabledSteps_CustomOrder(t *testing.T) {
	p := &Project{
		Name: "test",
		Pipeline: &PipelineConfig{
			Steps: []PipelineStep{
				{ID: "deploy", Enabled: true},
				{ID: "check", Enabled: true},
			},
		},
	}
	steps := p.GetEnabledSteps()
	expected := []string{"deploy", "check"}
	if !stringSliceEqual(steps, expected) {
		t.Errorf("应保留自定义顺序: 期望 %v, 得到 %v", expected, steps)
	}
}

// ===================== Filter =====================

func TestFilter_NoArgs(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "proj-a", Enabled: true},
			{Name: "proj-b", Enabled: false},
			{Name: "proj-c", Enabled: true},
		},
	}
	result := cfg.Filter([]string{})
	if len(result) != 2 {
		t.Fatalf("应返回 2 个启用项目, 得到 %d", len(result))
	}
	if result[0].Name != "proj-a" || result[1].Name != "proj-c" {
		t.Errorf("返回了错误的启用项目: %v", result)
	}
}

func TestFilter_ByName(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "proj-a", Enabled: true},
			{Name: "proj-b", Enabled: true},
			{Name: "proj-c", Enabled: false},
		},
	}
	result := cfg.Filter([]string{"proj-a"})
	if len(result) != 1 || result[0].Name != "proj-a" {
		t.Errorf("按名称过滤失败: 得到 %v", result)
	}

	// 禁用项目不应匹配
	result = cfg.Filter([]string{"proj-c", "proj-a"})
	if len(result) != 1 || result[0].Name != "proj-a" {
		t.Errorf("禁用的项目不应返回: 得到 %v", result)
	}
}

func TestFilter_NoMatch(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "proj-a", Enabled: true},
		},
	}
	result := cfg.Filter([]string{"nonexistent"})
	if len(result) != 0 {
		t.Errorf("不存在的项目应返回空: 得到 %v", result)
	}
}

func TestFilter_MultipleNames(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "a", Enabled: true},
			{Name: "b", Enabled: true},
			{Name: "c", Enabled: true},
			{Name: "d", Enabled: false},
		},
	}
	result := cfg.Filter([]string{"a", "c", "d"})
	if len(result) != 2 {
		t.Fatalf("应返回 2 个（a 和 c）, 得到 %d", len(result))
	}
	if result[0].Name != "a" || result[1].Name != "c" {
		t.Errorf("返回了错误的项目: %v", result)
	}
}

func TestFilter_DuplicateName(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{Name: "proj-a", Enabled: true},
			{Name: "proj-a", Enabled: true}, // 同名不同配置
		},
	}
	result := cfg.Filter([]string{"proj-a"})
	// 匹配第一个就 break 了
	if len(result) != 1 {
		t.Fatalf("应返回 1 个（break 后只返回第一个匹配）, 得到 %d", len(result))
	}
}

// ===================== normalize (DeployConfig) =====================

func TestNormalize_Defaults(t *testing.T) {
	d := &DeployConfig{}
	d.normalize()
	if d.Port != 22 {
		t.Errorf("端口应默认为 22, 得到 %d", d.Port)
	}
	if d.AuthType != "key" {
		t.Errorf("认证类型应默认为 key, 得到 %q", d.AuthType)
	}
}

func TestNormalize_PreservesValues(t *testing.T) {
	d := &DeployConfig{Port: 2222, AuthType: "password"}
	d.normalize()
	if d.Port != 2222 {
		t.Errorf("已设置的端口不应被覆盖: 得到 %d", d.Port)
	}
	if d.AuthType != "password" {
		t.Errorf("已设置的认证类型不应被覆盖: 得到 %q", d.AuthType)
	}
}

func TestNormalize_PortZeroOnly(t *testing.T) {
	d := &DeployConfig{Port: 0, AuthType: "password"}
	d.normalize()
	if d.Port != 22 {
		t.Errorf("Port 为 0 时应默认 22, 得到 %d", d.Port)
	}
	if d.AuthType != "password" {
		t.Errorf("已设置的 AuthType 不应被覆盖: 得到 %q", d.AuthType)
	}
}

// ===================== Load =====================

func TestLoad_Success(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "projects.json")
	content := `{
		"projects": [
			{"name": "test-proj", "path": "/tmp/test", "enabled": true}
		]
	}`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("应加载 1 个项目, 得到 %d", len(cfg.Projects))
	}
	if cfg.Projects[0].Name != "test-proj" {
		t.Errorf("项目名称不匹配: 得到 %q", cfg.Projects[0].Name)
	}
	if cfg.Projects[0].CiDir == "" {
		t.Error("CiDir 应被自动填充")
	}
}

func TestLoad_WithDeployConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "projects.json")
	content := `{
		"projects": [
			{
				"name": "deploy-proj",
				"path": "/tmp/app",
				"enabled": true,
				"deploy": {
					"host": "example.com",
					"user": "admin",
					"remote_dir": "/opt/app"
				}
			}
		]
	}`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Projects[0].Deploy == nil {
		t.Fatal("Deploy 配置不应为 nil")
	}
	// Port 应被 normalize 为 22
	if cfg.Projects[0].Deploy.Port != 22 {
		t.Errorf("端口应被 normalize 为 22, 得到 %d", cfg.Projects[0].Deploy.Port)
	}
	// AuthType 应被 normalize 为 "key"
	if cfg.Projects[0].Deploy.AuthType != "key" {
		t.Errorf("AuthType 应被 normalize 为 key, 得到 %q", cfg.Projects[0].Deploy.AuthType)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/projects.json")
	if err == nil {
		t.Fatal("加载不存在的文件应返回错误")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "projects.json")
	os.WriteFile(configPath, []byte("{invalid json}"), 0644)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("加载非法 JSON 应返回错误")
	}
}

// ===================== StepDefaultCommands =====================

func TestStepDefaultCommands(t *testing.T) {
	expectedKeys := []string{"check", "build", "test", "push", "deploy"}
	for _, key := range expectedKeys {
		if _, ok := StepDefaultCommands[key]; !ok {
			t.Errorf("StepDefaultCommands 缺少键: %q", key)
		}
	}
	// 验证描述非空
	for k, v := range StepDefaultCommands {
		if v == "" {
			t.Errorf("StepDefaultCommands[%q] 描述为空", k)
		}
	}
}

// ===================== RemoteConfig & RuleState =====================

func TestRemoteConfig(t *testing.T) {
	r := RemoteConfig{Name: "origin", URL: "git@github.com:user/repo.git", Enabled: true}
	if r.Name != "origin" {
		t.Error("Name 字段不匹配")
	}
	if !r.Enabled {
		t.Error("Enabled 应为 true")
	}

	r2 := RemoteConfig{Name: "backup", URL: "git@backup:repo.git", Enabled: false}
	if r2.Enabled {
		t.Error("Enabled 应为 false")
	}
}

func TestRuleState(t *testing.T) {
	r := RuleState{ID: "tsc", Enabled: true}
	if r.ID != "tsc" || !r.Enabled {
		t.Error("RuleState 字段不匹配")
	}
}

// ===================== 辅助函数 =====================

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
