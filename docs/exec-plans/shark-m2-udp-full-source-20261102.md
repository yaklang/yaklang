# 完整未识别 UDP 的字节来源：2026-11-02

M2-74/N09.R01关闭一个有限来源缺口：应用识别失败后，完整且未超消息预算的UDP载荷
被截成64B probe，导致拥有字节页和按完整原包匹配的差分检查缺证据。生产修改为
`b49fa5825929f43ad36efc060fa4211f9d8e8bec`，三个既有测试辅助函数随后加强为完整源断言；
最终干净代码、构建、回放、Windows及完整本地验证均为
`26f773d7da112b2f3dd9ece929d24ddb8c40abac`。仍在唯一M2 PR #5371，SMB继续跳过。

## 有限合同

只在UDP carrier已通过现有资格检查、事件为`unrecognized`且载荷不超过`MaxMessageBytes`
时拥有完整载荷；`ProbeBytes`继续限制识别工作，不再决定这个完整来源的字节数。
`Completeness=datagram`只表示承载数据报完整，应用协议仍未知。`Protocol`为空、原拒绝诊断、
会话和流不变；公开字段被修改后也不能绕过私有`nativeUnrecognizedUDP`拒绝门得到成功解析。
截断、非法承载、超消息预算保留原有界预览/错误；TCP未识别前缀合同不变。

