# TLS 1.3 已观察服务端握手顺序：2026-11-02

M2-73关闭已知服务端初始握手消息重复/倒序的有限合同。核心实现为
`cd2213502bbd7ba793dafeb3cf0bbb62686d3dce`；实际Windows复验发现诊断在默认详情栏截断，
`d85312d5f5a53b5011978348c70efccf560f7383`仅缩短该诊断，是最终干净构建/实际运行代码。
仍在唯一M2 PR #5371，main d705已包含，SMB继续搁置；不把本项当完整握手状态机验收。

## 故障与有限规则

[RFC8446 §4.3/§4.4](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3)规定服务端EE、
可选CertificateRequest、Certificate、CertificateVerify、Finished的顺序。旧代码虽验证
个别消息的角色/epoch和正文，却能重复接纳EE，或在已接纳CertificateVerify后接纳更早的
EE/Certificate。记录AEAD认证不能证明消息顺序符合协议。

仅在TLS1.3、已知服务端方向、认证后、初始handshake epoch比较已接纳消息的顺序。
允许任意缺失前缀/中间消息；不因捕获缺包伪造“缺少前序消息”的错误。失败正文、集合预算
拒绝不推进观察状态；握手gap不抹去已经观察到的顺序。状态属于单条TLS连接，复制试算后
提交，不跨方向、连接、接口或VLAN继承。原记录认证/序号消费与语义拒绝独立保留。
应用epoch的PHA请求继续使用其原关联/生命周期规则，不重启初始flight。

未解释的CompressedCertificate不充当已解析Certificate来推进此状态。客户端初始顺序、
完整消息必需性、所有扩展及协商依赖、transcript/签名/Finished MAC/PKI均未由本合同验收。
没有把一个完整观察子序列当作完整成功握手。

## 独立真实来源与逐字段参考

原Windows mTLS17帧来源241保持。1209独立AEAD和TCP checksum重算两个同长度故障：
用原21B EE替换初始CR record形成重复EE；用15B紧凑CR替换首EE、原EE替换CR形成CR→EE。
紧凑CR只提议原CH已有的Ed25519；其余空间为TLS inner padding。保留全部原帧、时间戳、
TCP序列/确认和record长度。它们不是新peer，也不是签名/transcript仍有效的交互。
source manifest SHA256 `3d663a677eec87d7466b8fa8fd8459b949fadcd13669e1d2d9fbac61aa9ebbcb`。

1216重新完整TShark4.6.9双遍、空配置、明确原服务端端口Decode As和原keylog：两故障
消息类型/长度、EE扩展/ALPN和CR算法共18字面组检查通过；其他九个消息的字段保持。
TShark能够显示这些字段不等于它认可消息顺序。最终1231共18次旧/新SDK、CLI、flow运行：
正常三导出全字节相同；两个故障旧各HTTP2、新各HTTP0，完整flow导出保持，失败record仍
AEAD=true/TLSv1.3，错误不发布成功握手正文。summary SHA256
`ffcd2906c81a5bf6445b366c7caf632e89514820ce28f6e71a19c24883ab0fcc`。

1223派生相同tuple先好后坏/先坏后好、双接口交错、双VLAN交错六个34帧控制，显式纳秒精度。
VLAN仅替换link前缀，原IP/TCP字节逐帧SHA保持；这仍是来源克隆，不是六条独立新实抓。
最终1232共18次执行，均22事件、两个独立TCP owner，132个引用全部在本来源/接口/VLAN；
正常HTTP2/故障0，六组flow与旧版相同。1241重新六次完整组合参考：两tuple复用的消息类型
完全相同；两接口/两VLAN例第二个来源不完整解出，四个strict失败保留。隔离参考1216和
独立原始源仍通过，不把参考关联政策直接称为Wireshark错误或豁免完整差分。

## 新抓 TLS 1.2/1.3

1218本机loopback重新生成测试证书、强制客户端证书并实际交换HTTP：TLS1.2为19帧/5255B，
SHA256 `c113f209efdd5953a68c78c090bd322e5f39cf7df10c4136cda54bf13fcaed7d`；
TLS1.3为17帧/6598B，SHA256
`01439c5da925f21fbd2aa3d5983de6375d83cb5f4759b6396df2b653a0811752`。
独立1221从线上字节重组，核对CH/SH、52/53B CR及28个字面字段，TLS1.3按实际SHA384套件
逐条AEAD并验证HTTP与peer文件一致。TLS1.2不冒称已由该独立脚本验证加密HTTP的AEAD。

1234实际执行两捕获的旧/新三种导出共12次及两个新完整TShark双遍。1240重新核验这些不可变
执行产物，零新增执行：30条TLS record的原字节/来源、4条HTTP与peer原字节、16组native
字面alias均正确，新旧三导出分别全字节相同。新参考GET/200及其余CR字面组一致。
唯一raw strict差异是TShark省略零长度context条目；SDK明确发表空值。仅当独立编码context
长度为0、参考长度00且原字节确实为空时，记录为明确格式差异，不改写原observed=[]/expected=[""]。
1234原strict失败保留，1240不是“所有字段完全同形”的证书；summary SHA256
`d276492bdf42b298e32475413f1fc6119200ff3bf976b4987c4ff54808835181`。

