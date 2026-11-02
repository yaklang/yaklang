# IPv4异常选项后的父头观察、严格API和实际页面：2026-11-02

M2-65仅关闭N14.U01：Sharon原帧163432、180643、184529在IPv4 option解码出错以后，
仍应保留可独立核验的固定头观察及具体父层拒绝。它不是内层协议识别，也不关闭N14、
完整TLS/QUIC阶段或所有核心协议。SMB仍按用户要求跳过；只提交同一M2 PR #5371。

网络运行代码和干净三入口构建为`a9031b178b7c2fdbfad637a41e7395646abadc08`。
随后`d8d22200b255e8aa32d2b593a3e1bcb17c289b53`只更新内嵌统计API文档，
网络Go实现、测试和fixture字节不变。下文明确区分构建源码、文档同步及最终CI源码。

## 实际问题与修复

三帧的IHL均为24，option分别为`2e000400`、`2e140000`、`15008048`。
依赖解码器发现option长度为零或超过头部后，已保存24B原始LayerContents，
但Version、Protocol和Id尚未赋值，均为零。原父层检查使用解码后的Version/Protocol，
因而漏掉三条已定位的诊断。它们的字面Version为4、next为4、ID分别为4/6/0。

现在仅在完整IHL、字面IPv4、Total Length零、非分片及next为4/41同时成立时，
从当前物理头保留观察，继续拒绝内层归一化、重组和transport。
`ipv4-carrier-extent-refusal` / `IPv4CarrierExtentUnavailable`保留具体option错误和
`Parent Header Decode Complete=false`；不猜子端点，不创造Raw、PDU SHA、CompletedIn或连接。
其他next protocol不因此选择IPIP；不把异常option一般化为已接纳的有效IP包。

详情、quick、packet worker和stream worker统一使用字面ID及next名称`IPv4 (4)`/`IPv6 (41)`。
原始固定头观察先显示，生成诊断归入`Header observation context`。
观察上下文可以重复ID值，但不能借用原始字段范围；有当前物理源的ID和Total Length各只有一个真实范围。
无当前包的stream观察不获得假范围。

## API计数及文档

964旧观察器和985新观察器分别直接运行file/reader API；985共16次严格/宽松控制执行。
原包、原始option错误、DecodeErrors计数均保持：strict返回原错误，AllowIncomplete宽松政策保留继续处理。
SDK审计harness原本明确使用AllowIncomplete=true，exit0不是“丢失解码错误”的证明。
三原帧每次新增一个头部诊断，不建立内层TCP flow。

`Messages`统计已接纳消息边界；此类无完整消息边界的诊断为Messages0、Decoded0、Malformed1，
捕获包/字节仍按实际输入，GetFields继续拒绝假完整PDU。Malformed/Incomplete等诊断数不能简单相加
当消息总量，也不能用Messages代替所有回调次数。本批只澄清注释，没有修改计数实现。
内嵌补全数据仅更新`pcapx.pcap_onProtocolStats.Document`，其余结构语义经整体JSON及gob roundtrip核对不变。
新doc.gob.zst为1424377B，SHA256 `59ae8b0435ae64e249cc97868bb83b705cd88c04803d422f4c2f0370b699d1b6`。

## 原始来源、参考及完整大捕获

继续使用937的21个来源控制：9个真实DNS应用字节派生场景及12个未改变原包字节的公开异常父包。
不是21次新peer抓包。984实际执行147次旧/新SDK、旧/新flow、新CLI及两套显式TSO参考配置。
986完成968项断言、967项字面参考字段检查：7个DNS及17个父头拒绝；三个option诊断缺口全部补齐。
DNS所有头、问题原字节、四个answer/一个OPT的全部RR opaque RData、typed alias、贡献顺序、
CompletedIn、接口/VLAN域保持；不把opaque字节验收扩展为DNSSEC/全部RData语义。

