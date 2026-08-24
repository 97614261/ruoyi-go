# compare.ps1 - benchmark the Go port against the original Java RuoYi, side by side.
#
# NOTE: ASCII only. PowerShell 5.1 decodes .ps1 as ANSI when there is no UTF-8 BOM.
#
# What it does:
#   1. builds the Go server and (unless -SkipBuild) the Java jar
#   2. starts ONE implementation at a time - never both, they would fight over CPU and MySQL
#   3. warms it up (JIT / connection pool / GC steady state), then benchmarks it
#   4. samples process memory while it runs
#   5. prints a side-by-side table
#
# Both hit the SAME MySQL and Redis with the SAME data, so the only variable is the middle layer.
#
# Usage:
#   .\scripts\compare.ps1                 # full run
#   .\scripts\compare.ps1 -SkipBuild      # jar already built
#   .\scripts\compare.ps1 -JavaOpts '-Xmx512m'
#
# The Java side is configured entirely through command line overrides -
# not a single file under RuoYi-Vue-master is modified.

param(
    [switch]$SkipBuild,
    [int]$Concurrency = 50,
    [int]$Requests = 1000,
    [int]$Warmup = 300,
    # JVM heap. Default heap is 1/4 of physical RAM, which makes the RSS
    # comparison meaningless - pin it to something you would actually deploy.
    [string]$JavaOpts = '-Xmx512m',
    [int]$GoPort = 8080,
    [int]$JavaPort = 8081,
    # Path to a JDK 17+ installation. RuoYi 3.9.2 is on Spring Boot 4.1.0,
    # whose baseline is JDK 17 - an older JDK cannot compile or run it.
    # Set this instead of changing your system JAVA_HOME.
    [string]$JavaHome = '',
    # Maven install directory, e.g. C:\Work\apache-maven-3.9.9
    # Only needed when mvn is not on PATH. Note that PowerShell and cmd.exe can
    # have different PATHs - mvn working in cmd does not mean it is visible here.
    [string]$MavenHome = ''
)

# NOT 'Stop'. Native tools write perfectly normal output to stderr -
# `java -version` writes the version there, maven writes warnings there -
# and with 'Stop' PowerShell turns every one of those lines into a terminating
# NativeCommandError. All error handling here is explicit: $LASTEXITCODE checks
# plus throw, which still unwinds to the finally block.
$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
$workspace = Split-Path -Parent $root
$javaRoot = Join-Path $workspace 'RuoYi-Vue-master'
$resultsDir = Join-Path $root 'test\results'
$javaJar = Join-Path $javaRoot 'ruoyi-admin\target\ruoyi-admin.jar'

function Write-Step($text) {
    Write-Host ""
    Write-Host "=============================================================" -ForegroundColor Cyan
    Write-Host " $text" -ForegroundColor Cyan
    Write-Host "=============================================================" -ForegroundColor Cyan
}

# Pull connection settings out of the Go config so the Java side points at the
# exact same database - no second place to keep in sync.
function Get-DataSource {
    $configPath = Join-Path $root 'configs\application.yml'
    $localPath = Join-Path $root 'configs\application.local.yml'
    if (Test-Path $localPath) { $configPath = $localPath }

    $line = Select-String -Path $configPath -Pattern '^\s*dsn:\s*"(.+)"\s*$' | Select-Object -First 1
    if ($null -eq $line) { throw "could not find mysql.dsn in $configPath" }
    $dsn = $line.Matches[0].Groups[1].Value

    # user:pass@tcp(host:port)/dbname?params
    if ($dsn -notmatch '^([^:]+):(.*)@tcp\(([^)]+)\)/([^?]+)') {
        throw "unrecognised DSN format: $dsn"
    }
    return @{
        User     = $Matches[1]
        Password = $Matches[2]
        HostPort = $Matches[3]
        Database = $Matches[4]
        Config   = $configPath
    }
}

