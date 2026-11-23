[CmdletBinding(DefaultParameterSetName = 'Start')]
param(
    [Parameter(ParameterSetName = 'Check')][switch]$Check,
    [Parameter(ParameterSetName = 'Stop')][switch]$Stop
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$config = Join-Path $root 'configs/pod.local.yml'
$statePath = Join-Path $root '.local/processes.json'
$endpoints = @(
    'http://127.0.0.1:18110/health'
    'http://127.0.0.1:5177/api/health'
    'http://127.0.0.1:5178/dev-api/health'
)

function Test-LocalPort([int]$Port) {
    $client = [Net.Sockets.TcpClient]::new()
    try {
        try { $null = $client.ConnectAsync('127.0.0.1', $Port).Wait(400) } catch { }
        return $client.Connected
    } finally {
        $client.Dispose()
    }
}

# Explorer can have a different PATH from the development terminal.
function Find-LocalTool([string]$Name, [string[]]$Directories) {
    if (Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue) { return }
    foreach ($directory in $Directories) {
        if (Test-Path -LiteralPath (Join-Path $directory $Name) -PathType Leaf) {
            $env:Path = "$directory;$env:Path"
            return
        }
    }
    throw "Cannot find $Name. Install it and add it to Windows PATH."
}

function Wait-ForEndpoint([string]$Url, [Diagnostics.Stopwatch]$Timer) {
    do {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
            if ($response.StatusCode -eq 200) { return }
        } catch { }
        Start-Sleep -Milliseconds 300
    } while ($Timer.Elapsed.TotalSeconds -lt 40)
    throw "Service did not become ready at $Url. Check .local/server, studio and admin logs."
}

$startupLock = [System.Threading.Mutex]::new($false, 'Local\PODStudioDevStart')
$hasLock = $false
try {
    try { $hasLock = $startupLock.WaitOne(0) } catch [System.Threading.AbandonedMutexException] { $hasLock = $true }
    if (-not $hasLock) { throw 'POD Studio is already starting or stopping. Wait for it to finish, then retry.' }

    if ($Stop) {
        & (Join-Path $PSScriptRoot 'pod.ps1') -Action stop
        Find-LocalTool 'go.exe' @((Join-Path $env:ProgramFiles 'Go/bin'))
        & (Join-Path $PSScriptRoot 'pod-redis.ps1') -Action stop
        Write-Host 'POD Studio is stopped.' -ForegroundColor Green
        return
    }

    $runtime = Join-Path $env:USERPROFILE '.cache/codex-runtimes/codex-primary-runtime/dependencies'
    Find-LocalTool 'go.exe' @((Join-Path $env:ProgramFiles 'Go/bin'))
    Find-LocalTool 'node.exe' @((Join-Path $env:ProgramFiles 'nodejs'), (Join-Path $runtime 'node/bin'))

    if (-not (Test-Path -LiteralPath $config -PathType Leaf)) { throw 'Missing configs/pod.local.yml. See docs/POD_LOCAL.md.' }
    foreach ($frontend in @('studio', 'web')) {
        if (-not (Test-Path -LiteralPath (Join-Path $root "$frontend/node_modules/vite/bin/vite.js") -PathType Leaf)) {
            throw "Dependencies missing in $frontend. Run pnpm install --frozen-lockfile there."
        }
    }

    $ports = @(18110, 5177, 5178)
    $imageConfig = Select-String -LiteralPath $config -Pattern '^\s+imageAIURL:\s*["'']?http://127\.0\.0\.1:(\d+)["'']?\s*$' | Select-Object -First 1
    if ($imageConfig) {
        $imagePort = [int]$imageConfig.Matches[0].Groups[1].Value
        $ports += $imagePort
        $endpoints += "http://127.0.0.1:$imagePort/health"
    }
    $expectsWorker = [bool](Select-String -LiteralPath $config -Pattern '^\s+mode:\s*(simulation|qwen|managed)\s*(#.*)?$' -Quiet)

    $records = @()
    $running = @()
    if (Test-Path -LiteralPath $statePath) {
        $records = Get-Content -LiteralPath $statePath -Raw -Encoding UTF8 | ConvertFrom-Json
        $running = @(foreach ($record in $records) {
            $process = Get-Process -Id ([int]$record.id) -ErrorAction SilentlyContinue
            if ($process -and $process.StartTime.ToUniversalTime() -eq ([datetime]$record.startedAt).ToUniversalTime()) {
                if ($process.Path -ne $record.path) { throw 'Cannot verify a recorded POD process. Check .local/processes.json.' }
                $process
            }
        })
    }

    if ($running.Count) {
        $expectedCount = $ports.Count + [int]$expectsWorker
        if ($running.Count -ne $records.Count -or $running.Count -ne $expectedCount) {
            throw 'Some POD services are already running. Run stop.bat, then reopen start.bat.'
        }
        Write-Host 'POD Studio processes are already running.'
    } else {
        foreach ($port in $ports) {
            if (Test-LocalPort $port) { throw "Port $port is occupied. Identify its owner before starting POD Studio." }
        }
        if ($Check) {
            Write-Host 'Go, Node.js, frontend dependencies, local configuration and app ports passed preflight.'
            exit 0
        }

        if (Test-Path -LiteralPath $statePath) { Remove-Item -LiteralPath $statePath }
        & (Join-Path $PSScriptRoot 'pod-redis.ps1') -Action start
        $redisTimer = [Diagnostics.Stopwatch]::StartNew()
        while (-not (Test-LocalPort 6387)) {
            if ($redisTimer.Elapsed.TotalSeconds -ge 30) { throw 'POD Redis did not become ready. Check .local/redis5/redis.err.log.' }
            Start-Sleep -Milliseconds 200
        }
        & (Join-Path $PSScriptRoot 'pod.ps1') -Action start
    }

    $startupTimer = [Diagnostics.Stopwatch]::StartNew()
    foreach ($url in $endpoints) { Wait-ForEndpoint $url $startupTimer }
    if ($expectsWorker) {
        $workerPath = Join-Path $root '.local/bin/pod-worker.exe'
        $workerRecord = (Get-Content -LiteralPath $statePath -Raw -Encoding UTF8 | ConvertFrom-Json) | Where-Object { $_.path -eq $workerPath }
        $worker = if ($workerRecord) { Get-Process -Id ([int]$workerRecord.id) -ErrorAction SilentlyContinue }
        if (-not $worker -or $worker.Path -ne $workerPath -or $worker.StartTime.ToUniversalTime() -ne ([datetime]$workerRecord.startedAt).ToUniversalTime()) {
            throw 'POD Worker is not running. Check .local/worker.err.log and .local/worker.out.log.'
        }
    }

    Write-Host ''
    Write-Host 'POD Studio is ready.' -ForegroundColor Green
    Write-Host 'Creator  http://127.0.0.1:5177/'
    Write-Host 'Admin    http://127.0.0.1:5178/'
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
} finally {
    if ($hasLock) { $startupLock.ReleaseMutex() }
    $startupLock.Dispose()
}
