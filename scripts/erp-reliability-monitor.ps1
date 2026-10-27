# Internal companion of verify-erp-reliability.ps1, never a service controller.
[CmdletBinding()]
param(
 [Parameter(Mandatory=$true)][string]$Evidence,
 [Parameter(Mandatory=$true)][int]$ChildID,
 [Parameter(Mandatory=$true)][int]$RunnerID,
 [Parameter(Mandatory=$true)][string]$Binary
)
$ErrorActionPreference='Stop'
$repo=Split-Path -Parent $PSScriptRoot
$base=[IO.Path]::GetFullPath($Evidence)
$root=Split-Path $base -Parent
$logs=[IO.Path]::GetFullPath((Join-Path $repo 'logs'))
if((Split-Path $root -Parent) -ne $logs -or (Split-Path $base -Leaf) -ne 'native' -or -not (Split-Path $root -Leaf).StartsWith('erp-verify-')){throw 'Unexpected evidence root.'}
if([IO.Path]::GetFullPath($Binary) -ne (Join-Path $root 'runner.exe')){throw 'Owned executable required.'}
$runner=Get-CimInstance Win32_Process -Filter ('ProcessId='+$RunnerID)
$child=Get-CimInstance Win32_Process -Filter ('ProcessId='+$ChildID)
if($null -eq $runner -or $null -eq $child -or $runner.ExecutablePath -ne $Binary -or $child.ExecutablePath -ne $Binary -or $child.ParentProcessId -ne $RunnerID){throw 'Owned child identity not verified.'}
$ids=@(@{name='test-server';id=$ChildID;created=$child.CreationDate},@{name='load-client';id=$RunnerID;created=$runner.CreationDate})
$path=Join-Path $base 'windows-process-metrics.jsonl'
if(Test-Path -LiteralPath $path){throw 'Prior monitor evidence retained.'}
$last=@{};$cores=[Environment]::ProcessorCount
while(-not(Test-Path -LiteralPath (Join-Path $base 'stop-monitor.json'))){
 $now=[datetime]::UtcNow;$samples=@();$errors=@()
 foreach($entry in $ids){
  try{
   $p=Get-CimInstance Win32_Process -Filter ('ProcessId='+$entry.id)
   if($null -eq $p -or $p.CreationDate -ne $entry.created -or $p.ExecutablePath -ne $Binary){throw 'Owned process missing/changed.'}
   $ticks=[double]$p.UserModeTime+[double]$p.KernelModeTime;$pct=$null
   if($last.ContainsKey($entry.id)){$elapsed=($now-$last[$entry.id].at).TotalSeconds;if($elapsed -gt 0){$pct=($ticks-$last[$entry.id].ticks)/10000000/$elapsed*100}}
   $last[$entry.id]=@{at=$now;ticks=$ticks}
   $samples+=[ordered]@{name=$entry.name;pid=$entry.id;workingSetBytes=[long]$p.WorkingSetSize;cpuOneCorePercent=$pct;cpuMachinePercent=$(if($null -ne $pct){$pct/$cores}else{$null})}
  }catch{$errors+=$entry.name+': unavailable'}
 }
 $sample=[ordered]@{atUtc=$now.ToString('o');logicalCPUs=$cores;processes=$samples;errors=$errors}
 $json=$sample|ConvertTo-Json -Depth 5 -Compress
 $json|Add-Content -LiteralPath $path -Encoding UTF8
 $temporary=Join-Path $base 'latest.tmp'
 [IO.File]::WriteAllText($temporary,$json,[Text.UTF8Encoding]::new($false))
 Move-Item -LiteralPath $temporary -Destination (Join-Path $base 'latest.json') -Force
 Start-Sleep -Milliseconds 1000
}
