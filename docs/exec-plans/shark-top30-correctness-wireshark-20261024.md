# Yak Shark 常见 Top 30 协议正确性与易用性计划

日期：2026-10-24。目标环境：Windows 终端、企业内网、Web/API 与常见服务排障。

本文累计记录初始审计及后续实施，**最新结果见第 13 节**。前文的“当前”表示该轮审计时的快照；不能将早期能力限制或待办当作最终实现状态，也不能将测试通过等同于所有协议版本完备。正确性优先于吞吐量，资源上限、取消和密钥隔离仍须保留。

## 1. 先回答截图里的 TLS 问题

`TLS` 与 `TCP/TLS` 当前表达了不同事实：前者来自当前包载荷的快速特征判断，后者来自连接前缀识别。纯 ACK 可以属于 TLS 连接，但自身没有 TLS 记录。将两者混放在 Protocol 列，会让用户误以为协议在两种状态之间切换。

更严重的是，两套判定没有使用同一证据门槛。`shark-cli/decode.go:sniffPayload` 只检查少量记录头字节，TCP 和 UDP 都会调用；底层 pcapx 则有传输、分帧、协商和认证状态。快速识别成功不等于完整 TLS 记录成立，更不等于已经解密。

本轮用同一份封存的真实回环捕获，实际运行了官方 TShark 4.6.9 和当前 Yak 构建。结果说明：底层已有部分较深的能力，展示、证据投影及文档没有完整跟上。不能照旧 TODO 把已有会话解析再做一遍。

### 1.1 实测差分

基线：Git HEAD `ce25ff3706085eb66fd5b72127f4eaa44fa5c1cf`，保留本聊天上一轮 Windows UI 修复的未提交改动。

- TShark：`4.6.9 (v4.6.9-0-g2d548b197c75)`，从 Wireshark 官方 PortableApps 包解包，未运行安装器；外层签名验证为 Wireshark Foundation，状态 Valid。
- 比对开启 TCP/TLS 重组、关闭名称解析、使用独立配置目录；分别测试无密钥、正确的测试会话密钥、错误密钥。
- 样本：封存 `first-batch-m1/tls-h2-bidi.pcap`，92 帧，含 TLS 1.2、TLS 1.3、HTTP/2 和 gRPC。样本是原生回环捕获，不是本轮手工拼包。
- 两边统一按记录长度字段计 TLS 记录。单数 `tls.record.content_type` 会漏计无密钥时以 opaque type 展示的 TLS 1.3 密文记录，不能用它单独作为记录数 oracle。

| 观察项 | TShark 4.6.9 | Yak / pcapx | 结论 |
|---|---|---|---|
| 捕获帧数 | 92 | 92 | 一致 |
| TLS 记录数，无密钥 / 有密钥 | 51 / 51 | 51 / 51 | 此样本的记录边界一致 |
| 无密钥下 gRPC 消息 | 0 | 0 | 没有伪造明文 |
| 正确密钥下 gRPC 消息 | 12 | 12 | 数量一致 |
| 12 条 gRPC 载荷 | `0a036d3130/31/32`，各 4 次 | 相同字节与出现次数 | 逐载荷比对一致 |
| 正确密钥下 AEAD 验证 | 能解析内层 | 39 个 TLS 事件为 authenticated-plaintext | 仅说明记录认证，不说明证书信任或 Finished 验证 |
| 错误密钥下 gRPC / 已认证明文 | 0 | 0 / 0 | pcapx 另产生 2 个 AuthenticationFailed 事件 |
| 无密钥时列表 Protocol | TCP 53、TLSv1.2 20、TLSv1.3 19 | TCP 9、TLS 39、TCP/TLS 44 | 连接归属混入包协议列 |
| 有密钥时列表 Protocol | TCP 53、TLSv1.2 6、TLSv1.3 5、HTTP2 16、GRPC/PB 12 | 与无密钥时完全相同 | 内层事件存在，摘要没有充分投影 |
| Yak JSON 的 PDU 附带结果 | — | 无密钥 51，有密钥 84 | 不能据列表标签断言底层没有解密能力 |

逐字节验证覆盖的是上述限定样本，不代表所有 TLS 密码套件、所有 gRPC 语义或所有协议均已正确。

### 1.2 已复现的负例与展示缺口

以下是独立构造的控制样本，证据类别为 synthetic，不能称为真实业务捕获。

| ID | 输入 / 场景 | 当前 Yak | TShark / 源码证据 | 应有处理 |
|---|---|---|---|---|
| F01 | UDP 443 载荷 `16 03 03 00 04 ff ff ff ff` | Protocol=TLS | TShark=UDP | 普通 UDP 不套用 TCP TLS 记录识别；DTLS、QUIC 分别进入自己的语法和状态机 |
| F02 | TCP 443 载荷 `16 03 03 ff ff 01` | Protocol=TLS | TShark=TCP；声明记录长 65535，且捕获不足 | 按支持的版本范围检查长度；区分非法长度和捕获不全，不能宣称完整解码 |
| F03 | 完整明文 Alert `15 03 03 00 02 02 28` | Info 主要是 TCP flags / 序号 | TShark=Fatal / Handshake Failure | Info 显示 Alert 级别、原因和可见性 |
| F04 | 无握手上下文，记录版本 `03 04`，仅 1 字节载荷 | TLS | TShark 也显示 TLSv1.3 Application Data | 这个显示差异不能替代规范裁决；没有协商证据就不宣称“已协商 TLS 1.3” |
| F05 | 正确密钥解出 HTTP/2、gRPC | 列表标签不变 | 内层事件 84 条已在 JSON PDU 中 | 摘要、详情、流查看共用事件证据，保留外层 TLS 与内层协议关系 |
| F06 | TLS Stream 页，启动时提供 `--tls-keylog` | 源码无条件写“no decryption keys are loaded” | `tui_stream.go:streamLines` | 读取实际密钥配置、命中、支持范围及认证结果；TCP 原始流与解密应用流分别展示 |
| F07 | 协议目录、旧 TODO、运行时能力 | 状态不一致 | TLS、PostgreSQL、LDAP、MQTT 5、QUIC/HTTP3 等已有更多运行时与回归代码 | 先盘点 profile 与测试证据，再生成清单；不按协议名称整体宣称 done |

### 1.3 当前 TLS 能力边界

已核查 `bin_parser_tls_session.go`、`protocol_tls_retry.go`、相应显式规则及回归测试：记录分帧、跨记录握手缓存、ClientHello / ServerHello、SNI / ALPN、证书摘要、HRR 和 KeyUpdate 已有不同程度的实现。

初始审计时 TLS 记录解密限定为 AES-128-GCM：TLS 1.3 的 `0x1301`，TLS 1.2 的 `0xc02f`、`0xc02b`。第 13 节已补充并实抓验证 AES-256-GCM、ChaCha20-Poly1305；单条显式 YAML 入口的能力仍不能等同于实时会话全覆盖。

现有 `Authentication Verified` 主要由记录 AEAD 成功设置。它不能直接等价于 Finished transcript 校验、CertificateVerify 校验、主机名校验或证书链信任。新 UI 必须拆开显示这些事实。

TLS 1.3 的协商版本应来自 ServerHello 的 supported_versions；ClientHello 是提议，记录层 legacy version 不是协商结论。ECH 场景必须区分外层可见 SNI、内层未知及 GREASE，不能把外层名称当作真实业务目标。规范核对还应纳入 2026 年的 RFC 9846 与 RFC 9849，保留 RFC 8446 历史样本兼容。

## 2. 本轮回归的真实结果

执行了以下范围的 Windows 回归，而非只读测试代码：

```powershell
go test -json -count=1 -timeout=5m ./internal/trafficfixture/... ./common/bin-parser/... ./common/pcapx/... ./common/yak/cmd/yakcmds/shark-cli
```

- `common/bin-parser`、其 parser / stream_parser 等子包、`common/pcapx/pcaputil`、Shark CLI 等测试通过。
- JSON 中记录 81,382 个 pass 测试事件、62 个 fail 测试事件、25 个 skip 测试事件。计数包含子测试，不能写成“81,382 个独立协议场景”或“30 个协议完成验收”。
- `internal/trafficfixture`：`TestWorkspaceExportPreservesSealedFiles` 失败于 keylog 的 POSIX 权限位断言。Windows 的权限语义需要单独验证 ACL；不能仅把这条断言删除、跳过或据此认定真实访问权限已经安全。
- `common/pcapx/pcapdb`：61 个失败事件，包含子测试。典型根因是 `openIndex` 的 Windows SQLite 文件 URI产生 `invalid uri authority: C:`；部分后续导入等待也因此失败。需要用小范围修复验证因果，不能把所有失败都直接归为一个根因。
- 所以本轮全套命令退出码为 1，整体门槛未通过。两个失败包单独列为跨平台前置问题，不改写为绿色结果。

TLS、HRR、KeyUpdate、QUIC Initial、QUIC 重复 KeyUpdate、HTTP3 informational response、MQTT 5、RDP 协商、SIP/STUN、LDAP 等选定测试的 pass 已从 JSON 核实。这些证据只覆盖对应测试声明的范围。

## 3. 产品与解析共用的正确性合同

### 3.1 包、记录、消息、连接分别表达

建议新增统一投影层，输入现有 ProtocolEvent 与抓包事实，输出列表、详情、流、过滤、导出的同一语义模型：

| 概念 | 要表达的事实 | 禁止的推断 |
|---|---|---|
| transport | TCP / UDP 等本包传输层 | 443 等于 TLS，UDP 443 等于 QUIC |
| packet protocol | 本包承载或完成重组的 PDU | 连接曾用 TLS，因此每个 ACK 都是 TLS 消息 |
| conversation stack | 已观察到的连接层次、状态和方向 | A/B 首次观察顺序等于客户端/服务端角色 |
| recognition evidence | 端口提示、结构验证、协商、已认证解密分别记录 | 启发式概率分数等于验证成功 |
| completeness | complete / incomplete / context-required / limited 等现有状态的规范投影 | 缓存淘汰、缺包或超限后仍显示完整 |
| visibility | wire / encrypted / authenticated plaintext / decompressed / derived | 解析到了 TLS 外壳就得到了内层业务内容 |

JSON schema 变更应先做兼容性设计：保留已有字段，新增清晰的展示协议、连接协议、证据和版本来源；原 `protocol` / `protocol_scope` 的迁移用版本化合同和固定回归约束，不能悄悄改语义。

默认 Protocol 列建议显示本包事实：纯 ACK 仍为 TCP；已知 TLS 连接归属放进 Connection 或 Info。完成重组的包可显示 TLS / HTTP2 / gRPC，同时能跳到贡献包。保留可选的连接归属显示模式，图例写清依据。

### 3.2 TLS 默认展示

| 场景 | 建议 Protocol / Info |
|---|---|
| ClientHello | TLS / Client Hello · offered TLS 1.3,1.2 · SNI=… · ALPN offers=h2,http/1.1 |
| ServerHello | TLSv1.3 / Server Hello · selected AES_128_GCM_SHA256；证据为协商扩展 |
| TLS 1.3 密文、无密钥 | TLSv1.3（仅已观察协商时）/ Encrypted record · key unavailable；不猜内层类型 |
| 连接中途开始抓包 | TLS / negotiated version unknown · handshake not captured |
| Alert 可见 | TLS / Alert · fatal · handshake_failure |
| 纯 ACK | TCP / ACK · connection TLSv1.3；当前 TLS records=0 |
| 密钥命中、AEAD 成功、内层 HTTP2 | HTTP2 / HEADERS · stream=… · over authenticated TLS；保留 TLS 父记录 |
| gRPC | gRPC / service/method、message length、status（有证据时）；无 descriptor 不猜 protobuf 字段名 |
| 缺密钥 / 不支持套件 / 错密钥 / 缺上下文 | 分别显示 missing-key / unsupported-suite / authentication-failed / context-required |

详情顶部展示可读的版本、记录类型、握手类型、套件名称、SNI、ALPN 和状态；原始数字值仍保留。Connection 页展示握手时间线、双方角色依据、提议与选中值、证书可见性、密钥 epoch 和失败阶段。

### 3.3 字节来源与点击高亮

- 保留 Frame、TCP reassembled PDU、TLS plaintext、解压后实体、派生字段等独立数据源。
- 每个字段记录所属数据源和区间；跨包消息记录多个贡献区间。
- 解密或解压字段的区间只能高亮对应明文或解压数据源，不能假装它存在于原始 frame 的相同 offset。
- 派生的协商状态、耗时和推断角色没有原始字节时明确标注 generated。
- 当前保留窗口以外的证据显示 evicted / unavailable；禁止把历史缺失显示为从未发生。

### 3.4 统一协议分发

抓包数据进入传输分帧、会话推进、字段投影后，再由共用结果驱动展示。收紧 `sniffPayload` 等快速路径的权限：它可以提供候选或提示，不能覆盖结构与状态验证结果。

复用现有有状态解析和 YAML / StructuredPlan 的职责分工，避免新增一套只服务 UI 的完整 dissector。静态字段规则与会话状态分别验收，不能用 native 会话测试代替某个 YAML 入口已经完工的证据。

## 4. Top 30 的排序口径

以下是面向 Windows 桌面与企业网日常排障的工程优先级，综合常见程度、依赖关系和使用价值。它不是依据本次 92 帧或封存语料计算出的全球流量占比排名。

按“协议家族”计 30 项：IPv4/IPv6、DHCPv4/v6、邮件等关联版本合并。HTTP/2 与 QUIC/HTTP3 分开，因为传输、重组与压缩上下文不同。交付仍拆成具体版本和阶段，不能给整个家族一次性盖 done。