## 回归、fuzz、实际 Windows 和完整回放

| 验收 | 实际结果 |
| --- | --- |
| red1208 | 187pass/21fail；包括失败父项。20对中18对旧实现实际误接纳，两对重复CR已有context拒绝，只是诊断不同。stdout `c96aecb33cb4112948ded04826da66ad66bfddb692a677c22dd856603b9629d8` |
| 完整1213 | 干净cd，290132pass/25历史optional skip/0fail。范围包含整个common/pcapx/...，较原pcaputil范围另有100pass/1skip；这些不算新增。原本地ID/动作保持，实际新增246全pass。stdout `f0a6de7d6351ada46398f9388a272b5e73fe74bba73793852c8178deb0bc8d46` |
| 最终1226 | 干净d853，2617个TLS/QUIC及CLI相关pass/0skip；仅诊断文字在完整运行后变化，完整与最终相关输入/终态逐项核对。不冒称完整1213来自d853。stdout `1e1e4e66b8be5a27e606fd7c141c15b5bcf8cb27dcc0ab7cf9099e25eb1c3490` |
| 246新增 | 双方向20重复/倒序对子；三套件/双方向全部31个非空观察子序列；缺前缀/gap、正文及预算拒绝原子性、未知角色/客户端/opaque消息、PHA独立、新流、真实AEAD与跨record重复；六fuzz种子 |
| 插桩1220 | cd明确coverage构建，六种子33766执行，1CPU/总768MiB/最多2进程，第三进程拒绝正控通过；d853只改诊断文本。stderr `34b4a5852b5bf851210dc30f77b105109f3ed0eb359c96b08ebc97dc3d7f7f89` |
| 最终Windows1233 | 两完整捕获实际跑44截图状态，含6等待/38交互；主线程查看两个错误页。完整诊断在默认详情半窗可见、错误无借用范围、75B拒绝record全字节读回、180×48/80×30、鼠标/Tab、CP936恢复及拥有进程关闭。closure `7cd92490a04669838012d4e92012eed70bac387b854bd11d76389fa078165e9d` |
| 完整公共1219/1235 | 两来源379455物理帧，cd和d853分别重新SDK/flow/CLI各六次；全部逐字节保持已资格化父输出。最终summary `5b15b2c2f8af2fe658acb0cf5fe79636ac7c278a5bb52c7fcc3a8cc26b115a3e`；输出保持不等于全部协议/会话与Wireshark一致 |
| 综合1242 | `5d91eb76250b29d461fab405b35c836a887e619b00a80112d6897537f4cabdf0`；空闲25343045632B，高于8GiB底线 |

Go1.22.12；最终events/flow/yak二进制SHA256依次为
`b2058b1a231e194dbf24cd78ab21cb7ccd8a4334d531afb3a12aac06be7f1a2b`、
`e31363c2650161e3652463ec96e12eec428bbd4cf513e9390bae30376d265afb`、
`73e11760e2051756ff2d7acee5840fb822620407ad0c940c2797009d7a3bd14e`。
小回放/参考2CPU/512MiB/单进程；完整SDK3CPU/2GiB；Windows1CPU、进程512MiB、总1GiB、
最多8进程。源捕获、密钥/keylog、完整导出/PDML和截图均在仓库外，两个fixture ZIP不变。

原失败保留：1212构建参数误用短SHA被守卫拒绝，1214固定完整SHA；1222默认详情栏诊断
截断，d853缩短后1233完整复验；1236检查器误用receipt大写hash键，1240按实际schema读小写键，
不改变任何产物或断言。没有取消失败或靠删除断言过关。

## CI 与未关闭条目

父`add08e5072baefceec69533821b25704de58ca1f`已独立47约定检查名、91执行检查、2状态全成功。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41436592291)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41436599060)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41436605748)实际checkout、314985原始ID/动作、
59新增及131084 required均闭合，零required skip；官方artifact13138390248/17052413B，SHA256
`20a9b6e2e7797ef6ed438d14de4851d268ea0460e2d7555806321c2826c541aa`，闭合1229
`6c6d5c2c7571ec58cb0bc3f366ee4b78fd09229a2331a6e6316bea89116ac419`。
本批最终说明head另验315231原始ID、315206pass/25历史optional skip、246新增及全部CI。

1237/1238重新跑四组QUIC域控制：full/full接口与VLAN各0strict差异；两组app-only仍各2664，
1239各验证2007个来源准确UDP预览，无借用QUIC/H3；N09继续开放。N13.P04仍保留客户端及
完整阶段/协商依赖、CompressedCertificate和高级恢复；N10 TCP政策/代次、N14完整IP恢复继续
开放。当前73父项63有限完成、7进行中/3SMB搁置、10未关闭；非SMB17包13有限完成/4开放，
粗估16–32 AI实施小时，不含构建/CI等待和新问题。SMB S01–S06继续由承接者独立实施。
