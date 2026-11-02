# 普通 TLS 1.3 PSK 初始认证边界：2026-11-03

M2-76的干净代码及最终本地构建为`e700fc4d7ef3341d755e706562cc05a609e2fc80`。
仍在唯一M2 PR #5371，包含main `d705e2a1a0468fc7fda23f2b0fead3ebecd2c431`；M1已合并，SMB继续跳过。
父`f85833d7e215c2dd10794093b145d4c042e80128`已经独立47约定检查/91执行/2状态、
315396原始测试ID、131084 required全pass/零required skip及官方产物闭合。
本批说明提交必须另验自己的最终SHA，父成功不代替新提交成功。

## 有限规则

[RFC8446 §4.3.2/§4.4](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4)
的普通已选中PSK替代初始证书认证。旧版即使完整观察到ServerHello选中PSK，仍接纳
CertificateRequest、Certificate、CertificateVerify及opaque CompressedCertificate，继续生成HTTP。
新私有模式来自完整接纳的ServerHello，不来自443端口、客户端单方面offer、公开字段或session ID。
TLS1.3、认证后的初始epoch才执行此拒绝；记录认证与正文/协议资格分别保留。
即使ClientHello缺失，完整ServerHello里的选择仍有证据；捕获gap不能抹掉已经观察到的模式。
正文/预算/协商拒绝采用试算提交，不污染模式。应用epoch的PHA保持独立。

