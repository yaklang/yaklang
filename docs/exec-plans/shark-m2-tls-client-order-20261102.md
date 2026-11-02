# TLS 1.3 客户端认证观察顺序：2026-11-02

M2-75补齐初始客户端认证块的有限观察合同，最终干净代码/构建及全部本地运行为
`fbda7244c011ecbfd38bb34f899b813a81f7570f`。仍在唯一M2 PR #5371；SMB继续搁置。
此前`a0d6679da`只把74说明里的“IPv4 checksum”改准为“IPv4中的UDP checksum”，不改变运行代码。

## 有限规则与原误接纳

旧实现已经检查消息正文、角色/epoch及PHA，却会接纳客户端重复Certificate、
CertificateVerify后回到Certificate，或在已完整观察到空Certificate后再接纳CertificateVerify。
[RFC8446 §4.4/§4.4.2.3/§4.4.3](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4)
规定认证块的Certificate、CertificateVerify、Finished顺序；空客户端证书列表跳过签名消息。
记录AEAD认证与消息顺序、签名、transcript及信任评估是不同证据。

只约束TLS1.3、已知客户端方向、认证后且仍在初始handshake epoch的已接纳观察。
任意缺失前缀/中间消息可以保留，不凭捕获缺口虚构缺少必需消息的错误；gap也不能删除
已经观察到的顺序。正文/预算拒绝不推进状态，空列表事实私有保留，不受公开字段修改影响。
单连接保存客户端顺序及空列表事实，复制试算成功后提交；不继承另一方向或旧连接的状态。
应用epoch的PHA请求/响应继续使用独立关联和生命周期。opaque CompressedCertificate不
充当已解析Certificate。未在本合同验证所有初始消息、完整协商依赖、签名/Finished MAC或PKI。

## 原始来源、密码学及逐字段参考

使用最近1218实际抓取的自有Windows mTLS1.3/1.2会话：17帧/6598B及19帧/5255B。
本批重新运行这些真实来源，没有把它们重复算作两次新抓包。TLS1.3源SHA256为
`01439c5da925f21fbd2aa3d5983de6375d83cb5f4759b6396df2b653a0811752`，TLS1.2为
`c113f209efdd5953a68c78c090bd322e5f39cf7df10c4136cda54bf13fcaed7d`。

1291从线上原字节及独立keylog推导handshake密钥，重加密三个同长度故障：第二个Certificate、
先CertificateVerify后Certificate、空Certificate后原CertificateVerify。保留全部原帧、时间戳、
TCP序列/确认及record长度，只重算必要checksum；原CV record为286B，空Certificate用inner
padding填满。第三例错误位于未改动的原CV record，不把最后一个改动record误当错误目标。
manifest `6e179a9a81a515bc4059330f0f50d63da2f0847dcbd1d677ee5cbcdf1178562b`。

1304独立重组原始初始握手、解密8条handshake record，使用原证书公钥验证服务端/客户端
两份CertificateVerify签名，并验证两个Finished MAC。原始握手均通过；三个修改后的client
transcript MAC均不匹配。这些是自有验证脚本的证据，产品仍明确标注签名/MAC/信任未评估，
也没有声称故障样本是新peer实际接受的有效transcript。summary
`faf5e2d841540121ddf73c65fd8c4a0db44956e9fa801bdf2bce731ca5584799`。

1294实际旧26f/新fbda的SDK、CLI和flow共24次：正常三输出全字节相同；三个故障旧各
HTTP2、新各HTTP0，完整TCP flow均相同。错误record保持AEAD=true、TLSv1.3及原286B拥有源，
不发布成功握手正文。summary `0a3e153b53432b07d0413566ae19a279b78497f0a2d129ef43c57c012952df89`。
1312又实际旧/新回放TLS1.2三种入口共六次，全部逐字节相同，作为版本范围控制，
summary `e5a3d9efab6e430104c91bac7a65b66d3ae785b9b7470d11b47937b211dc7d78`。

