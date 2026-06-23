package runner

import (
	"testing"
	"time"

	"ci-cd/internal/config"
)

// ===================== RunDeploy =====================

func TestRunDeploy_ThroughExecutor(t *testing.T) {
	mock := &mockExec{}
	old := defaultExec
	defaultExec = mock
	defer func() { defaultExec = old }()

	proj := makeProject("deploy-test")
	result, err := RunDeploy(proj, "staging")
	if err != nil {
		t.Fatalf("RunDeploy 失败: %v", err)
	}
	if result.Action != "deploy" {
		t.Errorf("Action 应为 deploy, 得到 %s", result.Action)
	}
	if len(mock.calls) != 1 {
		t.Errorf("应调用 1 次 executor, 得到 %d", len(mock.calls))
	}
}

// ===================== getRemoteCommands =====================

func TestGetRemoteCommands_Default(t *testing.T) {
	cmds := getRemoteCommands(ProjectTypeUnknown, "/opt/app", &config.DeployConfig{})
	if cmds.UploadTarget != "/opt/app/" {
		t.Errorf("Unknown 类型 upload_target 应为 /opt/app/, 得到 %s", cmds.UploadTarget)
	}
	if cmds.StartCmd != "echo 'no-op'" {
		t.Errorf("Unknown 类型 start_cmd 应为 echo 'no-op', 得到 %s", cmds.StartCmd)
	}
}

func TestGetRemoteCommands_CustomDeploy(t *testing.T) {
	deploy := &config.DeployConfig{
		StartCmd: "systemctl start app",
		StopCmd:  "systemctl stop app",
	}
	cmds := getRemoteCommands(ProjectTypeReact, "/opt/app", deploy)
	if cmds.StartCmd != "systemctl start app" {
		t.Errorf("自定义 start_cmd 应优先, 得到 %s", cmds.StartCmd)
	}
	if cmds.StopCmd != "systemctl stop app" {
		t.Errorf("自定义 stop_cmd 应优先, 得到 %s", cmds.StopCmd)
	}
}

func TestGetRemoteCommands_CustomOnlyStart(t *testing.T) {
	deploy := &config.DeployConfig{StartCmd: "custom-start"}
	cmds := getRemoteCommands(ProjectTypeReact, "/opt/app", deploy)
	if cmds.StartCmd != "custom-start" {
		t.Errorf("自定义 start_cmd 应生效, 得到 %s", cmds.StartCmd)
	}
	if cmds.StopCmd == "" {
		t.Error("stop_cmd 不应为空")
	}
	if cmds.StatusCmd == "" {
		t.Error("status_cmd 不应为空")
	}
}

func TestGetRemoteCommands_Frontend(t *testing.T) {
	cmds := getRemoteCommands(ProjectTypeReact, "/opt/app", &config.DeployConfig{})
	if cmds.UploadTarget != "/opt/app/dist/" {
		t.Errorf("前端 upload_target 应为 /opt/app/dist/, 得到 %s", cmds.UploadTarget)
	}
}

func TestGetRemoteCommands_Maven(t *testing.T) {
	cmds := getRemoteCommands(ProjectTypeMaven, "/opt/app", &config.DeployConfig{})
	if cmds.UploadTarget != "/opt/app/" {
		t.Errorf("Maven upload_target 应为 /opt/app/, 得到 %s", cmds.UploadTarget)
	}
	if cmds.StartCmd == "" || cmds.StartCmd == "echo 'no-op'" {
		t.Error("Maven start_cmd 不应为 no-op")
	}
}

func TestGetRemoteCommands_MavenMulti(t *testing.T) {
	cmds := getRemoteCommands(ProjectTypeMavenMulti, "/opt/app", &config.DeployConfig{})
	if cmds.UploadTarget != "/opt/app/services/" {
		t.Errorf("MavenMulti upload_target 应为 /opt/app/services/, 得到 %s", cmds.UploadTarget)
	}
	if cmds.StartCmd != "cd /opt/app && docker-compose up -d" {
		t.Errorf("MavenMulti start_cmd 不匹配, 得到 %s", cmds.StartCmd)
	}
}

// ===================== findArtifact =====================

func TestFindArtifact_FrontendNoDist(t *testing.T) {
	dir := t.TempDir()
	_, err := findArtifact(dir, ProjectTypeReact)
	if err == nil {
		t.Error("前端无 dist/ 时应返回错误")
	}
}

func TestFindArtifact_MavenNoTarget(t *testing.T) {
	dir := t.TempDir()
	_, err := findArtifact(dir, ProjectTypeMaven)
	if err == nil {
		t.Error("Maven 无 target/*.jar 时应返回错误")
	}
}

func TestFindArtifact_UnknownType(t *testing.T) {
	_, err := findArtifact("/tmp", ProjectTypeUnknown)
	if err == nil {
		t.Error("Unknown 类型应返回错误")
	}
}

// ===================== failResult =====================

func TestDeployFailResult(t *testing.T) {
	r := failResult("connection refused", time.Now())
	if r.Status != "fail" {
		t.Errorf("Status 应为 fail, 得到 %s", r.Status)
	}
	if r.ErrorLog != "connection refused" {
		t.Errorf("ErrorLog 不匹配")
	}
}

// ===================== 辅助函数 =====================

// makeProject 创建一个带默认值的 Project，减少测试重复代码。
func makeProject(name string) config.Project {
	return config.Project{
		Name:  name,
		Path:  "/tmp/" + name,
		CiDir: ".",
		Enabled: true,
	}
}
