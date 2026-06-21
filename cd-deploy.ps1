param(
    [Parameter(Mandatory)]
    [string]$ProjectName,
    [Parameter(Mandatory)]
    [ValidateSet("upload","start","stop","status","test")]
    [string]$Action,
    [string]$ConfigPath
)

# 设置输出编码为 UTF-8，避免中文乱码
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

if (-not $ConfigPath) { $ConfigPath = Join-Path $PSScriptRoot "projects.json" }

function Read-ProjectConfig {
    param([string]$Name)
    $config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
    return $config.projects | Where-Object { $_.name -eq $Name } | Select-Object -First 1
}

function Invoke-SFTPUpload {
    param($DeployConfig, [string]$LocalPath, [string]$RemotePath)

    # OpenSSH 在 Windows 上的安装路径
    # 注意：32-bit PowerShell 访问 System32 会被重定向到 SysWOW64（无 OpenSSH）
    # 因此需要同时检查 System32（原生）和 SysNative（32→64 位桥接）
    $sshDir = "$env:SystemRoot\System32\OpenSSH"
    if (-not (Test-Path "$sshDir\sftp.exe")) {
        $sshDir = "$env:SystemRoot\SysNative\OpenSSH"
    }
    $sftpPath = Join-Path $sshDir "sftp.exe"
    $scpPath  = Join-Path $sshDir "scp.exe"

    $sshPath = Join-Path $sshDir "ssh.exe"

    if ($DeployConfig.auth_type -eq "key") {
        $sshOpts = @("-oPort=$($DeployConfig.port)", "-oStrictHostKeyChecking=accept-new", "-oUserKnownHostsFile=$PSScriptRoot\.known_hosts", "-i", "$($DeployConfig.identity_file)")
    } else {
        $sshOpts = @("-oPort=$($DeployConfig.port)", "-oStrictHostKeyChecking=accept-new", "-oUserKnownHostsFile=$PSScriptRoot\.known_hosts")
    }

    # 先通过 SSH 创建远程目录
    & $sshPath @sshOpts "$($DeployConfig.user)@$($DeployConfig.host)" "mkdir -p $RemotePath" 2>&1

    if (Test-Path $sftpPath) {
        $batchFile = [System.IO.Path]::GetTempFileName() + ".txt"
        # SSH 的 mkdir -p 已创建目录，SFTP batch 只需 put
        @"
put -r "$LocalPath" "$RemotePath"
bye
"@ | Out-File -Encoding ASCII $batchFile
        & $sftpPath @sshOpts "-b" $batchFile "$($DeployConfig.user)@$($DeployConfig.host)" 2>&1
        $success = $LASTEXITCODE -eq 0
        Remove-Item $batchFile -Force
        if (-not $success) { Write-Output "  ❌ sftp 上传失败" }
    } elseif (Test-Path $scpPath) {
        Write-Output "  ⚠ sftp 不可用，使用 scp 上传..."
        & $scpPath @sshOpts "-r" "$LocalPath" "$($DeployConfig.user)@$($DeployConfig.host):$RemotePath" 2>&1
        $success = $LASTEXITCODE -eq 0
    } else {
        Write-Error "未找到 scp.exe 或 sftp.exe，请安装 OpenSSH 客户端"
        $success = $false
    }
    return $success
}

function Invoke-SSHCommand {
    param($DeployConfig, [string]$Command, [int]$TimeoutSeconds = 60)
    # 解析 OpenSSH 路径（与 Invoke-SFTPUpload 保持一致）
    $sshDir = "$env:SystemRoot\System32\OpenSSH"
    if (-not (Test-Path "$sshDir\ssh.exe")) {
        $sshDir = "$env:SystemRoot\SysNative\OpenSSH"
    }
    $sshExe = Join-Path $sshDir "ssh.exe"
    if (-not (Test-Path $sshExe)) { $sshExe = "ssh.exe" } # 兜底：用 PATH

    # StrictHostKeyChecking=accept-new: 首次接受并记录主机密钥，后续变更拒绝（防中间人攻击）
    if ($DeployConfig.auth_type -eq "key") {
        $sshArgs = "-oPort=$($DeployConfig.port) -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" -i `"$($DeployConfig.identity_file)`" $($DeployConfig.user)@$($DeployConfig.host) `"$Command`""
    } else {
        $sshArgs = "-oPort=$($DeployConfig.port) -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" $($DeployConfig.user)@$($DeployConfig.host) `"$Command`""
    }
    $outFile = "$env:TEMP\ssh_out_$([System.Guid]::NewGuid()).txt"
    $errFile = "$env:TEMP\ssh_err_$([System.Guid]::NewGuid()).txt"
    $proc = Start-Process -FilePath $sshExe -ArgumentList $sshArgs -NoNewWindow -RedirectStandardOutput $outFile -RedirectStandardError $errFile -PassThru
    $proc | Wait-Process -Timeout $TimeoutSeconds
    if (-not $proc.HasExited) { $proc.Kill(); return $false }
    $output = Get-Content $outFile -Raw
    Remove-Item $outFile, $errFile -Force -ErrorAction SilentlyContinue
    return @{ Success = $proc.ExitCode -eq 0; Output = $output }
}