1306新跑四个完整TShark4.6.9双遍/空配置/显式原端口Decode As。源期望在参考前独立固定，
63组客户端类型、长度、证书向量/DER字节、扩展长度和签名算法/长度/签名字节逐组比较：
58组原样相同，5组为已明确的零长度context字段省略，raw expected=[""]/observed=[]仍保存。
1311直接使用实际SDK的类型化`field_values.values`，旧/新26组字面alias及拥有正文均与
独立源一致，derived字段没有借用外层密文字节范围。没有新增SDK执行或用数量抵消字段差异。
reference summary `f72031e6636752a224db06021b0e7519375b945d305ce5d6961b5576363eca4f`；
native summary `7f199aeeeab03422aaf6ebd965c47e710ef1b583238f92a4f6bcdfafadf4fa59`。
TShark显示故障正文不等于它认可消息顺序或签名，本批不宣称参考也拒绝这些故障。

## 连接复用、接口与VLAN

1307将每种故障分别与正常来源组成先好后坏/先坏后好、双接口交错、双VLAN交错，共18个
34帧控制。只是原始来源克隆，不是18次新独立实抓；link前缀变化外的IP/TCP字节保持，
PCAPNG明确纳秒精度。manifest `618002ef2408433bf5bea57e37475f17cbaeb2dee38ed73c0ebddcdb0800fbd9`。

1308实际72次SDK/CLI/新旧flow运行，全部各28事件/34物理行、两个独立owner。正常源HTTP2、
故障源0；504个引用逐一落在自身的原始帧、接口及VLAN域，目标record/来源/状态与列表PDU
一致，完整TCP导出与旧版相同。空证书故障的CV与正常CV密文字节相同，必须先用原始引用
及域确认来源，不能拿同一个record SHA当连接身份。summary
`b7c4d0a054f4f909d07cbbc81c1b02ff29faf2fee454d7d1a14e8a8bac4afccf`。

1309另跑全部18个完整组合参考：六tuple复用的类型和限定字面profile一致，六接口及六VLAN
组合不能完整得到两个来源的预期客户端消息，12个失败原样保留；summary
`2158824dab733c4b91636bb5399996956bc497e1ca0e55aa13b0b7b2b655595a`。
不将其称作Wireshark bug，也不把隔离参考或原始源成功当组合参考已通过。产品保持用户
要求的接口/VLAN域隔离；共享捕获下的完整会话政策一致性仍开放。

## 回归、fuzz和实际Windows

| 验证 | 实际结果 |
| --- | --- |
| red1287 | 61pass/45fail，包含失败父项；106个初始测试ID，不冒称后来增加的13个控制也跑过此红测。stdout `8bffd836f6407e2efe42504867cfbf2bbbacbe18dcc9acb0500df7cd56f3c350` |
| 119新增 | 三套件/双方向全部7个非空认证子序列，重复/倒序、空列表、缺口、正文和预算原子性、未知角色/opaque消息、公开字段正反修改、独立PHA、新连接、真实AEAD及跨record/同record倒序；六fuzz种子 |
| 相关1290 | 2945pass/0skip；未提交工作区时运行，源码哈希与最终提交相同。stdout `272e6d720d5eea407b163161c0df56075ab39c04075ed46c6d1d20f759fd7e3b` |
| 最终完整1292 | 干净fbda：290297pass/25历史optional skip/0fail，包含整个bin-parser、pcapx、CLI及CI脚本。父全部本地ID/动作保持，另119个新增pass；stdout `47784f9c8b597d66bfe93cf19c491e8184c780fb970c92b23e4e0136cc2916f8` |
| 插桩1296 | coverage构建、六种子70717执行；1CPU/总768MiB/最多2并发进程，第三进程拒绝正控通过。stderr `4a5588731e9e0e7643652d01c8fcffdd511171be2d1228a8d94e0f01349b7b6f` |
| 实际Windows1305 | 三错误页共66状态，含9等待/57交互；主线程查看三页。默认详情半窗完整显示具体错误、TLSv1.3/record AEAD，错误无借用范围，完整286B拒绝record读回；mouse/Tab、80×30/180×48、CP936恢复、拥有进程关闭通过。closure `cb42692208d6e38a8c0858ece67ca9b077276dc768c6c249278fdebe4d594f77` |
| 完整公共1298 | 两大捕获379455物理帧，最终SDK/flow/CLI六次完整重跑，全部与已验收UDP来源输出逐字节相同。summary `168000c2daad708101223b5f096328d37719adaacf473ea4eeec36ace331b237`；输出保持不等于全部Wireshark语义正确 |
| QUIC1302/1303 | 最终fbda重新四域组合，原contract/checker不变全部0差异；app-only仍不借用身份。runtime `bb6e2b5afd000cf64f94e5fec32b549bd52339715404a448c27656d6fb7b443e`；结果summary `2b696ad565620f9b432f34905dd8fee9c711395502c9f78eb52219cec282d3cb` |
| 综合1313 | `daaf3e13b26c50315cbc2acc336df69989d04fae41aa4079991d8541fde859c7`；空闲21028552704B，高于8GiB底线 |

