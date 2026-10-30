# M2 协议正确性验收清单

基线为 M1 最终全绿提交 `39729081a3300381154d96cf1b4866f23976260b`；
[验收记录与 PR/CI 链接](shark-m1-acceptance-397.md)。新分支为
`codex/shark-m2-protocol-correctness`。此表按可验收工作项计数，不按协议数量、
代码量或测试数量估算完成度。M1 尚未合并时，M2 PR 以 M1 分支为基线，避免重复展示 M1。

当前状态：**26 项，已完成 17，进行中 6，未开始 3；剩余 9 项**。
完成项为 M2-01/02/03、12..24 和 26 的有限合同，不代表整个 SMB profile 验收完成。
最终本地选定矩阵合并为 **17917 pass /1 历史 live skip /0 fail**，不重复计数；
canonical/pcap 包保留原通过输入，CLI 复跑保留全部 175 个旧测试 ID，另增加 3 个。
首轮真实 CLI 失败仍保留，不用后来的通过改写历史。原停放 22 个 Go 文件已恢复。
M2-02/03/22 的逐包证据来自 e7 构建；后续修改的完整交付仍须新构建大捕获与精确 SHA CI。
新增问题单列，不进入 M1。以下 AI 时间是继续执行所需的粗估区间，
不含 CI 排队、下载及新发现问题；与宽范围条目重叠的实现时间不重复相加。

