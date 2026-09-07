# 严格解析合同与旧验收材料对齐

2026-09-07。本轮修正四组旧验收不一致，完整 bin-parser 回归只剩 `TestP0RoadmapCovered` 的四个目录状态错误。没有修改协议解码器、原始捕获、manifest、P0 门槛或目录支持状态；没有扩展 [14 项延期范围](../../ACTIVE_SCOPE.md)。

## 改动与证据

| 验收项 | 本轮修正 | 保留的约束 |
| --- | --- | --- |
| RMI 分支测试 | 使用有界 `RMIHeader`，SingleOp Ping 和完整 Call；断言消息类型、序列化版本、signed operation、method hash 与 String 参数 | 两个旧向量原样保留为负例：StreamProtocol 头不能夹带握手前消息；只有序列化头不构成完整 Call。原有 legacy 七字节读取边界测试不变 |
| RMI 评分 | Schema 20 → 0，总分 95/A → 75/B；JNDI、RMI、JMX 评分别名同步 | 沿用 YAML-only 静态上限，不添加协议名豁免。新增检查锁定原生规则及文档的保守分数；不声称完整 JNDI/JMX 语义 |
| WinRM 汇总测试 | 正例改用既有 `gen-winrm-identify-valid`，除 HTTP Method/Path 外检查 Content Length、Identify 及其 namespace | `pr5023-gen-winrm-http` 和 `gen-winrm-http-valid` 均继续作为负例，所有记录仍检查；正例继续逐短前缀拒绝，伴随样本必须与既有拒绝合同一致 |
| GSS-API 评分与 corpus 边界 | 评分保持旧 `spnego.yaml` 子集 80/B，另将 corpus 验收对齐到既有严格 `gssapi.yaml/GSSAPIHTTP` | 原始缺少 NegotiationToken 的消息仍拒绝；完整 token 正例与 tokenless 401 challenge 分开，后者不声称已解码 token 或验证协商结果 |

实现入口和原始字段测试见 [RMI](../../protocol_corpus_rmi_test.go)、[WinRM](../../protocol_corpus_winrm_test.go)、[GSS-API](../../protocol_corpus_gssapi_test.go)。本轮只是把旧汇总验收与这些已存在的严格路径对齐，没有让无效消息成为成功。RMI 的 SingleOp/Stream 区分及 CallData 结构参照 [Oracle RMI §10.2–10.3](https://docs.oracle.com/en/java/javase/17/docs/specs/rmi/protocol.html)；tokenless 初始 challenge 与协商 token 的区别参照 [RFC 4559 §4.1](https://www.rfc-editor.org/rfc/rfc4559.html#section-4.1)。

## 验证结果

在仓库根目录、Go 1.22.12 / darwin-arm64 执行，工作树基线仍为 `wip/optimize-protocol-parse` / `0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7` 加未提交实现。测试耗时是单次正确性检查，不能用于性能比较。

定向命令：

```sh
go test ./common/bin-parser -run '^(TestP1BranchRows|TestP1ScorecardsCovered|TestP1NativeSchemaRemainsUncredited|TestProtocolCorpusPR5023CorrectedCompanionsEveryRecordAndBoundary|TestSPNEGOScoresMatchCurrentRuleScope|TestProtocolCorpusRMI.*|TestProtocolCorpusWinRM.*|TestProtocolCorpusGSSAPI.*)$' -count=1 -timeout=3m
```

通过，包耗时 2.929 s。首次执行因新负例预期错误文本不准确而失败（3.549 s）：精确有界入口实际返回 `byte 12: truncated Content Type`，旧 legacy 入口返回的是 `byte 7: truncated Type`；只修正新负例的诊断断言，未修改解析行为。随后补充的文档分数锁定也纳入以下完整回归。

完整命令及无工具截断的输出见 [library-regression.txt](library-regression.txt)：

```sh
go test ./common/bin-parser/... -count=1 -timeout=5m
```

- 退出码 1，主包 193.302 s。唯一顶层失败为 `TestP0RoadmapCovered`，列出 LDAP、MySQL、PostgreSQL、SMB3 四项 `partial`。
- `parser` 1.761 s、`parser/base` 0.759 s、`parser/stream_parser` 5.442 s、`protocol-impl` 2.069 s，均通过。
- `protocol-impl/msrdp` 为 `[no tests to run]`；`internal/corpusutil`、`parser/ser_parser`、`rules`、`utils` 无测试文件。
- 没有运行整个 Yaklang 仓库全量。日志中的样例输出与预期错误路径栈保留，不将调试栈另算成失败测试。

相比 [前轮失败对照](../config-store-20260907/failure-comparison.txt)，RMI 分支、RMI 评分、WinRM 汇总与 GSS-API 旧合同四组失败已消除；仍不能宣称完整套件全绿。

完整回归结束后另执行 [定向 race 检查](targeted-race.txt)，退出码 0，主包 27.105 s：包括本轮四组修正、RMI/WinRM/GSS-API 全部专门检查、Memcached/Cassandra 字段与错误边界，以及六组字段清单与变异守卫。Apple linker 的 `LC_DYSYMTAB` 警告原样保留；没有执行全库 race。

## 性能与剩余验收边界

本轮没有修改小配置存储、公开解析后端、样本或基准程序，因而没有重复性能计时。重新执行前轮 `summarize.py` 成功，其输出与已归档 `summary.json` 逐结构一致：18 组吞吐结果、9 组比较、12 轮回放及新旧导出摘要均保留。[已测吞吐与方法](../config-store-20260907/README.md) 继续适用原冻结程序；不能将本轮正确性测试耗时当作新的加速比。

[当前源码摘要](source-identity.txt) 记录验收源码、未改动的目录/路线图/P0 检查，以及 manifest 和优化后端身份；后两者与前轮归档一致。`go.mod`、`go.sum` 对 HEAD 无差异，`git diff --check -- common/bin-parser` 通过。测试源码摘要用于定位本轮工作树，不声称仅 checkout HEAD 能复现这些未提交内容。

剩余 P0 冲突涉及整套历史验收口径：当前样本字段检查通过，不等于所有 PDU 或会话语义都完整；目录 `partial` 不能仅为通过门槛改成 `new` 或 `stable`。本轮未修改路线图、目录、G8、失败阈值或添加豁免。若继续补齐更多协议语义，会超出当前“Memcached/Cassandra 后转入评估”的执行范围；若改为按当前样本字段范围验收，也需要明确的范围决策，不能默默替换原门槛。

性能评估、持续无损容量验证、客户端交互及 AI 模型评估是独立结论。有限队列仍有丢弃，真实客户端与模型正确率仍未测；现有 aid 文本工具尚未接入本批显式字段 profile。没有借本轮验收修正扩大到新的客户端/aid 实现，整体 goal 未标记完成。
