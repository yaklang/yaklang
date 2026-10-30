# M2 协议正确性验收清单

基线为 M1 最终全绿提交 `39729081a3300381154d96cf1b4866f23976260b`；
[验收记录与 PR/CI 链接](shark-m1-acceptance-397.md)。新分支为
`codex/shark-m2-protocol-correctness`。此表按可验收工作项计数，不按协议数量、
代码量或测试数量估算完成度。M1 尚未合并时，M2 PR 以 M1 分支为基线，避免重复展示 M1。

当前状态：**22 项，已完成 6，进行中 3，未开始 13；剩余 16 项**。
完成项为来源定位 M2-01 和五个有限修复合同 M2-12..15、20，不代表整个 SMB profile
验收完成。首批 canonical/native/session/展示矩阵 **80 pass /0 skip /0 fail**，
原停放 22 个 Go 文件已恢复并纳入测试。M2-02/03/05 仍需大捕获与域隔离证据，不由
单元通过提前关闭。新增问题单列，不进入 M1。以下 AI 时间是继续执行所需的粗估区间，
不含 CI 排队、下载及新发现问题；与宽范围条目重叠的实现时间不重复相加。

| ID | 验收条目 | 状态 | 必需证据 | 剩余 AI 实施粗估 |
|---|---|---|---|---|
| M2-01 | 定位 SMB1→SMB2 丢失记录并冻结独立来源 | 已完成 | 5 个 offer、107 个完整记录/121 命令头；6 个真实三段重组；VLAN81/80 分离，参考关联仅作跨域参考 | 0 |
| M2-02 | 复用 canonical CIFS，保留全部重复方言/OEM/空名称/字节范围 | 进行中（已恢复，未运行） | 两显式入口、owned Node 投影、原 YAML/旧 SMB/Carrier 兼容、边界与预算正反例 | 30–90 分钟 |
| M2-03 | 修复原生探测停止及 SMB1→SMB2 完整记录接续 | 进行中（已恢复，未运行） | 1/7/64-byte 拆分、合并记录、完整性拒绝、offer≠选定版本、02ff≠最终协商、原生/API 一致 | 45–120 分钟 |
| M2-04 | 121 命令的 header/body/compound 字段逐一正确 | 未开始 | 来源先冻结的值/位置/边界；原始 IDs 与关联/继承信息分开；未选布局显式保留，不仅补标签 | 2–4 小时 |
| M2-05 | SMB 乱序/丢失/重传/四元组复用/接口/VLAN 隔离 | 进行中（已恢复，未运行） | 实际 TCP 路径 eager/deferred、多个 workers、重传不重复拥有字节；跨域不关联、不继承 dialect | 1–2 小时 |
| M2-06 | SMB Windows 递进页面和字段来源 | 未开始 | 实际新版页面、同值不同位置、原始帧/完整 PDU/opaque 区分、来源切换/缩放/编码 | 45–90 分钟 |
| M2-07 | 大捕获 SDK/CLI/flow 与隔离 Wireshark 逐包闭合 | 未开始 | 107/121 实际恢复；新增 PDU/flow 编号有来源账本；旧整对象与行保持、差异逐条解释 | 1–2 小时 |
| M2-08 | TLS 高级丢包乱序恢复与复用边界 | 未开始 | 逐方向/版本/密钥代际及缺开头矩阵，真实流量与独立负控；不伪造协商/身份/明文 | 2–4 小时 |
| M2-09 | QUIC 与 HTTP/3 高级恢复边界 | 未开始 | CID/域/连接代际、加密级别和密钥代际、STREAM/QPACK 恢复；实际认证和来源 | 2–4 小时 |
| M2-10 | 剩余协议、字段、会话及 fuzz 差异转为逐项验收 | 未开始 | 从现有差异报告建立稳定问题 ID、预期/来源/测试，新增问题追加，不能隐去未映射项 | 首轮梳理 1–2 小时；修复另估 |
| M2-11 | M2 交付的完整回归与精确 SHA CI | 未开始 | 当前修改的独立证据、所需 fuzz、Windows 与大包回放；所有必需/专项 CI 同一 SHA 全绿 | 30–90 分钟实施，执行等待另计 |
| M2-12 | 缓存 dialect 不覆盖当前 NEGOTIATE 的 literal 值 | 已完成（有限合同） | `TestSMB2CurrentNegotiateDialectIsNotCachedContext`：0311→02ff/0210，当前 wire 与独立 Connection Dialect；不把 request offer 当选择 | 0 |
| M2-13 | 显式旧 SMB binding 不被新原生路径绕过 | 已完成（有限合同） | `TestSMBExplicitLegacyBindingRetainsCanonicalFields`：完整 TREE_DISCONNECT，eager/deferred ×1/3 workers，原 Command/MID 和 JSON 保持 | 0 |
| M2-14 | SMB1 status 格式与 NTSTATUS alias 正确 | 已完成（alias 合同） | `TestSMB1AliasContract*`：请求/响应四组合，NTSTATUS 仅响应且 bit 置位；raw Status 与请求格式偏好分开，保留范围 | 0 |
| M2-15 | SMB1 PID 保留两 literal 半部和正确 32-bit 值 | 已完成（alias 合同） | `TestSMB1AliasContract*`：高/低边界、11227788、JSON、范围/缺值/越界、原生分片与拥有值；实际 Windows 页仍归 M2-06 | 0 |
| M2-16 | NEGOTIATE 的失败/残缺正文不污染连接状态 | 未开始 | 完整声明方言列表、成功结构和边界；错误 envelope 的 ByteCount 不充当 dialect；错误/截断正反例 | 45–90 分钟 |
| M2-17 | 失败 CREATE 不建立虚假文件上下文 | 未开始 | 长错误正文和残缺成功不打开文件；合法成功仍能关联 READ；pending/status/预算与方向检查 | 30–60 分钟 |
| M2-18 | CLOSE 响应时间字段不作为 literal FileId | 未开始 | 响应时间不产生 FileId；匹配成功关闭请求 handle、失败不关闭、未匹配不伪关联 | 30–60 分钟 |
| M2-19 | related compound 保留 literal SID/TID | 未开始 | 全 1 literal 与 effective/context 分开；只用 effective 做关联；首个 related 拒绝，不能跨 PDU/flow/域继承 | 45–90 分钟 |
| M2-20 | 未匹配/同向 NEGOTIATE 不选定连接 dialect | 已完成（有限合同） | `TestSMB2UnmatchedOrSameDirectionCannotSelectDialect`：当前 wire 保留、cache 不变、pending 不被同向消耗，随后反向可匹配；坏正文仍归 M2-16 | 0 |
| M2-21 | SMB2 probe 的固定头完整性与探测窗口 | 未开始 | 当前 8-byte prefix 会在 StructureSize 尚未检查时 accept；核验完整头与 ProbeBytes=64 的交互、误识别/拒绝后的恢复，不能只依赖四字节 magic | 30–90 分钟 |
| M2-22 | SMB2 请求/响应头 union 与完整 literal 来源 | 未开始 | request ChannelSequence/Reserved 不当 Status；Credit、同步/异步 ID、signature/NextCommand 与每个 compound 头范围，逐包对照 | 45–90 分钟 |

