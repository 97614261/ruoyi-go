<#
.SYNOPSIS
    Local Windows reliability verification; preview by default.
.DESCRIPTION
    Execute writes NEW fixtures only to ruoyi_go_mt / local Redis DB13.
    No migration, old fixture reopening, existing password reset or remote access.
    Six transaction/restart cases use only a separate owned test executable.
    Quick is tool acceptance, not a replacement for the 30-minute standard run.
#>
[CmdletBinding()]
param(
 [switch]$Execute,
 [switch]$Plan,
 [ValidateSet('quick','standard')][string]$Profile='quick',
 [string]$GoPath='go',
 [string]$Config='configs/multitenant.local.yml',
 [string]$Accounts='logs/multitenant-accounts.local.json'
)
$ErrorActionPreference='Stop'
if ($Execute -and $Plan) { throw 'Choose -Execute or -Plan.' }
if (-not $Execute) {
 [ordered]@{mode='PLAN';profile=$Profile;effects='No build/config/DB/Redis/process actions. Execute creates three fresh test enterprises, runs six faults and mixed business cycles, reconciles SQL and disables fixtures.';baselineSeconds=$(if($Profile -eq 'quick'){15}else{120});loadSeconds=$(if($Profile -eq 'quick'){120}else{1800});cooldownSeconds=$(if($Profile -eq 'quick'){30}else{300});excluded='Linux runtime, remote deployment, production capacity, DDL, git commit/push'} | ConvertTo-Json
 exit 0
}
if ($env:OS -ne 'Windows_NT') { throw 'Execute currently requires Windows native process metrics. Linux runtime is not covered.' }
$repo=Split-Path -Parent $PSScriptRoot
$go=(Get-Command $GoPath -CommandType Application -ErrorAction Stop).Source
Push-Location -LiteralPath $repo
try {
 foreach($privateFile in @($Config,$Accounts)) {
  if(-not (Test-Path -LiteralPath $privateFile -PathType Leaf)){throw 'Private local prerequisites missing. No password reset is performed.'}
 }
 $id='v'+[datetime]::UtcNow.ToString('MMddHHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8)
 $relative='logs/erp-verify-'+$id
 $root=Join-Path $repo $relative
 if(Test-Path -LiteralPath $root){throw 'Fresh evidence directory required.'}
 New-Item -ItemType Directory -Path $root|Out-Null
 # Evidence can contain synthetic identifiers and raw diagnostics; keep it local.
 $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 & icacls.exe $root '/inheritance:r' '/grant:r' ('*'+$sid+':(OI)(CI)F') | Out-Null
 if($LASTEXITCODE -ne 0){throw 'Cannot restrict evidence directory ACL.'}
 $binary=Join-Path $root 'runner.exe'
 & $go build -mod=readonly -o $binary ./test/erpreliability
 if($LASTEXITCODE -ne 0){throw 'Reliability helper build failed; no fixtures were created.'}
 & $binary -mode verify -execute -root $relative -config $Config -accounts $Accounts -profile $Profile
 if($LASTEXITCODE -ne 0){throw "Reliability verification FAILED. Retain evidence: $root"}
 $receipt=Get-Content -LiteralPath (Join-Path $root 'run-complete.json') -Raw -Encoding UTF8|ConvertFrom-Json
 $cleanup=Get-Content -LiteralPath (Join-Path $root 'fixture-cleanup.json') -Raw -Encoding UTF8|ConvertFrom-Json
 if(-not $receipt.passed -or -not $cleanup.disabled -or $cleanup.ranges -ne 0){throw 'Final receipt/cleanup missing.'}
 Write-Host ('PASS; local evidence: '+$root)
} finally {Pop-Location}
