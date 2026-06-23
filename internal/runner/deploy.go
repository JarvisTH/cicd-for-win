// deploy.go — 部署逻辑，替代 cd-deploy.ps1 脚本。
// 使用 SSH/SFTP 将构建产物上传到远程服务器并执行启动/停止/状态命令。
package runner

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"ci-cd/internal/config"
	"ci-cd/internal/sshutil"
)

// RunDeployInternal 将构建产物通过 SFTP 上传到远程服务器并启动服务。
// 对应 cd-deploy.ps1 脚本的 upload 逻辑。
func RunDeployInternal(project config.Project, ciDir string) (Result, error) {
	start := time.Now()

	if project.Deploy == nil || project.Deploy.Host == "" {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: "未配置部署信息",
		}, fmt.Errorf("项目 %s 未配置部署信息", project.Name)
	}

	deploy := project.Deploy
	projectType := DetectProjectType(project.Path)

	// 确定构建产物路径
	artifact, err := findArtifact(project.Path, projectType)
	if err != nil {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: err.Error(),
		}, err
	}

	// 获取远程命令模板
	cmds := getRemoteCommands(projectType, deploy.RemoteDir, deploy)

	// 建立 SSH 连接
	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: fmt.Sprintf("SSH 配置失败: %v", err),
		}, err
	}

	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: fmt.Sprintf("SSH 连接失败: %v", err),
		}, err
	}
	defer client.Close()

	// 创建远程目录
	if err := createRemoteDir(client, cmds.UploadTarget); err != nil {
		fmt.Fprintf(logWriter, "  ⚠ 创建远程目录失败: %v\n", err)
	}

	// SFTP 上传
	fmt.Fprintf(logWriter, "[%s] 📤 上传 %s → %s:%s\n", project.Name, artifact, deploy.Host, cmds.UploadTarget)
	if err := sftpUpload(client, artifact, cmds.UploadTarget); err != nil {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: fmt.Sprintf("上传失败: %v", err),
		}, err
	}
	fmt.Fprintf(logWriter, "  ✅ 上传完成\n")

	// 部署后执行启动命令
	fmt.Fprintf(logWriter, "  🚀 正在启动项目（类型: %s）...\n", projectType)
	if cmds.StopCmd != "" && cmds.StopCmd != "echo 'no-op'" {
		fmt.Fprintf(logWriter, "  停用旧服务...\n")
		runSSHCommand(client, cmds.StopCmd)
	}

	if cmds.StartCmd != "" && cmds.StartCmd != "echo 'no-op'" {
		result := runSSHCommand(client, cmds.StartCmd)
		if result.ExitCode == 0 {
			fmt.Fprintf(logWriter, "  ✅ 启动成功\n")
		} else {
			fmt.Fprintf(logWriter, "  ❌ 启动失败: %s\n", result.Stderr)
		}
	}

	return Result{
		Status:   "pass",
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
	}, nil
}

// RunDeployAction 执行指定的部署动作（start/stop/status/test）。
func RunDeployAction(project config.Project, ciDir, action string) (Result, error) {
	start := time.Now()

	if project.Deploy == nil || project.Deploy.Host == "" {
		return Result{
			Status:   "fail",
			Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
			ErrorLog: "未配置部署信息",
		}, fmt.Errorf("项目 %s 未配置部署信息", project.Name)
	}

	deploy := project.Deploy
	projectType := DetectProjectType(project.Path)
	cmds := getRemoteCommands(projectType, deploy.RemoteDir, deploy)

	sshCfg, err := sshutil.BuildSSHConfig(deploy, ciDir)
	if err != nil {
		return failResult(fmt.Sprintf("SSH 配置失败: %v", err), start), err
	}

	addr := fmt.Sprintf("%s:%d", deploy.Host, deploy.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return failResult(fmt.Sprintf("SSH 连接失败: %v", err), start), err
	}
	defer client.Close()

	switch action {
	case "start":
		result := runSSHCommand(client, cmds.StartCmd)
		if result.ExitCode == 0 {
			fmt.Fprintf(logWriter, "✅ 启动成功\n")
		} else {
			fmt.Fprintf(logWriter, "❌ 启动失败\n")
		}
	case "stop":
		result := runSSHCommand(client, cmds.StopCmd)
		if result.ExitCode == 0 {
			fmt.Fprintf(logWriter, "✅ 已停止\n")
		} else {
			fmt.Fprintf(logWriter, "❌ 停止失败\n")
		}
	case "status":
		result := runSSHCommand(client, cmds.StatusCmd)
		fmt.Fprintf(logWriter, "📊 状态: %s\n", strings.TrimSpace(result.Stdout))
	case "test":
		result := runSSHCommand(client, "echo connected")
		if result.ExitCode == 0 {
			fmt.Fprintf(logWriter, "✅ SSH 连接成功\n")
		} else {
			fmt.Fprintf(logWriter, "❌ SSH 连接失败\n")
		}
	}

	return Result{
		Status:   "pass",
		Duration: fmt.Sprintf("%.1fs", time.Since(start).Seconds()),
	}, nil
}

