param(
    [Parameter(Mandatory)]
    [string]$ProjectName,
    [Parameter(Mandatory)]
    [ValidateSet("upload","start","stop","status","test")]
    [string]$Action,
    [string]$ConfigPath
)

if (-not $ConfigPath) { $ConfigPath = Join-Path $PSScriptRoot "projects.json" }

function Read-ProjectConfig {
    param([string]$Name)
    $config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
    return $config.projects | Where-Object { $_.name -eq $Name } | Select-Object -First 1
}

function Invoke-SFTPUpload {
    param($DeployConfig, [string]$LocalPath, [string]$RemotePath)
    $batchFile = [System.IO.Path]::GetTempFileName() + ".txt"
    @"
put -r "$LocalPath" "$RemotePath"
bye
"@ | Out-File -Encoding ASCII $batchFile

    # StrictHostKeyChecking=accept-new: 首次连接自动接受主机密钥并记录，后续变更则拒绝（防中间人攻击）
    if ($DeployConfig.auth_type -eq "key") {
        $sftpArgs = "-oPort=$($DeployConfig.port) -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" -i `"$($DeployConfig.identity_file)`" -b $batchFile $($DeployConfig.user)@$($DeployConfig.host)"
    } else {
        Write-Warning "密码模式不支持 SFTP，降级为密钥模式"
        $sftpArgs = "-oPort=$($DeployConfig.port) -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" -b $batchFile $($DeployConfig.user)@$($DeployConfig.host)"
    }
    & "sftp.exe" $sftpArgs 2>&1
    Remove-Item $batchFile -Force
    return $LASTEXITCODE -eq 0
}

function Invoke-SSHCommand {
    param($DeployConfig, [string]$Command, [int]$TimeoutSeconds = 60)
    # StrictHostKeyChecking=accept-new: 首次接受并记录主机密钥，后续变更拒绝（防中间人攻击）
    if ($DeployConfig.auth_type -eq "key") {
        $sshArgs = "-oPort=$($DeployConfig.port) -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" -i `"$($DeployConfig.identity_file)`" $($DeployConfig.user)@$($DeployConfig.host) `"$Command`""
    } else {
        $sshArgs = "-oPort=$($DeployConfig.port) -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=`"$PSScriptRoot\.known_hosts`" $($DeployConfig.user)@$($DeployConfig.host) `"$Command`""
    }
    $outFile = "$env:TEMP\ssh_out_$([System.Guid]::NewGuid()).txt"
    $errFile = "$env:TEMP\ssh_err_$([System.Guid]::NewGuid()).txt"
    $proc = Start-Process -FilePath "ssh.exe" -ArgumentList $sshArgs -NoNewWindow -RedirectStandardOutput $outFile -RedirectStandardError $errFile -PassThru
    $proc | Wait-Process -Timeout $TimeoutSeconds
    if (-not $proc.HasExited) { $proc.Kill(); return $false }
    $output = Get-Content $outFile -Raw
    Remove-Item $outFile, $errFile -Force -ErrorAction SilentlyContinue
    return @{ Success = $proc.ExitCode -eq 0; Output = $output }
}

function Get-RemoteCommands {
    param([string]$ProjectType, [string]$RemoteDir)
    switch ($ProjectType) {
        "React" {
            return @{ upload_target = "$RemoteDir/dist/"; start_cmd = "nginx -s reload"; stop_cmd = "nginx -s stop"; status_cmd = "pgrep nginx && echo 'running' || echo 'stopped'" }
        }
        "Vue" {
            return @{ upload_target = "$RemoteDir/dist/"; start_cmd = "nginx -s reload"; stop_cmd = "nginx -s stop"; status_cmd = "pgrep nginx && echo 'running' || echo 'stopped'" }
        }
        "Maven" {
            return @{ upload_target = "$RemoteDir/"; start_cmd = "nohup java -jar $RemoteDir/*.jar > app.log 2>&1 &"; stop_cmd = "pkill -f 'java -jar $RemoteDir/*.jar' || true"; status_cmd = "pgrep -f 'java -jar $RemoteDir/*.jar' && echo 'running' || echo 'stopped'" }
        }
        "MavenMulti" {
            return @{ upload_target = "$RemoteDir/services/"; start_cmd = "cd $RemoteDir && docker-compose up -d"; stop_cmd = "cd $RemoteDir && docker-compose down"; status_cmd = "cd $RemoteDir && docker-compose ps" }
        }
        default { return @{ upload_target = "$RemoteDir/"; start_cmd = "echo 'no-op'"; stop_cmd = "echo 'no-op'"; status_cmd = "echo 'unknown'" } }
    }
}

$proj = Read-ProjectConfig $ProjectName
if (-not $proj) { Write-Error "未找到项目: $ProjectName"; exit 1 }

$projectType = Get-ProjectType $proj.path
if (-not $projectType) {
    # 从 projects.json 的 path 推断类型（降级）
    if (Test-Path "$($proj.path)/package.json") { $projectType = "React" }
    elseif (Test-Path "$($proj.path)/pom.xml") { $projectType = "Maven" }
    else { $projectType = "Unknown" }
}

$remoteCmds = Get-RemoteCommands -ProjectType $projectType -RemoteDir $proj.deploy.remote_dir

switch ($Action) {
    "test" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command "echo connected"
        if ($result.Success) { Write-Host "✅ SSH 连接成功" } else { Write-Host "❌ SSH 连接失败"; exit 1 }
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
        Write-Host "✅ 上传完成"
    }
    "start" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.start_cmd
        if ($result.Success) { Write-Host "✅ 启动成功" } else { Write-Host "❌ 启动失败" }
    }
    "stop" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.stop_cmd
        if ($result.Success) { Write-Host "✅ 已停止" } else { Write-Host "❌ 停止失败" }
    }
    "status" {
        $result = Invoke-SSHCommand -DeployConfig $proj.deploy -Command $remoteCmds.status_cmd
        Write-Host "📊 状态: $($result.Output)"
    }
}
