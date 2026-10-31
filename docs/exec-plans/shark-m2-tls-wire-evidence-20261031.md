# M2 TLS 原始字段、递进页面与恢复边界检查点

本批代码/Windows/回放/fuzz 构建为 `82fc8ec2f101ef716a715f1f54e8fbf7ebcf701e`，
分支 `codex/shark-m2-tls-wire-evidence`，基线为已 ready 且精确 SHA CI 全绿的
[#5378](https://github.com/yaklang/yaklang/pull/5378) `174e141a46d59ef065dc23356704c06666ea8e77`。
后续报告提交不改变这些已运行二进制；最终交付 SHA、四项完整 CI、实际 checkout 与
下载 artifact 的验收另记录在 PR，旧 SHA 的成功不代替最终提交。

本批关闭 M2-37 的有限合同。M2-08/09 的高级 TLS/QUIC、M2-04 的全部 SMB 字段、
M2-36 的其他 SMB1 正文仍开放。相同输出或计数不等于与 Wireshark 全部正确性一致。

## 修改及可验收边界

- 原生 TLS `DisplaySnapshot` 的 `tls.record.wire_type/version/length` 分别拥有
  `[0,1)`、`[1,3)`、`[3,5)`；TLS1.3 `opaque_type` 拥有外层 `[0,1)`。
  必须由私有已接纳头字段与当前 Raw 的 type/version/length 一致，且记录声明长度完整。
  认证、内层 content type、协商版本及 cipher 不借用外层记录头范围。
- 不向 `GetFields`/Session 增添一套范围元数据，不改变原字段树、DTO schema 或拒绝语义。
  完整但 codec 拒绝的记录仍可提供已接纳 literal 头；无私有头的 framer 拒绝不提供此范围。
  公开缓存被编辑不能覆盖私有值；Raw 头被编辑或截断不能沿用旧范围。
  此为头部与当前 Raw 的一致性合同，并非对可编辑公开 Raw 全文重新认证。
- TLS 页面递进为 `Record header` → `Record decoding` → `Observed connection`；
  原 `Session Establishment` 仍保留。保留每个值、字段来源和剩余字段，不因分组丢内容。
  TLS1.3 的 wire legacy `0x0303` 与观察到的协商 `0x0304` 分开。
- Stream 起点不再漏掉完整 ClientHello/ServerHello。TLS envelope/encrypted 只有通过
  原始 domain、物理 TCP 代次、方向端点、offset 0、完整 Raw/前缀/hash、完成来源、
  成功字段查询以及可信 TLS 记录检查后才显示 `Validated TLS record PDU`。
  连接标签、短密文、错误/残缺记录、跨域 ref、解密子 PDU 不能冒充已验证起点。
  起点标签说明 record/Handshake，不以连接版本冒充当前记录的协商或认证证据。

## 本地原断言、失败与 fuzz

原样本 ZIP SHA256 `0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`，
rules SHA256 `246c5d57806597efc0be6e7fd63e734cdd6659f0c0f13d8a60fb409cc34c8391`，
均未修改。Go 1.22.12 windows/amd64；所有 Go、SDK、UI 顺序运行并核对 HEAD/index/输入，
构建/普通测试最多 3 CPU/2GiB，fuzz 1 CPU/768MiB，实际 UI 1 CPU/512MiB。

| 证据（均在仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`） | 实际结果 |
|---|---|
| `test-M2-TLS-header-red80-01` | 新范围断言初始 19 fail；没有范围的真实缺陷 |
| `test-M2-TLS-projection80-03` | 在 Session 加元数据的候选触发 825 个旧断言失败；该实现全部撤回 |
| `test-M2-TLS-projection80-04` | 只修字段投影后原选定矩阵 3257 pass /0 skip /0 fail |
| `test-M2-TLS-prefix-full90-02` | 完整 CLI 1045 pass /2 历史 optional skip /0 fail |
| `test-M2-TLS-QUIC-final92-01` | 干净 82fc 的 TLS/QUIC/起点/详情回归 3557 pass /0 skip /0 fail；48.61s |
| `fuzz-M2-TLS-header91-01` | 真正覆盖插桩构建，90s，38400 execs、26 new interesting、0 fail |

上述矩阵相互重叠，不相加。CLI 两个历史 skip 不计为 Windows 验收；另有实际控制台验证。
fuzz 经过原生 PEAP 携带 TLS、分两段输入、关闭/预算回收，核对完整头、derived/context 无
假范围及公开字段投毒不影响结果；不是仅 fuzz 一个格式化函数。完整外部 corpus 留在原目录。
最终92测试日志 SHA `cd0bfe76f3b5bea4f47b8541795bbaf080d9efe3fd9b45efb645085b4b38aaa4`；
fuzz汇总 SHA `cd306e9bf8f02151b4bf97bcce3e302a5baeced25c7cecac00e618866c1f2c8d`。

## 真实 TLS 的缺口、乱序与代次

采用原始自有真实捕获：226 physical frames、63799B、DLT_NULL、纳秒时间，
SHA `80b46205a38a11f61f0698d01d953b04e6428cebf54989ce9925911497c2cfdb`。
八个输入只按冻结 ledger 删除/重排完整原始记录，字节/caplen/wirelen/ns 保持；不是宣称
在真实端点制造网络丢包。双独立 Python AEAD/端点 body hash 为内容来源，TShark 4.6.9
使用隔离配置、完整输入、显式 TLS 端口与乱序重组。冻结72/77检查器在读取候选前制作。
本轮实际重新运行 82fc；参考12+4次完整 TShark 运行沿用已冻结结果，未冒称本轮又跑参考。

| TLS1.2 / TLS1.3 场景 | 实际结果 |
|---|---|
| 永久缺段，各一 | 各132 TLS、24 GET/21响应、一个显式洞；严格 source/字段/回调门均通过 |
| 同代晚到补齐，各一 | 各136 TLS、24 GET/24响应；严格门均通过 |
| 新 SYN 后、匹配 SYN-ACK 前晚到旧段 | TLS1.3严格通过；TLS1.2保留 strict72 exit4、8项差异 |
| 匹配新 SYN-ACK 后、最终 ACK 前晚到旧段，各一 | 各132 TLS、24 GET/21响应、一个显式洞；严格77门均通过 |

严格72的 TLS1.2 C1 差异为多4 TLS、多3 HTTP及预期洞1/实际0。检查器的“看到新 SYN
就丢弃旧历史”是源计划中的保守策略，不能直接视为唯一协议正确答案。补充审计92重跑
独立 Python AEAD/peer，确认这7个事件均属于旧 native Flow2/TCPFlow2，非下一代3；
全部原字节/hash/方向/refs/父子/response_to/三个返回体与来源一致，并有下一代密钥拒绝
旧密文的 InvalidTag 负控。其真实回调均在 Feed57，不早于旧方向字节首次完整可用的57。
TLS close_notify也独立认证。保留 strict72 非通过，未通过删除真实旧内容来凑参考计数。

`CompletedIn` 是字节贡献位置，不等于分析器真正回调时间。本批 observer 对每个实际
Feed/Close 记录 callback phase/frame；不以 CompletedIn45/49/51冒充实际到达57。
参考对 C1 也有流分配策略差别；本批不声称 Wireshark 有 bug，也不以该样本决定所有
RFC 端点状态。见 [TCP 接收处理](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.10.7.4)。

82fc严格汇总：`runtime-M2-TLS-gap91-01/summary91.json`
SHA `81a0fa364442276a73fbee5769d2eeba789912b0f868c8cdde6827b67bb10d0e`；
confirmed汇总 SHA `ac841f98e37ff5c356d77784bf3c4118053dd5c4a0f09da7b5a3a4a26a0c8b3f`。
审计92基于8f的实际输出，摘要 SHA `4178c86a04864d20395ae2a2b6ccdffe035d1b03be1b0e5fcb5ff4e2daabd5d9`；
主线程另核对82fc C1的完整events及真实arrival trace分别与8f逐字节一致，未改写其来源SHA。

## QUIC 同 CID 的接口/VLAN 隔离

来源为真实2018帧捕获及独立 source index：2025个受保护观测、2022认证、3个真实坏tag，
三个连接/三种cipher/双向epoch0/1/2、CID轮换、NAT及乱序来源已冻结。四个完整PCAPNG
输入保留原L3/UDP/QUIC/ns；分别是接口/VLAN80与81下 full/full、full/app-only。
此为受控克隆隔离矩阵，不把克隆域冒称新的独立真实连接。

82fc实际四次SDK回放及原严格73检查：full/full两项均通过，每域2022认证、3坏tag不产
PN/明文、6个tail，4556个frame scalar/byte guards，各域流独立。app-only两项仍严格
exit4、各2664差异；保留该结果，不宣称无握手域已正确解析全部QUIC。
独立补充来源核验各2007个UDP预览：1332个明确截断为64B、675个完整短payload，全部
字节/域/来源准确，未借另一域握手、QUIC/H3/TLS字段、PN或认证结论。严格检查器期待
完整PDU造成的重复来源差异不能直接称2664个协议误识别；预览核验也不能代替完整解析。

严格91摘要 SHA `c1d3a8ffd3524e2159b72539fb6adf5025182c26795532bdaee86473f7ed196c`；
有界预览来源摘要 SHA `85507bff5533118140e54991ded0bb8e063b66b2903807ef97a0aebb4b3968e6`。
参考沿用73冻结的12次隔离 TShark 实际运行。缺开头、0-RTT、KeyUpdate、STREAM/QPACK等
完整高级恢复仍归M2-09，不由以上有限隔离门宣布完成。

## 大捕获回归与仍开放的 SMB 字段

外部 Weber `The-Ultimate-PCAP.pcapng` SHA
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`。
82fc重新执行SDK/flow/Yak完整回放：42223原生事件、51328包行、29 capture-close发布行。
与已验9d构建比较，唯一变化是4404完整TLS PDU的3个literal范围，另965个opaque_type范围；
2639个直接TCP、47个RADIUS携带、1718个RDPUDP携带TLS均覆盖。其余4个TLS为不完整
序列洞事件，不发布假头范围。SDK与CLI的每个变化按ID、完整source SHA及snapshot精确
连接；同时检查`pdus`与`published_pdus`，未跳过较晚发布的载体TLS。

全部其他事件/CLI值、GetFields、Session、来源、父子、ID、status、包协议计数不变；
完整flow导出逐字节一致。此为回归差分，不是全字段与Wireshark一致证明。
初始差分适配器对CLI整文件不变、可选tcp_flow_id和publication行的错误假设及失败保留；
最终适配器仅允许逐原始头验证的新增范围，不过滤其他差异。
`Weber-TLS-range-diff91-05/summary91.json` SHA
`47527349d991b0a1fb90783348c3e36c6f11d1671095e49929e65689fdcc6e56`。

旧严格SMB04重跑：5个offer、107完整记录/121命令头、全部281来源匹配；已映射错误0，
仍1400覆盖缺口（1353未发布/47范围未映射），full_selected_acceptance=false。
独立QI gate93个PDU、113个唯一命令/731字段闭合；检查器含兼容副本为125命令/809叶、
8382断言0差异，不能把兼容副本当成新增命令覆盖。独立392项映射闭合后仍1008缺口，
未借VLAN80/81的跨域selector降低计数。其他16条旧SMB1仍需原生正文/状态工作。

## 实际 Windows 页面

干净82fc二进制、完整原226帧、原外部keylog，各两页实际原生控制台运行，18状态/页，
共36；不是终端截图mockup。frame42完整密文PDU1158B，Record Length1153、HEX0481；
frame154 PDU1151B、length1146、HEX047a。高亮均精确PDU[3,5)，不拿1129B已认证
HTTP明文替代密文长度。列表与递进链显示HTTP/1.1及TLS1.2/1.3，切换明文/密文来源独立。
主线程已实际查看两页selected/bytes/native-wire/narrow/restored PNG，并检查mouse/tabs、
80x30→180x48 resize、CP936恢复及正常退出。渲染图来自实际console cells/attributes，
不是Windows Terminal像素截图；完整PDU/hash另验，可见前416B不称全文已显示。

源81冻结门/实际闭合 `M2-Windows-TLS-root-closure91-01/summary81.json` SHA
`5b16d9676788308031826751e41e5f7bed51887a7b57bdf8d24726fa3e3c95b5`。
新增起点门核对实际restored cells中的4个source record（连接2/6双向）、完整原始hash、
原frames34/36/144/146、准确517/105/1775/1221B和Handshake标签及各32B可见头；
`Windows-TLS-prefix91-01/summary91.json` SHA
`b3df363a82cc726af6567f67b7a691f7bb2d5ae1ac04952c81d85d461d6c33b1`。
之前8f页面的unknown起点和两次字段导航失败原样保留；新运行只调整导航位置，不改源期望。

大型/自有捕获、密钥、完整导出、控制台图片和fuzz corpus全部留仓库外。样本ZIP未变化。
本批结束时C盘可用约31.7GB，未删除用户文件或缓存。其余未关闭条目及AI时间以
[M2验收清单](shark-m2-correctness-checklist.md)为准。
