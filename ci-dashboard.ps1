Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$script:projectList = @()
$script:projectStatus = @{}
$script:runspaces = @{}
$script:configPath = Join-Path $PSScriptRoot "projects.json"

function Write-Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "HH:mm:ss"
    if ($script:logBox) {
        $script:logBox.AppendText("[$timestamp] $Message`r`n")
        $script:logBox.ScrollToCaret()
    }
}

function Load-Projects {
    if (Test-Path $script:configPath) {
        $config = Get-Content $script:configPath -Raw -Encoding UTF8 | ConvertFrom-Json
        return $config.projects
    }
    return @()
}

function Save-Projects {
    $config = @{ projects = $script:projectList }
    $config | ConvertTo-Json -Depth 5 | Out-File -Encoding UTF8 $script:configPath
}

function Show-ProjectDialog {
    param($Project = $null)
    $dialog = New-Object System.Windows.Forms.Form
    $dialog.Text = if ($Project) { "Edit: $($Project.name)" } else { "Add Project" }
    $dialog.Size = New-Object System.Drawing.Size(500,320)
    $dialog.StartPosition = "CenterParent"
    $dialog.FormBorderStyle = "FixedDialog"
    $dialog.MaximizeBox = $false

    $lblName = New-Object System.Windows.Forms.Label; $lblName.Text = "Name:"; $lblName.Location = New-Object System.Drawing.Point(10,15); $lblName.Size = New-Object System.Drawing.Size(60,25)
    $txtName = New-Object System.Windows.Forms.TextBox; $txtName.Location = New-Object System.Drawing.Point(80,13); $txtName.Size = New-Object System.Drawing.Size(200,25)
    if ($Project) { $txtName.Text = $Project.name }

    $lblPath = New-Object System.Windows.Forms.Label; $lblPath.Text = "Path:"; $lblPath.Location = New-Object System.Drawing.Point(10,45); $lblPath.Size = New-Object System.Drawing.Size(60,25)
    $txtPath = New-Object System.Windows.Forms.TextBox; $txtPath.Location = New-Object System.Drawing.Point(80,43); $txtPath.Size = New-Object System.Drawing.Size(300,25)
    if ($Project) { $txtPath.Text = $Project.path }

    $btnBrowse = New-Object System.Windows.Forms.Button; $btnBrowse.Text = "..."; $btnBrowse.Location = New-Object System.Drawing.Point(385,42); $btnBrowse.Size = New-Object System.Drawing.Size(30,25)
    $btnBrowse.Add_Click({ $f = New-Object System.Windows.Forms.FolderBrowserDialog; if ($f.ShowDialog() -eq "OK") { $txtPath.Text = $f.SelectedPath } })

    $grp = New-Object System.Windows.Forms.GroupBox; $grp.Text = "Deploy"; $grp.Location = New-Object System.Drawing.Point(12,80); $grp.Size = New-Object System.Drawing.Size(460,130)
    $lblHost = New-Object System.Windows.Forms.Label; $lblHost.Text = "Host:"; $lblHost.Location = New-Object System.Drawing.Point(10,25); $lblHost.Size = New-Object System.Drawing.Size(60,25)
    $txtHost = New-Object System.Windows.Forms.TextBox; $txtHost.Location = New-Object System.Drawing.Point(75,23); $txtHost.Size = New-Object System.Drawing.Size(180,25)
    $lblPort = New-Object System.Windows.Forms.Label; $lblPort.Text = "Port:"; $lblPort.Location = New-Object System.Drawing.Point(270,25); $lblPort.Size = New-Object System.Drawing.Size(40,25)
    $txtPort = New-Object System.Windows.Forms.TextBox; $txtPort.Text = "22"; $txtPort.Location = New-Object System.Drawing.Point(310,23); $txtPort.Size = New-Object System.Drawing.Size(60,25)
    $lblUser = New-Object System.Windows.Forms.Label; $lblUser.Text = "User:"; $lblUser.Location = New-Object System.Drawing.Point(10,55); $lblUser.Size = New-Object System.Drawing.Size(60,25)
    $txtUser = New-Object System.Windows.Forms.TextBox; $txtUser.Location = New-Object System.Drawing.Point(75,53); $txtUser.Size = New-Object System.Drawing.Size(180,25)
    $lblRemote = New-Object System.Windows.Forms.Label; $lblRemote.Text = "Remote:"; $lblRemote.Location = New-Object System.Drawing.Point(10,85); $lblRemote.Size = New-Object System.Drawing.Size(60,25)
    $txtRemote = New-Object System.Windows.Forms.TextBox; $txtRemote.Location = New-Object System.Drawing.Point(75,83); $txtRemote.Size = New-Object System.Drawing.Size(300,25)
    if ($Project -and $Project.deploy) { $txtHost.Text = $Project.deploy.host; $txtPort.Text = $Project.deploy.port; $txtUser.Text = $Project.deploy.user; $txtRemote.Text = $Project.deploy.remote_dir }
    $grp.Controls.AddRange(@($lblHost, $txtHost, $lblPort, $txtPort, $lblUser, $txtUser, $lblRemote, $txtRemote))

    $btnOK = New-Object System.Windows.Forms.Button; $btnOK.Text = "Save"; $btnOK.Location = New-Object System.Drawing.Point(280,220); $btnOK.Size = New-Object System.Drawing.Size(90,30); $btnOK.DialogResult = "OK"
    $btnCancel = New-Object System.Windows.Forms.Button; $btnCancel.Text = "Cancel"; $btnCancel.Location = New-Object System.Drawing.Point(380,220); $btnCancel.Size = New-Object System.Drawing.Size(90,30); $btnCancel.DialogResult = "Cancel"
    $dialog.Controls.AddRange(@($lblName, $txtName, $lblPath, $txtPath, $btnBrowse, $grp, $btnOK, $btnCancel))

    if ($dialog.ShowDialog() -eq "OK") {
        return @{ name = $txtName.Text; path = $txtPath.Text; enabled = $true; deploy = @{ host = $txtHost.Text; port = [int]$txtPort.Text; user = $txtUser.Text; remote_dir = $txtRemote.Text; auth_type = "key" } }
    }
    return $null
}

