# M2：CertificateVerify 完整正文、拥有字节与 Windows 展示

本项关闭 M2-47/N15 的有限合同：版本化 CertificateVerify 向量、完整签名方案、长度、拥有签名字节及 Header→Body→Context 展示。签名、证书信任、完整握手顺序与 PHA 上下文不由产品验证；TLS/QUIC 全阶段与丢包恢复仍归 M2-43/N13。SMB 继续按用户要求搁置。

代码为 `e5f4e795242343316af0ec838d85ec885200a9f2`；实际 Windows 审查后发现列表先显示 Finished、CV 方案离开可见区，已另以 `488147e9cd490f863edbca5720ff39a85d8cfcf1` 修复并重新构建、回放和验页面。保持同一 M2 分支/PR #5371；M1 #5353 已合并。普通提交，没有强推或历史重写。

## 解析与展示边界

- 完整四字节握手头及 uint24 长度必须一致；签名向量必须精确消费正文，不接受残缺头、短签名、错误向量或多余尾字节。输出拥有字节，不借用调用者输入。
- TLS1.0/1.1 正文是 uint16 长度与签名；TLS1.2/1.3 先读取完整 uint16 签名标识，再读长度和签名。旧版本 body decoder 的支持不是旧 CBC/SSL3 解密能力承诺。
- 必须有独立观察到的协商版本；record legacy version 不能选正文语法。无协商/不支持版本仅发表 qualified envelope，不发布正文别名或范围。
- TLS1.3 CV 要求握手明文已通过记录 AEAD；TLS1.2 初次客户端 CV 在 CCS 前是明文，不虚构 AEAD=true。已知角色约束不能允许旧 TLS 服务器发送客户端 CV，也不允许 TLS1.3 服务器的应用密钥阶段 CV。
- 空 opaque vector、未知签名代码保留 literal，不能由“正文可读”推导签名合法。TLS1.2 ECDSA hash/signature 对不选择曲线；TLS1.3 完整方案独立于 cipher-suite PRF。例如真实 RSA 0x0804 在 0x1302/SHA384 套件中仍用 RSA-PSS/SHA256。
- `tls.handshake.sig_hash_alg` 复用原注册，以消息顺序投影 CR 和 CV；新签名别名使用 Wireshark 的 `tls.handshake.client_cert_vrfy.sig_len`、`.sig`。这些是 derived literal，不标外层 PDU/frame 范围。
- worker/JSON 后重新解析完整编码并比对每个正文值，才建立独立来源。TLS1.2/1.3 头[0,4)、方案[4,6)、长度[6,8)、签名[8,end)；旧版本签名从6开始。空签名没有伪造非空范围。拥有源和字段行继续受已有总预算限制。
- 单 record 和同物理 frame 多 PDU 均保留具体正文摘要；CV 优先于 CR/普通 Finished，KeyUpdate 与实际应用层身份保持更高优先级。详情仍保持消息顺序；不会把连接已观察到的 HTTP/1.1 当作当前 CV 包的应用正文。

## 原始来源和独立核验