保留工具失败：1288新测试错误地期待直接`consumeTLS`调用填充外层事件诊断字段，25失败；
1290改为验证真实返回的typed malformed错误、具体原因、认证序号和成功正文不发表，实际外层
PDU/列表由1294/1308/Windows另验，未改变任何既有测试。1300嵌套工具字符串引号错误，
没有启动UI；1305分开展开后完整复验。1310可选平铺导出工具的嵌套proof目录被包装器父路径
守卫拒绝，未编译/运行SDK；1311直接检查SDK已有的typed values，不把工具失败算通过。

Go1.22.12；最终events/flow/yak二进制SHA256依次为
`9046d0e6172b033bcaacb4895b1a82e8d9152b4cc813a201e272983b0f8f69de`、
`c66352419fddd8dec50c83f16d286bd6d653deacb485bb087f6da1fcbd68ed6f`、
`b7f449f2e81c7f22749a4021db97ce16347d204ac6147605d7420d4de17c9647`。
完整SDK/Go测试3CPU/2GiB，小源/参考2CPU/512MiB/单进程；Windows1CPU、进程512MiB、
总1GiB、最多8进程、Go heap256MiB。这是原生控制台单元验收，非全部Terminal/DPI/字体/IME。
本批102次小源原生执行、22次新完整参考分开计数；实际截图状态不当独立协议测试数。
所有捕获、keylog/密钥、完整导出/PDML和截图在仓库外，两份fixture ZIP保持原字节。

## CI及剩余范围

父`dccafe51d65ccc4bba7256c0377665e7ea150668`已独立47约定检查名、91执行检查、2状态全绿：
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41459106990)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41459114621)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41459121826)。315277原始ID/动作、46新增、
131084 required全pass/0skip及实际checkout均闭合。官方artifact13148034822/16839805B，SHA256
`c6d9d36072d944c66e113ccce15467ffef86cf80a7c9776ee3c5df79f7382d50`，闭合1286
`eee457d5d7cf6c70db2cd3fe180c685ae718307238094a1dc50d4b1d4f820b1a`。
本批含说明的新最终head另验315396原始ID、315371pass/25历史optional skip、119新增和
131084 required全pass/0skip及全部CI，不以父成功替代新提交。

当前75父项65有限完成、7进行中/3SMB搁置、10未关闭。非SMB17包13有限完成/4开放，
N09/N10/N13/N14仍粗估16–32 AI实施小时，构建/CI等待及新发现另计。P04只新增客户端
认证观察子序列及已观察空列表合同；所有必需消息、PSK/证书类型/压缩等协商依赖、
完整阶段及TLS/QUIC高级恢复、TCP代次和完整IP恢复/字段仍开放，核心协议未声明完备。
