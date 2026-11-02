# TLS旧代晚到来源及HTTP递进页面：2026-11-02

M2-10.N08关闭有限的被动捕获政策和来源验收，不修改TCP/TLS解析代码；原strict72的
TLS1.2 C1失败继续保留。实际Windows页面另外暴露HTTP正文排在起始行前的问题，
新增M2-62并以 `d46ffff5fadbb2067208cd66e27fa7a1a73d8907` 修复。仍只进入M2 PR #5371。
SMB的3项搁置和独立任务书保持，不做SMB语义分析。

## N08有限结论及原差异

单个新SYN只作为开连接观察；匹配的反向SYN-ACK或已有代次可确认的关闭边界才退休
旧解析状态。在此之前，拥有旧代字节及密钥的连续前缀可完成旧消息，不能因此赋给新代。
这是一项被动分析政策，不是对真实接收端是否接受某个包的判断。

| 真实源派生控制 | native TLS / HTTP | 原strict结果 | 新参考HTTP |
|---|---|---|---|
| TLS1.2、1.3永久记录缺失，各225帧 | 各132 / 45 | 两例均0差异 | 本轮未重复导出 |
| 两版本同代晚到，各226帧 | 各136 / 48 | 两例均0差异 | 本轮未重复导出 |
| TLS1.2旧数据在新SYN后、匹配SYN-ACK前 | 136 / 48 | **exit4，8差异，0覆盖洞；保留失败** | 45 |
| TLS1.3相同包序调整 | 132 / 45 | 0差异 | 45 |
| 两版本旧数据在匹配新SYN-ACK后 | 各132 / 45 | 两例均0差异 | 各45 |

857以当前747代码重新构建实际Feed/Close观察器，运行8个原生观察、8个未改strict、
8个CLI、8个flow及4个新TShark，共36次执行。四个新参考共5,784个字面TCP/帧字段检查。
所有捕获仍有8个物理连接。此次没有用重新编号、忽略差异或改变原预期把strict变绿。

源是自有对端实际产生的TLS1.2/1.3完整与恢复通信，原捕获226帧、8连接、48个HTTP，
SHA256 `80b46205a38a11f61f0698d01d953b04e6428cebf54989ce9925911497c2cfdb`。
故障控制只删除/移动原记录，保留包内容和原时间；时间回退不代表新的真实对端交互。
参考固定TShark4.6.9、空配置、显式keylog/两个服务端口、双遍和乱序重组。

## 旧代内容和关闭边界的独立证据

859重跑两条独立Python解密路径：136个原TLS记录，其中94个AEAD记录验证通过，
48个HTTP与实际对端内容一致。当前TLS1.2 C1全部136 TLS和48 HTTP逐项加入原始来源账本，
包括方向、域、SHA、所有贡献帧、完成帧、PDU父子、response_to及实际Feed因果。
多出的4 TLS和3 HTTP均属于旧TCPFlow2，全部在Feed57首个连续可交付前缀处发布，
没有分配给新TCPFlow3；拿新代密钥解旧记录明确得到InvalidTag。
CompletedIn仍是最后字节贡献帧，不等于callback发布时刻。

861另用公开SequenceObservation在实际OnPacket之前和OnClosed时记录游标，4次原生
运行的全部事件导出与857逐字节相同，独立原始包前缀模型与当前游标一致：

| 控制 | 观察到的RST及游标 | 实际旧代关闭 |
|---|---|---|
| TLS1.2 C1 | RST原56/输入55；seq2521460966，连续捕获前缀2521457368，前方3598B | 输入56仅新SYN；输入57旧洞补齐、旧消息交付后以rst关闭；新SYN-ACK输入58 |
| TLS1.2 C2 | 相同RST仍在缺口前方 | 输入57匹配新SYN-ACK退休旧代并保留gap；输入58旧数据不解为旧代或新代消息 |
| TLS1.3 C1/C2 | RST原168/输入167；seq及发送方向连续前缀均413833401 | 输入167已以rst关闭，保留反向gap；后续新SYN和晚到旧数据不能恢复已关闭旧代 |