TShark固定4.6.9，空个人配置，双遍，IP及TCP乱序重组打开，显式分TSO开/关。
Wireshark默认TSO对零IP长度推断的差异仍保留，关闭TSO的有限DNS控制与native数量一致。
本轮option拒绝不把依赖解码器的零值当原始字段，也不把默认参考标签当唯一正确性证明。
依据见[RFC791 §3.1](https://www.rfc-editor.org/rfc/rfc791.html#section-3.1)与
[Wireshark v4.6.9 IP实现](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-ip.c)。

982用干净a9031构建完整回放Weber、Sharon，SDK/flow/CLI共6个新导出。
Weber全部三个导出及两份完整TCP flow逐字节不变。996逐对象闭合Sharon264103→264106：
原264103事件的拥有字节、来源、状态、顺序、域、连接和全部导出字段保持；仅三个事前登记原帧增加诊断。
其余328124个物理CLI行及442个独立publication完整字段保持，物理行内异步published_pdus也全部检查。
只允许映射全局PDU引用的类型位置，不改wire ID、帧号、flow ID或任意数值。

来源与配置不变：Weber51328物理帧，SHA256
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`，使用外部tls.keys；
Sharon328127物理帧，SHA256 `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`，
不使用keylog。379455物理帧全部回放；捕获、keylog及完整分析导出均在仓库外。

## Windows、测试与保留失败

三原始包各自从PCAP运行实际Windows CLI；989/991共60个基础状态，另993选择三页滚动后的具体option错误。
64/64/111B物理帧完整读回；Total Length的Source0[16,18)或非零ID6的[18,20)精确高亮。
错误上下文本身无借用范围，breadcrumb仅Ethernet→IPv4，不显示DNS/TCP子身份。
鼠标/Tab、80×30窄屏/180×48恢复、CP936恢复及自有后代进程关闭已核；995主线程看六个实际PNG。
长错误在当前左侧视口按宽度截断，完整原错误仍保留在字段/API；不是全部字体、DPI、IME组合验收。

42个新精确ID全部要求pass：29 native、13 CLI，涵盖三option、file/reader×strict/loose、
workers1/2/4×开关协议消费者、字面ID/范围、其他carrier负控与四个fuzz种子。
977相关回归341pass/0skip；981独立插桩fuzz实际47454次，四种子完成，硬1CPU/768MiB job/最多2同时进程。
997文档同步后完整四包回归：280441pass、23历史optional skip、零fail；stdout SHA256 `a10e2ab079a77f4fba281771a33ebfed2efcfdadc66d3223ebf12354d09cf217`。全程输入/HEAD保持。
998内嵌文档测试1pass/0skip。没有删断言或修改既有捕获、全语料和Windows测试来过关。

保留所有失败：965揭示漏诊断；966新测试错误假设Messages包括头部诊断，修正为现有接纳边界语义并加捕获计数/GetFields拒绝；
972揭示页面投影缺失；973作者测试未设置声称的非零ID，修正输入；976新测试混淆带范围wire ID与无范围生成上下文，
加强为各一个且生成字段无范围。979完整回归发现嵌入文档陈旧，随后同步文档并重跑完整回归。
989测试错误要求未滚动视口含全部option错误，保留原结果并新增三页实际滚动检查；
988比较器末尾用错summary文件名，990全部比较通过但摘要沿用旧数字，996重新完整运行并从实际计数输出。
没有把失败悄悄标绿，也不将输出稳定性等同完整Wireshark正确性。

## 证据、CI及剩余工作

证据根`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`：

| 有限闭合 | SHA256 |
|---|---|
| IP-options-control-check986-01/summary986.json | `ee17b32b674b56ca6a99dbc3123ee37acd1f3dc0bc5482d698117d2826158cc8` |
| IP-options-public-check996-01/summary996.json | `79ba54e45afb68574f641637806547a74302f5027ff317bcaa5890a4aa420e8d` |
| IPv4-options-observer985-01/summary985.json | `4e7825a4468f512162f75fd3e8fed1b2aaa4093f279f965f24c84c95ee77dbb5` |
| IP-options-qualification999-01/summary999.json | `27c093d561076c08d73dedbd5dae167d51ef63f519f3a953ee843599e87895a8` |

两个fixture ZIP仍逐字节不变；PCAP、密钥、fuzz corpus、截图、完整导出均未入库。
944/975对已关闭的同内容导出先SHA及逐字节核对，再以硬链接保留全部路径，分别释放5676315340B和2325767270B；
旧不相同导出没有合并。当前保留8GiB运行下限，本次本地闭合可用空间46436364288B。

已发布父`93c852275052dd252be662136a97f61d6c017f8a`已独立47约定检查/91实际执行/2statuses全绿：
[流量专项](https://github.com/yaklang/yaklang/actions/runs/41381222731)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41381226049)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41381229232)。
完整314266 raw ID为314241pass/25历史optional skip，37新增及131084 required均pass/0skip；
artifact SHA256 `2689db86c5cab0c27d6e89ebf4fad08553d16adf5d9bab7c06298d6bfedb12ad`，
970综合闭合`3a7c946242f0926c7854f1d9351542808259f8cc10ad2d811040d40ad137ec1d`。

父成功不认证本批。新最终head须事前冻结314308 raw ID，保留全部父动作，42新增全部pass；
预期314283pass/25历史optional skip，131084 required pass/0skip，再闭合最终SHA全部CI和官方产物。
当前65父项中55有限完成、7进行中、3 SMB搁置，仍10未关闭；非SMB17包12有限完成/5开放。
N14.U01三帧有限合同关闭，但N14完整字段/transport代次、IPv6/其他carrier、冲突/缺片/预算仍开放，
N09 QUIC缺握手、N10六TCP/N11二十四UDP会话政策、N13完整阶段及协商依赖继续开放。
已知五包实施粗估20–40 AI小时，构建/CI/回放等待与新问题另计，不是核心协议完备承诺。