| ID | 验收条目 | 状态 | 必需证据 | 剩余 AI 实施粗估 |
|---|---|---|---|---|
| M2-01 | 定位 SMB1→SMB2 丢失记录并冻结独立来源 | 已完成 | 5 个 offer、107 个完整记录/121 命令头；6 个真实三段重组；VLAN81/80 分离，参考关联仅作跨域参考 | 0 |
| M2-02 | 复用 canonical CIFS，保留全部重复方言/OEM/空名称/字节范围 | 已完成（选定请求 profile） | 两显式入口、owned Node 投影、原 YAML/旧 SMB/Carrier 兼容、边界与预算正反例；f613/e7 实际 5 个 offer 的字段、范围和拥有字节验值 | 0 |
| M2-03 | 修复原生探测停止及 SMB1→SMB2 完整记录接续 | 已完成（记录/来源门） | 1/7/64-byte 拆分、合并记录、完整性拒绝、offer≠选定版本、02ff≠最终协商；f613/e7 的 107/121 和 281 全记录来源门通过，完整 body 仍归 M2-04 | 0 |
| M2-04 | 121 命令的 header/body/compound 字段逐一正确 | 进行中（仍有覆盖缺口） | e7 选定原生 header 和部分正文值/范围已验；同一 checker04 对比从 3376 降到 1568 gaps，其中 1521 未发表字段/47 未映射范围；缺项不视为已正确 | 4–8 小时 |
| M2-05 | SMB 乱序/丢失/重传/四元组复用/接口/VLAN 隔离 | 进行中（矩阵与来源分区通过） | 实际 TCP 路径 eager/deferred、多个 workers；12 域双向对应、6 个三段重组、4 次完整重传不重复；全部 pending/dialect/file 隔离、异步 compound 缺前驱/晚到恢复及新 SID/TID 分配仍需验收 | 3–5 小时 |
| M2-06 | SMB Windows 递进页面和字段来源 | 进行中 | 实际 e7 页确认排序问题，导航失败保留；新版 Header/Body/compound/worker 矩阵已过，实际窗口/高亮、来源切换/缩放/编码待复验 | 30–60 分钟 |
| M2-07 | 大捕获 SDK/CLI/flow 与隔离 Wireshark 逐包闭合 | 进行中 | 107/121 实际恢复和 3 个 e7 回放闭合；新旧全部对象/行的差异来源账本、新版本字段/会话/Windows 仍需闭合 | 1–3 小时 |
| M2-08 | TLS 高级丢包乱序恢复与复用边界 | 未开始 | 逐方向/版本/密钥代际及缺开头矩阵，真实流量与独立负控；不伪造协商/身份/明文 | 2–4 小时 |
| M2-09 | QUIC 与 HTTP/3 高级恢复边界 | 未开始 | CID/域/连接代际、加密级别和密钥代际、STREAM/QPACK 恢复；实际认证和来源 | 2–4 小时 |
| M2-10 | 剩余协议、字段、会话及 fuzz 差异转为逐项验收 | 未开始 | 从现有差异报告建立稳定问题 ID、预期/来源/测试，新增问题追加，不能隐去未映射项 | 首轮梳理 1–2 小时；修复另估 |
| M2-11 | M2 交付的完整回归与精确 SHA CI | 进行中 | e7 的 41 Essential、Diff/path-case 成功，traffic 暴露 M2-26；本地修复通过后仍须最终 SHA 全绿，不能使用 e7 旧成功代替 | 30–90 分钟实施，执行等待另计 |
| M2-12 | 缓存 dialect 不覆盖当前 NEGOTIATE 的 literal 值 | 已完成（有限合同） | `TestSMB2CurrentNegotiateDialectIsNotCachedContext`：0311→02ff/0210，当前 wire 与独立 Connection Dialect；不把 request offer 当选择 | 0 |
| M2-13 | 显式旧 SMB binding 不被新原生路径绕过 | 已完成（有限合同） | `TestSMBExplicitLegacyBindingRetainsCanonicalFields`：完整 TREE_DISCONNECT，eager/deferred ×1/3 workers，原 Command/MID 和 JSON 保持 | 0 |
| M2-14 | SMB1 status 格式与 NTSTATUS alias 正确 | 已完成（alias 合同） | `TestSMB1AliasContract*`：请求/响应四组合，NTSTATUS 仅响应且 bit 置位；raw Status 与请求格式偏好分开，保留范围 | 0 |
| M2-15 | SMB1 PID 保留两 literal 半部和正确 32-bit 值 | 已完成（alias 合同） | `TestSMB1AliasContract*`：高/低边界、11227788、JSON、范围/缺值/越界、原生分片与拥有值；实际 Windows 页仍归 M2-06 | 0 |
| M2-16 | NEGOTIATE 的失败/残缺正文不污染连接状态 | 已完成（结构/边界合同） | `TestSMB2NegotiateErrorAndTruncationCannotSelectDialect`、`*DeclaredCollectionsAndBuffersAreComplete`、`*ContextAlignmentAndOwnedBounds`：完整列表、成功结构、安全缓冲与上下文边界；错误 ByteCount≠dialect，残缺不消费 pending；未声明认证或全部协商语义完成 | 0 |
| M2-17 | 失败 CREATE 不建立虚假文件上下文 | 已完成（有限合同） | `TestSMB2FailedCreateDoesNotOpenFile`、`*CreateContextBoundsBeforeOpening`：长错误正文、完整 fixed body、上下文 Next/名称/数据边界和预算；错误无 FileId/打开状态 | 0 |
| M2-18 | CLOSE 响应时间字段不作为 literal FileId | 已完成（有限合同） | `TestSMB2CloseResponseTimesAreNotFileID`：真实 60-byte 响应时间/大小，独立 Request File ID；成功关闭、失败保留、未匹配不伪关联；修正历史 24-byte request-shaped response 测试构造器 | 0 |
| M2-19 | related compound 保留 literal SID/TID | 已完成（有限合同） | `TestSMB2RelatedLiteralAndContextHaveSeparateSources`、`*IgnoresConcreteIDsWithoutChangingLiterals`、`*CannotInventIDsOrInheritUnmatchedFile`：全 1/具体 literal 均保留；request effective 用前项，response 不套占位符；首 related 拒绝，不跨 PDU；完整 TCP 域隔离仍归 M2-05 | 0 |
| M2-20 | 未匹配/同向 NEGOTIATE 不选定连接 dialect | 已完成（有限合同） | `TestSMB2UnmatchedOrSameDirectionCannotSelectDialect`：当前 wire 保留、cache 不变、pending 不被同向消耗，随后反向可匹配；坏正文仍归 M2-16 | 0 |
| M2-21 | SMB2 probe 的固定头完整性与探测窗口 | 已完成（固定头合同） | `TestSMB2ProbeRequiresFixedHeaderBeyondGenericWindow`：8/9/32/63/64/67 字节均需更多，68-byte 头才可 admit；StructureSize/NextCommand/transform 头检查，公开 Probe 与 Feed 均在通用窗口裁剪前验证 | 0 |
| M2-22 | SMB2 请求/响应头 union 与完整 literal 来源 | 已完成（literal 头部合同） | `TestSMB2HeaderUnionAndOwnedLiteralRanges`；e7 的 121 头与来源先冻结值/范围逐包比较，top-level header 缺项/差异为 0；signature 仅拥有字节，未声明认证通过 | 0 |
| M2-23 | 异步 CREATE 以匹配请求的域建立文件上下文 | 已完成（有限合同） | `TestSMB2AsyncCreate*`：pending→final AsyncId 精确关联，使用 request TID7，不伪造响应 TreeID；错误/错方向/未知 AsyncId/另一 SID 不开户，同 FileId 的 TID8 独立 | 0 |
| M2-24 | related 文件上下文经过无 FileId 的 READ/WRITE 响应仍接续 | 已完成（顺序完成合同） | `TestSMB2RelatedFileContextSurvivesNoFileIDResponses`：WRITE+CLOSE、CREATE+READ+CLOSE，完整或按序分开的响应；私有请求链身份/ordinal、来源 MID、范围不伪造、预算先计入；失败 CREATE/未匹配/他链不借用，失败 READ 仍保留已用 ID；高级缺前驱晚到归 M2-05 | 0 |
| M2-25 | SMB 页面 Header→Body→上下文、兼容副本消重与范围归属 | 进行中（CLI 矩阵通过） | 12 个 `TestSMB*` 入口：真实 session/compound，原事件不变，typed/值/范围/owner 反例，父 IPC 决策、worker 不重判、不同 PDU 同值独立；新版 Windows 实际页待闭合 | 30–60 分钟 |
| M2-26 | SMB2 前移探测不抢占短 DLMS Wrapper | 已完成（跨协议回归合同） | e7 全 traffic 12 叶/14 fail actions 暴露 `<8` 字节时仅 00 前缀被提前拦截；前移路径现须完整签名，原 `TestDLMSWrapperSealedReplayMatrix/wrapper-short-header` 全 workers/eager/deferred/order 断言和样本原样通过 | 0 |

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
f613 大包第一次独立闭合：3 个实际 SDK/flow/CLI 回放 exit0；盲计划03对 5/5 legacy、
107/107 target records、121 headers 和全部 281 source records 的字节/贡献/完成位置/
捕获域通过。选定字段验收仍失败：**0 已映射值错误，3255 coverage gaps**（2463 字段未发布、
792 范围未映射），不是全部字段正确。复用的完整 TShark 4.6.9 PDML 与输入哈希固定，
此次未重新运行 Wireshark。大包 14 条 related 命令没有全 1 SID/TID，不能把它们描述成
14 次实际覆盖；上述 authored 原生测试才触发该边界。外部大包和完整导出继续不入库。

