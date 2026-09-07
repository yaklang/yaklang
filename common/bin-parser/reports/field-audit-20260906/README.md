# 当前字段证据与历史验收状态复核

2026-09-06。本目录记录正确性检查，不是吞吐或模型能力测量。

基线为 `wip/optimize-protocol-parse`，HEAD `0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7` 加当前未提交工作树；仅 checkout 此提交不足以复现。Go 1.22.12、darwin/arm64。manifest SHA-256 为 `870350c644462dffd3462ac8b71adcfff1fe725c19ace6d0d39765b1ab9181ca`；[本次新增检查](../../protocol_corpus_field_inventory_test.go) SHA-256 为 `14f1f2657b3b6b75460e500d40e870fb191d1896b5fa432353c669451e1d1dc7`。

## 样本边界

| 字段检查组 | 独立捕获数 | 原始记录数 |
| --- | ---: | ---: |
| LDAP / CLDAP | 5 | 11 |
| MySQL / MariaDB | 3 | 49 |
| PostgreSQL | 1 | 88 |
| SMB2 / SMB3 | 4 | 19 |
| Memcached | 3 | 18 |
| Cassandra | 1 | 20 |
| 合计 | 17 | 205 |

每组实际调用原有字段断言，LDAP 组另调用 CLDAP 搜索每字段检查。上述记录包含控制包、加密内容和负样本，不能视为全部已解码的应用消息。原始错误样本保留预期拒绝，不通过修写原始捕获改变结论。

清单检查固定样本 ID、路径、摘要、记录数、分类与路线图映射，并检查按名称、dissector 或已固定 ID 可关联的 manifest 记录。变异检查覆盖新增、缺失、重命名、重复及各固定属性变化；未知且未映射的协议仍需独立分类审计。新增样本触发检查失败后，仍需人工审查并扩展实际字段断言；增加一个清单条目本身不等于支持新数据。

## 复现命令与实际结果

在仓库根目录分别执行：

```sh
go test ./common/bin-parser -run '^TestP0RoadmapCovered$' -count=1 -timeout=1m
go test ./common/bin-parser -run '^(TestProtocolCorpusCurrentFieldEvidence|TestCurrentFieldInventoryRejectsDrift|TestProtocolCorpus(LDAPFields|MySQLFields|PostgreSQLFields|SMB3).*)$' -count=1 -v -timeout=3m
go test -race ./common/bin-parser -run '^(TestProtocolCorpusCurrentFieldEvidence|TestCurrentFieldInventoryRejectsDrift)$' -count=1 -timeout=2m
```

- [P0 状态门槛](p0-state-conflict.txt)：退出码 1，包耗时 0.840 s；LDAP、MySQL、PostgreSQL、SMB3 仍为 `partial`。
- [字段、清单变异与边界联合检查](fields-and-boundaries.txt)：退出码 0，包耗时 2.225 s。
- [字段及清单变异 race 检查](field-audit-race.txt)：退出码 0，包耗时 3.927 s；保留 Apple linker 的 `LC_DYSYMTAB` 警告，不把警告描述为检查失败。

各耗时来自一次 Go 包检查，不构成性能比较。没有执行全库完整套件，没有扩大协议实现范围，也没有修改历史门槛、目录支持状态或评分卡。这组通过项不能替代失败的 P0 门槛；指定的 14 项延期仍按 [本次范围](../../ACTIVE_SCOPE.md) 保留。
