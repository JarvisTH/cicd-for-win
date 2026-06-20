function Get-ProjectType($projectPath) {
    $pkgFile = "$projectPath/package.json"
    if (Test-Path $pkgFile) {
        $pkg = Get-Content $pkgFile -Raw | ConvertFrom-Json
        $deps = @{}
        if ($pkg.dependencies) { $deps = $pkg.dependencies }
        if ($pkg.devDependencies) { $pkg.devDependencies.PSObject.Properties | ForEach-Object { $deps[$_.Name] = $_.Value } }
        if ($deps.Keys -contains "react") { return "React" }
        if ($deps.Keys -contains "vue" -or $deps.Keys -contains "vue-router") { return "Vue" }
        if ($deps.Keys -contains "@angular/core") { return "Angular" }
        if ($deps.Keys -contains "next") { return "Next" }
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

function Invoke-Check($projectPath) {
    $type = Get-ProjectType($projectPath)
    $rulesDir = Join-Path $PSScriptRoot "rules"
    Write-Host "[$type] 开始代码检查..."

    switch ($type) {
        "React" {
            & "npx.cmd" tsc --noEmit 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ TypeScript 类型检查失败"; return $false }
            & "npx.cmd" eslint src/ 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ ESLint 检查失败"; return $false }
        }
        "Vue" {
            & "npx.cmd" vue-tsc --noEmit 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ vue-tsc 类型检查失败"; return $false }
            $eslintConfig = Join-Path $rulesDir "eslint-vue.mjs"
            & "npx.cmd" eslint -c "$eslintConfig" src/ 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ ESLint 检查失败"; return $false }
        }
        "Maven" {
            & "mvn.cmd" compile -Xlint:all 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ 编译检查失败"; return $false }
            $checkstyleConfig = Join-Path $rulesDir "checkstyle.xml"
            & "mvn.cmd" checkstyle:check -Dcheckstyle.config="$checkstyleConfig" 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ Checkstyle 检查失败"; return $false }
        }
        "MavenMulti" {
            Push-Location $projectPath
            & "mvn.cmd" compile -Xlint:all 2>&1
            $exitCode = $LASTEXITCODE
            Pop-Location
            if ($exitCode -ne 0) { Write-Host "❌ 多模块编译检查失败"; return $false }
        }
        default {
            Write-Warning "未知项目类型: $type，跳过检查"
            return $true
        }
    }
    Write-Host "✅ 代码检查通过"
    return $true
}

function Invoke-Build($projectPath) {
    $type = Get-ProjectType($projectPath)
    Write-Host "[$type] 开始构建..."

    switch ($type) {
        "React" {
            & "npm.cmd" run build 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ npm run build 失败"; return $false }
            if (-not (Test-Path "$projectPath/dist/index.html")) { Write-Host "❌ 产物 dist/index.html 不存在"; return $false }
            Write-Host "✅ 构建成功: dist/"
        }
        "Vue" {
            & "npm.cmd" run build 2>&1
            if ($LASTEXITCODE -ne 0) { Write-Host "❌ npm run build 失败"; return $false }
            if (-not (Test-Path "$projectPath/dist/index.html")) { Write-Host "❌ 产物 dist/index.html 不存在"; return $false }
            Write-Host "✅ 构建成功: dist/"
        }
        "Maven" {
            Push-Location $projectPath
            & "mvn.cmd" clean package -DskipTests 2>&1
            $exitCode = $LASTEXITCODE
            Pop-Location
            if ($exitCode -ne 0) { Write-Host "❌ Maven 构建失败"; return $false }
            $jar = Get-ChildItem "$projectPath/target/*.jar" | Where-Object { $_.Name -notmatch 'sources|javadoc|original' } | Select-Object -First 1
            if (-not $jar) { Write-Host "❌ 未找到 *.jar 产物"; return $false }
            Write-Host "✅ 构建成功: $($jar.Name)"
        }
        "MavenMulti" {
            Push-Location $projectPath
            & "mvn.cmd" clean install -DskipTests 2>&1
            $exitCode = $LASTEXITCODE
            Pop-Location
            if ($exitCode -ne 0) { Write-Host "❌ 多模块构建失败"; return $false }
            Write-Host "✅ 全部模块构建成功"
        }
        default {
            Write-Warning "未知项目类型: $type，跳过构建"
            return $false
        }
    }
    return $true
}

function Invoke-Test($projectPath) {
    $type = Get-ProjectType($projectPath)
    Write-Host "[$type] 开始测试..."

    $report = @{ total = 0; passed = 0; failed = 0; skipped = 0; coverage = ""; failures = @(); raw_log = "" }

    switch ($type) {
        "React" {
            # 检测测试框架
            $pkgPath = "$projectPath/package.json"
            $pkg = Get-Content $pkgPath -Raw | ConvertFrom-Json
            $isJest = ($pkg.devDependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'jest' }) -or ($pkg.dependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'jest' })
            $isVitest = ($pkg.devDependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' }) -or ($pkg.dependencies | Get-Member -MemberType NoteProperty | Where-Object { $_.Name -match 'vitest' })

            if ($isVitest) {
                Write-Host "  检测到 Vitest"
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
                    } catch { Write-Warning "解析 Vitest JSON 失败" }
                }
                # 覆盖率
                $covFile = "$projectPath/coverage/coverage-summary.json"
                if (Test-Path $covFile) {
                    try {
                        $cov = Get-Content $covFile -Raw | ConvertFrom-Json
                        $lines = $cov.total.lines.pct
                        $report.coverage = "$($lines)%"
                    } catch { Write-Warning "解析覆盖率失败" }
                }
            } elseif ($isJest) {
                Write-Host "  检测到 Jest"
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
                    } catch { Write-Warning "解析 Jest JSON 失败" }
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
                Write-Host "  检测到 Vitest"
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
                    } catch { Write-Warning "解析 Vitest JSON 失败" }
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
            $output = & "mvn.cmd" test -Dmaven.test.failure.ignore=true 2>&1
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
                    } catch { Write-Warning "解析 $($xmlFile.Name) 失败: $_" }
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
                } catch { Write-Warning "解析 JaCoCo 失败" }
            }
        }
        "MavenMulti" {
            Push-Location $projectPath
            $output = & "mvn.cmd" test -Dmaven.test.failure.ignore=true 2>&1
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
            Write-Warning "未知项目类型: $type，跳过测试"
            return $false, $null
        }
    }

    # 输出结果摘要
    if ($report.total -gt 0) {
        Write-Host "📊 测试结果: $($report.passed)/$($report.total) 通过" -ForegroundColor $(
            if ($report.failed -eq 0) { 'Green' } else { 'Red' }
        )
        if ($report.coverage) { Write-Host "📈 覆盖率: $($report.coverage)" }
        if ($report.failed -gt 0) {
            Write-Host "❌ $($report.failed) 个失败:" -ForegroundColor Red
            foreach ($f in $report.failures) {
                Write-Host "   - [$($f.suite)] $($f.test): $($f.message)"
            }
        }
    } else {
        Write-Host "⚠️ 未检测到测试用例或测试框架" -ForegroundColor Yellow
    }

    return ($report.failed -eq 0), $report
}

