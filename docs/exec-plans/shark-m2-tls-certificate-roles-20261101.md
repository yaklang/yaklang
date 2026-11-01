# M2：TLS Certificate 的已知服务端角色约束

生产代码和干净SDK/flow/CLI构建：`17fe3ae44f38ec51826bd813f164480e265b6276`。
独立CI测试样本修正：`b3221501717f8887192e779d4f51ce1c42035940`，只改RAG的一个测试文件，
不改变任何生产代码；不冒称从后续说明提交重建生产二进制。
继续同一M2分支/[PR #5371](https://github.com/yaklang/yaklang/pull/5371)，普通提交，不强推。
SMB继续跳过，没有新增SMB语义分析。

## 发现与修复

前一Certificate正文合同检查内部向量，但没有限制已知服务端的请求上下文及空证书列表。
这两个输入可通过原生接纳并继续发布后续应用。依据
[RFC8446 §4.4.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4.2)，
服务端Certificate的request context必须为空，certificate_list必须非空。
客户端可合法发送空证书列表；应用密钥时期的客户端响应也可能带PHA context。

现在只在观察到ClientHello、方向角色已确定后，对服务端增加上述两个检查。
不从端口或Wireshark流编号推断角色。错误保留MalformedMessage及具体原因，
不发布被拒绝的Certificate正文/别名，不继续向应用解析器发布明文。
记录AEAD事实、已观察的TLSv1.3连接和准确原始记录仍保留。

公共`DecodeTLSCertificate`是无角色的语法API，仍可读取两种结构完整的向量供调用者检查。
角色未知的原生连接仍因missing-context保留加密封套，不凭已有测试密钥猜测方向。
原有记录序号消费、密钥、IV与epoch不被这两个正文条件重置。

这是M2-56的两个限定子条件，不新增父项分母。初始客户端context、服务端完整阶段、
PHA offer/context唯一性及请求响应关联、证书类型协商和信任仍未验收；N13整体开放。

## 源、参考及实际回放

全部证据继续留在仓库外`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
沿用本轮新实抓的550实际OpenSSL peer、41个原始物理包、授权NSS秘密及552独立密码验证。
没有提交PCAP、keylog、密钥、完整导出或页面；两个捕获ZIP/manifest不改变。

`TLS13-certificate-role-controls581-01/` 在产品运行之前生成两个同长AEAD衍生控制：

- 非空服务端context：用一个上下文字节替换未知扩展的一个载荷字节，同步修正完整向量长度，原证书DER保持。
- 空服务端列表：用8字节完整空列表握手和TLSInnerPlaintext padding保持原记录长度。

两者内部向量均由独立函数确认完整；保留全部原始PCAP记录、时间戳、序号和方向，
只重算目标记录AEAD及受影响TCP校验和。它们是构造的认证控制，不是新的端点网络流量，
不声明修改后CertificateVerify签名、Finished或整个transcript仍有效。
manifest SHA256 `b46857148edf92b7227619be38f04b05455e2ab689047e620f5fbf17f35c21dd`。

`certificate-role-reference587-01/` 对原流量和两个控制完整两遍回放，使用原TShark SHA256
`662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`、
隔离配置、实际端口50646 Decode As及同一授权keylog。
三次合计23个字面字段检查通过。参考均展示Certificate，未对这两项角色条件报错；
如实保留参考行为，不据其仍显示字段宣布控制合法。

`certificate-roles586-01/` 为旧0f1/新17fe × 原流量/两个控制 × SDK/CLI/flow，18次实际执行：

| 输入 | 旧事件/HTTP | 新事件/HTTP | 其他约束 |
|---|---:|---:|---|
| 实际原流量 | 35 / 12 | 35 / 12 | 三种导出全部逐字节相同 |
| 服务端非空context控制 | 35 / 12 | 6 / 0 | 具体错误在CLI PDU及Info，全部41物理行保留 |
| 服务端空列表控制 | 35 / 12 | 6 / 0 | 具体错误在CLI PDU及Info，全部41物理行保留 |

三组flow分别逐字节相同；两个拒绝事件无Certificate别名或成功消息。
这不把构造的坏transcript当有效对端交换，也不声称产品已完成签名或信任验证。

`runtime-certificate-role-public590-01/` 又执行两个完整公开捕获的六次SDK/flow/CLI回放，
从17fe干净源码构建、全程source/input guard通过。
回放前冻结六个0f1完整输出SHA；六份新导出全部逐字节相同。
覆盖原有306380个原生事件、379455物理行及580个publication，不重编号、不放宽差异，
不把保持既有输出解释成所有协议与Wireshark全部正确性一致，也不重新验收SMB。

## 本地与Windows

`TLS-cert-role583-red02/` 保留修复前完整43 ID：30 pass /13 fail，
包括三套件、两个方向的12个真实错误接纳及父测试失败。
最初582还错误要求角色未知时解密，QA panic保留；修正为原有missing-context合同后才得到583。
`TLS-cert-role585-green01/` 相关902 ID全pass/0skip。
`TLS-cert-role592-full01/` 完整受影响包239551 pass /12历史optional skip /0 fail，
日志SHA256 `88293566a464fbeb4ae6b5c4990065a02af16d0f4738aa1280d68b5c67d59479`。
相比前次完整运行恰好43新增，所有父ID和终态原样保留。

本批增加的是记录级AEAD角色矩阵，未声称新增完整状态机或TLS消息分片fuzz目标。
前一正文合同的58636次插桩执行保留为其自身证据，不冒称从角色构建重新执行。

`Windows-certificate-roles594-01/` 为新CLI实际运行两个错误页，共34状态。
严格检查选中帧6的TLSv1.3身份、真实详情左半窗中完整的`Error: MalformedMessage: ...`，
不是只查列表Info或历史VT文本。被拒绝的Certificate body不显示；仍可查看AEAD认证的记录事实和原记录字节。
packet/stream worker、mouse/Tab、80×30↔180×48和CP936恢复通过，拥有进程均关闭。
root已分别看过两个deep-protocol-fields图。真实console cells/attributes固定字体渲染不代表全部Terminal字体/DPI/IME。

591仅验证列表文字，593导航到含错误的长摘要仍不足以证明详情字段可读；两次结果均保存，
594从实际worker定位独立Error字段并要求错误完整出现在详情半窗，未删减断言。
旧SDK harness不导出Error，QA明确绑定同次CLI的相同PDU ID、状态和source，再转为worker DTO；
错误期待文字由固定角色条件决定，不从候选输出推导正确答案。

## CI失败及修正

前一说明提交066的专项traffic已独立闭合：313643 pass /25历史optional skip，
59新增及131084 required均全pass/0skip，全部313668 ID各一次run/terminal。
[完整traffic](https://github.com/yaklang/yaklang/actions/runs/41320480693)实际checkout、官方完整产物和上传日志均核验；
16823959 B，SHA256 `da0db628c015a18f6bfadb0036eb48b965f0e7cb50b7ca09efcd70bbf6b8ad30`。
这不是066整体CI全绿：其[Essential](https://github.com/yaklang/yaklang/actions/runs/41320487059)
的RAG查询必需测试失败，Gate因此失败；旧成功和skipped不代替。

已下载失败job完整官方产物13088858234，3415600 B，官方digest和本地SHA256均为
`79c633c8f248519faf6fd738c267b7c0b56c194d146e58a2734e386cd6cc809e`。
原始包日志明确是在13.16秒后无法构造相似度[0.85,0.99)、与其他文档低于0.35的随机文本，
还未到查询结果断言。b322仅用1024维正交词表及确定性文本代替随机拒绝采样，
运行时仍从真实mock embedding核验三个分数区间及交叉分数0。
0.4/0.6/0.8过滤阈值和3/2/1结果数保持；原Contains多余参数只是消息，现ElementsMatch核验全部预期身份。

`RAG-fixture603-repeat50/` 同四个唯一ID各重复50次，200个pass终态/0fail/0skip，
不叠加成200个新测试。`RAG-fixture604-mustpass/` 干净b322的RAG必需集26 pass /0skip /0fail。
两次均使用明确纳入RAG目录的源码快照；较早599/600及未纳入该目录的traffic专用快照结果保留。
最初598的本地生成脚本漏一个引号导致编译失败，已保留并修正，未改变任何期待。
605另核验生产构建输入只发生这一个非生产RAG测试变化，完整traffic包输入只新增README的五行角色说明；
其余已测试生产输入、五份证据摘要和两个捕获ZIP的哈希全部保持。
源码资格摘要SHA256 `33691ded0b7c289df82712f4931d28f7c02ece089b0b26e16754c320d7fada46`。

新最终提交的traffic期待已在读取未来产物前冻结：313711原始ID，313686 pass /25历史optional skip，
父313668逐项保持，43新增必须全pass，131084 required全pass/0skip。
manifest SHA256 `33c6315dffcf7f8767cb862fa70047bdb61a3af65d3213871394e3c9e4e74800`。
最终head须独立47必需检查名、两个status、全部实际执行检查、实际checkout及完整官方产物闭合；
066的专项成功、7a9整体成功均不包含新角色修复或RAG测试修正。

| 证据 | SHA256 |
|---|---|
| 三次新参考587 | `138be6e9fc9b02f8cbeb3642d41b67f700965e9e0b1b4e60a02680b35cb627bc` |
| 18次实际回放586 | `aa71b36df9bb024528154f7ce265a6a060ac4105f9825ba09b0335ce6abb0796` |
| 六次完整大包590 | `9a759ae19799d1a52b3c94b7b2ceddc009e8ba9f47c7152c44d5e98a5cc3efb8` |
| 两实际Windows错误页594 | `43f5f50ecb30fdfbe69e46050626b5490a196043b3925791b66630ff497abe14` |
| 066专项CI闭合595 | `7a93e1ca11192358874f74706fd4616eac764fe8712ced8deb072cf2b7a18ffc` |

总数保持56父项/46有限完成/10未关闭（其中3 SMB搁置），非SMB17包仍11有限完成/6开放。
后续粗估23–46 AI实施小时，不含CI/构建/回放等待及新发现；核心协议未宣告完备。
