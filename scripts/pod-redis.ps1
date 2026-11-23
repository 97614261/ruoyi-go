param([ValidateSet('start','stop')][string]$Action='start')
$ErrorActionPreference='Stop'
$podRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$redisDir=Join-Path $podRoot '.local/redis5'
$recordPath=Join-Path $redisDir 'process.json'
if($Action -eq 'stop') {
 if(-not (Test-Path -LiteralPath $recordPath)){ Write-Output 'No POD Redis process recorded.';return }
 $record=Get-Content -LiteralPath $recordPath -Raw -Encoding UTF8 | ConvertFrom-Json
 $owned=Get-Process -Id $record.id -ErrorAction SilentlyContinue
 if($owned) {
  if($owned.Path -ne $record.path -or $owned.StartTime.ToUniversalTime() -ne ([datetime]$record.startedAt).ToUniversalTime()){throw 'Redis process identity changed; no process was stopped.'}
  $podProcessesPath=Join-Path $podRoot '.local/processes.json'
  if(Test-Path -LiteralPath $podProcessesPath){throw 'Stop POD API/Worker with scripts/pod.ps1 -Action stop before stopping Redis.'}
  . (Join-Path $PSScriptRoot 'pod-env.ps1')
  Push-Location $podRoot
  try{& go run ./cmd/podsetup -mode stop-cache -config (Join-Path $podRoot 'configs/pod.local.yml');if($LASTEXITCODE -ne 0){throw 'POD Redis shutdown was not confirmed.'}}finally{Pop-Location}
  if(-not $owned.WaitForExit(5000)){throw 'POD Redis has not exited yet.'}
 }
 Remove-Item -LiteralPath $recordPath
 return
}
$redisExe=Join-Path $podRoot '.local/tools/redis5/redis-server.exe'
$redisConfig=Join-Path $redisDir 'redis.conf'
if(Test-Path -LiteralPath $recordPath){
 $record=Get-Content -LiteralPath $recordPath -Raw -Encoding UTF8 | ConvertFrom-Json
 $existing=Get-Process -Id $record.id -ErrorAction SilentlyContinue
 if($existing){
  if($existing.Path -ne $redisExe -or $existing.Path -ne $record.path -or $existing.StartTime.ToUniversalTime() -ne ([datetime]$record.startedAt).ToUniversalTime()){throw 'POD Redis record points to a different process; no process was changed.'}
  Write-Output 'The recorded POD-only Redis process is already running.';return
 }
 Remove-Item -LiteralPath $recordPath
}
if(-not (Test-Path -LiteralPath $redisExe) -or -not (Test-Path -LiteralPath $redisConfig)){throw 'Prepare the portable Redis executable and private local configuration first. See POD_LOCAL.md.'}
$probe=[Net.Sockets.TcpClient]::new()
try{try{$null=$probe.ConnectAsync('127.0.0.1',6387).Wait(400)}catch{};if($probe.Connected){throw 'Port 6387 is already occupied; no process was stopped.'}}finally{$probe.Dispose()}
$started=Start-Process -FilePath $redisExe -ArgumentList @(('"'+$redisConfig+'"')) -WorkingDirectory $redisDir -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $redisDir 'redis.out.log') -RedirectStandardError (Join-Path $redisDir 'redis.err.log')
try{
 $metadata=$null
 for($attempt=0;$attempt -lt 20;$attempt++){ $metadata=Get-Process -Id $started.Id -ErrorAction Stop;if($metadata.Path -eq $redisExe){break};Start-Sleep -Milliseconds 50 }
 if($metadata.Path -ne $redisExe){throw 'Could not verify the launched Redis executable.'}
 @{id=$metadata.Id;path=$metadata.Path;startedAt=$metadata.StartTime.ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content -LiteralPath $recordPath -Encoding UTF8
}catch{if(-not $started.HasExited){$started.Kill()};throw}
Write-Output 'POD-only portable Redis launched on 127.0.0.1:6387. Existing Redis services were not changed.'
