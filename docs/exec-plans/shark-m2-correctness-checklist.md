# M2 协议正确性验收清单

2026-11-01已将9个分批PR实际归并到M2阶段PR #5371，并移到rebase后的M1基线。
[当前两阶段分支、树核对及历史归档](shark-stage-consolidation-20261101.md)。下文分批SHA/分支/PR
状态保留为当时证据。M1现已合并，M2保持唯一活动阶段分支/draft PR，SMB继续搁置。

2026-11-01新进展：M1 #5353 squash 到main `dba37562abddd57fc6386818dbd7953eb52d314c`，
与已验收a9的整个树相同。M2经普通merge `f99ef2b7ad97c135d9f2e49add582cf8b8e46e2d`
连接新main；M2树及相对M1补丁SHA均不变，未改写历史/强推，PR #5371现以main为基线。
[CertificateRequest正文、实际Windows列表缺口修复及大捕获证据](shark-m2-tls-certificate-request-20261101.md)。

随后又将最新main `238faf87f36e9eff5ec1ea0ff06e8c06d473214c` 普通合入，合并为49bb，
兼容测试修正/干净构建为 `d1a6e62123ea8c6b6d3b43838c12beb86943ddaa`。
[真实PHA Finished修复、9组回放、36个新Windows状态及精确CI门](shark-m2-tls-finished-20261101.md)。
随后8a检查点已实际闭合47checks、2statuses及313158个精确traffic ID（313133pass/25历史optional skip）；
合并后6次完整大捕获346和逐对象347也已闭合，非SMB及原ID/raw/来源无变化，SMB仅保留main差异。
新增CertificateVerify的原生/拥有字段/列表修复为e5和488，
[175帧7连接、13签名/19MAC独立核验、11组22次回放及72个Windows状态](shark-m2-tls-certificate-verify-20261101.md)。
631检查点已实际47checks/2statuses及313256精确traffic ID全绿，313231pass/25历史optional skip，
98个新增CV和131084 required均全pass/0skip。随后同分支完成
[QUIC CRYPTO层级、拒绝字节与实际页面五个有限合同](shark-m2-quic-crypto-qualification-20261101.md)，
最终代码/干净构建为 `0fc42e285292b38090c24e77d114bdb7da009d61`。
新文档提交的CI仍须由PR按实际head_sha闭合，不能拿631、8a或旧分批全绿替代新代码。
随后同分支完成[捕获TCP承载资格、原始IPv4零长度及实际页面](shark-m2-tcp-carrier-and-length-20261101.md)，
代码/干净构建为 `788c8e27ba38d9037a4a75284bbcada74f843d7e`。
53/54两个有限合同已有本地、原始来源、实际回调和Windows证据；最终说明head的完整CI仍归11。
其后3d703检查点实际47checks/2statuses及313527精确traffic ID全绿（313502pass/25历史optional skip）。
同分支新增[缺请求HTTP头、未定界流观察和实际页面](shark-m2-http-orphan-observation-20261101.md)，
代码/干净构建 `c0b53eedab27bc5e4d0c30956d69b591b5d128a5`；82新ID本地全pass，
54个原头与1048个后续观察均有独立来源；新最终head完整CI不沿用3d703成功。
随后7a9检查点已实际47checks/2statuses及313609精确traffic ID全绿，313584pass/25历史optional skip，
82新增与131084 required均全pass/0skip，完整artifact及实际checkout已核。
同分支新增[Certificate条目扩展校验、拥有正文/字段和真实Windows翻页](shark-m2-tls-certificate-vectors-20261101.md)，
生产代码/干净构建 `0f1ab555287149b3b3ced55191597848f135ad66`，QA工具 `15290b57c24a02fdfc5fd1da94dd25c7f34a7291`。
59新ID、58636插桩执行、30实际回放、全部306380事件/380035行差分及46 Windows状态已有本地证据；
新最终head须独立完整CI，7a9成功不包含Certificate修改。N13完整角色、阶段、PHA关联和高级恢复仍开放。
随后17fe补齐[已知服务端Certificate的空context/非空列表条件](shark-m2-tls-certificate-roles-20261101.md)，
作为M2-56的限定补充，不增加父任务分母。43新ID、18次实际旧/新回放、完整大包六输出字节相同及34 Windows状态通过；
本地完整239551pass/12历史optional skip。066专项CI313643pass/25历史optional skip已核，但其整体CI因RAG随机采样失败未全绿；
b322只修正该必需测试的确定性输入并加强身份断言，50次重复及26必需集通过；新最终head仍独立验收。
后续c1ed已47必需检查名/2statuses及全部91执行检查全绿，313686pass/25历史optional skip，
43新增及131084 required全pass/0skip，实际checkout和官方完整产物已核。
同一M2继续[初始证书上下文、认证恢复和诊断字段](shark-m2-tls-initial-context-and-diagnostics-20261102.md)，
最终代码/干净构建 `7aab30def3f2c545e108c15e1ac0297cd891809a`。49 native/10 CLI新ID、
18控制+36 PHA实际运行、两大包六输出字节相同及44 Windows状态通过；新最终head须独立CI，不沿用c1ed。

