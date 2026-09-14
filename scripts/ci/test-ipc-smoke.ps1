$ErrorActionPreference = 'Stop'
$env:CGO_ENABLED = '0'
Push-Location (Join-Path $PSScriptRoot '../..')
try {
  # Build only the small endpoint test package. No full engine, databases,
  # downloaded engine binaries, TCP listeners or external services are needed.
  $started = [DateTime]::UtcNow
  go test -mod=readonly -count=1 -timeout=45s -v -run '^TestIPCSmoke$' ./common/utils/engineendpoint
  if ($LASTEXITCODE -ne 0) { throw "IPC smoke tests failed (exit $LASTEXITCODE)" }
  Write-Output ('IPC smoke including compilation: {0:N1}s' -f ([DateTime]::UtcNow - $started).TotalSeconds)
} finally {
  Pop-Location
}
