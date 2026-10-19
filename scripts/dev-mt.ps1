param([string]$NodePath)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $projectRoot 'configs\multitenant.local.yml'
if (-not (Test-Path -LiteralPath $configPath)) { throw 'Run go run ./cmd/tenantinit first. Do not use the original database.' }
if (-not $NodePath) {
  $nodeCommand = Get-Command node -ErrorAction SilentlyContinue
  if (-not $nodeCommand) { throw 'Node.js not found. Add it to PATH or pass -NodePath.' }
  $NodePath = $nodeCommand.Source
}
if (-not (Test-Path -LiteralPath $NodePath)) { throw 'NodePath does not exist.' }
$viteEntry = Join-Path $projectRoot 'web\node_modules\vite\bin\vite.js'
if (-not (Test-Path -LiteralPath $viteEntry)) { throw 'Run npm ci in web first.' }
foreach ($port in @(8081,5173)) {
  $probe = [System.Net.Sockets.TcpClient]::new()
  try {
    $connection = $probe.ConnectAsync('127.0.0.1',$port)
    if ($connection.Wait(800) -and $probe.Connected) { throw "Port $port is in use. This script never stops existing processes." }
  } catch [System.AggregateException] { } finally { $probe.Dispose() }
}
Push-Location $projectRoot
try {
  New-Item -ItemType Directory -Path 'bin','logs' -Force | Out-Null
  go build -o bin\server-mt.exe ./cmd/server
  if ($LASTEXITCODE -ne 0) { throw 'Backend build failed.' }
  $backend = Start-Process -FilePath (Join-Path $projectRoot 'bin\server-mt.exe') -ArgumentList '-config','configs/multitenant.local.yml' -WorkingDirectory $projectRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $projectRoot 'logs\backend.dev.log') -RedirectStandardError (Join-Path $projectRoot 'logs\backend.err.log') -PassThru
  $frontend = Start-Process -FilePath $NodePath -ArgumentList ('"'+$viteEntry+'"') -WorkingDirectory (Join-Path $projectRoot 'web') -WindowStyle Hidden -RedirectStandardOutput (Join-Path $projectRoot 'logs\frontend.dev.log') -RedirectStandardError (Join-Path $projectRoot 'logs\frontend.err.log') -PassThru
  $state = @{backendPid=$backend.Id;frontendPid=$frontend.Id;frontend='http://localhost:5173';backend='http://localhost:8081'}
  [System.IO.File]::WriteAllText((Join-Path $projectRoot 'logs\dev-mt-processes.json'),($state | ConvertTo-Json))
  $state | ConvertTo-Json
} finally { Pop-Location }
