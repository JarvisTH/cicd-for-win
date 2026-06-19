# ci-mcp-server.ps1
# 通过 stdio 实现 MCP 协议（JSON-RPC 2.0）
# 被 Host（AtomCode/Claude Desktop）自动调用

$ciDir = Split-Path $PSCommandPath -Parent
$ciExe = Join-Path $ciDir "ci.exe"

while ($true) {
    $line = [Console]::In.ReadLine()
    if (-not $line) { continue }

    $request = $line | ConvertFrom-Json

    switch ($request.method) {
        "tools/list" {
            $schema = & $ciExe describe --format mcp 2>&1
            $response = @{ jsonrpc = "2.0"; id = $request.id; result = ($schema | ConvertFrom-Json) }
        }
        "tools/call" {
            $toolName = $request.arguments.name
            $args = $request.arguments.arguments
            $action = $toolName -replace "^ci_", ""

            switch ($action) {
                "passwd" {
                    # ci passwd [username] [password]
                    $username = if ($args.username) { $args.username } else { $null }
                    $password = if ($args.password) { $args.password } else { $null }
                    $result = if ($username -and $password) {
                        & $ciExe passwd $username $password 2>&1
                    } elseif ($username) {
                        & $ciExe passwd $username 2>&1
                    } else {
                        & $ciExe passwd 2>&1
                    }
                }
                "report" {
                    # ci report <project> [--list] [--json] [--delete <id>]
                    if ($args.project) {
                        if ($args.delete) {
                            $result = & $ciExe report $args.project --delete $args.delete 2>&1
                        } else {
                            $result = & $ciExe report $args.project --json 2>&1
                        }
                    } else {
                        $result = & $ciExe report --help 2>&1
                    }
                }
                "serve" {
                    # ci serve [--port 8080]
                    $port = if ($args.port) { $args.port } else { "8080" }
                    $result = & $ciExe serve --port $port --no-open 2>&1
                }
                "list" {
                    # ci list，无参数
                    $result = & $ciExe list --json 2>&1
                }
                "doctor" {
                    # ci doctor [--json]
                    $result = & $ciExe doctor --json 2>&1
                }
                "project_list" {
                    # ci project list [--json]
                    if ($args.json -eq "true") {
                        $result = & $ciExe project list --json 2>&1
                    } else {
                        $result = & $ciExe project list 2>&1
                    }
                }
                default {
                    # check / test / build / push / deploy / hooks / status
                    # 这些工具接受可选的 project 参数
                    if ($args.project) {
                        $result = & $ciExe $action $args.project --json 2>&1
                    } else {
                        $result = & $ciExe $action --json 2>&1
                    }
                }
            }

            $response = @{
                jsonrpc = "2.0"
                id = $request.id
                result = @{ content = @(@{ type = "text"; text = "$result" }) }
            }
        }
        default {
            $response = @{ jsonrpc = "2.0"; id = $request.id; error = @{ code = -32601; message = "未知方法" } }
        }
    }

    $response | ConvertTo-Json -Compress -Depth 5
}
