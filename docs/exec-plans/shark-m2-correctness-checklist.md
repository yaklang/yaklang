# M2 协议正确性验收清单

2026-11-03后续：[M2-85 CONNECT 隧道帧序列、晚到上下文与实际Windows](shark-m2-http3-connect-tunnel-20261103.md)。
当前85项75有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，17–34 AI实施小时。
R07四个真实来源目标有限关闭；客户端跨方向先后不可证明时显示 order-unverified，完整N09继续开放。
CONNECT运行b08完整291224pass/25历史skip、92新增；普通合并86b完整保留远端#5387 WCF framing，
合并后1369定向pass/1外部WCF样本可选skip、完整CLI1719pass/2历史skip，合并集合291382pass/26skip，不冒充一次完整86b运行。
86b实抓84保护包/16消息/28参考头对、41小源/15新参考、90实际Windows、17528插桩、六完整公共及四strict域通过。
WCF正文仍opaque，外部企业样本本机未验，不额外计作完整协议验收；SMB不分析不修复。
父4d52296c8已独立47/91/2及316231原始ID/官方产物全绿；新最终SHA须另验316482原始ID/动作、131084 required零skip及全部CI。

2026-11-03后续：[M2-84 晚到方法的响应正文复验与实际Windows](shark-m2-http3-late-body-20261103.md)。
当前84项74有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，18–36 AI实施小时。
R06四个真实来源乱序目标有限关闭；新R07成功CONNECT后的HEADERS帧序列缺口已复现并保留，完整N09仍开放。
0bb为代码/完整本地运行/实际构建同一SHA：291132pass/25历史skip、41新增；34791插桩，41小源执行/15新参考，
新16消息/56头/32帧、旧21原包/13目标、72实际Windows、六完整公共及四strict域控制通过。
e0fb已独立47/91/2及316190原始ID/官方产物全绿；本批最终说明head须另验316231原始ID/动作及全部CI。

2026-11-03后续：[M2-83 晚到HTTP3请求的有界关联和实际Windows](shark-m2-http3-late-association-20261103.md)。
当前83项73有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，18–36 AI实施小时。
N09.R05七个冻结真实来源变换有限关闭，新R06晚到HEAD方法的旧响应正文复验已复现并追加，完整N09仍开放。
生产/实际构建f1fd完整291084pass/25历史skip；后续b625只加fuzz测试，严格合并291091pass/25历史skip，49新ID均pass。
21109插桩、33新native/11新参考、21原包/13目标/四H3消息、七关联当前视图、36实际Windows和六完整公共回放通过。
最后独立全绿1505已47/91/2及316141精确ID/官方产物闭合；本项最终head须独立316190原始ID/动作和全部CI。

2026-11-03后续：[M2-82 迟到SETTINGS的QPACK恢复、原始字节与实际Windows](shark-m2-qpack-late-settings-20261103.md)。
当前82项72有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，17–34 AI实施小时。
R04当前连接有限恢复关闭，R05七个晚到请求关联及完整N09生命周期保留。解析器7825完整291039pass/25历史skip，
后续0510只改两个CLI文件，完整CLI1685pass/2历史skip；合并集合291042pass/25历史skip/0fail，66新ID均pass，
不冒充0510一次全模块运行。71334插桩、最终33小源/11新参考、21原包/13目标/四H3消息、18实际Windows通过。
最后独立全绿525已47/91/2及316075精确ID/官方产物闭合；本项最终说明head须另验316141及全部CI。

2026-11-03后续：[M2-81 有界早期恢复、同数据报关联和实际Windows](shark-m2-quic-early-recovery-20261103.md)，运行6941，50新增、290976最终完整pass/25历史skip、864定向、32642插桩。
实抓原始21/21已打开、四HTTP3消息及两个响应关联与独立来源/新参考对上，33新native/11新参考/36实际Windows；六完整公共及原四域strict保持。
当前81项71有限完成、7进行中/3SMB搁置、10未关闭；N09.R03有限关闭，R04保留，新增R05七个晚到请求关联缺口。
非SMB四包18–36实施小时。d344已独立47约定检查/91执行/2statuses、316025精确traffic ID及官方产物全绿；Java/RAG官方产物另核8新及3原失败ID各一次pass；本批最终SHA须独立316075及全部CI，父成功不认证恢复代码。

