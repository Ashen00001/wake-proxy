# Runs vet + tests. Exit 0 on success, 1 on failure. Last line is machine-readable.
$ErrorActionPreference = 'Continue'
Set-Location (Join-Path $PSScriptRoot '..')

$go = 'go'
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $cand = Join-Path $env:USERPROFILE 'sdk-go\go\bin\go.exe'
    if (Test-Path $cand) { $go = $cand }
}

& $go vet ./...
if ($LASTEXITCODE -ne 0) {
    Write-Output 'VERIFY wake-proxy FAIL stage=vet'
    exit 1
}

$out = & $go test -v ./... 2>&1 | ForEach-Object { "$_" }
$status = $LASTEXITCODE
$out | Where-Object { $_ -match '^(--- FAIL|FAIL|ok|panic)' }
$passed = @($out | Where-Object { $_ -match '^--- PASS' }).Count

if ($status -ne 0) {
    Write-Output "VERIFY wake-proxy FAIL stage=test tests=$passed"
    exit 1
}
Write-Output "VERIFY wake-proxy PASS tests=$passed"
exit 0
