param([ValidateSet('windows','linux')][string]$TargetOS='windows',[switch]$SkipAdmin)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Push-Location $root
$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldWork=$env:GOWORK;$oldCGO=$env:CGO_ENABLED
try{
 if(-not $SkipAdmin){Push-Location admin-web;try{& npm.cmd run build:prod;if($LASTEXITCODE -ne 0){throw '后台构建失败'}}finally{Pop-Location}}
 if(-not(Test-Path -LiteralPath portal/admin-dist/index.html)){throw '缺少后台构建产物'}
 $env:GOOS=$TargetOS;$env:GOARCH='amd64';$env:CGO_ENABLED='0';$env:GOWORK='off'
 $stamp=Get-Date -Format 'yyyyMMdd-HHmmss-fff'
 $folder=Join-Path $root "dist/portal-$TargetOS-$stamp"
 if(Test-Path -LiteralPath $folder){throw '目标已存在，不覆盖'}
 New-Item -ItemType Directory $folder -Force | Out-Null
 $suffix=if($TargetOS -eq 'windows'){'.exe'}else{''}
 foreach($item in @(@{Name='portal';Package='./cmd/server'},@{Name='portal-init';Package='./cmd/portalinit'},@{Name='portal-migrate';Package='./cmd/portalmigrate'},@{Name='portal-recovery-check';Package='./cmd/portalrecoverycheck'})){
  $target=Join-Path $folder ($item.Name+$suffix)
  if($item.Name -eq 'portal'){& go build -trimpath -ldflags '-X main.defaultConfigPath=configs/portal.local.yml -X main.portalBuild=true' -o $target $item.Package}else{& go build -trimpath -o $target $item.Package}
  if($LASTEXITCODE -ne 0){throw "构建失败: $($item.Name)"}
 }
 New-Item -ItemType Directory (Join-Path $folder 'configs'),(Join-Path $folder 'docs') -Force | Out-Null
 Copy-Item -LiteralPath configs/portal.example.yml -Destination (Join-Path $folder 'configs/portal.example.yml')
 foreach($name in @('PORTAL_DELIVERY.md','PORTAL_STEP56_ACCEPTANCE.md','PORTAL_PHASE2_PLAN.md','PORTAL_PHASE2_ACCEPTANCE.md','PORTAL_SCHEDULE_ACCEPTANCE.md')){Copy-Item -LiteralPath (Join-Path $root "docs/$name") -Destination (Join-Path $folder "docs/$name")}
 Copy-Item -LiteralPath (Join-Path $root '门户使用说明.md') -Destination (Join-Path $folder '使用说明.md')
 Copy-Item -LiteralPath admin-web/LICENSE -Destination (Join-Path $folder 'LICENSE.vue')
 Copy-Item -LiteralPath sql/portal/LICENSE.ruoyi -Destination (Join-Path $folder 'LICENSE.sql')
 $hashes=Get-ChildItem -LiteralPath $folder -File -Recurse | ForEach-Object{$hash=Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256;"$($hash.Hash.ToLower())  $($_.FullName.Substring($folder.Length+1).Replace('\','/'))"}
 [IO.File]::WriteAllLines((Join-Path $folder 'SHA256SUMS.txt'),$hashes,[Text.UTF8Encoding]::new($false))
 $zip=$folder+'.zip'
 Compress-Archive -LiteralPath (Get-ChildItem -LiteralPath $folder | Select-Object -ExpandProperty FullName) -DestinationPath $zip
 Write-Output "发布包: $zip"
 Write-Output '未包含本地配置、账号、数据库备份、附件和日志；服务器仍需私有配置、MySQL/Redis和持久化素材。'
}finally{$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:GOWORK=$oldWork;$env:CGO_ENABLED=$oldCGO;Pop-Location}
