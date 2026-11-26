$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    Write-Host '1/5 Checking gofmt...'
    $unformatted = & gofmt -l .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    if ($unformatted) {
        Write-Error ("gofmt reported unformatted files:`n" + ($unformatted -join "`n"))
        exit 1
    }

    Write-Host '2/5 Running go test ./...'
    & go test ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Write-Host '3/5 Running js/wasm go vet...'
    $env:GOOS = 'js'
    $env:GOARCH = 'wasm'
    try {
        & go vet ./cmd/web ./internal/demo ./internal/rules ./internal/audio ./internal/codec ./internal/report
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    }

    Write-Host '4/5 Checking JS syntax (node --check)...'
    $jsFiles = @(Get-ChildItem (Join-Path $root 'frontend\*.js') | Sort-Object Name)
    foreach ($f in $jsFiles) {
        & node --check $f.FullName
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    Write-Host '5/5 Running JS test suite (node --test)...'
    $testFiles = @(Get-ChildItem (Join-Path $root 'tests\*.test.mjs') | Sort-Object Name)
    if ($testFiles.Count -eq 0) {
        throw 'No test files found in tests\*.test.mjs'
    }
    $testPaths = @($testFiles | ForEach-Object { $_.FullName })
    & node --test @testPaths
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Write-Host 'All checks passed.'
} finally {
    Pop-Location
}