function Get-RemoteCommands {
    param([string]$ProjectType, [string]$RemoteDir, $DeployConfig)

    # 如果 deploy 配置中指定了自定义命令，优先使用
    if ($DeployConfig.start_cmd -or $DeployConfig.stop_cmd -or $DeployConfig.status_cmd) {
        return @{
            upload_target = "$RemoteDir/";
            start_cmd     = if ($DeployConfig.start_cmd)  { $DeployConfig.start_cmd }  else { "echo 'no-op'" }
            stop_cmd      = if ($DeployConfig.stop_cmd)   { $DeployConfig.stop_cmd }   else { "echo 'no-op'" }
            status_cmd    = if ($DeployConfig.status_cmd) { $DeployConfig.status_cmd } else { "echo 'unknown'" }
        }
    }

    switch ($ProjectType) {
        "React" {
            return @{
                upload_target = "$RemoteDir/dist/"
                start_cmd = "if command -v nginx >/dev/null 2>&1; then nginx -s reload && echo 'nginx reloaded' || echo 'nginx reload failed'; elif command -v python3 >/dev/null 2>&1; then cd $RemoteDir/dist && nohup python3 -m http.server 8080 > $RemoteDir/http.log 2>&1 < /dev/null & sleep 1 && echo 'python3 http.server started on 8080'; elif command -v python >/dev/null 2>&1; then cd $RemoteDir/dist && nohup python -m SimpleHTTPServer 8080 > $RemoteDir/http.log 2>&1 < /dev/null & sleep 1 && echo 'python http.server started on 8080'; else echo 'WARN: no nginx/python3/python found, files uploaded only'; fi"
                stop_cmd = "if command -v nginx >/dev/null 2>&1; then nginx -s stop && echo 'nginx stopped'; else pkill -f 'python3 -m http.server' 2>/dev/null || pkill -f 'python -m SimpleHTTPServer' 2>/dev/null || true; echo 'process stopped'; fi"
                status_cmd = "if command -v nginx >/dev/null 2>&1; then if pgrep nginx >/dev/null 2>&1; then echo 'nginx running'; else echo 'nginx stopped'; fi; elif pgrep -f 'python3 -m http.server' >/dev/null 2>&1; then echo 'python3 http.server running'; elif pgrep -f 'python -m SimpleHTTPServer' >/dev/null 2>&1; then echo 'python http.server running'; else echo 'stopped'; fi"
            }
        }
        "Vue" {
            return @{
                upload_target = "$RemoteDir/dist/"
                start_cmd = "if command -v nginx >/dev/null 2>&1; then nginx -s reload && echo 'nginx reloaded' || echo 'nginx reload failed'; elif command -v python3 >/dev/null 2>&1; then cd $RemoteDir/dist && nohup python3 -m http.server 8080 > $RemoteDir/http.log 2>&1 < /dev/null & sleep 1 && echo 'python3 http.server started on 8080'; elif command -v python >/dev/null 2>&1; then cd $RemoteDir/dist && nohup python -m SimpleHTTPServer 8080 > $RemoteDir/http.log 2>&1 < /dev/null & sleep 1 && echo 'python http.server started on 8080'; else echo 'WARN: no nginx/python3/python found, files uploaded only'; fi"
                stop_cmd = "if command -v nginx >/dev/null 2>&1; then nginx -s stop && echo 'nginx stopped'; else pkill -f 'python3 -m http.server' 2>/dev/null || pkill -f 'python -m SimpleHTTPServer' 2>/dev/null || true; echo 'process stopped'; fi"
                status_cmd = "if command -v nginx >/dev/null 2>&1; then if pgrep nginx >/dev/null 2>&1; then echo 'nginx running'; else echo 'nginx stopped'; fi; elif pgrep -f 'python3 -m http.server' >/dev/null 2>&1; then echo 'python3 http.server running'; elif pgrep -f 'python -m SimpleHTTPServer' >/dev/null 2>&1; then echo 'python http.server running'; else echo 'stopped'; fi"
            }
        }
        "Maven" {
            return @{ upload_target = "$RemoteDir/"; start_cmd = "rm -f $RemoteDir/app.jar && for f in $RemoteDir/*.jar; do mv \$f $RemoteDir/app.jar && break; done; nohup java -jar $RemoteDir/app.jar > $RemoteDir/app.log 2>&1 &"; stop_cmd = "pkill -f 'java -jar $RemoteDir/app.jar' || true"; status_cmd = "pgrep -f 'java -jar $RemoteDir/app.jar' && echo 'running' || echo 'stopped'" }
        }
        "MavenMulti" {
            return @{ upload_target = "$RemoteDir/services/"; start_cmd = "cd $RemoteDir && docker-compose up -d"; stop_cmd = "cd $RemoteDir && docker-compose down"; status_cmd = "cd $RemoteDir && docker-compose ps" }
        }
        default { return @{ upload_target = "$RemoteDir/"; start_cmd = "echo 'no-op'"; stop_cmd = "echo 'no-op'"; status_cmd = "echo 'unknown'" } }
    }
}

