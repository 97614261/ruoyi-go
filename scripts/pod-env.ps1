$ErrorActionPreference = 'Stop'
$podProjectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$env:GOWORK = 'off'
$env:GOCACHE = Join-Path $podProjectRoot '.local/go-build'
$env:GOTMPDIR = Join-Path $podProjectRoot '.local/go-tmp'
New-Item -ItemType Directory -Path $env:GOCACHE, $env:GOTMPDIR -Force | Out-Null
# Keep Go test temporary paths in the same writable workspace. This is process-local.
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