| 排名 | 协议 / 家族 | 首个可验收目标 | 高级内容重点 |
|---|---|---|---|
| 01 | TCP | 字段、选项、会话方向、PDU 重组证据一致 | 重传/乱序/缺口、窗口、RTT、重叠策略 |
| 02 | IPv4 / IPv6 | 基础字段、扩展头与分片事实准确 | 重组、重叠、隧道、ICMP 引用包关联 |
| 03 | TLS 1.2 / 1.3 | 关闭误识别，准确版本与握手/密文展示 | 跨记录握手、HRR、认证明文、KeyUpdate、ECH 边界 |
| 04 | DNS / DoT / DoH / DoQ | 查询/应答、具名 RDATA、事务与错误 | EDNS、SVCB/HTTPS、加密载体、重试/回退 |
| 05 | UDP | 长度、校验和与数据报边界 | 非标准端口、隧道域、截断/卸载区分 |
| 06 | Ethernet / VLAN | 地址、类型、标签栈与捕获类型 | QinQ、隧道/接口域、截断与 padding |
| 07 | HTTP/1.x | 请求/响应、正文边界与关联 | chunk/trailer、压缩、CONNECT、升级与歧义 |
| 08 | QUIC / HTTP3 | 正确识别、加密边界、Initial 与连接状态 | 密钥阶段、CID/迁移、QPACK、多路流、0-RTT |
| 09 | HTTP/2 / gRPC | 双向 SETTINGS/HPACK 与事务关联 | 多路流、CONTINUATION、trailer、压缩及描述符边界 |
| 10 | ARP | 请求/应答与地址绑定事实 | Gratuitous ARP、冲突证据、时序与域隔离 |
| 11 | ICMP / ICMPv6 / NDP | 类型、代码、错误引用与邻居发现 | PMTU、RA/NA/NS、校验和与嵌套包 |
| 12 | DHCPv4 / DHCPv6 | 地址配置流程、选项与租约关联 | relay、IA_NA/IA_PD、重传、续租与状态迁移 |
| 13 | mDNS / LLMNR / NBNS | 名称解析家族分别识别与展示 | 缓存刷新、冲突、组播、名称作用域 |
| 14 | WebSocket | 可靠 Upgrade、掩码、分片消息 | 压缩上下文、控制帧交错、关闭握手 |
| 15 | SMB2 / SMB3 | 协商、Session/Tree/File 与操作关联 | compound、credit、异步、签名/加密边界 |
| 16 | Kerberos | 请求/应答、票据外壳、错误与传输 | pre-auth、FAST、referral、时钟与密文边界 |
| 17 | SPNEGO / NTLM | 机制协商、令牌边界与宿主关系 | mechListMIC、MIC/签名是否验证、跨承载关联 |
| 18 | LDAP / LDAPS | BER 操作、message ID 与业务结果 | controls、SASL、StartTLS、分页与异步 |
| 19 | RDP | TPKT/X.224、协议选择与失败阶段 | MCS/GCC/通道、TLS/CredSSP 及解密前提 |
| 20 | SSH | banner、KEXINIT 与加密切换 | 算法协商、NEWKEYS/rekey、包序列和不透明区 |
| 21 | PostgreSQL | 启动、认证、simple/extended query | portal、pipeline、COPY、TLS/GSS、异步通知 |
| 22 | MySQL / MariaDB | 能力协商、认证、命令与结果集 | prepared/binary、压缩、多结果与 TLS 切换 |
| 23 | Redis | RESP2/3 类型、命令/响应与 push 区分 | pub/sub、事务、错误、协议协商与认证状态 |
| 24 | MQTT | 3.x/5.0 版本、会话、topic 与 QoS | Topic Alias、属性、持久会话、重发与 AUTH |
| 25 | SMTP / IMAP / POP3 | 命令、响应、正文和认证状态 | 多行/literal、pipelining、STARTTLS/STLS 与 MIME 来源 |
| 26 | FTP / FTPS | 控制流程、状态码、控制/数据关联 | PASV/EPSV/PORT/EPRT、数据重组、AUTH TLS/PROT |
| 27 | NTP | 时间戳、模式、stratum、请求/应答 | era、KoD、精度、NTS 外壳与认证状态 |
| 28 | SNMP | OID、类型、值、错误与请求/应答 | v3 USM、GETBULK、Trap/Inform、认证/隐私边界 |
| 29 | STUN / TURN / ICE | 属性、事务与 ChannelData 关联 | 完整性、permission/refresh、ICE 状态和复用 |
| 30 | SIP / RTP / RTCP | 呼叫/SDP/媒体关联与解码前提 | fork、重协商、动态 payload、RTCP、SRTP 边界 |

Kafka、MongoDB、AMQP、Syslog、RADIUS、Modbus、OPC UA 等进入后续服务/工控队列，不降低已有回归要求。如果用户真实工作负载以数据库或消息队列为主，基于经授权的代表性捕获重新排序，Kafka/MongoDB 应提前；不能把人工排序称为测量结论。

## 5. Top 30 逐项深度分析与验收卡

每张卡的“已有”来自源码和当前回归，尚未全部进行本轮新的 TShark 差分。下一轮逐协议记录 native-wire、真实捕获、合成控制和独立 oracle 的不同证据等级。

### 01 TCP

- 已有：有界重组、乱序与重传处理、缺口标记、连接代际和 UI 独立缓存；相关重组回归通过。
- 补齐：统一 frame number、原始/相对 seq/ack、flags、MSS/WS/SACK/TS 和有效窗口；SYN 未观察到时不猜窗口缩放。
- 高级：将重传、乱序、duplicate ACK、zero window、RTT 作为附带证据的诊断；重叠冲突采用显式策略并保留冲突字节。
- 验收：SYN/FIN 占序号、回绕、四元组复用、乱序补齐、重复与冲突、丢包、snaplen 截断；TCP 纯 ACK 不新增 TLS PDU。重分段后 PDU 字节与语义保持一致。

### 02 IPv4 / IPv6

- 已有：网络层规则、扩展头与部分隧道/分片路径及测试；需要统一 CLI 使用的具体路径与覆盖范围。
- 补齐：IPv4 IHL/total length/options/flags/offset 与 IPv6 next-header/extension chain/fragment ID 等使用具名值。
- 高级：版本各自的分片规则、重叠与超时、原子分片、隧道内外域、ICMP 引用的内层头部；重组产物有独立 ByteSource。
- 验收：合法/截断扩展头、循环/超限链、乱序分片、重复、缺片、重叠、接口/VLAN 域冲突；普通缺片不得标成解码成功，卸载影响不得直接当成链路坏包。

### 03 TLS

- 已有：本轮已做 92 帧实际差分；限定 AES-128-GCM profile 与 12 条 gRPC 载荷相符，HRR/KeyUpdate 回归通过。
- 补齐：优先收紧传输与记录识别，并将记录/握手/协商状态接到 Info 与详情；同包多记录、跨包记录和跨记录握手分别列出。
- 高级：提议/选中 ALPN、SNI、完整 extension 列表、证书链的可见摘要、HRR/PSK/resumption/early-data 状态、KeyUpdate epoch、记录 AEAD 与 Finished/证书验证拆分；JA3/JA4 仅作为带版本的指纹，不做唯一身份或恶意结论。
- 验收：明文/密文握手、跨记录 Certificate、TLS 1.2 重协商边界、CCS、Alert、缺钥/错钥/缺握手/缺包、不同套件与密钥更新；每个可解密 profile 都有独立端点和错误 tag 样本。AES-256-GCM、ChaCha20-Poly1305 单独交付，不先贴支持标签。
- ECH 只陈述观察到的 outer、可能的扩展和可证明的状态；未解密 Inner 不展示其 SNI。DTLS 有独立数据报/epoch/握手分片规则，不复用 TCP 记录识别。

### 04 DNS 与加密载体

- 已有：UDP/TCP DNS、EDNS、具名 RDATA 及原生 DoH 等代码与测试；实际支持依版本/profile 核实。
- 补齐：QNAME/QTYPE、rcode、各 section、TTL、CNAME 关系；A/AAAA/PTR/MX/TXT/SRV/SOA/CAA/SVCB/HTTPS 清晰投影，保留未知 RDATA。
- 高级：UDP TC 后 TCP 回退、EDNS 扩展 rcode/ECS、DNSSEC 字段与验证状态分开、DoH MIME/HTTP 状态、DoT/DoQ 的认证明文与父流关系。
- 验收：压缩指针循环/越界、TCP 长度跨包、ID 重用与重试、乱序答复、NXDOMAIN/NODATA/timeout 区分；加密外壳或端口 853 本身不能产生 DNS 问题和答案。

### 05 UDP

- 已有：gopacket 基础字段及实时分发；F01 证实上层快速识别会越过传输边界。
- 补齐：长度、捕获长度、端口、校验和状态与实际 payload 长度；应用识别不修改原始 transport。
- 高级：IPv4 zero checksum、IPv6 checksum 规则和卸载背景分别处理；五元组相同而接口/隧道域不同的数据报保持隔离。
- 验收：单数据报多消息、长度过短/超出、空载荷、非标准端口及相似魔数；UDP 不使用 TCP 的流缓冲规则，字节相同但传输不同须得到不同准入结论。

### 06 Ethernet / VLAN

- 已有：Ethernet、VLAN 及其他 link type 规则；Shark 能处理回环等捕获类型。
- 补齐：源/目的 MAC、EtherType、802.3 长度与 EtherType、PCP/DEI/VID 的具名投影；不要把 Npcap loopback 的 4 字节头解释为 Ethernet。
- 高级：QinQ/多标签上限、广播/组播与 padding、接口 ID 和捕获 section 归属；FCS 是否包含/是否可校验分别说明。
- 验收：Ethernet、VLAN、stacked VLAN、loopback、SLL/SLL2、raw IP 的同一内层结果与真实 offset；短帧、截断标签和不支持 link type 明确报出。

### 07 HTTP/1.x

- 已有：完整头部识别、HTTP 会话与正文 framing、Upgrade、压缩等路径已有测试。
- 补齐：method/authority/path/status、请求响应 ID、正文长度、interim response、trailer 与正文数据源；Info 优先显示业务摘要。
- 高级：HEAD/204/304、chunked、gzip/deflate、CONNECT 隧道、101 切换、pipelining；CL/TE 歧义作为明确异常，不能静默采用不同解析器的不同边界。
- 验收：逐边界切包、粘包、提前 EOF、重复/冲突长度、畸形 chunk、压缩错误；未完整头部不能只因前缀 GET 就确认为 HTTP。关联和正文 hash 与独立端点/TShark 核对。

### 08 QUIC / HTTP3

- 已有：QUIC v1 状态、Initial 保护解除、有限密钥解密、流/ACK 与 HTTP3/QPACK 代码；部分测试明确为外部解密算法输入，不能当原生 wire 证据。
- 补齐：long/short header、version、CID、packet number space、加密级别、CRYPTO/STREAM 与 HTTP3 stream ID 的可读摘要。
- 高级：Retry、CID 迁移、PN 重建、key phase、0-RTT、SETTINGS、QPACK 动态表与 blocked 状态；QUIC Initial 的公开可派生密钥不等于整条连接已能解密。
- 验收：真实 QUIC 客户端/服务端捕获、非标准端口、缺 Initial、错 tag、重复 PN、流重叠、跨包 CRYPTO 与 QPACK 缺上下文；普通 UDP 443 和任意明文字节不能直接进入 HTTP3。v2 单独列 profile。

### 09 HTTP/2 / gRPC

- 已有：双向 SETTINGS、持续 HPACK、多流、TLS 解密交付和 gRPC 消息；本轮对照了 12 条 gRPC 原始载荷。
- 补齐：stream ID、帧类型、压缩头的有序列表、`:method/:path/:status`、request/response、trailers 和 grpc-status。
- 高级：CONTINUATION、并发、flow control、RST/GOAWAY、informational response、gRPC streaming/压缩；未知 protobuf descriptor 时保留 message bytes 和类型未知。
- 验收：双向表状态隔离、头块跨帧/跨 TCP 包、动态表更新、伪首部顺序、错误流 ID、丢失表上下文和消息长度；不能把若干成功 DATA 帧当成完整 RPC 成功。

### 10 ARP

- 已有：字段规则及协议事件；截图已能读到请求与地址信息。
- 补齐：Who has / Tell、is at、opcode、sender/target MAC/IP；字节字段优先于重复的 Session/Fields 元数据。
- 高级：Gratuitous ARP、probe、重复地址观察与 MAC 变化时间线；观察到变化只能称冲突/变化证据，不能直接归因 ARP 欺骗。
- 验收：请求/应答、非 Ethernet/IPv4 的地址长度、截断与非法组合；按接口/VLAN 域分开邻居表，独立抓包工具读出相同地址。

### 11 ICMP / ICMPv6 / NDP

- 已有：ICMP、IPv6 扩展相关规则和测试。
- 补齐：type/code 的名称、Echo ID/seq、时间/错误引用；NS/NA/RS/RA、Target Link-Layer Address 等选项。
- 高级：Packet Too Big/PMTU、引用内层 transport 与 flow 关联、DAD/路由通告生命周期；单点抓包的丢包比例和 RTT 明确统计前提。
- 验收：常见差错、未知 type/code、截断引用、嵌套层数与校验和；NDP 必要 hop limit/地址规则和选项长度，错误包不能反向污染连接状态。

### 12 DHCPv4 / DHCPv6

- 已有：DHCPv6 原生捕获与独立 oracle，DHCP 基础规则和 option 解析。
- 补齐：事务 ID、DORA / SOLICIT-ADVERTISE-REQUEST-REPLY、地址/前缀/租期、server/client identifier 与 option 名称。
- 高级：relay、嵌套 option、IA_NA/IA_PD、多地址/前缀、重传/续租/rebind；客户端标识与 MAC 不强行等同。
- 验收：option overload、零长/重复/未知 option、嵌套 relay 限制、状态码与失败流程；同一事务 ID 不同接口/客户端不能错配。

### 13 mDNS / LLMNR / NBNS

- 已有：DNS 及相应封装规则，原生 mDNS 等样本覆盖部分路径。
- 补齐：分别显示协议名、query/answer、QU/cache-flush、NBNS 名称编码与类型，不能用同一套单播 DNS 假设覆盖全部协议。
- 高级：DNS-SD PTR/SRV/TXT 关系、组播重试、TTL=0 goodbye、名称冲突证据和链路作用域。
- 验收：同 ID 多组播响应、压缩指针、同名异服务、非 ASCII 名称与畸形编码；地址/名称关联的有效期和来源明确，不能将“查询某名称”直接当作进程或主机身份。

### 14 WebSocket

- 已有：HTTP Upgrade、帧与分片/压缩相关运行时和原生捕获回归。
- 补齐：握手请求/101/Accept 验证、opcode/FIN/RSV/mask、消息边界、文本/二进制和关闭码。
- 高级：permessage-deflate 双向上下文、fragment 中间的 ping/pong/close、超大消息与上下文重置。
- 验收：未协商 RSV、错误掩码、无匹配 Upgrade、UTF-8 无效、控制帧分片、压缩炸弹边界；重组消息与端点记录逐字节一致。

### 15 SMB2 / SMB3

- 已有：SMB2 会话、compound 与部分操作字段及 Samba 等独立捕获证据；SMB3 transform 不代表已解密。
- 补齐：dialect、command、status、MessageId、SessionId/TreeId/FileId、路径和读写长度；请求响应、compound 子命令分别关联。
- 高级：credit、异步 STATUS_PENDING、签名验证状态、SMB3 加密/压缩边界；协议外壳、认证完成和内容恢复各自验收。
- 验收：UTF-16/offset、compound 对齐/循环、长度、重复 MessageId、签名缺钥与加密 transform；不把 SMB1 回退或未验证 NTLM 当作 SMB3 认证成功。

### 16 Kerberos

