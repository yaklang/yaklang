# Setup llvm-mingw for Windows on ARM64 CGO builds.
# Standard MinGW/gcc on Windows cannot assemble runtime/cgo's ARM64 assembly;
# llvm-mingw provides aarch64-w64-mingw32-clang (see agentsview, nginx-ui, go-cross/cgo-actions).
$ErrorActionPreference = 'Stop'

$LLVM_MINGW_VERSION = '20260602'
$LLVM_MINGW_NAME = "llvm-mingw-$LLVM_MINGW_VERSION-ucrt-aarch64"
$LLVM_MINGW_SHA256 = 'cb5c20fbe1808e31ada5cbe4efd9daa2fee19c91dac6ec5ca1ac46a9c7247e37'

$zip = Join-Path $env:RUNNER_TEMP 'llvm-mingw.zip'
$url = "https://github.com/mstorsjo/llvm-mingw/releases/download/$LLVM_MINGW_VERSION/$LLVM_MINGW_NAME.zip"

Write-Host "Downloading llvm-mingw from $url"
Invoke-WebRequest -Uri $url -OutFile $zip

$actual = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLowerInvariant()
$expected = $LLVM_MINGW_SHA256.ToLowerInvariant()
if ($actual -ne $expected) {
    throw "Checksum mismatch for llvm-mingw`n expected $expected`n actual   $actual"
}

$extractRoot = Join-Path $env:RUNNER_TEMP 'llvm-mingw'
if (Test-Path $extractRoot) {
    Remove-Item -Recurse -Force $extractRoot
}
New-Item -ItemType Directory -Path $extractRoot | Out-Null

if (Get-Command 7z -ErrorAction SilentlyContinue) {
    7z x $zip -o"$extractRoot" | Out-Null
} else {
    Expand-Archive -Path $zip -DestinationPath $extractRoot -Force
}

$bin = Join-Path $extractRoot "$LLVM_MINGW_NAME\bin"
$cc = Join-Path $bin 'aarch64-w64-mingw32-clang.exe'
$cxx = Join-Path $bin 'aarch64-w64-mingw32-clang++.exe'

if (-not (Test-Path $cc)) {
    throw "CC not found at $cc"
}

& $cc --version
Write-Host "Using CC=$cc"

Add-Content -Path $env:GITHUB_PATH -Value $bin
Add-Content -Path $env:GITHUB_ENV -Value "CC=$cc"
Add-Content -Path $env:GITHUB_ENV -Value "CXX=$cxx"
