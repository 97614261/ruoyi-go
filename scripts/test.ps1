<#
.SYNOPSIS
    Run API tests and write results to test/results/ for humans and AI to read.

.DESCRIPTION
    NOTE: This script is intentionally ASCII-only.
    Windows PowerShell 5.1 decodes .ps1 files using the system ANSI codepage
    unless the file starts with a UTF-8 BOM. On a Chinese Windows install that
    turns any non-ASCII text into mojibake and breaks string literals, so the
    script fails to parse. Keep it ASCII.

    Produces two files:
      test/results/latest.log    full output
      test/results/summary.log   failures + counts (read this first)

.EXAMPLE
    .\scripts\test.ps1
    .\scripts\test.ps1 TestPost
    .\scripts\test.ps1 'TestMenu|TestDept'
    .\scripts\test.ps1 -Full
    .\scripts\test.ps1 -Config configs\application.test.yml
    .\scripts\test.ps1 -Unit
#>
param(
    # Regex passed to "go test -run". Empty means run everything.
    [Parameter(Position = 0)]
    [string]$Run = "",

    # Also print the full output to the console (summary only by default).
    [switch]$Full,

    # Config file the API tests should use. Point this at a throwaway database
    # if you do not want tests writing into your dev DB. The tests already clean
    # up after themselves, but a separate DB removes the question entirely.
    [string]$Config = "",

    # Also run the pure unit tests (pkg/cronx, internal/job ...). Those need
    # neither MySQL nor Redis, so they are the part that could run in CI today.
    [switch]$Unit
)

$ErrorActionPreference = "Stop"

# Go writes UTF-8, but PowerShell decodes a child process's stdout using
# [Console]::OutputEncoding, which defaults to the OEM codepage (cp936 on a
# Chinese Windows). Without this the captured Chinese text becomes mojibake
# in both the console and the log files.
$previousEncoding = [Console]::OutputEncoding
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
try {

$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

$resultDir = Join-Path $projectRoot "test\results"
New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
$logFile = Join-Path $resultDir "latest.log"
$summaryFile = Join-Path $resultDir "summary.log"

# Override config via env vars instead of editing application.yml:
#   server.mode -> SERVER_MODE   release mode skips the gin route dump
#   log.level   -> LOG_LEVEL     error only, drops per-request INFO lines
$env:SERVER_MODE = "release"
$env:LOG_LEVEL = "error"
$env:GIN_MODE = "release"

# main_test.go reads this; empty means ../configs/application.yml
if ($Config -ne "") {
    if (-not (Test-Path $Config)) { throw "config not found: $Config" }
    $env:RUOYI_TEST_CONFIG = (Resolve-Path $Config).Path
    Write-Host "==> test config: $($env:RUOYI_TEST_CONFIG)" -ForegroundColor Yellow
}
else {
    Remove-Item Env:\RUOYI_TEST_CONFIG -ErrorAction SilentlyContinue
}

# ./test/ needs MySQL + Redis. The other packages are pure unit tests.
$packages = @("./test/")
if ($Unit) {
    $packages += @("./pkg/...", "./internal/...")
}

$goArgs = @("test") + $packages + @("-v", "-count=1")
if ($Run -ne "") {
    $goArgs += @("-run", $Run)
}

Write-Host "==> go $($goArgs -join ' ')" -ForegroundColor Cyan
$started = Get-Date

# 2>&1 also captures build errors
$output = & go @goArgs 2>&1 | ForEach-Object { $_.ToString() }
$exitCode = $LASTEXITCODE
$elapsed = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)

$output | Out-File -FilePath $logFile -Encoding utf8

# --- build summary ---
# Only collect "file_test.go:NN" lines when something actually failed.
# t.Logf output looks exactly like t.Errorf output, so on a green run this
# filter would report benchmark/report logging as FAILURE DETAILS.
$failDetails = @()
if ($exitCode -ne 0) {
    $failDetails = @($output | Where-Object { $_ -match "_test\.go:\d+" })
}
$caseResults = @($output | Where-Object { $_ -match "^\s*--- (FAIL|SKIP)" })
$buildErrors = @($output | Where-Object { $_ -match "^#|cannot use|undefined:|syntax error|declared and not used" })
$passCount = @($output | Where-Object { $_ -match "^\s*--- PASS" }).Count
$failCount = @($output | Where-Object { $_ -match "^\s*--- FAIL" }).Count
$skipCount = @($output | Where-Object { $_ -match "^\s*--- SKIP" }).Count

if ($exitCode -eq 0) { $verdict = "ALL PASSED" } else { $verdict = "FAILED" }

$summary = New-Object System.Collections.Generic.List[string]
$summary.Add("time    : $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')")
$summary.Add("command : go $($goArgs -join ' ')")
$summary.Add("elapsed : $elapsed s")
$summary.Add("verdict : $verdict (exit=$exitCode)")
$summary.Add("cases   : pass=$passCount fail=$failCount skip=$skipCount")
$summary.Add("fulllog : test/results/latest.log")
$summary.Add("")

if ($buildErrors.Count -gt 0) {
    $summary.Add("=== BUILD ERRORS ===")
    foreach ($line in $buildErrors) { $summary.Add($line.TrimEnd()) }
    $summary.Add("")
}
if ($failDetails.Count -gt 0) {
    $summary.Add("=== FAILURE DETAILS ===")
    foreach ($line in $failDetails) { $summary.Add($line.TrimEnd()) }
    $summary.Add("")
}
if ($caseResults.Count -gt 0) {
    $summary.Add("=== FAILED / SKIPPED CASES ===")
    foreach ($line in $caseResults) { $summary.Add($line.TrimEnd()) }
}

$summary | Out-File -FilePath $summaryFile -Encoding utf8

# --- print ---
if ($Full) {
    foreach ($line in $output) { Write-Host $line }
    Write-Host ""
}
foreach ($line in $summary) {
    if ($line -match "FAIL|ERROR") { Write-Host $line -ForegroundColor Red }
    elseif ($line -match "^===") { Write-Host $line -ForegroundColor Yellow }
    elseif ($line -match "ALL PASSED") { Write-Host $line -ForegroundColor Green }
    else { Write-Host $line }
}

exit $exitCode

}
finally {
    Remove-Item Env:\RUOYI_TEST_CONFIG -ErrorAction SilentlyContinue
    [Console]::OutputEncoding = $previousEncoding
}