- 已有：显式字段入口、TCP/UDP 实时路径及独立 Kerberos 捕获回归。
- 补齐：AS/TGS/AP、realm、principal、etype、kvno、错误码、票据外壳及 pre-auth 类型；密文字段保留不可见状态。
- 高级：pre-auth 交互、referral、FAST 外壳、时间误差与会话关联；解密需要对应类型密钥和阶段上下文，不能只凭有 keytab 就宣称全面解密。
- 验收：BER 长度、APPLICATION tag、TCP prefix、分段与双向配对、错误请求和密文改变；避免将 AS-REP 或票据出现等同于登录成功。

### 17 SPNEGO / NTLM

- 已有：机制/令牌解析、SMB2 Session Setup 包装、NTLM 显式字段与回归。
- 补齐：mech OID、NegTokenInit/Resp 状态、NTLM message type/flags、TargetInfo、域/用户名的字段来源与可见性。
- 高级：MIC/mechListMIC、签名/封装是否验证、通道绑定与宿主 SMB/HTTP/LDAP 关系；凭据和响应材料默认按已有脱敏政策处理。
- 验收：嵌套 ASN.1、offset/length 越界、重复/未知 OID、挑战与响应跨流误关联；能解出 Authenticate 字段不等于已验证口令或认证成功。

### 18 LDAP / LDAPS

- 已有：已存在 Bind/Search/Modify 等会话与显式字段、SASL/StartTLS 代码；旧 TODO 尚未同步。
- 补齐：operation、message ID、DN/filter/attributes、result code、SearchEntry/Done/Reference 与请求关联；默认沿用敏感属性脱敏。
- 高级：paged-results/controls、abandon、unsolicited、SASL protected layer、StartTLS 明确切换点，解密明文回到原 LDAP 状态。
- 验收：长 BER、嵌套 filter、控制项、重复 ID、并发 Search、LDAPv2/v3 profile、TLS 前后字节；权限不足等业务失败不能标成解析失败，保护层未解密不能继续读 BER。

### 19 RDP

- 已有：TPKT/X.224、cookie、协商选择/失败、明文 MCS/GCC 与通道；源码明确图形/音频/重定向不在现有限定范围。
- 补齐：requested / selected protocol、negotiation failure、TLS/CredSSP 边界、可见通道信息及阶段时间线。
- 高级：在授权解密及上下文齐全时接入 CredSSP、静态/动态虚拟通道；图形/设备重定向另建 profile 与端点 oracle，不能用协商成功冒充完整桌面解码。
- 验收：与 S7/COTP 等相似 TPKT 协议的消歧、阶段跳转、失败、截断和加密后边界；不能只因端口 3389 或 cookie 就宣称会话成功。

### 20 SSH

- 已有：banner、二进制包、协商/NEWKEYS 等会话与语义审计回归。
- 补齐：协议版本/software 字符串、KEXINIT 双方算法列表、选中算法、host-key 指纹的可见性、加密切换阶段。
- 高级：rekey、压缩协商和方向独立状态；只有经过明确密钥及算法 profile 验证后才投递认证/通道层明文。
- 验收：banner 前说明行、跨包行尾、binary length/padding、算法列表与首选匹配、NEWKEYS 后密文、断续抓包；不从密文中猜命令、SFTP 文件或登录结果。

### 21 PostgreSQL

- 已有：Startup、simple/extended、portal/pipeline、COPY、异步与 TLS handoff 等代码和测试；不是从零建设。
- 补齐：角色依据、认证类型与结果、query/statement/portal、parameter、RowDescription/DataRow、ErrorResponse/SQLSTATE、事务状态与取消关联。
- 高级：prepared/binary 格式、pipeline sync/error recovery、COPY 双向、通知、SSL/GSS 和握手可见性；server release 与 wire profile 分别记录。
- 验收：长度前缀、前后端同码不同义、无 Startup 的 context-required、交错 query/notice、TLS 切换、NULL/空值/二进制列；结果与客户端输出核对，不能只确认 Query 文本。

### 22 MySQL / MariaDB

- 已有：经典会话及部分 prepared/compression/TLS 等后续实现已超过旧文档；先逐 profile 核实实际分支。
- 补齐：server capabilities、client capability intersection、sequence ID、auth plugin/结果、COM 命令、字段类型、NULL 与结果行、EOF/OK/multi-results。
- 高级：prepared/binary、压缩 envelope、query attributes、LOCAL INFILE、TLS 后状态继承；MariaDB 差异独立声明，复制/新协议不混入经典连接验收。
- 验收：多包大消息、sequence 重置、EOF/OK 歧义、二进制值、压缩错误、auth switch/失败、多结果中途 ERR；SQL 查询发出不代表执行成功。

### 23 Redis

- 已有：RESP2/3、类型、push、认证和响应关联等测试，以及 M2 原生会话样本。
- 补齐：有序 typed value、NULL/空 bulk/错误、command 与参数边界、请求响应和 unsolicited push 独立计数。
- 高级：HELLO 协商、pub/sub、unsubscribe-all、MULTI/EXEC、pipeline、错误后的认证状态；事务内 queued 与真正执行结果分开。
- 验收：递归/长度/计数预算、binary bulk、流式类型 profile、push 插入、鉴权失败与断续抓包；与 redis-cli/服务端记录及 TShark 支持范围分别核对。

### 24 MQTT

- 已有：CONNECT 驱动的 3.x 与 MQTT 5 状态/属性/QoS 测试；不能根据旧 README 认定 MQTT 5 完全缺失。
- 补齐：版本来源、client ID、session present/expiry、topic、packet ID、reason code、properties 和 QoS 交互。
- 高级：Topic Alias、Will、shared subscription、persistent session、receive maximum、AUTH、QoS2 重发去重与 DUP 标记。
- 验收：无 CONNECT 不猜版本；remaining-length 边界、非法重复属性、方向约束、topic alias 超界、ID 复用、断线重连；QoS1 重发可展示多个帧，但一次消息交付的语义必须说明。

### 25 SMTP / IMAP / POP3

- 已有：三种协议的命令/响应、literal/多行及 STARTTLS/STLS 等回归；分别维护版本和阶段。
- 补齐：SMTP envelope/DATA、IMAP tag/untagged/literal、POP3 状态/多行/dot-stuffing、业务状态与认证结果；协议相关敏感内容沿用脱敏策略。
- 高级：pipelining、IMAP literals/IDLE、MIME/编码和压缩的独立来源；TLS 升级后回到相应协议需要认证明文与原上下文。
- 验收：多行终止、literal 长度、跨包 dot terminator、并发 tag、STARTTLS 失败/成功、auth 失败；“服务器接受投递”与“邮件送达收件人”不能混为一谈。

### 26 FTP / FTPS

- 已有：控制会话、被动/主动模式、AUTH TLS 边界和原生实验室会话回归。
- 补齐：命令/参数/响应码、控制连接与对应数据连接、传输方向、文件/目录操作及失败阶段。
- 高级：PASV/EPSV/PORT/EPRT、NAT 地址不一致、并发传输、数据重组/完整性、PROT C/P；加密控制流不能自动提供数据流明文。
- 验收：相似 220 banner 负例、多行响应、不同 data tuple、传输中断、REST 与 TLS 升级；只有具备足够数据连接证据才恢复文件并给出 hash/完整性。

### 27 NTP

- 已有：NTP v3/v4 有限识别和 profile 边界回归。
- 补齐：LI/version/mode、stratum、poll/precision、reference/originate/receive/transmit timestamps 和 KoD 名称。
- 高级：era 与时间转换、请求响应来源一致性、offset/delay 的计算前提、扩展字段与 NTS 可见外壳/验证状态。
- 验收：错 mode/version、短包/扩展长度、错误 originate 关联、epoch/era 边界、广播模式和异步抓包；不能用抓包机器时间直接替代 NTP 四时间戳计算，也不将 stratum 当成可信度证明。

### 28 SNMP

- 已有：v1/v2c/v3、Trap/Inform、USM 相关状态及三类独立捕获/oracle 回归。
- 补齐：OID、ASN.1 typed value、request ID、error status/index、GET/SET/GETNEXT/GETBULK/Trap/Inform、varbind 列表。
- 高级：v3 engine/time/window、auth/priv 参数、discovery、有限 MIB 名称映射；数值 OID 和未经验证的名称映射同时可见。
- 验收：BER 长度、错误 varbind、response matching、GETBULK 行列、未知 OID、错密钥/未知算法；社区字符串和认证材料默认脱敏，不能将解出 USM 头称为认证或解密成功。

### 29 STUN / TURN / ICE

- 已有：STUN/TURN 会话、ChannelData 与独立捕获；T20 验证了部分方法/准入边界。
- 补齐：method/class/transaction ID、XOR-MAPPED/RELAYED 地址、错误码、请求响应与通道数据来源。
- 高级：MESSAGE-INTEGRITY/FINGERPRINT 的分别验证、Allocate/permission/channel/refresh 生命周期、ICE candidate/nomination 与角色状态。
- 验收：TLV 对齐、IPv6 XOR、未知方法、事务域隔离、过期 permission/channel、错误完整性；不将所有 STUN 都标作 TURN，抓到候选地址不等于 NAT/ICE 已成功建立媒体路径。

### 30 SIP / RTP / RTCP

- 已有：SIP、SDP、RTP/RTCP、RTSP 承载与原生双向捕获/oracle，T20 及其边界回归通过。
- 补齐：Call-ID/CSeq/method/status、branch/tag、dialog 与 offer/answer；媒体按协商 tuple、方向、SSRC、payload type 与采样时钟关联。
- 高级：fork、re-INVITE、offerless INVITE、RTCP mux、序号回绕、jitter、SR/RR；SRTP 明文与认证需独立密钥/profile，媒体字节出现不表示能还原音视频。
- 验收：SIP TCP Content-Length/UDP 边界、错配 SDP、动态 payload、SSRC 切换、RTCP/RTP 碰撞、丢包与重排；jitter/loss 报告注明单点捕获和时钟依据，不能把 200 OK 等同于端到端通话成功。

## 6. 正确性验收体系

### 6.1 验证单元

单元采用“协议 + 版本/profile + 阶段 + 可见性 + 承载 + 字节来源”，例如 `TLS 1.3 / AES128-GCM / application epoch / authenticated plaintext / TCP`。禁止通过一个 ClientHello、一个端口或一个 parser score 宣称整个协议等同 Wireshark。

沿用封存 ZIP、内容哈希、来源/许可和 expected contract。新样本、负例和字段合同增加新批次；不改写旧黄金、不从 Yak 当前输出生成自己的正确答案。

### 6.2 每个 profile 的硬门槛

| 门槛 | 必须满足的证据 |
|---|---|
| G1 结构 | 具名字段、类型、长度、枚举、未知值保留；正常可见的字段不得用整块 raw 代替 |
| G2 准入 | 标准/非标准端口、错误传输和相似魔数负例；端口提示不提升为结构已验证 |
| G3 消息边界 | 完整、分段、粘包、乱序、重复、缺口、截断；PDU/事务数量和字节匹配 |
| G4 状态 | 双方向、版本/能力、角色依据、切换、恢复/失败、ID 复用与生命周期 |
| G5 独立语义 | RFC/厂商规范 + 独立端点记录或另一实现；TShark 是参照，不是唯一真值 |
| G6 展示/过滤/导出 | CLI/TUI/JSON/离线结果同源；PDU 贡献帧、来源区间和未验证状态正确 |
| G7 加密/压缩 | 缺钥、错钥、不支持算法、错误 tag/压缩体；认证失败不交付明文，生成数据不伪造 frame offset |
| G8 边界/资源 | 空/短包、非法长度、递归/集合/内存上限、取消、到期与释放；limited/evicted 与 decoded 明确分开 |
| G9 平台 | Windows/Npcap/ConPTY 与至少一套 Unix CI；路径、ACL、pcapng section/interface 和编码正确 |
| G10 兼容 | 既有规则字段/ABI、封存答案不回退；声明的分支表均有测试，skip 不算验收通过 |

正确性门槛失败时，本 profile 不准入交付；吞吐量提升不能抵扣错误字段、误识别、丢失消息或错误关联。

### 6.3 差分方法

1. 固定 TShark 版本、配置、插件、Decode As、重组、名称解析、TLS 密钥设置；保存命令、二进制/pcap 哈希。
2. 比较规范字段与语义，避免直接比较 GUI 文案或每帧 Protocol 列；帧与 PDU 是多对多。
3. 以 capture domain、方向、stream generation、PDU/事务、父子来源对齐。TLS 外层记录与解密内层消息分别计数。
4. 类型和数值无损归一化；保留重复项、顺序和字节。无序集合仅在规范允许时排序。
5. 先核对整份捕获，再核对字段与消息载荷；单帧 selected-input 断言不能代替 whole-capture oracle。
6. TShark 不支持、两版本结果不同、规范与实现冲突时，产生 unresolved discrepancy；附原始字节和规范条款，独立端点/实现复核后才更新期待值。
7. TLS 的记录数使用完整 record envelope / length 等证据；不能遗漏 opaque records，也不能把同一记录的 outer / inner type 重复计数。

### 6.4 变形与生成测试

- 对小型完整 PDU 遍历所有 TCP 切点；大消息覆盖长度头、变长字段、压缩块、认证 tag、帧尾等边界和固定 seed 的随机分段。
- 合法重分段、TCP 重传、无关流交错、workers 1/2/4、完整/延迟字段交付，规范化语义应一致。
- 删除包/截断/篡改单字节不要求仍成功；应产生可解释的 incomplete/malformed/authentication-failed 等状态，不得保持原成功正文。
- 不把有界缓存丢弃或 UI skipped 与内核丢包混成一个指标，也不以输出行数稳定掩盖消息丢失。
- Fuzz seed 包含真实缺陷最小化样本。发现错误先固化样本和期望，再修复；不以增加超时或内存掩盖语义问题。

## 7. 实施顺序和可审查交付

| 阶段 | 工作包 | 依赖与退出条件 |
|---|---|---|
| A0 证据与平台基线 | 能力/profile 清单；冻结本轮 TLS 控制样本；单独修 Windows PCAPDB URI 和 keylog 导出权限验证 | 保留失败历史；数据库流程相应门槛转绿；不阻止可独立推进的 TLS 展示修复 |
| A1 识别与语义模型 | 统一当前包/连接证据、传输准入、typed status、来源与 JSON 兼容合同；修 F01/F02 | UDP 负例不再标 TLS，非法长度处理明确，pure ACK 不声称包含 TLS PDU |
| A2 TLS 可用闭环 | 握手/记录摘要、版本/套件、SNI/ALPN、密钥状态、TLS/HTTP2/gRPC 父子链；修 F03/F05/F06 | 本轮 92 帧 oracle、无钥/正确钥/错钥及新的多记录/跨记录捕获，全链路展示与导出验收 |
| A3 字节来源与网络基础 | Frame/重组/明文/解压区、点击定位、跨包贡献关系；TCP、IP、UDP、Ethernet/VLAN、ARP、ICMP、DHCP | 三种以上窗口尺寸、live/offline、贡献包/字段/来源互跳；基础协议相似负例和重组门槛通过 |
| B Web 与名称解析 | DNS/加密 DNS、HTTP1、HTTP2/gRPC、QUIC/HTTP3、WebSocket、名称发现 | 双端真实会话、压缩/表状态/升级/密钥/负例；不可见内容与未支持版本明确 |
| C Windows 企业网 | SMB、Kerberos、SPNEGO/NTLM、LDAP、RDP、SSH | 协商/认证/业务结果分开；真正的双向与失败会话，不仅是握手字段 |
| D 数据和服务会话 | PostgreSQL、MySQL/MariaDB、Redis、MQTT、邮件、FTP | 命令/响应/事务/业务状态；高级 profile 逐个门禁，不打包宣称整个家族完成 |
| E 运维和实时通信 | NTP、SNMP、STUN/TURN/ICE、SIP/RTP/RTCP | 时间/会话/媒体关联证据、认证边界和真实双向捕获；诊断结论带前提 |
| F 发布验收 | Windows + Unix、封存 corpus、外部 oracle、使用场景、兼容与文档生成 | Top 30 每个承诺 profile 的 G1–G10 通过；遗留项明确，不以目录 done 数量代替验收 |