2026-11-03后续：[M2-80 CI尾部丢失和Java错误导入身份](shark-m2-ci-drain-and-imports-20261103.md)。新旧回归20轮220全pass；两个完整包5轮447唯一ID/442pass/5历史skip，0fail。
c292专项316025原始ID、85新增和131084 required已逐项闭合，但两个Essential实际失败，整体不全绿。
当前80项70有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包17–34实施小时。
新最终SHA独立全部CI；原始先到0-RTT恢复原型15项全红留仓库外，N09.R03/R04继续开放。

2026-11-03后续：[M2-79 QUIC早期密钥、包号顺序和实际Windows](shark-m2-quic-early-order-20261103.md)。
运行e714，85新增、290926最终完整pass/25历史skip、两个真实复用端点连接/21独立AEAD、
33新native执行、11新参考、605字面检查、40568插桩和36实际Windows状态通过有限合同。
当前79项69有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包17–34实施小时。
父776已独立47/91/2及315940 ID全绿；本批最终head须另验316025。
原始先到0-RTT仍有1缺密钥及1 H3上下文缺口，新增N09.R03/R04明确跟踪，不算修复。

2026-11-03后续：M2-78补RFC8773空标志/CH-SH配套/提议与EE位置，以及完整拒绝Hello的拥有来源展示，
[224新增、114小源执行、20新参考和94实际Windows状态](shark-m2-tls-certificate-psk-20261103.md)。
当前78项68有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包仍16–32实施小时。
e31已独立47/91/2、315716 ID及官方产物全绿；bb的最终说明head另验315940，不沿用父成功。
本批保留8个组合参考失败，完整RFC8773/HRR/其他位置、TLS/QUIC恢复、六fuzz及IP边界仍开放。

2026-11-03后续：M2-77补缺ClientHello时ServerHello自足字面约束及列表/详情/原规则树一致展示，
[186新增、真实来源残缺控制、126小源执行、22新参考和84实际Windows状态/精确字段高亮](shark-m2-tls-server-hello-observation-20261103.md)。
当前77项67有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包仍16–32实施小时。
4153已独立47/91/2、315530原始ID及官方产物全绿；最终head另验315716。8个本批组合参考失败、
完整阶段/扩展依赖、重组边界及六fuzz TCP政策继续开放，不将同ISN复制样本冒充真实新generation。

2026-11-03后续：M2-76补普通已选中PSK的初始证书认证拒绝，
[134新增、四真实连接/独立38 AEAD/3签名/8MAC、138小源执行、30参考和88实际Windows状态](shark-m2-tls-psk-authentication-20261103.md)。
当前76项66有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包仍16–32实施小时。
f858已独立47/91/2及315396原始ID全绿；本批最终head另验315530。完整P04、RFC8773配套/位置、
16个本批组合参考失败及1个无效DER参考字段差异保留，不因TLS13/PSK标签正确就关闭。

2026-11-02后续：M2-75补齐已知TLS1.3客户端认证块重复/倒序及空证书后CV拒绝，
[119新增、独立2签名/2MAC、102小源执行、22参考和66实际Windows状态](shark-m2-tls-client-order-20261102.md)。
当前75项65有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包仍16–32实施小时。
dccafe已47/91/2和315277原始ID全绿；本批最终head另验315396，完整P04和12个组合参考失败仍开放。

2026-11-02后续：M2-74/N09.R01补完整未识别UDP的拥有载荷，
[46新增、8119原始来源/4833扩展、原QUIC四严格门和实际Windows](shark-m2-udp-full-source-20261102.md)。
当前74项64有限完成、7进行中/3SMB搁置、10未关闭；非SMB四包仍16–32实施小时。
97112已47/91/2和315231原始ID全绿；本批最终head另验315277及全部CI，N09整体仍开放。

2026-11-02后续：M2-73补已知服务端初始TLS1.3消息重复/倒序拒绝，
[246新增、新实抓36帧/独立源、新参考、六域控制和实际Windows](shark-m2-tls-server-order-20261102.md)。
当前73项63有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，16–32实施小时。
父add08已47/91/2及314985原始ID全绿；本批最终head独立验收315231原始ID和全部CI，完整P04仍开放。

