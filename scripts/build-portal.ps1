param(
 [ValidateSet('windows','linux')][string]$TargetOS='windows',
 [switch]$SkipAdmin
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Push-Location $root
$oldWork=$env:GOWORK
$oldOS=$env:GOOS
$oldArch=$env:GOARCH
try {
 if(-not $SkipAdmin){
  Push-Location (Join-Path $root 'admin-web')
  try {
   & npm.cmd run build:prod
   if($LASTEXITCODE -ne 0){throw '管理后台构建失败'}
  } finally {Pop-Location}
 }
 if(-not (Test-Path -LiteralPath 'portal/admin-dist/index.html')){throw '请先构建管理后台'}
 $env:GOWORK='off'
 $env:GOOS=$TargetOS
 $env:GOARCH='amd64'
 New-Item -ItemType Directory 'dist' -Force | Out-Null
 $suffix=if($TargetOS -eq 'windows'){'.exe'}else{''}
 $binary=Join-Path 'dist' ('portal-'+$TargetOS+'-amd64'+$suffix)
 & go build -trimpath -ldflags "-X main.defaultConfigPath=configs/portal.local.yml -X main.portalBuild=true" -o $binary ./cmd/server
 if($LASTEXITCODE -ne 0){throw 'Go构建失败'}
 Write-Output "门户二进制：$binary。另行准备私有配置、已初始化独立库及持久附件目录。"
} finally {
 $env:GOWORK=$oldWork
 $env:GOOS=$oldOS
 $env:GOARCH=$oldArch
 Pop-Location
}