每个工作包拆为小范围 PR 或等价可审查改动：问题与复现 → 规范与字段合同 → 单一 profile 实现 → 新旧回归 → 源码/CLI/文档一致性。一个 profile 未闭环时，不并行铺开几十个“有名字、无语义”的入口。

性能优化排在相应工作包的正确性基线之后，使用相同完整输出、相同语料和相同资源设置比较。保留 worker 扩展能力，但不为了吞吐量省掉字段、校验、父子关系或错误诊断。

## 8. 首批具体待办

1. 把 F01/F02/F03/F04 制成新的有来源标记的控制批次，建立前端识别与 pcapx 准入一致性回归；F04 用来防止将 Wireshark 启发式标签当成协商证明。
2. 增加 `packet protocol`、`conversation protocol stack`、`negotiated version source`、`visibility/authentication` 的兼容字段，按单一结果投影列表与详情。
3. 对 F03 的 Alert 和 ClientHello/ServerHello 实现可读 Info；记录/握手/应用消息数量分别展示。
4. 从实际 provider、查钥、算法和 AEAD 结果生成密钥状态，去掉 TLS Stream 页无条件“no keys”文案。
5. 将现有 HTTP2/gRPC 事件接入消息/流视图，提供原始流、认证明文与消息视图选择；未知 protobuf schema 保留原始字节。
6. 修 Windows SQLite URI 的盘符/空格/Unicode/UNC/特殊字符合同；用 ACL 验证代替仅依赖 Unix mode 位的密钥导出测试。
7. 为上述每项提供固定独立期待值与机器可读差异报告；这批验收后再扩展 TLS 套件、更多协议 profile。

## 9. 核查入口、证据与复现

### 9.1 仓库入口

- `common/yak/cmd/yakcmds/shark-cli/decode.go`：包摘要、`sniffPayload`、详情与快速字段。
- `common/yak/cmd/yakcmds/shark-cli/protocol.go`：前缀识别、`applyApplication`、连接归属标签。
- `common/yak/cmd/yakcmds/shark-cli/command.go`：密钥参数与 JSON PDU 投影。
- `common/yak/cmd/yakcmds/shark-cli/tui_stream.go`：流来源、明文/密文提示。
- `common/pcapx/pcaputil/bin_parser_tls_session.go`、`protocol_tls_retry.go`：TLS 状态和授权密钥范围。
- `common/pcapx/pcaputil/bin_parser.go`：ProtocolEvent 与字段投影。
- `common/pcapx/pcaputil/first_batch_m1_tls_test.go`、`protocol_reliability_test.go`：TLS、KeyUpdate、HRR 的既有验收。
- `common/pcapx/pcapdb/pcap_database.go`、`internal/trafficfixture/workspace_test.go`：本轮平台失败入口。
- `common/bin-parser/PROTOCOL_DELIVERY.md` 与 `testdata/corpus-batches/README.md`：继续遵守规则交付与封存样本合同；同时澄清静态规则评分和运行时 profile 能力的区别。

### 9.2 本机证据位置

过程日志、工具、解包内容、捕获与临时密钥保存在仓库外：`C:\tmp\yak-shark-protocol-audit-20261024`。本计划不把外部日志或临时 EXE 纳入版本库。

| 文件 | 内容 |
|---|---|
| `comparison-summary.json` | 92 帧无钥/有钥的 TShark–Yak 差分、载荷与控制包标签 |
| `tls-tshark-no-key.tsv` / `tls-tshark-key.tsv` | 固定重组设置、独立配置目录的原始 TShark 字段输出 |
| `tls-yak-no-key.jsonl` / `tls-yak-key.jsonl` | 当前 CLI 的原始 JSON |
| `tls-pipeline-no-key.jsonl` / `tls-pipeline-key.jsonl` | 底层事件与 session 证据 |
| `wrong-key-summary.json` | 错误测试密钥的零明文与认证失败统计 |
| `tls-boundary-controls.pcap` / `boundary-tshark.tsv` / `boundary-yak-cli.jsonl` | F01–F04 的构造控制证据 |
| `go-test.jsonl` / `test-summary.json` | 全套命令、包结果、失败与 skip 明细 |
| `source-fingerprints.json` / `tshark-version.txt` / `tshark-folders.txt` | 关键源文件、工具与独立配置证据 |
| `audit.go` / `compare.py` | 本机审计辅助工具；不是产品解析器或独立语义真值来源 |

固定 SHA-256：

- 捕获：`aa633bc473e7e650047eadddcb41f3946e014527e216d16fa92634dbae5dc999`。
- 本轮 Yak EXE：`9d40a8182e8be0842a0978b222287bc9f2358e740e3ef2a9a69919f71539d66d`。
- 官方 PortableApps 包：`71bcfda9eefec8088faaf0b8e6d026369ea83ae6149cd0b76ccf0b3194240db0`。

在无 `&` 的仓库路径/别名中导出封存样本，避免当前 Windows CGO 对源目录字符的已知限制；导出目标必须是新目录：

```powershell
go run ./internal/trafficfixture/cmd/corpus export C:\tmp\new-shark-audit-corpus
```

以下展示 TShark 复现参数；`$tshark`、`$pcap`、`$keylog` 指向显式选定的程序、封存捕获和该测试会话密钥。关闭密钥比对时把 keylog 设为空字符串；输出直接写文件时应保留 UTF-8 原始字节，不让 shell 的文本重编码制造伪差异。

```powershell
& $tshark -2 -n -r $pcap `
  -o tcp.desegment_tcp_streams:TRUE `
  -o tls.desegment_ssl_records:TRUE `
  -o tls.desegment_ssl_application_data:TRUE `
  -o "tls.keylog_file:$keylog" `
  -T fields -E header=y -E occurrence=a `
  -e frame.number -e frame.protocols -e _ws.col.Protocol -e _ws.col.Info `
  -e tls.record.length -e tls.record.content_type -e tls.record.opaque_type `
  -e tls.handshake.type -e tls.handshake.version `
  -e tls.handshake.extensions.supported_version `
  -e tls.handshake.extensions_server_name -e tls.handshake.ciphersuite `
  -e tls.handshake.extensions_alpn_str -e http2.streamid `
  -e grpc.message_length -e grpc.message_data

& $yak shark --pcap-file $pcap --tls-keylog $keylog --json
```

本轮只用了封存样本中的临时测试会话密钥；没有自动搜集用户浏览器或生产会话密钥。后续测试同样显式选择经授权的输入，不把密钥材料写入诊断、仓库或通用导出文件。

### 9.3 一手参照

