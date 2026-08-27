# contractcheck.ps1 - start both implementations, run contract probes, and clean up.
#
# PowerShell 5.1 reads BOM-less scripts as ANSI, so keep this file ASCII-only.

param(
    [switch]$All,
    [switch]$FileProbes,
    [switch]$WriteProbes,
    [switch]$CrudProbes,
    [switch]$PermissionProbes,
    [switch]$ValidationProbes,
    [switch]$AllowDifferences,
    [string]$JavaRoot = '',
    [string]$JavaHome = '',
    [string]$Username = 'admin',
    [string]$Password = 'admin123',
    [int]$GoPort = 8080,
    [int]$JavaPort = 8081,
    [int]$GoRedisDB = 0,
    [int]$JavaRedisDB = 1,
    [int]$ReadyTimeoutSeconds = 120
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$workspace = Split-Path -Parent $root
if ($JavaRoot -eq '') { $JavaRoot = Join-Path $workspace 'RuoYi-Vue-master' }
$javaAdminRoot = Join-Path $JavaRoot 'ruoyi-admin'
$javaJar = Join-Path $javaAdminRoot 'target\ruoyi-admin.jar'
$resultsRoot = Join-Path $root 'test\results'
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$runRoot = Join-Path $resultsRoot ".contractcheck-run-$stamp"
$serverExe = Join-Path $runRoot 'ruoyi-server.exe'
$checkerExe = Join-Path $runRoot 'contractcheck.exe'
$routeAuditExe = Join-Path $runRoot 'routeaudit.exe'
$goUploadRoot = Join-Path $runRoot 'go-upload'
$javaUploadRoot = Join-Path $runRoot 'java-upload'
$goLog = Join-Path $resultsRoot "go-contractcheck-$stamp.log"
$javaLog = Join-Path $resultsRoot "java-contractcheck-$stamp.log"
$goErr = Join-Path $resultsRoot "go-contractcheck-$stamp.err.log"
$javaErr = Join-Path $resultsRoot "java-contractcheck-$stamp.err.log"
$contractLog = Join-Path $resultsRoot "contractcheck-$stamp.log"
$goProcess = $null
$javaProcess = $null
$exitCode = 2

function Get-DataSource {
    $configPath = Join-Path $root 'configs\application.yml'
    $localPath = Join-Path $root 'configs\application.local.yml'
    if (Test-Path -LiteralPath $localPath) { $configPath = $localPath }

    $line = Select-String -Path $configPath -Pattern '^\s*dsn:\s*"(.+)"\s*$' | Select-Object -First 1
    if ($null -eq $line) { throw "could not find mysql.dsn in $configPath" }
    $dsn = $line.Matches[0].Groups[1].Value
    if ($dsn -notmatch '^([^:]+):(.*)@tcp\(([^)]+)\)/([^?]+)') {
        throw "unsupported mysql.dsn format in $configPath"
    }
    return @{
        User = $Matches[1]
        Password = $Matches[2]
        HostPort = $Matches[3]
        Database = $Matches[4]
        Config = $configPath
    }
}

function Resolve-JavaExe {
    if ($JavaHome -eq '') { return 'java' }
    $exe = Join-Path $JavaHome 'bin\java.exe'
    if (-not (Test-Path -LiteralPath $exe)) { throw "java.exe not found under $JavaHome" }
    return $exe
}

function Assert-PortFree([int]$Port) {
    $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)
    if ($listeners.Count -ne 0) { throw "port $Port is already in use" }
}

function Wait-Port([string]$Name, [int]$Port, $Process, [int]$Seconds) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        $Process.Refresh()
        if ($Process.HasExited) { throw "$Name exited before readiness (exit=$($Process.ExitCode))" }
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $task = $client.ConnectAsync('127.0.0.1', $Port)
            if ($task.Wait(750) -and $client.Connected) { return }
        }
        catch { }
        finally { $client.Dispose() }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name readiness timeout after $Seconds seconds"
}

function Stop-ExactProcess($Process) {
    if ($null -eq $Process) { return }
    $running = Get-Process -Id $Process.Id -ErrorAction SilentlyContinue
    if ($null -ne $running) {
        Stop-Process -Id $Process.Id -Force -ErrorAction SilentlyContinue
        Wait-Process -Id $Process.Id -Timeout 15 -ErrorAction SilentlyContinue
    }
}

if ($All) {
    $FileProbes = $true
    $WriteProbes = $true
    $CrudProbes = $true
    $PermissionProbes = $true
    $ValidationProbes = $true
}

if ($GoRedisDB -eq $JavaRedisDB) { throw 'Go and Java Redis DB values must differ' }
if (-not (Test-Path -LiteralPath $javaJar)) { throw "Java jar not found: $javaJar" }

