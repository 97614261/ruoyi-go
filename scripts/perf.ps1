# perf.ps1 - performance testing helper
#
# NOTE: ASCII only. PowerShell 5.1 decodes .ps1 as ANSI when there is no UTF-8 BOM,
# so Chinese characters here would turn into mojibake and break string literals.
# WARNING: index, unindex, and all modify database indexes. Use an isolated perf database only.
# Never point this script at production. Verify the configured server and database before running.
#
# Usage:
#   .\scripts\perf.ps1 all       # unattended: explain/bench before -> add indexes -> explain/bench after
#   .\scripts\perf.ps1 seed      # load perf data (100k users / 1k depts / 500k logs)
#   .\scripts\perf.ps1 explain   # SQL level:  EXPLAIN + timing
#   .\scripts\perf.ps1 bench     # HTTP level: concurrent load
#   .\scripts\perf.ps1 index     # create candidate indexes
#   .\scripts\perf.ps1 unindex   # drop them (full rollback)
#   .\scripts\perf.ps1 clean     # remove all perf data
#
#   .\scripts\perf.ps1 seed -Users 20000 -Depts 200 -OperLogs 50000
#   .\scripts\perf.ps1 bench -Concurrency 100 -Requests 3000 -Label mytest
#
# Every run writes its own file under test/results/, named by action + label,
# so nothing overwrites anything and you never have to stop and copy things out.

param(
    [Parameter(Position = 0)]
    [ValidateSet('all', 'seed', 'explain', 'clean', 'index', 'unindex', 'bench')]
    [string]$Action = 'all',

    [int]$Users = 100000,
    [int]$Depts = 1000,
    [int]$OperLogs = 500000,

    [int]$Concurrency = 50,
    [int]$Requests = 1000,
    [string]$Url = 'http://127.0.0.1:8080',

    # Suffix for the output file name; defaults to a timestamp
    [string]$Label = ''
)

# NOT 'Stop'. Native tools write normal output to stderr (go build errors,
# maven warnings), and with 'Stop' PowerShell turns each of those lines into a
# terminating NativeCommandError. Errors here are handled explicitly via
# $LASTEXITCODE + throw, which still unwinds to finally.
$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
$resultsDir = Join-Path $root 'test\results'
$script:StartedServer = $null

function Write-Step($text) {
    Write-Host ""
    Write-Host "=============================================================" -ForegroundColor Cyan
    Write-Host " $text" -ForegroundColor Cyan
    Write-Host "=============================================================" -ForegroundColor Cyan
}

function Get-LogPath($action, $label) {
    if ([string]::IsNullOrWhiteSpace($label)) {
        $label = Get-Date -Format 'HHmmss'
    }
    return (Join-Path $resultsDir "perf-$action-$label.log")
}

# Run a go command, echo its output live, and also save it to a file.
function Invoke-Tool($displayCommand, $arguments, $logPath) {
    $header = @(
        "time    : $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')",
        "command : $displayCommand",
        ""
    )
    $output = & go @arguments 2>&1 | Out-String
    $exit = $LASTEXITCODE

    ($header + $output) | Set-Content -Path $logPath -Encoding UTF8
    Write-Host $output

    if ($exit -ne 0) {
        Write-Host "FAILED (exit=$exit) - see $logPath" -ForegroundColor Red
        throw "command failed: $displayCommand"
    }
    Write-Host "-> $logPath" -ForegroundColor Green
}

function Test-ServerUp {
    try {
        $response = Invoke-WebRequest -Uri "$Url/health" -UseBasicParsing -TimeoutSec 3
        return ($response.StatusCode -eq 200)
    }
    catch {
        return $false
    }
}

# Build and start the server in the background so 'all' can run unattended.
# Uses a compiled binary rather than 'go run' - 'go run' spawns a child process
# that would survive Stop-Process on the parent.
function Start-LocalServer {
    Write-Host "Server not reachable at $Url, starting one..." -ForegroundColor Yellow

    $binDir = Join-Path $root 'bin'
    if (-not (Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir | Out-Null }
    $exe = Join-Path $binDir 'server.exe'

    go build -o $exe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    $serverLog = Join-Path $resultsDir 'perf-server.log'
    $process = Start-Process -FilePath $exe -WorkingDirectory $root -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $serverLog -RedirectStandardError "$serverLog.err"

    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Milliseconds 500
        if (Test-ServerUp) {
            Write-Host "Server up (pid $($process.Id)), log -> $serverLog" -ForegroundColor Green
            return $process
        }
    }

    try { Stop-Process -Id $process.Id -Force } catch { }
    throw "server did not become healthy in 15s - see $serverLog"
}