- [Wireshark 官方下载](https://www.wireshark.org/download.html)、[TShark 手册](https://www.wireshark.org/docs/man-pages/tshark.html)：本轮工具版本和差分操作参照。
- [Wireshark 重组](https://www.wireshark.org/docs/wsug_html_chunked/ChAdvReassemblySection.html)、[TCP 分析](https://www.wireshark.org/docs/wsug_html_chunked/ChAdvTCPAnalysis.html)、[流查看](https://www.wireshark.org/docs/wsug_html_chunked/ChAdvFollowStreamSection.html)、[Expert Information](https://www.wireshark.org/docs/wsug_html_chunked/ChAdvExpert.html)：消息、字节来源、诊断与交互参照。
- [TLS Wiki](https://wiki.wireshark.org/TLS)、[TLS 字段参考](https://www.wireshark.org/docs/dfref/t/tls.html)、[QUIC 字段参考](https://www.wireshark.org/docs/dfref/q/quic.html)：对照字段名和密钥配置，不将 GUI 标签作为唯一真值。
- [RFC 9846：TLS 1.3](https://www.rfc-editor.org/info/rfc9846/)、[RFC 8446 历史规范](https://www.rfc-editor.org/rfc/rfc8446.html)、[RFC 9849：ECH](https://www.rfc-editor.org/rfc/rfc9849.html)、[IANA TLS 参数](https://www.iana.org/assignments/tls-parameters)、[TLS 扩展注册表](https://www.iana.org/assignments/tls-extensiontype-values)。
- [RFC 9293：TCP](https://www.rfc-editor.org/rfc/rfc9293.html)、[RFC 9001：QUIC/TLS](https://www.rfc-editor.org/rfc/rfc9001.html)、[RFC 9113：HTTP2](https://www.rfc-editor.org/rfc/rfc9113.html)、[RFC 9114：HTTP3](https://www.rfc-editor.org/rfc/rfc9114.html)、[RFC 9460：SVCB/HTTPS](https://www.rfc-editor.org/rfc/rfc9460.html)。
- [PostgreSQL 协议](https://www.postgresql.org/docs/current/protocol.html)、[MySQL 协议](https://dev.mysql.com/doc/dev/mysql-server/latest/PAGE_PROTOCOL.html)、[Redis RESP](https://redis.io/docs/latest/develop/reference/protocol-spec/)、[MQTT 5.0](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html)。实现验收时固定文档版本，不追随可变 latest 链接悄悄改变合同。
- [Microsoft MS-SMB2](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/)、[RFC 4511：LDAP](https://www.rfc-editor.org/rfc/rfc4511.html)、[RFC 8489：STUN](https://www.rfc-editor.org/rfc/rfc8489.html)、[RFC 8656：TURN](https://www.rfc-editor.org/rfc/rfc8656.html)。各未完成 profile 在实施前追加其精确规范条款、版本和独立样本。

最终交付表只描述经验证的 profile、已知边界与未完成项；性能数据和目录评分不替代这些事实。

## 10. 后续实施记录：统一递进展示与识别粘性

本节记录本聊天随后实际落地的实现，前文基线结果保持原样。已增加 30 个家族的入口回放矩阵及 12 个关联 profile 用例；“覆盖”指声明的样本与合同，不能将此表读成全部协议版本、密码套件和异常流量均已完整实现。

共用合同已落地：包内消息、已确认连接层次、诊断状态分别投影；只有前两个字段碰巧相似的残包、端口提示及缺少上下文的 TLS 密文头不会建立连接身份。连接身份由已经验证的事件更新，标量缓存与原始消息保留窗口分离。TCP 的新 SYN/ISN、接口/VLAN 域与 QUIC CID/flow 隔离；纯 ACK 仍为 TCP，但保留连接层次。已识别之后不逐消息重新全协议探测。

| 家族 | 本轮代表性验证与处理 | 保留的边界 |
|---|---|---|
| 01 TCP | 重分段、乱序/重传现有回归；ACK 身份、SYN 代际复用新增回归 | 缺口保留，不跨缺口拼内容 |
| 02 IPv4/IPv6 | IPv4/IPv6 样本、分片域与重叠拒绝回归 | 捕获不全和未支持隧道不冒充完整 |
| 03 TLS | 1.2/1.3、正确/错误/缺密钥、候选头拒绝与粘性 | 解密套件仍限已有实现；AEAD 不等于证书信任 |
| 04 DNS 家族 | DNS UDP/TCP、DoH、原生 DoQ | 明文长度前缀 DNS 显示 DNS，不能据此声称 DoT |
| 05 UDP | 不拼残缺数据报、会话 ID 与域隔离 | 端口只提示；不套 TCP 识别 |
| 06 Ethernet/VLAN | 标签栈与域、带 VLAN 样本 | 旧语料中的 Frame Relay Inverse ARP 不是 Ethernet ARP 支持证据 |
| 07 HTTP/1 | 1.0/1.1 从已解析字段显示，正文中异协议字符串不切换身份 | 请求头完整与正文完整分别表达 |
| 08 QUIC/HTTP3 | 原生 v1、CID 后续密文、HTTP3 与独立认证明文字节、UDP 会话页 | 未知 CID 不收编；密钥和版本范围维持明确限制 |
| 09 HTTP2/gRPC | 多路流/HPACK 现有回归；TLS 内层名称、消息字节、错误不被成功记录遮盖 | 无 protobuf schema 不编造字段名 |
| 10 ARP | 合成 Ethernet ARP 控制及现有真实捕获中的 ARP | 观察不等于地址所有权 |
| 11 ICMP/NDP | ICMP 与原生 IPv6 邻居发现 | 引用和邻居状态保持观察语义 |
| 12 DHCPv4/v6 | v4/v6 原生载体回放与状态回归 | 不把租约观察当成身份认证 |
| 13 mDNS/LLMNR/NBNS | mDNS/LLMNR 样本；新增 NBNS 名称、NB 地址、NBSTAT 全边界解析 | NBNS 记录类型 33 不按 DNS SRV 解释；未实现的事务关联明确为 unavailable |
| 14 WebSocket | 原生 Upgrade 与帧、错误 Upgrade 拒绝、连接阶段保留 | 裸两字节帧不建立 WebSocket 身份 |
| 15 SMB2/SMB3 | Samba 与结构/compound/会话回归，原始字节与认证子层分开 | SMB3 transform 保留密文边界 |
| 16 Kerberos | 登录与错误捕获、结构/传输回归 | 票据密文不冒充可见用户内容 |
| 17 SPNEGO/NTLM | 接上 SMB2 安全缓冲区；复用 bounded DER；真实 Type 1/2/3 与 guest oracle | 客户端名字为声明；MIC/凭证未验证 |
| 18 LDAP | Bind/Search、message ID、StartTLS/SASL 既有回归 | 安全层切换后不把密文当 BER 操作 |
| 19 RDP | TPKT/X.224 协商与现有阶段回归 | TLS/CredSSP 为显式边界 |
| 20 SSH | banner、KEX、NEWKEYS、连接粘性与分段 | 没有完整 banner 的 SSH 前缀不识别；后续密文不作明文 |
| 21 PostgreSQL | 原生与生成启动/查询、扩展/认证/COPY 等既有回归，重分段 | 缺少角色/启动上下文保持诊断 |
| 22 MySQL/MariaDB | 原生 MySQL、prepared、结果集、能力协商与分段回归 | MariaDB/压缩/TLS 的版本范围按已有测试声明 |
| 23 Redis | 原生 RESP2/3、push/流水线既有回归、分段和缓存探测基准 | 后续响应复用已建立状态 |
| 24 MQTT | 3.x/5.0 既有状态回归、5.0 入口与分段矩阵 | 短 CONNECT 特征不由界面直接放行 |
| 25 SMTP/IMAP/POP3 | 三类入口、会话分段、认证/STARTTLS 既有回归 | 加密切换不继续明文命令识别 |
| 26 FTP/FTPS | 控制、PASV/PORT/数据关联及既有安全切换回归 | 不猜任意数据连接属于 FTP |
| 27 NTP | 原生 v3/v4、截断/版本/模式负例 | 认证/扩展未覆盖范围不宣称验证 |
| 28 SNMP | 原生 v1/v2c/v3 边界与会话/安全回归 | 私有载荷、认证未验证时保持不透明 |
| 29 STUN/TURN/ICE | STUN/TURN 真实载体、事务与 ChannelData 既有回归 | 结构或事务观察不等于 ICE 成功或完整性验证 |
| 30 SIP/RTP/RTCP | 原生 SIPp 双向 RTP、SDP/RTCP 关联与现有复用负例 | 无 SDP/显式选择的弱 RTP 外观不收编 |

新增测试入口：`shark-cli/top30_test.go`、`shark-cli/conversation_test.go`、`pcaputil/protocol_top30_sticky_test.go`、`protocol_auth_envelope_test.go`、`protocol_nbns_test.go`。15 组 TCP 会话分别按 1/2/3/7/31 字节切分，与未切分的 PDU 边界、状态、字节哈希及主要消息语义比较；测试同时确认识别后探测次数不随后续消息增长。

本轮执行 `go test -json -count=1 -timeout=5m ./common/bin-parser/... ./common/pcapx/pcaputil ./common/yak/cmd/yakcmds/shark-cli` 通过；CLI 最终改动另行重跑。此命令不包含前文仍存在平台基线问题的 `pcapdb`/fixture 导出包，不能写成全仓库无失败。

性能微基准（Windows/amd64，i5-13490F，200 ms 本机单次测量）：已建立 Redis 会话为 **0 probes/op**；连接展示投影移除重复名称表分配后，由约 **2535 ns / 4912 B** 降到 **280 ns / 200 B 每次**。这是展示投影的局部微基准，不是全协议抓包吞吐提升倍数。

新增规范依据：[RFC 1002](https://www.rfc-editor.org/rfc/rfc1002.html)、[RFC 4178](https://www.rfc-editor.org/info/rfc4178/)、[MS-NLMP 消息语法](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-nlmp/907f519d-6217-45b1-b421-dca10fc8af0d)、[MS-SMB2 Session Setup](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/5a3c2c28-d6b0-48ed-b917-a86b2ca4575f)。Samba 样本的 Type/名称/guest 结果由已有 TShark oracle 独立核对，不将服务端 STATUS_SUCCESS 解释成凭证验证成功。

## 11. 新鲜实抓、逐包差分与故障修复

本节记录用户要求继续实抓后的结果。新增可重跑工具位于
`common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/`。每次启动真实 socket
服务，生成新的证书、端口、密钥、nonce、CID 和会话；Npcap 仅抓取这些测试
端口。不是从固定协议样本制造“成功”结果。输出保存在仓库外
`C:/tmp/yak-shark-fresh-live-20261024/`，不将会话密钥提交到仓库。

### 11.1 本轮实际输入

| 输入 | 原始帧数 | 场景与独立依据 |
|---|---:|---|
| `capture-01/live.pcap` | 755 | TLS 1.2/1.3、6 次恢复会话请求、HTTP keepalive、双向 gRPC、UDP/TCP DNS、QUIC/HTTP3、TLS→Redis 四元组复用；45 次应用操作 |
| `recheck/capture-01/live.pcap` | 753 | 重新启动相同服务，使用新的端口、证书、密钥、nonce 与 CID；45 次新的应用操作 |
| `internet-01/interface-8.pcap` | 33 | 公网 Cloudflare HTTPS，系统根证书验证，两个 HTTP/2 请求收到 200；另记录 QUIC Initial 尝试 |

共 **1541 个原始实抓帧**，三个输入的 Npcap 丢包均为 0。两个本机会话中的
中间 QUIC 连接经过真实 UDP 代理，分别主动丢弃 8 包，延迟 13/14 包，实际
触发重传和到达顺序变化。抓包过滤只保留代理客户端这一侧，避免转发双重观察。
公网 QUIC 未完成握手，不能作为公网 HTTP/3 解码成功证据；本机真实 HTTP/3
往返及其故障变体提供解码验证。

Wireshark 对照工具仍为独立的官方 TShark 4.6.9。对动态测试端口显式 Decode As，
开启 TCP 乱序重组，使用本次客户端自己的 NSS key log。每份输入、每个变体均
保留全部帧的 `frame-comparison.csv`，列出双方包列名称、连接层次、Info、事件
编号/状态与原始帧号；`tshark.tsv`、`yak.jsonl`、`events.jsonl` 保留原始对照。
不会用双方每个 Protocol 字符串相同替代消息正确性检查。

### 11.2 实际发现和修正

| 问题 | 修正与验证 |
|---|---|
| snaplen 残包、TCP 缺段导致整个 CLI 退出 | Shark 使用允许不完整的检查模式，继续展示其余包；原始低层 Replay API 的默认严格行为保留。残包明确显示捕获/线上长度和 incomplete，缺口不会拼接 |
| 缺少客户端 WINDOW_UPDATE 时，把完整服务端 HTTP/2 DATA 报成窗口超限，连带丢掉正常 gRPC 响应 | 原生捕获上下文传到 TLS 内层；对端 TCP 有待补缺口时，窗口超限验证标记 unavailable，保持本方向完整帧及消息。缺段/截短场景从 8 条提升到与 TShark 相同的 9 条 gRPC，完整字节哈希一致 |
| 缺少 QUIC Initial 后，随机密文被自动认成 CoAP 响应，第二轮又出现伪 DELETE 请求 | CoAP 原生 UDP 会话按端点和捕获域有界保存；新自动会话要求请求与 URI 证据，孤立响应不能建立身份；已知 RFC7252 选项逐项校验长度。两轮缺 Initial 控制均无 CoAP 误认 |
| QUIC ACK 丢失导致同一 RETIRE_CONNECTION_ID 出现在不同 PN 中，误报缺少 CID 上下文 | 有界退役记录让重复退役幂等，继续处理同包后续 STREAM；重传的 NEW_CONNECTION_ID 不能复活退役路由；未知序号仍拒绝，退役记录计入预算 |
| 第二次 SYN/SYN-ACK 未捕获时，UI 将旧 TLS 连接身份粘到后来的 Redis 字节上 | RST 后未观察到新握手的数据保持未关联；关闭期间回退查找只接受实际接收当前包的记录。正常完整握手仍识别 TLS→Redis，两次缺 SYN 变体均无旧身份继承 |
| TLS AEAD 失败后，后续包只有笼统 TCP，缺少停止解码的解释 | 保留已验证连接栈，给后续数据行及 JSON 增加解码停止原因；失败记录不会产生明文。当前实现仍保守停止该 TLS 解析器，不猜测密钥代际或跨断点继续拼内层消息 |
| HTTP/3 行只写 1-RTT/stream，控制帧与正文难区分 | 已解析帧显示 SETTINGS/HEADERS/DATA 及 DATA 字节数；未凑齐帧时显示 frame reassembly，不冒充完整 DATA。字段页、明文字节源、UDP 会话页保持递进层次 |
| 抓包时间戳倒退使连接的 first/last 反向 | 流记录使用最早/最晚观察时间，乱序输入不倒退 last |

收紧准入也核查了旧语料：nDPI CoAP 的第 1–5 帧只含未知选项 2，没有观察到可靠
URI；TShark 的详细树对它们显示 `Invalid Option Number 2`。这 5 帧不再凭
code/token 形状建立自动身份，原生回放统计由 819/47512 变为 814/47388。
测试显式断言这五帧保持 unrecognized。裸四字节 GET 同样需要显式字节会话
检查；完整 GET /a 及已建立会话的后续请求/响应保持支持。这是明确的准入边界，
没有把减少识别数当成解码提升。

### 11.3 差分验收与边界

两份新鲜 pcap 各派生 **13 种故障变体，共 26 次回放**：TCP/QUIC 乱序、缺段、
重复 TCP 段、TCP/QUIC snaplen、截短后原段重传补全、TLS 密文翻转、缺 ClientHello、
缺 QUIC Initial、未知短头 CID、第二代 TCP SYN/SYN-ACK 缺失。每份变体都有源
SHA-256、操作说明及新帧→原始帧映射；解析器不读取这些旁路答案。

- 完整本机抓包分别解析 **242/241 条 TLS record、10/10 条双向 gRPC 消息、
  12/12 个 HTTP/3 请求路径**；gRPC 全部消息与 TShark、应用自身 SHA-256 三方一致。
- 外网实抓的 **18 条 TLS record、14 个 HTTP/2 帧事件**无解析错误，双方均保留 33 帧。
- TCP 重排、重复段、截短后补传保持完整 10 条消息；真正缺失字节时只保留可验证
  内容，9 条 gRPC 与 TShark 相同。不产生不在应用 oracle 中的新消息字节。
- 第一份 HTTP/3 的 **27 个逻辑 DATA 帧**内容与 TShark 一致；TShark 在一次 STREAM
  重传上重复显示 DATA，逻辑消息统计去重，逐包 CSV 仍保留该帧。
- 第二份 TShark 对重传重复显示一次请求路径，并在一个乱序流中缺少部分 DATA；
  去重后的 12 条路径相同，Yak 的 12 份完整响应 SHA-256 全与独立应用记录一致。
  `h3_data_equal=false` 留在第二份 summary 中，没有掩盖差异或要求 Yak 丢掉正确正文。
- TLS 验签失败的保守停止策略保留 8 条已确认 gRPC，TShark 可继续恢复 9 条；这是
  当前恢复能力边界，报告没有把它写成逐包全部一致。连接身份保留但后续明文不可用。
- 未知/已退役 CID 的晚到短头包可能保持不透明；缺 Initial 不据密文外形猜 HTTP/3。
  缺少新握手时，RST 后数据不会猜成新连接身份。二者优先避免串会话和误识别。

Windows ConPTY 使用 CP936 和真实颜色控制缓冲区，验证新实抓的 gRPC 与 HTTP/3
字段页、字节来源切换、消息/关联证据页、80×30→180×48 缩放、退出及代码页还原。
原生快照与绘图位于同一输出目录；本次没有把终端截图当成解析正确性证据。

最终 `go test -json -count=1 -timeout=5m ./common/pcapx/pcaputil
./common/yak/cmd/yakcmds/shark-cli` 两包通过；HTTP/3 行摘要的最终改动另行完整
重跑 CLI 包通过。低层严格重组、并发 worker、原始协议语料和 Top30 矩阵均在
上述包测试中运行，未将这个结果扩张成全仓库测试通过。

本轮新增核对规范：[RFC7252 §5.10 选项表](https://www.rfc-editor.org/rfc/rfc7252.html#section-5.10)、
[RFC9000 §5.1.2 CID 退役](https://www.rfc-editor.org/rfc/rfc9000.html#section-5.1.2)。
要求 URI 证据是本工具为防止自动误识别采用的保守策略，不是声称 RFC 禁止合法的裸根路径请求。

## 12. 中途 TLS 实测与剩余协议的新增实抓检查

### 12.1 Wireshark 没有握手时的实际行为

本轮从前一轮公网 HTTPS 实抓中取出一帧完整 Application Data，完全移除握手及
密钥。原始会话已知为 TLS 1.3。官方 TShark 4.6.9 对以下四份单帧输入的结果为：

| 输入 | Protocol | Info | 观察含义 |
|---|---|---|---|
| 443、完整 record | TLSv1.2 | Application Data | 无握手仍能识别 record；这里不是协商版本证明 |
| 改为 18447、未 Decode As | TLSv1.2 | Application Data | 该样例的 record 启发式不依赖 443 |
| 18447、显式 Decode As TLS | TLSv1.2 | Application Data | 与自动识别结果相同 |
| 443、移除五字节 record header | SSL | Continuation Data | 此端口/载体条件下按 SSL 续传展示，不能据此证明所有中途密文都可确认 |

因此“Wireshark 没有 session/handshake 就只显示 TCP”不成立；“Wireshark 显示
TLSv1.2 就证明协商了 TLS 1.2”也不成立。测试包的 record legacy version 为
0x0303，原会话实际是 TLS 1.3。脚本 `remaining/midstream_tls.py` 保留派生方式、
输入 SHA-256 与逐项原始输出，可重新执行。

Yak 现在分层表达：完整普通加密 record 显示 **TLS / Application Data record**，
详情明确“握手未捕获、协商版本未知、认证未验证”；证据更弱时显示 **TLS?**。
至少两条独立完整 record 可在原生同一流/代际/捕获域中保留“推断的 TLS carrier”，
不会冒充协商版本。已有可信握手身份不会降级。已确认连接中的未完成消息分段
显示协议名及 `connection-associated bytes`；纯 ACK 保持 TCP，并保留连接栈。
原始字段树与隔离解码子进程同样传递这些边界，避免列表与详情相互矛盾。

### 12.2 两次新增真实 socket 抓包

输出根目录：`C:\tmp\yak-shark-top30-live-20261024`。

| 输入 | 帧数 | 应用记录 | Npcap 丢包 | PCAP SHA-256 |
|---|---:|---:|---:|---|
| capture-05/live.pcap | 860 | 49 | 0 | 0b64ad100e1b10cd35a840131689bc3fb223506755ce7dfa13f4768358e997a2 |
| capture-06/live.pcap | 854 | 49 | 0 | 4989cecfbc22c268295f29f118abca6100fd55cb8080771570ad61fa2b54d66e |

两次共 **1714 个新实抓帧、98 条成功应用记录**，各自使用新随机端口、nonce、
证书和会话密钥。第二次在修复后重新运行服务和客户端，不是重复固定 PCAP。
早期 capture-01 至 04 是测试服务退出/文件模式等问题的诊断输入，不计成功覆盖。

| 协议组 | 本轮实抓来源与具体范围 |
|---|---|
| SSH | x/crypto SSH 服务端/客户端；同连接两次 exec、较大输出；NEWKEYS 后只验证加密载体及连接粘性，不宣称解出命令正文 |
| WebSocket | Gorilla 双端、协商压缩、二进制 echo；第二次实际正文为 3/2048/70000 字节 |
| FTP/FTPS | pyftpdlib 2.2.0 + Python ftplib；明文/显式 AUTH TLS，各两次被动 RETR；FTPS 数据为 PROT P |
| SMTP | aiosmtpd 1.4.6 + smtplib；一封明文、同连接 STARTTLS 后两封邮件 |
| SMB2/NTLM | Impacket 0.13.1，SMB 2.0.2；两份文件下载和一次错误密码拒绝；不作为 SMB3 加密支持证明 |
| MQTT | AMQTT 0.12.1 broker + Paho 2.1.0；3.1.1 QoS 0/1/2 发布与订阅交付，含 70000 字节消息 |
| PostgreSQL/MySQL | lib/pq、go-sql-driver/mysql 独立客户端；最小 wire peer，协议启动/认证及同连接三次文本查询；不是运行完整数据库 |
| LDAP | go-ldap 客户端 + 最小 BER peer，Bind 与三次 Search；不是生产目录服务 |
| POP3/IMAP | 标准库客户端 + 最小 mail-store peer；各两次 RETR/FETCH literal |
| SNMP/NTP/DHCP | gosnmp v1/v2c GET、ntplib 与受控应答端、DHCPv4 受控 Discover/Offer/Request/Ack 字节交换；不更改系统租约或系统时间 |

每份 PCAP 都用官方 TShark、最终 Yak CLI 和原生事件回放器生成全部帧 CSV。
随机端口对 TShark 显式 Decode As，TCP 乱序重组开启，密钥仅来自该次测试客户端。
正文 oracle 来源于独立客户端收到的内容，解析器不读取 manifest 或变体侧表。

### 12.3 新发现及修正

| 发现 | 修正及验证 |
|---|---|
| MySQL 80 字节问候开头 `50 00` 被 SLMP 短前缀占用，对端又误认 PostgreSQL | 完整 MySQL 问候的版本、salt、能力证据优先；实抓 12 个逻辑消息含三份完整结果集，逐包列 24 条 MySQL 与 TShark 相同 |
| MQTT 3.x 仍走无状态旧分支，没有 QoS 关联；发布正文不能作为统一字段读取 | 3.1/3.1.1/5 会话路径统一；legacy PUBLISH 不按 v5 properties 读取；保留精确正文、版本和 QoS 状态，QoS2 四步回归 |
| FTP/SMTP 的升级回复后停止整个流，带正确 key log 也看不到后续内容 | 原生捕获继续 TLS record 与认证明文子流；重置 STARTTLS 前的应用协商状态，连接栈明确 STARTTLS → TLS → 应用 |
| 被动 FTP 数据没有与控制流关联，FTPS 文件正文也无法定位 | 有界、单次使用的控制命令/端点/地址/捕获域关联；PROT P 只在成功回复后生效；冲突映射保持不关联，TTL、关闭、预算、超大 chunk 分拆均有回归 |
| SMB LOGOFF/错误响应清空 SessionId 后丢失请求关联，EOF 报两次未完成 | 唯一 MID/命令、相反方向限定的回退；歧义不关联；CANCEL 不新建期待响应；补齐常见命令名 |
| SSH NEWKEYS 后的正常密文又显示 TCP | 保留 SSH 加密载体，明确 plaintext unavailable；不把二次密钥交换的密文冒充明文解析 |
| POP3 中途 `+OK` 被当 Redis，随后邮件正文报 RESP 错误 | 单独 RESP scalar 不建立原生 Redis 身份；已建立结构化 RESP 会话继续正常消费标量回复；74 个故障变体由此暴露并验证修复 |
| NTP 单包无状态解码将服务端响应写为 missing-request | 改为 unavailable-stateless-datagram，明确当前路径没有进行事务关联；另验证捕获中响应 origin 精确等于请求 transmit |
| 列表依赖连接上下文，字段子进程却丢失上下文；正文中外协议字符串形成伪字段子树 | 隔离 IPC 传递连接栈、证据、加密状态；续传字节不会作为新 SSH/HTTP 消息重新探测，保留传输字段与真实 PDU 来源 |

### 12.4 验收结果与尚存边界

两次完整抓包的正文核验分别通过：FTP/FTPS 四份完整文件、MQTT 六次发布/交付、
WebSocket 六个解压帧、SMTP 三封、POP3/IMAP 各两份、PostgreSQL/MySQL 各三行、
LDAP 三个 cn、SNMP 十二个响应值、SMB 两份完整文件及 DHCP 两份应答。
NTP origin/transmit 精确相等；LDAP 11、SNMP 12、SMB 56 个事件的消息 ID 与其
源帧上的 Wireshark 字段一致。完整输入没有 malformed/incomplete/unrecognized；
两条 SSH Encrypted 是预期安全边界。

capture-05 派生 **74 个故障输入、63409 个回放帧**，包括 15 个服务的丢包/截断、
TCP 重复、完整重传补全、中途开始及适用的连续段重排；全部通过。断言检查没有
产生原输入中不存在的成功 PDU，数据流片段不产生新正文，重复/完整补传保留原
消息集合。故障确实可能减少识别，测试不要求残包冒充完整消息。

最终 30 家族矩阵再次逐包对照完成，共 **1822 个家族用例回放帧**；共用载体会
重复执行，因此这个数字不是新的唯一实抓帧数。ARP 控制为合成，其余使用已封存
来源载体。Kerberos、RDP、SIP/RTP、TURN、mDNS 等本机未新建完整生产服务，仍
以既有载体和负例/分段/状态回归为证据；不能声称 TOP30 所有版本均已实网完整验证。
混合数据库 fixture 同时含 PostgreSQL/MySQL，混合缓存 fixture 同时含 Redis/Kafka，
矩阵保留其他协议的真实事件，未按用例名称把它们删除。

本节当时 TLS/QUIC 的 13 种故障也用新二进制重跑通过。当时的边界包括：AEAD 失败后
Yak 保守停止，gRPC 可保留 8 条而 TShark 恢复 9 条（第 13 节已修复该限定案例）；公网 QUIC 连接未完成；NTP
原生数据报尚未做有状态关联；被动 FTP 新路径不覆盖主动 PORT/EPRT；SMB3 加密、
完整数据库/目录、RDP 登录和未实现 TLS 套件不能由此次结果推断为支持。

验证命令记录：`bin-parser/...` 全测试通过；最后修改后完整重跑 `pcaputil` 和
`shark-cli` 通过，最终展示文案另完整重跑 CLI 通过。新增 FTP 数据 profile 的
Winlab 8 段/32 字节由 TShark Follow TCP Stream 独立核对，封存旧控制计数保持 14，
以显式新增 profile 断言扩展，没有修改原始封存输入/答案。

Windows CP936 原生 ConPTY 验证了无握手 TLS、FTP 数据、MQTT 的字段/字节/会话/
证据页及 80×30→180×48 缩放；另做真实 HTTP/UDP 抓包、鼠标/筛选/缓存、60×18
窄窗、退出和代码页恢复。最终程序位于 `build/shark-windows/yak.exe`，现有已启动
进程需退出后重新启动才能载入更新。

## 13. SSDP 接入、TLS/QUIC 恢复及 Windows 实跑复核

### 13.1 截图中的 SSDP 为什么未识别

截图的 `239.255.255.250:1900`、`NOTIFY * HTTP/1.1`、NT/NTS/USN 是
SSDP/UPnP 设备发现。旧目录虽有 `upnp.yaml`，其模板只覆盖很窄的 NOTIFY
排列；UDP 的 SSDP import 又指向 HTTP 模板，原生 UDP 分发也没有接入 SSDP。
所以出现了“port hint: SSDP”，实际事件却是 ProtocolNotRecognized。

新增 `application-layer/ssdp.yaml` 和共用的有界解码器，显式 bin-parser、编译
规则、原生 UDP 与页面共用语义。覆盖 NOTIFY alive/byebye/update、M-SEARCH
组播/单播及 200 响应，保留大小写无关头、厂商未知头、顺序与精确字节区间。
检查完整 CRLF 结束、请求行、多字段身份组合、USN/目标一致性、数值边界、
冲突重复头；正文、拼接报文、非法长度、截断不能冒充完整 SSDP。

SSDP 每个数据报独立验证，不因端口或先前 NOTIFY 将后续垃圾粘成 SSDP；不虚构
搜索事务匹配。真实 Android/DLNA 的 `NT: ssdp:all` 与缺少 UPnP SERVER token
作为明确的 Conformance Warnings 保留，识别协议不等于厂商完全符合 UDA。
`Identity Verified: false` 也明确区分设备自报标识和认证身份。

页面递进为 `Ethernet → IPv4 → UDP → SSDP`，摘要显示操作、NTS、目标、寿命与
LOCATION；字段页优先展示关键语义，厂商头仍可展开并定位源字节。未抓取任何
设备自报 LOCATION URL。

格式与边界对照 [UPnP Device Architecture 2.0](https://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v2.0-20140901.pdf)；
TLS/QUIC 密钥世代对照 [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446.html)
与 [RFC 9001](https://www.rfc-editor.org/rfc/rfc9001.html)。设备偏离规范的内容保留为告警。

### 13.2 新实抓输入与独立语义证据

新输出根目录为 `C:\tmp\yak-shark-recovery-20261024`。下列为本轮新增的
**3390 个实际捕获帧**，不把封存回放、变体或重复执行算成新抓包。

| 输入 | 帧数 | 独立实现及本轮结果 |
|---|---:|---|
| discovery-01/physical.pcap | 391 | Windows 物理网卡、一次 M-SEARCH；含截图中 192.168.0.109 的实际设备广播；391 个包均与 Wireshark SSDP 方法、HOST、LOCATION、SERVER 对齐 |
| discovery-01/loopback.pcap | 7 | 新 socket 交换：5 个合法 SSDP、1 个缺身份的 NOTIFY、1 个垃圾数据报；只前 5 个完整识别 |
| tls-suites-02/live.pcap | 204 | OpenSSL 1.1.1 独立客户端、Python SSL 服务端；6 种套件，同连接各 3 次 HTTP 请求；18 个响应正文哈希同时与客户端和 TShark 相等 |
| capture-01/live.pcap | 752 | 45 次应用操作；真实 QUIC 代理丢 8 包、延迟 14 包，Npcap drops=0；TLS/gRPC、H3/CID、会话恢复、TCP 元组复用 |
| quic-epochs-02/live.pcap | 2036 | 独立 aioquic 1.7.0 双端；三种 AEAD、每连接 6 次 H3 请求、两次密钥更新、CID 更换、NAT 重绑定、三个新连接复用同一原始 UDP socket |

TLS 实抓套件是 TLS 1.3 AES128-GCM/SHA256、AES256-GCM/SHA384、ChaCha20/
SHA256，以及 TLS 1.2 ECDHE-ECDSA 对应的三种 AEAD。客户端执行证书验证，
manifest 记录实际协商密码和收到的正文；这不表示 Yak 自己完成证书链验证。
解码结果为 120 个 TLS record、36 个 HTTP 请求/响应事件。

aioquic 网络真实丢 12 个数据报、延迟 187 个、损坏 3 个密文；发起 6 次密钥更新、
3 次 CID 更换、3 次 NAT 重绑定。18 个请求与 18 个响应的 **36 个完整正文**
均匹配独立双端记录；2042 个认证 QUIC 包号逐个与所在捕获帧的 TShark 字段一致。
三个连接各自保留 0→1→0 密钥阶段，重绑定前后保持同一 FlowID，新连接各用不同
FlowID。三个损坏密文只产生认证失败，不提供任何正文。

保留一项差分：TShark 4.6.9 的 HTTP/3 PDU 字段直接输出 33 个正文，Yak 为 36。
再次独立按 TShark 已认证的 QUIC STREAM ID/offset/data 重组，检查所有重叠字节
一致、无洞，再按 H3 帧长度提取 DATA，得到同样 36 个哈希。因此这三个差异是该
输入上 Wireshark 应用层重组遗漏，不能伪报为两边 HTTP/3 层计数一致。
`quic_epochs.py`、`quic_compare.py` 保留了独立实现、逐包 CSV、STREAM 原始 JSON
与遗漏哈希；产品解析器不读取这些 oracle。

固定输入 SHA-256：

| 文件 | SHA-256 |
|---|---|
| discovery-01/physical.pcap | 2a6bff670a346b856e9ae01f20a4a0b912386c12fb95192866e997e3f6fb35ad |
| discovery-01/loopback.pcap | 21f7bd399a24e9cfbb1d4b95d126d7b739c8bd6d3b033a51ebe96ef85fd97948 |
| tls-suites-02/live.pcap | e1e532ac3170d329a2d5cdd8f80d7772a4849fc3d94f2ebd14ac61b7cfa96b78 |
| capture-01/live.pcap | 19d62c0b4b3b7e7de6e1712a3601b4d40e2e6b9dc1927ca855781d3bcadd8113 |
| quic-epochs-02/live.pcap | b039d241cea75dbdae8538190ebbc440c8c3e3876f9c2d8591f8da06a421e707 |

tls-suites-01 是不兼容 OpenSSL 1.1.1 命令选项的失败诊断运行，不算成功覆盖；
quic-epochs-01 是未加入密文损坏的探索运行，也没有计入上述新增帧总数。

### 13.3 找到并修正的恢复、复用和展示问题

| 问题 | 修正与证据 |
|---|---|
| TLS AES256/ChaCha 套件、SHA384 PRF/secret/key 长度不全 | TLS 1.2/1.3 AEAD 参数按套件处理；QUIC 增加 AES256；上述独立双端真实流量和正文哈希核验 |
| 一个 TLS record 认证失败就关闭整个流，后续正常反方向 gRPC 也丢失 | 原生路径隔离单条失败；保留反方向和可靠 record 边界。只在 AEAD 认证成功后提交有限序号/密钥世代恢复；四次连续失败后停止试探，保持加密封套 |
| 恢复时把旧 plaintext 当新 plaintext，或跨字节洞拼成内层消息 | 每 record 先清空旧明文；失败清理该方向未完成内层缓存；不可恢复的同向应用洞保持 unavailable，完整已认证 record 明文作为独立字节来源查看，不冒充 HTTP/gRPC |
| TLS KeyUpdate/Finished 丢失和 QUIC 大包号窗口、旧阶段乱序 | 三种 AEAD 均测试序号跳跃、未捕获 KeyUpdate、旧 key phase 晚到、重复包、第二次 key update；认证失败不能推进包号或 key phase；包号额外窗口搜索固定有界 |
| CID 退役即删除路由，真实晚到 ACK/RETIRE 包变成普通 UDP | 保留有界退役历史；仅在 AEAD 验证成功后关联；不重新激活 CID，活动路由优先、歧义拒绝、捕获域隔离。实抓 3 个退役 CID 晚到包全部恢复原连接 |
| 中途接入连接后遇到新 SYN，身份沿用旧代 | 新 SYN 与中途序号起点不同时先结束旧 flow、释放缓存，再建新代；原始 SYN 延迟到达且起点一致时补充握手，不误重置。1/2/4 worker 和清理计数回归 |
| 同包一条 TLS record 失败、另一条已恢复，列表和首条详情看起来矛盾 | 列表明确 Includes rejected TLS record；详情显示认证/拒绝条数并优先呈现恢复记录，每条 PDU 保留独立认证与字节来源 |
| 已知 QUIC CID 上认证失败显示 UDP，容易误解连接协议改变 | 保留 QUIC 加密载体并明确认证失败，绝不提升为 HTTP/3；无连接证据的失败不建立 QUIC 身份 |
| 离线 PCAP 页面把内核丢包显示 0 | 改为 unknown (file)，避免将未提供的捕获统计写成确认零丢包 |

此前 8 对 9 条 gRPC 的限定问题已关闭：新鲜 752 帧输入中的 TLS 密文翻转变体，
现在 Yak 与 Wireshark 均保留 9 条完全相同的 gRPC 正文。13 个故障变体共
9764 个回放帧全部通过，覆盖丢包、截断、重传补全、乱序、重复、缺 ClientHello、
缺 QUIC Initial、未知 CID、第二代 SYN 丢失，且不能新增原始输入不存在的正文。

TCP 字节洞导致 record 边界未知时，仍不扫描任意密文猜测新 header；“某条 record
可恢复认证”也不表示丢失的明文、同向 HTTP/2 流控或 QPACK 状态可凭空重建。
TLS 恢复搜索有固定记录/世代预算，QUIC 历史 CID 也会有界淘汰；超出边界保持明确
不完整或加密状态，而非为了提高识别率编造内层协议。

### 13.4 核心协议的实际复查结论

最终程序重新执行 30 家族矩阵：30 项、1822 个回放帧通过逐包输出检查；其输入
仍是版本化封存载体和一条明确合成的 ARP 控制，不称为新增真实流量。
`bin-parser/...` 全部测试通过；最后解析器变更后完整 `pcaputil`、`shark-cli`
通过，最后展示变更后再次完整跑 `shark-cli`。源码封存输入未被改写。

**核心协议不能称为全部完备。** 此次确实扩大并核实了常见配置，但 QUIC v2、
所有 0-RTT/Retry 组合、TLS CBC/旧套件、ECH 内层、SMB3 加密、完整 RDP 登录、
所有 Kerberos/SPNEGO 分支、真实数据库/目录服务的全部认证与扩展、NTP 有状态
事务、FTP 主动模式等，仍没有由这些测试证明完整覆盖。第 2 节的全仓库 Windows
fixture 权限和 pcapdb SQLite URI 失败也不能被本轮选定包通过覆盖掉。

### 13.5 Windows 实际运行页面与复现

在 Windows CP936 原生控制台实跑 SSDP、TLS 恢复、AES256 HTTP/3 的字段、字节、
会话/证据页、80×30→180×48 缩放与退出恢复。另实抓本机 HTTP/UDP 验证筛选、
缓存、鼠标、60×18 窄窗及代码页恢复。每页 PNG 按实际 ConPTY 控制台字符和颜色
属性绘制，属于实际运行缓冲区快照，未伪装为桌面像素截图或设计稿。

证据位于输出根目录的 `native-ssdp-final`、`native-tls-final3`、`native-quic-final`
及 `native-live-final2`，最终 PNG 为 `ssdp-native-final.png`、
`tls-recovery-native-final.png`、`quic-native-final.png`。
辅助程序源码及渲染器位于 `testdata/live-audit/discovery/console` 与
`testdata/live-audit/discovery/render_console.py`。

在 cmd 或 PowerShell 中从仓库目录启动新程序：

```text
.\build\shark-windows\yak.exe shark
```

需要退出旧实例后启动这个路径；旧进程不会自动加载新的解析器。

最终 EXE SHA-256：`cdebbf73ce77c436302f8f5f0e1ce86a73de35c64a95d9259d04b7e2b8a028a8`。
最后两次完整 CLI 回归与最后一次完整 pcaputil 回归均通过；最终故障/矩阵输出和
`final-verification.json` 保留具体构建哈希、帧数与控制台恢复结果。


## 14. 常见端口提示与原生解析补齐，SSH 深度复核

### 14.1 目录、规则和分发的真实缺口

本轮沿 `tcpPortProtocols` / `udpPortProtocols` 全部端口提示核对原生 profile，
并逐项核对新 profile 的公开目录条目、YAML entry、共享有界解析器及实际事件。
旧目录有规则，不代表原生抓包已经调用了它；原生实现存在，也不代表它已登记在
profile 清单。两个问题分别修复，并有清单一致性测试，不能用声明代替实跑。

新增共享规则 `network_control_fields.yaml` 与对应原生分发：

| 入口 | 本轮支持及主要字段 | 明确边界 |
|---|---|---|
| IKE / IKE NAT-T | v1/v2 头、精确总长、有界 payload 链；v2 SA proposal/transform、KE/Nonce/Notify/ID；显示 IKEv1/v2 与具体交换名 | SK/SKF、v1 加密体保持密文；不推断认证、SA 成功或隧道建立；NAT-T 此入口仅处理 Non-ESP marker + IKE |
| WireGuard | 148/92/64 字节握手/响应/cookie 封套，传输 receiver/counter；观察到的接收索引、方向、捕获域关联 | 无私钥，不验证 MAC/身份/重放，不解析内层 IP；索引历史与超时有界 |
| RIP | v1/v2 request/response、完整路由项、metric/保留字段、v2 简单口令 | 不验证口令，不安装路由；MD5 等扩展尚不支持 |
| NetBIOS Datagram | RFC1002 编码及作用域名称、类型/flags/ID/offset/length、查询/错误/数据 | 分片明确展示，不冒充已重组 SMB 数据 |
| ONC RPC / UDP | RPCv2 CALL/REPLY、XID/program/version/procedure、400 字节上限 opaque_auth 与 XDR padding | 应用过程体保持原始数据；REPLY 明示缺少有状态请求关联，不冒充完整 NFS/Portmap 过程 |
| L2TPv2 | 可选头字段、Ns/Nr、控制消息与有界 AVP、SCCRQ 等具体名字 | hidden AVP 和 PPP 数据不解密、不冒充隧道认证 |
| OpenVPN / UDP | 明文控制封套、ACK/packet/session ID、观察到交换后的 data 封套 | tls-auth/tls-crypt 配置未知；数据保持密文，key ID 不是认证证据 |
| IPMI / RMCP | ASF、IPMI1.5 检查和、RMCP+ 头、Open Session Request 算法项、完整性与加密标志分离 | 协商算法/密钥未知时，认证 trailer 有界保留，不宣称 MAC 或身份通过；没有真实 BMC 会话证明 |
| Telnet / TCP | IAC WILL/WONT/DO/DONT、完整 SB/SE、转义 IAC、原始 Data token | 不把终端文本解释为登录结果；有界前缀探测不扫描任意数据找 0xff |
| NetBIOS Session Service / TCP | Called/Calling name、正/负/重定向响应、keepalive；可进入 SMB2 | 不执行重定向，无 SMB1；SMB2 前保留 NBSS 递进载体 |

九个 UDP profile（含 NAT-T）及两个 TCP 入口使用同一共享语法构建 bin-parser
树与原生语义，原生事件可取得字段及根标量的精确 PDU 字节区间。内部区间表不进入
展示；嵌套字段不借用同名根字段的偏移。所有新控制 profile 保持 `partial`。

目录通过 `ProfileOf` 关联到原有路线图家族，不增加封存 corpus 的家族数、不抬高
P1 分数，也没有改写封存 pcap/manifest。旧规则 API 继续保留；除了 RIP 和此前的
SSDP UDP import，其他历史 Ethernet/UDP 导入没有全部改用新树，以免破坏既有
WGInit/Sender、L2TP PPP 等字段路径。新原生入口及直接调用的 canonical entry
则明确使用新语法。此区分已写入 CLI README 与目录说明。

### 14.2 SSH 改进与真实独立实现

SSH 从仅靠 packet number 的解释改成结合已观察协商选择布局：固定 DH、DH-GEX、
P-256 ECDH、Curve25519 分开校验。KEXINIT 必须包含完整十组名称列表、follows
和 reserved；校验包长、padding 和布局，NEWKEYS 不能夹带额外字段。DH-GEX 的
31 是 GROUP，不能当成服务器主机公钥；未知方法显示 layout unconfirmed。
`first_kex_packet_follows` 猜错时只丢弃该包的语义解释，后续正确交换继续处理；
扩展标识 ext-info / kex-strict 不作为可协商密钥算法。

新版 Reply/NEWKEYS 保留已观察到的具体 KEX、host-key、加密/MAC/压缩参数；
字段页优先显示算法、公钥指纹、加密方式及验证状态，列表不再只给 ECDH 家族名。
公钥指纹从公开 blob 计算，绝不等同服务器身份验证。Yak 的 Signature Verified /
Peer Identity Verified 仍为 false；NEWKEYS 后的登录、channel、重协商内容仍是密文。

使用独立 Windows OpenSSH 10.0 客户端与 Paramiko 5.1.0 服务器完成五条连接：
Curve25519@libssh、P-256、group14-sha256、DH-GEX-sha256、客户端默认算法。
默认实际选择 `mlkem768x25519-sha256`；确认其协商名、外层包和主机公钥 blob，
**并没有实现或验证完整 ML-KEM 混合方法体**，该部分明确标注未确认布局。
每条连接做临时公钥认证、严格 known_hosts 校验、128 KiB 数据传输，RekeyLimit=32K；
五条应用正文共 655360 字节均由独立两端核对。测试服务器只返回固定 audit 正文，
不执行收到的 shell 命令，临时私钥不进入仓库或提供给 Yak。

历史 M1 的一字节 Curve25519 公钥是错误样本。保留其原始字节和哈希，明确断言
malformed；另加有效 32 字节 companion 验证正向粘性。没有为了通过而替换封存输入。
NEWKEYS 的严格语义 oracle 仅补充独立双端固定配置的算法常量，仍逐字段精确比较，
没有移除旧断言或将实际输出反抄为期望值。

### 14.3 本轮新抓包与逐包核对

环境是 Windows + Npcap loopback + TShark 4.6.9。以下是新鲜的本机应用流量及
明确受控的 socket 报文，没有把它们称为外网/生产环境抓包。输出根目录：
`C:/tmp/yak-shark-ingress-20261024`。

| 输入 | 帧数 | 独立证据与检查结果 |
|---|---:|---|
| ssh-04/live.pcap | 379 | 五种真实 SSH 应用连接；42 条明文识别/握手事件逐项匹配 WS message number/列表与应用协商结果，五个主机公钥 SHA256 全匹配；10 条加密边界事件；Yak 的 174 SSH / 203 TCP / 2 就绪 UDP 帧与 WS 对齐 |
| wireguard-02/live.pcap | 21 | 两个 pinned wireguard-go 端点，在普通 UDP socket 上真实加密、认证、解密交付 16 个内层包；18 条协议帧与 WS 的 type/sender/receiver/counter 全匹配，另 3 条就绪 UDP；覆盖 MTU 1420 附近的包长 |
| control-05/live.pcap | 130 | Scapy 2.8.0 独立编码的 16 条正向 UDP 控制报文与 16 条残包/垃圾负例；原始字节及 56 项适用标量检查；两个真实 telnetlib3 5.0.1 4096 字节二进制传输完整重组匹配；3 条 NBSS 控制事件 |

WireGuard 使用内存 TUN，不创建系统适配器、不改变路由；端点私钥未提供给 Yak。
Yak 能识别封套和关联索引，依然显示 encrypted / authentication unverified。
控制报文组是实际发收并捕获的独立 wire controls，**不是完整 VPN、路由器、RPC
应用服务器或 BMC 的端到端会话**。Telnet 数据包含全部 256 个字节，验证 IAC
转义还原，而非只比较几条固定协商样本。

Windows 拒绝绑定 UDP 500/4500（WinError 10013），因此 IKE/NAT-T 使用记录在
manifest 的随机端口，Yak `--decode-as udp:PORT:PROFILE` 与 TShark `-d` 同时显式
选择。没有改系统服务、防火墙或申请提权。显式选择仍验证结构，四条错误 IKE
产生 malformed 诊断，其 CLI 行保持 UDP；其他负例也没有成功提升为应用协议。

不能宣称两个程序所有标签完全一致：WS 在显式选择下会给部分垃圾报文 RIP、
OpenVPN、ESP 等标签，Yak 保守保留 UDP；RPC CALL 显示 ONC-RPC，而 WS 根据
program 显示 Portmap。Telnet 的 ACK-only 帧在捕获/启发式状态下也有标签差异。
CSV 完整保留每个帧号、双边协议、信息、原生事件和递进栈，没有为了相同而归一化。
ACK-only 在 Yak 保持 TCP，连接栈另示 Telnet/SSH，这与数据包有完整应用消息是
不同证据层次。共核对 530 个上述新输入帧，未把重放计作新鲜流量。

| 输入 | SHA-256 |
|---|---|
| ssh-04/live.pcap | ebd709a638c54322a1a8e7254f8a2b15c468d53e3f103f1eaa2239d82d53bbf2 |
| wireguard-02/live.pcap | a96b4b4c85d7ffd5b807ff5ebd86aa0863bbbecf0dbbe0d527ac66d69d621717 |
| control-05/live.pcap | 7b8542fba0ea75f061d36a908276d69f5a97d4e2fcade9521aae0f7a0f165f3e |

成功的 ssh-02（302 帧）和 wireguard-01（18 帧）另保留为故障矩阵基线。
ssh-03 启动时漏了首段应用数据，其对应流在 WS 也只显示 TCP，没有把 Yak 的
保守拒绝当作漏识别修成猜测。control-04 则暴露了 Telnet 协商夹带 SB、客户端先
发送数据的合法顺序；原始 127 帧保留，修复后在 comparison-final 再核对通过，
不重复计入上述 530 帧。捕获脚本现先实际捕获 UDP 就绪标记，再启动业务；这些
标记原样留在 pcap 并检查不能提升为 SSH/WireGuard。
早期导入/保留端口失败、控制 manifest 变量错误运行均不计入成功证据。

### 14.4 本轮修复的识别、关联和页面问题

- WireGuard 传输帧错误地要求总长为 16 的倍数。官方实现 `calculatePaddingSize`
  会将填充限制在 MTU，1420 字节内层包产生 1452 字节 WireGuard 报文。本轮独立
  两端已认证交付且 WS 成功识别的八个包，修复前均漏掉，修复后全部识别。现在
  保留至少 32 字节封套/tag、保留字节及接收索引关联检查，不从密文长度推断 padding。
  `before-fix-events.jsonl` 保留原始失败证据。
- Telnet 在非标准端口依赖相邻三字节命令，遇到夹在协商之间的 SB 就提前放弃。
  改为最多 1024 字节的连续 IAC 前缀、有界完整子协商或双向对应 option 证据；
  Probe 和 Feed 的 NeedMore 一致，所有切分、错误转义、无关 option、超长负例验证。
  同时修复已识别 Telnet 未进入语义 consume 的问题。
- NBSS Session Request 通常 72 字节，超过通用 64 字节探测窗，会过早停止。
  增加严格长度/flags 的有界等待；Session Request/Response/Keepalive 及所有切分
  正常。NBSS→SMB2→后续 NBSS 控制仍能分派，递进栈保留真实载体。
- OpenVPN 新客户端 hard-reset Session ID 在同一 UDP 元组复用时，不再继承旧
  服务端 key-ID 证据；新 FlowID 必须重新观察交换，旧 reset 重传则保留同一代。
  ACK 的 Remote Session ID 也须匹配已观察对端，未知关联不能成功提升 DATA。
- WireGuard receiver 历史按方向、端点、捕获域和超时隔离；错误 receiver、缺
  initiation、跨域和过期数据不能获得关联。控制类超时淘汰不再误删 QUIC/TFTP。
- RMCP+ Payload Length 不包含 integrity trailer；完整性标志不等于加密标志。
  修正两者，保留配置相关 trailer 为未验证数据，明文 IPMI 校验和仍单独检查。
- 新控制字段的根标量区间正确映射到 PDU 字节来源；修复通用 Other decoded fields
  增加一层缩进导致区间失效的问题。内部区间元数据不显示，返回字段副本不能修改
  解析器持有的范围。SSH Reply 的具体算法及验证状态也已在真实页面补齐。

故障矩阵 14 个变体、2524 个回放帧全部通过：重复、丢 KEXINIT、丢 identification、
数据包乱序、snaplen、原字节重传修复、TCP 四元组复用、缺 WireGuard initiation、
未知 receiver。另将真实 SSH KEXINIT 切成两个重算 IP/TCP checksum 的包，后半段
先到，仍恢复全部原始消息；不是仅检查加密批量数据的乱序标签。每个变体都有逐帧
WS/CLI CSV 和原始输入哈希。不得新增原始输入不存在的成功 PDU；完整、可修复及
复用流必须保留同样的消息多重集，复用的四条 SSH 连接保持四个独立 FlowID。
没有密钥就不声称加密正文恢复，缺字节且没有重传时仍明确不完整。

### 14.5 最终验证与 Windows 实际页面

最终 `mtu-final-tests.jsonl` 对 `bin-parser/...`、`pcaputil`、`shark-cli` 的九个
有测试包完整回归全部通过；目录/corpus/路线图/P1 分数一致性也在其中。共享控制
解析器另做 20 秒、双 worker Go fuzz，50886 次输入，无崩溃；MTU 修正后再次完整
跑全部选定包。封存输入保持原样。第 2 节的全仓库 Windows 特有限制不被本结果覆盖。

最终 EXE 在 CP936 原生 Windows 控制台跑 SSH、WireGuard、Telnet、NBSS：字段、
字节来源、会话消息/关联证据、80×30→180×48 缩放、q 退出及代码页恢复全部通过。
另实抓 HTTP/UDP 验证 SGR 鼠标、缓存/筛选、60×18 窄窗和退出恢复。最新文件在
`verified-native-ssh`、`verified-native-wireguard`、`verified-native-telnet`、
`verified-native-nbss`；PNG 是实际控制台单元格和颜色缓冲区的原样渲染，不是设计稿
或桌面像素截图。`verified-native-live` 保留最终 EXE 的实际实时抓包界面验收。

运行新版（cmd / PowerShell，从仓库目录）：

```text
.\build\shark-windows\yak.exe shark
```

最终 EXE SHA-256：`ae6013342d7edef96c4574522ac7514e2c0bc40641e4d81d4c73a21def6e4a46`。
本轮仍不宣称任何协议的所有版本、认证、扩展与配置均完备；具体缺口已逐项列在
14.1，SSH 混合方法体、加密重协商解密、生产 VPN/BMC 全会话等仍需独立新证据。

依据包括 [RFC4253](https://www.rfc-editor.org/rfc/rfc4253.html)、
[RFC7296](https://www.rfc-editor.org/rfc/rfc7296.html)、
[RFC1002](https://www.rfc-editor.org/rfc/rfc1002.html)、
[RFC2453](https://www.rfc-editor.org/rfc/rfc2453.html)、
[RFC5531](https://www.rfc-editor.org/rfc/rfc5531.html)、
[RFC2661](https://www.rfc-editor.org/rfc/rfc2661.html)、
[WireGuard 协议](https://www.wireguard.com/protocol/) 及 pinned wireguard-go 的
`device/send.go` 实际 MTU 填充行为、
[Intel IPMI v2.0 规范](https://www.intel.com/content/dam/www/public/us/en/documents/product-specifications/ipmi-v2-rev1-1-spec-errata-6-markup.pdf)。


## 15. 向 Wireshark 基础设施推进：统一可查询字段契约

### 15.1 剩余工作的优先级与验收标准

协议数量不能作为基础设施完成度。本轮检查到已有流重组、ProtocolSession、错误
分类、ByteSource、原生入口及简易字段筛选，但注册和字段语义仍分散，尚不能视为
通用 Wireshark 替代基础设施。以下作为后续顺序；第一行本轮已落地首批，其余仍未
宣称完成。

| 优先级 | 工作 | 验收标准 |
|---|---|---|
| P0，首批已实现 | 统一字段注册、类型、筛选与导出 | 稳定字段 ID、类型/位宽/枚举/来源；同一注册同时用于 SDK、CLI 和 TUI 字段筛选；精确整数、缺失与不确定语义有测试 |
| P0，下一阶段 | 通用 dissector 注册与分发表 | 能力、canonical rule/entry、端口路由、启发式证据、DecodeAs 和 SDK 清单形成明确关联；冲突、歧义、版本范围和禁用行为可检查；不能仅靠列表声明覆盖 |
| P0 | 通用连接/事务与恢复框架 | 捕获域、连接代际、方向/子流、事务 ID、上下文升级/失效、洞与重传事件统一；支持 UDP 会话代际和多路复用，错误能隔离到消息/子流/方向/连接 |
| P0 | 完整类型化字段树与来源图 | 不限于本轮的选定平面别名：重复字段保留实例/父子关系，位字段、TLV、重组/解密/解压来源可追溯；无对应原始字节时不能伪造 frame offset |
| P0 | 统一 Expert Info | 已有错误 taxonomy 扩展为稳定 diagnostic ID、类别、严重度、作用域、关联字段/包；展示“未捕获/不支持/未验证/验证失败/损坏/预算不足”各自含义 |
| P1 | 查询、UI 与应用 API | 字段自动补全、Apply as Filter、对象/事务/会话查询、稳定 JSON schema、版本和取消契约；现有 pcapdb JSON-path 查询需单独桥接，不能冒充已经统一 |
| P1 | 持续差分与跨平台门禁 | 固定输入哈希、独立应用 oracle、Wireshark 版本、差异分类及最小复现；协议/注册/字段契约变更必须带回归；Windows/Linux/macOS、长时间/资源与恶意输入验证 |

插件 SDK、完整协议树 API、完整 Wireshark display-filter 兼容、所有协议的共享事务
状态机都仍未完成。底层 UDP/TCP 入口、启发式顺序和原生 profile 的多数分发表仍有
手工注册，不能把这次字段注册表当作整个 dissector 注册框架已经完成。

### 15.2 本轮完成的字段底座

新增 `protocol_field_registry.go`，首批 **78 个注册字段**，schema version 1。
覆盖事件/连接元数据、DNS、TLS/QUIC、HTTP/2/3、gRPC、SSH，以及上一轮新增的常见
控制协议字段。不是全部 parser 字段；原始完整字段树继续经 GetFields/Decode 提供。

`RegisteredDisplayFields()` / `LookupDisplayField()` 返回独立副本，包含稳定名字、
标签、owner、类型/位宽、枚举及说明。`DisplayFieldValues()` 返回拥有的 occurrence
快照及逻辑 PDU ByteSource，标记 `event`、`pdu`、`context`、`derived`；根标量已知
范围指向逻辑 PDU，不指向凭空猜出的捕获帧位置。上下文、公钥指纹、验证结果和
TLS 1.3 解密内层类型没有伪造的线上偏移。

DisplayFields 兼容 map、typed filter 编译和 JSON `field_values` 由同一注册投影。
大于 53 位宽的无符号字段附 `exact_value` 十进制文本，让使用浮点 JSON 的客户端
也有无损读取途径；Go SDK、过滤比较及解码 worker 的 UseNumber 保留精确 uint64。
这不是把已有 JSON 的所有外层引用结构都迁移成字符串 ID；兼容 ByteSource/事件
封套仍保持原格式，客户端应遵循各自版本契约。

新增 CLI：

```text
.\build\shark-windows\yak.exe shark --list-fields
.\build\shark-windows\yak.exe shark --list-fields --json
.\build\shark-windows\yak.exe shark -r traffic.pcap --json --display-filter "wg.type == 4 and wg.counter >= 2"
```

`--display-filter` / `-Y` 在打开捕获/输出文件前校验，TUI 以可编辑 `fields ...`
搜索显示。它只影响展示及 JSON/文本行，`-w` 仍写出捕获 BPF 接收的所有包。现有
`--count` 和捕获 BPF 的含义不变。支持有界 and/or/not/括号、存在、比较、字符串
contains、地址 CIDR；不是完整 Wireshark 表达式语言。字段名有些沿用 Wireshark，
有些是 Yak 的 observed/verification 字段，不承诺所有字段字符串表示都与 WS 相同。

`DisplayFilter.Match(event)` 继续按单 PDU；`MatchEvents(events)` /
`EvaluateEvents(events)` 按调用者明确提供的同一包 PDU 集合。CLI/TUI 统一使用
后者，不能用相邻包或其他连接的事实凑出一次匹配。

### 15.3 修正的基础语义问题

- 旧投影把任意 Session 的 Authentication Verified 映射成 TLS 字段，也把任意
  Stream ID 映射成 HTTP/2。现逐字段限制协议/载体范围：WireGuard、IPMI、OpenVPN、
  SSH 不会因同名键成为 TLS，HTTP/3 stream 不会变成 HTTP/2。
- 缺少 DNS question type 曾导出 present nil，存在性判断可能误匹配。现在不存在
  就不发布；0、false 与缺失分开，`!=` 需要字段存在且所有 occurrence 不等，
  `not field` 才明确检查缺失。
- 类型与表达式由注册表约束。无符号整数拒绝负数、分数、溢出、带引号数值；
  numeric contains、非地址 CIDR、布尔排序在编译时拒绝，不能靠运行时猜类型。
- uint64 不能经过 float64 后冒充精确值。覆盖 2^53+1、2^60+7、MaxUint64、JSON
  往返和 worker 详情；不安全浮点、NaN/Inf、非法 UTF-8 等明确使投影不完整。
- 原 TUI 对每个 PDU 单独尝试整条表达式，同包 TLS 与 HTTP 的联合条件会漏掉。
  现在合并同包字段，保留每字段重复值语义；事件级 SDK 仍不自动跨 PDU。
- 字段遍历、occurrence 和 packet union 各有预算。截断/不可表示不会变成否定条件
  的“证据”；SDK 返回 Complete=false 与原因，CLI 明确报错，TUI 显示 unavailable。
- 实际差分发现 TLS 1.3 的 opaque_type 与 content_type 不能混用。新增
  `tls.record.wire_type` 表示外层头；已观察 TLS1.3 才提供 opaque_type；只有 AEAD
  成功且有内层类型才提供 TLS1.3 content_type。中途无握手的加密记录只提供 wire
  type，不猜版本和内层。TLS legacy record version 与协商版本继续分别登记。

### 15.4 验证、差异与运行页面

完整 pcaputil、shark-cli 回归通过；本轮没有改动协议解码器规则或封存输入。
类型筛选/投影另进行 20 秒双 worker fuzz，48868 次执行，无崩溃。独立测试还覆盖
不可表示标量、合并预算、IPv4-mapped IPv6 前缀、根字节区间边界及返回对象所有权。

`field_contract_compare.py` 复用先前实抓并带哈希的四组输入：SSH、WireGuard、控制
协议、TLS 六套件。**734 个唯一输入帧**；TLS 另带独立 keylog 回放一次，总基础
回放 938 帧，没有称为新增抓包。所有 JSON PDU 检查注册字段、类型位宽、精确值、
map/occurrence 一致及 namespace。16 组实际 CLI 字段筛选与 TShark 比较：

| 查询 | Yak / WS 匹配帧数 |
|---|---:|
| WireGuard initiation / response | 1 / 1，各一组 |
| type4 且 counter>=2 | 12 / 12 |
| type4 且不存在 sender 字段 | 16 / 16 |
| SSH KEXINIT / NEWKEYS | 10 / 10，各一组 |
| RIP response metric2 | 1 / 1 |
| IKE_SA_INIT / RPC CALL / L2TP SCCRQ / OpenVPN DATA / RMCP+ | 4 / 4，其余各 1 / 1 |
| 已知 legacy TLS application record | 21 / 21 |
| TLS1.3 opaque record，无 keylog | 39 / 39 |
| TLS1.3 已认证内层 application type，带 keylog | 21 / 21 |
| RIP version2，包括已知负例 | 2 / 3，保留下面的差异 |

唯一保留差异是 control-05 的第 14 帧：独立记录的三字节残缺 RIP，在显式 WS 选择
后仍暴露 version2 前缀字段；Yak 严格拒绝完整消息身份，故不发布该字段。差异及
原因原样记录，没有改成宽松识别或抹去帧号；其余筛选帧集合完全一致。TLS 第一次
差分暴露了 39 个语义不一致帧，修正 opaque/inner 字段后再跑通过。

证据根目录：`C:/tmp/yak-shark-infrastructure-20261024`。
`field-differential-final` 包含 schema、全部 JSON 行、每条查询的双边帧集合、差异
和输入 SHA256。`verified-full-tests.jsonl`、`scalar-limits-tests.txt` 及
`field-fuzz.txt` 保留测试证据；不能替代全仓库或其他 OS 的完整回归。

最终 Windows EXE 在 CP936 原生控制台运行 `--display-filter` 初始字段筛选：
WireGuard 12 条、SSH 1 条、无密钥 TLS opaque-only 33 条可见，且字段、字节来源、
会话/证据、80×30→180×48 缩放、q 退出和代码页恢复通过。输出为
`native-wg-filter`、`native-ssh-filter`、`native-tls-filter`，对应 PNG 是实际控制台
字符/颜色缓冲区渲染，不是设计稿。Worker 另核对 MaxUint64 原样显示。

最终 EXE SHA-256：`43a4831638b996ef834bdc04ca76a342eb929f9629c6ef72da47e3166e75453b`。
设计参照 [Wireshark dissector/field 注册](https://www.wireshark.org/docs/wsdg_html_chunked/ChDissectAdd.html)
和 [Expert Info API](https://www.wireshark.org/docs/wsdg_html/#ChDissectExpertInfo)，
但本轮落实的是 Yak 的首批字段契约，不把名称接近视为实现/行为完全兼容。