正文修复的闭合矩阵为 `test-M2-SMB59-03/receipt.json`，日志 SHA256
`9c87aeb09ba23a87ccdeeca87ed4ae59891485200f476eb073e6f3dab2182895`。
59-01 保留公开 Probe 在 64 字节截断造成的真实失败，59-02 的 163 pass 后又补了
related 具体 ID、响应 ID 和未匹配前项文件上下文检查，最终为 167 pass。完整 body、
新版大包字段验收、Windows 页面、fuzz 和最终 M2 SHA CI 仍未关闭。

e7 的同一严格 checker04 对照为 **3376→1568 coverage gaps**，已映射值/范围差异 0；
原 checker03 和它的 3255 缺口结果均保留，不能跨不同适配器直接比较数字。新增 Header/
Body 类型适配仍逐一保留 typed 子字段缺口，不把完整 opaque Data 当成盐值/算法等已解析。
此次又以同一 TShark 二进制、空配置、全捕获解码和 `tcp.reassemble_out_of_order:TRUE`
实际执行：281 个选中包的所有协议树、属性、字段值与范围复现了旧冻结 PDML，仅排除
文档根节点生成时间；此不扩张 Yak 已验字段范围。新参考完整留在仓库外，闭合摘要 SHA256
`932316a4528f380f1b2869e6261fcb942d13438069d4e18da9de66971067018d`。

本地分包闭合 `M2-local-closure60-01/summary.json` SHA256
`4709c4f6ecb005d1b88989f110631bc18e8946a3524c3278a720218427db44c7`；仅接受原矩阵中
已通过且输入未变的 parser/pcap 包，CLI 使用最终复跑。唯一历史 skip 为
`TestTCPWorkersLiveIntegrity`。原 60-01 CLI 的实际失败（单树被错误附加实现来源后缀）保留；
修复命名后 Commands 精确匹配、每命令 Header/范围等原断言不变，另加反例。

新增 `FuzzSMB2StatefulBodySources` 从已建立 pending 的真实原生 session 进入任意正文，
检查范围/拥有字节/预算/释放/无 panic。第一轮 323772 次通过但无覆盖插桩，明确不计成
覆盖引导证据；第二轮带 Go fuzz 插桩实际 **70826 次/27 新覆盖输入，0 fail**，日志 SHA256
`053d7f7d3d2fca32427dceac0495fd5c6f157297df069f12743edd20afab9806`。
两轮都从仓库外 cwd 运行，失败 corpus 和 cache 亦在仓库外；正文/范围 fuzz 不代表全部
协议状态序列或 TLS/QUIC 高级恢复已完成。
