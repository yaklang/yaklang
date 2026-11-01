# M2：真实 PHA Finished、拥有明文和 main 同步

本记录关闭 M2-46 的有限合同：Finished 长度和记录边界、初次与应用密钥阶段的转换、拥有字节和递进展示。
不代表 TLS/QUIC 完整握手状态机、证书信任、签名或 transcript MAC 已在产品中验证。
M2-43/N13 和其他字段、会话、fuzz 差异仍开放；SMB 缺口继续搁置。

## 代码与基线

- 原生修复：`9c394cdbf7ce0928852345afa6de5dfe149645ce`。
- 展示修复及第一份干净构建：`7c11f22f40a51df3cb130112c077bb331c069552`。
- main 更新至 `238faf87f36e9eff5ec1ea0ff06e8c06d473214c`，普通合并为 `49bb60b02655bb73c75be39ffae9c26a8836c582`。
- 合并测试兼容修正、当前干净构建：`d1a6e62123ea8c6b6d3b43838c12beb86943ddaa`。
- 保持 M1/M2 两阶段：M1 #5353 已合并；M2 继续在 `codex/shark-m2-protocol-correctness` / [#5371](https://github.com/yaklang/yaklang/pull/5371)。没有强推或历史重写。

main 新增企业流量、DCE/RPC、NTLM、数据库等代码和测试，不能把旧 7c 的验证重标为合并后验收。
冲突涉及压缩规则和既有 SMB 状态文件。SMB 仅合并两边既有改动，不新增其协议工作；保留 M2 原有链身份和预算。
规则根据合并后的 YAML 重新生成，两次产物相同：
`56b5dc10ec0c4ae27aeff16702c860286ac42e28e424df3d95157e703f47e71a`。
生成期间其他输入及索引未变，收据336为 `bfd6156b161769adb0c7eff5f85bf6f7d83bad60bbfee1ba53daa0b00ddb8ec2`。

## 实际缺陷与修复

真实 TLS1.3 后握手客户端认证（PHA）成功之后，旧代码把客户端 Finished 再次当作初次 Finished，清空应用密钥并将记录序号归零。
原始29帧中，客户端 PHA Finished 使用应用序号4；下一条 GET 使用序号5。旧构建只发表4条完整HTTP，随后记录认证失败、响应失去请求上下文。

现在仅初次 Finished 切换到应用密钥；应用阶段保留当前密钥、IV、secret和序号，不改变另一方向。
完整消息先验证受支持协商版本/套件对应的 verify_data 长度：TLS1.3 SHA256为32、SHA384为48，已支持TLS1.2 AEAD套件为12字节。
TLS1.3 Finished还必须结束于记录边界；拖带完整或部分后续握手字节时，在状态提交前拒绝。
清文或错误角色的TLS1.3 Finished不能推进状态；缺版本保留带限定的未解释 envelope，不用记录legacy版本猜正文。

`DecodeTLSFinished`仅校验完整framing和已支持套件的长度。正文与编码均拥有字节。
新增 `tls.handshake.verify_data` 是精确十六进制literal，来源为derived，不附外层密文或物理帧范围。
详情按照头部、正文、上下文展示；应用阶段Finished在拥挤摘要中保留可见性。
页面明确显示 `transcript MAC not evaluated`；记录AEAD通过不等于握手签名、transcript或证书信任通过。

