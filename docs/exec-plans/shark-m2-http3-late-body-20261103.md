# M2-84：晚到 HTTP/3 方法的响应正文复验与实际 Windows 页面

本项有限关闭 N09.R06：响应先解码、请求方法后来才完整时，复验已经观察到的响应 DATA 和 FIN。
原始响应事件、Decode、ResponseTo 和字节来源保持不可变；后来的当前视图增加明确的复验结论。
这不认证对端交付、表示大小、完整 HTTP/3/QUIC 生命周期或全部 Wireshark 诊断等价。
新增 CONNECT 隧道帧序列缺口 N09.R07 单独保留。SMB 不分析、不修复。

代码、完整本地测试与干净 Windows 构建均为 `0bb6508ed47d0adc7da48cd8e28dd8c86411365e`。
其后的六份 Markdown 说明不改变运行代码；最终说明 SHA 必须另验全部 CI。
父 `e0fb24890448e2c4610b7e7ab1690525066352d5` 已独立全绿：47 个约定检查名、
91 个执行检查、两个 statuses，316190 原始 Traffic ID/动作与官方产物逐项闭合。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41563235938)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41563240309)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41563245187)。
闭合1718 SHA256 `b9c83c11c3790b0c1da3f3e81fdecd8b8c36abcd93a067deec524c97bd3750cc`。

## 修复范围

- 私有 STREAM 状态保留有界的原响应 DATA 计数见证和最初 wire FIN 观察，分别记录头观察号、
  正文贡献 PDU、FIN PDU、物理引用和后来方法观察号。FIN 先到而字节仍缺失时不宣称正文完成。
  每类最多64个来源，超限明确不完整；拥有字符串、数组开销计入现有连接内存预算。
- 后来有效方法触发原响应复验：HEAD 非空 DATA 违规；合法空 HEAD 可以有非零 Content-Length；
  普通响应在完整 FIN 后检查短正文；成功 CONNECT 禁止 Content-Length，隧道 DATA 不当普通正文计长。
  正常已知方法路径不分配这些迟到缓存；建立关联或 RESET 后释放不再需要的见证。
- 真实新语义违规只隔离原响应方向的消息状态，不返回为当前合法请求的错误，不丢弃其他 STREAM。
  已有布局/字段错误不清除、不修复成“有效”。关闭、退休、RESET、错误方向/域/owner 均不复活。
- 复验结论区分 `violation-observed`、`no-violation-observed`、`unverified-prior-failure`。
  只有头来源、正文来源及原 FIN 完整且重组已排空时，Body Coverage Complete 才为真。
  零字节 DATA 没有正文贡献字节，不能伪造 payload 来源；它自己的两字节帧头仍有真实来源。
- 沿用有界 Inspector 关联与 revision，当前响应视图可查询十一项 `http3.late_message.*` 上下文别名，
  不给密文 range。列表优先显示“earlier response PDU #… violation”，详情把复验结论放在大来源树之前。
  原观察的长度计数与后来的方法语义复验分开保留；capture-time JSON 由后来请求携带新发现，
  当前 Inspector/TUI 响应视图提供补充结论。Malformed 原事件统计不会被追溯重写。

