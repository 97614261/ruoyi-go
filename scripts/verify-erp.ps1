<#
.SYNOPSIS
    Verify the ERP foundation without using the legacy single-tenant API suite.
.DESCRIPTION
    Keep this script ASCII-only for Windows PowerShell 5.1.
    Default: formatting, Go units/vet, frontend tests and production build.
    Integration is opt-in and WRITES fixtures to the dedicated local database.
    No migration, service restart, credential reset or Redis cleanup is run.
.EXAMPLE
    .\scripts\verify-erp.ps1 -Plan
    .\scripts\verify-erp.ps1
    .\scripts\verify-erp.ps1 -Integration -Count 2
#>
[CmdletBinding()]
param(
    [string]$GoPath = 'go',
    [string]$GofmtPath = '',
    [string]$NodePath = 'node',
    [switch]$Integration,
    [ValidateRange(1, 5)][int]$Count = 1,
    [switch]$Plan
)

$ErrorActionPreference = 'Stop'
if (-not $Integration -and $Count -ne 1) {
    throw 'Count applies only to -Integration; use the default Count=1 otherwise.'
}
$projectRoot = Split-Path -Parent $PSScriptRoot
$webRoot = Join-Path $projectRoot 'web'
$frontTests = @(Get-ChildItem -LiteralPath (Join-Path $webRoot 'test') -Filter '*.test.mjs' -File |
    Sort-Object Name | ForEach-Object { 'test/' + $_.Name })
if ($frontTests.Count -eq 0) { throw 'No frontend test files found.' }

$steps = @(
    [pscustomobject]@{ name = 'format'; tool = 'gofmt'; cwd = $projectRoot; arguments = @('-l', 'cmd', 'internal', 'pkg', 'test/multitenant') },
    [pscustomobject]@{ name = 'go-unit'; tool = 'go'; cwd = $projectRoot; arguments = @('test', '-mod=readonly', './cmd/...', './internal/...', './pkg/...', '-count=1', '-timeout=5m') },
    [pscustomobject]@{ name = 'go-vet'; tool = 'go'; cwd = $projectRoot; arguments = @('vet', '-mod=readonly', './cmd/...', './internal/...', './pkg/...', './test/multitenant') },
    [pscustomobject]@{ name = 'frontend-test'; tool = 'node'; cwd = $webRoot; arguments = @('--test') + $frontTests },
    [pscustomobject]@{ name = 'frontend-build'; tool = 'node'; cwd = $webRoot; arguments = @('node_modules/vite/bin/vite.js', 'build') }
)
if ($Integration) {
    $steps += [pscustomobject]@{ name = 'integration'; tool = 'go'; cwd = $projectRoot; arguments = @('test', '-mod=readonly', './test/multitenant', "-count=$Count", '-timeout=15m', '-v') }
}
if ($Plan) {
    [pscustomobject]@{
        integrationRequested = [bool]$Integration
        integrationEffects = 'Creates dedicated local customer fixtures; retains records and disables customers after tests. Does not test original demo credentials.'
        steps = $steps
    } | ConvertTo-Json -Depth 6
    exit 0
}

function Resolve-Tool([string]$Value) {
    $tool = Get-Command $Value -CommandType Application, ExternalScript -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $tool) { throw "Required executable not found: $Value" }
    return $tool.Source
}

$executables = @{ go = (Resolve-Tool $GoPath); node = (Resolve-Tool $NodePath) }
if ($GofmtPath -eq '') {
    $gofmtName = if ($env:OS -eq 'Windows_NT') { 'gofmt.exe' } else { 'gofmt' }
    $GofmtPath = Join-Path (Split-Path -Parent $executables.go) $gofmtName
}
$executables.gofmt = Resolve-Tool $GofmtPath
if (-not (Test-Path -LiteralPath (Join-Path $webRoot 'node_modules/vite/bin/vite.js') -PathType Leaf)) {
    throw 'Frontend dependencies missing. Run npm ci in web/ before verification.'
}
if ($Integration) {
    foreach ($relative in @('configs/multitenant.local.yml', 'logs/multitenant-accounts.local.json')) {
        if (-not (Test-Path -LiteralPath (Join-Path $projectRoot $relative) -PathType Leaf)) {
            throw "Integration prerequisite missing: $relative. Do not substitute the original database/config."
        }
    }
    # The test package additionally validates the local DSN/database and platform
    # bcrypt record. Missing prerequisites must fail, not skip or reset passwords.
    try {
        $health = Invoke-WebRequest -Uri 'http://127.0.0.1:8081/health' -UseBasicParsing -TimeoutSec 5
        if ($health.StatusCode -ne 200) { throw 'Not healthy.' }
    } catch { throw 'Dedicated local backend is not healthy on 127.0.0.1:8081. Nothing was started/stopped.' }
    Write-Host 'Integration enabled: dedicated local fixtures WILL be written; existing customer passwords are not changed.'
}