# Resolve which java.exe to use and make sure it is new enough.
# Checking up front beats a wall of compiler errors ten minutes into a build.
function Resolve-Java {
    $exe = 'java'
    if ($JavaHome -ne '') {
        $exe = Join-Path $JavaHome 'bin\java.exe'
        if (-not (Test-Path $exe)) { throw "no java.exe under -JavaHome '$JavaHome'" }
    }

    # java -version writes to stderr, hence the 2>&1
    $versionText = (& $exe -version 2>&1 | ForEach-Object { "$_" }) -join "`n"
    # matches: version "21.0.1"  /  version "17"  /  version "1.8.0_181"
    if ($versionText -notmatch 'version "([0-9]+)(\.([0-9]+))?') {
        throw "could not parse java version from:`n$versionText"
    }
    $major = [int]$Matches[1]
    if ($major -eq 1) { $major = [int]$Matches[3] }   # 1.8 style

    if ($major -lt 17) {
        throw @"
JDK $major is too old. RuoYi 3.9.2 runs on Spring Boot 4.1.0, which needs JDK 17+.
Install a JDK 21 and either set JAVA_HOME, or pass it directly:
  .\scripts\compare.ps1 -JavaHome 'C:\Program Files\Eclipse Adoptium\jdk-21...'
"@
    }
    return @{ Exe = $exe; Major = $major }
}

# RuoYi's pom pins maven-compiler-plugin 3.13.0, which refuses to run on
# anything older than this.
$script:MinMavenVersion = [version]'3.6.3'

function Get-MavenVersion($exe) {
    try {
        $text = (& $exe -v 2>&1 | ForEach-Object { "$_" }) -join "`n"
        if ($text -match 'Apache Maven ([0-9]+\.[0-9]+\.[0-9]+)') { return [version]$Matches[1] }
    }
    catch { }
    return $null
}

