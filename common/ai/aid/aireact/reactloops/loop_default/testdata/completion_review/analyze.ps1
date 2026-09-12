param([Parameter(Mandatory=$true)][string]$RunDirectory)
$ErrorActionPreference = 'Stop'
$raw = Get-Content -Raw -LiteralPath (Join-Path $RunDirectory 'run.log')
$events = @([regex]::Matches($raw, '(?ms)^LAB_EVENT (\{.*?^\})') | ForEach-Object {
    try { $_.Groups[1].Value | ConvertFrom-Json } catch { }
})
$timeline = @()
$todoUpdates = @()
foreach ($event in $events) {
    if (!$event.Content) { continue }
    try { $content = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($event.Content)) | ConvertFrom-Json } catch { continue }
    if ($event.NodeId -eq 'timeline_item') { $timeline += $content }
    if ($content.PSObject.Properties['applied_delta'] -and $content.PSObject.Properties['closed_todos']) { $todoUpdates += $content }
}
$lastTodos = $todoUpdates | Select-Object -Last 1
$summary = [ordered]@{
    finished = $raw.Contains('LAB_FINISHED')
    seeded_decisions = [regex]::Matches($raw, 'LAB_SEEDED_DECISION [0-9]+').Count
    primary_decisions = [regex]::Matches($raw, 'start to call aicommon.CallAITransaction in ReActLoop\[default\]').Count
    completion_checkpoints = @($timeline | Where-Object raw_text -Match '^\[\[COMPLETION_REVIEW_REQUIRED\]\]').Count
    accepted_finishes = @($timeline | Where-Object raw_text -Match '^\[finish\]').Count
    closed_todos = @($lastTodos.closed_todos | Where-Object { $_ })
    open_todos = @($lastTodos.open_todos | Where-Object { $_ })
    observed_models = @($events | Where-Object AIModelName | Select-Object -ExpandProperty AIModelName -Unique)
}
if (Test-Path -LiteralPath (Join-Path $RunDirectory 'result.json')) {
    $result = Get-Content -Raw -LiteralPath (Join-Path $RunDirectory 'result.json') | ConvertFrom-Json
    $summary.route_names = @($result.routes.name)
}
$summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $RunDirectory 'trace-summary.json') -Encoding utf8
$timeline | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $RunDirectory 'timeline.json') -Encoding utf8
$summary | ConvertTo-Json -Depth 12
