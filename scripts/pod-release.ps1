param([ValidateSet('amd64','arm64')][string]$Architecture = 'amd64')
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'pod-env.ps1')
Push-Location $podProjectRoot
$podOldGOOS=$env:GOOS
$podOldGOARCH=$env:GOARCH
$podOldCGO=$env:CGO_ENABLED
try {
  foreach ($podFrontend in @('studio','web')) {
    Push-Location (Join-Path $podProjectRoot $podFrontend)
    try {
      if ($podFrontend -eq 'studio') { & pnpm build } else { & pnpm build:prod }
      if ($LASTEXITCODE -ne 0) { throw "$podFrontend build failed" }
    } finally { Pop-Location }
  }
  $podRelease=Join-Path $podProjectRoot ('.local/releases/pod-linux-' + $Architecture + '-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff'))
  if (Test-Path -LiteralPath $podRelease) { throw 'Release target already exists.' }
  New-Item -ItemType Directory -Path (Join-Path $podRelease 'bin') | Out-Null
  $env:GOOS='linux'; $env:GOARCH=$Architecture; $env:CGO_ENABLED='0'
  foreach ($podCommand in @(@('server','pod-api'),@('worker','pod-worker'),@('podsetup','podsetup'))) {
    & go build -trimpath -o (Join-Path $podRelease ('bin/' + $podCommand[1])) ('./cmd/' + $podCommand[0])
    if ($LASTEXITCODE -ne 0) { throw "Linux $($podCommand[0]) build failed; incomplete release retained." }
  }
  Copy-Item -LiteralPath 'studio/dist' -Destination (Join-Path $podRelease 'studio') -Recurse
  Copy-Item -LiteralPath 'web/dist' -Destination (Join-Path $podRelease 'admin') -Recurse
  Copy-Item -LiteralPath 'deploy' -Destination (Join-Path $podRelease 'deploy') -Recurse
  New-Item -ItemType Directory -Path (Join-Path $podRelease 'image-ai') | Out-Null
  foreach ($podImageFile in @('README.md','requirements.txt','server.py','setup_models.py','smoke.py')) {
    Copy-Item -LiteralPath (Join-Path 'scripts/image-ai' $podImageFile) -Destination (Join-Path $podRelease ('image-ai/' + $podImageFile))
  }
  New-Item -ItemType Directory -Path (Join-Path $podRelease 'docs'),(Join-Path $podRelease 'configs') | Out-Null
  Copy-Item -LiteralPath 'configs/pod.example.yml' -Destination (Join-Path $podRelease 'configs/pod.example.yml')
  foreach ($podDoc in @('POD_BETA_OPERATIONS.md','POD_USER_GUIDE.md','POD_LOCAL.md','POD_QWEN.md')) {
    Copy-Item -LiteralPath (Join-Path 'docs' $podDoc) -Destination (Join-Path $podRelease ('docs/' + $podDoc))
  }
  $podManifest=@(Get-ChildItem -LiteralPath $podRelease -File -Recurse | Sort-Object FullName | ForEach-Object {
    [ordered]@{path=[IO.Path]::GetRelativePath($podRelease,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
  })
  [IO.File]::WriteAllText((Join-Path $podRelease 'manifest.json'),(ConvertTo-Json -InputObject ([ordered]@{format=1;platform="linux/$Architecture";containsCredentials=$false;files=$podManifest}) -Depth 5),[Text.UTF8Encoding]::new($false))
  Write-Output "Release prepared: $podRelease (no local configuration, accounts, keyring, user data or model weights). Linux runtime acceptance still required."
} finally {
  $env:GOOS=$podOldGOOS; $env:GOARCH=$podOldGOARCH; $env:CGO_ENABLED=$podOldCGO
  Pop-Location
}