后续每次报告列本表已完成/进行中/未开始数量、关闭的 ID、新增 ID 和证据链接。新增问题不能藏入既有“完成”项；估时随范围和实际失败更新。账户剩余额度到 50% 时停止新增范围，提交、推送并确认最新提交 CI 后暂停；最多 1 个子 agent，主 agent 负责关键路径。

原始字段表仍保留 94/39 共 133 个唯一条目：历史分类为 8 个 DNS 显示修复、122 个
TLS 归属解释、1 个参考限制、2 个无损显示差异，未分类 0。此为固定清单的历史分类，
不是 TLS/QUIC 高级恢复或新 HEAD 的完整证书。M2-10 继续保留稳定问题 ID、来源与
最终构建复验；新 SMB 问题不塞回旧清单“归零”。

首批来源冻结结果为仓库外 `test-M2-SMB58-02/receipt.json`；日志 SHA256
`65ec932cb9991f64072d78bd4b34025fdd9bc09c1a67d80c760233460c29602c`。
此前 58-01 的 10 个 fail action（8 个叶/2 个父）保留：新测试错误要求所有分片 Feed
返回 nil，而接口明示返回 `ErrNeedMore`。修正为逐片精确 Consumed、非空错误仅允许
NeedMore 且标志一致、最后片必须完整，原最终字段/范围/拥有值断言全部保留后通过。
当前大包 107/121、Windows 页面、完整 body、fuzz 和最终 M2 SHA CI 尚未验收。
