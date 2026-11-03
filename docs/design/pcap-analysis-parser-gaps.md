# PCAP 真实测试中的协议解析缺口

> 状态：协议缺口证据与验收设计
> 来源：[PCAP 分析 Agent 的 TDD 开发工作流](./pcap-analysis-agent-tdd.md)
> 当前优先级和执行状态：[PCAP 协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md)
> 上游工作流边界：[PCAP 分析 Agent 单一优化 TODO](./pcap-analysis-todo.md)
> 用途：集中记录真实 agent 测试中发现的协议解析证据、技术边界和回归设计。

这里记录的是“PCAP、IP、TCP 和流重组已经成功，但应用层协议无法识别或无法形成结构化消息”的情况。普通的工具易用性、Skill 指导、模型回答质量和密钥解密能力不放进本文；需要协议层修复的，在本文保留证据和验收边界，并由独立协议解析分支实施。

本文只描述测试时的实现和观察。当前执行状态以 `../work3` 工作树的 [协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md) 为准，本文不另建一套活动状态。

## 1. 状态口径

- `unrecognized`：应用协议没有匹配现有规则，或初始探测窗口不足。原始流仍可用于后续取证，但还没有协议字段。
- `context-required`：协议族已识别，但缺少必要阶段或协商上下文。
- `incomplete`：协议已识别，但没有观察到完整消息边界。
- `malformed`：协议已识别，但当前解析器认为报文字节非法。

不能把 `unrecognized` 直接当成恶意协议、未知协议或解析器崩溃。它只说明当前应用层能力没有完成分类和消息化。

## 2. 缺口总表

| ID | 协议/层 | 测试来源 | 当前观察 | 缺少的核心能力 | 后续建议 |
| --- | --- | --- | --- | --- | --- |
| PG-001 | WCF over `net.tcp` | `redline-01`，Session 119 | `530/531` 为 `unrecognized`；TCP 双向流 `237/238` 可读取，但从原始字节才能得到 WCF 契约 | MC-NMF 消息分帧、WCF `net.tcp` 识别，以及按需的 .NET Binary XML/SOAP 字段解析 | 先做通用 framing 和协议标识，不先写业务方法名映射 |
| PG-002 | DCE/RPC authenticated PDU | `smb-01`，445/135/49688 | 原记录中 record 26 报 `invalid bind result 3`，record 27 报 `48 unconsumed message bytes`；该解析缺口现已实现，`smb-03` 已完成真实 agent 回归 | NTLMSSP auth verifier、BindAck 多 context 结果、带认证 trailer 的 Request 通用头解析 | 实现证据保留在本节；真实 agent 回归已通过 |
| PG-003a | NBNS UDP/137 分发 | `host-user-windows-01`，UDP/137 | 20 条消息被标成 `dns`，没有生成 `nbns` 协议记录 | 将 UDP/137 的名称服务流量分发到 NBNS 规则 | 先修识别，不扩展到身份关联 |
| PG-003b | NBNS 名称解码 | `host-user-windows-01`，UDP/137 | 摘要保留 first-level 编码名，没有还原 NetBIOS 主机名和 suffix | 解码 32 字节名称并保留原始编码、还原名称和 suffix | 不把未解码字符串直接当作主机名 |
| PG-004 | CLDAP / UDP LDAP | `host-user-windows-01`，UDP/389 | 34 条 UDP/389 为 `unrecognized`；TCP LDAP 有 47 条 decoded 和 13 条 SASL `context-required` | 将现有 CLDAP 规则接到 UDP/389 分发，并保留响应操作边界 | 不把 TCP LDAP 的结果冒充 UDP CLDAP |

本轮记录范围只限底层捕获与协议分发事实：链路层到 IP、紧邻的 UDP 承载分发、NetBIOS 名称的确定性解码。用户身份归并、实体置信度、C2 判断和最终报告不纳入这三项。

状态更新（2026-11-03）：PG-003a/PG-003b 已由 main `dba37562a` 关闭。UDP/137 现在进入原生 `nbns` datagram profile；原样本回放得到 20 条 `nbns`，详情还原 `DESKTOP-HSJD5R8`，同时保留 first-level 编码后的 `Name Bytes` 和 `Suffix=0`。PG-004 仍未解决：bin-parser 已有 CLDAP 规则，但 pcaputil 的 UDP 原生分发表尚未注册 UDP/389。

## 3. PG-001：WCF over `net.tcp`

