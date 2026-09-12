param([Parameter(Mandatory=$true)][string]$RunDirectory)
$ErrorActionPreference = 'Stop'
$expected = Get-Content -Raw -LiteralPath (Join-Path $RunDirectory 'expected.json') | ConvertFrom-Json
$actual = Get-Content -Raw -LiteralPath (Join-Path $RunDirectory 'result.json') | ConvertFrom-Json
$requiredSources = @{
    checkout = @('defaults.json','routes/checkout.json')
    invoice = @('defaults.json','routes/invoice.json','regional/invoice.json')
    export = @('defaults.json','routes/export.json','runtime/export.json','runtime/export-final.json')
}
if ($actual.routes.Count -ne $expected.routes.Count) { throw 'Wrong number of active routes.' }
foreach ($want in $expected.routes) {
    $got = @($actual.routes | Where-Object name -EQ $want.name)
    if ($got.Count -ne 1) { throw "Missing or duplicate route: $($want.name)" }
    foreach ($field in @('timeout','retries','proof')) {
        if ($got[0].$field -ne $want.$field) { throw "Wrong $field for $($want.name)." }
    }
    if (!$got[0].sources -or $got[0].sources.Count -lt 2) { throw "Missing source chain for $($want.name)." }
    foreach ($requiredSource in $requiredSources[$want.name]) {
        if ($requiredSource -notin $got[0].sources) { throw "Missing required source $requiredSource for $($want.name)." }
    }
    foreach ($relativeSource in $got[0].sources) {
        if (!(Test-Path -LiteralPath (Join-Path (Join-Path $RunDirectory 'fixture') $relativeSource) -PathType Leaf)) {
            throw "Invalid source reference: $relativeSource"
        }
    }
}
if (@($actual.excluded).Count -ne 1 -or $actual.excluded[0] -ne 'retired') { throw 'Disabled route was not correctly excluded.' }
Write-Output 'PASS: all three active routes, deep override, migration recovery, proof values and disabled-route boundary.'