全部数据在 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/` 外部证据目录。没有把真实抓包、密钥/keylog、完整导出、截图或大捕获放入 Git/项目 ZIP。

| 原始来源 | 物理帧/实际连接 | 用途 |
| --- | --- | --- |
| source305 原始 RSA PHA | 29/1 | 后握手认证、后续三轮 HTTP |
| source316 真正同四元组复用 | 52/2 | 两个不同 SYN ISN/ClientHello 的独立代次 |
| source241 初次 TLS1.2 mTLS | 19/1 | CCS 前明文 CV、Finished 与 HTTP |
| source241 初次 TLS1.3 mTLS | 17/1 | 初次双向证书认证 |
| 新 source356 ECDSA P-384 | 29/1 | 真正 TLS1.3 PHA、0x0503、102B DER 签名 |
| 新 source356 Ed25519 | 29/1 | 真正 TLS1.3 PHA、0x0807、64B 签名 |

合计175个原始物理帧、7条实际连接。源 oracle 以 wire TCP、keylog、AEAD、transcript 和标准密码库独立验证13个 CV签名、19个 Finished MAC、34条 peer HTTP正文；没有以产品或 TShark 输出作为期望。它们不等于产品已经验证这些密码学结论。

新 EC/Ed 来源只在本机 loopback，Python SSL/OpenSSL 实际协商、服务端触发 PHA，密文不是手工构造。捕获SHA256分别为：

- EC384：`55325b05a63dec2ae8bd33e84fe9eac583456ebd0f12464965b971a43dc3c819`。
- Ed25519：`d8abdcdc51e2009283b68c118fb5b6aff300a7425e330b4fce51b8f8ba47b7f6`。

新 oracle357 SHA256 为 `cdf7b2619fa2cc66c20cd4cc48f6c02515a426fc9cb8ca8d05914d1e9d30d40d`、`16a3f9ff2af1fec3bf38ca983f73574a4093d9f77eb1d3c33d48dad983b5a92f`；各验证2签名/3MAC/6HTTP，含签名和 transcript 位翻转拒绝。
TLS1.2 新350单独验证客户端签名，保留原2MAC/2HTTP：`11d17bf05c4c09638961391a5d548180cd27694940467f7771d153aaa227f45d`。

## 实际原生/CLI 与参考

488干净构建的11组22次完整 SDK/CLI，收据376：`71ca2fa486f0dab24526a702ac297e756bce607f196a529c6daf92576f4b3a17`。每个 CV 编码、方案、长度、签名字节、phase、derived来源、CLI两种别名形式及原生ID均逐项核验。

| 场景 | HTTP / Finished / CV | 资格 |
| --- | --- | --- |
| 原包、精确重传、乱序认证 flight，各自 | 6 / 3 / 2 | 原文精确一致，重传不重复发表 |
| 整个认证 flight 丢失 | 4 / 2 / 1 | 不捏造 PHA 完成或缺失后的连续 HTTP |
| 仅截短 frame20 末尾7B | 4 / 2 / 2 | 最后的 Finished残缺；前面的完整CV必须保留 |
| 首个 Certificate 密文位翻转 | 4 / 2 / 1 | 后续CV记录虽完整，握手边界在认证失败后不可证；恢复缺口继续开放 |
| 同四元组两代 | 12 / 6 / 4 | 2个原生TCP代次，各6条HTTP，不跨代继承 |
| 初次 mTLS1.2 / mTLS1.3 | 2 / 2 / 1；2 / 2 / 2 | 协商版本和明文/认证语义分别准确 |
| 新 EC384 / Ed25519，各自 | 6 / 3 / 2 | 不假设RSA长度或从cipher PRF推签名算法 |

11个新隔离 TShark 运行，空profile、两遍、明确自有端口及keylog。所有已发布CV的每个 literal 和相对字节范围与独立来源一致。原9组参考355闭合：`8c6f14e10e9354a4a0c29775f95aaa580098f3b8a2f8bffb622cf63bf02257c7`；新两组358：`18cdee0b569df3e609df18bfe2442743e91fc0b50aff13a666ef35c391a04597`。

损坏 Certificate 场景，原始来源证明后续有2条完整CV wire记录，但本系统与Wireshark各仅发表1个CV；这不是通过条件被改成1，N13仍保留恢复/可观察性边界。不能从“参考也少1”推导已经正确恢复。末尾截断的CV则确实完整，原生和参考均2个。

原355v01把所有负控都假定成1个CV，在截断场景失败；v02/03的完整捕获lexer和纳秒header假设也未通过preflight，均未验收。改由原始完整认证记录的字节哈希冻结计数后，v04暴露上述参考恢复缺口；v05保留源计数和缺口，不用参考反向修改产品期望。全部失败留存。

## Windows 实际页面

488新二进制再次运行 TLS1.3 RSA-PHA、TLS1.2明文CV、EC384、Ed25519 四页72状态。完整握手264/264/110/72B分别与源编码逐字节一致，签名高亮[8,264)、[8,264)、[8,110)、[8,72)。鼠标、Tab、80×30/180×48和CP936恢复通过。
收据379：`c183d0bafa4d3ec3374c0ee884a62423298e8beec83db165f9cdcc83079054b8`。还直接检查所选 packet行的可见方案名；初次379 checker假定ASCII `>`，实际是pane border及`›`，失败保留，v02只修正行定位，源字节与高亮门不变。

原e5的367页中，字段和高亮正确但实际packet摘要被Finished前移，已写372红测并修复；最终378页已直接看图确认CV方案在前。图来自真实Windows console cells/attributes的固定渲染，不是 Terminal字体、DPI或IME全覆盖验收。

## 测试、fuzz 与 CI

- 348旧原生54fail →349同54pass；351原展示10pass/6fail →353同16pass。
- 362合包边界最终红1pass/3fail；修复后363完整CLI1127pass/2历史optional skip。
- 完整同CI范围354：313246pass/25历史optional skip/0fail，875.421秒，输入和索引未变，stdout `ef174b1c5199b29522e1fc3fb0b87efd3112243e82cffd5361696b175834aed6`，receipt `5cb699982e9eecb21e74a9f70be5ef3e1b473b3b24c9ee516eb65218c856d29e`。
- 后续实际页面发现372真实红1fail；只改CLI排序和对应断言。最终373完整CLI1127pass/2历史optional skip/0fail，stdout `e836fe807f3efaef63ea796937ebbbcc3f8a637ee68c4e61fcee3160d2418fc0`，receipt `11e284bbd37db4b4f2662af4e17dbebebfbbf4a5d6ec7db4463efc7e934feae1`。原native/其他测试输入未变，只有3个CLI文件变化，不把较早354说成最终源码的整套重新执行。
- e5插桩CV fuzz370：5/5种子完成、75064执行、0fail，61.359秒。硬限1CPU/总768MiB/最多2同时进程，峰值315060224B；关job后精确路径CIM无残留。总生命期5进程不等于5并发。所有temp/corpus隔离在仓库外。不把此有限framing/ownership fuzz当完整AEAD/会话状态fuzz。
- 新CI读取前冻结313256个原始ID/终态：313231pass、25历史optional skip；保留父313158全部ID/动作，98新增CV全部要求pass，131084 required全pass/0skip。manifest `f4fb69406f3cc6a2b05700576b6da8d338b05490ba38f13a70a7a1d6df06ee56`。
- SV本地3worker/CI2worker的10161帧分别恰好一次，不重命名CI ID；既有GUID测试在GITHUB_ACTIONS内的历史skip保留，不用本地pass改写CI。失败的364 preflight保留，374在任何新artifact读取前冻结。

最后文档提交的CI仍须核对其准确head、实际checkout、完整产物和全部checks/statuses，不能拿父8a的全绿替代。父8a已单独闭合47checks/2statuses/313158IDs，收据 `cc1256031ca8a015a72e8894c0523ba7c7555d199e20a1a519399e53b92198ea`。

## 大捕获与尚未关闭的差异

488的两份原始大捕获已完成6次完整回放380及逐对象差分381。380收据SHA256为015a5451a33f28d7ca4f3d587bcb2a7d51469e5540ac46266790e637ee24f5cb，381为0accb4dc5c722ff276e1c7ed007ca9c1b624f0e2c810b8971552b92b28887063。Weber42223事件中仅ID41013新增完整CV正文及3个derived别名，1个CLI物理包49524的摘要变化；Sharon263109事件及全部CLI输出不变。两份flow JSON逐字节一致（824/840观察流、14172/7700回调），协议列字面相同仍为38703/51328、39777/328127，不能叫正确率。382v02逐条校验完整CV编码、256B签名、0x0804及[0,1)/[1,4)/[4,6)/[6,8)/[8,264)参考相对范围，并验证SDK session/expanded fields及CLI两种字段表示后，才扣除这些已验证新增，所有原ID/raw/source/既有值及其他CLI内容精确相同。初版382遗漏SDK expanded-fields副本而失败，修正后两个副本都先验证，原失败保留；没有直接忽略字段。闭合382SHA256：613dfcd079f8ed03eb944558a6f75cc187b144c34565bd940620d28149a22e99。原始文件仍为Weber SHA `ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`、Sharon SHA `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`，不入库。

新365完整输入解码、仅以CV显示过滤限制PDML序列化，补充原329所有379455物理帧的census，不能把filtered输出说成新的全列统计。Weber有1条CV；Sharon参考有5个CV样式观察。
371独立读PCAPNG全部328127帧，保存5个物理来源。4帧带IPv4 MF，1帧IP total_length=0；记录legacy_version均0x0300，不是协商版本证明。已能在边界内读取的3个候选分别两种CV语法均向量不完整；frame307516仅在TLS12/13裸向量语法下可读，但空签名、代码0、无协商及碎片上下文，不能算确认的有效握手。完整事实/偏移/哈希保留，不能通过补标签把它们当5个已修复正确消息。此来源归因不是关闭N14/IP重组。
365收据 `023f4d9978ed3c09e3d32da3b6446d63c2c30c0e47a51449edd3a7f5b9e67ebe`；371收据 `e2b15bc26b49c83e734a982c4b82d28cc4f3b52622d5272f62196567764d73d2`。

N13完整阶段、PHA offer/context唯一性及多轮关联、缺握手资格、QUIC高级恢复仍开放。N07永久缺HTTP方法、N08新SYN政策、N09 app-only QUIC、N10六TCP及N11二十四UDP fuzz、N14碎片基础继续逐项处理。SMB三个父项继续搁置，见独立任务书。

规范：[TLS1.3 RFC9846 §4.5.2](https://www.ietf.org/ietf-ftp/rfc/rfc9846.html#section-4.5.2)、[TLS1.2 RFC5246 §4.7/7.4.8](https://www.rfc-editor.org/rfc/rfc5246.html)、[IANA TLS SignatureScheme](https://www.iana.org/assignments/tls-parameters/)。这些结构和独立验签不替代产品握手状态机或证书信任。
