# Self-contained runner tests. No application service/database is required.
# Keep ASCII for Windows PowerShell 5.1; no Pester dependency.
$ErrorActionPreference = 'Stop'
$runner = Join-Path (Split-Path -Parent $PSScriptRoot) 'verify-erp.ps1'
$shell = (Get-Process -Id $PID).Path
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('erp-runner-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $testRoot 'scripts'),(Join-Path $testRoot 'web/test'),(Join-Path $testRoot 'web/node_modules/vite/bin') -Force | Out-Null
Copy-Item -LiteralPath $runner -Destination (Join-Path $testRoot 'scripts/verify-erp.ps1')
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'fixtures/verify-tool.ps1') -Destination (Join-Path $testRoot 'tool.ps1')
New-Item -ItemType File -Path (Join-Path $testRoot 'web/test/sample.test.mjs'),(Join-Path $testRoot 'web/node_modules/vite/bin/vite.js') | Out-Null
$runner = Join-Path $testRoot 'scripts/verify-erp.ps1'
$tool = Join-Path $testRoot 'tool.ps1'
$previous = @{}
foreach ($name in @('ERP_VERIFY_CALLS','ERP_VERIFY_FAIL','ERP_VERIFY_DIRTY','ERP_VERIFY_WARNING','MT_TEST_ROOT')) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
function Invoke-Runner([string[]]$Extra = @(), [string]$Formatter = $tool) {
    $nativePreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $PSNativeCommandUseErrorActionPreference = $false
        $lines = @(& $shell -NoProfile -File $runner -GoPath $tool -GofmtPath $Formatter -NodePath $tool @Extra 2>&1 | ForEach-Object { $_.ToString() })
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $nativePreference }
    return [pscustomobject]@{ code = $code; output = ($lines -join "`n") }
}
function Latest-Summary {
    $file = Get-ChildItem -LiteralPath (Join-Path $testRoot 'test/results') -Recurse -Filter summary.json |
        Sort-Object FullName | Select-Object -Last 1
    return Get-Content -LiteralPath $file.FullName -Raw -Encoding utf8 | ConvertFrom-Json
}
try {
    $env:ERP_VERIFY_CALLS = Join-Path $testRoot 'calls.jsonl'
    $env:ERP_VERIFY_FAIL = '0'
    $env:ERP_VERIFY_DIRTY = '0'
    $env:ERP_VERIFY_WARNING = '0'
    $env:MT_TEST_ROOT = 'inherited-value-not-an-opt-in'
    $planRun = Invoke-Runner @('-Plan')
    Assert-True ($planRun.code -eq 0) 'Default plan failed.'
    $plan = $planRun.output | ConvertFrom-Json
    Assert-True (-not $plan.integrationRequested -and $plan.steps.Count -eq 5) 'Default plan included integration.'
    Assert-True (-not (Test-Path -LiteralPath $env:ERP_VERIFY_CALLS)) 'Plan executed an external command.'
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $testRoot 'test/results'))) 'Plan wrote results.'
    foreach ($step in $plan.steps) {
        Assert-True (-not ($step.arguments -contains './test/')) 'Legacy API suite included.'
        Assert-True (-not ($step.arguments -contains './...')) 'Unbounded all-package test included.'
    }
    Write-Host 'PASS: read-only plan, default isolation, no legacy API suite'

    $integrationRun = Invoke-Runner @('-Plan','-Integration','-Count','2')
    Assert-True ($integrationRun.code -eq 0) 'Integration plan failed.'
    $integrationPlan = $integrationRun.output | ConvertFrom-Json
    Assert-True ($integrationPlan.integrationRequested -and $integrationPlan.steps.Count -eq 6) 'Integration missing.'
    Assert-True ($integrationPlan.steps[-1].arguments -contains './test/multitenant') 'Wrong integration package.'
    Assert-True ($integrationPlan.steps[-1].arguments -contains '-count=2') 'Count not forwarded.'
    Assert-True ((Invoke-Runner @('-Plan','-Count','2')).code -ne 0) 'Meaningless Count was accepted.'
    Assert-True ((Invoke-Runner @('-Integration')).code -ne 0) 'Integration ran without its private prerequisites.'
    Assert-True (-not (Test-Path -LiteralPath $env:ERP_VERIFY_CALLS)) 'Failed integration preflight executed commands.'
    Write-Host 'PASS: explicit integration and parameter validation'

    # Stub executables prove orchestration only, never product correctness.
    $ok = Invoke-Runner
    Assert-True ($ok.code -eq 0) 'Successful commands were rejected.'
    $report = Latest-Summary
    Assert-True ($report.verdict -eq 'CHECKS_PASSED' -and $report.integration -eq 'NOT_REQUESTED') 'Misleading verdict.'
    Assert-True (@($report.steps | Where-Object status -ne 'PASSED').Count -eq 0) 'Incomplete success.'
    $calls = @(Get-Content -LiteralPath $env:ERP_VERIFY_CALLS -Encoding utf8 | ForEach-Object { $_ | ConvertFrom-Json })
    Assert-True ($calls.Count -eq 5) 'Unexpected commands executed.'
    Assert-True ($calls[1].arguments -contains './cmd/...') 'Go arguments not passed separately.'
    Assert-True ($calls[1].arguments -contains '-mod=readonly') 'Module metadata is not protected.'
    Assert-True ($calls[1].proxy -eq 'off' -and $calls[1].noProxy -eq 'none' -and $calls[1].toolchain -eq 'local') 'Unexpected automatic dependency/toolchain download allowed.'
    Assert-True ($calls[3].arguments -contains 'test/sample.test.mjs') 'Frontend test enumeration missing.'
    Write-Host 'PASS: argument arrays and honest default summary'

    $env:ERP_VERIFY_FAIL = '1'
    $failed = Invoke-Runner
    Assert-True ($failed.code -ne 0) 'Native failure masked.'
    $report = Latest-Summary
    Assert-True ($report.verdict -eq 'FAILED' -and $report.steps[1].exitCode -eq 23) 'Native exit code lost.'
    Assert-True ($report.steps[2].status -eq 'NOT_RUN') 'Continued after failure.'
    Write-Host 'PASS: fail-fast, native exit preserved, later steps not run'

    $env:ERP_VERIFY_FAIL = '0'
    $env:ERP_VERIFY_DIRTY = '1'
    Assert-True ((Invoke-Runner).code -ne 0) 'Dirty gofmt accepted.'
    $report = Latest-Summary
    Assert-True ($report.steps[0].status -eq 'FAILED' -and $report.steps[1].status -eq 'NOT_RUN') 'Format failure not isolated.'
    Write-Host 'PASS: gofmt output fails without modifying files'

    $env:ERP_VERIFY_DIRTY = '0'
    $env:ERP_VERIFY_WARNING = '1'
    # gofmt output intentionally fails on any output. Test stderr behavior with
    # a separate quiet formatter so only Go/Node emit the harmless warning.
    $warning = Invoke-Runner -Formatter (Join-Path $PSScriptRoot 'fixtures/verify-quiet.ps1')
    Assert-True ($warning.code -eq 0) 'Harmless native stderr hid the successful exit code.'
    Write-Host 'PASS: stderr warning does not mask exit status'
    $env:ERP_VERIFY_WARNING = '0'
    $nested = Join-Path $PSScriptRoot 'fixtures/verify-nested.ps1'
    $nativeFormatter = Join-Path $PSScriptRoot 'fixtures/verify-native.cmd'
    & $shell -NoProfile -File $nested -Runner $runner -Tool $tool -Formatter $nativeFormatter
    Assert-True ($LASTEXITCODE -eq 0) 'Nested invocation lost the real native formatter exit code.'
    $report = Latest-Summary
    Assert-True ($report.verdict -eq 'CHECKS_PASSED') 'Nested native invocation did not pass.'
    Write-Host 'PASS: nested script and native formatter exit scope'
    Write-Host ("Runner tests passed. Retained temporary evidence: " + $testRoot)
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}
