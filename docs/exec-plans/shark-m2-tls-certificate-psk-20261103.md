# M2-78：RFC8773 Hello 接纳与 Windows 拒绝记录展示

本批有限验收为扩展33的空标志、CH/SH配套及提议关联、EE位置、原子接纳和拥有来源展示。
运行代码为 `bb217cad07c32388a9c2caf6a10ece172036060c`；原生核心为
`c4a2490b294d38e468ff6eb27dafd1cc2a3fc8ad`。父交付
`e31fc553c06c7e06fafa64ff42a112c46c3b55b6` 已独立47约定检查名、91执行检查及2 statuses全成功；
[traffic](https://github.com/yaklang/yaklang/actions/runs/41501019191)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41501026370)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41501034100)。本说明之后的最终head必须另验CI。

当前78父项、68有限完成、7进行中/3 SMB搁置，10项未关闭。非SMB仍17包、13有限完成/4开放
（N09/N10/N13/N14），已知剩余粗估16–32 AI实施小时，构建/CI/回放等待和新发现另计。
不把已完成子合同或导出稳定性写成全部核心协议、字段、会话或Wireshark正确性一致。

## 问题与最终行为

此前看见SH扩展33就放宽普通已选中PSK的证书认证限制，却未检查其空标志、Hello配套及客户端提议。
现在先检查完整向量和配套，再允许私有初始认证模式提交；失败不留下协商、retry或PSK状态。
依据 [RFC8773 §4–5.1](https://www.rfc-editor.org/rfc/rfc8773.html#section-4)：

- CH中的33必须为空，伴随PSK、key_share和包含psk_dhe_ke的modes；不允许同时early_data。
- SH中的33必须为空，并带TLS1.3版本声明、PSK选择和非空完整key_share向量；完整CH可证实是否提议过33。
- 不把缺CH当未提议，也不从票据年龄、端口或opaque identity猜测PSK为external或resumption。
- 保留合法空客户端key_share向量、未知模式枚举与DHE并存、仅DHE模式、未选择扩展的普通PSK回退和未知CH观察。
- EE中的33在native与独立正文API中均明确拒绝；跨两个已认证record的握手片段仍不能绕过位置检查。

RFC8773 §4的SH配套列表并不要求把CH专用45放入SH。本批没有错误添加这个要求。
上述检查不验证DH点、binder、签名、Finished MAC或PKI，也不完成其他消息位置和整个RFC8773阶段验证。

原生拒绝后还发现真实Windows详情回读存在“列表TLS、详情TCP”，或完整错误CH仍显示TCP/
“handshake not captured”。新增有界无状态 `ObserveTLSCertificatePSKHelloRecord` 只检查含33的
单个明文Hello外层，先验证完整向量/重复项/元素预算，再调用现有native framing reader；
它不接纳扩展语义，不修改连接，返回scalar观察与自有record偏移。

展示还要求对应的三个原生RFC8773错误之一、非零PDU/flow/generation、完整自有SHA和同域贡献引用，
以及一致record类型、版本、长度和明确未认证cleartext。完整错误CH/SH显示“观察到但被拒绝”，
协议列、详情及原包规则树一致；不虚构TLS版本，不建立后续ACK的粘性身份。
扩展显示具体名称 `tls_cert_with_extern_psk (33)`；type/length/data只定位自有record，错误文字没有借用范围。
短前缀、损坏尾向量、跨域来源、端口提示、伪造协商或缺少认证观察都不获得这个投影。

## 真实来源与逐包控制

使用本轮此前实抓的两个TLS1.3完整捕获：真正full→resumed，以及提供旧票据但被拒绝后完整握手回退。
原始来源由1328独立验证38 AEAD、3 CertificateVerify签名、8 Finished MAC及对端明文；
这些仅证明原始来源，产品和本批变异不因此宣称binder、完整握手或证书信任已验证。
没有把本批派生控制说成新对端实抓或合法变异transcript。

1444在原始23帧resumed连接中构造8类Hello故障；保持时间戳和其余TLS载荷，按长度差更新TCP序号/ACK、
IP长度及校验和，保留原帧映射和record SHA。对照旧版指旧版Yak Shark（98fd），Wireshark参考为4.6.9。

| 控制 | 旧Yak目标Hello | 新Yak目标Hello |
|---|---|---|
| SH未提议的空33 | decoded | malformed，不提交协商/PSK |
| SH非空33 | decoded | malformed |
| SH缺PSK | decoded | malformed |
| SH缺share | 原已malformed：真实CH只提议DHE | malformed，33配套在本地先拒绝 |
| SH零长share | 原已malformed | malformed，33向量在本地先拒绝 |
| CH非空33 | decoded | malformed |
| CH同时early_data | decoded | malformed |
| CH仅psk_ke | decoded；旧SH随后才拒绝DHE选择 | malformed，CH阶段即拒绝 |

不把两个继承拒绝算成新增误解码修复，也不声称8例原先都发布了HTTP。8例新目标均无成功正文alias、
无已接纳TLS1.3/PSK，后续HTTP均0；两个原始正控的SDK/CLI/flow完整输出逐字节相同。
最终1474共60次native执行。最终SDK10个完整事件导出还与c4旧验收相同，UI改动没有改写native事实。

1450以另一个真实完整连接的不同SYN ISN为good，转换其端口到相同tuple，保留TCP序号/ACK/flags及TLS字节。
2个bad源×复用/接口/VLAN×两种顺序共12个46帧profile；最终1475有48次执行、216个同域原始来源引用、
独立两个owner，good均2个HTTP/bad均0，完整flow输出保持。72个错误CH后的ACK没有粘性TLS；
错误SH前已存在的合法CH观察仅保留generic TLS，不虚构TLS1.3。
另有真实TLS1.2控制6次执行，三种完整输出保持；合计114次最终小源执行。

## Wireshark参考与仍存在的差异

8个隔离完整捕获均重新运行two-pass TShark、空配置、明确TLS端口、授权原keylog；
按目标Hello子树比较type/version/扩展type向量，24组字面值完全一致。第一版比较器错误包含同包EE/Finished，
原PDML和失败结果保留，1446仅纠正子树范围；没有把工具误差修成产品变化。

12个合并完整捕获另运行参考：4复用profile匹配，4接口和4VLAN仍有字段/初始消息差异，原结果保留。
20次新完整参考不是完整连接、协商或会话政策等价验收。Wireshark能展示字面扩展不意味着坏组合合法。
此前字段、会话、恢复和fuzz差异没有被本批一致性覆盖或清零。

## 本地测试、fuzz与实际Windows

没有改旧测试或弱化断言，新增3个测试文件共224个ID。最初1437为6pass/92fail（98 ID），
EE补充1439为141pass/13fail，展示red1459为11pass/3fail；实际worker1458也保存了错误缺握手提示。
1460的新测试停止诊断缺Raw，不能代表真实remainder；补真实完整remainder后再验，未放宽产品来源守卫。

基础完整回归1442：290771pass/25历史optional skip、0fail。执行HEAD仍为e31加六个待提交核心文件，
随后与c4干净构建的2419个Go/规则/归档等输入hash匹配；不是宣称当时已经在干净c4提交运行。
最终bb的native TLS/API为3012pass/0skip，完整CLI为1672pass/2历史skip；
严格并集290866 ID = 290841pass + 25历史skip，每个父本地ID/动作保持；不是最终bb的单次全套命令。

Windows既有6777个ID不在Linux清单，既有 `TestWindowsToPcapGuid` 为本地pass/Linux skip；
本批逐项保持各平台父基线。新增224均pass且与父Linux ID集合不交叉；最终CI事前清单为
315940 ID = 315915pass + 25历史optional skip，父315716 ID/动作完全保留，131084 required仍必须全pass/0skip。
自动draft skipped不计为执行成功，最终head的全部CI和官方产物仍须另验。

两轮非空插桩分别74312次/7种子（c4核心）、43399次/6种子（bb观察API）；1CPU、768MiB、最多2同时进程，
第三进程拒绝正控通过，cache/corpus/temp均在仓库外。c4已有核心Go代码在bb未改，仅新增无状态API和测试。

Windows两种真实派生故障：错误SH1225B、错误CH1779B；实际原生控制台两错误页54状态、两literal点击页40状态，
共94（12等待/82非等待）。完整错误、拥有记录全部字节、80×30/180×48、鼠标/Tab及CP936恢复通过。
type33精确高亮自有record的 `[1221,1223)` 和 `[1484,1486)`，都是0021；根agent目视检查四张最终页面。
图片和console状态不入库，位于仓库外 `Windows-cert-PSK1469-01` / `Windows-cert-PSK-literal1470-01`。

两份大公开捕获的六个最终完整导出逐字节保持，379455物理包/306855 SDK事件/379926 CLI含发布行；
先固定父候选及全部旧hash，按dataset先验证SDK一致，再运行CLI，不豁免新native差异。
原QUIC四域严格合同及checker未改，mismatch/coverage gap均0；不因此关闭高级QUIC恢复。

## 证据、资源与剩余工作

本批资格闭合1477 SHA256：`ebcda3673e60dc2d4274ab2da8f861dc0487667b2020b33c8b20d55b60e95704`。
关键来源1444：`cbe4e823abfe55530d9f088625eefd0fb42e2388e571494ed543aa2ed6485144`；
域来源1450：`f6d5d280ca603af27d5117d995100dfae5f6b6b8a2900579b7dcb67176cfa292`；
隔离参考1446：`cf5041b962de69d8ac6836fc60fa3234e40964769efca324994e8884a7aeda82`；
完整大包1466：`da3da3b81c2fd7515ce5ab2fa65ccb668682ef1b0c3ac2a2483a82d65b0e2cd2`。
详细输入、命令、二进制、来源、原始失败、控制台及引用映射均在仓库外资格目录的hash链中。

两受控fixture ZIP hash保持，外部大捕获、keylog/密钥、完整导出、PDML、fuzz材料和截图继续不入Git。
10个已关闭相同fuzz程序副本及新增6个相同旧导出改为hardlink，原路径、内容、hash保留；
9个既有NTFS压缩文件复核hash保持，未把其已压缩空间重复计为新回收。
构建结束后只清理显式Go构建缓存根中三天以上的hash条目37756个，逻辑95.35GB，实际可用空间增加约60GB；
达到64GiB保留目标，近期缓存、模块下载、代码、捕获和证据保持。资格闭合时可用66322264064B。

本批仅关闭A02的空标志/Hello配套及EE位置子合同。RFC8773完整HRR程序、其他消息位置、外部/恢复PSK身份证明、
具体配套诊断细分、完整必需消息/重新进入hello、非单record/多消息拒绝展示、opaque扩展内部语义仍开放。
随后继续N09 QUIC高级恢复/迁移、N10六个TCP generation/政策fuzz差异、N13完整阶段及N14完整IP重组/字段。
SMB S01–S06及107完整记录/121命令头、字段分母继续保留并跳过；主线程没有重新分析或修SMB。