function Ensure-Server {
    if (Test-ServerUp) {
        Write-Host "Server already running at $Url" -ForegroundColor Green
        return
    }
    $script:StartedServer = Start-LocalServer
}

function Stop-LocalServerIfStarted {
    if ($null -ne $script:StartedServer) {
        Write-Host ""
        Write-Host "Stopping the server we started (pid $($script:StartedServer.Id))" -ForegroundColor Yellow
        try { Stop-Process -Id $script:StartedServer.Id -Force } catch { }
        $script:StartedServer = $null
    }
}

function Invoke-Explain($label) {
    $log = Get-LogPath 'explain' $label
    Write-Step "SQL level: EXPLAIN + timing  [$label]"
    Invoke-Tool "go run ./cmd/perfseed -explain" @('run', './cmd/perfseed', '-explain') $log
}

function Invoke-Bench($label) {
    $log = Get-LogPath 'bench' $label
    Write-Step "HTTP level: $Concurrency concurrent, $Requests requests each  [$label]"
    Invoke-Tool "go run ./cmd/perfbench -url $Url -c $Concurrency -n $Requests" `
        @('run', './cmd/perfbench', '-url', $Url, '-c', "$Concurrency", '-n', "$Requests") $log
}

Push-Location $root
$previousEncoding = [Console]::OutputEncoding
try {
    # Go writes UTF-8; the console defaults to GBK on zh-CN Windows and would mangle it
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8

    if (-not (Test-Path $resultsDir)) {
        New-Item -ItemType Directory -Path $resultsDir | Out-Null
    }

    switch ($Action) {
        'seed' {
            Write-Step "Loading perf data: $Users users / $Depts depts / $OperLogs oper logs"
            Write-Host "Target DB is whatever configs/application.yml points at." -ForegroundColor Yellow
            Write-Host "Interface tests will fail while this data is loaded - run 'clean' when done." -ForegroundColor Yellow
            go run ./cmd/perfseed -users $Users -depts $Depts -operlogs $OperLogs -confirm
            if ($LASTEXITCODE -ne 0) { throw "seed failed" }
        }
        'clean' {
            Write-Step "Removing all perf data (id >= 900000)"
            go run ./cmd/perfseed -clean -confirm
            if ($LASTEXITCODE -ne 0) { throw "clean failed" }
        }
        'index' {
            Write-Step "Creating candidate indexes (rollback: perf.ps1 unindex)"
            go run ./cmd/perfseed -index
            if ($LASTEXITCODE -ne 0) { throw "index failed" }
        }
        'unindex' {
            Write-Step "Dropping candidate indexes"
            go run ./cmd/perfseed -unindex
            if ($LASTEXITCODE -ne 0) { throw "unindex failed" }
        }
        'explain' {
            Invoke-Explain $Label
        }
        'bench' {
            Ensure-Server
            Invoke-Bench $Label
        }
        'all' {
            Ensure-Server

            Invoke-Explain 'before'
            Invoke-Bench 'before'

            Write-Step "Creating candidate indexes"
            go run ./cmd/perfseed -index
            if ($LASTEXITCODE -ne 0) { throw "index failed" }

            Invoke-Explain 'after'
            Invoke-Bench 'after'

            Write-Step "Done"
            Write-Host "Four logs written:" -ForegroundColor Green
            Get-ChildItem -Path $resultsDir -Filter 'perf-*-before.log' | ForEach-Object { Write-Host "  $($_.FullName)" }
            Get-ChildItem -Path $resultsDir -Filter 'perf-*-after.log' | ForEach-Object { Write-Host "  $($_.FullName)" }
            Write-Host ""
            Write-Host "Indexes are still in place. To roll back:  .\scripts\perf.ps1 unindex" -ForegroundColor Yellow
            Write-Host "To remove perf data:                       .\scripts\perf.ps1 clean" -ForegroundColor Yellow
        }
    }
}
finally {
    Stop-LocalServerIfStarted
    [Console]::OutputEncoding = $previousEncoding
    Pop-Location
}
