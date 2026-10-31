# M2-10 非 SMB 有限差异清单

本清单分13个可验收条目，7项在明确有限范围已有证据、6项未关闭。
不按协议数或测试数计算系统完成率。原133条不是133个待修bug，也不因分类为0未审阅
而等于全部协议/会话/fuzz正确。SMB按用户要求跳过，见[搁置记录](shark-m2-smb-deferred-20261031.md)。

[4477本批真实来源、七场景、实际callback、Windows、大捕获与133门绑定](shark-m2-non-smb-20261031.md)。
外部逐项账本位于 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/M2-nonSMB-next171-01/`：
133个捕获SHA/frame/field稳定ID、6个TCP政策例项和24个UDP历史例项均保留；不提交完整导出。
原171是164运行前的静态盘点，不能当4477运行证书；N01/N03的新绑定由闭合173另给。

| ID | 验收项 | 状态及范围 | 未关闭的验收门 | 剩余AI实施粗估 |
|---|---|---|---|---|
| M2-10.N01 | 8个DNS可读名称 | 有限完成；两捕获各4例 | 137逐项原始name bytes/可读及escaped alias/txid/type/class/响应关联；173以完整SDK精确字节绑定新4477 | 0 |
| M2-10.N02 | 2个非法UTF8显示差异 | 无损保留合同完成，文字差异保留 | 原字节与replacement文字分开；不能以此豁免合法文本错误，不能称全部字段parity | 0 |
| M2-10.N03 | 122个TLS归属及1个RDPUDP参考限制 | 有限完成；参考限制仍单列 | 完整record raw SHA/长度/方向/域/贡献帧与完成帧逐项核；derived补充不冒充原承载端点；173绑定新4477 | 0 |
| M2-10.N04 | TLS三个外层头字段范围及Stream起点 | 有限完成（M2-37） | 私有已接纳头、当前完整Raw、域/物理TCP代次/offset0/全部来源一致；没有认证/上下文的假密文范围 | 0 |
| M2-10.N05 | TLS1.3 KeyUpdate具体字段及握手字节源 | 有限完成（M2-38） | 131六个真实更新；认证5B/typed请求值/derived alias/明文独立高亮；不自动关闭N12 | 0 |
| M2-10.N06 | HTTP迟到/乱序请求后的响应恢复 | 有限完成（M2-39） | 真实163/167恢复两例旧6差异；HEAD/CONNECT/Expect100、预算、原贡献及callback因果门 | 0 |
| M2-10.N07 | 永久缺客户端请求的HTTP响应边界 | 开放，已证差异/可观察性边界 | WS169显示6响应，本系统1完整响应及未知方法头；不得猜GET或吞HEAD/CONNECT正文。建立partial可观察性门，不承诺缺信息时完整消息划分等价 | 2–4小时 |
| M2-10.N08 | TLS1.2 C1新SYN未配SYNACK的历史恢复政策 | 开放，尚非已证parser bug | 保留旧strict exit4/8差异；独立证明所有旧Flow2来源、AEAD/peer/父子/response_to/实际Feed57；不得抹成跨代串流或直接改预期 | 1–2小时 |
| M2-10.N09 | QUIC app-only缺握手时的身份及来源资格 | 开放，覆盖与政策 | 两域组合各2664 strict差异；各2007准确UDP预览不等于QUIC语义。CID/epoch/STREAM/QPACK/0RTT须分开冻结真实源与拒绝门 | 3–6小时 |
| M2-10.N10 | 6个fuzz TCP tuple分区/复用差异 | 开放，政策/代次证明 | 固定packet header/时间/域/原序号和实际SYN/SYNACK/FIN/RST；每例分别核分区与HTTP关联；不直接用WS tcp.stream号代替连接事实 | 3–6小时 |
| M2-10.N11 | 24个fuzz UDP分区差异的应用会话 | 开放，物理来源清单及语义缺口 | 从原24例建立完整header/hash/domain/time ledger，逐协议按transaction/CID/会话生命周期验复用/乱序/缺失及域隔离；不能仅把udp.stream划分当唯一答案 | 4–8小时 |
| M2-10.N12 | KeyUpdate record尾边界和换钥原子性 | 有限完成（M2-41；其他换钥消息仍开放） | 176/180独立源、39ca真实SDK三类误接受及HTTP泄漏；6bac同v02六源通过及12DTO拒绝，私有secret/IV/AEAD/seq及反向/预算测试、96752插桩fuzz。父阶段列表问题另列M2-42；后续9af/229实际60状态已关闭有限UI合同 | 0 |
| M2-10.N13 | TLS record/message/连接资格及CertificateRequest正文 | 开放，有限来源审计完成、正文覆盖缺口确认（M2-43） | 238/239原wire及全捕获WS：两疑点默认也TLS并报结构错，不是已证标签误识别。240坏41B正文CA声明70仅余13。241真实mTLS12/13、244独立来源/AEAD、245的55字面+1空向量匹配参考；native两消息仍envelope，52正文typed事实未发布。正文校验/字段/源/UI与资格、粘性/复用/丢失/域隔离仍开放；见[审计证据](shark-m2-tls-admission-audit-20261031.md) | 后续实施3–6小时 |

原5个未闭合有限项粗估13–26个AI实施小时；N13有限来源审计已完成，正文/资格后续实施粗估3–6小时。
当前6项合计16–32小时，不含CI/构建/回放等待及新问题；不是核心协议完备承诺。
M2-42列表可见性已由9af及229实际60状态关闭有限合同，
[源码、原红测、页面和两大包逐对象验证](shark-m2-tls-row-priority-20261031.md)。
[N12实际重现、修复及剩余边界](shark-m2-tls-record-boundaries-20261031.md)；后续先补N13正文校验/字段及身份资格，
再N07可观察性、N08政策门及N09–N11逐项来源。N10.02只做两原SYN来源及TSO显示审计，
尚无建立连接或产品误合并证明，不把stream编号当连接事实；其余5个TCP例仍未审。

N12规范依据：[RFC8446 §5.1](https://www.rfc-editor.org/rfc/rfc8446.html#section-5.1)。
旧循环先handshakeMessage再处理同record余字节的错误已被实际复现并修复；其他握手record对齐不自动关闭。
原已经关闭的HTTP3 prefix标签与resumed-QPACK投影不重复列成开放。全部TLS/QUIC扩展、
SSH加密应用等未在这12项中穷尽，不能由此推导TOP30或核心协议完备。
