# Yak Shark M2 SMB 协议正确性任务书

最新独立全绿检查点为`add08e5072baefceec69533821b25704de58ca1f`，47约定检查/91执行/2状态及
314985原始ID/131084 required闭合；[traffic](https://github.com/yaklang/yaklang/actions/runs/41436592291)。
后续d853仅补[非SMB服务端TLS观察顺序与Windows诊断](shark-m2-tls-server-order-20261102.md)，
新最终head CI另验；main d705已普通合入。SMB S01–S06及历史107/121/字段分母仍须承接者开工独立复核。

最新独立全绿检查点为`a7967c3d0a382338ac0bbe2dc2bb8f38845f203b`：47约定检查/91执行/2状态、
314926原始ID、131084 required全pass/零required skip；[traffic](https://github.com/yaklang/yaklang/actions/runs/41420719964)。
后续388普通合入main d705，cf补[非SMB EE拥有字段及首EOF窗口](shark-m2-ee-fields-and-eof-statistics-20261102.md)，
新最终head CI另验，不替代SMB S01。S01–S06、107/121和字段缺口继续保留；承接者从开工时远端最新M2固定精确HEAD。

最新已独立全绿基线为`912d8b6c8910039730e1f15e31f37a2f06dca3f3`，47约定检查/91执行/2状态、
314824原始ID及131084 required闭合；[traffic](https://github.com/yaklang/yaklang/actions/runs/41409852472)。
后续4c2e仅补[TLS EE资格、复用和实际Windows](shark-m2-tls-encrypted-extensions-20261102.md)，
新最终head另验CI，不代替SMB验收。S01–S06、107/121须开工复核及阶段边界保持；从开工时远端
最新M2固定精确HEAD。主线程继续跳过SMB语义分析。

最新已核验全绿父基线为`220d52898ac06f9951f2d82e53aab46c559d21b8`，47约定检查/91实际执行/2statuses、314350原始ID及131084 required闭合；[traffic](https://github.com/yaklang/yaklang/actions/runs/41393130244)。后续469仅补物理TCP异常flags页面，[证据](shark-m2-tcp-invalid-flags-20261102.md)，其新最终head CI另验，不涉及SMB应用分析。S01–S06及107/121待复核缺口保持，开工固定远端最新M2。

最新已独立全绿父基线为`b603d21e02939b2fd874ecbbc93828c735008b31`，47约定检查/91实际执行/2statuses、314520原始ID及131084 required已闭合；[traffic](https://github.com/yaklang/yaklang/actions/runs/41402609456)。运行ddd补位字段展示，后续83f4修复通用IPv4头分发，[新范围及Windows实抓](shark-m2-ipv4-header-dispatch-20261102.md)，新最终head CI另验，均不替代SMB验收。S01–S06保持，107/121须重新核对；开工仍固定远端最新M2精确SHA。

最新可复核父基线为`93c852275052dd252be662136a97f61d6c017f8a`，47约定检查/91实际执行/2statuses及314266精确traffic ID已全绿；[traffic](https://github.com/yaklang/yaklang/actions/runs/41381222731)。后续M2-65仅补非SMB option父头观察和内嵌文档，[实际证据](shark-m2-ipv4-option-observation-20261102.md)，新最终head另验CI。下文旧检查点保留为历史；承接人从开工时远端最新M2固定S01。SMB 107/121为待复核历史缺口，不能用上述非SMB验收替代S01，S01–S06范围不变。

承接 Yak Shark M2 的 SMB 专项：补齐已定位的原生解析、类型化字段、连接状态和递进展示，使用真实捕获与 Wireshark 逐包验证。正确性优先。只补协议名称、端口提示或让报告数字相同，不能算完成。

主线程继续 TLS、QUIC 和其他非 SMB 工作；SMB 由承接人独立实施。每项必须说明实现边界、复现输入、修复前后差异和验收证据。

## 当前基线

- M2 阶段分支：`codex/shark-m2-protocol-correctness`，阶段 PR [#5371](https://github.com/yaklang/yaklang/pull/5371)。
- 最新已独立全绿检查点为 `9b0b8a3cc9af7a19aca39f9c363ca44b566f7bb0`：47验收检查名、2statuses、全部91实际执行检查成功；313819pass/25历史optional skip，74新增及131084 required全pass/0skip，实际checkout和完整官方产物已核。[traffic](https://github.com/yaklang/yaklang/actions/runs/41336269942)、[Essential](https://github.com/yaklang/yaklang/actions/runs/41336273614)、[Diff](https://github.com/yaklang/yaklang/actions/runs/41336277251)。后续非SMB代码/构建为 `25c342b86232636532fcf5978f9b17e6ffd1a057`，[PHA请求约束和实际Windows](shark-m2-tls-pha-context-20261102.md)，本说明最终head CI另验。上述TLS/公共回放不替代SMB S01；承接人仍从开工时远端最新M2独立固定基线。
- 开工从 M2 远端最新提交建立独立工作区并记录精确 HEAD。main `238faf87f36e9eff5ec1ea0ff06e8c06d473214c` 已普通合入49bb；可复核全绿检查点为 `8a02294759c1ead373c60c60fb870c7649d92fa7`，实际47checks/2statuses、313133pass/25历史optional skip、131084 required全pass/0skip已逐项核验。[完整traffic](https://github.com/yaklang/yaklang/actions/runs/41277905022)与[Essential](https://github.com/yaklang/yaklang/actions/runs/41277858361)均成功，不计自动draft skipped。后续非SMB正文工作继续进入同一M2，最终head仍看阶段PR的独立CI，不沿用旧结果。M1 #5353 已合并，保持唯一M2活动阶段。
- 历史SMB审计基准仍为 `a4df082e02cdc1aef427baff7a4fd70774fb4911`（M1 `a9b8df4f64364065765fc0f7f1df4ed9d68cf7dc`），当时47检查、2状态、312727 pass/25历史optional skip、131084 required全通过。旧107/121及1008分母须在S01重新复核，不能把main合并或TLS验证当SMB已验收。
- 随后非SMB检查点 `6318013ae9716674b1741e203d1314c7e170e8ef` 已实际47checks/2statuses全绿，313231pass/25历史optional skip、131084 required全pass/0skip；[traffic](https://github.com/yaklang/yaklang/actions/runs/41282119380)、[Essential](https://github.com/yaklang/yaklang/actions/runs/41282070062)。后续同分支QUIC代码/构建为 `0fc42e285292b38090c24e77d114bdb7da009d61`，其最终说明head须另验CI；不将这批QUIC/Windows结果当SMB重新验收。承接人仍从开工时远端最新M2提交复核S01。
- 最新非SMB代码/干净构建为 `788c8e27ba38d9037a4a75284bbcada74f843d7e`，[TCP承载资格、原始长度、实际回调与Windows证据](shark-m2-tcp-carrier-and-length-20261101.md)。最终说明head的CI另验；本批不分析SMB，不能用大包输出保持或非SMB修复替代S01的SMB逐记录基线。
- 后续已核全绿检查点为 `3d70333bab584719207e80283e3d385cc2905b82`：47checks/2statuses，313502pass/25历史optional skip，131084 required全pass/0skip；[traffic](https://github.com/yaklang/yaklang/actions/runs/41301299644)、[Essential](https://github.com/yaklang/yaklang/actions/runs/41301302855)、[Diff](https://github.com/yaklang/yaklang/actions/runs/41301306047)。其后同一M2的HTTP代码/干净构建为 `c0b53eedab27bc5e4d0c30956d69b591b5d128a5`，[有限观察及完整回放](shark-m2-http-orphan-observation-20261101.md)，最终说明head须另验CI。上述非SMB运行不替代S01；承接人仍从开工时远端最新M2重新固定基线。
- 9 个分批 PR 已归入 M2。原 SMB 批次 #5380 的已有代码已保留；最终 fuzz 取消、重建回放和 Windows 的未关闭项仍然有效。
- 最新已独立全绿检查点为 `7a9a1b14959f6487e09e0144fcbdea2d63bcc94c`：47必需检查名、2状态、313584pass/25历史optional skip；82新增及131084 required全pass/0skip，实际checkout和官方完整产物已核。[traffic](https://github.com/yaklang/yaklang/actions/runs/41310969981)、[Essential](https://github.com/yaklang/yaklang/actions/runs/41310973638)、[Diff](https://github.com/yaklang/yaklang/actions/runs/41310977188)。其后本地Certificate生产构建为 `0f1ab555287149b3b3ced55191597848f135ad66`、QA为 `15290b57c24a02fdfc5fd1da94dd25c7f34a7291`，新head另验CI；不将其非SMB证据当S01已通过。上述基线记录均保留历史事实，承接人仍从开工时远端最新M2独立固定S01。
- 在独立 worktree 或 clone 开发，使用临时 `codex/` 分支，交付目标为 M2。不得修改 M1、强制覆盖 M2 或在主线程正在测试时改动其源码目录。临时分支不是第三个阶段，合入后清理。

## 已有成果和保留缺口

2026-11-02交接续记：最新已独立全绿检查点为 `ce26c0d9238194e4dde43547e536199a357ff803`，
47验收检查名、2statuses及全部91实际执行检查成功，313745pass/25历史optional skip、
131084 required全pass/0skip；[traffic](https://github.com/yaklang/yaklang/actions/runs/41329494037)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41329497344)、[Diff](https://github.com/yaklang/yaklang/actions/runs/41329500697)。
随后非SMB代码/干净构建480b5补充证书epoch与恢复握手边界，最终说明head须另验CI。
以上不重新验收SMB；承接人从开工时远端最新M2独立固定S01，保留原107/121和字段分母。

SMB1 到 SMB2 协商后的探测恢复已有实现，107 个完整记录、121 个命令头以及全部 281 条来源门属于已有有限成果。先复核并保持它们，不把这些记录再次算成新增成果。

字段审计保留同一 checker04 的 1400 个原始 coverage gaps；其中 392 个 QUERY_INFO 映射项已独立闭合，另外仍有 1008 项：961 个字段未发表、47 个范围未映射。原审计中的“已映射值错误为 0”只覆盖已映射项目。

QUERY_INFO 固定 envelope、请求选择器保留，以及 FileEaInformation、FileStandardInformation 的有限布局已实现。其他类型化内容、其余命令、compound 与完整生命周期仍开放。来源齐全、单元测试通过和实际产品字段验收要分别记录。

[SMB 搁置记录](https://github.com/yaklang/yaklang/blob/a4df082e02cdc1aef427baff7a4fd70774fb4911/docs/exec-plans/shark-m2-smb-deferred-20261031.md)保留所有原失败、取消与较早运行。

## 工作包和完成条件

| 工作包 | 范围 | 完成条件 |
|---|---|---|
| S01 基线和来源 | 当前构建、原始大捕获、checker、281 条来源及 107/121 恢复门 | 记录代码与工具版本、配置、输入 SHA256；旧问题可复现，分母与记录身份固定 |
| S02 SMB2 字段 | M2-04 剩余布局、字段与范围 | 逐布局原生与 canonical 接通，协议目录和字段注册一致；请求和响应分别核验 |
| S03 SMB1 剩余正文 | M2-36 其余 12 条记录、连接和文件状态 | 从完整源与规范实现，不把外部草稿当已完成代码 |
| S04 会话和重组 | M2-05 的复用、乱序、丢失、重传、异步及 compound | 同域正例、跨域负控和状态退休门通过，缺上下文保守拒绝 |
| S05 展示和来源 | 摘要、字段树、物理帧、重组 PDU、字节高亮 | Windows 实际运行，字段与其字节源逐项对应，上下文不借当前位置 |
| S06 验证与交付 | 原失败、真实流量、fuzz、全量回放和最终 CI | 必需项无缺失或跳过；精确最终 SHA、完整产物及未关闭清单可复核 |

## SMB2 字段范围

M2-04 原盘点有 19 个父项。第 08 项的请求固定 envelope、第 19 项的补充来源网格已有有限成果；其余 17 个布局父项继续验收。第 19 项的来源齐全不等于那 12 条记录的产品正文已经通过。

| 原 ID | 布局和验收重点 |
|---|---|
| M2-04.01 | NEGOTIATE 固定字段、逐 dialect 值及各自范围 |
| M2-04.02 | NEGOTIATE 类型化 context，未知 context 保留原字节 |
| M2-04.03 | CREATE 请求与响应固定字段 |
| M2-04.04 | CREATE context 链及实际观察到的 payload 类型 |
| M2-04.05 | CLOSE reserved 字段与响应语义 |
| M2-04.06 | SESSION_SETUP 请求、响应和错误 envelope |
| M2-04.07 | TREE_CONNECT 请求、响应 envelope |
| M2-04.09 | QUERY_INFO 响应及其余选择器对应内容 |
| M2-04.10 | IOCTL 请求 envelope |
| M2-04.11 | IOCTL 响应与原捕获中三类控制 payload |
| M2-04.12 | QUERY_DIRECTORY 请求 |
| M2-04.13 | QUERY_DIRECTORY 响应及目录项链 |
| M2-04.14 | READ 请求 envelope 与 channel 语义 |
| M2-04.15 | READ 响应数据边界 |
| M2-04.16 | WRITE 请求、响应和错误分发 |
| M2-04.17 | LOGOFF、TREE_DISCONNECT 正文与生命周期 |
| M2-04.18 | 共享 ERROR 布局及状态决定的分发 |

每项都核对字段类型、宽度、有符号性、未知枚举、精确字节范围、零长度、截断、溢出、错误状态及集合预算。一个命令的 offset 和 length 不能借后一个 compound 命令的字节通过。保留原始数值与拥有的字节；展示文本不代替 literal。

原大捕获中的请求位于 VLAN81，响应位于 VLAN80。Wireshark 中部分响应选择器来自跨域继承装饰，不能作为响应 wire 字段，也不能为了对齐显示跨 VLAN 借请求。使用同域真实或独立构造正例证明类型化响应，跨域输入保持 opaque 或明确缺少上下文。

## SMB1 正文和状态

现有索引 23–26 的 TREE_CONNECT_ANDX、DELETE 四个有限 profile 保持回归。剩余 27–30 的 NT_CREATE_ANDX、WRITE_ANDX，以及 31–38 的 TRANSACTION2、SET_INFORMATION2、QUERY_INFORMATION2 等 8 条记录，按原索引逐项拆分。

- source110 已有 119 项独立事实和 32 个门；source117 的 open/write patch 位于仓库外，基于旧提交，尚未应用或编译。先核对当前代码再决定可复用部分，不能直接整包套用。
- 核对请求和响应、成功和错误状态、WordCount/ByteCount、Unicode/OEM、raw NUL 视图、AndX/secondary fragment 及命令局部边界。
- 观察到的 UID、TID、FID、文件名和操作关联分别保存。只凭相同 MID 或熟悉的字节长度，不能借旧请求、旧文件或别的命令完成解析。
- 对未支持布局、加密或压缩载荷明确保留 opaque 和原因；不把字段存在推断为已验证签名、认证或文件操作成功。

## 重组和连接验收矩阵

至少覆盖 eager/deferred、多个 workers、本地与跨进程 DTO，并在实际 TCP 路径执行以下矩阵：

1. TCP、NetBIOS 和应用消息边界不同位置的分片；每个 offset、length 和贡献帧正确。
2. 乱序、重复段、完整重传、永久洞、截断、重叠冲突；重传不能重复发布，残包不能靠公开字段缓存伪装成功。
3. 新 SYN、未建立的新 SYN、FIN/RST、四元组复用；旧连接与新代次不串流。
4. MID 重用、重复响应、迟到响应、未匹配响应、错误 SID/TID，以及异步 pending/final、AsyncId 和取消。
5. compound 缺前驱、前驱晚到、related operations、不同命令间的 UID/TID/FID 继承边界；无有效前项时不制造关联。
6. 相同端点、相同 ID 在不同 section、interface、VLAN 中的正负组合。每一层状态都按捕获域隔离。
7. 预算恰好足够、恰差一字节、超限、释放和拒绝后的状态；拒绝不得部分消费 pending 或污染另一方向。
8. 修改公开 DTO、字段树或调用者输入后，私有已接纳消息、拥有字节与后续解码保持不变。

分别核对原始包时间、贡献帧、重组完成帧、实际 Feed 回调、父子关系和 response_to。Wireshark 的 stream 编号不是连接事实的唯一答案。

## 展示和字节来源

列表显示具体协议版本、命令及关键结果，处理重复 Packet Name 摘要。详情按 Header、Body、Observed context 递进；当前 packet literal 与连接缓存值分开，不能用缓存的协商结果覆盖当前包的值。

字段点击对应完整 PDU 内的真实位置；同值出现在不同命令时仍保留各自位置。物理帧、重组 PDU、解密或解码后的拥有字节分别显示。没有原始范围的请求上下文、推导值或认证状态不能制造高亮。

Windows 实际启动新构建，核对列表、展开树、鼠标、Tab、字节源切换、窄宽窗口和 CP936。保存真实窗口截图或控制台 cells/attributes 记录，并明确图片类型；字体、DPI、IME 只按实际测过的范围验收。

## 输入和复核资料

主要公开捕获：[Johannes Weber The Ultimate PCAP](https://weberblog.net/the-ultimate-pcap/)。已用解压文件 SHA256：
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`。

本机文件：
`C:/tmp/yak-shark-public-corpus-20261024/weber-20260710/The-Ultimate-PCAP.pcapng`。
工具基线为 Go1.22.12、TShark4.6.9；首次复核记录实际二进制哈希与隔离配置。先完整捕获解码，再筛选目标，保留默认识别与显式 Decode As 的区别。

外部证据根：
`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。

| 证据目录 | 用途 |
|---|---|
| M2-SMB-remaining-layouts60-01 | 19 父项的原始来源、字段分配与验收清单；旧状态须结合后续报告 |
| M2-SMB-query-info-source62-01 | 93 个完整 PDU、113 命令、731 固定字段 |
| M2-SMB-neutral12-source62-01 | 另外 12 条完整记录的中立事实 |
| M2-SMB-QI-types-source63-01 | 已选定的类型化输出事实 |
| M2-SMB1-open-write-source110-01 | open/write 的独立原始事实 |
| M2-SMB1-open-write-source117-draft | 未应用、未编译的旧基线草稿 |
| M2-Weber-SMB-audit68-01 | 后续字段审计与保留差异 |
| M2-Windows-SMB1-file-closure116-01 | 较早 Windows 有限页面验收 |
| test-M2-SMB-authority-full126-01 | 最后相关回归及精确输入快照 |
| fuzz-M2-SMB1-file127-01 | 用户取消的最终 fuzz，不计通过 |
| stage-consolidation-20261031-01 | 当前两阶段归并与 a4df 的完整 CI 证据 |

交接时核对目录内 manifest、原始输入哈希和实际执行 SHA。大捕获、真实抓包、keylog/密钥、完整 JSONL/PDML、图片和 fuzz corpus 继续留在仓库外，不能打入中央样本 ZIP。

## 实现入口和协作边界

- 目录和原生分发：`common/bin-parser/protocol_catalog.go`、`common/pcapx/pcaputil/protocol_registry.go`。
- 共享解析：`common/bin-parser/parser/cifs*`、`common/bin-parser/parser/stream_parser/cifs*`、`smb2*`，及 `rules/application-layer/cifs.yaml`、`smb2.yaml`；规则归档用既有工具重建。
- 原生状态与字段：`common/pcapx/pcaputil/bin_parser_smb1*`、`bin_parser_smb2*`、`protocol_smb1*`；字段注册位于 `protocol_field_registry.go`。
- 展示：`common/yak/cmd/yakcmds/shark-cli/smb_summary.go`、`protocol_events.go`、`presentation.go`。
- fixture 读取复用 `internal/trafficfixture`，必须落盘的样本写测试私有临时目录。

优先修改 SMB 专用文件。通用 framing、调度、字段注册和展示文件由 SMB 与非 SMB 两条工作线共用；改动前约定接口和文件归属，合入时重跑另一条工作线的回归。不得提高生产预算、修改全局拒绝语义或删除非 SMB 断言来兼容新实现。

## 执行顺序和交付

先做 S01，冻结来源和失败基线；再逐布局提交 S02、S03，同时补齐该布局所需的 S04 状态门；最后完成 S05、S06。每个提交都应形成可复核的小范围行为，不按协议数量或测试条数推算完成率。

每个验收项交付：

- 问题 ID、profile、规范版本与具体条款、修复前后行为。
- 原始捕获 SHA256、物理帧/贡献帧、独立字段值和范围；实际值错误、字段未发布、范围未映射、参考限制分别计数。
- 原生/canonical/SDK/CLI 的同一输入结果和精确字节来源；不任意重编号或删除差异字段。
- 正常、残缺、错误和跨域负控；原失败 seed 必须重放。fuzz 使用实际覆盖插桩并报告执行次数、覆盖输入及失败，不以普通循环次数冒充。
- 新 SDK/CLI/flow 构建的真实回放、独立 Wireshark 对比、Windows 页面证据和该次源码快照。
- 精确最终提交的完整 CI、实际 checkout、完整 artifact 与必需测试清单；旧 SHA 的绿灯、缺路径静默跳过或仅本地通过不能替代。

执行期间保持源码、HEAD、index、规则和 fixture 不变。Go 构建/测试沿用3CPU、2GiB、`-p1`，fuzz1CPU/768MiB；本机 Go/SDK/UI 依次运行。资源不足先保全结果和清理本任务临时文件，不停止用户服务。

剩余字段17项已有粗估17–35个AI实施小时，会话边界约3–5小时。SMB1剩余正文、最终fuzz/页面和新增问题要先盘点再估；这些数不能当作整个SMB任务总工期。每次报告已完成、进行中、未开始及证据，新增问题追加稳定ID。

## 规范和最终验收口径

以原始字节和已固定版本的一手规范确定字面字段与布局：[MS-CIFS](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cifs/)、[MS-SMB](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb/)、[MS-SMB2](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/)，文件信息类型按相应 MS-FSCC 条款核对。每个 profile 记录实际采用的文档版本和章节。

Wireshark用于逐包参考。遇到装饰字段、跨域上下文或派生坐标时保留差异和原因，不能用错误参考反改原字节，也不能以“参考有问题”为由跳过其他合法字段。

S01–S06逐项验收后才关闭对应M2项。未支持布局继续明确列出，不把“已识别SMB”写成“SMB全部正确”。

