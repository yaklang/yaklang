# ServerHello 字面约束与拒绝记录展示：2026-11-03

M2-77最终本地构建及展示代码为`98fd2445670c55901e8f85efa4bd12c41616cc57`。
仍在唯一M2 PR #5371；包含main d705，M1已合并，SMB继续跳过。
本批父`4153bbd7b7fee2d3b2d9833801c96423858000a3`已经独立47约定检查名、91最新执行检查、
2statuses及315530原始ID/131084 required全pass闭合。父自动Essential首次HTTP响应劫持测试
token不符，原日志/产物保留；同SHA失败任务重跑成功，未改断言或代码。
[父traffic](https://github.com/yaklang/yaklang/actions/runs/41485063054)，闭合1358 SHA256
`604c6ca7bb334a02caff8499a60823d38274f63783204acf165e2e35c3fc8e34`。
本批最终说明提交必须另验自己的CI，父成功不能替代。

## 原漏检、原子状态和观察边界

[RFC8446 §4.1.3](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.1.3)规定TLS1.3
ServerHello的legacy_version为0x0303、压缩方法为0。这两个字面约束不依赖ClientHello。
旧版却只在观察到ClientHello时检查，服务器单侧捕获能错误接纳不同legacy版本或非零压缩值。
b611把自足约束移到公共路径；只有版本提议和session ID回显等关联检查继续依赖完整ClientHello。
缺ClientHello不虚构回显错误，合法单侧ServerHello和旧版TLS仍接纳；HRR的自足字段与缺上下文
分别报告。失败沿用试算提交，不接纳协商版本、PSK模式或成功握手正文。

实际回放另发现：首个记录已明确报告legacy参数错误，同包后续停止解码诊断却使协议列降为TCP；
修复列表后，详情和原包规则树又仍写“handshake not captured”。最终同时修正这三处：保留
`TLS`、完整ServerHello被拒绝的原因、声明版本/字段与“协商未成立”；其他方向或后续ACK不继承身份。

新增无状态`ObserveTLSServerHelloRecord`复用原生hello语法，要求单完整明文record、恰好一个
完整ServerHello及完整外层扩展向量，受元素数和record长度限制。只返回字面观察，不应用协商规则、
不改输入、不证明认证/对端/信任，未支持扩展的内部内容仍opaque。展示还要求匹配的原生错误、
完整自有record SHA、非零PDU/flow/generation、同域贡献引用及一致record元数据。
短前缀、损坏向量、客户端消息、另一来源/域、伪造协商字段都不能得到此投影。

详情中的声明版本明确标为未协商；legacy版本、cipher suite和压缩字段只定位到自有明文record，
错误说明没有借用字节范围。此投影只覆盖上述单record合同，不承诺所有碎片/同record多消息的
拒绝记录均有此展示，也不把观察API当完整扩展内部语义或握手验证器。

## 真实来源、残缺捕获与逐包验证

复用本轮此前实抓的1324真实完整/恢复两连接及1330票据被拒回退捕获，不重复计作新抓包。
独立1328已验证原始38 AEAD、3签名、8 Finished MAC和实际peer明文，summary
`670a4cfcc77b5d208c00d19da28589035c109b163bea2d654081249b49385fad`。

1361从完整与恢复连接各保留服务器单侧合法源，恰好删除原ClientHello物理帧；再各生成
legacy_version=0x0301/0x0304及compression=1/255四个字面故障，共10个22帧来源。
完整ServerHello落在第5帧，保留所有其余时间、TCP序号/ACK/长度及精确record跨度，修改TCP校验和。
这是消息丢失和字段故障控制，不是新的真实peer或有效修改transcript。manifest
`fcda3787be95c9190cb7d65f498298fb0828678268c40a9a49dd17791644800d`。

最终1418的72次新旧SDK/CLI/完整flow运行中，两合法单侧源和两完整真实来源的三类导出逐字节相同。
8故障旧版接纳ServerHello，新版均MalformedMessage；保留完整1215/1221B来源，不提交TLS1.3
协商或PSK成功状态。列表/详情/字节来源一致，完整flow均相同。缺CH的故障源原本也不能正确解密
HTTP，不能把本批说成“修掉了8个HTTP误解密”。summary
`a67b863947e5749e7b8bec787f8bb200ac6f9e7998f4a8257799603a26d30415`。

1363另跑10份完整TShark 4.6.9双pass空配置参考：ServerHello type、legacy版本、压缩和声明
selected_version共40组原始字面一致，summary
`bedb1cbdf24a6b6ba9f81e78232bbc86c8b83b17426501bc31574ce1df79d5ec`。
参考会显示声明的TLS1.3以及这些字面字段，不等于它与原生引擎在约束拒绝、协商状态或会话政策上等价。

## 真实不同序号的连接复用及域隔离

1404把两个不同真实连接的原始SYN序号用于12组控制：两故障×复用/接口/VLAN×双顺序。
只将正常来源TCP端口和checksum平移到同tuple，保留TLS及TCP序号/ACK/flags，并明确规范化时间；
不是独立新peer。manifest `054bee5753405a750707d62ab775c82c6a0268abe056c0da608e9c7c0e8c177f`。

最终1417的48次运行均45物理行、两个owner，正常HTTP2/错误0，204个字节引用精确同源同域；
132个坏连接后续无payload行保持TCP，没有把拒绝记录投影变成会话粘性。
12个缺口诊断按旧owner保留：10个capture_close非物理记录、2个在新包处理期间发布于
`published_pdus`，有明确analysis-only作用域、没有伪造当前包字节引用，也不污染新包协议列。
完整flow与父版相同；summary
`b799c09ef948494c8125de1f922b0c1ac34f1d1d3bf45e5e3b29a1630c4f7dc7`。

1413另跑全部12个完整组合参考：4个tuple复用的限定字面及初始消息一致，4接口/4 VLAN
仍不能完整得到两个来源的预期消息/字段，8个原始失败保留，不称作Wireshark bug或完整政策一致。
summary `37b6caec12aebfdfe24c53e3cc1199abf023e24b158a5b4b761268837798b901`。

保留早期工具结果：相同ISN的复制源不能作为新generation证据；1391/1400/1403没有冒充真实新连接。
1405错误地只找独立EOF记录，忽略已有的physical-row `published_pdus`；正确审计两种已声明
作用域后1417通过，未为此改写连接政策、旧断言或出版语义。

## 回归、实际Windows与健康检查

| 验证 | 实际结果 |
| --- | --- |
| red1359 | 初始114个新ID为65pass/49fail；后来增加的19个HRR控制及展示测试不冒称跑过此红测。stdout `e1ced387f925cb06a5cc9aabb3d93912d50cfec539e7e4d95a76c0fc96ed887c` |
| 186新增 | 133个字面/缺上下文/HRR/fuzz ID，23个完整record观察API边界ID，30个展示/来源/逐字段范围ID；没有修改既有测试或弱化断言 |
| 完整基础1364 | 干净b611，整个bin-parser/pcapx/CLI/CI脚本：290564pass/25历史optional skip；stdout `79a0c833cfeb29d4c428950adbfe1eb9db2a596df2c5101698f6680285afc95e` |
| 后续精确补验 | API加入后的EDE相关pcaputil 5385pass/0skip，最终干净98fd完整CLI 1645pass/2历史skip。1428逐文件验证基础原生代码不变、仅增加API及测试、最终CLI独立全包运行；合并唯一ID为290617pass/25历史skip，所有父ID/动作保持。这不是宣称在98fd重跑过一次全部基础测试命令 |
| 双插桩 | 原生字面72955次、相同最终API代码74693次，各六种子，1CPU/768MiB/最多2并发进程及第三进程拒绝正控；两份stderr分别 `79c036124666c5162715a9aece9b8870c6b33522b4852dc37e67ffbacaf5b0c6`、`d2c36a50e12206f51b9a95958ddc3d4d725b7723bfda39634d3ce06defd8cc88` |
| TLS1.2实际来源1424 | 19帧旧实抓源，6次新旧SDK/flow/CLI导出逐字节相同；summary `d1e2c3a74ddfeb299080863c39e44ec3b25b86b6218abc943c3d8e130aceb603` |
| 最终Windows1419 | 两实际错误页44状态；消除列表、详情和原规则树的错误“未捕获握手”，完整1215/1221B读回，6组声明字面范围守卫、错误无范围；CP936/resize/mouse/Tab/拥有进程关闭通过。closure `b99cc3594df42e081306d8c8585147b41c85841978002a4087b6269c41ff0884` |
| 实际点击1425 | 两页40状态，真实选择legacy version和压缩字段，只高亮自有record的[9,11)与[78,79)，实际为0301/01；不是捕获外层偏移或错误说明范围。主线程查看四个最终页面。closure `3fd31f95e1723aa0d7623ea976ff8f7b4facf514e7117c590939bca61fc24704` |
| 完整公共1420 | 最终98fd两大捕获379455物理帧，六次SDK/flow/CLI完整导出均与上批已资格化输出逐字节一致；summary `d9a01b88c2d3ce596b46a3c73e715a4c67ff8a2b7daa43b8f8cf951ac020b560`。事前1392扫描306855事件的本投影宽候选为0，不拿稳定输出当全部Wireshark正确性 |
| QUIC1422/1423 | 最终构建重新四原域profile，原contract/checker未改，全部0差异；高级恢复/迁移仍开放 |
| 综合1428 | `42219fec776e7a113fbbdc0239be486ce6a6d51f3f779c15e69949c53603144f`；126小源原生执行、22新完整参考、84 Windows状态（12等待/72交互），所有来源/ID/构建哈希守卫通过 |

保留工具失败：1358新测试误把ProtocolErrorKind作为error，未编译，不算红测；1381有一项新测试
误期待观察API验证opaque key_share内部语义，改为其声明支持的坏ALPN向量，未改旧测试；
1408/1409分别重现错误详情说明及原规则树的同类说明，1414和最终1416修正复验。
1397物理行查询没有排除EOF记录，1398分开处理；1421使用工具不支持的零byte-page参数，
1425以受支持参数实际完成高亮和恢复，不把失败截图或仅worker字段当实际页面验收。

Go1.22.12；最终events/flow/yak二进制SHA256依次为
`afa3e5733bd151be07bb6f153920118eccad1d6d47bd259492367ee4529709ae`、
`7e35412eb11f095cedceadbef8c31d7307ee7805321280e1a6717fa099ebdba1`、
`b9bdf43a283713d701b28fc536341ede7f345efd725a978c712d6214c98649d2`。
完整Go/公共回放3CPU/2GiB，小源/参考2CPU/512MiB/单进程；Windows1CPU、进程512MiB、
总1GiB、最多8进程、Go heap256MiB。Windows验收是实际原生控制台单元，不覆盖全部Terminal/DPI/IME。

硬盘接近8GiB预留线时，对已关闭、字节完全相同的24份旧公共导出做NTFS硬链接去重，约6.13GB
重复字节，共同路径/内容/SHA保持，1428重新验证。原始捕获、keylog、密钥、fuzz、日志和完整
导出路径均保留；验收时约11GB可用。所有这些证据留在仓库外，两个受版本控制的fixture ZIP不变。

## 未关闭及最终提交门

主表77项67有限完成、7进行中、3 SMB搁置，仍10未关闭；非SMB17包13有限完成/4开放，
N09/N10/N13/N14粗估16–32 AI实施小时，不含CI/构建/回放等待和新问题。
完整初始阶段/必需消息、protected hello/重新进入、非本合同的记录拆分/多消息展示、扩展内部
语义/配套/位置（包括RFC8773）、binder/证书类型/压缩、TLS/QUIC高级恢复和会话政策、
六fuzz TCP政策、完整IP字段/重组及SMB S01–S06继续保留。

本批最终说明提交事前冻结父315530原始ID/动作加186新增pass，目标315716 ID
（315691pass/25历史optional skip），131084正式required零skip。必须在最终SHA独立验收
全套CI、官方产物大小/SHA/CRC/checkout和唯一run/terminal；旧提交、本地组合验证和draft skip
不能替代。最终结果和精确链接写入PR及交接Page。