测试证据：

- 运行目录：`/tmp/pcap-agent-lab.4eSy4K/redline-01`。
- 捕获：`testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap`。
- Session 119 为 `10.7.10.47:49744` 到 `194.26.135.119:12432`。
- 流 `237` 为客户端到服务端，流 `238` 为反方向；两向字节量可由 `streams` 查询。
- 协议记录 `530/531` 显示应用协议未识别，摘要为 `TCP application protocol not recognized`，诊断窗口为 64 字节。
- Agent 从 stream 原始字节中看到 `net.tcp://...`、WCF 契约和授权字段，才形成 WCF 判断。
- 当前协议计划中 WCF 仍是 `todo`，见 [PROTOCOL_TODO.md](../../common/bin-parser/PROTOCOL_TODO.md) 和 [protocol_roadmap.go](../../common/bin-parser/protocol_roadmap.go)。

这里缺少的不是 TCP 或 SOAP 的普通 HTTP 外层，而是 WCF `net.tcp` 自己的分帧和编码：

1. 从双向 TCP 流中识别 MC-NMF/WCF `net.tcp` 会话。
2. 在可能从会话中途开始的捕获中，找到有界消息边界。
3. 提取消息类型、序号/调用关联和可读的契约标识。
4. 必要时再解析 .NET Binary XML/SOAP；原始字节始终保留。

统一开发边界：

- 不要求捕获包含完整握手；中途捕获应能明确标记为 midstream。
- 不把 `net.tcp` 字符串当作唯一判据；需要 framing 和结构证据。
- 不反序列化或执行样本中的对象。
- 不把认证字段、凭据枚举列表和完整载荷回显到最终报告。
- 不先实现具体 WCF 方法名、业务对象或服务端行为推断。

回归验收：

- `protocols` 至少能把该会话识别为 WCF，而不是空协议或 `unrecognized`。
- 能得到有界消息边界、方向、流 ID 和调用关联。
- 原始流仍可按 offset 分页读取；解析失败时保留明确状态。
- 同一自然语言任务中，agent 不需要自行解码 base64 chunks 才能发现 WCF。

## 4. PG-002：DCE/RPC authenticated PDU

测试证据：

- 运行目录：`/tmp/pcap-agent-lab.4eSy4K/smb-01`。
- 捕获：`testdata/enterprise-traffic-analysis/lateral-movement-psexec/capture.pcapng`。
- 135 端口为 Endpoint Mapper，49688 为 svcctl 动态端口。
- 持久化库只有 record 25、26、27 三条 DCE/RPC 消息；record 26 为 `invalid bind result 3`，record 27 为 `48 unconsumed message bytes`。
- record 27 的 48 字节正好对应 8 字节 auth verifier 头和 40 字节认证值。
- Agent 只能通过 shell 解析 PDU 头，才看到 49688 上 9 个 request 和 9 个 response。

后续验证：

