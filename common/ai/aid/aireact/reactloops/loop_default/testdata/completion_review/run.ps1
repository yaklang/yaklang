param(
    [Parameter(Mandatory=$true)][string]$Binary,
    [Parameter(Mandatory=$true)][string]$RunDirectory,
    [ValidateSet('natural','replay','greeting')][string]$Mode = 'natural'
)
$ErrorActionPreference = 'Stop'
if (!$env:REACT_LAB_API_KEY) { throw 'Set REACT_LAB_API_KEY in the calling process.' }
$binaryPath = (Resolve-Path -LiteralPath $Binary).Path
$runPath = [IO.Path]::GetFullPath($RunDirectory)
if ($Mode -eq 'greeting') {
    if (Test-Path -LiteralPath $runPath) { throw 'Use a fresh run directory.' }
    New-Item -ItemType Directory -Path $runPath | Out-Null
    '你好' | Set-Content -LiteralPath (Join-Path $runPath 'prompt.txt') -Encoding utf8
} else {
    & (Join-Path $PSScriptRoot 'prepare.ps1') -OutputDirectory $runPath
}
$values = @{
    REACT_LAB_PROMPT = Join-Path $runPath 'prompt.txt'
    REACT_LAB_WORKDIR = Join-Path $runPath 'workspace'
    REACT_LAB_FIXTURE = (Join-Path $runPath 'fixture').Replace('\','/')
    REACT_LAB_RESULT = (Join-Path $runPath 'result.json').Replace('\','/')
    YAKIT_HOME = Join-Path $runPath 'home'
}
$previous = @{}
try {
    foreach ($entry in $values.GetEnumerator()) {
        $previous[$entry.Key] = [Environment]::GetEnvironmentVariable($entry.Key, 'Process')
        [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process')
    }
    $scriptName = if ($Mode -eq 'replay') { 'replay.yak' } else { 'run.yak' }
    $code = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot $scriptName)
    & $binaryPath -c $code *> (Join-Path $runPath 'run.log')
    if ($LASTEXITCODE -ne 0) { throw "yak exited with code $LASTEXITCODE; inspect run.log" }
} finally {
    foreach ($entry in $previous.GetEnumerator()) {
        [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process')
    }
}
if ($Mode -ne 'greeting') {
    & (Join-Path $PSScriptRoot 'verify.ps1') -RunDirectory $runPath
}
Write-Output "Completed $Mode run in $runPath"
