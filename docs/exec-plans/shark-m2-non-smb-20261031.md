# M2 非 SMB：TLS KeyUpdate、HTTP 迟到上下文与原差异复核

2026-10-31 按用户要求停止分析 SMB，缺口保留在
[SMB 搁置记录](shark-m2-smb-deferred-20261031.md) 和草稿
[#5380](https://github.com/yaklang/yaklang/pull/5380)。本批
[#5381](https://github.com/yaklang/yaklang/pull/5381) 从全绿
`8ed202b62d56ceaedf8a00cea0806090daeed193`（#5379）分出，
只携带 SMB 搁置文档，不携带 #5380 的 SMB 生产改动。

本批字段提交 `b2a0be7568f9478a55592e053454cd9aea8ef807`，HTTP 修复及
最终运行代码为 `4477ebd3d3c718481f9311b3fad133a3d43ab53a`。
报告提交不改写这些构建、dirty 输入收据和历史失败的 source。
完整 M2-08/09、全部协议与 Wireshark 的字段/会话一致性仍未完成。

## 两个实际问题与修复边界

KeyUpdate 在旧实现已驱动更新密钥，CLI 也已有 Key Update 名称，但 SDK 缺少
`request_update`、枚举名和完整握手字节，SDK Summary 仍笼统显示 Application Data。
现在仅在 TLS1.3、已认证、应用阶段、正文恰为一个值为 0/1 的字节时发表：

- `Key Update Request`（uint8）、`Key Update Request Name`、owned 5-byte
  `Encoded Key Update`；无效版本、状态、长度或枚举拒绝，不提前更新密钥/序号。
- 官方别名 `tls.handshake.key_update.request_update`。沿用现有 v1 schema 的
  `origin=derived` 表示解密所得，描述明确它是认证后 literal，不附假物理 PDU range。
- 页面逐层保留外层 Application Data(23)/legacy 0x0303/22-byte ciphertext payload、
  认证结果、内层 Handshake(22) 和 Key Update。新增独立握手明文来源；请求字节
  的 `[4,5)` 只指向该 5-byte 来源，不能指向物理密文。

第二处问题来自真实流量的迟到/乱序控制：TLS 已正确恢复 AEAD 和全部 KeyUpdate，
HTTP 却因请求方法尚未到达而永久停止响应方向。直接返回 NeedMore 也不够，旧半初始化
header 会绕过正文与隧道长度初始化。修复仅对本 flow 此前成功提交过真实 HTTP 请求头、
且请求方向唯一的连接有界等待；真实方法、请求 ID 与所有队列预算成功提交后，重新解析
原响应。普通完整请求先发表再重放；Expect100 可在完整请求头之后、正文完成之前重放。
HEAD 不吞后续响应，CONNECT 不把隧道当 HTTP body；保留原响应贡献包、父 PDU 与时间。

初始孤儿响应保持原即时 context-required；永久没有方法时，不默认 GET，不扫描下一条
看似 header 的内容。等待受原 stream/capture 预算约束，额外 header 状态单独计费；
禁止 payload-omission 跳边界，Close 仅发表完整已验证头及明确未知的正文边界并回收预算。
角色、队列和等待状态保留在私有 flow 中；公共 event 改写不能授权重放。

## 真实来源与独立参考

证据根仍为仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
`M2-TLS13-keyupdate-source131-01/real-peer03/live.pcap` 是本机 OpenSSL1.1.1s
loopback 实际 SSL_key_update 通信，非手工构造密文：53 个物理帧、29 个 TLS record、
25 个 AEAD record、2 个独立 Finished HMAC、6 次 KeyUpdate、12 条 HTTP 消息。
4 次显式双向请求和 2 次自动响应，更新值依次为 0/0/1/0/1/0，两方向均到 epoch3。

抓包 SHA256 `a17eac72939386defbda490063d0d1829b91fb4d1c9b59a0f62d13dae63adb0e`；
独立 facts `859ebe4ecd6702fbd9fa0a2c5293e893abe4ad71a9d7c5dd01ab88e102b1d00b`；
冻结合同 `aa440c9cb08fe52d904e2847834170840a0aacc76eb779f8f22d326da0040e17`。
Python 从初始 traffic secret 独立 HKDF/AES-GCM，只有认证 KeyUpdate 后派生下一代，
没有拿后续 secret 预解密。所有 12 条明文与真实 peer 收发字节精确对应。

TShark4.6.9（binary SHA662e10fd…1f69b）隔离配置、完整53帧 PDML/fields 实际运行，
29 个外层头、6 个官方 KeyUpdate 值、6 个 literal URI 和6个 body与来源一致。
PDML decrypted-view position 不是物理明文位置；size0装饰 URI 不混入 literal 计数。
最早只搜索 handshake 子树漏看 sibling 字段的错误已纠正，原参考保留。
字段可对照 [Wireshark TLS 参考](https://www.wireshark.org/docs/dfref/t/tls.html)，
更新规则对应 [RFC8446 §4.6.3](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.6.3)。

6 个控制捕获只删除/调换原捕获包，原序号、时间戳和包内字节保持。冻结131/142的
预期和 strict-prefix 永久洞策略在观察新结果之前选定，没有根据候选输出改来源。

## 本地测试与保留的失败

| 证据 | 实际结果与解释 |
|---|---|
| KeyUpdate red140 | 5pass/5fail，确有字段、版本、SDK Summary 和页面来源缺口 |
| broad144 | 3517pass/1fail，新增 origin 超出原 v1 schema；改用既有 derived，没有放宽旧注册表测试 |
| broad146、CLI146 | 3518pass及1054pass/2历史optional skip，两个矩阵不相加 |
| fuzz151 | b2a，90s/106344 exec/0fail；真正插桩；从认证后握手状态开始，不冒称完整 AEAD fuzz |
| HTTP156/157 | 新测试对 *ProtocolError typed-nil 误用 NoError；属于 harness 错误，不能作产品红绿证据 |
| HTTP红158 | 仅将新测试 typed pointer 的检查改为 Nil 后，原生产代码1pass/8fail；实际失败在响应仍被立即停止 |
| HTTP绿160 | 保留同一组协议断言，9pass/0skip/0fail（7个Test函数及GET/HEAD两子例，共9terminal） |
| broad161 | HTTP/TLS/QUIC/WebSocket/DoH/IPP/字段相关5002pass/0skip/0fail |
| CLI166 | 全包1054pass/2历史optional skip/0fail |

绿160 stdout SHA `3274e6ff2d255a5ed722985f7a8e918fb8da264ebf7559f534c156be2a0ab5ab`；
broad161 `738d5cb10a954a85f1bd2f08451272996f3b9d5863cd1305fca3b6b151937d11`；
CLI166 `2e0b1f380603ccb8f63bedf7ce331662288843fd48fa310b13601a63daa906f3`。
以上 HTTP 测试执行时 HEAD 为 b2a 加已固定 dirty 输入，随后提交4477，不改写收据。
本批13个新增父测试/32个实际terminal已冻结为最终CI必跑清单，包含普通fuzz seeds；
本地pass不代替最终SHA的实际CI执行。
Go1.22.12 Windows/amd64；Go/SDK/UI顺序执行且运行时源码/index/输入不变。
构建/普通测试3CPU/2GiB，fuzz1CPU/768MiB，UI1CPU/512MiB。

## 4477 实际七场景、回调与 Windows

构建162的 events/flows/yak/arrival 四个二进制均干净4477、exit0、输入不变。
Yak SHA `7d37d97c90977a9f53124ca3cf50677955dacbe876b100ace2a80e40e98dd8b3`；
events `fc94f66257bcf9ece0b0da489619c98820a3421b27e6e025628fae11eda12f03`。
7 个普通 SDK 及 7 个独立实际 Feed/Close observer 进程全部0退出；observer使用
Workers=1，每次真实Feed/Close边界写atomic marker，不以CompletedIn代替回调时刻。
每个observer的完整SDK导出都与对应普通SDK逐字节一致，trace仅绑定同一份导出hash。

| 真实控制 | TLS/AEAD/KeyUpdate | 完整HTTP | 原严格142与实际回调门 |
|---|---|---|---|
| 原始 | 29/25/6 | 12（6请求、6响应） | 0diff/0gap，通过 |
| 客户端KU0迟到 | 29/25/6 | 12 | 旧6差异修复为0diff/0gap，通过 |
| 客户端epoch1应用乱序 | 29/25/6 | 12 | 旧6差异修复为0diff/0gap，通过 |
| 服务端KU0迟到、epoch1应用乱序（各一例） | 各29/25/6 | 各12 | 各0diff/0gap，通过 |
| 服务端KU0永久丢失 | 21/17/3 | 8（6请求、2响应） | 0diff/0gap，有明确未填TCP洞，通过 |
| 客户端KU0永久丢失 | 20/16/3 | 2（1请求、1响应）另保留完整未知方法响应头 | 原严格合同仍6diff/0gap，不通过，不修改原预期 |

完整原始及四个迟到/乱序场景分别验证412个native literal、130个alias、12个owned
KeyUpdate副本、41个实际callback下界；这些是各次计数，不相加当独立功能数量。
永久客户端场景没有实际请求方法，停止确认后续body边界；TLS record/KeyUpdate均正确。
保留原6项HTTP完整性差异，不将保守拒绝写成与参考全部一致。

新参考169对以上7份完整输入各跑一份两遍PDML，加version/folders共9个进程，顺序
硬限制2CPU/512MiB且全部关闭。七例外层TLS头和KeyUpdate方向/值计数与独立来源完全
相同；所有发表的literal URI/status/body均核原peer字节，size0 URI仍不计新消息。
永久客户端情况下，Wireshark能显示6个HTTP响应，本系统只接受1个完整响应及后续头诊断；
这项差别明确保留，不能由Wireshark的显示替代缺失请求方法的独立证明。

`summary163.json` SHA `e1441180e13b594076225d3d04620dec6e15d377fb036ab609c99ed4764db298`；
实际回调167 `1026cd81bca54106eb5d3aca5cfd0a1816e9b99c0d1adb0b98fb3ffd86754b71`；
新参考169 `4d4debb941faa6971e54a258d6f51bbab1cd8e4da9428d8071b8ba9b07ea1bd6`。

Windows165/170用4477真正启动UI，两页（原26/36帧）36个native console状态均通过：
完整71-byte物理帧、27-byte密文TLS PDU、5-byte认证握手来源相互独立；请求值只高亮
offset4；mouse/tabs、80/180列、Stream起点、CP936退出恢复均核验。实际查看两张选择
页与窄屏页；截图来自真实console cells/attributes，不声称Terminal像素/字体/DPI/IME。
闭合SHA `46db9a7fb1cf792d5de4b7bb0d0aeff4b368d7bf5bb01dee0e733efad7b35cbf`。

两个完整大捕获的4477构建162/回放164/闭合173均已结束：

| 捕获 | SDK事件 / 协议事件种类 | SDK / CLI / flow 与4bfb基线133 |
|---|---|---|
| Weber，原PCAPNG SHA ecbca543…73f4 | 42223 / 60 | 三份完整导出逐字节相同 |
| Sharon，原PCAP SHA aa4fcc81…714c | 263109 / 54 | 三份完整导出逐字节相同 |

6个实际进程exit0，inputs/source不变。没有删除字段、归一化ID或容忍重排；完整SDK
对象包含字段、session和来源。两个大捕获没有KeyUpdate，故真实更新验收仍来自131。
闭合173 SHA `022c1ff2b65bc1ec2f1a4f448fb76df9ba9bbae3cd3be177dc753dab8ef5440e`。
各协议事件计数及输入/产物摘要另存本批summary；事件不是物理帧数。

133项复核的137生成结果也核对其独立SHA，而不只核历史original-review表；输入、
4bfb完整SDK及4477完整SDK的精确字节同一性把既有逐项字段门绑定到新构建。此闭合
没有再启动TShark，不把137/169的参考执行改写成164的新执行，也不是全部协议正确性。

最终报告提交对应的47 checks、2 statuses、完整traffic artifact、实际checkout与
v191 full的131084项required验收记录在PR #5381；不能用4477或其他旧SHA成功代替。

## 133 条既有字段差异的复核

在干净4bfb构建132上，两个完整捕获分别产生42223和263109个 SDK 事件；三种实际
SDK/flow/CLI回放均exit0，没有以字段查询代替实际导出。重新逐条核原94+39稳定问题，
`M2-field-revalidation137-01/summary137.json` SHA
`f45cd00c2e36a79613d02af2268e70adb59982978d8bc24b9249bd6400a37ff8`。

| 分类 | Weber | Sharon |
|---|---:|---:|
| 精确 TLS record 字节相同、贡献包/完成帧归属差别 | 87 | 35 |
| readable DNS 名称显示修复后通过 | 4 | 4 |
| 非法UTF8原字节无损保留，Wireshark替换文字不同 | 2 | 0 |
| RDPUDP原参考能力限制，另有独立derived字节证明 | 1 | 0 |
| 未复核 | 0 | 0 |

4 个新 TShark 调用提供完整原捕获上的 physical/DNS 选定字段；完整TLS oracle明确
按固定hash复用，不冒称本轮重新生成。512MiB参考首次OOM、1GiB/60s首次超时均保留；
137按1GiB和600s截止顺序重跑，4调用全部0退出。选定值、record raw hash、贡献来源、
域、方向、端点、DNS类型与关联逐项核对，不能把133项重分类当全部协议/会话0差异。

捕获ZIP、manifest、规则归档与消费者未修改。大捕获、真实抓包、raw PDU、keylog、
证书/私钥、完整导出、fuzz corpus和截图继续留仓库外，未打进ZIP或提交Git。

## 仍开放

- 永久缺失请求方法的HTTP正文边界；初始中途流不借未来方法。此次有限恢复不覆盖
  所有截断/缺头/升级与重组边界，不能将连接粘性冒充已验证内容。
- TLS KeyUpdate跨record/同record合并等更高级边界、新连接/接口/VLAN全组合尚未新验收。
  原TLS1.2 C1严格失败的8项策略差别保留；已审计的旧Flow2历史未串入新Flow3，不抹掉失败。
- QUIC缺握手app-only两个域组合各2664严格差异保留；准确UDP预览不等于完整QUIC。
  CID、epoch、STREAM、QPACK、0RTT仍开放。
- 所有协议关键字段、会话和fuzz差异的总清单仍在推进；133历史项的选定复核不表示
  TOP30或核心协议已经完备。SMB所有未关闭项继续搁置，不作为当前前置条件。