$dataSource = Get-DataSource
$javaExe = Resolve-JavaExe
$jdbcUrl = "jdbc:mysql://$($dataSource.HostPort)/$($dataSource.Database)?useUnicode=true&characterEncoding=utf8&zeroDateTimeBehavior=convertToNull&useSSL=true&serverTimezone=GMT%2B8"
$envNames = @(
    'SPRING_DATASOURCE_DRUID_MASTER_URL',
    'SPRING_DATASOURCE_DRUID_MASTER_USERNAME',
    'SPRING_DATASOURCE_DRUID_MASTER_PASSWORD',
    'SPRING_DATA_REDIS_DATABASE',
    'SERVER_PORT',
    'RUOYI_PROFILE',
    'UPLOAD_PATH'
)
$oldEnvironment = @{}
foreach ($name in $envNames) { $oldEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

try {
    Assert-PortFree $GoPort
    Assert-PortFree $JavaPort
    if (-not (Test-Path -LiteralPath $resultsRoot)) { New-Item -ItemType Directory -Path $resultsRoot | Out-Null }
    New-Item -ItemType Directory -Path $runRoot | Out-Null

    Push-Location $root
    try {
        & go build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "Go server build failed (exit=$LASTEXITCODE)" }
        & go build -o $checkerExe ./cmd/contractcheck
        if ($LASTEXITCODE -ne 0) { throw "contractcheck build failed (exit=$LASTEXITCODE)" }
        & go build -o $routeAuditExe ./cmd/routeaudit
        if ($LASTEXITCODE -ne 0) { throw "routeaudit build failed (exit=$LASTEXITCODE)" }
        & $routeAuditExe -java-root $JavaRoot
        if ($LASTEXITCODE -ne 0) { throw "routeaudit failed (exit=$LASTEXITCODE)" }
    }
    finally { Pop-Location }

    [Environment]::SetEnvironmentVariable('SPRING_DATASOURCE_DRUID_MASTER_URL', $jdbcUrl, 'Process')
    [Environment]::SetEnvironmentVariable('SPRING_DATASOURCE_DRUID_MASTER_USERNAME', $dataSource.User, 'Process')
    [Environment]::SetEnvironmentVariable('SPRING_DATASOURCE_DRUID_MASTER_PASSWORD', $dataSource.Password, 'Process')
    [Environment]::SetEnvironmentVariable('SPRING_DATA_REDIS_DATABASE', [string]$JavaRedisDB, 'Process')
    [Environment]::SetEnvironmentVariable('SERVER_PORT', [string]$JavaPort, 'Process')
    [Environment]::SetEnvironmentVariable('RUOYI_PROFILE', $javaUploadRoot, 'Process')
    $javaProcess = Start-Process -FilePath $javaExe -ArgumentList @('-Xmx512m', '-jar', $javaJar) -WorkingDirectory $javaAdminRoot -RedirectStandardOutput $javaLog -RedirectStandardError $javaErr -WindowStyle Hidden -PassThru
    foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name, $oldEnvironment[$name], 'Process') }

    [Environment]::SetEnvironmentVariable('UPLOAD_PATH', $goUploadRoot, 'Process')
    $goProcess = Start-Process -FilePath $serverExe -WorkingDirectory $root -RedirectStandardOutput $goLog -RedirectStandardError $goErr -WindowStyle Hidden -PassThru
    [Environment]::SetEnvironmentVariable('UPLOAD_PATH', $oldEnvironment['UPLOAD_PATH'], 'Process')
    Wait-Port 'Go' $GoPort $goProcess $ReadyTimeoutSeconds
    Wait-Port 'Java' $JavaPort $javaProcess $ReadyTimeoutSeconds

    $arguments = @(
        "-go-base=http://127.0.0.1:$GoPort",
        "-java-base=http://127.0.0.1:$JavaPort",
        "-go-redis-db=$GoRedisDB",
        "-java-redis-db=$JavaRedisDB",
        "-username=$Username",
        "-password=$Password"
    )
    if ($FileProbes) { $arguments += '-file-probes' }
    if ($WriteProbes) { $arguments += '-write-probes' }
    if ($CrudProbes) { $arguments += '-crud-probes' }
    if ($PermissionProbes) { $arguments += '-permission-probes' }
    if ($ValidationProbes) { $arguments += '-validation-probes' }
    if ($AllowDifferences) { $arguments += '-fail-on-diff=false' }

    $output = @(& $checkerExe @arguments 2>&1)
    $exitCode = $LASTEXITCODE
    $output | Set-Content -LiteralPath $contractLog -Encoding UTF8
    $output | ForEach-Object { Write-Output $_ }
    Write-Output "contractcheck_log=$contractLog"
}
catch {
    Write-Error $_
    if (Test-Path -LiteralPath $goErr) { Get-Content -LiteralPath $goErr -Tail 40 }
    if (Test-Path -LiteralPath $javaErr) { Get-Content -LiteralPath $javaErr -Tail 40 }
    $exitCode = 2
}
finally {
    foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name, $oldEnvironment[$name], 'Process') }
    Stop-ExactProcess $goProcess
    Stop-ExactProcess $javaProcess

    if (Test-Path -LiteralPath $runRoot) {
        $resolvedRunRoot = (Resolve-Path -LiteralPath $runRoot).Path
        $resolvedResultsRoot = (Resolve-Path -LiteralPath $resultsRoot).Path
        if (-not $resolvedRunRoot.StartsWith($resolvedResultsRoot + '\.contractcheck-run-', [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "refusing to clean unexpected run directory: $resolvedRunRoot"
        }
        Remove-Item -LiteralPath $resolvedRunRoot -Recurse -Force
    }

    if ($exitCode -eq 0) {
        foreach ($file in @($goLog, $goErr, $javaLog, $javaErr)) {
            if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Force }
        }
    }
}

exit $exitCode
