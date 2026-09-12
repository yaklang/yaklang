param([Parameter(Mandatory=$true)][string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Use a fresh output directory for every run.' }
$fixtureDir = Join-Path $OutputDirectory 'fixture'
New-Item -ItemType Directory -Path $fixtureDir -Force | Out-Null
$files = @{
    'index.json' = @{ environment='production'; primary='routes/checkout.json'; registry='registry.json'; defaults='defaults.json'; migrations='migrations.json' }
    'defaults.json' = @{ timeout=30; retries=2 }
    'registry.json' = @{ routes=@(@{ name='invoice'; enabled=$true; file='routes/invoice.json' }, @{ name='export'; enabled=$true; file='routes/export.json' }, @{ name='retired'; enabled=$false; file='routes/retired.json' }) }
    'routes/checkout.json' = @{ name='checkout'; enabled=$true; timeout=45; proof='CHECK-741' }
    'routes/invoice.json' = @{ name='invoice'; enabled=$true; retries=4; override='regional/invoice.json' }
    'regional/invoice.json' = @{ timeout=75; proof='BILL-852' }
    'routes/export.json' = @{ name='export'; enabled=$true; override='legacy/export-runtime.json' }
    'migrations.json' = @{ 'legacy/export-runtime.json'='runtime/export.json' }
    'runtime/export.json' = @{ timeout=90; retries=1; proof='EXPORT-963'; include='runtime/export-final.json' }
    'runtime/export-final.json' = @{ timeout=120; proof='EXPORT-FINAL-174' }
    'routes/retired.json' = @{ name='retired'; enabled=$false; reference='unrelated/archive.json' }
    'unrelated/archive.json' = @{ note='Historical, disabled route. Its contents do not affect production.' }
}
foreach ($entry in $files.GetEnumerator()) {
    $target = Join-Path $fixtureDir $entry.Key
    New-Item -ItemType Directory -Path (Split-Path $target) -Force | Out-Null
    $entry.Value | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $target -Encoding utf8
}
$unixFixture = $fixtureDir.Replace('\','/')
$reportPath = (Join-Path $OutputDirectory 'result.json').Replace('\','/')
@"
审计本地生产发布配置，从 $unixFixture/index.json 开始，确认所有启用 route 的最终生效 timeout、retries 和 proof，并写入 $reportPath 。
路径均相对 fixture 根目录。defaults 提供初始值，route 覆盖默认值，override 和 include 指向后续覆盖文件（后读覆盖前读），migrations 记录迁移后的路径。只依据实际文件，不能猜默认值或把缺失文件当成配置不存在。
结果格式为 {"routes":[{"name":"...","timeout":0,"retries":0,"proof":"...","sources":["相对路径"]}],"excluded":["禁用 route 名"]}。核对结果文件可读取且所有启用路由都有原始文件依据后结束。
"@ | Set-Content -LiteralPath (Join-Path $OutputDirectory 'prompt.txt') -Encoding utf8
@{
    routes=@(
        @{name='checkout';timeout=45;retries=2;proof='CHECK-741'},
        @{name='invoice';timeout=75;retries=4;proof='BILL-852'},
        @{name='export';timeout=120;retries=1;proof='EXPORT-FINAL-174'}
    )
    excluded=@('retired')
} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'expected.json') -Encoding utf8