$stamp = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$resultRoot = Join-Path $projectRoot ("test/results/erp-$stamp")
New-Item -ItemType Directory -Path $resultRoot -Force | Out-Null
$summaryPath = Join-Path $resultRoot 'summary.json'
$results = @($steps | ForEach-Object {
    [pscustomobject]@{ name = $_.name; status = 'NOT_RUN'; exitCode = $null; elapsedSeconds = 0; log = ($_.name + '.log') }
})
$summary = [ordered]@{
    startedAtUtc = [DateTime]::UtcNow.ToString('o')
    finishedAtUtc = $null
    verdict = 'FAILED'
    integration = $(if ($Integration) { 'REQUESTED' } else { 'NOT_REQUESTED' })
    scope = 'Development checks only; not browser, backup/restore, capacity or production certification.'
    steps = $results
}
$previousEncoding = [Console]::OutputEncoding
$previousTestRoot = $env:MT_TEST_ROOT
$previousProxy = $env:GOPROXY
$previousNoProxy = $env:GONOPROXY
$previousToolchain = $env:GOTOOLCHAIN
$verifyExit = 1
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    # Missing dependencies/toolchain must fail instead of downloading or editing
    # module metadata during verification. Install dependencies explicitly first.
    $env:GOPROXY = 'off'
    $env:GONOPROXY = 'none'
    $env:GOTOOLCHAIN = 'local'
    if ($Integration) { $env:MT_TEST_ROOT = $projectRoot }
    for ($i = 0; $i -lt $steps.Count; $i++) {
        $step = $steps[$i]
        $result = $results[$i]
        Write-Host ("==> " + $step.name)
        $started = [DateTime]::UtcNow
        Push-Location $step.cwd
        try {
            $nativePreference = $ErrorActionPreference
            try {
                # PS5.1 wraps native stderr as ErrorRecord. Warnings must not
                # hide LASTEXITCODE, and a later success must not mask a failure.
                $ErrorActionPreference = 'Continue'
                $PSNativeCommandUseErrorActionPreference = $false
                # Native tools update the global automatic variable. A local
                # null shadows it when this script is invoked via & from a
                # parent script (notably PowerShell 7), falsely failing a clean
                # formatter. Use the same scope for reset and capture.
                $global:LASTEXITCODE = $null
                $stepArguments = @($step.arguments)
                $output = @(& $executables[$step.tool] @stepArguments 2>&1 | ForEach-Object { $_.ToString() })
                $nativeExit = $global:LASTEXITCODE
                if ($null -eq $nativeExit) { $nativeExit = 1 }
            } finally { $ErrorActionPreference = $nativePreference }
        } finally { Pop-Location }
        $output | Out-File -LiteralPath (Join-Path $resultRoot $result.log) -Encoding utf8
        $result.elapsedSeconds = [math]::Round(([DateTime]::UtcNow - $started).TotalSeconds, 3)
        $result.exitCode = $nativeExit
        if ($step.name -eq 'format' -and @($output | Where-Object { $_.Trim() -ne '' }).Count -gt 0 -and $nativeExit -eq 0) {
            $result.exitCode = 1
        }
        if ($result.exitCode -ne 0) {
            $result.status = 'FAILED'
            throw ("Step failed: " + $step.name + '. See its log; remaining steps were not run.')
        }
        $result.status = 'PASSED'
    }
    $summary.verdict = 'CHECKS_PASSED'
    $verifyExit = 0
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
} finally {
    try {
        $summary.finishedAtUtc = [DateTime]::UtcNow.ToString('o')
        $summary | ConvertTo-Json -Depth 6 | Out-File -LiteralPath $summaryPath -Encoding utf8
    } finally {
        if ($null -eq $previousTestRoot) { Remove-Item Env:\MT_TEST_ROOT -ErrorAction SilentlyContinue }
        else { $env:MT_TEST_ROOT = $previousTestRoot }
        if ($null -eq $previousProxy) { Remove-Item Env:\GOPROXY -ErrorAction SilentlyContinue }
        else { $env:GOPROXY = $previousProxy }
        if ($null -eq $previousNoProxy) { Remove-Item Env:\GONOPROXY -ErrorAction SilentlyContinue }
        else { $env:GONOPROXY = $previousNoProxy }
        if ($null -eq $previousToolchain) { Remove-Item Env:\GOTOOLCHAIN -ErrorAction SilentlyContinue }
        else { $env:GOTOOLCHAIN = $previousToolchain }
        [Console]::OutputEncoding = $previousEncoding
    }
}
Write-Host ("Verdict: " + $summary.verdict + '; integration=' + $summary.integration)
Write-Host ("Summary: " + $summaryPath)
exit $verifyExit