# Find a mvn.cmd that is actually new enough.
#
# Two things make this fiddly:
#   - PATH is not enough. PowerShell (especially inside a conda base env) can
#     have a different PATH than cmd.exe, so `mvn -v` working in a cmd window
#     says nothing about whether Get-Command can see it here.
#   - Having *a* maven is not enough. An old one on PATH will happily start and
#     then fail one minute in with "requires Maven version 3.6.3". So every
#     candidate gets its version checked before we commit to it.
function Resolve-Maven {
    if ($MavenHome -ne '') {
        $exe = Join-Path $MavenHome 'bin\mvn.cmd'
        if (-not (Test-Path $exe)) { throw "no bin\mvn.cmd under -MavenHome '$MavenHome'" }
        $version = Get-MavenVersion $exe
        if ($null -eq $version -or $version -lt $script:MinMavenVersion) {
            throw "maven at '$MavenHome' is version $version, need $script:MinMavenVersion or newer"
        }
        Write-Host "Maven    : $exe  (version $version)" -ForegroundColor Green
        return $exe
    }

    # NOTE: do not name a loop variable $home - it is a read-only PowerShell
    # automatic variable and assigning to it aborts the script.
    $candidates = @()

    $onPath = Get-Command mvn -ErrorAction SilentlyContinue
    if ($null -ne $onPath) { $candidates += $onPath.Source }

    foreach ($mavenRoot in @($env:MAVEN_HOME, $env:M2_HOME)) {
        if ($mavenRoot) { $candidates += (Join-Path $mavenRoot 'bin\mvn.cmd') }
    }

    # Search two levels deep: unzipping apache-maven-3.9.9-bin.zip often leaves
    # C:\Work\apache-maven-3.9.9-bin\apache-maven-3.9.9\bin\mvn.cmd
    foreach ($base in @('C:\Work', 'C:\Program Files', 'C:\tools', 'C:\', $env:USERPROFILE)) {
        if (-not $base -or -not (Test-Path $base)) { continue }
        $level1 = Get-ChildItem $base -Directory -Filter 'apache-maven-*' -ErrorAction SilentlyContinue
        foreach ($dir in $level1) {
            $candidates += (Join-Path $dir.FullName 'bin\mvn.cmd')
            $level2 = Get-ChildItem $dir.FullName -Directory -Filter 'apache-maven-*' -ErrorAction SilentlyContinue
            foreach ($nested in $level2) {
                $candidates += (Join-Path $nested.FullName 'bin\mvn.cmd')
            }
        }
    }

    $found = @()
    foreach ($candidate in ($candidates | Select-Object -Unique)) {
        if (-not $candidate -or -not (Test-Path $candidate)) { continue }
        $version = Get-MavenVersion $candidate
        if ($null -eq $version) { continue }
        $found += [pscustomobject]@{ Exe = $candidate; Version = $version }
    }

    # Newest first, so a freshly unzipped 3.9.x wins over an old 3.6.0
    $usable = $found | Where-Object { $_.Version -ge $script:MinMavenVersion } | Sort-Object Version -Descending
    if ($usable.Count -gt 0) {
        Write-Host "Maven    : $($usable[0].Exe)  (version $($usable[0].Version))" -ForegroundColor Green
        return $usable[0].Exe
    }

    $detail = if ($found.Count -eq 0) { "  (none found)" }
    else { ($found | ForEach-Object { "  $($_.Version)  $($_.Exe)" }) -join "`n" }

    throw @"
No maven >= $script:MinMavenVersion found. What was found:
$detail

RuoYi's pom pins maven-compiler-plugin 3.13.0, which requires $script:MinMavenVersion or newer.
Download the binary zip from https://maven.apache.org/download.cgi, unzip it, and pass the
folder that directly contains bin\mvn.cmd:
  .\scripts\compare.ps1 -JavaHome '...' -MavenHome 'C:\Work\apache-maven-3.9.9'
Or build the jar yourself and rerun with -SkipBuild.
"@
}

function Test-Ready($port) {
    try {
        # /captchaImage is anonymous and exists in both implementations.
        # Do NOT use /health here - the Java version has no such endpoint.
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/captchaImage" -UseBasicParsing -TimeoutSec 3
        return ($r.StatusCode -eq 200)
    }
    catch { return $false }
}

function Wait-Ready($port, $name, $seconds) {
    for ($i = 0; $i -lt ($seconds * 2); $i++) {
        if (Test-Ready $port) { return $true }
        Start-Sleep -Milliseconds 500
    }
    return $false
}

function Stop-Tree($process) {
    if ($null -eq $process) { return }
    try { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue } catch { }
    Start-Sleep -Seconds 1
}

function Get-MemoryMB($process) {
    try {
        $p = Get-Process -Id $process.Id -ErrorAction Stop
        return [math]::Round($p.WorkingSet64 / 1MB, 1)
    }
    catch { return 0 }
}

$previousEncoding = [Console]::OutputEncoding
Push-Location $root
$goProcess = $null
$javaProcess = $null

try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    if (-not (Test-Path $resultsDir)) { New-Item -ItemType Directory -Path $resultsDir | Out-Null }

    $ds = Get-DataSource
    Write-Host "Database : $($ds.Database) @ $($ds.HostPort)  (from $($ds.Config))" -ForegroundColor Green
    Write-Host "Both implementations will hit this same database." -ForegroundColor Green

    # Fail fast on the JDK version rather than after a ten minute build
    $jdk = Resolve-Java
    Write-Host "JDK      : $($jdk.Exe)  (major $($jdk.Major))" -ForegroundColor Green

    # ---------- build ----------
    Write-Step "Building"
    $binDir = Join-Path $root 'bin'
    if (-not (Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir | Out-Null }
    $goExe = Join-Path $binDir 'server.exe'
    go build -o $goExe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    Write-Host "Go server built." -ForegroundColor Green

    if (-not $SkipBuild) {
        $mvnExe = Resolve-Maven
        Write-Host "Building the Java jar (this creates target/ folders under RuoYi-Vue-master)..." -ForegroundColor Yellow
        Write-Host "First build downloads the whole Spring Boot 4 dependency tree - expect several minutes." -ForegroundColor Yellow

        # Maven picks its JDK from JAVA_HOME, not from PATH. Override it just for
        # this call so the user's system settings stay untouched.
        $savedJavaHome = $env:JAVA_HOME
        if ($JavaHome -ne '') { $env:JAVA_HOME = $JavaHome }

        Push-Location $javaRoot
        try {
            & $mvnExe clean package -DskipTests
            if ($LASTEXITCODE -ne 0) { throw "mvn package failed" }
        }
        finally {
            Pop-Location
            $env:JAVA_HOME = $savedJavaHome
        }
    }
    if (-not (Test-Path $javaJar)) { throw "jar not found: $javaJar" }
    Write-Host "Java jar: $javaJar" -ForegroundColor Green

    # ---------- Go ----------
    Write-Step "Go: starting on port $GoPort"
    $goLog = Join-Path $resultsDir 'compare-go-server.log'
    $env:SERVER_PORT = "$GoPort"
    $goProcess = Start-Process -FilePath $goExe -WorkingDirectory $root -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $goLog -RedirectStandardError "$goLog.err"
    if (-not (Wait-Ready $GoPort 'Go' 30)) { throw "Go server did not start - see $goLog" }

    $goIdleMem = Get-MemoryMB $goProcess
    Write-Host "Go ready (pid $($goProcess.Id)), idle memory ${goIdleMem} MB" -ForegroundColor Green

    $goBenchLog = Join-Path $resultsDir 'compare-go-bench.log'
    $goOutput = & go run ./cmd/perfbench -url "http://127.0.0.1:$GoPort" -c $Concurrency -n $Requests -warmup $Warmup -cross 2>&1 | Out-String
    $goLoadMem = Get-MemoryMB $goProcess
    Write-Host $goOutput
    @("impl    : Go", "idleMB  : $goIdleMem", "loadMB  : $goLoadMem", "") + $goOutput | Set-Content -Path $goBenchLog -Encoding UTF8

    Write-Host "Go memory after load: ${goLoadMem} MB" -ForegroundColor Green
    Stop-Tree $goProcess
    $goProcess = $null

    # ---------- Java ----------
    Write-Step "Java: starting on port $JavaPort"
    $javaLog = Join-Path $resultsDir 'compare-java-server.log'
    $jdbcUrl = "jdbc:mysql://$($ds.HostPort)/$($ds.Database)?useUnicode=true&characterEncoding=utf8&zeroDateTimeBehavior=convertToNull&useSSL=false&serverTimezone=GMT%2B8"

    # Everything is a command line override - RuoYi-Vue-master stays untouched
    $javaArgs = @(
        $JavaOpts.Split(' ')
        '-jar', $javaJar,
        "--server.port=$JavaPort",
        "--spring.datasource.druid.master.url=$jdbcUrl",
        "--spring.datasource.druid.master.username=$($ds.User)",
        "--spring.datasource.druid.master.password=$($ds.Password)"
    ) | Where-Object { $_ -ne '' }

    $javaProcess = Start-Process -FilePath $jdk.Exe -ArgumentList $javaArgs -WorkingDirectory $javaRoot -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $javaLog -RedirectStandardError "$javaLog.err"
    if (-not (Wait-Ready $JavaPort 'Java' 120)) { throw "Java server did not start - see $javaLog" }

    $javaIdleMem = Get-MemoryMB $javaProcess
    Write-Host "Java ready (pid $($javaProcess.Id)), idle memory ${javaIdleMem} MB" -ForegroundColor Green

    $javaBenchLog = Join-Path $resultsDir 'compare-java-bench.log'
    $javaOutput = & go run ./cmd/perfbench -url "http://127.0.0.1:$JavaPort" -c $Concurrency -n $Requests -warmup $Warmup -cross 2>&1 | Out-String
    $javaLoadMem = Get-MemoryMB $javaProcess
    Write-Host $javaOutput
    @("impl    : Java (JVM opts: $JavaOpts)", "idleMB  : $javaIdleMem", "loadMB  : $javaLoadMem", "") + $javaOutput | Set-Content -Path $javaBenchLog -Encoding UTF8

    Write-Host "Java memory after load: ${javaLoadMem} MB" -ForegroundColor Green
    Stop-Tree $javaProcess
    $javaProcess = $null

    # ---------- summary ----------
    Write-Step "Done"
    $summary = Join-Path $resultsDir 'compare-summary.log'
    @(
        "time        : $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')",
        "concurrency : $Concurrency",
        "requests    : $Requests per endpoint (after $Warmup warmup requests)",
        "database    : $($ds.Database) @ $($ds.HostPort) - identical for both",
        "jvm opts    : $JavaOpts",
        "",
        "memory (process working set, MB)",
        "  Go   idle $goIdleMem   under load $goLoadMem",
        "  Java idle $javaIdleMem   under load $javaLoadMem",
        "",
        "throughput: see compare-go-bench.log and compare-java-bench.log"
    ) | Set-Content -Path $summary -Encoding UTF8

    Get-Content $summary | Write-Host
    Write-Host ""
    Write-Host "Logs:" -ForegroundColor Green
    Write-Host "  $goBenchLog"
    Write-Host "  $javaBenchLog"
    Write-Host "  $summary"
}
finally {
    Stop-Tree $goProcess
    Stop-Tree $javaProcess
    Remove-Item Env:\SERVER_PORT -ErrorAction SilentlyContinue
    [Console]::OutputEncoding = $previousEncoding
    Pop-Location
}