UDP长度包含头和数据，不能靠端口或前几个匹配字段证明应用身份，参见
[RFC768](https://www.rfc-editor.org/info/rfc768/)。这里没有新增QUIC、TLS、DHT或RTPS识别，
没有因字节更完整而创建连接、认证成功或虚构完成帧。未知PDU仍不发表`CompletedIn`。

## 真实新抓与参考

1252在本机Windows loopback真实交换六种长度的UDP echo：0/16/64/65/1200/4096B，
请求响应共12物理帧，载荷合计10882B。它是自有的未知应用数据交换，不冒称六种新协议。
原PCAP为11482B，SHA256
`0df2633c1058fc4918da232bedd542e4677b36a5ec0f021064f9d7bd6b6647f1`；
producer SHA256 `23665a15e59fb4d92d9ab59c8e3ae176510c7e8d36f468d821dae77068efcdf0`。

1264实际运行旧d853/新26f的SDK、flow、CLI共六次。1269独立复核这些不可变执行产物，
零新增原生执行，并新跑完整TShark4.6.9双遍、空配置、不使用Decode As。12帧UDP端口、
长度、checksum字面值和完整Data与独立线上源一致，零长度Data省略明确处理。IPv4中的UDP
checksum为零的意义是未提供，不把producer的`verified_or_absent=false`误报为校验失败。
SDK和行只允许完整Raw、来源SHA和承载完整性变化，其余所有字段及完整flow导出相同。
summary `9d21f22452143a8281db91023cce371ad96e75e4f6b5f3f0b123fcf230261743`。

## 两个完整公开捕获

在读取候选的大捕获输出前，1266从原始PCAP字节和已验收父事件固定全部8119条未识别UDP
来源。逐项校验IP版本/头长/总长、IPv6扩展范围、UDP端口/长度/载荷边界及单一物理引用，
并验证旧前缀。没有用候选Raw生成期望。该集合无需IP碎片重组；域元数据保持旧账本，
不把本次源检查当所有封装分类语义的重新验收。

| 来源 | 未识别UDP事件 | 比旧预览长 | 完整载荷字节 |
| --- | ---: | ---: | ---: |
| Weber The-Ultimate-PCAP | 3243 | 1906 | 1526217 |
| Sharon ultimate_wireshark_protocols | 4876 | 2927 | 2837829 |
| 合计 | 8119 | 4833 | 4364046 |

两个原捕获SHA分别为`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`、
`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
源清单summary `9977df65ca5f5d437b5365bc91e92afe05c8ebc379e2a713ae2ee0886bfdad80`。

1267用最终构建重新完整SDK/flow/CLI，共六次实际执行、379455物理帧。1272对全部306855
SDK事件及379926个CLI物理/发布行逐对象核验：8119个候选各只出现一次，完整字节、来源SHA
和datagram完整性恰与事前原始来源一致；4833个扩展载荷全部正确。其他298736个SDK行、
371807个CLI行保持逐字节相同，所有协议/status数量和两份完整flow导出相同。
只对明确来源建立精确变化模型，没有数值重映射、任意忽略键或用总体数量抵消逐包错误。
summary `9e6d6f1234971f36ecf1309c3bd75a16939ed84cd8d438519a8653e9a959af42`。
这是有限字节来源验收；没有宣称全部协议、字段和会话与Wireshark一致。

## 原QUIC严格门

1275重新运行最终26f的四组既有接口/VLAN控制；1276使用原contract73及原checker73，
没有修改标准。四组均exit0、0 mismatch、0 coverage gap。full/full各4556个来源保护，
每域2022认证包、3坏tag无PN/明文、6未保护尾部；full/app-only各2278保护，未知域的
2004个opaque application包与3个坏tag都保留完整原始UDP，不借另一域的ALPN/PN/HTTP3。

之前两组app-only各2664个严格差异来自1332个长载荷被截后无法匹配，以及对应1332个来源
缺映射。现在补齐真实载荷后原门直接通过，没有补QUIC标签。原失败/64B预览审计仍保存。
summary `2b696ad565620f9b432f34905dd8fee9c711395502c9f78eb52219cec282d3cb`；
contract SHA `b09a91c894f3c4ca46376f8e446023e5cba9bac72e2541b0e0b0ebe073457d3b`，
checker SHA `15b543fb9cb1b3e9a0c4e66f646c96cf6693febc09bf3d59c752ab100236665e`。
N09.R01有限关闭，CID/epoch/STREAM/QPACK/0RTT及高级恢复仍另需语义证据，N09整体开放。

## 测试、实际Windows和资源

| 验证 | 实际结果 |
| --- | --- |
| red1246 | 4pass/24fail，真实重现完整来源被截；stdout `f1ce58b15114885a4fa9158d0636612d642f9215ca17f60974c45c07d43b7ce1` |
| 新增测试 | 46个精确ID，全pass：0至65507B、直接/私有路径，截断/非法承载、128B消息预算先于完整复制、借用载荷修改及公开身份毒化负控、六个fuzz种子 |
| 完整1261 | 26f，290078pass/24历史optional skip/0fail；bin-parser、pcaputil、CLI及CI脚本；stdout `acb7dbdbe506f51b76f194dacb32eae4d01c86c6a4f60981470c5bb4ad17d843` |
| 补充1280 | 同一干净26f，PCAPDB/inspect/pcapx/arpx的100pass/1历史optional skip；stdout `76b815be7a2e89060c270f3252a386dfcac52f4b1fa2a1147c74abb73121ddab` |
| 合并本地口径 | 290178pass/25历史optional skip/0fail；与上批1213的全部本地ID/动作相同，另有46个新增pass，不把两个执行说成一次运行 |
| 插桩1263 | 明确coverage构建，六种子71701执行、全通过；1CPU/总768MiB/最多2并发进程，原第三进程拒绝正控保持；stderr `317194b9fb0b7679af7e91867098ce3fcdab860512fbb0dd8c1f4969a3b269b2` |
| 实际Windows1274 | 22状态，含3等待/19交互；实际PCAP/Yak运行，选中PDU拥有源1200B，翻页0..1200全部与原payload一致，主线程查看首尾两张PNG；mouse/Tab、80×30/180×48、CP936恢复及拥有进程关闭通过。summary `cecb406ca37390648d9eada2740da0b3229928ad43b218893226960b01ec66e8` |
| 综合1282 | `20cb051a2f9ad6336c046aadf39950de9118e6c9f03527d2058d8a7b0e86b6c3`；空闲24633987072B，高于8GiB底线 |

1250完整运行的120个旧测试失败保留：它们依赖原64B来源合同。26f只加强三个DHT/RTPS辅助
函数：完整wire相等并保留原前缀断言，冻结ZIP原期望仍核对，其余拒绝、协议/会话、类型和
不可变Decode断言未删。没有修改样本、取消测试或降低协议识别要求。

验证工具失败也保留：1264检查器误把省略的`CompletedIn`当必有键，1268/1269按实际schema
重新逐字段验证；1273预检误把Source1当`Sources[1]`而非额外源数组的第一项，1274修正
工具索引后完整实际运行。1277/1278证明工具误用CI与本地不同范围/宿主可选动作，1279的
嵌套集合计算停止，1282线性合并原全部本地ID并明确CI宿主GUID skip/本机pass的历史差别。
CI预期清单仍原样保留父全部ID/动作，不拿本地pass覆盖CI历史optional skip。

Go1.22.12，最终events/flow/yak二进制SHA256依次为
`5114e092eb7240b3bef777301f558831467aedcad3513f4d16d63dc8699c3bf0`、
`1d5acac46c798e67bd7f6827118e76bd1a3aa41e1bf758f102fb74ebd153d886`、
`0600580acbe8654de89d0a852aea58497d52a8a8109c74885337c36a8c09f659`。
完整SDK/测试3CPU/2GiB，QUIC2CPU/512MiB/单进程；Windows1CPU、进程512MiB、总1GiB、
最多8进程、Go heap256MiB。Windows仅QA helper在仓库外增加精确根行选择，产品代码未为
截图特殊处理；这是原生控制台单元验收，不是全部Terminal/DPI/字体/IME验收。
两个fixture ZIP不变；真实/大型捕获、密钥/keylog、完整导出/PDML及截图全部留在仓库外。

## CI与剩余工作

父`97112f9af350c6bcf36a465fe46634276d161f78`已独立47约定检查名、91实际执行检查、2状态
全成功：[traffic](https://github.com/yaklang/yaklang/actions/runs/41442647790)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41442654303)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41442660960)。315231原始ID/动作、246新增、
131084 required全pass/0skip及实际checkout均闭合，官方artifact13142470738/17098576B，
SHA256 `a67a38c0b6536396561d597cba55b081066258abdfc48e78b1aa74a42afc3c95`；
综合1257 `9d90758d94b1c67727b156df832bbcac7f7e083b1d4cbbb4a2a93a0605ceab7d`。
本批包含说明的新最终head须另验315277原始ID、315252pass/25历史optional skip，46新增和
131084 required全pass/0skip及全部CI；父成功不认证本批新代码。

当前74父项64有限完成、7进行中/3SMB搁置、10未关闭。非SMB17包13有限完成/4开放，
仍为N09/N10/N13/N14，粗估16–32 AI实施小时，构建/CI等待与新发现另计。客户端及完整
TLS阶段、压缩证书/协商依赖、TLS/QUIC高级恢复、TCP代次和完整IP恢复/字段仍开放。
SMB S01–S06交由承接者独立复核，不由本项替代。