# Get-ProjectType 从项目路径推断项目类型
function Get-ProjectType {
    param([string]$ProjectPath)
    if (Test-Path "$ProjectPath/package.json") { return "React" }
    elseif (Test-Path "$ProjectPath/pom.xml") {
        # 检测是否为多模块 Maven 项目
        $pom = [xml](Get-Content "$ProjectPath/pom.xml")
        if ($pom.project.modules -and $pom.project.modules.module) { return "MavenMulti" }
        return "Maven"
    }
    return $null
}

$proj = Read-ProjectConfig $ProjectName
if (-not $proj) { Write-Error "未找到项目: $ProjectName"; exit 1 }

$projectType = Get-ProjectType $proj.path
if (-not $projectType) {
    # fallback: 从 projects.json 的 type 字段推断
    $projectType = $proj.type
}
if (-not $projectType) { $projectType = "Unknown" }

$remoteCmds = Get-RemoteCommands -ProjectType $projectType -RemoteDir $proj.deploy.remote_dir -DeployConfig $proj.deploy

switch ($Action) {
    "test" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command "echo connected"
        if ($result.Success) { Write-Output "✅ SSH 连接成功" } else { Write-Output "❌ SSH 连接失败"; exit 1 }
    }
    "upload" {
        $artifact = $null
        if (Test-Path "$($proj.path)/dist") { $artifact = "$($proj.path)/dist" }
        else {
            $jars = Get-ChildItem "$($proj.path)/target/*.jar" | Where-Object { $_.Name -notmatch 'sources|javadoc' } | Sort-Object LastWriteTime -Descending
            if ($jars) { $artifact = $jars[0].FullName }
        }
        if (-not $artifact) { Write-Error "未找到构建产物，请先执行 ci build"; exit 1 }
        Invoke-SFTPUpload -DeployConfig $proj.deploy -LocalPath $artifact -RemotePath $remoteCmds.upload_target
        if ($LASTEXITCODE -ne 0) { Write-Error "上传失败"; exit 1 }
        Write-Output "✅ 上传完成"

        # 上传成功后根据项目类型执行不同的启动方式
        Write-Output "🚀 正在启动项目（类型: $projectType）..."
        switch ($projectType) {
            "Maven" {
                # Maven 项目：先停旧进程再启新进程
                Write-Output "  停用旧服务..."
                Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.stop_cmd | Out-Null
                Start-Sleep -Seconds 1
                $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.start_cmd
                if ($result.Success) { Write-Output "✅ 启动成功" } else { Write-Output "❌ 启动失败" }
            }
            "MavenMulti" {
                # Docker Compose：up -d 会自动重建容器
                $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.start_cmd
                if ($result.Success) { Write-Output "✅ Docker 容器已启动" } else { Write-Output "❌ 启动失败" }
            }
            default {
                # React/Vue 等前端项目：直接 reload nginx
                $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.start_cmd
                if ($result.Success) { Write-Output "✅ nginx 已 reload" } else { Write-Output "❌ nginx reload 失败" }
            }
        }
    }
    "start" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.start_cmd
        if ($result.Success) { Write-Output "✅ 启动成功" } else { Write-Output "❌ 启动失败" }
    }
    "stop" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.stop_cmd
        if ($result.Success) { Write-Output "✅ 已停止" } else { Write-Output "❌ 停止失败" }
    }
    "status" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.status_cmd
        Write-Output "📊 状态: $($result.Output)"
    }
}
