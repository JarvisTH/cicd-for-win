param(
    [ValidateSet("check","build","test")]
    [string]$Action = "check",
    [string]$ProjectPath = (Get-Location).Path,
    [switch]$Json,
    [string]$CustomCommand = "",  # 自定义命令（为空时使用默认逻辑）
    [string]$CustomArgs = "",     # 自定义额外参数
    [string]$RuleStates = ""      # 代码检查规则开关，JSON 数组：[{"id":"tsc","enabled":true}]
)

$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# 全局错误日志收集器（替代 Start-Transcript，避免污染 stdout）
$script:errorLog = [System.Collections.ArrayList]@()

# Invoke-CmdSafe 安全执行命令并捕获输出到 errorLog
# 不使用 Write-Output（避免污染 stdout 导致 JSON 解析失败），输出仅在失败时收集到 errorLog
function Invoke-CmdSafe($cmd, $cmdArgs) {
    $output = & $cmd @cmdArgs 2>&1
    $code = $LASTEXITCODE
    if ($code -ne 0) {
        # 失败时收集完整输出到 errorLog
        $script:errorLog.AddRange(@($output))
        # 打印前 5 行和后 10 行的摘要到 stderr（不污染 stdout）
        $lines = @($output)
        $total = $lines.Count
        if ($total -le 20) {
            $lines | ForEach-Object { [Console]::Error.WriteLine($_) }
        } else {
            $lines[0..4] | ForEach-Object { [Console]::Error.WriteLine($_) }
            [Console]::Error.WriteLine("... ($total lines total, showing first 5 and last 10) ...")
            $lines[($total-10)..($total-1)] | ForEach-Object { [Console]::Error.WriteLine($_) }
        }
    }
    return $code
}

function Get-ProjectType($projectPath) {
    $pkgFile = "$projectPath/package.json"
    if (Test-Path $pkgFile) {
        $pkg = Get-Content $pkgFile -Raw | ConvertFrom-Json
        # 将 dependencies 和 devDependencies 的属性名收集到 hashtable（兼容 PSObject）
        $depNames = @{}
        if ($pkg.dependencies) { $pkg.dependencies.PSObject.Properties | ForEach-Object { $depNames[$_.Name] = $true } }
        if ($pkg.devDependencies) { $pkg.devDependencies.PSObject.Properties | ForEach-Object { $depNames[$_.Name] = $true } }
        if ($depNames.ContainsKey("react")) { return "React" }
        if ($depNames.ContainsKey("vue") -or $depNames.ContainsKey("vue-router")) { return "Vue" }
        if ($depNames.ContainsKey("@angular/core")) { return "Angular" }
        if ($depNames.ContainsKey("next")) { return "Next" }
        return "Node"
    }
    if (Test-Path "$projectPath/pom.xml") {
        $pom = [xml](Get-Content "$projectPath/pom.xml")
        if ($pom.project.packaging -eq "pom" -or $pom.project.modules) { return "MavenMulti" }
        return "Maven"
    }
    if (Test-Path "$projectPath/build.gradle") { return "Gradle" }
    if (Test-Path "$projectPath/Cargo.toml") { return "Rust" }
    if (Test-Path "$projectPath/go.mod") { return "Go" }
    return "Unknown"
}

# Test-RuleEnabled 判断指定规则 id 是否启用。
# $states 为 hashtable（id -> bool）；为空或不含该 id 时默认启用。
function Test-RuleEnabled($states, $id) {
    if (-not $states) { return $true }
    if ($states.ContainsKey($id)) { return [bool]$states[$id] }
    return $true
}

