param(
  [ValidateSet('prepare','init','migrate','check','doctor','start','stop','test','backup','bundle','restore-check')][string]$Action = 'check',
  [int]$RedisDB = 15,
  [string]$MySQLDump = 'mysqldump',
  [string]$Bundle = '',
  [string]$TestConfig = '',
  [switch]$UpgradeAdmin,
  [switch]$UpgradeCatalog,
  [switch]$UpgradeCreators,
  [switch]$UpgradeManaged,
 [switch]$UpgradeOfficial
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'pod-env.ps1')
Push-Location $podProjectRoot
try {
  if (($UpgradeAdmin -or $UpgradeCatalog -or $UpgradeCreators -or $UpgradeManaged -or $UpgradeOfficial) -and $Action -ne 'restore-check') { throw 'Upgrade flags are restricted to a new restore-check copy.' }
  $podConfig = Join-Path $podProjectRoot 'configs/pod.local.yml'
  $podStatePath = Join-Path $podProjectRoot '.local/processes.json'
  if ($UpgradeCreators -and ($UpgradeAdmin -or $UpgradeCatalog)) { throw 'Upgrade flags cannot be combined.' }
  if ($UpgradeManaged -and ($UpgradeAdmin -or $UpgradeCatalog -or $UpgradeCreators -or $UpgradeOfficial)) { throw 'Upgrade flags cannot be combined.' }
  if ($Action -in @('backup','bundle','restore-check')) {
    if ($Action -eq 'restore-check' -and -not $Bundle) { throw 'Provide -Bundle with a trusted local bundle directory.' }
    $podExtra = @()
    if ($UpgradeAdmin) {
      if ($Action -ne 'restore-check') { throw 'UpgradeAdmin is restricted to a new restore-check copy.' }
      $podExtra += '-upgrade-admin'
    }
    if ($UpgradeCatalog) {
      if ($Action -ne 'restore-check' -or $UpgradeAdmin) { throw 'UpgradeCatalog is restricted to a new restore-check copy and cannot combine with UpgradeAdmin.' }
      $podExtra += '-upgrade-catalog'
    }
    if ($UpgradeCreators) { $podExtra += '-upgrade-creators' }
    if ($UpgradeManaged) { $podExtra += '-upgrade-managed' }
 if ($UpgradeOfficial) { if ($UpgradeAdmin -or $UpgradeCatalog -or $UpgradeCreators -or $UpgradeManaged) { throw 'Upgrade flags cannot be combined.' }; $podExtra += '-upgrade-official' }
    & go run ./cmd/podsetup -mode $Action -config $podConfig -mysqldump $MySQLDump -bundle $Bundle @podExtra
    if ($LASTEXITCODE -ne 0) { throw "POD $Action failed" }
    return
  }
  if ($Action -in @('prepare','init','migrate','check','doctor')) {
    & go run ./cmd/podsetup -mode $Action -config $podConfig -redis-db $RedisDB
    if ($LASTEXITCODE -ne 0) { throw "podsetup $Action failed" }
    return
  }
  if ($Action -eq 'test') {
    if (-not $TestConfig) { throw 'Provide -TestConfig with an isolated simulation config. Do not test against a qwen execution config.' }
    $podTestConfig = (Resolve-Path -LiteralPath $TestConfig -ErrorAction Stop).Path
    if (-not (Select-String -LiteralPath $podTestConfig -Pattern '^\s+mode:\s*simulation\s*(#.*)?$')) { throw 'POD integration tests require an explicit simulation config.' }
    if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) { throw 'Install pnpm and add it to PATH before testing studio.' }
    & go test ./internal/... ./pkg/... ./cmd/... ./sql/pod
    if ($LASTEXITCODE -ne 0) { throw 'Go checks failed' }
    $env:POD_TEST_CONFIG = $podTestConfig
    & go test ./test/pod -count=1
    if ($LASTEXITCODE -ne 0) { throw 'POD integration checks failed' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go static checks failed' }
    & go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    Push-Location (Join-Path $podProjectRoot 'studio')
    try {
      & pnpm test
      if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
      & pnpm build
      if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    } finally { Pop-Location }
    Push-Location (Join-Path $podProjectRoot 'web')
    try {
      $podAdminTests = @(Get-ChildItem -LiteralPath 'test' -Filter 'pod-*.test.mjs' -File | Sort-Object Name | ForEach-Object { $_.FullName })
      if ($podAdminTests.Count -eq 0) { throw 'No POD admin frontend tests found.' }
      & node --test @podAdminTests
      if ($LASTEXITCODE -ne 0) { throw 'Admin frontend tests failed' }
      & pnpm build:prod
      if ($LASTEXITCODE -ne 0) { throw 'Admin frontend build failed' }
    } finally { Pop-Location }
    return
  }
  if ($Action -eq 'stop') {
    if (-not (Test-Path -LiteralPath $podStatePath)) { Write-Output 'No POD-owned processes recorded.'; return }
    $podProcesses = Get-Content -LiteralPath $podStatePath -Raw -Encoding UTF8 | ConvertFrom-Json
    $ownedProcesses = @()
    foreach ($record in $podProcesses) {
      $process = Get-Process -Id $record.id -ErrorAction SilentlyContinue
      if ($process -and $process.Path -eq $record.path -and $process.StartTime.ToUniversalTime() -eq ([datetime]$record.startedAt).ToUniversalTime()) {
        $ownedProcesses += $process
      } elseif ($process) { throw 'PID was reused; refusing to stop an unrelated process.' }
    }
    foreach ($process in $ownedProcesses) {
      if (-not $process.HasExited) { $process.Kill(); if (-not $process.WaitForExit(5000)) { throw 'A POD-owned process did not exit in time.' } }
    }
    Remove-Item -LiteralPath $podStatePath
    Write-Output 'Stopped recorded POD development processes.'
    return
  }
  if (-not (Get-Command node -ErrorAction SilentlyContinue)) { throw 'Install Node.js 22.12+ and add it to PATH before starting studio.' }
  if (Test-Path -LiteralPath $podStatePath) { throw 'A process record already exists. Run stop before starting again.' }
  & go run ./cmd/podsetup -mode check -config $podConfig
  if ($LASTEXITCODE -ne 0) { throw 'Isolated environment check failed' }
  $imageAIConfig = Select-String -LiteralPath $podConfig -Pattern '^\s+imageAIURL:\s*["'']?http://127\.0\.0\.1:(\d+)["'']?\s*$'
  $imageAIPort = if ($imageAIConfig) { [int]$imageAIConfig.Matches[0].Groups[1].Value } else { 0 }
  $imagePython = Join-Path $podProjectRoot '.local/image-ai-venv/Scripts/python.exe'
  $imageModels = Join-Path $podProjectRoot '.local/image-ai-models'
  if ($imageAIPort -and (-not (Test-Path -LiteralPath $imagePython) -or -not (Test-Path -LiteralPath (Join-Path $imageModels 'manifest.json')))) { throw 'Image AI is enabled but not installed. See scripts/image-ai/README.md.' }
  $ports = @(18110,5177,5178)
  if ($imageAIPort) { $ports += $imageAIPort }
  foreach ($port in $ports) {
    $probe = [System.Net.Sockets.TcpClient]::new()
    try {
      $attempt = $probe.ConnectAsync('127.0.0.1', $port)
      try { $null = $attempt.Wait(400) } catch { }
      if ($probe.Connected) { throw "Port $port is already in use. No process was stopped." }
    } finally { $probe.Dispose() }
  }
  $podBinDir = Join-Path $podProjectRoot '.local/bin'
  New-Item -ItemType Directory -Path $podBinDir -Force | Out-Null
  $podExe = Join-Path $podBinDir 'pod-server.exe'
  & go build -o $podExe ./cmd/server
  if ($LASTEXITCODE -ne 0) { throw 'Backend build failed' }
  $launchWorker = [bool](Select-String -LiteralPath $podConfig -Pattern '^\s+mode:\s*(simulation|qwen|managed)\s*(#.*)?$')
  $podWorker = Join-Path $podBinDir 'pod-worker.exe'
  if ($launchWorker) {
    & go build -o $podWorker ./cmd/worker
    if ($LASTEXITCODE -ne 0) { throw 'Worker build failed' }
  }
  $podNode = (Get-Command node -ErrorAction Stop).Source
  $podVite = Join-Path $podProjectRoot 'studio/node_modules/vite/bin/vite.js'
  if (-not (Test-Path -LiteralPath $podVite)) { throw 'Run pnpm install in studio first.' }
  $podAdminVite = Join-Path $podProjectRoot 'web/node_modules/vite/bin/vite.js'
  if (-not (Test-Path -LiteralPath $podAdminVite)) { throw 'Run pnpm install in web first.' }
  $started = @()
  try {
    $expectedPaths = @()
    if ($imageAIPort) {
      $imageScript = Join-Path $podProjectRoot 'scripts/image-ai/server.py'
      $started += Start-Process -FilePath $imagePython -ArgumentList @(('"' + $imageScript + '"'), '--models', ('"' + $imageModels + '"'), '--port', $imageAIPort) -WorkingDirectory $podProjectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $podProjectRoot '.local/image-ai.out.log') -RedirectStandardError (Join-Path $podProjectRoot '.local/image-ai.err.log')
      $expectedPaths += $imagePython
      $imageReady = $false
      for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($started[0].HasExited) { throw 'Image AI exited. Check .local/image-ai.err.log.' }
        try { $result = Invoke-RestMethod -Uri "http://127.0.0.1:$imageAIPort/health" -TimeoutSec 1; if ($result.ready) { $imageReady = $true; break } } catch { }
        Start-Sleep -Milliseconds 500
      }
      if (-not $imageReady) { throw 'Image AI did not become ready in time.' }
    }
    $started += Start-Process -FilePath $podExe -ArgumentList @('-config', ('"' + $podConfig + '"')) -WorkingDirectory $podProjectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $podProjectRoot '.local/server.out.log') -RedirectStandardError (Join-Path $podProjectRoot '.local/server.err.log')
    $started += Start-Process -FilePath $podNode -ArgumentList @(('"' + $podVite + '"')) -WorkingDirectory (Join-Path $podProjectRoot 'studio') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $podProjectRoot '.local/studio.out.log') -RedirectStandardError (Join-Path $podProjectRoot '.local/studio.err.log')
    $expectedPaths += @($podExe, $podNode)
    $started += Start-Process -FilePath $podNode -ArgumentList @(('"' + $podAdminVite + '"')) -WorkingDirectory (Join-Path $podProjectRoot 'web') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $podProjectRoot '.local/admin.out.log') -RedirectStandardError (Join-Path $podProjectRoot '.local/admin.err.log')
    $expectedPaths += $podNode
    if ($launchWorker) {
      $started += Start-Process -FilePath $podWorker -ArgumentList @('-config', ('"' + $podConfig + '"')) -WorkingDirectory $podProjectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $podProjectRoot '.local/worker.out.log') -RedirectStandardError (Join-Path $podProjectRoot '.local/worker.err.log')
      $expectedPaths += $podWorker
    }
    $records = @()
    for ($index = 0; $index -lt $started.Count; $index++) {
      # A just-started process can temporarily have no readable executable path.
      $metadata = $null
      for ($attempt = 0; $attempt -lt 20; $attempt++) {
        $metadata = Get-Process -Id $started[$index].Id -ErrorAction Stop
        if ($metadata.Path -eq $expectedPaths[$index]) { break }
        Start-Sleep -Milliseconds 50
      }
      if ($metadata.Path -ne $expectedPaths[$index]) { throw 'Could not verify the launched process executable path.' }
      $records += @{id=$metadata.Id;path=$metadata.Path;startedAt=$metadata.StartTime.ToUniversalTime().ToString('o')}
    }
    $records | ConvertTo-Json | Set-Content -LiteralPath $podStatePath -Encoding UTF8
  } catch {
    foreach ($process in $started) { if (-not $process.HasExited) { $process.Kill() } }
    throw
  }
  Write-Output 'POD processes launched. Creator: http://127.0.0.1:5177 ; Admin: http://127.0.0.1:5178 ; API health: http://127.0.0.1:18110/health'
  Write-Output 'Check the URLs and .local logs to verify readiness. Use scripts/pod.ps1 -Action stop to stop only these processes.'
} finally { Pop-Location }
