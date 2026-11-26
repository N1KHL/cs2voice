$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $outDir = Join-Path $root 'build\web'
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null

    $env:GOOS = 'js'
    $env:GOARCH = 'wasm'

    Write-Host 'Running go vet (js/wasm)...'
    & go vet ./cmd/web ./internal/demo ./internal/rules ./internal/audio ./internal/codec ./internal/report
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Write-Host 'Building build/web/main.wasm...'
    & go build -trimpath -ldflags '-s -w' -o (Join-Path $outDir 'main.wasm') ./cmd/web
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $goroot = (& go env GOROOT).Trim()
    $wasmExec = Join-Path $goroot 'lib\wasm\wasm_exec.js'
    if (-not (Test-Path $wasmExec)) {
        throw "wasm_exec.js not found at $wasmExec"
    }
    Copy-Item -Force $wasmExec (Join-Path $outDir 'wasm_exec.js')

    Write-Host 'Copying frontend assets...'
    Copy-Item -Force -Recurse (Join-Path $root 'frontend\*') $outDir

    Write-Host 'Generating licenses.txt...'
    $sb = [System.Text.StringBuilder]::new()
    $addFile = {
        param([string]$relPath)
        $full = Join-Path $root $relPath
        $norm = $relPath -replace '\\', '/'
        [void]$sb.Append("==================== $norm ====================`n`n")
        [void]$sb.Append([System.IO.File]::ReadAllText($full, [System.Text.Encoding]::UTF8))
        [void]$sb.Append("`n`n")
    }
    & $addFile 'LICENSE'
    & $addFile 'THIRD_PARTY_NOTICES.md'
    $licenseFiles = Get-ChildItem (Join-Path $root 'licenses\*.txt') | Sort-Object Name
    foreach ($lf in $licenseFiles) {
        & $addFile ("licenses/" + $lf.Name)
    }
    $utf8NoBom = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText((Join-Path $outDir 'licenses.txt'), $sb.ToString(), $utf8NoBom)

    Write-Host 'Web build complete: build/web/'
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}