function Invoke-Check($projectPath, $ruleStates) {
    $type = Get-ProjectType($projectPath)
    $rulesDir = Join-Path $PSScriptRoot "rules"
    [Console]::Error.WriteLine("[$type] 开始代码检查...")

    # 切换到项目目录执行，确保 tsc/eslint/mvn 能找到项目配置文件和源码
    Push-Location $projectPath
    try {
        switch ($type) {
            "React" {
                if (Test-RuleEnabled $ruleStates 'tsc') {
                    $code = Invoke-CmdSafe "npx.cmd" @("tsc","--noEmit")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ TypeScript 类型检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过 TypeScript 类型检查（已禁用）") }
                if (Test-RuleEnabled $ruleStates 'eslint') {
                    $code = Invoke-CmdSafe "npx.cmd" @("eslint","src/")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ ESLint 检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过 ESLint 检查（已禁用）") }
            }
            "Vue" {
                if (Test-RuleEnabled $ruleStates 'tsc') {
                    $code = Invoke-CmdSafe "npx.cmd" @("vue-tsc","--noEmit")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ vue-tsc 类型检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过 vue-tsc 类型检查（已禁用）") }
                if (Test-RuleEnabled $ruleStates 'eslint') {
                    $eslintConfig = Join-Path $rulesDir "eslint-vue.mjs"
                    $code = Invoke-CmdSafe "npx.cmd" @("eslint","-c",$eslintConfig,"src/")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ ESLint 检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过 ESLint 检查（已禁用）") }
            }
            "Maven" {
                if (Test-RuleEnabled $ruleStates 'compile') {
                    $code = Invoke-CmdSafe "mvn.cmd" @("compile","-q")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ 编译检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过编译检查（已禁用）") }
                if (Test-RuleEnabled $ruleStates 'checkstyle') {
                    $checkstyleConfig = Join-Path $rulesDir "checkstyle.xml"
                    $code = Invoke-CmdSafe "mvn.cmd" @("checkstyle:check","-Dcheckstyle.config=$checkstyleConfig")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ Checkstyle 检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过 Checkstyle 检查（已禁用）") }
            }
            "MavenMulti" {
                if (Test-RuleEnabled $ruleStates 'compile') {
                    $code = Invoke-CmdSafe "mvn.cmd" @("compile","-q")
                    if ($code -ne 0) { [Console]::Error.WriteLine("❌ 多模块编译检查失败"); return $false }
                } else { [Console]::Error.WriteLine("⊘ 跳过多模块编译检查（已禁用）") }
            }
            default {
                [Console]::Error.WriteLine("未知项目类型: $type，跳过检查")
                return $true
            }
        }
    } finally {
        Pop-Location
    }
    [Console]::Error.WriteLine("✅ 代码检查通过")
    return $true
}

function Invoke-Build($projectPath) {
    $type = Get-ProjectType($projectPath)
    [Console]::Error.WriteLine("[$type] 开始构建...")

    Push-Location $projectPath
    try {
        switch ($type) {
            "React" {
                $code = Invoke-CmdSafe "npm.cmd" @("run","build")
                if ($code -ne 0) { [Console]::Error.WriteLine("❌ npm run build 失败"); return $false }
                if (-not (Test-Path "dist/index.html")) { [Console]::Error.WriteLine("❌ 产物 dist/index.html 不存在"); return $false }
                [Console]::Error.WriteLine("✅ 构建成功: dist/")
            }
            "Vue" {
                $code = Invoke-CmdSafe "npm.cmd" @("run","build")
                if ($code -ne 0) { [Console]::Error.WriteLine("❌ npm run build 失败"); return $false }
                if (-not (Test-Path "dist/index.html")) { [Console]::Error.WriteLine("❌ 产物 dist/index.html 不存在"); return $false }
                [Console]::Error.WriteLine("✅ 构建成功: dist/")
            }
            "Maven" {
                $code = Invoke-CmdSafe "mvn.cmd" @("clean","package","-DskipTests","-q")
                if ($code -ne 0) { [Console]::Error.WriteLine("❌ Maven 构建失败"); return $false }
                $jar = Get-ChildItem "target/*.jar" | Where-Object { $_.Name -notmatch 'sources|javadoc|original' } | Select-Object -First 1
                if (-not $jar) { [Console]::Error.WriteLine("❌ 未找到 *.jar 产物"); return $false }
                [Console]::Error.WriteLine("✅ 构建成功: $($jar.Name)")
            }
            "MavenMulti" {
                $code = Invoke-CmdSafe "mvn.cmd" @("clean","install","-DskipTests","-q")
                if ($code -ne 0) { [Console]::Error.WriteLine("❌ 多模块构建失败"); return $false }
                [Console]::Error.WriteLine("✅ 全部模块构建成功")
            }
            default {
                [Console]::Error.WriteLine("未知项目类型: $type，跳过构建")
                return $false
            }
        }
    } finally {
        Pop-Location
    }
    return $true
}

function Invoke-Test($projectPath) {
    $type = Get-ProjectType($projectPath)
    [Console]::Error.WriteLine("[$type] 开始测试...")

    $report = @{ total = 0; passed = 0; failed = 0; skipped = 0; coverage = ""; failures = @(); raw_log = "" }

    switch ($type) {
        "React" {
            # 检测测试框架
            $pkgPath = "$projectPath/package.json"
            $pkg = Get-Content $pkgPath -Raw | ConvertFrom-Json
            $isJest = ($pkg.devDependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'jest' }) -or ($pkg.dependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'jest' })
            $isVitest = ($pkg.devDependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' }) -or ($pkg.dependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' })

            if ($isVitest) {
                [Console]::Error.WriteLine("  检测到 Vitest")
                $output = & "npx.cmd" vitest run --reporter=json 2>&1
                $exitCode = $LASTEXITCODE
                # 尝试从 stdout 提取 JSON（vitest --reporter=json 输出到最后一行）
                $jsonLines = $output | Where-Object { $_ -match '^\{"numTotalTestSuites"' } | Select-Object -Last 1
                if ($jsonLines) {
                    try {
                        $r = $jsonLines | ConvertFrom-Json
                        $report.total = $r.numTotalTests
                        $report.passed = $r.numPassedTests
                        $report.failed = $r.numFailedTests
                        $report.skipped = $r.numPendingTests
                        foreach ($suite in $r.testResults) {
                            foreach ($t in $suite.assertionResults) {
                                if ($t.status -eq 'failed') {
                                    $report.failures += @{ suite = $suite.name.Split('/')[-1]; test = $t.fullName; message = $t.failureMessages[0] -replace "`n"," " }
                                }
                            }
                        }
                    } catch { [Console]::Error.WriteLine("解析 Vitest JSON 失败") }
                }
                # 覆盖率
                $covFile = "$projectPath/coverage/coverage-summary.json"
                if (Test-Path $covFile) {
                    try {
                        $cov = Get-Content $covFile -Raw | ConvertFrom-Json
                        $lines = $cov.total.lines.pct
                        $report.coverage = "$($lines)%"
                    } catch { [Console]::Error.WriteLine("解析覆盖率失败") }
                }
            } elseif ($isJest) {
                [Console]::Error.WriteLine("  检测到 Jest")
                $output = & "npx.cmd" jest --json --coverage 2>&1
                $exitCode = $LASTEXITCODE
                # Jest 输出 JSON 在 stdout 最后一段
                $jsonStart = $output.IndexOf('{"numTotalTestSuites":')
                if ($jsonStart -ge 0) {
                    $jsonStr = $output.Substring($jsonStart)
                    $endIdx = $jsonStr.LastIndexOf('}')
                    if ($endIdx -ge 0) { $jsonStr = $jsonStr.Substring(0, $endIdx + 1) }
                    try {
                        $r = $jsonStr | ConvertFrom-Json
                        $report.total = $r.numTotalTests
                        $report.passed = $r.numPassedTests
                        $report.failed = $r.numFailedTests
                        $report.skipped = $r.numPendingTests
                        foreach ($suite in $r.testResults) {
                            foreach ($t in $suite.assertionResults) {
                                if ($t.status -eq 'failed') {
                                    $report.failures += @{ suite = $suite.name.Split('/')[-1]; test = $t.fullName; message = $t.failureMessages[0] -replace "`n"," " }
                                }
                            }
                        }
                    } catch { [Console]::Error.WriteLine("解析 Jest JSON 失败") }
                }
            } else {
                # 兜底：直接跑 npm test
                $output = & "npm.cmd" test 2>&1
                $exitCode = $LASTEXITCODE
            }
            $report.raw_log = ($output | Out-String).Trim()
        }
        "Vue" {
            # Vue 项目同 React 检测逻辑
            $pkgPath = "$projectPath/package.json"
            $pkg = Get-Content $pkgPath -Raw | ConvertFrom-Json
            $isVitest = ($pkg.devDependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' }) -or ($pkg.dependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' })

            if ($isVitest) {
                [Console]::Error.WriteLine("  检测到 Vitest")
                $output = & "npx.cmd" vitest run --reporter=json 2>&1
                $exitCode = $LASTEXITCODE
                $jsonLines = $output | Where-Object { $_ -match '^\{"numTotalTestSuites"' } | Select-Object -Last 1
                if ($jsonLines) {
                    try {
                        $r = $jsonLines | ConvertFrom-Json
                        $report.total = $r.numTotalTests
                        $report.passed = $r.numPassedTests
                        $report.failed = $r.numFailedTests
                        $report.skipped = $r.numPendingTests
                        foreach ($suite in $r.testResults) {
                            foreach ($t in $suite.assertionResults) {
                                if ($t.status -eq 'failed') {
                                    $report.failures += @{ suite = $suite.name.Split('/')[-1]; test = $t.fullName; message = $t.failureMessages[0] -replace "`n"," " }
                                }
                            }
                        }
                    } catch { [Console]::Error.WriteLine("解析 Vitest JSON 失败") }
                }
                $covFile = "$projectPath/coverage/coverage-summary.json"
                if (Test-Path $covFile) {
                    try { $cov = Get-Content $covFile -Raw | ConvertFrom-Json; $report.coverage = "$($cov.total.lines.pct)%" } catch {}
                }
            } else {
                $output = & "npm.cmd" test 2>&1
                $exitCode = $LASTEXITCODE
            }
            $report.raw_log = ($output | Out-String).Trim()
        }
        "Maven" {
            Push-Location $projectPath
            $output = & "mvn.cmd" test "-Dmaven.test.failure.ignore=true" 2>&1
            $exitCode = $LASTEXITCODE
            Pop-Location
            $report.raw_log = ($output | Out-String).Trim()

            # 解析 Surefire XML 报告
            $surefireDir = "$projectPath/target/surefire-reports"
            if (Test-Path $surefireDir) {
                $xmlFiles = Get-ChildItem "$surefireDir/TEST-*.xml"
                foreach ($xmlFile in $xmlFiles) {
                    try {
                        [xml]$xml = Get-Content $xmlFile.FullName
                        $ts = $xml.testsuite
                        $report.total += [int]$ts.tests
                        $report.failed += [int]$ts.failures + [int]$ts.errors
                        $report.skipped += [int]$ts.skipped
                        if ($ts.testcase) {
                            foreach ($tc in $ts.testcase) {
                                if ($tc.failure -or $tc.error) {
                                    $msg = if ($tc.failure) { $tc.failure.message } else { $tc.error.message }
                                    $report.failures += @{ suite = $ts.name; test = $tc.name; message = "$msg" }
                                }
                            }
                        }
                    } catch { [Console]::Error.WriteLine("解析 $($xmlFile.Name) 失败: $_") }
                }
                $report.passed = $report.total - $report.failed - $report.skipped
            }

            # 尝试 JaCoCo 覆盖率
            $jacocoReport = "$projectPath/target/site/jacoco/jacoco.xml"
            if (Test-Path $jacocoReport) {
                try {
                    [xml]$xml = Get-Content $jacocoReport
                    $counters = $xml.report.counter
                    $lineCounter = $counters | Where-Object { $_.type -eq 'LINE' }
                    if ($lineCounter) {
                        $covered = [int]$lineCounter.covered
                        $missed = [int]$lineCounter.missed
                        $total = $covered + $missed
                        if ($total -gt 0) { $report.coverage = "{0:N1}%" -f (($covered / $total) * 100) }
                    }
                } catch { [Console]::Error.WriteLine("解析 JaCoCo 失败") }
            }
        }
        "MavenMulti" {
            Push-Location $projectPath
            $output = & "mvn.cmd" test "-Dmaven.test.failure.ignore=true" 2>&1
            $exitCode = $LASTEXITCODE
            Pop-Location
            $report.raw_log = ($output | Out-String).Trim()
            # 多模块：遍历所有子模块的 surefire 报告
            $modules = Get-ChildItem "$projectPath" -Directory | Where-Object { Test-Path "$($_.FullName)/pom.xml" }
            foreach ($mod in $modules) {
                $surefireDir = "$($mod.FullName)/target/surefire-reports"
                if (Test-Path $surefireDir) {
                    $xmlFiles = Get-ChildItem "$surefireDir/TEST-*.xml"
                    foreach ($xmlFile in $xmlFiles) {
                        try {
                            [xml]$xml = Get-Content $xmlFile.FullName
                            $ts = $xml.testsuite
                            $report.total += [int]$ts.tests
                            $report.failed += [int]$ts.failures + [int]$ts.errors
                            $report.skipped += [int]$ts.skipped
                            if ($ts.testcase) {
                                foreach ($tc in $ts.testcase) {
                                    if ($tc.failure -or $tc.error) {
                                        $msg = if ($tc.failure) { $tc.failure.message } else { $tc.error.message }
                                        $report.failures += @{ suite = "$($ts.name) [$($mod.Name)]"; test = $tc.name; message = "$msg" }
                                    }
                                }
                            }
                        } catch {}
                    }
                    $report.passed = $report.total - $report.failed - $report.skipped
                }
            }
        }
        default {
            [Console]::Error.WriteLine("未知项目类型: $type，跳过测试")
            return $false, $null
        }
    }

    # 输出结果摘要到 stderr
    if ($report.total -gt 0) {
        [Console]::Error.WriteLine("📊 测试结果: $($report.passed)/$($report.total) 通过")
        if ($report.coverage) { [Console]::Error.WriteLine("📈 覆盖率: $($report.coverage)") }
        if ($report.failed -gt 0) {
            [Console]::Error.WriteLine("❌ $($report.failed) 个失败:")
            foreach ($f in $report.failures) {
                [Console]::Error.WriteLine("   - [$($f.suite)] $($f.test): $($f.message)")
            }
        }
    } else {
        [Console]::Error.WriteLine("⚠️ 未检测到测试用例或测试框架")
    }

    return ($report.failed -eq 0), $report
}


$start = Get-Date
$result = $false
$testReport = $null

# 如果指定了自定义命令，直接执行（跳过自动检测逻辑）
if ($CustomCommand -ne "") {
    [Console]::Error.WriteLine("[$Action] 执行自定义命令: $CustomCommand $CustomArgs")
    # 将命令与参数拆分为数组，用 & 直接调用而非 Invoke-Expression，避免命令注入
    # CustomArgs 按空格拆分为独立参数
    $cmdParts = @($CustomCommand)
    if ($CustomArgs) {
        $cmdParts += ($CustomArgs -split '\s+' | Where-Object { $_ })
    }
    if ($Action -eq "test") {
        # test 步骤需要尝试解析 JSON 报告
        Push-Location $ProjectPath
        try {
            $output = & $cmdParts[0] @($cmdParts[1..($cmdParts.Count-1)]) 2>&1
            $exitCode = $LASTEXITCODE
        } finally {
            Pop-Location
        }
        $result = ($exitCode -eq 0)
        # 尝试从输出中提取 JSON 测试报告
        $jsonLine = $output | Where-Object { $_ -match '^\{"numTotalTestSuites"' } | Select-Object -Last 1
        if ($jsonLine) {
            try {
                $r = $jsonLine | ConvertFrom-Json
                $testReport = @{
                    total = $r.numTotalTests; passed = $r.numPassedTests
                    failed = $r.numFailedTests; skipped = $r.numPendingTests
                    coverage = ""; failures = @(); raw_log = ($output | Out-String).Trim()
                }
                foreach ($suite in $r.testResults) {
                    foreach ($t in $suite.assertionResults) {
                        if ($t.status -eq 'failed') {
                            $testReport.failures += @{ suite = $suite.name.Split('/')[-1]; test = $t.fullName; message = ($t.failureMessages[0] -replace "`n"," ") }
                        }
                    }
                }
            } catch { [Console]::Error.WriteLine("解析自定义测试命令的 JSON 输出失败") }
        }
    } else {
        Push-Location $ProjectPath
        try {
            & $cmdParts[0] @($cmdParts[1..($cmdParts.Count-1)]) 2>&1 | ForEach-Object { [Console]::Error.WriteLine($_) }
            $result = ($LASTEXITCODE -eq 0)
        } finally {
            Pop-Location
        }
    }
    if ($result) { Write-Output "✅ 自定义命令执行成功" }
    else { [Console]::Error.WriteLine("❌ 自定义命令执行失败") }
} else {
    # 解析规则开关 JSON（形如 [{"id":"tsc","enabled":true},{"id":"eslint","enabled":false}]）为 hashtable
    $ruleStatesHash = $null
    if ($RuleStates -ne "") {
        try {
            $ruleArr = $RuleStates | ConvertFrom-Json
            $ruleStatesHash = @{}
            foreach ($item in $ruleArr) {
                $ruleStatesHash[$item.id] = [bool]$item.enabled
            }
        } catch { [Console]::Error.WriteLine("解析 RuleStates 失败: $_") }
    }

    switch ($Action) {
        "check" { $result = Invoke-Check $ProjectPath $ruleStatesHash }
        "build" { $result = Invoke-Build $ProjectPath }
        "test"  { $result, $testReport = Invoke-Test $ProjectPath }
    }
}

$duration = (Get-Date) - $start
if ($Json) {
    $output = @{
        project  = Split-Path $ProjectPath -Leaf
        action   = $Action
        status   = if ($result) { "pass" } else { "fail" }
        duration = "{0:N1}s" -f $duration.TotalSeconds
    }
    if ($testReport) { $output.report = $testReport }
    # 失败时从 $script:errorLog 收集错误详情，供前端展示
    if (-not $result -and $script:errorLog.Count -gt 0) {
        $errLog = ($script:errorLog -join "`n").Trim()
        if ($errLog.Length -gt 5000) { $errLog = $errLog.Substring($errLog.Length - 5000) }
        $output.error_log = $errLog
    }
    # 使用 -Compress 输出单行 JSON，避免 Write-Output 输出干扰 JSON 解析
    $output | ConvertTo-Json -Depth 10 -Compress
}
exit $(if ($result) { 0 } else { 1 })