function Initialize-Form {
    $form = New-Object System.Windows.Forms.Form
    $form.Text = "CI/CD Control Panel"
    $form.Size = New-Object System.Drawing.Size(900,550)
    $form.StartPosition = "CenterScreen"

    $split = New-Object System.Windows.Forms.SplitContainer
    $split.Dock = "Fill"; $split.Orientation = "Horizontal"; $split.SplitterDistance = 350

    $topPanel = New-Object System.Windows.Forms.Panel; $topPanel.Dock = "Fill"
    $script:projectListView = New-Object System.Windows.Forms.ListView
    $script:projectListView.Dock = "Fill"
    $script:projectListView.View = "Details"; $script:projectListView.FullRowSelect = $true
    $script:projectListView.Columns.Add("Project", 200); $script:projectListView.Columns.Add("Type", 100); $script:projectListView.Columns.Add("Status", 150)

    $btnPanel = New-Object System.Windows.Forms.Panel; $btnPanel.Dock = "Bottom"; $btnPanel.Height = 40
    $btnCheck = New-Object System.Windows.Forms.Button; $btnCheck.Text = "Check All"; $btnCheck.Location = New-Object System.Drawing.Point(10,8); $btnCheck.Size = New-Object System.Drawing.Size(80,25)
    $btnCheck.Add_Click({
        foreach ($proj in $script:projectList) {
            $script:projectStatus[$proj.name].Status = "running"; Write-Log "[$($proj.name)] Check..."
            $ps = [powershell]::Create()
            $ps.AddScript({ param($p) & "powershell.exe" -ExecutionPolicy Bypass -File (Join-Path (Split-Path $PSCommandPath -Parent) "ci-runner.ps1") -Action check -ProjectPath $p }).AddArgument($proj.path) | Out-Null
            $script:runspaces["$($proj.name):check"] = @{ PS = $ps; AR = $ps.BeginInvoke() }
        }
    })
    $btnBuild = New-Object System.Windows.Forms.Button; $btnBuild.Text = "Build All"; $btnBuild.Location = New-Object System.Drawing.Point(100,8); $btnBuild.Size = New-Object System.Drawing.Size(80,25)
    $btnBuild.Add_Click({
        foreach ($proj in $script:projectList) {
            $script:projectStatus[$proj.name].Status = "running"; Write-Log "[$($proj.name)] Build..."
            $ps = [powershell]::Create()
            $ps.AddScript({ param($p) & "powershell.exe" -ExecutionPolicy Bypass -File (Join-Path (Split-Path $PSCommandPath -Parent) "ci-runner.ps1") -Action build -ProjectPath $p }).AddArgument($proj.path) | Out-Null
            $script:runspaces["$($proj.name):build"] = @{ PS = $ps; AR = $ps.BeginInvoke() }
        }
    })
    $btnAdd = New-Object System.Windows.Forms.Button; $btnAdd.Text = "+ Add"; $btnAdd.Location = New-Object System.Drawing.Point(190,8); $btnAdd.Size = New-Object System.Drawing.Size(60,25)
    $btnAdd.Add_Click({ $p = Show-ProjectDialog; if ($p) { $script:projectList += $p; $script:projectStatus[$p.name] = @{ Status = "idle" }; Save-Projects; Write-Log "Added: $($p.name)" } })
    $btnPanel.Controls.AddRange(@($btnCheck, $btnBuild, $btnAdd))
    $topPanel.Controls.Add($script:projectListView); $topPanel.Controls.Add($btnPanel)
    $split.Panel1.Controls.Add($topPanel)

    $script:logBox = New-Object System.Windows.Forms.RichTextBox
    $script:logBox.Dock = "Fill"; $script:logBox.ReadOnly = $true; $script:logBox.BackColor = "Black"; $script:logBox.ForeColor = "LimeGreen"
    $split.Panel2.Controls.Add($script:logBox)
    $form.Controls.Add($split)

    $timer = New-Object System.Windows.Forms.Timer; $timer.Interval = 500
    $timer.Add_Tick({
        $done = @()
        foreach ($id in $script:runspaces.Keys) {
            $t = $script:runspaces[$id]
            if ($t.AR.IsCompleted) { try { $t.PS.EndInvoke($t.AR); Write-Log "[$id] OK" } catch { Write-Log "[$id] FAIL: $_" }; $t.PS.Dispose(); $done += $id }
        }
        $done | ForEach-Object { $script:runspaces.Remove($_) }
    })
    $timer.Start()
    $form.ShowDialog()
}

# Entry
$script:projectList = Load-Projects
if ($script:projectList.Count -eq 0) {
    if ([System.Windows.Forms.MessageBox]::Show("No projects configured. Add one now?", "First Run", "YesNo") -eq "Yes") {
        $p = Show-ProjectDialog; if ($p) { $script:projectList += $p; Save-Projects }
    }
}
foreach ($proj in $script:projectList) {
    if (-not $script:projectStatus.ContainsKey($proj.name)) { $script:projectStatus[$proj.name] = @{ Status = "idle" } }
}
Initialize-Form