// remoteCommands 保存远程部署的模板命令。
type remoteCommands struct {
	UploadTarget string
	StartCmd     string
	StopCmd      string
	StatusCmd    string
}

// getRemoteCommands 根据项目类型返回远程命令模板。
// 对应 cd-deploy.ps1 的 Get-RemoteCommands 函数。
func getRemoteCommands(projectType ProjectType, remoteDir string, deploy *config.DeployConfig) remoteCommands {
	// 如果 deploy 配置中指定了自定义命令，优先使用
	if deploy.StartCmd != "" || deploy.StopCmd != "" || deploy.StatusCmd != "" {
		startCmd := deploy.StartCmd
		if startCmd == "" {
			startCmd = "echo 'no-op'"
		}
		stopCmd := deploy.StopCmd
		if stopCmd == "" {
			stopCmd = "echo 'no-op'"
		}
		statusCmd := deploy.StatusCmd
		if statusCmd == "" {
			statusCmd = "echo 'unknown'"
		}
		return remoteCommands{
			UploadTarget: remoteDir + "/",
			StartCmd:     startCmd,
			StopCmd:      stopCmd,
			StatusCmd:    statusCmd,
		}
	}

	switch projectType {
	case ProjectTypeReact, ProjectTypeVue:
		return remoteCommands{
			UploadTarget: remoteDir + "/dist/",
			StartCmd:     `if command -v nginx >/dev/null 2>&1; then nginx -s reload && echo 'nginx reloaded' || echo 'nginx reload failed'; elif command -v python3 >/dev/null 2>&1; then cd ` + remoteDir + `/dist && nohup python3 -m http.server 8080 > ` + remoteDir + `/http.log 2>&1 < /dev/null & sleep 1 && echo 'python3 http.server started on 8080'; elif command -v python >/dev/null 2>&1; then cd ` + remoteDir + `/dist && nohup python -m SimpleHTTPServer 8080 > ` + remoteDir + `/http.log 2>&1 < /dev/null & sleep 1 && echo 'python http.server started on 8080'; else echo 'WARN: no nginx/python3/python found, files uploaded only'; fi`,
			StopCmd:     `if command -v nginx >/dev/null 2>&1; then nginx -s stop && echo 'nginx stopped'; else pkill -f 'python3 -m http.server' 2>/dev/null || pkill -f 'python -m SimpleHTTPServer' 2>/dev/null || true; echo 'process stopped'; fi`,
			StatusCmd:   `if command -v nginx >/dev/null 2>&1; then if pgrep nginx >/dev/null 2>&1; then echo 'nginx running'; else echo 'nginx stopped'; fi; elif pgrep -f 'python3 -m http.server' >/dev/null 2>&1; then echo 'python3 http.server running'; elif pgrep -f 'python -m SimpleHTTPServer' >/dev/null 2>&1; then echo 'python http.server running'; else echo 'stopped'; fi`,
		}
	case ProjectTypeMaven:
		return remoteCommands{
			UploadTarget: remoteDir + "/",
			StartCmd:     `rm -f ` + remoteDir + `/app.jar && for f in ` + remoteDir + `/*.jar; do mv "$f" ` + remoteDir + `/app.jar && break; done; nohup java -jar ` + remoteDir + `/app.jar > ` + remoteDir + `/app.log 2>&1 &`,
			StopCmd:      `pkill -f 'java -jar ` + remoteDir + `/app.jar' || true`,
			StatusCmd:    `pgrep -f 'java -jar ` + remoteDir + `/app.jar' && echo 'running' || echo 'stopped'`,
		}
	case ProjectTypeMavenMulti:
		return remoteCommands{
			UploadTarget: remoteDir + "/services/",
			StartCmd:     `cd ` + remoteDir + ` && docker-compose up -d`,
			StopCmd:      `cd ` + remoteDir + ` && docker-compose down`,
			StatusCmd:    `cd ` + remoteDir + ` && docker-compose ps`,
		}
	default:
		return remoteCommands{
			UploadTarget: remoteDir + "/",
			StartCmd:     "echo 'no-op'",
			StopCmd:      "echo 'no-op'",
			StatusCmd:    "echo 'unknown'",
		}
	}
}