2026-11-02后续：M2-71补齐EE拥有正文/字面字段/来源和递进展示，M2-72修复首个EOF统计陈旧，
[59新增、独立AEAD/新参考、六完整回放及实际Windows](shark-m2-ee-fields-and-eof-statistics-20261102.md)。
当前72项62有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，16–32实施小时。
最新main d705经388普通合入，M1已合并；父a796已47/91/2及314926原始ID全绿，新最终head另验314985及全部CI。

2026-11-02后续：M2-70修复EE错误位置/版本和未提议ALPN污染状态，
[102新ID、真实源故障/复用、66实际Windows状态及大包回放](shark-m2-tls-encrypted-extensions-20261102.md)。
当前70项60有限完成、7进行中/3SMB搁置、10未关闭；N13.P04从未开始改进行中，完整阶段仍开放。
非SMB四包仍16–32实施小时。父912已47/91/2及314824原始ID全绿，本批最终head独立314926及全部CI。

2026-11-02后续：M2-69/N14.U02修复通用IPv4 options偏移和物理片段误分发，
[304新ID、Windows新实抓45包/906检查、76公开见证/829检查、实际递进页面](shark-m2-ipv4-header-dispatch-20261102.md)。
当前69项59有限完成、10未关闭；N14整体仍开放，非SMB四包剩余粗估16–32实施小时。
父b603已47/91/2及314520原始ID全绿，新最终head另验；既有断言保持，原native-plan失败已修复。

2026-11-02后续：M2-68修复raw位字段被误显示为字节容器，
[145新ID、17物理引用的新参考/34 worker、六完整导出及40实际Windows状态](shark-m2-packed-field-display-20261102.md)。
当前68项58有限完成、10未关闭；N14.U02新增合法IPv4 options偏移故障并继续修复，U03位字段有限关闭。
非SMB四包仍开放，含新U02粗估17–35实施小时。父422已47/91/2及314375原始ID全绿，新最终head另验。

2026-11-02后续：M2-67补完整TCP异常SYN组合的物理拒绝诊断，
[315包逐项参考、完整六导出和40状态实际Windows](shark-m2-tcp-invalid-flags-20261102.md)。
当前67项57有限完成、10未关闭；N10仍开放，非SMB四包16–32实施小时。父220完整CI已独立闭合，
新最终head另验314375原始ID及全部CI；25新增CLI项全须pass。

2026-11-02后续：M2-66/N11修正原24组UDP分区的混层报告错误，
[9066原包逐位置参考、42新ID、覆盖插桩和真实DNS域控制](shark-m2-udp-oracle-layer-20261102.md)。
当时66项中56有限完成、10未关闭；非SMB17包13有限完成/4开放。
本项不修改解析器/TUI，不宣称所有UDP应用会话正确。已发布3d2已全部CI/314308原始ID闭合，新head另验。

2026-11-02后续：M2-65补齐三个真实异常option父头观察，保留strict/loose原错误与计数，
[逐字段来源、147控制执行、完整大捕获及六页实际Windows](shark-m2-ipv4-option-observation-20261102.md)。
当时65项中55有限完成、10未关闭；N14.U01有限关闭，N14整体仍开放。
已发布93c检查点已47/91/2及314266原始ID全绿；本批最终head另验314308精确ID及全部CI。

2026-11-02后续：M2-64拒绝零长度IPIP父层借归一化建立DNS/TCP或污染共享fragment状态。
[21来源控制、147次执行、完整字段/发布差分及27个实际Windows状态](shark-m2-ipip-parent-extent-20261102.md)。
当时64项中54有限完成、10未关闭；当时N14.U01三个option缺口后由65有限关闭。

2026-11-02后续：新增M2-63修复零长度IPv4片段误供重组、推进时钟和隔离预算的入口。
[实际DNS故障控制、完整大捕获分账、Windows及fuzz证据](shark-m2-ipv4-fragment-extent-20261102.md)。
当时63项中53个有限完成、10未关闭；非SMB五个开放包粗估20–40 AI实施小时。

