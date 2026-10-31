# M2-42 TLS多记录物理帧的列表摘要

从最终全绿的 [PR #5382](https://github.com/yaklang/yaklang/pull/5382)
`13ae993d4f81a363d1b78d20439374184f62550c` 开分支 `codex/shark-m2-tls-row-priority`。
父提交47 checks+2 statuses全部success；traffic实际checkout/上传与下载artifact SHA均核实，
312560 pass/25历史optional skip/0fail，131084 required及新增8父/37终态ID逐个通过。
其闭合SHA256 `8058318def57b54478166e797860f87c9e038b2528461482d8a3a85b6e56d199`。

本批代码/干净构建为 `9af551414ebc8f47fb47f471e231ea52fbfbe37a`，
[PR #5383](https://github.com/yaklang/yaklang/pull/5383)正确以父分支为base。
实际运行时HEAD不冒称后续文档提交；最终交付SHA的完整CI另固定在PR说明和外部收据。
SMB按用户要求跳过，[已有缺口](shark-m2-smb-deferred-20261031.md)不分析、不扩实现。

问题与修改
----------

真实Windows214/222控制台中，同物理帧先有握手残片、后有完整KU时，列表保留前者长摘要，
把KeyUpdate枚举挤出可见区；握手/AppData非法交错时，原MalformedMessage原因还会被
后一个ContextRequired覆盖。详情正确不能算列表也正确，父PR明确保留这两处失败。

仅修改有界展示投影：

- 同优先级TLS完整record摘要前移，保留分片文字和原消息数量；即使完整record晚于前三个采样也可见。
- 保留最初的TLS MalformedMessage诊断，后续context故障单列，不用后果替换原因，也不重复原诊断。
- HTTP等更具体应用身份仍保留；只有诊断而没有成功消息时不据此建立TLS身份。
- 不重写事件、PDU、字段、源、范围、域、流或原生解析/重组状态。

新回归先在父实现实际复现3 pass/7 fail（含父/中间终态）；修复后10终态通过。
另加不覆盖应用身份、诊断不重复、无成功消息不伪造TLS身份后，完整CLI为
1066 pass/2历史optional skip/0fail。新增3父测试/11终态action ID，9叶；
不把终态数量冒称独立叶数量。原红日志SHA256
`38ccbfd73f5ad59c51e1fa554d756b2c924d7052e0fbea629c39e7f394f5990e`，
完整CLI日志SHA256 `e2f70afe803abddad046f0dca3f0b68169a753e1c48af459dbaa8be65de1d576`。

四个新Windows可执行文件实际构建成功。events、flows和实际Feed observer与196逐字节相同，
SHA分别为 `516b0c3f4eeed0a00ac70d82136473d3e75477cf7cd30912c7410423ef3f0d03`、
`07d51eeda82a67789c4ad3e07ddd6b9ebe960d321e3c5ef074d937c777f6c23c`、
`682315a7b56b148c43a28e273c40fdc870be451341db94dbfc9e19126fb964d4`；
新Yak为 `82329f6ad7646543fe864f8401786ae4cbbc885392090d6e90cf060ebea930b3`。
这同时限制本批范围：没有修改核心协议识别、TLS密钥/连接状态或字段输出。

Windows实际验收
---------------

外部证据根为 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
仍用176独立冻结的三份AEAD控制PCAP，不是新增真实peer抓包；源字节来自原真实OpenSSL131。
旧214/215失败及图片不覆盖。旧222补足source cycle后详情/来源通过，列表两例仍失败。
新229复用相同原始控制、独立期望和导航，旧197导航来自6bac并明确单列；
228与196的events可执行文件精确相同及两份闭合receipt是复用依据，不冒称197为9af执行。

新外部223 helper仅增加两次b/两份截图，依次验所有必需源。其首轮221构建超过180秒后终止，
不算产品失败/通过；成功223构建和源哈希另固定。不是修改产品以跳过UI条件。
9af新Yak实际执行三页、每页20状态，总60，全部exit0。

| 页面 | 必须可见的列表 | 选中字段与独立源 |
|---|---|---|
| 跨两个旧key record的合法KU | Key Update / update_not_requested | 认证重组握手5B的[4,5)，不借密文范围；目标wire record23B亦实际切换观察 |
| 同record双KU拒绝 | MalformedMessage及record boundary原因 | 原wire PDU32B，[3,5)为Record Length=27，无成功KU/握手源 |
| 握手分片中夹AppData拒绝 | MalformedMessage及interleaves原因 | 原wire PDU142B，[3,5)为Record Length=137，无伪HTTP或成功KU源 |

所有物理包/PDU/合法握手完整字节逐字节对应176，全部已显示source window唯一对应显式冻结源；
同帧其他record可作为wire源，不冒充当前PDU或成功握手。AEAD验证true与语义Malformed拒绝分列。
Header→Decoding→Observed connection、鼠标/Tab、180x48→80x30→180x48、CP936恢复均通过。
root另实际看完三页selected/narrow/restored九张新图并签新哈希；未复用旧视觉证明。
原list正负断言全部恢复且通过；有限UI接受为true。闭合SHA256
`691a03e19b2fdd50e67e4e66b935935defbe278dfc3ddbde06c61e5ca4663e6e`。
这些是实际console cells/attributes输出的渲染，不能代替Windows Terminal字体/DPI/IME全覆盖。

大捕获与剩余范围
----------------

两份大型公开捕获231由9af新构建完整重跑SDK/flow/CLI，6次实际执行全部exit0。
四份完整SDK/flow导出与199逐字节一致；Weber的51357行CLI也逐字节一致。
Sharon328127行CLI中仅5行的info/protocol_status变化；其他所有对象包括完整pdus/alias/source/
范围/域/协议stack/stream ID严格相同，0归一化。SDK仍为42223/263109事件、60/54协议，
事件数量与物理CLI帧数不混算。因完整SDK相同，才将137固定133项字段证据绑定新构建；
本轮没有fresh完整TLS TShark，不据此扩张到所有协议/字段/会话/fuzz parity。

232的v02 policy冻结时候选目录已由主线创建，但未读候选内容；首次“目录必须不存在”失败保留，
不冒称创建前冻结。允许变换及关联条件来自原199、已提交9af和先前约束。
仅允许字段列表不能自动通过：每项须原native TLS事件、同ID/flow/方向/端点、相同SourceBytes及
当前物理域/帧的PacketRefs。2正例/9合成适配器负控不算产品结果。
首轮机械门通过但5项人工复核待办仍失败保留；root随后完整对比新旧Info并直接读取原SDK：

| Sharon帧 / 接口 | 原native事实 | 新展示 |
|---|---|---|
| 173218 / 226 | 155665 TLS hello structure malformed，后155666 ContextRequired | 原因先显示；仍TCP，status改malformed |
| 225608 / 226 | 191635部分握手，后191636 invalid TLS record header | 保留片段，错误前置，mixed-record-outcomes；原TLS连接资格另审 |
| 309343 / 226 | 250332部分Certificate Request，后250333 invalid TLS record header | 保留片段，错误前置，mixed-record-outcomes；原TLS连接资格另审 |
| 311948 / 226 | 252252 hello structure malformed，后252253 ContextRequired | 两者保留且原因在前；仍TCP，status改malformed |
| 327689 / 226 | 262318 hello structure malformed，后262319 ContextRequired | 两者保留且原因在前；仍TCP，status改malformed |

root逐项证明SHA256 `72756d8f92e397aa8ed7b47ed11ccb8b221f627c8d57d7a9d76888c81866c937`。
这是把已有解析器事实正确显示，不等于上述native协议身份已与原wire/Wireshark全部验证。
最终232闭合SHA256 `5aad9c4d47c6afd9b094fe86f656ab08c4ec53b08816011d0c223eb428ce7220`；
精确交付SHA的CI见PR说明/外部收据。

Sharon输入SHA256 `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
新增M2-43/N13：225608的原SDK含Type6/Length0，309343含Type13/Length41，均
Handshake Incomplete=true、Authentication Verified=false，CLI仍有connection_protocol=TLS和
validated protocol events。当前只确认这些literal/展示及其未在本批改变；尚未独立提取物理源、
逐包核Wireshark及判断record framing、message资格和连接确认能否同等对待，不能直接宣称误识别已证或已修。
该两项在完成的UI修复之外继续开放，后续还须检查其对粘性、复用、丢失及接口/VLAN隔离的影响。

M2-42只是三页及有界摘要合同。M2-10.N07永久缺请求方法时的HTTP划分、N08 TLS1.2 C1、
N09 QUIC缺握手、N10六TCP分区及N11二十四UDP会话差异仍开放，粗估13–26 AI实施小时。
新增N13来源/资格调查1–2小时，若证实需要修复另估；六项合计14–28小时，不含执行等待、
N13后续修复及新增问题，不是全部核心协议完备的估算。旧七场景6项HTTP差异、C1八项、
QUIC两个app-only场景各2664项strict差异不被本次UI通过抹平。
N10.02来源/TSO显示审计未证明误合并，其他五个TCP例仍未审。

ZIP、manifest、批次index和规则归档不变。公开PCAP、keylog/密钥、完整导出、截图、
fuzz corpus全留仓库外；不改Git历史，不强推。当前批最终CI以精确交付SHA、实际checkout、
完整artifact及全部必需ID为准，父SHA成功不作替代。