# 主入口
param(
    [ValidateSet("check","build","test")]
    [string]$Action = "check",
    [string]$ProjectPath = (Get-Location).Path,
    [switch]$Json,
    [string]$CustomCommand = "",  # 自定义命令（为空时使用默认逻辑）
    [string]$CustomArgs = ""       # 自定义额外参数
)

$start = Get-Date
$result = $false
$testReport = $null

# 如果指定了自定义命令，直接执行（跳过自动检测逻辑）
if ($CustomCommand -ne "") {
    Write-Host "[$Action] 执行自定义命令: $CustomCommand $CustomArgs"
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
            } catch { Write-Warning "解析自定义测试命令的 JSON 输出失败" }
        }
    } else {
        Push-Location $ProjectPath
        try {
            & $cmdParts[0] @($cmdParts[1..($cmdParts.Count-1)]) 2>&1 | Write-Host
            $result = ($LASTEXITCODE -eq 0)
        } finally {
            Pop-Location
        }
    }
    if ($result) { Write-Host "✅ 自定义命令执行成功" }
    else { Write-Host "❌ 自定义命令执行失败" }
} else {
    switch ($Action) {
        "check" { $result = Invoke-Check $ProjectPath }
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
    $output | ConvertTo-Json -Depth 10
}
exit $(if ($result) { 0 } else { 1 })
