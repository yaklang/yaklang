# M2-81：先到 0-RTT 的有界恢复、同数据报响应关联和实际 Windows

本项有限关闭 N09.R03。运行/构建提交为 `6941e45db788a8b5192e9f3f6d4ef13765f2ddd2`，
在唯一 M2 PR #5371 中继续；SMB 不分析、不修改。当前 81 个父项，71 个在明示有限合同完成，
7 个进行中、3 个 SMB 搁置，共 10 个未关闭。非 SMB 17 包中 13 个有限完成、4 个开放。
本批另复现 N09.R05，连同 R04 继续列账；已知后续粗估 18–36 AI 实施小时，等待/新问题另计。

## 问题、行为和边界

M2-79 已补早期 secret 和包号边界，但实抓中第 15 帧的 0-RTT 先于当前 ServerHello/套件。
原生只给出缺密钥诊断，之后不重试；21 个真实受保护包只有 20 个打开。
现在在已观察的当前 capture-domain/CID owner 内，私有保留完整密文和独立拥有的来源元数据。
最多 `min(MaxCollectionElements, 64)` 条，密文、事件及可变来源均计入字节预算，
并纳入会话全局保留预算。没有 provider、owner 不明或已关闭/退休的 owner 不缓存。
已有密钥时明确失败的 tag/非法包不新增 pending；失钥时暂存的密文后来认证失败只发布拒绝，不推进 PN/STREAM。