[RFC8773](https://www.rfc-editor.org/rfc/rfc8773.html)允许另一个证书与外部PSK组合模式。
本批对ServerHello中观察到的extension 33保留保守例外，并测试合法构造的组合流程，
**没有完成该扩展的空向量、CH/SH配套依赖、提议资格和其他位置验证**，不宣称完整支持RFC8773。
只有客户端提出PSK、服务器未选中时，正常证书回退仍接纳；未知模式不凭猜测拒绝证书。
不把此合同称作完整握手成功、binder、签名、Finished MAC、压缩正文或PKI验证。

## 新抓真实来源与独立证据

在Windows本机用自有Python/OpenSSL 3.5.8、TLS1.3/AES256GCM_SHA384、ALPN http/1.1
建立四个实际连接，两份完整捕获各46帧；捕获进程只过滤自有loopback服务端口。

| 来源 | 实际行为 | 捕获SHA256 |
| --- | --- | --- |
| live1324 | 首次完整证书握手，第二次真实恢复；双端backend均为`[false,true]` | `25dcdeac1face644c86b7678015c0f6b7a4d8946dafcfab98554cfdeff9b588f` |
| fallback1330 | 轮换服务端ticket context；第二次仍提出前次票据，但服务器不选择，双端均`[false,false]`并正常证书回退 | `c8c9ac5517e5214d6bfe08d7ade2ce28bbe2a666743cc773e39ec59c9707cf99` |

1320首次工具尝试未完成orderly TLS关闭，实际没有恢复，保留失败结果，不算成功恢复样本。
1328在运行候选SDK/参考之前，逐原始TCP字节重组并独立核验38条AEAD记录，其中18条handshake记录、
3个CertificateVerify签名、8个Finished MAC及四份完整请求/响应。两次第二连接的identity均匹配
各自首次连接已认证NewSessionTicket；只有真正恢复的第二个ServerHello含选择。
这不验证binder，也不让产品仅凭keylog就声称已经关联票据或评估信任。summary：
`670a4cfcc77b5d208c00d19da28589035c109b163bea2d654081249b49385fad`。

1338在真实恢复连接的第29物理帧，用同长度AEAD重新编码4种违规消息，完整保留46原帧、时间、
TCP序号/ACK/长度，校验TCP checksum和全部record贡献跨度。目标record均43B，保留16个
坏tag/nonce/key/AAD拒绝负控；四个修改transcript的8个Finished MAC均不成立。
这四个是故障注入，不是合法对端握手；小证书/压缩正文仅作策略向量，不冒称合法X509或压缩流。
manifest：`1c8d16d67f6607b084a8a95a9eff7e9aeeebf6308f4ea67ede2e0af4118b1188`。

## 逐包、字段、连接与参考差异

1339共36次新旧SDK、CLI、完整flow执行。两份正常来源三类导出均逐字节一致，每份HTTP4；
四故障旧版HTTP4，新版只保留无关首次正常连接的HTTP2。目标均MalformedMessage，
同时保留TLSv1.3、PSK Selected、AEAD成功、完整43B重组来源；拒绝正文和成功别名不发表。
四份完整flow均与旧版一致。summary：
`55a783803069c7ea52a9a80519ef7d984a2383f72cc982c15cf2c551f52b5062`。
1348另核四个真实ServerHello的PSK选择/未选择、identity长度/hash/age、binder未验证、typed来源，
以及四个拒绝事件不发表正文，summary：
`658c14a038c17e81008cd661059bb5988304cfbd13369a5f4aa2a088aa620f87`。

1343/1344把故障PSK连接与另一真实完整回退连接组成24个profile：4故障×tuple复用/接口/VLAN×双顺序。
只将正常源TCP端口和checksum平移到同一tuple，保留TLS内容、TCP序号/ACK/flags；明确规范化时间，
不是新的独立抓包。每组46物理行/21事件，96次运行、504个引用逐一落回所属源/接口/VLAN域；
两个owner分开，正常HTTP2/错误0，回退不继承PSK模式，完整flow与旧版相同。summary：
`0d955bec4803037888480751040fba786cdc2c4704a82095e053019e3400979e`。

1340对六个完整来源新跑TShark 4.6.9双pass、空配置、显式自有端口解码及各自keylog，
1345核对全部74条初始握手消息、393组来源字面：383原样一致，9个零长度context被参考省略，
另1个无效单字节证书源为`01`而PDML证书value为`0100`，原差异完整保留。
1341原严格核验因此失败，未把期望改成参考输出；1345明确记录该故障向量的差异。
TShark显示违规消息，不代表它验证普通PSK认证政策、修改后的MAC或证书合法性。
字面summary：`1250f8e65c2f0ede4dddbd1d95e16e8a44ba8ee418e323ec6a6a0b6caaa2b263`。

1346另跑24份完整组合参考：8个tuple复用profile消息类型及PSK选择一致，8接口与8 VLAN
组合不能完整还原两个来源的预期初始消息/选择，16个原始失败不删除、不称作Wireshark bug，
也不以隔离源成功代替组合参考成功。产品继续按要求隔离域；共享捕获的完整会话政策差异仍开放。
summary：`800724ba95f802c92e6adc45be802cff7089fc4132c59797c5e468af08be937c`。

## 回归、实际页面与资源

| 验证 | 实际结果 |
| --- | --- |
| red1321 | 当时79个新ID为30pass/49fail；后加控制不冒称跑过此红测，stdout `b5c07b37df19b8ae142170c04d6ce4c5a12b055688a79b0ea40c81a52e3441dd` |
| 134新增 | 三套件/双客户端方向，完整选择/仅offer/缺CH/仅CH，四种初始消息、私有状态/公开字段反改、gap、拒绝原子性、PHA、42个真实AEAD拒绝和六fuzz种子 |
| 最终完整1331 | 干净e700：290431pass/25历史optional skip/0fail，整个bin-parser/pcapx/CLI/CI脚本；全部父本地ID及动作保持，stdout `935741ded346d293dffc354cf449f7d4da8082c97b6bc8ec1a923f30f95db330` |
| 插桩1334 | 六种子38299次，1CPU/总768MiB/最多2并发进程及第三进程拒绝正控；stderr `f7f0e9357e4425ef50ca8106e3f3a144f7156b0fd99cadda602ba5d42a89b2f1` |
| TLS1.2实流1350 | 重跑此前实际19帧捕获，6次新旧三类导出逐字节一致；summary `838a66eebcf3be6a77fa07b7dad57cd782be6fb4163abc864d7d5e32e3ce244f` |
| Windows1342 | 四页88状态，含12等待/76交互；主线程看过四页。默认半窗完整具体错误、TLSv1.3/record AEAD、PSK context，错误无借用字节范围，整43B读回、mouse/Tab、80×30/180×48、CP936恢复、拥有进程关闭通过；closure `4e5f53d2567849ffa0b8bdf997b5ddd47eab2038ada29bc6cf3537a6846a391f` |
| 公共1335 | 两大捕获379455物理帧，最终六次SDK/flow/CLI完整导出均与父验收运行逐字节一致；summary `c65f7184a157eb497de3a51bd58156a36c3ba78ecb5024234eed0c252ee45a9f`；不等同全部Wireshark语义正确 |
| QUIC1336/1337 | 原四域contract/checker不变均0差异；runtime `166b7a471232c9c06a45d4ca9452975eae0e7ca53899ee8931cf2e72d6b5e2b1`；原checker summary `2b696ad565620f9b432f34905dd8fee9c711395502c9f78eb52219cec282d3cb` |
| 综合1352 | `7320eb4fd952c9cdcc7481ef906e647a2b5066c60075a6a563aa7e757e67427a`；138小源原生执行、30新完整参考，来源/动作/构建/产物哈希逐项守卫，硬盘空闲约16GB，高于8GiB底线 |

保留1322的三个missing-CH失败：移除错误的“必须先知客户端角色”限制后，1325相关3024pass；
完整ServerHello已经足以证明普通PSK禁止初始证书，不需要猜客户端方向。
1334第一次在coverage二进制构建完成前启动，未执行fuzz；待构建完成另存重试日志，实际38299才算验收。
没有改旧断言、ZIP、工作流、required配置，也没有把跳过当通过。

Go1.22.12，events/flow/yak二进制SHA256依次为
`5349e3247c21710fdf83715afaa231ca0a03ea2c6c9b51a7d117a7212ed5f177`、
`7be1ab073feb2bbfac521f7ddb89930a36666515e0560b955411e214f1074a9f`、
`b7517163db13fb8418a2c722e03be0e382d5ae782aca1d4c290e98d3aef8116b`。
Go/公共回放3CPU/2GiB，小源/参考2CPU/512MiB/单进程；Windows1CPU、进程512MiB、总1GiB、
最多8进程、Go heap256MiB。原生控制台单元验收不代表所有Terminal/DPI/字体/IME均已覆盖。
全部捕获、keylog、私钥、完整导出、PDML、截图、fuzz和CI产物均在仓库外
`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`；两个已跟踪fixture ZIP哈希及许可不变。

## 未关闭与最终交付门

主表76项66有限完成、7进行中、3 SMB搁置，仍10未关闭；非SMB N01–N17为13有限完成/4开放，
N09/N10/N13/N14约16–32 AI实施小时，不含CI/构建/回放等待及新问题。
N13.P04完整初始消息/必需性、重新进入hello的阶段边界、PSK与证书类型/压缩及扩展依赖、
RFC8773向量/配套/位置、binder/外部票据资格均未关闭；高级TLS/QUIC丢包恢复/迁移、六fuzz
TCP连接政策、完整IP重组和字段、SMB任务书S01–S06继续保留。

最终说明提交冻结父315396原始ID/动作加134新增pass，目标315530原始ID（315505pass/25历史optional skip），
131084正式required全pass/零skip。官方产物的大小、SHA/CRC、原始唯一run/terminal、checkout和
最终SHA全套CI必须独立闭合，自动draft skip及旧SHA成功均不能替代。新结果以PR里的精确SHA/CI链接为准。
