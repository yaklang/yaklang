# M2 QUIC CRYPTO 资格、拒绝字节与实际页面验收

本批关闭 M2-48..52 五个有限合同，代码和最终干净构建均为
`0fc42e285292b38090c24e77d114bdb7da009d61`。继续同一个 M2 分支和
[PR #5371](https://github.com/yaklang/yaklang/pull/5371)，没有创建第三个阶段或改写历史。
M1 #5353 已合并，SMB 继续按[任务书](shark-m2-smb-taskbook-20261101.md)搁置。
这些结果不表示全部 TLS/QUIC 阶段、恢复边界或核心协议完备，也不把输出相同当成与
Wireshark 全部字段/会话正确性相同。最终说明提交的 CI 归 M2-11，必须按 PR 实际 head 验收。

## 已实现的有限合同

| 项 | 实现与拒绝门 | 保留边界 |
|---|---|---|
| M2-48 | QUIC 的 Initial 只接纳 ClientHello/ServerHello；Handshake 接纳 EE/Certificate/CR/CV/Finished/CompressedCertificate 消息封套；Application 接纳 NST。TLS EndOfEarlyData、KeyUpdate 和错误层级拒绝。未知消息或未来版本明确 unsupported | CompressedCertificate 仅识别消息封套，未实现解压/证书验证；未宣称完整 TLS 状态机 |
| M2-48 | QUIC Hello 必须符合已支持的 TLS1.3 profile；客户端不得提议旧 TLS，不能由 legacy word 猜协商结果；NST 需要观察到的版本，early_data 最大值必须为 0xffffffff | 不收紧 TLS over TCP 的合法票据值；QUIC 的 0-RTT/传输参数和完整恢复仍开放 |
| M2-48 | 一次 CRYPTO delivery 中完整 TLS 消息批先验证再提交；合法前缀加错误后项不留下半提交的 TLS/ALPN/read 状态。观察到 Finished 后不能延长该方向的 Handshake CRYPTO，残头及 pending 尾也拒绝；完全重传保留 | TLS 消息批原子性不等于所有 QUIC 帧或整包回滚，密钥供应/其他帧的整个状态事务另属后续范围 |
| M2-49 | 同一 UDP 数据报合并多个包时，列表先显示拒绝原因，保留邻包摘要；标明 mixed-packet-outcomes。详情诊断在大的正常包树之前，不被后续 context 原因覆盖，不重复同一错误 | 已解析 HTTP/3 等应用身份继续优先；错误本身不建立没有依据的 QUIC 身份 |
| M2-50 | 已由 native 接纳、认证和层级合格的当前 CRYPTO 消息显示具体名称；完成消息不被普通包号摘要挤出采样预算，残缺 CRYPTO 明示重组未完成 | Initial 的公开保护不认证对端、不选择协商版本。与 HTTP/3 同帧时应用摘要优先，握手仍完整保留在 PDU 快照/详情 |
| M2-51 | applyFrames 拒绝以及其重传保留完整拥有解密载荷、SHA256 和诊断来源；不指向外层密文。先前帧可能已生效时明确 parsing stopped，不写成整包未交付 | 先前合法 CID 帧实际生效的反例已验；拒绝 TLS 批仍不推进。诊断用 PacketRefs 归因，不捏造 decoded-message CompletedIn |
| M2-52 | CRYPTO 向量在本地和 JSON worker 中均显示 HEX 字节，长度/认证/帧上下文合格才提供独立字节源；类型→偏移→长度→数据递进 | 无认证、缺声明长度、长度不符或脱离帧的元数据不建立认证明文字节范围；偏移是拥有字节源坐标 |

依据：[RFC9001 §4.1.3/4.2/4.4/4.6/6/8.3](https://www.rfc-editor.org/rfc/rfc9001.html)
与 [RFC8879](https://www.rfc-editor.org/rfc/rfc8879.html)。包 AEAD 成功与 TLS 消息在该层级合法
分别验证；不把 Wireshark 展示消息正文当成规范接纳证明。当前产品没有因此增加签名、
Finished MAC、证书信任或完整 PHA 上下文验证。

## 新抓真实流量与独立来源

外部目录根为 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`，下表路径均相对此目录。
使用实际 Windows Npcap Loopback、aioquic 1.7.0 对端、Go1.22.12 windows/amd64、
同一 TShark 二进制（SHA256 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`）。
TShark 空配置、两遍、关闭名称解析；实抓参考仅对声明的自有服务器端口做 Decode As。
公开大包参考不做 Decode As。完整原始包、keylog/私钥、分析导出和截图继续全部在仓库外。
本次没有修改捕获 ZIP 或将外部捕获/密钥放入仓库。

- `QUIC-level-source394-01` 是新的真实 UDP 抓包：2046 个物理帧（2 个 readiness marker），
  2053 个受保护包；独立 AEAD 成功 2050，3 个实际损坏 tag 不推断 PN/明文。
  3 条新连接共用原客户端 socket，分别 AES128-GCM、AES256-GCM、ChaCha20-Poly1305；
  18 次实际请求，6 次客户端换钥请求、3 次 CID 轮换、3 次 NAT 端口变化。
- 对端发送路径实际记录 12 次丢弃、187 次延迟和3次损坏；源解密确认184个乱序包，
  双向 epoch 0/1/2。丢弃次数来自对端记录，不能凭未见 wire 给每个缺包编造物理帧。
  这3条是新的完整连接，不宣称 TLS resumption/0-RTT 对端验收。
- `QUIC-level-source396-02` 从原包、ClientHello random 和自有 keylog 独立恢复 AEAD、
  CID、PN、CRYPTO/STREAM/QPACK，先得出源事实再与参考比。36 个 HTTP/3 头和36个正文
  都符合真实对端哈希，81 个完整逻辑帧和36个完整流无洞拼接。
- `QUIC-level-reference397-01` 验证18个 cipher/direction/epoch 源单元，138个错误密钥/
  密文/AAD 假设全部拒绝；73537项参考字段检查无差异。参考少1个完整 DATA view，另有
  1个重复 HEADERS view，均按独立来源单列，没有减少产品需要交付的36个正文。
- 最终 `fresh-QUIC-final437-01` 复核6次完整 SDK/CLI/flow 执行，源 packet/frame/blob
  检查14153项、应用来源范围2622项，全部2050认证包、21条握手封套和36个完整流通过。
  SDK 和 flow 相对488逐字节相同，CLI只有6行 Info 补充来源已确认的名称；其余字段、
  ID、raw、域、来源和所有 PDU 快照相同。与 Finished 同帧的 HTTP/3 摘要保持应用优先。

## AEAD 有效的受控边界与字段

`QUIC-level-controls398-02` 是从上述实抓原包构造的30个捕获前缀，不是30次新抓流量。
保留源帧编号/顺序、PN、长度，独立重新加密并验证 AEAD、错误 tag 反例和 UDP checksum；
24个错误 Application 层 CR/CV/Finished/KeyUpdate，3个非法 NST early_data 值，
3个合法 0xffffffff NST 对照。所有未改前缀字节原样保留。

旧488构建实际60次 SDK/CLI：27个非法样本中18个被当正常消息接纳，3个合法对照通过。
最终0fc的 `QUIC-controls-final431-01` 同样60次执行：27个非法全部拒绝、3个合法全通过；
全部30个保留 QUIC 身份，非法消息不产生 TLS Handshakes，拒绝原因位于 Info 开头。
`QUIC-controls-closure438-02` 进一步独立解密全部27个完整拒绝载荷并核对其SHA、归因和
SDK/CLI快照；3个合法票据核对30个 scalar/digest aliases及21项新参考值/相对消息范围。
不把明文相对范围标成捕获密文坐标。票据/证书信任以及跨连接票据关联未因此实现。

Wireshark 对30个受控样本均展示了握手消息。包号/解密观察与源一致，不代表它对27个非法
位置作了规范接纳判断；本系统按上述 QUIC 规则拒绝，不能以参考也展示为理由误接纳。

## 实际 Windows 页面

最终 `Windows-QUIC432-02` 四页 **83状态**：CR错误18、KeyUpdate错误22、NST错误25、
合法NST18。1034/1020/1173B完整拥有拒绝载荷与独立解密逐字节相同，页面可见416B前缀
逐字节和高亮核对；合法CRYPTO完整26B全部显示并高亮。完整来源由worker数据核验，
不是声称每个大载荷在一个viewport里全部可见。诊断/具体消息名在实际列表行中可见。
鼠标、Tab、80×30/180×48缩放、CP936恢复均通过。
图来自真实console cells/attributes固定渲染，未推广到Terminal字体、DPI、IME等全部行为。

首次实际检查发现：CRYPTO拒绝缺少可查看的完整明文，合法Crypto Data在JSON worker变成
Base64且没有range。0fc修复两条；其消息前移/错误前移代码分别为89f/3a，原生层级修复90bc。
最终图像和完整快照只留在外部目录，不进入捕获ZIP。

## 本地测试与保留的失败

- 原层级/批事务红测384为8pass/15fail；版本/票据政策红测386为5pass/13fail；
  Finished尾红测392为9fail。对应绿测及其输入/输出SHA均保留，不能把旧失败改成通过。
- 389前置完整同CI范围313332pass/25历史optional skip/0fail，输入未变。
  它早于0fc的错误载荷路径修改，不称为最终0fc完整重跑；最终原生QUIC423为388pass/0skip，
  CLI426为1182pass/2历史optional skip，精确最终交付的全量回归另由CI验收。
- coalesced展示红406-02为21fail；名称红414为8pass/7fail；错误载荷红421为10fail；
  Crypto Data红425为5pass/3fail。另保留406-01无有效测试终态的编译失误，不计成有效红测。
- 插桩410验证CRYPTO消息资格/批状态/拥有字节，3a构建，5/5种子、59692执行、0fail；
  插桩430验证认证后拒绝载荷/重复包/状态与字节拥有，0fc构建，5/5种子、57193执行、0fail。
  都在仓库外cwd/cache/corpus/temp，硬限1CPU、总768MiB、最多2个同时进程，峰值约205/337MiB；
  关闭job后按准确拥有路径确认没有残留。不是对全部QUIC恢复或任意协议fuzz的验收。
- 396首次少提取两个审计依赖，398首次只认微秒PCAP；后续适配保留实际纳秒精度/原包元数据。
  432首次是控制器模板变量重名，未运行UI；02修正控制器，未更改产品或字节预期。
- 434首次要求HTTP/3应用行也显示载体Finished，超出既定应用优先规则。437只重查已冻结的
  6次导出，仍要求全部21条源握手在PDU中存在、所有别名/来源不变，载体行具体显示；未删源消息。
- 438首次对拒绝诊断也要求CompletedIn；02以精确PacketRefs及Raw SHA归因，不伪造消息完成。
  439首次TShark在512MiB限额内分配失败；02按已约定2CPU/1GiB限额重跑，峰值约567MiB。
  两者都不是产品通过证据，也没有放宽字段、包号或完整性断言。

## 两大原始捕获与证据闭合

最终0fc六次完整回放435覆盖Weber51328帧、Sharon328127帧。
相对488，42223和263109个SDK事件均逐字节相同；824/840个flow和14172/7700次callback
的完整flow导出也分别逐字节相同。CLI只有Weber48060/48072/48084/48094/48107/48120
六行补充TLS Client Hello；其他所有字段/ID/raw/来源/域/publication完全相同，Sharon无变化。
439-02从全部51328个源包中定位六个唯一原字节窗口，以公开Initial密钥独立AEAD及完整
ClientHello结构验证，再跑完整输入两遍参考（只限制六帧PDML序列化），类型/长度/PN全部一致。
Initial验证不证明对端身份，也不从ClientHello提议推导协商版本。

原全帧参考列的literal相同仍为Weber38703/51328、Sharon39777/328127。
这不是正确率，也不把其余差异全部算成bug；原字段/会话/fuzz差异继续归逐项清单。

| 外部闭合文件 | SHA256 |
|---|---|
| `QUIC-level-source396-02/summary.json` | `b1c1a5d87bc3fd6309092f5e407d4a8273ae439a56ffddccccd968a8cce42c26` |
| `QUIC-level-reference397-01/summary397.json` | `1ab67873f921d54081d6a2f47122bad0b321a9487a1807123b86b4d9ff55e4f1` |
| `QUIC-controls-final431-01/summary400.json` | `03a3fbc5bab7bf409180f6b1b75e3bea63a4233be0480c71240f3896b06a1c7f` |
| `QUIC-controls-closure438-02/summary438.json` | `a398685f01607c55152464f3a636382050a1f29c918a6a57818d3c1e9049e009` |
| `Windows-QUIC432-02/closure432.json` | `1efc6017cdb53cc559f86eac1336d2c8eee959b893bd9e4f85384ec927725d48` |
| `fresh-QUIC-final437-01/summary404.json` | `0390d142fb0f18f24f8343e9abd9280ba03237cc4c8889f3d79e75011bf5d587` |
| `runtime-QUIC-public435-01/summary435.json` | `dc13af3cb7346c95afb25e564267a28ff6b312378d2b262bd7b4d08fa199bb54` |
| `QUIC-public-comparison436-01/summary436.json` | `0920d2953396ae1c627daa0ae2ce719a5a2b214ef1b493eac4760923d23d9100` |
| `QUIC-public-source439-02/summary439.json` | `8a088d1f03638db75e50840470eb497658e7ec90e6a7d5c2292713560e956e3d` |

新CI产物读取前，`QUIC-CI427-02` 已冻结313400个原始ID/终态：313375pass、25历史optional skip；
保留631的313256个ID及终态，144新增（89native/55CLI）全部必须pass。
131084 required必须全pass/0skip。manifest SHA256
`f3e21f939afa21d0d53a8559ac3fa86d2caeb7cdf6f7deeaa803df1a41eca4c3`。
SV10161帧保留CI2worker原始ID并恰好一次，本地3worker/Windows-only及既有Actions GUID skip
分别处理；不更名CI实际ID、不删断言或把旧SHA/自动draft skipped算本次通过。

N13的PHA offer/上下文唯一性/多轮关联、完整TLS阶段、QUIC协商依赖/传输参数、缺握手恢复，
以及N07/N08/N09/N10/N11/N14继续开放。全部消息批/包级事务、0-RTT及完整证书处理不在本批关闭。
