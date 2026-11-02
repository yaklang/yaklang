# TLS EncryptedExtensions 资格和原状态保持：2026-11-02

M2-70修复N13.P04复核中找到的EE资格错误。代码和干净SDK、flow、Windows构建为
`4c2e72f17f5aec28d5f7c545b30bc1450a5ccf6e`；仍在唯一M2 PR #5371，SMB继续搁置。
这是有限合同，完整角色/阶段/协商、N09高级恢复及其他既有未关闭项继续开放。

## 原故障和实现

旧实现把EE交给通用扩展读取器。EE中的supported_versions可以把已选TLS1.3版本改成TLS1.2；
最终ServerHello释放完整ClientHello后，也无法拒绝从未提出的ALPN。客户端方向、TLS1.2和
application-key阶段的EE亦会进入读取器，EE集合预算未生效。独立1127为12pass/33fail，日志保留。

依据：[RFC8446 §4.2/§4.3.1](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3.1)、
[RFC7301 §3.2](https://www.rfc-editor.org/rfc/rfc7301.html#section-3.2)。
先验证当前版本、已知方向、密钥阶段和完整扩展包络；拒绝13/21/41/43/44/45/49/50/51
出现在EE，检查重复和集合预算，再调用通用读取器。完整已接纳CH的ALPN vector单独拥有、
计入session storage，在finalSH释放正文后继续存在；选择须逐字节属于提议，缺失提议也是
已知事实。缺CH上下文不伪造空提议。拒绝保持版本、ALPN和保留状态；记录AEAD认证事实及
已消费序号分别保留。新流不继承前流提议。

其他扩展仍有opaque范围；全部扩展位置/提议依赖、完整消息顺序、证书类型/压缩、QUIC参数
继续归P04。此批未新增EE所有叶字段的拥有字节视图，不验证transcript、签名、PKI或RCV.NXT。

## 真实来源、字段参考和复用

仓库外Windows mTLS捕获241/tls13为17帧，SHA256
`deadc7ec4bb9672ffeb96046565ef02b69009cd9c7d18b2258a2c113696c6999`。
独立244事实定位frame6/server handshake序号0/AES256-GCM-SHA384的EE，record43B；
CH提出http/1.1，原record物理span[1265,1308)。1134派生未提议h2-extra、错误版本扩展、
错误key_share三个控制；独立重算AEAD/TCP checksum，保留全部17原帧、TCP序列/确认、长度和
时间戳。版本故障用9字节inner padding保持record长度。这不是新实抓或合法peer transcript。

1138完整原捕获及三个完整控制重新TShark4.6.9双遍、空配置、明确端口Decode As，
事前独立编码定义的22组EE类型、消息/扩展长度、扩展类型及ALPN原字节检查全相同。
参考解出字段不等于认可消息位置，规范资格另验。1139共24次旧/新SDK、flow、CLI实际执行：
原捕获三导出全字节相同；三个新EE均malformed且AEAD仍true、版本仍0x0304、不发布握手成功
内容或后续HTTP。旧未提议ALPN/key_share后各仍发布2个HTTP；旧版本扩展已破坏后续解码。
四组完整flow输出均保持。

1147逐字节合成四个34帧控制：相同tuple先好后坏、先坏后好，两个接口交错的两种顺序；
显式tsresol=9保留纳秒。它们是原来源克隆，不冒充独立新peer。1148共12次执行，每例21事件、
两个独立TCP owner，84个引用均属各自来源/接口，正常HTTP2/异常0，四组旧/新flow相同。
1149完整组合双遍参考：两种顺序复用的字面字段全相同；两个接口例第二条EE均未在frame12
解出，strict仍失败。分开参考1138和源原字节保持；不将参考关联政策直接判成Wireshark错误。

## 验证和实际页面

证据均在仓库外`C:\tmp\yak-shark-ESP-SSH-UI-20261028`。PCAP、keylog/密钥、完整JSON/PDML、
原始帧、fuzz缓存和截图未入库；两个fixture ZIP保持原SHA256。

| 验收 | 实际结果 |
| --- | --- |
| 完整本地1137 | 289727pass/24历史optional skip/0fail；102新ID全部pass，所有父本地ID/动作保持。另两个既有Issue1043测试已在父CI清单，不算新增；stdout `d0efed439514809def9a74af49420f95984649270c29ecda50463bc3d3f193b6` |
| 原生矩阵 | 三套件、双server方向、CH缺失/无提议/大小写/未提议、finalSH释放、拥有副本/新流、真实AEAD、分record拼接、位置/版本/方向/epoch及预算；拒绝消息保持状态，记录序号独立消费 |
| 实际Windows1144 | 三完整捕获、66真实console状态，TLSv1.3和Loopback→IPv4→TCP→TLSv1.3保持；代码/具体错误分列、无虚构错误范围。每页完整43B原record、80×30/180×48、键鼠/Tab、CP936恢复及拥有进程关闭；主线程实际查看三个01-selected-field.png，不冒称全Terminal/DPI/IME验收 |
| 插桩1145 | 明确`go test -c -fuzz=FuzzTLSEncryptedExtensionsAdmission`；独立包络/位置/ALPN模型，6种子/38752执行。1CPU、总768MiB、最多2并发进程，第三进程拒绝正控通过；stderr `c3438beb2fc98ca3a07f9b299f36e50a7dbea12f865046457bd45c3ae3f13338` |
| 六完整大包1140 | Weber/Sharon共379455物理帧，SDK/flow/CLI全部逐字节保持；summary `2634fa375bf4be05959944a4a34588828672f0aecf9501d1f6587f557168d223`。输出不变不等于全部字段/会话与Wireshark相同 |
| 综合1155-02 | `dc226728cf0e73c5b9c10dafe747b7e221ad89d017c57500206e6e1ba622648e`；空闲38728658944B，高于8GiB底线 |

原失败保留：1126 helper拼写构建失败，1127才是有效red；1141预检误用typed前缀，1142沿用
IP专用导航偏移，1144按实际索引选择且原错误/范围断言保持；1146误假定微秒，1147显式纳秒
逐帧重读；1155汇总首次误按list读取已有dict，修正schema后重新完整核验，不当产品通过证据。

## QUIC复核、CI和剩余范围

1150/1151用本批SDK重跑四组真实来源域控制：interfaces/VLAN full/full均0差异，每域2022认证/
3坏tag/6尾部、4556来源检查；两组app-only仍strict exit4/2664差异，各2278来源检查。
1152各确认B的2007个UDP预览（1332个截断64B、675个完整短载荷），来源差异和借用QUIC/H3
声明均0，不等于完整QUIC packet解析/恢复。1124新跑12个combined/isolated双遍参考保留关联
政策差异及明确Decode As。没有枚举或猜测其他连接secret；N09继续开放。

父`912d8b6c8910039730e1f15e31f37a2f06dca3f3`已47约定检查、91执行、2状态全成功，实际
checkout/314824原始ID动作与事前清单一致；304新增及131084 required全pass/零required skip。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41409852472)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41409858314)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41409862112)。官方产物13125368544、17018595B，
SHA256 `59baa067918f1c4e3d3ded5c891c79bc747dded83008055ca986a7fb3d83979b`；
闭合1131 `d30dc67d993baf8d08c063c4319d4a19186c2458881485fba413b086bde1e860`。
本批最终head独立验收314926原始ID/动作、102新增及全部CI，不借用912成功。

70父项60有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，约16–32
AI实施小时，等待/新问题另计。N13子口径6有限完成/1进行中，P04未关闭；新缺口单列，
不减少原完整阶段/恢复分母。