// findArtifact 查找项目的构建产物。
func findArtifact(projectPath string, projectType ProjectType) (string, error) {
	switch {
	case isFrontendType(projectType):
		distPath := filepath.Join(projectPath, "dist")
		if fi, err := os.Stat(distPath); err == nil && fi.IsDir() {
			return distPath, nil
		}
		return "", fmt.Errorf("未找到 dist/ 构建产物，请先执行 ci build")
	case isMavenType(projectType):
		jars, err := filepath.Glob(filepath.Join(projectPath, "target", "*.jar"))
		if err != nil {
			return "", fmt.Errorf("读取 target 目录失败: %w", err)
		}
		// 过滤掉 sources/javadoc/original 包
		for _, j := range jars {
			base := filepath.Base(j)
			if !containsAny(base, "sources", "javadoc", "original") {
				return j, nil
			}
		}
		return "", fmt.Errorf("未找到 *.jar 构建产物，请先执行 ci build")
	default:
		return "", fmt.Errorf("不支持的项目类型: %s", projectType)
	}
}

// sftpUpload 通过 SFTP 上传文件或目录。
func sftpUpload(client *ssh.Client, localPath, remoteDir string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("SFTP 连接失败: %w", err)
	}
	defer sftpClient.Close()

	fi, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return sftpUploadDir(sftpClient, localPath, remoteDir)
	}
	return sftpUploadFile(sftpClient, localPath, remoteDir)
}

// sftpUploadFile 上传单个文件。
func sftpUploadFile(client *sftp.Client, localPath, remoteDir string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remotePath := path.Join(remoteDir, filepath.Base(localPath))
	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("创建远程文件 %s 失败: %w", remotePath, err)
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	return err
}

// sftpUploadDir 递归上传目录。
func sftpUploadDir(client *sftp.Client, localDir, remoteDir string) error {
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		localPath := filepath.Join(localDir, entry.Name())
		remotePath := path.Join(remoteDir, entry.Name())
		if entry.IsDir() {
			client.MkdirAll(remotePath)
			if err := sftpUploadDir(client, localPath, remotePath); err != nil {
				return err
			}
		} else {
			if err := sftpUploadFile(client, localPath, remotePath); err != nil {
				return err
			}
		}
	}
	return nil
}

// createRemoteDir 在远程服务器上创建目录。
func createRemoteDir(client *ssh.Client, remoteDir string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()
	return sftpClient.MkdirAll(remoteDir)
}

// sshResult 保存 SSH 命令执行结果。
type sshResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// runSSHCommand 在远程服务器上执行命令。
func runSSHCommand(client *ssh.Client, command string) sshResult {
	session, err := client.NewSession()
	if err != nil {
		return sshResult{ExitCode: -1, Stderr: fmt.Sprintf("创建 session 失败: %v", err)}
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = -1
		}
	}

	return sshResult{
		ExitCode: exitCode,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
	}
}
