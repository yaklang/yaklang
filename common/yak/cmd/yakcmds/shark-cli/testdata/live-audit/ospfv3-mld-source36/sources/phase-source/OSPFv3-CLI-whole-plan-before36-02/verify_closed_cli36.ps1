param(
    [Parameter(Mandatory = $true)][string]$AfterRoot,
    [Parameter(Mandatory = $true)][string]$OutRoot
)
& 'C:\Users\V\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe' -B -X utf8 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\OSPFv3-CLI-whole-plan-before36-02\frozen\cli_source_plan.py' verify --plan 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\OSPFv3-CLI-whole-plan-before36-02' --before 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\runtime-public-CLI-35' --field-plan 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\OSPFv3-field-plan-before-native-02\frozen02\plan.json' --after $AfterRoot --native-proof 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\OSPFv3-native37-whole-636-01' --native-proof-sha256 '463250f963d5816b1e9b3565fb27ae3e444f74f4cae4455caa183d5727936022' --native-ledger-closure 'C:\tmp\yak-shark-ESP-SSH-UI-20261028\OSPFv3-native-ledger-closure36-01.json' --native-ledger-closure-sha256 'b79a428f0badf6919588ce6b677944444cc4f65257359c7f9c1f809d19f7df12' --source '636816f1da7c97228a6adf11cbc363a41c53c33f' --out $OutRoot
exit $LASTEXITCODE