规范依据：[RFC9110 §8.6](https://www.rfc-editor.org/rfc/rfc9110.html#section-8.6)、
[§9.3.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.2)、
[RFC9114 §4.1.2](https://www.rfc-editor.org/rfc/rfc9114.html#section-4.1.2)。

## 新实抓、独立来源与 Wireshark

仓库外 `HEAD-body-source1708-01/live.pcap` 为 Windows 自有 loopback 两个实际完整/恢复连接，
复用同一对 UDP socket，第二连接接纳 0-RTT。使用 aioquic 1.7.0、独立 CA/证书验证与实际响应收集。
SHA256 `c18a967c99589bf4d1d6ad3c44a99dfe348b4f69d4ee1f470313531932045cdb`。
第一个 HEAD 响应故意带280字节 DATA；第二个无正文但 Content-Length=280，作为合法负例。
客户端按 QUIC STREAM 原字节收集，再以独立 frame walk/pylsqpack 核对，不将通用库的正文计数当判定。

1710 独立来源早于候选修复：20物理记录、22保护包的原字节/PN/AEAD、四条 HTTP/3 消息、14对头字段、
四个 DATA 帧（含零长度）和八个帧包络。记录 early-data 观察，不声称被动验证 binder/Finished/PKI。
1712 三个冻结乱序控制只移动原始保护包，保留全部22包各一次；拆分原 coalesced 包并重算外层长度/校验和。
它们是一次实抓的变换，不是三次额外实抓。1713 在父实现复现四个晚到目标的正文检查缺口。

最终1733：原样本加三控制，8次原生/CLI执行、4次全新完整两遍 Wireshark 4.6.9，
88次保护包比较、16条独立消息比较，四个晚到目标全部通过。
原已知方法的坏 HEAD 继续 malformed，合法空 HEAD 继续 decoded；晚到目标原事件和来源不变，
当前视图与合法请求中的补充结论一致，头、DATA、FIN 与方法观察号精确匹配独立来源。
1742 逐包字面复核全部16条消息、56对头字段、32个 HTTP/3 帧：原生、Wireshark 与独立来源一致。
包括空 DATA 的帧头、无正文贡献引用、非空 DATA 全文哈希、帧长度和 STREAM 区间。
不声称 Wireshark expert 诊断与本系统后来复验的输出形式完全相同。

旧1490实抓及10控制：最终33次原生/11次全新参考保持，21保护包、353 QUIC 字面检查、
四条 H3 消息/14头/四正文/八帧及13目标（7合法、6跨层顺序拒绝）/276检查全部通过。
接口/VLAN 的四个原 strict 域控制均零真实差异、零 coverage gap；app-only 的未知应用仍不冒充已认证。

## 本地回归、fuzz、大捕获与 Windows

- 一次完整本地运行1731覆盖 bin-parser、pcapx、CLI、scripts/ci：291132 pass、25历史可选 skip、零失败，
  共291157唯一 ID，676.984秒。所有291116父 Windows ID/动作保持；41新增 ID 全部 pass。
  不删除旧断言、不改工作流或两个样本 ZIP、不靠路径缺失跳过测试。
- 六个种子的新原生插桩 fuzz：34791次，61.735秒；独立的 HEAD/POST/CONNECT、长度、缺口、早 FIN、
  RESET/退休和内存断言。硬限制1 CPU、768 MiB job、最多两个并发进程；第三进程拒绝正控通过。
  原生保护包 v1/v2 × 三套件、QPACK 恢复头与原 DATA/FIN/后来方法的独立观察号另有集成测试。
- 两个大公共捕获六份完整 SDK/flow/CLI 导出与冻结父逐字节相同：379455物理记录、306855 SDK事件、
  379926 CLI记录（含471独立发布）。这证明既有输出保持，不是全部语义与 Wireshark 一致。
- Windows1747 新构建真实 console 四页共72状态：违规原响应、合法空响应、合法后来请求和280字节正文。
  上下文选择保持原捕获 PDU、无虚假高亮；正文选择切至独立明文 arena，完整280字节/哈希及高亮精确。
  80×30/180×48、鼠标/Tab、CP936恢复和全部进程关闭通过；主线程查看实际 cells 渲染 PNG。
  `Windows-R06-1747-01/bad-response-context/console/01-selected-field.png` 与 payload 页均在仓库外。

1723 修复前15个失败 ID 与全部输出保留。1727 新 RESET 测试曾错误地先声明 final size=0再送非空请求，
传输层拒绝正确；改为实际合法 final size，保留原错误/无关联/内存释放断言，两个方向后续均通过。
没有借此放松任何已有检查。

## 证据摘要（全部完整产物在仓库外）

| 记录 | SHA256 |
| --- | --- |
| 独立来源1710 | 8587826456d4f062343582bcf66ef42d703c3b0001cb1dcbe22d1e7af2275204 |
| 冻结控制1712 | 7a36d950a5eb553d3844f62076f21d466d5cfa3ac2bfcab0fc2999511ff57792 |
| 父缺口1713 | b0940a2a61f13532ca7068fbe91be68075ad78d0e877a00f7ee9dc016e123520 |
| 最终新源1733 | 97c1ceea656e102a7c4e4fa4ac3dfb841fc9b4a9b5d05e1f6fdfa80612effcd3 |
| 16消息/56头/32帧1742 | d977dc2b26163e835e946858597da3ad10bf091c19f7481b7b8172155cc7b836 |
| 原21包/353字段1743 | 886c3af489eb56458309d8946adf0a90698d1b5914904f6264800b8cd3d2d503 |
| 原H3字面1744 | 1bc06dcbb002f4910c8be2cf26209d6a8544ee7db475b32ad5750664ef0d3858 |
| 13目标/276检查1745 | 7bdc4663f89226723c3d0621046f2abb5d70c746712975a4b58b201ff1c7a097 |
| 六完整公共1739 | cfc2a89b6e2e43d600381957edeb2f5853a1324dcef5b0b9e95060c811059774 |
| 四域strict1741 | 2b696ad565620f9b432f34905dd8fee9c711395502c9f78eb52219cec282d3cb |
| 实际Windows1747 | 15756d8e239aac7ed67f9d89f6a0735606fe86a02e12ada2058b242e116848f8 |
| 资格归档1749 | a829294894e5e958c4c2ff1a54a8314dc6ba01f853659be1e3194811485e207f |

完整 stdout1731 SHA256 `35c5760d3876f9dd0d57b3b1d554b3d2dd46a9edaf68088be32c1fe5bad095ce`。
原生 events/flows/yak 构建 SHA256 分别为
`a6216d68e489f137b597bc03e2f7f94b2e8cff4abd73a6a0de752220085cb4f8`、
`5c6b79d9ae0b88b6a34451bff1dfef13bbc1869b3e68e7b9818d3b844aecf0af`、
`f3230f80e55e2eb470f25e8107be846471ccff78e2e99bf8d4f627eb01d5c4b7`。
归档时磁盘空闲32905166848字节。大捕获、keylog/私钥、全文导出、PDML、fuzz和console产物均不入库。

## 仍开放与最终交付门

新增 N09.R07：成功 CONNECT 后原生仍接纳已知 HEADERS/trailers，方法晚到也未补此帧序列违规。
[RFC9114 §4.4](https://www.rfc-editor.org/rfc/rfc9114.html#section-4.4) 要求 connection H3_FRAME_UNEXPECTED；
不能把本项正文长度/方法检查当成完整隧道帧验证。仓库外1748 authored overlay：普通 GET trailers负例1 pass，
已知 CONNECT 与晚到 CONNECT 两个叶子失败，加父共3 fail；stdout SHA256
`4b65d3d0ec84f72a0388fd87e1642390f972e9b7941e7edf7c11b12dd96e6e77`。
失败证据不进通过分母，也不是实抓；后续需原生连接级错误、晚到观察、复用/域隔离与真实来源/页面验证。

主表84项74有限完成、7进行中/3 SMB搁置，仍10未关闭；非SMB17包13有限完成/4开放，
粗估18–36 AI实施小时，等待和新发现另计。N09完整 CID/epoch/STREAM/QPACK/0RTT/remembered SETTINGS、
N10六TCP政策fuzz、N13完整TLS阶段/协商及N14高级字段/重组继续开放。M1阶段范围保持，不宣称核心完备。

最终说明 SHA 在推送前固定316231原始 Traffic ID（316206 pass、25历史可选skip），
完整保留316190父 Linux ID/动作并要求41新增全pass；131084正式required仍须全pass/零required skip。
全部47约定检查名、所有实际执行检查、两个statuses与官方产物/checkout/digest/CRC/原始ID逐项另验。
自动draft skip、父成功和本地通过都不替代最终全绿。仍使用唯一M2分支及PR #5371，不改写历史、不强推。