规范依据：[RFC9846 §4.5.3、§4.7.2和§5.1](https://www.ietf.org/ietf-ftp/rfc/rfc9846.html)、
[历史TLS1.2 Finished](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.9)、
[AES-GCM记录保护](https://www.rfc-editor.org/rfc/rfc5288.html)、[ECC套件PRF](https://www.rfc-editor.org/rfc/rfc5289.html)。

## 独立真实来源

所有完整捕获、证书私钥、keylog、原始导出、工具和截图均留在仓库外：
`C:/tmp/yak-shark-ESP-SSH-UI-20261028`。不打入项目测试ZIP。

| 原始来源 | 物理帧 | 真实条件 | 捕获SHA256 |
|---|---:|---|---|
| source305 | 29 | OpenSSL3.5.8/Python实际TLS1.3 PHA，3轮HTTP | `d59fa1fe99f81311b88e3b004111c99c627e7526edeb6bacf28870b9a62475cb` |
| source316 | 52 | 相同四元组的2次新SYN/不同ISN和ClientHello random，各3轮PHA；无PSK恢复 | `f5b37341f6a01fe251a588308b30777221631d7385d23617ba5c23ea3289a9e3` |
| 原source241 TLS1.2 | 19 | 实际初次双向认证和HTTP，复用原捕获 | `b4d678871c7074ce1f465ac82b50a44a1c48442a1df44c8e8e153d2b1e3e1c34` |
| 原source241 TLS1.3 | 17 | 实际初次双向认证和HTTP，复用原捕获 | `deadc7ec4bb9672ffeb96046565ef02b69009cd9c7d18b2258a2c113696c6999` |

共117个原始物理帧、5条真实TLS连接。其中新抓取的两份文件为81帧、3条TLS1.3连接；不把派生切片或变异当新抓包。
独立oracle从原始PCAP/TCP字节、授权keylog和peer文件核验AEAD、13个Finished MAC、8个TLS1.3 CertificateVerify签名及22条HTTP。
实际PHA套件为0x1302；TLS1.2为0xc02f。其他支持套件的正文/epoch门另有明确的构造AEAD测试，不能称为这些真实来源覆盖了所有套件。

| 独立证据 | 收据SHA256 |
|---|---|
| 原始PHA wire/AEAD/3 MAC/2签名/6 HTTP，306 | `40e4ae16219f4b9e9f5cb4e1be0ea07a9b0faf7ea159f1ff74680b85b401c097` |
| 同四元组两代、完整源切片逐字节与6 MAC/4签名/12 HTTP，317 | `fae28151e95edf7b9fdb025ac2fcead97cc36689c4e3bff505699a78761c5102` |
| TLS1.2 PRF/记录AEAD/2 MAC/双方HTTP，320 | `7e002008f6d6b996f6f53c9c9e33ede551d42371ed920f10877b6bd8a2df1015` |
| TLS1.3初次双向认证2 MAC/2签名/双方HTTP，338 | `3ccef0955c6ce6c28649eb184064c88be6d2ec427801369745d579c634edbfce` |

oracle的密码学结果属于独立来源核验，产品仍明确未验证transcript MAC、CertificateVerify及证书信任。

## 逐项控制、参考和实际回放

控制319在运行修复后二进制之前冻结：原包、精确重传、认证flight乱序、整flight丢失、捕获截短7字节及密文翻转。
frame20包含Certificate、CertificateVerify和PHA Finished，丢失它不应描述成只丢一个Finished。
裁剪保留原on-wire长度；翻转保留原TCP checksum并明确是破坏控制。另保留真实同四元组两代输入。

合并后d1的新SDK/CLI完成9组、18次完整执行：

| 输入 | 原生完整HTTP | Finished消息 | 结果 |
|---|---:|---:|---|
| 原PHA | 6 | 3 | 全部peer字节一致，应用epoch保留 |
| 精确重传 | 6 | 3 | 不重复发表或换钥 |
| flight乱序补齐 | 6 | 3 | TCP重组后完整恢复 |
| 永久丢失flight | 4 | 2 | 不捏造PHA Finished，incomplete/context-required保留 |
| 截短flight | 4 | 2 | 不宣称后续应用连续完整 |
| 密文损坏flight | 4 | 2 | 认证失败诊断保留，不发表未验证PHA正文 |
| 真实同四元组两代 | 12 | 6 | 两个独立TCP代次，每代6条HTTP |
| 初次TLS1.2双向认证 | 2 | 2 | CCS之后的记录序号未重置 |
| 初次TLS1.3双向认证 | 2 | 2 | 两方向初次epoch正确 |

每个新verify_data及其完整编码与独立来源逐字节比较；SDK、CLI field_values和扁平fields分别核验。
d1收据341：`84e08948e018e85b19b6fde9cfab94adc4f56eb0825ba833911228ccad999c55`。

TShark二进制SHA256为 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
新318参考对7组完整输入使用独立空配置、two-pass及已知本机端口DecodeAs；所有已发表Finished的字节和[0,1)/[1,4)/[4,52)来源范围与oracle相同。
参考在完整PHA中显示2个请求、3个响应，在真实两代中为4个请求、6个响应；缺少的客户端GET在原wire、独立AEAD序号和双方peer文件中均可核验。
原生修复后分别为6和12条HTTP。保留该参考差异，不为追求输出相同而丢掉正确数据，也不声称已经证明Wireshark内部原因。
318收据：`d337ecc38d6db4a93d0c3dcdc77a2b4336d87bc9f2f73441d6a7fdd6b9401a0b`。

## Windows与字节来源

d1构建的events/flows/yak三个二进制均有干净输入收据。Go1.22.12 windows/amd64，GOMAXPROCS3、GOMEMLIMIT2GiB、-p1为构建目标，未冒称整个构建树有OS硬内存上限。
实际yak SHA256：`ba41da520c63067d4c13037f375cd3898d64f0a6a627ce98d94cfa367fd28340`。

主线程已查看实际控制台字符/属性生成的两页：TLS1.3完整52B，[4,52)正文；TLS1.2完整16B，[4,16)正文。
鼠标/Tab切换、80×30/180×48缩放及CP936恢复全部通过。共36状态；345收据：
`39cde0efd1e91ecab93ac67cc291d12ccde64ebfff9af0efb8c76752035380db`。
这不是Terminal字体/DPI/IME全覆盖。图片保留在外部 `Windows-main344-01`，不提交PCAP、keylog或完整截图导出。

caption另核验：当前握手包的protocol_stack止于TLS；`connection:`后的HTTP来自同流实际解密消息，未将ALPN当作本包正文。
旧工具的可选mapped-PDU检查要求PDU范围标题，不能用于独立握手明文源；首轮失败保留。采用原有选择模式后，独立checker仍检查全部编码和精确高亮，没有更改生产字节源或削弱PDU断言。

## 测试、fuzz与大捕获

- 原缺陷红测27失败；记录末尾边界红测5失败；展示红测5失败、9通过，全部保留。
- 两包完整本地315：238983 pass、12历史optional skip、0fail，输入未变。
- 完整CLI314：1107 pass、2历史optional skip、0fail。新增187个Finished终态ID，全部要求pass。
- 有效插桩327：5/5种子、36654次执行、0fail；Windows Job硬限1CPU、总768MiB、最多2同时进程，峰值212709376B，关闭后无遗留进程。不是完整TLS/QUIC状态空间证明。
- 327构建来源为7c，收据stderr SHA `38dc173d6700b5b7e82f25dbe5e8a4919246a7aa14f50bf9f051aa036c030f46`；没有内部fuzzworker参数覆盖。

在main合并前的7c上，Weber51328帧和Sharon328127帧完成6次完整SDK/flow/CLI回放；新329参考同样跑完整文件、空配置、无DecodeAs或display filter。
330检查保留每个原ID、raw、来源和既有非SMB字段：0非预期SDK变化、0非预期CLI非Info变化；新增两个Finished literal在Weber49524/49526与新参考一致，只有两处预期摘要资格变化。
两个flow JSON逐字节相同（824/840个观察流、14172/7700次回调）。不能与参考的641/35420个TCP stream直接当相同口径。
Sharon参考251431的verify_data为`<MISSING>`，明确记为非literal参考缺口，不转成十六进制值或算通过。
330闭合：`4707a2c2f4ac942a075af0392cfbad8676d3c02f3c54522b015e6d3d80054b80`。

7c证据不替代新main代码的完整大捕获结果。后续d1构建在8a文档等价检查点完成346的6次完整回放；
347逐对象比较保留全部ID/raw/来源，非SMB事件/字段无变化，两个flow JSON逐字节相同。
main原有SMB变化为176/2事件、165/2物理展示，仅保留差异，不展开SMB语义验收。
347收据：`468a218dbb4ab109bf774e051c3b07c90ce1af3de5be44b60b24f7a34dddcdec`。
外部完整大捕获SHA仍为Weber
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`和Sharon
`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。

## main合并的测试及最终CI门

完整本地337：313151 pass、25 skip、1失败，所有输入未变。唯一失败是main新增请求摘要错误期望status=0；
d1只修正两条请求摘要，并增加无Status/有Process ID断言，所有响应状态、请求路径、文件关联、读写检查保留。
339的原失败和头部union检查均通过。不能把337改写成全量0失败，也未把SMB缺口因此算完成。

CI342在读新CI产物前冻结：313158个精确ID/终态，313133 pass、25历史optional skip；131084 required全部须pass、0 skip。
187个新Finished测试和main新增58个可移植测试均要求pass；保留此前全部CI ID/终态。
本机3 worker与CI固定2 worker对同10161个SV帧均恰好一次执行，原测试源码未改；使用CI原分配的ID，不改帧或删除断言。
19个已有Windows专属本地ID不伪装为Linux CI；旧Unix测试保留于CI门。
冻结文件SHA：`3d9d1ac465d959cc575903fe8a1298bebad3c778c474dfecf6727f256e4812f6`。

最终提交CI仍以PR中的精确head_sha、实际checkout、完整artifact、所有必需checks/statuses为准；本文件不把旧700全绿、skipped或本地通过当成新提交全绿。
原700的47个指定检查/安全检查、2状态和完整traffic已单独闭合，收据 `ca929999774423b05345ea0c43401bef1c72687c6c3b4aef1c500d40dd01df46`。

后续8a的CI352已实际闭合：精确 `8a02294759c1ead373c60c60fb870c7649d92fa7` 的47个成功checks、2个成功statuses、4个选定成功workflow；自动draft skipped单列，不计成功。
[完整流量专项](https://github.com/yaklang/yaklang/actions/runs/41277905022)实际checkout与head均8a，产物313158个ID/终态和预冻结342完全一致，313133pass/25历史optional skip，131084 required全pass/0skip。
完整artifact SHA256 `14db27b82594a8a410c51951f76359cc6d6e762be50e88adff1bde2064773880` 同时与官方digest和上传日志一致。
[Essential](https://github.com/yaklang/yaklang/actions/runs/41277858361)也已成功。
闭合352：`cc1256031ca8a015a72e8894c0523ba7c7555d199e20a1a519399e53b92198ea`。该验收只属于8a，不替代后续CV或新功能的CI。

## 后续缺口

- M2-43/N13：PHA请求offer/上下文唯一性/多轮关联、完整阶段合法性、缺握手资格、TLS/QUIC高级恢复；不能由本次Finished长度/epoch证明关闭。
- 当时新增M2-47/N15：CV曾仅发表Type/Length/Authenticated；后续e5/488已关闭完整版本化正文/方案/长度/拥有来源及实际页面的有限合同，见[CV证据与保留缺口](shark-m2-tls-certificate-verify-20261101.md)。没有由此验证产品签名或关闭N13。
- N07–N11原HTTP方法不可观察性、新SYN政策、app-only QUIC及6 TCP/24 UDP fuzz分区继续开放。
- M2-45/N14：IP碎片来源分类和有界重组；不能把全部Fragment列差异视为可恢复记录。
- SMB M2-04/05/36按用户要求搁置；[独立任务书](shark-m2-smb-taskbook-20261101.md)的历史分母须在S01重验。