M2最初从M1全绿提交 `39729081a3300381154d96cf1b4866f23976260b` 分出；
[验收记录与 PR/CI 链接](shark-m1-acceptance-397.md)。新分支为
`codex/shark-m2-protocol-correctness`。此表按可验收工作项计数，不按协议数量、
代码量或测试数量估算完成度。M1 尚未合并时，M2 PR 以 M1 分支为基线，避免重复展示 M1。

后续分支 `codex/shark-m2-smb-query-info-types` 从已全绿的 M2 `f9faae9f82c0dbe6a657bfaa94d7d6acdc845380` 开始。
当前状态：**60 项，已完成 50，进行中 6，未开始 1，按用户要求搁置 3；剩余 10 项（含搁置）**。
完成项为 M2-01/02/03、06、12..42中除36外、44、46..60的有限合同，不含进行中的 M2-11；不代表核心协议或整个 SMB profile 验收完成。
随后已明确新增、关闭59的PHA offer/已观察context合同和60的认证后握手诊断身份/连接说明合同。
原宽口径剩余10项保持；非SMB仍17包中11有限完成、6开放，粗估23–46 AI实施小时，构建/CI/回放等待及新发现另计。
[新双轮实抓、三套件AEAD矩阵、实际参考差异、66 Windows状态和最终完整回放](shark-m2-tls-pha-context-20261102.md)。
原9b已独立47检查名/2statuses及91实际执行检查成功，313844原始traffic ID、官方产物和实际checkout闭合。
本批最终代码/干净构建25c新增101个ID，事前固定313945 ID和131084 required全pass/0skip门；包含本说明的新head仍须独立核验CI。
以下保留480b5当时范围：[独立AEAD控制、五次完整参考、九恢复/复用profile、三实际Windows页及全量输出](shark-m2-tls-epoch-boundary-20261102.md)。
当时原ce26已独立47检查名/2statuses及全部91实际执行检查成功，313770原始ID与事前清单一致；
当时新代码/干净构建480b5和其后说明head须独立核验，后续已由9b实际闭合；9b不包含本批25c。
M2-30/31/32 的正文恢复、异步身份与页面合同已通过本地及原始捕获验收；
M2-33 的有限WC17响应profile已经完成代码、原始来源和Windows验收，整批CI归M2-11。
M2-34/35的有限setup/当前wire prefix合同已完成本地、原始来源、fuzz和真实Windows验收，整批CI归M2-11。
M1 #5353后续普通合入main `b22442c81d64f7d825f133931d571f7d0f4d6586`及样本消费者清理，
最新 `761bd25449d1a27bd024785be7b760208a1b58cb` 已47checks+2statuses全绿，独立Windows完整traffic
311918 pass/25历史optional skip/0fail；这是当时仍需review、未合并的历史快照，M1现已合并，不回写旧验收证据。
2026-10-31按用户要求跳过SMB；M2-04/05/36搁置，M2-07只继续非SMB部分。
[SMB已保存实现、实际验证及全部未关闭缺口](shark-m2-smb-deferred-20261031.md)；
PR #5380保持draft，f743最终CI已47checks+2statuses全绿，但最终fuzz取消和重建/回放/UI缺口仍搁置，不作为其他协议前置。
最新非SMB代码/干净构建 `9af551414ebc8f47fb47f471e231ea52fbfbe37a`，
分支 `codex/shark-m2-tls-row-priority`，[PR #5383](https://github.com/yaklang/yaklang/pull/5383)。
[列表修复、60个Windows状态、两个大包五行差异及新资格审计项](shark-m2-tls-row-priority-20261031.md)。
从父#5382最终13ae全绿后才开分支；父47checks+2statuses成功、traffic312560pass/25历史skip，
131084 required和新增37终态全过且已ready。新42有限合同完成；43的
[独立来源、真实mTLS及正文覆盖审计](shark-m2-tls-admission-audit-20261031.md)已补。
两疑点Wireshark默认也TLS，正文校验/字段及资格门仍开放，尚非已证标签误识别。
后续分支 `codex/shark-m2-tls-admission-audit` 从#5383最终全绿26bc分出，此审计不改已ready的#5383。
此前非SMB代码 `ab24d5ec7657a058d3d52a0925f9589e9964d0dd`、测试正控修正及干净构建
`6bacd80c8df9e4a3352f88cadaee889e5058ca3a`，分支 `codex/shark-m2-tls-record-boundaries`，
[N12原失败、record边界/换钥恢复、真实回放及Windows列表新缺口](shark-m2-tls-record-boundaries-20261031.md)。
M2-41关闭有限native/详情字节合同；当时列表未过另列M2-42，后续9af三页60状态已通过有限UI门。
此前非SMB代码/构建为 `4477ebd3d3c718481f9311b3fad133a3d43ab53a`，分支 `codex/shark-m2-non-smb-correctness`，
[真实KeyUpdate、HTTP恢复、Windows和两个大捕获记录](shark-m2-non-smb-20261031.md)；
[M2-10按13个有限条目拆分：7项有限完成、6项未关闭](shark-m2-non-smb-differences-20261031.md)。
该分支从全绿8ed分出，只带SMB搁置文档，不含#5380的SMB解析改动。M2-38/39/40有限完成，08/09/10整体仍开放，最终SHA完整CI归11。
此前代码/构建为 `82fc8ec2f101ef716a715f1f54e8fbf7ebcf701e`，分支 `codex/shark-m2-tls-wire-evidence`，
[TLS字段、实际Windows、恢复/隔离、大捕获与全部未通过边界](shark-m2-tls-wire-evidence-20261031.md)。
M2-37有限合同完成，08/09整体仍开放；最终交付SHA的完整CI归11。
此前SMB1 setup代码构建为 `9d29705ab79f4f0b9dd1a74defeffbf416726560`，
[有限合同、全部失败及实际页面证据](shark-m2-smb-session-setup-20261031.md)，[PR #5378](https://github.com/yaklang/yaklang/pull/5378)。
QUERY_INFO阶段的代码构建为 `0769c2617d8f8fad4e66ecf2a323461669261a10`，
[完整验证与限制](shark-m2-smb-query-output-validation-20261031.md)，最终a407为47checks+2statuses全绿并ready。
最新WC17构建为 `7387371611d5073be5017960f9361b8b2b8cf000`，
[逐项来源、测试和实际页面](shark-m2-smb1-negotiate-20261031.md)；最终交付SHA的CI仍归M2-11。
新增 QUERY_INFO 和源码 oracle 见[本批范围与证据](shark-m2-smb-query-info-20261030.md)。
M2-04 的19个子项另计：QUERY_INFO envelope .08和正文来源补充 .19已完成
（后者仅oracle），12进行中、5未开始；尚未关闭的17项不能用来源完整性代替。
.08仅含固定正文/边界/字节范围及同域请求selector上下文，.09已验收七类中的class5/7两类，整体仍开放。
首批M2本地选定矩阵合并为 **17917 pass /1 历史 live skip /0 fail**，不重复计数；
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
| M2-04 | 121 命令的 header/body/compound 字段逐一正确 | 搁置（用户要求跳过SMB） | 60新构建93个QI PDU/113命令/731固定字段通过；旧checker04原始1400 gaps保留，另有独立392项QI映射闭合，余1008（961未发表/47范围未映射）；.08 envelope和.19来源完成，typed内容/其余命令仍开放 | 剩余17子项约17–35小时；等待另计 |
| M2-05 | SMB 乱序/丢失/重传/四元组复用/接口/VLAN 隔离 | 搁置（用户要求跳过SMB） | 实际 TCP 路径 eager/deferred、多个 workers；12 域双向对应、6 个三段重组、4 次完整重传不重复；全部 pending/dialect/file 隔离、异步 compound 缺前驱/晚到恢复及新 SID/TID 分配仍需验收 | 3–5 小时 |
| M2-06 | SMB Windows 递进页面和字段来源 | 已完成（四页有限合同） | dd63三页及60新QUERY_INFO body实际控制台页；中间重复InfoType只高亮174，不借70/278，完整物理/PDU字节、mouse/tabs/resize/CP936通过。未推广到全部Windows行为或高级typed内容 | 0 |
| M2-07 | 大捕获 SDK/CLI/flow 与隔离 Wireshark 逐包闭合 | 进行中 | 107/121和全部281来源门保持；60三种新构建完整回放、选定QI字段/Windows闭合；dd63→60的42223事件仅93个SMB2变化，其余42130和完整CLI/flow逐字节相同；M1→60全行账本保留，完整语义ID/字段/会话仍开放 | 1–3 小时 |
| M2-08 | TLS 高级丢包乱序恢复与复用边界 | 进行中（8场景实际复验，有限范围已闭合） | 82fc六来源72+两个匹配新SYNACK后77场景重跑，七个严格通过；C1 TLS1.2保留exit4/8项策略差异，额外4TLS+3HTTP经双Python AEAD/peer/父子/response_to/Feed57审计均旧Flow2，未串入新3。真实callback与CompletedIn分开；原严格失败不抹去。字节范围及TLS页面另闭合37；密钥代际/缺开头/KeyUpdate等整体未完成 | 2–4 小时；新增边界另计，不以7/8代替全部高级恢复 |
| M2-09 | QUIC 与 HTTP/3 高级恢复边界 | 进行中（四实际域组合已跑，缺握手整体仍开放） | 82fc接口/VLAN full/full两严格门通过，每域2022认证/3坏tag/6tail、4556来源检查；两个app-only严格73各2664差异原样保留。另核各2007个准确UDP预览（1332截64B/675完整短包），0借域身份，不等于完整QUIC解析。CID/epoch/STREAM/QPACK/0RTT仍需独立语义门 | 2–4 小时；执行等待及新问题另计 |
| M2-10 | 剩余协议、字段、会话及 fuzz 差异转为逐项验收 | 进行中 | 非SMB N01..N17共11有限完成/6未关闭；N07只关闭缺方法的partial可观察性，不承诺完整消息划分。原133项、6 TCP/24 UDP政策项、N13完整阶段及N09缺握手高级恢复仍开放；不将局部字段或展示通过当全部协议正确 | 当前6包粗估23–46 AI实施小时，不含CI/构建/回放等待及新问题 |
| M2-11 | M2 交付的完整回归与精确 SHA CI | 进行中 | ce26最终47验收检查名、2statuses及全部91实际执行检查成功，313770原始ID与官方产物闭合，131084 required全pass/0skip；旧分批成功保留历史。480b5新增阶段/边界和三页Windows已本地复核，含本说明的新最终head须独立全CI及313844原始ID清单；74新增全须pass，不用父成功或draft skip替代 | 30–90 分钟实施，执行等待另计 |
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
| M2-25 | SMB 页面 Header→Body→上下文、兼容副本消重与范围归属 | 已完成（有限合同） | 真实session/compound、typed/值/范围/owner、父IPC决策、不同PDU反例和真实Type3语义/原字节；dd63三页及60的QI桥接Go反例/worker、实际body页与traffic CI闭合；最新整批CI仍归11 | 0 |
| M2-26 | SMB2 前移探测不抢占短 DLMS Wrapper | 已完成（跨协议回归合同） | e7 全 traffic 12 叶/14 fail actions 暴露 `<8` 字节时仅 00 前缀被提前拦截；前移路径现须完整签名，原 `TestDLMSWrapperSealedReplayMatrix/wrapper-short-header` 全 workers/eager/deferred/order 断言和样本原样通过 | 0 |
| M2-27 | Windows 杀进程检查点不得在原子发布期间误报共享锁 | 已完成（本地有限合同，整批CI归11） | 首次完整traffic实际失败保留；只在现有20秒期限内重试Windows sharing/lock，权限/坏JSON仍失败；真实独占Win32句柄控制和原8场景WAL/事务/完整导出断言均通过，未改生产数据库代码 | 0 |
| M2-28 | Yak SSA 入口与include不受AST完成顺序影响 | 已完成（本地有限合同，整批CI归11） | 60原断言本地40轮5次失败，中间候选仍9次失败均保留；自身editor/显式可执行根/保留未visited来源及路径优先级合同通过，受影响包合并682 pass/11历史skip/0fail；[完整证据与行为边界](shark-m2-ssa-entry-20261030.md)，精确整批CI仍归11 | 0 |
| M2-29 | 官方 peter 捕获 raw110/PDML70 QUERY_INFO 观测计数归因 | 已完成（有限来源/固定头合同） | 两独立raw/stream源110=55+55；baseline70的40个响应均保留tcp.segment_data，三个独立重组配置反事实各恢复110；原生300构建110原PDU/固定头/包号/域/流逐一匹配，0缺失/重复/已映射差异、10join负控通过；未证明参考内部C状态或全部body/session一致 | 0 |
| M2-30 | 完整记录的body错误不得停止后续可识别SMB2流量 | 已完成（有限恢复合同，整批CI归11） | 0769三份原始官方捕获2350个QI、49178个固定头/envelope字段和来源逐一匹配；旧17/80布局保留完整raw/header/错误，不伪读PreviousSessionId；仅已知完整DirectTCP及全部compound头可恢复且无state提交；header/framing/资源/关联冲突仍fatal；canonical成功/continuation/ERROR、永久gap与恢复负控及真实Windows拒绝页通过 | 0 |
| M2-31 | AsyncId的pending建立、唯一性、固定身份与MID复用 | 已完成（有限身份合同，整批CI归11） | PENDING按literalMID绑定；final按已观察非零AsyncId；同ID幂等，换ID/他请求占用/同compound两类冲突拒绝；完成后删唯一alias，MID复用后的late99不借新selector；native/canonical/恢复预检0/1/max ID及workers矩阵、428项回归和覆盖引导fuzz通过 | 0 |
| M2-32 | 类型化SMB列表与拒绝记录的递进展示一致 | 已完成（四页有限合同，整批CI归11） | 实际列表显示FileStandardInformation/EA及精确整数；Header→Body→命名类型→literal；无正文解码的完整记录显示unsupported body layout，混合成功/失败回调顺序与redraw不丢状态；0769四个Windows实际控制台页80状态、完整物理/PDU字节与精确高亮、mouse/tabs/resize/CP936通过；8785 pass/3历史skip | 0 |
| M2-33 | 真实SMB1 NEGOTIATE扩展安全响应被误报malformed | 已完成（有限profile，整批CI归11） | 7387共享canonical/native WC17响应、uint64 FILETIME/int16时区/GUID/opaque blob；原frame16/17双副本156 literal/1142断言和源码67负控，0已映射差异；510矩阵、最终103984覆盖引导fuzz、两实际Windows页53状态通过；PID32/MID/域/代次/重复/早到/复用/现代载体含body拒绝时的旧资格退休、预算私有拒绝与cap计费均有负控。不是全部SMB1或token认证完成 | 0 |
| M2-34 | SMB1 SESSION_SETUP_ANDX扩展安全WC12/WC4尚未原生接入 | 已完成（有限profile，整批CI归11） | 9d共享canonical/native；原19–22四PDU/两对关联及续接，244双副本literal/raw alias/2214断言0差异。真实UID碰撞红复现后root-lineage/quarantine修复；多轮、孤儿/重复/复用/独立PID/现代载体、typed拒绝/临时副本预算、TCP乱序/重传/永久洞/新SYN与接口/VLAN通过。132777 wire及37657 state插桩fuzz0fail；Windows两页55状态、UTF16/raw NUL与opaque尾部已看图核验。token不认证、其余16命令未关闭 | 0 |
| M2-35 | 非HTTP流的Stream起始页仍显示HTTP专用缺头提示 | 已完成（当前wire prefix证据合同，整批CI归11） | 保留独立HTTP完整头证据；完整当前PDU须domain/物理TCP代次/双向端点/offset0/全部字节和SHA/完成来源/成功字段查询一致。连接标签、错误/残缺、另一代/域、后续或解密child不能冒充起始身份；首轮真实页仍unknown的证据保留，9d重跑实际显示223/153B SMB_COM_NEGOTIATE，窄屏与来源不变 | 0 |
| M2-36 | 其余16条原始SMB1命令的原生正文和状态 | 搁置（用户要求跳过SMB） | 已保存四个TC/Delete有限profile及共享owner；735相关回归通过。较早SDK/Windows/Weber及fuzz不替代最终76e0bd4运行；最终fuzz用户取消、重建/UI/CI未验收。另12条、active AndX、FID及其余正文见搁置记录 | 暂不估时、不作为后续协议前置 |
| M2-37 | 原生TLS literal范围缺失与TLS起点/详情来源不清 | 已完成（82fc有限合同，整批CI归11） | 私有已接纳头与当前完整Raw三词核对后提供3个头范围/opaque外层范围；GetFields/Session/DTO树不变、derived/context不借范围。完整TLS envelope/encrypted只在强证据及原domain/物理TCP代次/offset0/全部Raw前缀/source一致时显示起点；详情Header→Decoding→Connection不丢值。3557选定回归、38400插桩fuzz、大包4404个TLS精确SDK/CLI范围差分及2页36实际Windows状态/4原始Handshake起点通过；不是全部TLS语义完成 | 0 |
| M2-38 | TLS1.3 KeyUpdate具体字段、摘要与认证握手字节来源 | 已完成（131有限合同，整批CI归11） | 4477真实53帧/29TLS/25AEAD/6KU/12HTTP与独立Python及新WS169核对；typed请求值/枚举/owned5B/官方derived alias，独立明文来源[4,5)不借密文。b2a插桩106344次0fail，4477 Windows两页36状态；KU record尾等高级边界仍开放 | 0 |
| M2-39 | 迟到HTTP请求上下文造成响应永久停止 | 已完成（有既往唯一请求方向的有限恢复，整批CI归11） | 原实现红158的8fail保留，绿160的9terminal通过；HEAD/CONNECT/Expect100/预算/来源/初始孤儿负控。4477七真实场景两客户端旧6差异变0，6严格通过，实际Feed回调与独立父源核验；永久客户端仍6差异，不猜方法、不改预期 | 0 |
| M2-40 | 原94/39字段清单绑定当前非SMB构建与大捕获 | 已完成（133项有限清单，整批CI归11） | 137原133逐项新physical/DNS门及固定TLS参考哈希；4477两个完整捕获SDK42223/263109及CLI/flow共6导出与4bfb逐字节相同，闭合173据精确输出绑定133门，无ID归一化、无新TShark冒称；2非法UTF8文字差异及RDPUDP参考限制保留，不宣称全协议语义 | 0 |
| M2-41 | TLS1.3 KU record尾边界及跨epoch旧握手残片 | 已完成（N12有限native/详情合同，整批CI归11） | 176独立3正/3负AEAD源、39ca SDK实际误接受/HTTP泄漏；6bac同v02六门通过、12DTO负控；跨epoch认证成功才退休旧同向残片，三套AEAD/合法分片/坏尾/反向状态、5038原生与1055CLI/96752插桩fuzz通过。198/200真实七场景和207/208/209保留旧失败，199六大捕获导出与133逐字节相同；Windows详情及列表分列，不称全UI通过 | 0 |
| M2-42 | 多TLS PDU物理帧的列表摘要挤掉关键结果 | 已完成（9af有限展示合同，整批CI归11） | 原214/222列表失败保留，225红3pass/7fail、227完整CLI1066pass/2历史skip；完整消息前移且原Malformed原因不被后Context盖掉，不改事件/应用身份。229实际三页60状态及root9图、原list断言/全部源与高亮/resize/CP936通过；231四SDK/flow字节相同，CLI仅Sharon5行info/status按原native事实校正，其他pdus/字段/源/域/stream逐对象完全一致 | 0 |
| M2-43 | TLS身份资格分层与CertificateRequest正文 | 进行中（正文/字段/拥有源有限通过，阶段/恢复仍开放） | CR的56来源事实、55参考literal+1空向量、QUIC CRYPTO重组及有效fuzz81017次保留；46/47关闭Finished/CV正文，48..52关闭QUIC有限资格与实际页面。PHA offer/上下文唯一性/多轮关联、完整阶段、协商依赖/传输参数与缺握手高级恢复仍开放。损坏Certificate后CV边界不可证的缺口保留，不将源oracle核验说成产品验证 | 扩展后的后续实施4–8 AI小时，执行等待另计 |
| M2-44 | 多个完整TLS PDU挤掉后续具体正文摘要 | 已完成（本地及真实页面有限合同，最终CI归11） | 实际282页正文正确但列表缺请求，286红2pass/4fail；4f7仅对已校验CR/认证KU前移，同record及跨PDU均覆盖，不改原详情顺序/错误/应用身份。288完整CLI1093pass/2历史skip；292两页36实际Windows状态、完整明文及精确高亮/缩放/CP936；新字段与publication全部按原生ID核对 | 0 |
| M2-45 | IP碎片来源归因及有界重组基础接入 | 未开始（已知能力缺口，未验可恢复分组） | README明确未实现IP碎片重组；新297/300完整参考有大量Fragment列差异。须先冻结原wire/domain/offset/MF/ID/长度、完整/永久洞/重复/冲突及可恢复组；逐步接入IPv4/IPv6、预算/超时/域隔离/拥有来源与递进UI，不能直接将全部列差异当bug或猜补缺字节 | 后续实施8–16 AI小时，来源新问题另计 |
| M2-46 | PHA Finished误重置应用密钥/序号及缺少正文资格 | 已完成（有限合同，最终CI归11） | 9c/7c有界长度、记录末尾及原子epoch；真实source305红4条HTTP→6，source316同四元组两代12条HTTP，丢失/残缺/损坏不捏造PHA完成。d1干净9组/18次SDK+CLI、当前两页36 Windows状态及52/16B精确来源；有效36654次状态fuzz、187新终态；[完整证据及边界](shark-m2-tls-finished-20261101.md) | 0 |
| M2-47 | TLS CertificateVerify正文、签名方案与拥有来源 | 已完成（有限合同，M2-10.N15，最终CI归11） | e5完整版本化向量/方案/长度/拥有字节，488修复实际页面被Finished挤掉的可见摘要；175原始帧7连接、13签名/19MAC独立oracle，11组22次SDK/CLI与新隔离参考，残包/乱序/复用及72个新Windows状态。98新增精确ID全要求pass；75064次插桩framing/ownership fuzz。明确签名/信任/上下文未验证，损坏Certificate后的恢复不冒充关闭；[证据和保留失败](shark-m2-tls-certificate-verify-20261101.md) | 0 |
| M2-48 | QUIC TLS消息层级/版本/票据与TLS批提交资格 | 已完成（N16有限合同，最终CI归11） | 90bc逐消息完整性、层级、旧TLS拒绝、未知profile限定、NST early_data及Finished尾；新实抓2050认证包、三套件/六换钥请求/CID与NAT/真实丢延损，30个AEAD有效控制样本旧18误接纳→新27拒绝/3合法通过；不等于全包事务或完整TLS状态机 | 0 |
| M2-49 | 合并QUIC数据报的错误在实际页面不可见 | 已完成（有限展示合同，最终CI归11） | 3a错误前移、mixed-packet-outcomes及详情早诊断，保留正常邻包和应用身份；21红终态及完整CLI绿测；最终432-02四页83状态实际诊断可见，源/高亮/缩放/CP936通过 | 0 |
| M2-50 | 已确认QUIC握手仍只显示包号/1-RTT | 已完成（有限名称合同，最终CI归11） | 89f原生认证/层级/封套合格才显示当前消息名；不推导协商版本，应用行优先；实抓全部21封套及36完整流在PDU快照保留，CLI6行来源解释；公开Weber6行经独立Initial解密及新参考验型/长度/PN | 0 |
| M2-51 | CRYPTO拒绝缺完整解密字节且错误暗示整包回滚 | 已完成（有限错误路径合同，最终CI归11） | 0fc拥有载荷/SHA及重复包保留；实际早前CID帧已生效但TLS批不推进反例；27真实原包变体逐字节源核对，57193次原生插桩执行，解析停止与未交付分开，拒绝诊断不捏造CompletedIn | 0 |
| M2-52 | QUIC Crypto Data在worker变Base64且没有范围 | 已完成（有限字段来源合同，最终CI归11） | 实际424旧worker复现，0fc按认证/完整声明长度提供独立HEX字节源，Header→Offset→Length→Body；83状态内合法NST26B完整高亮，其他三页完整错误载荷/可见前缀均与源相同，缺认证/错长度/脱离帧拒绝假源 | 0 |
| M2-53 | IPv4零总长被TSO兼容值替代且原始范围缺失 | 已完成（N17有限原字节合同，最终CI归11） | fa/788保留Source0原始0000及精确两字节高亮，捕获前缀/声明范围/未确认TSO分开；52物理见证、752新参考检查及104真实worker，最终两实际Windows页36状态；不将零总长前缀当有效线上IP包 | 0 |
| M2-54 | TCP相似字节被锁定QUIC及未识别流伪装完整PDU | 已完成（N17有限承载/观察合同，最终CI归11） | caf/788拒绝默认TCP→UDP-only QUIC/H3/DoQ，显式算法API及真实UDP正控保留；75 native+34 CLI新终态、43457插桩执行、6例44实际回调/8段544B、全部379455物理包CLI及305332事件严格差分，唯一码/发表时机变化有精确映射。未知重组字节以owned observation/preview递进，未关闭六例连接政策 | 0 |
| M2-55 | 缺请求HTTP头和后续未定界数据的身份、别名及拥有来源 | 已完成（N07有限可观察性合同，最终CI归11） | c0私有头资格、64KiB有界预览/全量SHA、未知正文/上下文分开；真实45包/三个peer、54公开头216新参考检查及1102实际HTTP回调来源，1048后续观察不伪造消息/方法/状态码。82新终态全pass、10788插桩执行、两实际Windows页35状态；全部379455物理行和580 publication逐项过、flow逐字节相同。原缺方法完整划分差异保留；[完整证据和失败](shark-m2-http-orphan-observation-20261101.md) | 0 |
| M2-56 | TLS1.3 CertificateEntry内部扩展误接纳、拥有正文及逐项字节范围 | 已完成（N13有限正文合同，最终CI归11） | 0f1拒绝短头、越界和条目内重复类型，完整向量先于配置预算；公开正文/筛选别名及worker拥有完整编码，原证书摘要保留。59新终态、58636插桩执行；新实抓41包独立CV/两Finished及12实际消息验证，三坏控制由12 HTTP变0。大包唯一来源合格记录41538/帧49524变化，其余事件/行字面、两flow相同；46实际Windows状态覆盖全部860B及728/55B高亮。角色/证书类型/PHA关联不关闭；[完整证据](shark-m2-tls-certificate-vectors-20261101.md) | 0 |
| M2-57 | 普通字段视图重复诊断前缀遮住具体错误 | 已完成（有限可读性合同，最终CI归11） | 7aab分别展示结构化代码/原始理由，仅去掉一层精确代码前缀，保留不一致代码、冒号和非SSH的ssh:文本；导出不变、无虚构字节范围。10新ID覆盖9场景及直接/packet/stream三路径；630两实际Windows页44状态完整错误及75/893B目标记录读回，CP936/鼠标/缩放恢复通过。早期QUIC等专用路径和任意长消息不冒称已统一；[证据](shark-m2-tls-initial-context-and-diagnostics-20261102.md) | 0 |
| M2-58 | 认证恢复丢弃旧握手片段后误猜新消息起点 | 已完成（N13.C03有限合同，最终CI归11） | 原版在独立控制中发表类型84、接纳碰巧匹配的KeyUpdate。26c仅保留记录认证并暂停无起点证据的握手解析；三套件/双方向及同epoch持续负控、另一方向隔离保持。480b5 SDK摘要、递进详情和同包唯一提示不覆盖合法消息或改写应用文字；三实际Windows页66状态读回882/875/27B。九profile除两SDK摘要及一CLI提示外全值/来源/代次保持，六完整大包导出相同；[证据](shark-m2-tls-epoch-boundary-20261102.md) | 0 |
| M2-59 | PHA offer生命周期和连接内请求context唯一性 | 已完成（N13.P02有限合同，最终CI归11） | 保留完整ClientHello的私有offer观察至正文释放和换钥后；初始/后握手context历史拥有字节、集合/内存拒绝原子提交、新连接及域清理。新实抓41帧两轮PHA，23 tag/4 MAC/3签名/10 peer明文独立核验；旧版两坏控制均10 HTTP，新版帧16/28拒绝且前3/7 HTTP保持。五域/复用控制、九profile、三context别名及最终Windows通过；[证据](shark-m2-tls-pha-context-20261102.md) | 0 |
| M2-60 | 已认证握手错误详情回退TCP及连接诊断隐藏 | 已完成（有限页面合同，最终CI归11） | 真实Windows重现已知TLS1.3+AEAD记录详情顶部TCP，修复后仍malformed并保留连接停止说明；原始头/hash/贡献域、认证及套件缺失不借身份。独立页、同包合法消息/HTTP和流详情验收，三页66状态全文及85B明文/两107B记录读回。101新增中26为CLI，原两大捕获六导出保持；[证据](shark-m2-tls-pha-context-20261102.md) | 0 |

M2-56后续限定补充：17fe对已知服务端要求context为空、证书列表非空，保留客户端空列表、
PHA context及无角色语法API；43新ID和34实际Windows状态通过，两个坏控制不再发布HTTP，
合法原流量及两个大包六导出字节相同。[补充证据及独立CI测试修正](shark-m2-tls-certificate-roles-20261101.md)。
这不关闭完整角色/阶段、初始客户端context或PHA响应关联，N13整体仍开放。
该段为17fe当时范围。后续7aab已另补初始客户端Certificate和服务端CertificateRequest的空context条件，
并证明缺Finished时先经AEAD恢复application epoch再检查正文，不误拒绝带context的PHA。
49新ID、两源反例67字面参考检查、最终18+36运行及大包六输出字节相同；
[源、页面失败和新CI门](shark-m2-tls-initial-context-and-diagnostics-20261102.md)。完整阶段、服务端application-key Certificate、PHA offer/唯一性/关联仍开放。

后续每次报告列本表已完成/进行中/未开始数量、关闭的 ID、新增 ID 和证据链接。新增问题不能藏入既有“完成”项；估时随范围和实际失败更新。历史50%/70%额度限制已被用户后续指令替换；当前按最新授权在剩余5%时停止，提前保留提交和精确SHA CI收尾空间。最多1个子agent，主agent负责关键路径。

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

e5 完整 CI 的 pcap/parser 包已通过，失败集中在原有
`TestAuthenticationDetailsPrioritizeMeaningBeforeRawTree`：SMB 排序器错误地把无 wire-range
的认证元数据全按字母排序，使 raw Decoded Fields 早于 User Name。修复复用原有
`fieldPriority`，不隐藏 raw tree、不修改原测试；新增本地/packet/stream worker 的真实
AUTHENTICATE 来源、MIC/认证未验证和 SDK 事件不变断言。完整 CLI 本地复跑
`test-M2-full-CLI61-01` 为 **985 pass/2 skip/0 fail**，日志 SHA256
`5e7a161ff4784bf6f12e54d27e059b2ae57e3c2fcffe6d6121556025d7a2ded0`。
两 skip 为可选外部预览审计与控制台子进程入口；本阶段大包和真实 Windows 运行另有独立证据。
e5 的失败、两份全 CI artifact 和已通过的本地输入全部保留；最终提交仍须重新完成全部 CI。

来源盘点把 M2-04 拆成 19 个稳定子项（10 个已有部分实现、9 个未开始、功能验收完成 0），
与主表大项分别计数。原 1568 个缺口唯一分配；129 个仅是静态映射候选，不算修好。
另有 12 条完整记录尚未进入 269-record 正文中立 oracle，不能从 281 来源门通过推导
其全部字段正确。原始 PDML 的部分 QI/QD 响应 class 信息是跨 VLAN 请求继承装饰，
即使没有 generated 标记也不可当作响应 literal/range。保持 VLAN81 请求和 VLAN80 响应
隔离，原始整体差异保留；同域解码和类型化内容要另有同域正例与跨域负控。