这解释两版本结果不同的具体TCP证据，没有按TLS版本硬编码生命周期。
RFC9293与RFC5961规定接收端RST/SYN校验依赖接收状态；被动捕获游标不能冒充真实
RCV.NXT，也不能据此宣称对端接受或拒绝。C2退休之后的原来源前缀计算只用于反事实
比对，不称作产品仍保留的活连接。[RFC9293 §3.10.7.4](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.10.7.4)、
[RFC5961 §3.2](https://www.rfc-editor.org/rfc/rfc5961.html#section-3.2)。

原strict72把新SYN作为更早的边界，所以仍要求丢弃这些已被独立认证的旧内容。
N08现在验收的是明确、无跨代借用的现有政策；不称原strict通过，也不称与参考消息数一致。
现有pending-RST、opening-observations、预算和复用测试已在747完整CI通过，未为此添加
重复实现式测试或放宽断言。高级TLS/QUIC记录洞、完整角色/阶段和其他fuzz政策仍开放。

## 实际页面问题及修复

867原生Windows回放真实TLS1.2 C1，HTTP PDU42/帧57仍为旧连接#2，TLS父PDU41，
响应关联请求PDU35；右侧完整HTTP明文1129B正确。主agent看图发现左侧顺序却是
Headers→Body→FirstLine，状态行内部又是Status→Message→Version，不利于递进分析。
原图、单元格和872看图记录保留，未用之后成功覆盖发现。

d46只对已经解析成功的HTTP/1消息包装层使用语义顺序：

- 请求：FirstLine(Method、Path、Version)→Headers→Body→Trailers。
- 响应：FirstLine(Version、Status、Message)→Headers→Body→Trailers。
- 未知字段仍保留；重复header维持原顺序。正文中恰好名为FirstLine的字典及其他协议
  保持原顺序；不改全局排序、不改SDK字段、不授予生成字段虚假的字节范围。

869真实红测为3pass/4fail；修复后870的7个新ID全部pass，871完整CLI及CI脚本为
1362pass/2历史Windows optional skip/0fail。最初868是测试助手API编译错误，单列保留，
没有冒充产品红测。受影响之外的pcaputil/bin-parser生产输入未变，最终CI仍需完整重跑。

干净Windows程序SHA256 `7f8ab8fe989098d62e8083df8f1c843352b83a5569b60ea355d209936f1d47c6`。
875实际用新程序读取原PCAP，两页54状态显示请求Method→Path→Version和响应
Version→Status→Message，均在Headers/Body之前；主agent已看两张实际图。
请求82B和响应1129B完整逐页读回，Parent PDU元数据没有伪造高亮；80×30与180×48、
鼠标/Tab、CP936恢复均通过。DTO只用于单独的worker预检，实际UI重新解析PCAP。
证据是实际控制台单元格及属性，不代表全部Terminal字体、DPI、输入法均已验。

两个外部大型PCAP用新CLI完整回放，两份完整JSON导出与842逐字节相同。SDK/flow未
重复执行：873与834所有非测试生产构建输入中只改变CLI两个展示文件，非CLI完全相同。
旧六导出已有306,380事件、379,455物理行、580发布行的统计；此次输出稳定只证明展示
改动未改变这些完整输出，不能充当全部协议、字段、流、会话与Wireshark语义一致证书。

## 可复核证据及剩余范围

仓库外证据根 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`：

| 文件 | SHA256 |
|---|---|
| `N08-current857-01/summary857.json` | `98e856863f014dc1cd36ad2b305e574b9f24a14c538969752b4838b7f91995ce` |
| `N08-source859-01/summary859.json` | `d6db5906991410f2c30b8fb9511a8ce6ea3635f94e56fe3a6e0873f7d076b363` |
| `N08-receiver861-01/summary861.json` | `89f2b5531de66be461628a982484b07bdc0ede814a48f9bf92617ea185fffacb` |
| `N08-HTTP-qualification877-01/summary877.json` | `9a71ff673e06acdb89c6121cf4d85047796388db2c39bf047e6b8033297f3878` |
| `runtime-HTTP-order-public876-01/summary876.json` | `f58b17fcf638f65bf61e8022a57aea094201a0e86aef1bda2612f8dc85398990` |

归档ZIP未改，捕获、密钥、keylog、原始导出、图片和证据脚本都留在仓库外。
747检查点已独立完整CI全绿；包含本报告的新head必须另验，不沿用747的成功。

主清单新增M2-62后为62项：52个有限完成、7进行中、0未开始、3 SMB搁置，剩余10项。
非SMB17工作包中12项有限完成、5开放：N09、N10、N11、N13、N14，粗估22–44个AI实施
小时，构建、CI、回放等待及新问题另计。N13七个子项仍6有限完成/1未开始。
以上是有限范围交付，不是核心协议完备声明。
