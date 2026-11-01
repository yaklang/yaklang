# M2：TLS Certificate 向量、字段及拥有字节

本批原生/CLI代码和干净构建为 `0f1ab555287149b3b3ced55191597848f135ad66`，
Windows翻页验证工具为 `15290b57c24a02fdfc5fd1da94dd25c7f34a7291`。
两提交之间只改 `internal/trafficfixture/cmd/nativeconsole/main_windows.go`，未改生产解析代码。
继续唯一M2阶段分支和[PR #5371](https://github.com/yaklang/yaklang/pull/5371)，不强推、不创建第三个阶段。
SMB继续搁置，本批没有新的SMB语义分析。

前一检查点 `7a9a1b14959f6487e09e0144fcbdea2d63bcc94c` 已独立47个必需检查名、2个状态全绿，
91个实际执行检查均成功；自动draft skipped不计通过。
[traffic](https://github.com/yaklang/yaklang/actions/runs/41310969981)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41310973638)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41310977188)。
313584 pass / 25历史optional skip，82新增及131084 required均全pass/0skip。
完整产物16963252 B，SHA256 `e049f5d50c35c0023b0a10dc72906af8846afa5a5225c92bee9fe127a6ef12ce`，
官方digest、下载及上传日志一致；实际checkout和313609个原始ID逐项闭合。
该成功不包含本批Certificate修改；最终说明head的CI由阶段PR另验。

## 已修复的有限合同

此前TLS 1.3 CertificateEntry只校验扩展块总长度，块内短头、越界长度、重复类型仍会被接纳。
现在逐条检查完整扩展头、数据边界和每个证书条目内的类型唯一性；不同条目可使用相同类型。
先验证完整向量，再应用配置集合预算，之后才计算证书哈希、解释X.509和分配输出向量。
证书条目与扩展条目共同计入配置集合预算，既有64个证书的硬上限保留。
依据 [RFC8446 §4.4.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4.2) 的条目结构及
[§4.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.2) 的扩展块规则。
未知扩展载荷保持opaque，不据其类型推断已协商、已验OCSP/SCT或已验证证书信任。

新增公共Go API `DecodeTLSCertificate` 和拥有的 `TLSCertificateView`，接受完整、版本化握手编码。
返回请求上下文字节、证书列表长度、每个条目的证书字节及扩展向量，以及独立的原有X.509摘要。
API可按TLS1.0..1.3布局读取；本批新增的原生/页面正文投影限定TLS1.3，旧版本摘要路径保持兼容。
范围相对于完整握手编码，空值不制造范围；不是记录密文或物理帧偏移。

原生TLS1.3事件保留完整Encoded Certificate与私有已接纳字段，公开缓存或输入修改不能替代它们。
原有证书摘要保持；记录AEAD认证、正文向量解析、请求关联、证书类型、签名和信任分别表达。
本批不验证证书类型协商、角色/完整阶段规则、PHA请求context的唯一性和响应关联，N13继续开放。
后续17fe另补已知服务端空context/非空列表的两个条件，见[限定角色补充及新验证](shark-m2-tls-certificate-roles-20261101.md)。
本页0f1的构建、fuzz和阶段边界保留原事实，不将该补充当完整角色/阶段已验收。

列表增加条目数和上下文长度，已有CertificateVerify、KeyUpdate和已解码应用的优先级保留。
详情依次呈现握手头、Certificate body、Context、Certificate Entries、每条证书和Extensions，
之后是原有解释与未验证状态。JSON worker重新核对完整编码和正文值，使用独立握手明文字节源。
共享context/扩展筛选别名保持消息及条目出现顺序；证书列表、证书长度与证书字节也可筛选。
派生字段不借用密文范围，空context与没有这个字段分开。

SDK导出正文树；CLI JSON的PDU `fields` 是类型化筛选字段图，`field_values` 保留对应投影。
这两个接口各按自己的schema验收，不把CLI的扁平筛选图当SDK完整正文树。

## 新实抓流量和独立来源

所有证据位于仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
真实PCAP、密钥/keylog、完整导出、fuzz cache及页面不提交、不进入两个捕获ZIP。
Go1.22.12/windows-amd64；构建/回放采用既有3CPU/2GiB限制，fuzz和Windows另有硬限回执。

`TLS13-certificate-extension-source550-01/` 是本批新启动的实际OpenSSL peer。
通过官方custom-extension接口在ClientHello声明两个类型，再由服务端的Certificate发送，
客户端回调逐字节确认；没有脚本构造原始流量的TLS密文。
Npcap/dumpcap实际抓到41包，5746 B，SHA256
`49305ac8268ae494f22ad3686fe6f96775c3bb3306f3e2b3835a4df81543f08c`。
两个扩展数据分别56/55 B，实际双方还确认12条HTTP消息；没有KeyUpdate。

`TLS13-certificate-extension-oracle552-01/` 不读产品输出，从原TCP字节、NSS秘密和公开证书独立核验：
23个TLS记录、19个AEAD认证记录、860 B完整Certificate、728 B证书数据、两个扩展，
以及CertificateVerify签名、两次Finished MAC和全部12条实际收发消息。
这些独立验证不冒充产品已经验证签名或证书信任。

`TLS13-certificate-extension-controls554-01/` 另生成四个明确标注的衍生控制：
重复扩展类型、长度越界、留下3字节残缺扩展头，以及合法未知扩展内容变化。
均保持原记录长度、全部PCAP记录和时间戳，仅重算目标AEAD与受影响TCP校验和。
这是构造的认证控制，不是新的端点网络流量；修改后的transcript/CV/Finished不声明有效。

`TLS13-certificate-reference555-01/` 使用隔离配置、同一TShark、实际端口Decode As及授权keylog，
对原流量和四控制分别完整两遍回放。原流量与合法控制16个字面字段检查通过。
参考对越界长度提示截断，对3字节残尾提示未处理；此次对重复类型未报错，仍展示Certificate。
保留此行为差异，不把参考仍显示字段当成整个消息完整正确。

## 当前构建和运行

`bins-certificate553/` 是干净0f1构建；CLI SHA256
`ed40d571ff73d81f9df359e697c114b0d26c46e56b52d394a89fbae36348ac04`。
`TLS-cert-body547-full01/` 完整受影响包为239508 pass /12历史optional skip /0 fail。
日志SHA256 `20637dc0a7e070594dc3144e37b8c3754316bd64e8356b5173e093411911c2d2`。
该次是7a9 HEAD加冻结的本批源码；之后只改一行说明注释及独立QA翻页工具，577清单逐文件记录并核验。
干净15290的 `certificate-clean571-test01/` 又通过368个选定测试，含本批全部59新ID；
独立QA工具2个旧测试及新构建也通过。重复运行不叠加成新的测试数量。
59新ID由44 native、15 CLI组成，涵盖三套件、延迟解码、1/7/整段记录拆分、私有缓存、
相同端点的新会话不继承密钥/证书、重复值范围、JSON packet/stream worker及预算反例。

`fuzz-certificate559-01/` 使用最终插桩二进制，3/3种子、58636次执行、0 fail。
硬限1CPU、768MiB、同时2进程；峰值344526848 B，拥有进程随Job关闭。
目标包含内部向量、集合预算、每字节范围覆盖/不重叠及拥有数据不随输入修改，
不等于全部TLS阶段、消息分片及高级恢复已经验证。

`certificate-samples557-01/` 实际30次旧/新SDK、CLI、flow回放：
原流量和合法控制均35事件/12 HTTP，新旧其余事件逐字面不变；
三种坏向量原来35事件/12 HTTP，新版拒绝后为6事件/0 HTTP，不继续发布明文。
五个输入的新旧完整flow导出分别逐字节相同，全部41物理包仍在。
`certificate-errors579-01/` 另对原557实际CLI导出作严格补查：三个坏输入的帧6/PDU5
都保留具体extension错误、MalformedMessage和列表Info，所有行均无HTTP PDU及证书别名。
这补齐旧SDK导出不含error字段时的错误可见性验证，不修改557的脚本、失败或运行产物。
摘要SHA256 `dfaa7cc736ec0517050c935fd5045a97001a1006ebcaaceb8948d24b1f77b2af`。

## 两个大捕获及逐项差分

输入保持原SHA256：Weber `ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`，
Sharon `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
在新导出之前，556冻结旧事件中的一个TLS1.3证书候选；没有将后来变化的对象随意加入白名单。

`certificate-public-source563-01/` 独立遍历全部51328个Weber记录，
从接口243的原始IP/IPv6/TCP字节恢复目标37个物理记录和17个TLS记录，
13个保护记录完成AEAD认证，两次Finished MAC通过。
2972 B握手、两个证书条目的哈希及49522/49523/49524贡献范围与旧记录身份逐项相同。
原始时间戳字面量保留，不拿Wireshark stream编号代替连接事实。
`certificate-public-reference567-01/` 又对完整原捕获两遍回放，指定相同授权keylog、空配置、无Decode As，
证书条目及长度的8个字面检查与独立解密源相同。

`runtime-certificate-public560-01/` 六次新SDK/flow/CLI全捕获回放已完成。
`certificate-public-closure576-01/` 逐项核验306380个原生事件、379455个物理行及580个既有publication，
共380035个CLI行，没有重新编号、通用整数替换或额外丢弃：

| 输入 | 原生事件 | 物理行 / publication | 允许的变化 |
|---|---:|---:|---|
| Weber | 42749 | 51328 / 29 | 仅41538记录增加来源合格的正文/别名；49524行增加相应摘要和PDU筛选字段 |
| Sharon | 263631 | 328127 / 551 | SDK与CLI逐字节相同 |

两个flow导出逐字节相同，其余字面字段、来源、方向、域、流和行顺序均保持。
这是回归范围证明，不是所有协议与Wireshark全部正确性一致的证明，也不重新验收SMB。
`certificate-public-worker574-01/` 以原始RAW IP帧和当前实际DTO运行JSON worker：
完整2972 B拥有源相同，两个证书对应10个字面范围全部正确，没有借用物理帧或密文位置。

## 实际Windows页面及保留失败

`Windows-certificate573-01/` 为0f1 CLI和15290 QA工具实际运行，两页共46状态。
每页把完整860 B来源从真实字节窗逐页读回；728 B证书数据[11,739)及第二扩展55 B [805,860)
全部逐段核验高亮，区间外不被借用。packet/stream worker、鼠标/Tab、80×30与180×48及CP936恢复通过。
截图为真实console cells/attributes的固定字体渲染，不是所有Terminal字体/DPI/IME的验收。

最初566要求728 B在一个416 B可见窗口内全部显示，真实失败保留。
新增有界opt-in QA翻页，从起点到末尾检查全部字节；每个可见交集继续使用原exact_highlight断言，
并要求完整860 B覆盖、全部728/55个选中字节覆盖，没有删除或缩短原字段来通过。
558的首次独立读取把接口243当以太网而失败；563显式支持实际LINKTYPE_RAW，未猜偏移。
565的首次CLI校验误把类型化alias图当SDK正文树，失败保留；576按实际schema及全部同源值闭合。
538保留15个真实产品红测；541未初始化ClientRandom、543错误要求修改公开Raw后仍保留外层范围，
均保存并修正QA，输入拥有字节、派生值不变和错误范围撤销的断言保留。

## CI及剩余范围

577在读取未来产物前冻结313668个原始ID/终态：313643 pass /25历史optional skip。
父313609全部保持，59新增必须全pass，131084 required必须全pass/0skip。
manifest SHA256 `4adae739823abab72dc7ea18f18a2f80d5fd07378c24938bcd9b214b9dbc9edd`。
CI2worker/ASDU10161帧原ID、历史平台例外和Actions GUID skip均不改变。
最终说明提交仍须47个必需检查名、两个状态、实际checkout、官方artifact大小/SHA256/上传日志及
全部ID唯一run/terminal闭合；旧7a9成功不能代替。

证据摘要SHA256：

| 证据 | SHA256 |
|---|---|
| 新实抓独立552 | `99936664ba5827738451c7bb280e4b0a5b4d8747be69d52784f3a8603c3eb84f` |
| 衍生控制554 | `252b06cac06157aca60cfc1522897f4cc8ca2cf97595c8bfb0bd0e5254f06a9e` |
| 五次新参考555 | `825fd55867e65d0ef04671e18606e9d05a3ad2c73d67c73f27ed03922545c02a` |
| 30次实际回放557 | `c721db03a43c13c97aa2eb1d5337005eec096dfc102260000413dc1228a4525a` |
| 大包独立源563 | `87e7b689ce66bbb78a7c0ec565d06186c01b4d54bf64e6e6f996f6661261e336` |
| 大包新参考567 | `7ae988bd79cabbda6350291e00f14d66d160b4cefa419db0ee47154a42e281af` |
| 全量差分576 | `781251265bc7e1a908a459b4ff145d2278f3f144ba3732664a41e5da87085f7d` |
| 大包worker574 | `768e5b24f6f716c96222b8fd9bb49f5cc86eb59827b1177e86303cdf9768bc04` |
| 两实际Windows页573 | `6f3d85ede4d8fdad22c57b123a96abfcaf4f9ed6682cc46135e951bc2b660b80` |

本批关闭M2-56有限合同；总56父项，46有限完成、6进行中、1未开始、3 SMB搁置，仍剩10。
非SMB清单仍17包中11有限完成/6开放，粗估23–46 AI实施小时，等待及新发现另计。
N13完整阶段、角色与证书类型规则、PHA offer/context唯一性/关联、其他协商依赖及高级恢复仍开放；
TLS1.2新SYN政策、app-only QUIC、6 TCP/24 UDP会话政策、IP碎片基础仍按原清单继续。
核心协议没有宣告完备，两个捕获ZIP、来源/许可和样本manifest均未改变。
