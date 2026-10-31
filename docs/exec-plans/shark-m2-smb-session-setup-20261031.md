# M2-34/35：SMB1 setup 与真实起始字节展示

本批位于 `codex/shark-m2-smb-session-setup`，
[PR #5378](https://github.com/yaklang/yaklang/pull/5378)，基线为尚未合并的
[#5377](https://github.com/yaklang/yaklang/pull/5377)。代码、三个实际构建及以下最终本地运行
均为 `9d29705ab79f4f0b9dd1a74defeffbf416726560`。后续说明提交不冒充新的代码构建；
最终 PR 提交仍须完整 CI，完成链接和收据记录于 PR。本记录没有宣称全部 SMB、核心协议
或 Wireshark 字段/会话/fuzz 正确性一致。

父 PR 的最终 `d47f13b6b8a7ac8fdcdf6288b7be0061a88dae51` 已核实47 checks及两个status全绿：
[Essential](https://github.com/yaklang/yaklang/actions/runs/41116659994)、
[traffic](https://github.com/yaklang/yaklang/actions/runs/41116665656)、
[diff](https://github.com/yaklang/yaklang/actions/runs/41116671318)、
[path case](https://github.com/yaklang/yaklang/actions/runs/41116677077)。下载 artifact、实际checkout
及131084个required结果均验证；312361 pass、25历史可选skip、0fail、无required skip。
闭合收据SHA256为 `896f5fa94e8fe3f121a8febfb173d9f94215b437d76d78b3801502074111f269`。
它不代替本批最终SHA的CI。

## 有限实现与边界

新增目录/规则/原生分发相符的两个partial profile：

| profile | canonical入口 | 实际来源 |
|---|---|---|
| `cifs-session-setup-extended-request` | `CIFSDirectTCPSessionSetupExtendedRequest` | 原frame19/21，WC12 |
| `cifs-session-setup-extended-response` | `CIFSDirectTCPSessionSetupExtendedResponse` | 原frame20/22，WC4 |

共享有界codec先验证完整Direct TCP长度、SMB签名/命令/方向、extended-security/Unicode、
固定words、ByteCount和blob边界，再发布Node/typed拥有值。UTF-16按code unit找NUL，
验证surrogate pair；没有用逐字节双零delimiter或静默替换字符。
decoded字符串范围不含2B终止符，独立raw alias包含终止符。blob、pad和未知尾部均有
真实范围和独立拥有字节，空buffer保留空slice/零宽范围。终止AndXCommand=ff时忽略
AndXOffset，原response412/242仍按literal保留；不把10个terminal样本说成真实compound链。

规范：[MS-SMB请求](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb/a00d0361-3544-4845-96ab-309b4bb7705d)、
[响应](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb/e5a467bc-cd36-4afa-825e-3f2a7bfd6189)、
[UID生成](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cifs/a4a70059-d263-47e0-be65-1f773a9b08be)。
UID是opaque16-bit，不能套用TID的ffff禁值；测试包括1/2048/fffe/ffff及多轮续接。
两条请求各20B额外尾部尚无标准语义证明，保持opaque，不照抄参考的PrimaryDomain装饰。
OEM、active AndX及其他错误响应布局未闭合；已知Windows padded single-NUL变化明确为
unsupported，不能因本profile不支持就说整个wire格式非法。

setup关联使用独立PID32/MID/请求方向命名空间，不借NEGOTIATE的PID或SMB2 pending。
私有root-request lineage将MORE_PROCESSING_REQUIRED响应UID传到下一次请求。
孤儿/重复/已完成key复用、冲突UID及旧outstanding续接不能借或重新播种旧上下文；
无法证明关闭后UID复用时保持unavailable/ambiguous，不伪称协议违规。接口/VLAN和TCP
generation继续隔离。观测到完整modern envelope（包括其body被拒绝）退休旧资格。
字面UID/Action/状态、matched response及续接ID均不构成认证/签名验证或已建立session。

临时map副本在分配前收费，删除后仍保留的bucket计入拥有量；连接高水位直到close释放。
固定布局和完整记录边界已经证明的坏string/已知unsupported body保留完整raw、Header和
原typed错误，退休对应lineage，然后仅在下一个声明的Direct TCP边界继续；不扫描找头、
不发表部分成功Body。资源/坏framing不获得这个恢复门。公开DTO修改不能改写私有拒绝。

M2-35先移除非HTTP流的HTTP缺头提示，真实Windows查看又发现完整已验证SMB1起始PDU
未用于证据页。补充要求domain、物理TCP generation、双端点、方向、offset0、完整raw长度、
全部prefix字节、source SHA256、完成来源及成功字段查询相符，才显示当前起始PDU。
连接标签、后续PDU或decrypted child不能替代这些证据；HTTP完整头仍独立验证。

## 构建、原始捕获及独立字段门

工具为Windows amd64/Go1.22.12、TShark4.6.9隔离来源。原始完整输入均在仓库外，未截取
后送入候选：`smb2-getinfo.cap`2096帧、`getinfo.cap`316帧、`smb2-peter.pcap`792帧。
捕获SHA256依次为：

```text
a2fbbbe0512da4fb4a741935d1d0b742855361152d629009d30404d66d673119
867e02b4442bb3fcb4a4c298b1a3252080eeaa3a28dd909cbb5b299c7506963f
eabad564ae039cbdc24af89eb9ec9333f0bf08a9963e41ade6caef2495f7bde3
```

`bins-70b`三个源码干净构建exit0；events/flows/Yak二进制SHA256依次为：

```text
08a8cff278b155fb7a3a649768da28ec52bcdcd67ba2070a747f4ed1fe0a1f63
4eb55d30afd92112c31f332ff9b60a419bb6307c1189fcba60c57a906c6495a1
739c194d8bcee5e09a296565b5a99de5cda62668beebcb5468ee9a800cf0201e
```

三个完整SDK回放exit0，忽略embedded secrets且未提供key。首份输入2061→2065事件：
四条setup现可解析并到达下一条frame23拒绝记录；SMB1共7个事件，SMB2仍2058。
其余两份整个SDK导出逐字节不变。

候选出现前冻结的SS checker02验证四条原PDU228/416/340/246B、两对关联及一次续接，
字段/Session双副本共244个literal/raw/string alias出现项、2214断言，0差异/缺失。
这些出现项不等于244个独立标准语义字段。收据
`M2-SS1-real-source-audit70b-01/summary02.json` SHA256为
`095592517fa4b57ed6106fc0885964e93eb6859a4b7c291883f04cf9e94b4950`。
M2-33的frame16/17仍为156出现项/1142断言/0缺口，收据SHA256
`8801e6cfd2d6a36c1412e35ff42883b259bc6b7fdb1c8b88292fd35becaa6744`。
三份捕获2350个QI命令、49178个固定头/body-envelope字面检查仍通过；六同域typed配对
21字段亦通过。opaque buffer不是已解析其全部内部字段。

## 本地、fuzz及大捕获

最终选定矩阵694 pass/0 skip/0fail；完整parser+rules+CLI为9667 pass/3历史可选skip/0fail，
不将重叠矩阵相加。完整日志SHA256：
`44ed963b4ede3b589f9bc80cfe80fbcf996887ce10914d369b735d4388e8af93`。
三个skip是PreparedOperatorRemainingInventory、NativePreviewExternalCaptureAudit和
WindowsTerminalConsoleChild；真实外部回放/Windows运行另有完整独立收据。

UID冲突真实红复现（success/MPR×旧续接是否outstanding，四叶及父失败）保留于
`test-M2-SS1-UID-red70-03`，日志SHA256
`dfcb75b802d6b5056c02c36a82b3d4dccbde74ac50b6cd292b6edb5e8197a5a6`。
修正后的多轮、未知/冲突/复用/重复/早到、独立PID、私有拒绝、预算/释放、1/7/64-byte
拆分、coalescing、真实TCP乱序补齐/重传/永久洞/新SYN及接口/VLAN矩阵均通过。
首轮canonical测试两个负控构造错误（flags2字节、surrogate偏移）也保留原失败，修正的是
负控输入，完整字段/范围/拥有值断言没有删除。

两个实际覆盖插桩fuzz各90秒，全部corpus/cache留在仓库外：

| target | executions | new coverage inputs | fail | 日志SHA256 |
|---|---:|---:|---:|---|
| SMB1SetupMessageSources | 132777 | 18 | 0 | `dc56f700d3a4ae798c599b0ae83639352dc479f6465ca29e9650d935667d983d` |
| SMB1SetupAssociationSequence | 37657 | 130 | 0 | `c9aa2a6570a055c6d31e28c0081a7170bb085ee19c3f2423922ceaba4f89a231` |

大包仍是外部完整The-Ultimate-PCAP，SHA256
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`。
`runtime-M2-Weber70b-01`三个新SDK/flow/CLI执行均exit0、输入与源码未变；42223个native事件、
51328个完整物理行、29个publication及整个flow导出均与前一已验构建逐字节相同，没有
重编号或归一化。整个三份导出相等收据SHA256为
`21608175b5778f08f5f84555d62f9c38a39b485d06ef081e0b00e944e70d5d39`。
这只是保留性。来源门仍为5 offers、107目标记录/121头和全部281记录；原strict checker
仍非零退出且保留1400 coverage gaps（1353未发表/47范围未映射），不是全面通过。
独立QI门93 PDU/113唯一命令/731固定字段通过；125/809是含根兼容副本的检查次数，
不能当唯一命令数。另392项映射闭合后余1008，未把旧checker期望改成零。

## 实际Windows页面

原始2096帧完整回放，真实native-console cells/attributes渲染；不是Terminal窗口像素截图。
源码计划、完整物理/逻辑PDU及预期范围先冻结，再运行9d新Yak。
`windows-M2-SS1-setup70b-01`两页55个状态：frame20 Native OS decoded[226,320)94B与
raw[226,322)96B只差2B NUL；frame21 unknown tail[320,340)20B仍opaque。
DirectTCP→Header→Body→观察上下文、完整416/340B PDU、精确highlight、mouse/Tab、
80×30及180×48、quit/CP936恢复、与实际wire相符的两方向起始PDU均由主agent看图验收。
物理frame482B只核可见窗口及完整源hash，不冒充全部同时显示；frame406B同样单列来源。

首次70构建确实存在未知prefix提示，证据保留；70b加入完整当前wire PDU证据后重跑。
旧close70检查器误要求恢复到Stream Evidence页仍有Fields的#frame标题；close71保留旧文件，
改以所有状态的准确物理帧/长度/命令与源绑定pdu.id查询核验，三个Fields状态另须#frame，
恢复状态另须Evidence页；2正控/6负控通过。未移除任何字节、高亮、来源或交互断言。
主agent还曾误读缩放图片为通用request；原始字符复核旧/新两次32个宽屏状态均为正确
continuation阶段，并无产品回退。误判及更正均保留，没有为不存在的回退改代码。
最终两页收据SHA256为 `505f839585a6694fc16b61b6512c3a8a69a44df83fd8fe636a5084f357c4d607`，
独立原始字符更正收据为 `14abe45220ce977ca60c1f5f4094e0a7c6f733cdcce976c58e6916b12ad76be3`。

## 继续开放的范围

M2-34/35只验收上述有限合同。原22个SMB1完整PDU中其余16条/7类命令追加M2-36，
不因连接仍有SMB1标签就说正文已解析。SPNEGO/NTLM token语义、OEM、active AndX、
其他错误布局、其余typed QI以及高级状态仍开放；不能把server status success当身份验证。
M2-08/09的source71及TLS六case72已在仓库外冻结；新增真实删包/晚到/跨SYN派生均明确
区别于现场新抓故障。它们的源/AEAD/TShark检查不等于候选已通过；产品和callback到达
时序验收尚待后续分支，CompletedIn不冒充较晚交付时机。

本批没有新增PCAP、keylog、raw PDU或完整分析导出到Git。唯一capture ZIP仍18916097B，
SHA256 `0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`；
manifest/index旧hash不变。仅同步规则源及embed archive，后者SHA256为
`246c5d57806597efc0be6e7fd63e734cdd6659f0c0f13d8a60fb409cc34c8391`。