恢复须观察到当前连接的完整 ClientHello 提议、最终 ServerHello 选中 PSK identity 0，
以及服务端完整 EncryptedExtensions 的空 early_data 扩展。缺证据继续等待；明确拒绝或 TLS HRR
不向 STREAM/HTTP3 交付早期内容，QUIC Retry 与 TLS HRR 分开。
共享 TLS 原生 admission 和公开 EE decoder 都拒绝非空 early_data；私有决定与整批 TLS 消息一起提交，
非法后缀不能留下已接受的前缀决定。这符合 [RFC 9001 §4.6.2](https://www.rfc-editor.org/rfc/rfc9001.html#section-4.6.2)
及 [RFC 8446 §4.2.10](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.2.10) 的有限观察规则。
这不是 binder、Finished、签名、PKI、重放保护或完整握手合规性验证。

后来才提供早期 secret 时只补早期槽，不重装普通应用密钥/epoch。仅 PING/PADDING 的已认证传输观察
保持既有行为；explicit decrypted API 不被改写成服务端接纳证明。
重处理使用新的不可变 PDU，原缺密钥诊断保留；不改写已经发布的事件。
第 15 帧的字节仍贡献且完成于第 15 帧，补齐上下文的第 16 帧另记为 `Reprocessed In Packet`，
并以 `Reprocessed From PDU` 关联原观察，不伪造字节贡献或物理包。

最初实现 4532 已通过完整回归并打开全部 21 包，但仍漏响应关联：第 16 帧包含服务端握手和响应，
等整个数据报结束才恢复请求会晚于响应。源字节不改、只拆开数据报的控制复现了这个差异。
最终在完整握手已经满足条件后、下一个 coalesced 响应处理前恢复请求；预先给所有剩余 wire envelope
保留观察名额，缺预算则延后，不能挤掉实际报文。恢复资源拒绝会保留余下 wire 诊断并退休 owner，
不能用公开 Initial 密钥重新打开；恢复或实际 wire 的 close 都清除后续 pending。

## 来源、逐包和会话验收

所有外部捕获、keylog/私钥、完整导出、PDML、fuzz 和 PNG 都在仓库外
`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`；两个跟踪 fixture ZIP 未变。
实际来源沿用本次工作中新抓的 `QUIC-early-source1490-01/live.pcap`：19 个物理包、
两个真实 full/resumed HTTP/3 连接复用同一对 UDP sockets；两个端点确认恢复和早期接纳，证书验证开启。
捕获 SHA256 `2c7bd3871b18bc3fbe0485bc798b20d396b8e1e2ad119278aa90b6cb3a8f35d0`。
独立来源在 native/reference 之前已打开全部 21 包，核对 CH random、套件、ticket identity、CID、
CRYPTO/STREAM、QPACK、请求/响应头和正文；不把参考标签当身份依据。

| Profile | 最终事实及范围 |
|---|---|
| `QUIC-early-recovery-source1572-01` | 原 PDU21 缺密钥诊断保留；新 PDU25 恢复 HTTP3 请求，来源/完成帧均 15，上下文帧 16；响应 PDU26 的 response_to/transaction 都为 25 |
| `QUIC-early-recovery-final1573-01` | 原始实抓及原 10 个冻结变换：33 次新 native 执行、11 次新完整 two-pass TShark 4.6.9 参考；13 目标含 7 正例/6 PN 顺序拒绝，所有 flow 导出不变 |
| `QUIC-early-recovery-audit1574-01` | 全部 21 个原始受保护包的 AEAD/PN/原始源逐项对上独立来源和新参考，0 未打开来源，353 个 QUIC 字面字段检查 |
| `QUIC-early-control-audit1590-01` | 13 个变换目标独立 AEAD、拥有明文/拒绝和参考 PN 全对，276 个字面检查；Wireshark 对非法跨层 PN 的显示不是语义接纳 oracle |
| `QUIC-H3-recovery1589-01` | 两请求/两响应的 14 个头值、四份正文、8 个 H3 帧的类型/长度/范围/原字节源和新参考全部对上来源；两个响应都关联正确请求 |
| `Windows-QUIC-recovery1587-01` | 新构建实际请求/响应两页，36 个控制台状态；84B 请求 STREAM、292B 响应 STREAM 对独立 AEAD 精确一致，高亮、80×30/180×48 resize、鼠标/Tab、CP936 恢复通过；两张最终 PNG 已人工检查 |

Windows 原始第 15 行确实从 QUIC 更新为 **HTTP/3 → Loopback → IPv4 → UDP → QUIC → HTTP/3**，
能展开 HEADERS/DATA 和 POST/path；第 16 行显示 200/280B DATA，响应有实际请求关联。
JSON 是观察时刻的物理快照：第 15 行首次仍为 QUIC，第 16 行的 `published_pdus` 补充 PDU25，
其 byte_source 明确仍是 15；TUI 通过 history 刷新第 15 行。消费 JSON 必须同时读取这些 publication，
不能把其容器帧当作新贡献帧，也不能把首次快照计数冒充最终重访结果。
原诊断仍计入历史观察；“0 未打开来源”不等于历史 context-required 计数变为零。

## 回归、资源和证据

Windows Go 1.22.12，普通 Go 任务 `GOMAXPROCS=3` / `GOMEMLIMIT=2GiB` / `-p=1`。
SDK events/flows/CLI 三个干净构建均固定同一 6941 源码和所有输入摘要；最终 yak SHA256
`5ff0a2139802c591eb1fcff63720b59365d385bc165a3dfcfeeb05599adc4aa5`。

- 初始 15 个恢复回归先全红后全绿；新 coalescing/预算回归在 4532 明确失败，再通过最终修复。
  v1/v2、三套件、晚到 secret、公开 DTO/源字节修改、接口/VLAN、64 上限、字节/元数据/全局预算、
  坏 tag 后有效包、拒绝/HRR/未提议/错误 PSK、批原子、退休/close/EOF 均有用例。
- 最终完整 `QUIC-early-recovery-full1571-01`：291001 唯一 ID，290976 pass、25 历史 optional skip，0 fail。
  父 290951 个 Windows ID/动作逐项保持，50 新增全 pass；所有旧断言不变。
  stdout SHA256 `de48dacd5d48455c5fe6494ba310354fd3129928f4a32cf1816bbfa339306313`。
  相关 QUIC/HTTP3/QPACK/EE 定向 864 pass/0 skip。
- 显式覆盖插桩 `FuzzQUICEarlyRecoveryOwnership`：60 秒/32642 次/六种子，隔离 corpus/cache/temp；
  1 CPU、768MiB job、最多两个同时活动进程，第三进程拒绝负控；峰值约 525MiB，全部子进程收尾。
- 两个大公共捕获重新完成六份完整导出：379455 物理包、306855 SDK 事件、379926 CLI 行（含 471 publication）；
  六份均逐字节等于冻结父导出，SDK 等价在对应 CLI 前强制核验，不是 protocol-count 近似或豁免。
  四个原 QUIC 接口/VLAN strict 的原 checker/合同全 0 mismatch、0 coverage gap；不是新的全 0-RTT 域矩阵。

最终资格 `QUIC-recovery-qualification1593-01/summary1593.json` SHA256
`1a03168e31dcc5f85d8784d5868e328c53a4e063ba3fdfe41fe6d12db197e37e`。
执行输入、原生输出、完整参考、实际 console JSON/PNG 摘要均复核。早期新测试的短 CID 构造与
空切片断言错误、旧 Windows harness 未合并 publication/混淆方向与 key epoch 的失败保留；
修正测试/工具后完整重跑，未删旧断言。1592 聚合脚本的条件笔误不作为验收，1593 严格重验最终源与证据。

## CI 和未关闭事项

父 `d34493d93721134ef407530dca0ae7abeee8e64d` 已独立全部 CI 全绿：
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41531275106)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41531281689)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41531288802)，47 约定检查名/91 执行/2 statuses；
官方产物的 checkout/size/digest/CRC 及 316025 原始 ID（316000 pass/25 历史 skip）一致，
131084 required 全 pass/0 skip。闭合1568 SHA256
`56bb86155ddd44f80a2d648049284a3ecc0d764506aae4e4abca622a4cc6625c`。
Java/RAG 官方产物另确认 8 个新增核心及此前 3 个失败 ID 各实际一次 pass，闭合1569 SHA256
`ab14dd052b2f2bdbbd35d6308033d5b6cf55ce8e22fddd7cf6fe3f4e493bcc94`。
SMB Page 仅更新两个基线块，70 块顺序/ID 和 68 个任务块逐字/哈希不变，sequence26。
本批最终说明 SHA 必须独立验收 316075 原始 ID（316050 pass/25 历史 skip）、50 新增和全部 CI；
父成功不认证新恢复代码，也不认证本批说明提交。

**N09.R04 仍未关闭**：`delayed-early-after-server-response` 控制仍有迟到 peer SETTINGS 的 QPACK
encoder 上下文等待，不能伪造成功或丢弃原始来源。
**新增 N09.R05**：七个源冻结的有效早期请求变换中，请求在响应已经发布之后才被观察；
头和正文正确，但旧响应的关联仍未知。外部 `QUIC-late-association-gap1591-01/summary1591.json`
SHA256 `de593a57ab98c640f9c8d3b65910d86e675a4a9e4ee73fd85daa2e99ebbcae1f`。
本项只修同数据报内可在响应之前恢复的情况；不以标签或 21/21 AEAD 数量关闭 R04/R05。
完整缺握手身份、CID/epoch/STREAM/QPACK/0RTT 生命周期、N10 六 fuzz 政策差异、N13 TLS 高级阶段、
N14 字段/恢复及 SMB S01–S06 继续保留，不宣称所有核心协议完备或与 Wireshark 全部语义一致。
