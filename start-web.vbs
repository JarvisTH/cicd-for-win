' CI/CD 静默启动器（无 cmd 窗口）
' 双击此文件将在后台启动 CI/CD Web 服务，仅通过系统托盘图标交互
' 构建仍请使用 build-and-run-web.bat（需要看到编译输出）

Set sh = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
appDir = Left(WScript.ScriptFullName, InStrRev(WScript.ScriptFullName, "\"))
exePath = appDir & "ci.exe"

' 若 ci.exe 不存在，先静默构建一次
If Not fso.FileExists(exePath) Then
    ' 设置 GOROOT 适配自定义 Go 安装路径
    sh.Environment("Process").Item("GOROOT") = "D:\software\go"
    sh.Environment("Process").Item("PATH") = "D:\software\go\bin;" & sh.Environment("Process").Item("PATH")
    sh.CurrentDirectory = appDir
    sh.Run "cmd /c go build -o """ & exePath & """ .\cmd\ci", 0, True
End If

If Not fso.FileExists(exePath) Then
    MsgBox "ci.exe 不存在且自动构建失败，请先运行 build-and-run-web.bat", vbCritical, "CI/CD"
    WScript.Quit 1
End If

' 先关闭可能残留的旧进程，释放端口
sh.Run "taskkill /f /im ci.exe", 0, True

' 后台启动 serve（窗口模式 0=隐藏，False=不等待）
sh.Run """" & exePath & """ serve", 0, False