2026-11-02后续：N08完成有限被动连接政策/来源验收，原strict72的8差异保留；
新增M2-62修复真实Windows HTTP起始行排在正文后面的问题。
[40次原生/参考执行、独立旧代来源、54个新Windows状态及完整大捕获](shark-m2-tls-old-generation-and-http-display-20261102.md)。
当时62项中52个有限完成、10未关闭；非SMB17包中12有限完成、5开放，粗估22–44 AI实施小时。

2026-11-02此前：M2-61 完成有限 PHA 响应关联/顺序、晚到请求和剩余预览页面合同。
[真实来源、8 个捕获故障、域隔离、6 页 Windows 和剩余恢复差距](shark-m2-tls-pha-response-20261102.md)。
运行代码 e7f8c47，后续 42fc9e 仅补独立 fuzz 模型及最小种子；仍在同一 M2 PR #5371。
同时复核到 M1 已有有界 IP 重组基础，M2-45 改为现有基础的差分验收进行中，未重复算新实现。

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
当前状态：**79 项，已完成 69，进行中 7，未开始 0，按用户要求搁置 3；剩余 10 项（含搁置）**。
各项完成范围以表中有限合同及对应证据为准，不含进行中的 M2-11；不代表核心协议或整个 SMB profile 验收完成。
随后已明确新增、关闭59的PHA offer/已观察context合同和60的认证后握手诊断身份/连接说明合同。
原宽口径剩余10项保持；非SMB现17包中13有限完成、4开放，粗估17–34 AI实施小时，构建/CI/回放等待及新发现另计。
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
| M2-09 | QUIC 与 HTTP/3 高级恢复边界 | 进行中（R01–R06有限关闭，R07开放） | 84补晚到方法的旧响应DATA/FIN复验、原始字节/观察号、合法当前请求隔离和递进展示；四个新实抓来源变换通过，41新ID/34791插桩、41小源/15新参考、新16消息/56头/32帧、72实际Windows和原四域strict全0。成功CONNECT后HEADERS的已知/晚到方法两个失败叶子已另列R07，完整CID/epoch/STREAM/QPACK/0RTT与remembered SETTINGS仍待独立验收。[84证据](shark-m2-http3-late-body-20261103.md) | 5–10小时；等待和新问题另计 |
| M2-10 | 剩余协议、字段、会话及 fuzz 差异转为逐项验收 | 进行中 | 非SMB N01..N17共13有限完成/4未关闭；N11原24组混层比较错误已逐位置核实，9066包未建立应用会话，不代表所有UDP会话完成。N08有限政策/来源及N14的63/64/65入口合同保持；原133项解释、6 TCP政策、N13完整阶段、N09完整生命周期和新R07隧道帧序列及N14高级字段/恢复仍保留 | 当前4包粗估18–36 AI实施小时，不含CI/构建/回放等待及新问题 |
| M2-11 | M2 交付的完整回归与精确 SHA CI | 进行中 | e0fb已47/91/2、316190精确Traffic ID及官方产物闭合。本项0bb一次完整291132pass/25历史skip、41新增；34791插桩、41小源/15新参考、72实际Windows和六完整公共回放通过有限合同。最终说明head须独立316231原始ID/动作、41新pass、131084 required零skip及全部CI；不沿用父成功或draft skip | 30–90分钟实施，执行等待另计 |
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
| M2-43 | TLS身份资格分层与CertificateRequest正文 | 进行中（正文/字段/拥有源及已观察关联有限通过，阶段/恢复仍开放） | CR来源/正文及QUIC CRYPTO证据保留；46/47关闭Finished/CV正文，48..52关闭QUIC有限资格与实际页面，59/61关闭已观察offer/context及多轮响应关联有限合同。完整阶段、协商依赖/传输参数、压缩证书及缺握手高级恢复仍开放。损坏Certificate后CV边界不可证的缺口保留，不将源oracle核验说成产品验证 | 扩展后的后续实施4–8 AI小时，执行等待另计 |
| M2-44 | 多个完整TLS PDU挤掉后续具体正文摘要 | 已完成（本地及真实页面有限合同，最终CI归11） | 实际282页正文正确但列表缺请求，286红2pass/4fail；4f7仅对已校验CR/认证KU前移，同record及跨PDU均覆盖，不改原详情顺序/错误/应用身份。288完整CLI1093pass/2历史skip；292两页36实际Windows状态、完整明文及精确高亮/缩放/CP936；新字段与publication全部按原生ID核对 | 0 |
| M2-45 | 现有IP碎片重组的来源、字段与大捕获差分验收 | 进行中（M1有界基础；来源分账及63/64/65有限入口通过） | 423候选key/398完整byte-set不是全部恢复证书。65关闭N14.U01三option观察；264103已有事件完整字段保持，新增3具体诊断至264106，328124候选外CLI行/442独立发布及两个flow保持；完整字段/transport代次、IPv6/其他封装、冲突/缺片/预算恢复仍开放。[最新有限证据](shark-m2-ipv4-option-observation-20261102.md) | 剩余6–12 AI小时；CI/回放等待及新问题另计 |
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
| M2-61 | PHA响应观察关联、跨方向晚到请求与未解析余段展示 | 已完成（N13.P03有限观察合同，最终CI归11） | e7私有有界context/响应顺序、空证书/重复/孤儿及原子预算；每个TCP贡献ACK前缀不齐时保留未知，晚到请求关联且旧事件不改。8个真实源派生捕获旧误报8→0，6个晚到捕获HTTP恢复10，2永久缺失保留6；两组相同tuple/context跨域10/6不串。163次小捕获/参考导出、6页162 Windows状态及62,136次九种子模型fuzz通过；四包280304pass/23历史skip；六完整大捕获稳定。客户端认证记录永久丢失仍4/7差距，完整阶段和协商仍开放；[范围及证据](shark-m2-tls-pha-response-20261102.md) | 0 |
| M2-62 | HTTP起始行晚于正文、状态行内部顺序与线序不符 | 已完成（有限展示合同，最终CI归11） | d46只改已解析HTTP包装层：FirstLine→Headers→Body→Trailers，请求Method/Path/Version，响应Version/Status/Message；正文同名字典和其他协议不受影响，SDK字段/原字节/重复头保持。真实红测4fail→7新ID全pass，完整CLI/CI脚本1362pass/2历史skip；两个实际Windows页54状态、82/1129B完整读回和两完整大捕获CLI输出相同。[N08来源及页面证据](shark-m2-tls-old-generation-and-http-display-20261102.md) | 0 |
| M2-63 | 零长度IPv4片段借捕获长度进入重组和隔离预算 | 已完成（有限入口、来源和展示合同，最终CI归11） | b80先拒绝，6379明确byte单位；真实红测13fail、50新ID全pass、四包280362pass/23历史skip、最终32969次插桩fuzz。14实际DNS来源控制70次新旧/双TSO参考、所有RR opaque bytes/四alias/贡献源验收；Windows原异常帧20状态精确高亮原00 00。完整六导出及264101事件分账、所有物理行/非拒绝标签、发布和flow已核；[证据与默认TSO差异](shark-m2-ipv4-fragment-extent-20261102.md) | 0 |
| M2-64 | 零长度IPIP父层归一化后误建立内层及状态 | 已完成（有限父层、来源和展示合同，最终CI归11） | 6ef先拒绝协议4/41完整观察父头再接纳内层，严格子错误保留。37新ID、280399pass/23历史skip、34134插桩执行；21来源控制147运行、953断言/967参考字段及同tuple/共享fragment ID/接口/VLAN恢复通过。六完整导出、264094不受影响原生事件及328115 CLI行/442独立发布完整字段保持，两个flow不变；Windows27状态读回1764B并精确高亮0000。当时另列N14.U01三个option缺口，后由65有限关闭；[边界及证据](shark-m2-ipip-parent-extent-20261102.md) | 0 |
| M2-65 | IPv4 option解码错误后漏父头诊断及字段零值误用 | 已完成（N14.U01三原帧有限合同，最终CI归11） | a9031从完整字面头保留版本/ID/next及原option错误，拒绝内层及连接；42新ID、341相关回归/零skip、280441完整pass/23历史skip、47454插桩执行。16严格/宽松API运行保持原错误/计数；147控制、968断言/967参考字段、两大包六导出及所有其余字段保持；3原包60基础Windows状态及3页滚动错误/精确原字节范围通过。d8仅同步统计API内嵌文档；[证据与限制](shark-m2-ipv4-option-observation-20261102.md) | 0 |
| M2-66 | UDP参考多层stream被混算外层会话差异 | 已完成（原N11的24组报告归因，最终CI归11） | 976e独立原帧/地址/端口/四头字段范围选择PDML层，拒绝缺帧/歧义/错源，无部分成功输出。42新ID、43受影响pass/零skip、382630覆盖插桩；完整原捕获新参考/9066位置核对、9018参考头错误保留，48只称长度一致，所有原生事件均无应用FlowID。三DNS域正控和三个实际拒绝通过；不改解析器/TUI、不验收全部UDP会话。[范围和来源](shark-m2-udp-oracle-layer-20261102.md) | 0 |
| M2-67 | 完整TCP异常SYN组合已有核心拒绝但页面无解释 | 已完成（限定物理诊断/来源/展示合同，最终CI归11） | 469仅对字面SYN+FIN/RST完整头补同一诊断，列表/quick/detail/worker原始范围一致，清除陈旧连接展示。25新ID、110相关/1481完整受影响pass、2历史skip；六完整导出native/flow均相同，315物理行仅六展示键变、4725参考字段及167零IP字面值保持。原145238旧新实际40状态/241B完整读回，[46,48)精确50fe。N10完整政策仍开放。[范围和证据](shark-m2-tcp-invalid-flags-20261102.md) | 0 |
| M2-68 | raw位字段逻辑值与原字节容器混淆 | 已完成（N14.U03限定展示合同，最终CI归11） | ddd以类型/确切span/长度/padding限定bit展示，不改raw合同；145新ID/1626受影响pass，2历史skip；17原引用/10不同字节的新参考和34 worker只变预期Flags文本；六完整导出逐字节不变，旧新实际40 Windows状态精确高亮5b00。新N14.U02合法IPv4 options偏移另行修复。[范围及原失败](shark-m2-packed-field-display-20261102.md) | 0 |
| M2-69 | IPv4 options未消费及物理片段误作完整子协议 | 已完成（N14.U02限定包络/分发合同，最终CI归11） | 83f4先校验本层Version/IHL/总长/options TLV；片段留给owner，原生plan未经资格不产生副作用。304新增全pass、289623完整pass/24历史skip；35050插桩；双大包六导出及新实抓三导出全字节相同。Windows新实抓45包/16 options、906参考检查及6对HTTP/3握手关联；76公开见证829检查、242 worker、20实际递进页面状态。U02有限关闭，N14整体仍开放。[全部证据/原失败](shark-m2-ipv4-header-dispatch-20261102.md) | 0 |
| M2-70 | EE错误位置和未提议ALPN污染协商状态 | 已完成（有限消息资格合同，最终CI归11） | 4c2e拥有CH提议并校验版本/方向/epoch/已知位置及预算，拒绝原子且记录认证保持。102新增、289727本地pass/24历史skip；36控制执行/84复用引用、22字面参考、38752插桩、六完整导出及66实际Windows状态。两个接口组合参考和两组QUIC app-only严格差异保留，P04整体进行中。[限定范围/原失败](shark-m2-tls-encrypted-extensions-20261102.md) | 0 |
| M2-71 | EE正文、字面alias、拥有来源及递进字段 | 已完成（有限字段/展示合同，最终CI归11） | a74/868解析完整向量和ALPN，保留opaque其他Data、UTF8/raw分离；53新ID及双插桩43036/41362。独立公共25B与本机21B AEAD/新参考一致，306855事件/379926行仅一个EE事件/一行有资格化变化，24原生控制及六最终完整回放保持；实际Windows精确高亮，完整P04仍开放。[证据](shark-m2-ee-fields-and-eof-statistics-20261102.md) | 0 |
| M2-72 | 首个EOF协议统计停在旧采样窗口 | 已完成（N14.U04有限物理窗口合同，最终CI归11） | cf立即刷新最终128物理包、保留live间隔/计数和晚到观察；6新ID/1615最终CLI pass、2历史skip。完整新参考ESP78/IKEv2 50，实际Windows首EOF与搜索后均61%/39%；六完整导出逐字节不变。[证据](shark-m2-ee-fields-and-eof-statistics-20261102.md) | 0 |
| M2-73 | 已知服务端TLS1.3握手消息重复/倒序仍被接纳 | 已完成（有限观察子序列合同，最终CI归11） | cd/d853仅比较认证后已接纳消息，不因缺前缀拒绝；状态/预算拒绝不推进，PHA及未知角色独立。246新增、290132完整pass/25历史skip、2617最终相关pass、33766插桩；48最终小回放、36帧新实抓/30来源、六完整公共导出、44实际Windows状态。组合接口/VLAN与QUIC strict差异保留，P04整体开放。[完整证据](shark-m2-tls-server-order-20261102.md) | 0 |
| M2-74 | 完整未识别UDP仅保留64B probe导致来源和页面缺字节 | 已完成（N09.R01有限字节来源合同，最终CI归11） | b49/26f只在有效且未超消息预算的未知UDP拥有完整载荷，协议/拒绝和连接不提升；46新增、290178完整本地pass/25历史skip、71701插桩、新12帧/参考、8119源资格/4833扩展、六完整公共导出及22实际Windows状态通过。原四组QUIC checker全部0，N09高级语义仍开放。[完整证据](shark-m2-udp-full-source-20261102.md) | 0 |
| M2-75 | 初始客户端TLS1.3认证块重复/倒序及空证书后CV误接纳 | 已完成（有限观察合同，最终CI归11） | fbda只比较已认证/已知客户端/初始epoch的接纳观察，私有空列表事实、试算提交和PHA独立；119新增、290297完整本地pass/25历史skip、70717插桩。原始8 AEAD/2签名/2MAC、102小源运行/504域引用、22完整参考、26 native字面组、六完整公共和66实际Windows状态通过；5零context格式差异及12组合参考失败保留，完整P04仍开放。[证据](shark-m2-tls-client-order-20261102.md) | 0 |
| M2-76 | 普通已选中PSK仍误接纳初始证书认证消息 | 已完成（有限模式合同，最终CI归11） | e700私有完整SH观察，不把仅offer/缺上下文/端口当选择，缺CH/gap保留证据，拒绝原子、PHA及RFC8773保守例外独立。134新增、290431完整本地pass/25历史skip、38299插桩；四真实连接38 AEAD/3签名/8MAC，138小源执行、24域profile/504引用、30新参考、88实际Windows状态、六完整公共导出及四QUIC原严格门通过。9空context省略、1无效DER值和16组合参考失败保留；RFC8773配套/位置与完整P04开放。[证据](shark-m2-tls-psk-authentication-20261103.md) | 0 |
| M2-77 | 单侧ServerHello非法legacy参数被接纳及拒绝记录展示互相矛盾 | 已完成（有限字面/拥有来源合同，最终CI归11） | b611自足字段不依赖CH、错误不提交协商；98fd无状态完整record观察及声明/未协商分离，列表/详情/原规则树一致且真实字节范围。186新增、290617合并本地pass/25历史skip、双插桩72955/74693；126小源、12不同真实ISN域profile/204引用/12 owner-only诊断、22参考和84实际Windows状态通过。8组合参考失败、同ISN歧义和完整P04仍开放。[证据](shark-m2-tls-server-hello-observation-20261103.md) | 0 |
| M2-78 | RFC8773错误Hello仍改变PSK模式及完整拒绝记录被显示为缺握手 | 已完成（有限标志/配套/EE及拥有展示合同，最终CI归11） | c4先验空标志、CH/SH配套、已知提议及EE位置，bb有界外层观察统一CH/SH列表/详情/规则树与自有范围；224新增、290841合并本地pass/25历史skip、74312/43399插桩。114小源、12不同真实ISN域profile/216引用、72坏CH ACK不粘TLS、20参考、94实际Windows、六完整公共及四QUIC严格门通过；8组合参考、完整RFC8773/HRR与P04开放。[证据](shark-m2-tls-certificate-psk-20261103.md) | 0 |
| M2-79 | 原生漏早期密钥及0-RTT/1-RTT跨层PN顺序误接纳 | 已完成（N09.R02有限合同，最终CI归11） | e714按当前random/已观察suite加载可选早期secret，不反复覆盖应用epoch；私有生命周期PN界限先于duplicate，双方向、拒绝原子及v1/v2三套件通过。85新增、290926最终完整pass/25历史skip、21真实源AEAD、13变换目标/605字段、40568插桩、33新native/11新参考和36实际Windows；六完整公共及原四域strict保持。原始先到0-RTT与迟到SETTINGS恢复仍开放，未将导出相同算修好。[证据](shark-m2-quic-early-order-20261103.md) | 0 |
| M2-80 | CI中RAG排队尾部丢失和并发Java显式导入错误本包身份 | 已完成（有限根因修复，最终CI归11） | 由输出方排空后取消内部管线；显式导入fallback只移除错误本包猜测。新增8 ID先红后绿，旧断言不变；20轮220全pass、两完整包5轮442pass/5历史skip；不改Shark解析/ZIP/工作流。[证据](shark-m2-ci-drain-and-imports-20261103.md) | 0 |
| M2-81 | 先到0-RTT未恢复及同数据报内丢请求响应关联 | 已完成（N09.R03有限合同，最终CI归11） | 6941在当前owner/域内有界拥有密文/来源，观察完整接纳后新PDU重处理，原贡献15与上下文16分开；拒绝/坏tag/预算/关闭不交付，晚secret不重装epoch。握手后预留剩余wire预算并在响应前恢复请求。50新增、最终完整290976pass/25历史skip、864定向、32642插桩、21原包/四H3消息及两个响应关联、新参考/36实际Windows、六完整公共及原四域strict通过；R04及新R05保留。[证据](shark-m2-quic-early-recovery-20261103.md) | 0 |
| M2-82 | 迟到SETTINGS的QPACK未恢复及原指令字节/展示资格 | 已完成（N09.R04当前连接有限合同，最终CI归11） | 4024/7825/0510区分未知与已知零、恢复已拥有后缀，事务性表项/资源传播、其他STREAM和原始见证裁剪；页面独立源并拒绝说明树同名字段借高亮。66新ID全pass；解析器完整291039pass/25历史skip、后续完整CLI1685pass/2历史skip，合并291042pass/25历史skip；71334插桩，33新小源/11新参考、21原包/13目标、四H3消息/两关联、18实际Windows及六完整公共/原四域strict通过。R05和完整N09仍开放。[范围与证据](shark-m2-qpack-late-settings-20261103.md) | 0 |
| M2-83 | 晚到HTTP3请求无法补充已发布响应的关联及缓存/展示不一致 | 已完成（N09.R05头关联有限合同，最终CI归11） | f1fd原生私有关联观察、同Flow/域的有界历史视图及revision；不改原事件/ResponseTo/字节来源。b625只加fuzz，49新ID/合并291091pass与25历史skip；21109插桩、七真实来源变换、33新native/11新参考、21原包/13控制/四H3消息、36实际Windows和六完整公共通过。R06晚到HEAD正文复验新缺口明确保留，N09整体不关闭。[M2-83 晚到HTTP3请求的有界关联和实际Windows](shark-m2-http3-late-association-20261103.md) | 0 |
| M2-84 | 晚到方法未复验先前HTTP3响应正文及DATA/FIN来源 | 已完成（N09.R06已观察正文有限合同，最终CI归11） | 0bb在原生同owner/域/STREAM中保留有界DATA与wire FIN见证，补HEAD/CONNECT/FIN/缺口复验，不改原响应/字节、不误伤合法请求；41新ID、一次完整291132pass/25历史skip、34791插桩。新实抓原样本/三控制88保护包比较、16消息/56头/32帧；总41小源/15参考、72实际Windows、六完整公共和四strict域通过。R07隧道HEADERS两个失败叶子保留，完整N09仍开放。[84范围与证据](shark-m2-http3-late-body-20261103.md) | 0 |
| M2-85 | 成功CONNECT误接纳HEADERS及晚到类型证据/字节源 | 已完成（N09.R07有限合同，最终CI归11） | 类型varint先于长度/QPACK检查，连接0x105与首错保留；跨方向不可证明顺序保持未知，原事件/当前合法请求不改；有界同owner/域历史及九context别名。92新ID、b08完整291224pass/25历史skip；86b合并WCF后定向与完整CLI通过，合并集合291382pass/26skip（1新外部WCF可选skip）。新实抓/三控制84保护包、16消息/28参考头对、四坏目标/四合法复用对照；17528最终源插桩、90实际Windows、六完整公共及四strict域通过。完整N09仍开放。[85证据](shark-m2-http3-connect-tunnel-20261103.md) | 0 |

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