- `smb-03` 使用最新结构化能力完成同一自然语言任务，516.317 秒正常结束，20 次调用、0 次 shell。
- agent 直接查询到 svcctl Bind、ROpenSCManagerW、RCreateServiceWOW64W、ROpenServiceW、RStartServiceW、RQueryServiceStatus 及其响应，不再需要 shell 手工解析 PDU/NTLM 头。
- 认证后的 stub 仍保持 opaque；报告把服务业务返回码、命令明文和退出码标为未知，没有从加密载荷猜测成功结果。
- 独立复盘确认该回归通过，详细记录见 [TDD 的 SMB 最终复测](./pcap-analysis-agent-tdd.md#84-smbdce-rpc-本轮结论)。

缺口不是 SMB2 重启识别，而是认证 DCE/RPC 的通用解析：

1. PDU 通用头：版本、包类型、flags、frag length、auth length、call ID。
2. Bind/BindAck：每个 context 的 abstract/transfer syntax、result/reason。
3. 带认证 trailer 的 Request/Response：保持 stub opaque，同时解析 Context ID、OpNum 和调用关联。
4. 部分 context 成功、部分 context 被拒时，生成可用的部分结果，而不是整条消息 `malformed`。

统一开发边界：

- 不解析或破解 NTLMSSP 认证值。
- 不把 opaque stub 中的字节猜测成 svcctl 业务参数。
- 不把“有 Response”写成“服务创建成功”或“命令执行成功”。
- 解析结果应明确标记 `authenticated=true`、`stub_opaque=true` 等边界。

回归验收：

- 49688 的认证 Request/Response 能进入结构化索引。
- BindAck 保留各 context 的成功和拒绝原因。
- auth verifier 被正确纳入消息长度，不再产生 `unconsumed message bytes`。
- 即使 stub 不可读，也能查询 PType、Call ID、Context ID、OpNum 和方向。

## 5. PG-003：NBNS 分发与名称解码

测试证据：

- 运行目录：`/tmp/pcap-agent-lab.4eSy4K/host-user-windows-01`。
- 捕获：`testdata/enterprise-traffic-analysis/host-user-identification/Wireshark-tutorial-identifying-hosts-and-users-3-of-5.pcap`。
- UDP/137 共 20 条消息，协议字段都是 `dns`，状态是 `decoded`。
- 摘要形如 `DNS query ... EEEFFDELFEEPFACNEIFDEKEEDFFCDIAA Unknown`，名称仍是 NBNS first-level 编码，没有还原成可读主机名和 NetBIOS suffix。
- 仓库里已有 `application-layer/nbns.yaml`，UDP 规则也声明了 137 到 `NBNS` 的分发；本次持久化结果没有走到该结果。

### 5.1 PG-003a：UDP/137 被标成 DNS

问题只在协议归属：UDP/137 的 NetBIOS Name Service 流量进入了 DNS 解析路径，当前结果没有 `nbns` 协议记录。修复时先处理端口分发和协议名，不要求同时完成主机身份关联。

### 5.2 PG-003b：名称没有还原

即使进入正确协议，返回的名称仍是 first-level 编码字符串，缺少可读名称和 suffix。修复时应同时保留原始编码、还原名称和 suffix，避免把编码值当作主机名。

统一开发边界：

- 不把编码后的 32 字符直接展示为主机名。
- 解码结果保留原始编码、还原名称、suffix 和消息方向。
- 不因为名称出现就认定资产身份；NBNS 仍可能被伪造或缓存污染。

回归验收：

- UDP/137 的消息协议是 `nbns`，而不是 `dns`。
- 摘要或字段能看到还原后的名称和 suffix，同时保留原始编码。
- 原样本回放不再需要手写 Python 才能得到 NBNS 主机名。

## 6. PG-004：CLDAP 分发未生效

测试证据：

- 同一运行中，TCP/389 有 47 条 `ldap/decoded` 和 13 条 `ldap/context-required`，后者明确写着 LDAP SASL security-layer negotiation was not observed。
- UDP/389 另有 34 条消息，协议为空，状态是 `unrecognized`。
- 仓库已有 `application-layer/cldap.yaml`，目录状态是 partial，但当前回放没有把这些 UDP/389 数据报分发给它。

统一开发边界：

- 先修复 UDP/389 的协议分发和请求识别，不把响应操作假装已经完成。
- TCP LDAP 与 UDP CLDAP 分开记录；不能用 TCP 的 decoded 结果覆盖 UDP 缺口。
- SASL `context-required` 继续保持为上下文不足，不推断认证成功或用户身份。

回归验收：

- 原样本的 UDP/389 至少形成 `cldap` 消息，而不是空协议 `unrecognized`。
- 搜索请求的 base、scope 和属性可查询；暂不支持的响应有明确状态。
- TCP LDAP 的现有 decoded/context-required 结果不回归。

## 7. 暂不纳入本文


- TLS keylog 和正文解密：属于导入/解密能力，不是当前 WCF 或 DCE/RPC framing 缺口。
- DNS TXT 内部 base64 解码：属于载荷语义，不应为所有 TXT 记录自动解码。
- `stream_data` 的可打印字符串视图和流内搜索：属于通用取证工具能力，不属于特定协议解析。
- Skill 中的字段路径、JSON key 或具体服务方法：不进入协议解析文档，也不要求 Skill 随解析字段同步修改。

## 8. 执行与验收约束

协议缺口的全局优先级、是否开工和当前状态统一见 `../work3` 工作树的 [PCAP 协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md)。本文只保留技术依赖原则：先处理可复用的 framing、分发和 opaque 边界，再增加业务字段；每项都必须补确定性 Go/Yak 回归测试，并用原样本通过 pcapdb 回放。

实现完成后，结构化能力的变化和确定性测试写入本文；真实 agent 是否因此减少绕路，仍以 TDD 运行记录和 TODO 中的 `VERIFY` 任务为准，不能在本文假装已经验证。
