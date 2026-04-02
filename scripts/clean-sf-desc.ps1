# Called by clean-sf-desc.bat. UTF-8, no BOM.
#
# Requirements (per your request):
# 1) Clear metadata fields: title / title_zh / risk / rule_id / message -> ""
# 2) Clear heredoc bodies for tokens:
#    <<<DESC ... DESC
#    <<<REFERENCE ... REFERENCE
#    <<<SOLUTION ... SOLUTION
#    <<<UNSAFE ... UNSAFE
#    <<<SAFE ... SAFE
# 3) Replace literal "xxxxx" -> ""
# 4) Normalize rare inline style: <<<TOKEN TOKEN  (same line) -> multiline heredoc

$ErrorActionPreference = 'Stop'

$root = $args[0]
if ($null -ne $root) {
    # cmd/bat 可能把外层引号作为字符传入，去掉以保证 Test-Path 能命中
    $root = $root.Trim('"')
}
if (-not $root) {
    $root = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if (-not $root) { $root = (Get-Location).Path }

$files = @()
if (Test-Path -LiteralPath $root -PathType Leaf) {
    $files = @(Get-Item -LiteralPath $root)
}
else {
    $files = @(Get-ChildItem -LiteralPath $root -Filter '*.sf' -File -ErrorAction SilentlyContinue)
}

if ($files.Count -eq 0) {
    Write-Error 'No .sf files found.'
    exit 1
}

$utf8 = [System.Text.UTF8Encoding]::new($false)
$tokens = @('DESC', 'REFERENCE', 'SOLUTION', 'UNSAFE', 'SAFE')

foreach ($f in $files) {
    $raw = [System.IO.File]::ReadAllText($f.FullName, $utf8)

    # 1) placeholder literal
    $raw = $raw.Replace('"xxxxx"', '""')

    # 2) clear metadata fields (all occurrences)
    foreach ($k in @('title', 'title_zh', 'risk', 'rule_id', 'message')) {
        # Replace: title: "xxx" -> title: ""
        # Avoid anchoring the end of the line; be resilient to trailing \r / spaces.
        $pattern = '(?m)^([ \t]*' + $k + '[ \t]*:[ \t]*)"[^"]*"'
        $raw = [System.Text.RegularExpressions.Regex]::Replace($raw, $pattern, '$1""')
    }

    # 3) clear heredoc bodies (keep start/end markers)
    foreach ($t in $tokens) {
        $pattern = "(?s)(<<<$t[ \t]*\r?\n)[\s\S]*?\r?\n([ \t]*$t[ \t]*\r?\n)"
        # Keep one blank line between markers:
        #   <<<TOKEN
        #
        #   TOKEN
        $raw = [System.Text.RegularExpressions.Regex]::Replace($raw, $pattern, '${1}' + [Environment]::NewLine + '${2}')
    }

    # 4) normalize rare inline style: <<<TOKEN TOKEN
    $raw = [System.Text.RegularExpressions.Regex]::Replace(
        $raw,
        '(?m)<<<(DESC|REFERENCE|SOLUTION|UNSAFE|SAFE)[ \t]+\1',
        '<<<$1' + "`r`n" + '$1'
    )

    [System.IO.File]::WriteAllText($f.FullName, $raw, $utf8)
    Write-Host ("OK: {0}" -f $f.FullName)
}

