# Test double only: never executes Go, Node, database or network commands.
if ($env:ERP_VERIFY_CALLS) {
    $entry = [pscustomobject]@{ arguments = @($args); testRoot = $env:MT_TEST_ROOT; proxy = $env:GOPROXY; noProxy = $env:GONOPROXY; toolchain = $env:GOTOOLCHAIN }
    $entry | ConvertTo-Json -Compress | Add-Content -LiteralPath $env:ERP_VERIFY_CALLS -Encoding utf8
}
if ($env:ERP_VERIFY_DIRTY -eq '1' -and $args -contains '-l') { Write-Output 'internal/example.go' }
if ($env:ERP_VERIFY_WARNING -eq '1') { [Console]::Error.WriteLine('test warning on stderr') }
if ($env:ERP_VERIFY_FAIL -eq '1' -and $args -contains 'test') { exit 23 }
exit 0
