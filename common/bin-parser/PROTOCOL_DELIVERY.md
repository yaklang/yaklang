# bin-parser 协议交付标准

本文是 `common/bin-parser` 后续**逐协议交付**的验收标准。一个协议从 `todo` / `partial` 翻到 `done`，必须按本文打分；分数不够不得改路线图状态。

交付物是 YAML 规则 + 走 `parser.ParseBinary` 的测试，不是旁路实现。

**2026-09 修订（针对机械 A 返工）：**

- **去掉 leftover 不等于 Schema 25。** 无界 `*: raw` 清掉、再加几个 `uint8` 和一句 `if`，最高只到 15。25 分要求：leftover 合法 **并且** 有真实 TLV/列表循环 **并且** type/command 分发 **并且** 足够多的具名标量。
- **禁止 leftover 改名刷分。** `Body`/`Payload`/`Data`/`Rest` 从 `raw` 改成 `string` 仍算未完成 leftover。
- **P0 规则不得把 P1 卡抬到满分。** 同一 YAML 上的 P1 别名按该文件真实 leftover / 测试行数封顶。
- **P1 `done` 改为 B 级（≥ 75）。** A 级（≥ 90）留给 Wireshark 级主 PDU：引用样本上至少两类报文、具名字段断言、整帧。
- **L2 样本必须带出所断言的字段。** 只有魔数/版本的 RFC 附录 hex 不能当 Traffic 25。

---

## 1. 怎么用

每个协议单独开一张计分卡（见第 8 节模板），填完再改 `protocol_roadmap.go` / `protocol_catalog.go`。P0 已填结果见 [P0_SCORES.md](P0_SCORES.md) 与 `protocol_scores.go`。P1 已填结果见 [P1_SCORES.md](P1_SCORES.md) 与 `p1_scores.go`。

流程：

1. 列出该协议的 PDU / 字段 / `operator` 分支（第 6 节）。
2. 对照规范（RFC / MS-*/厂商文档）标出「必须命名的字段」和「规范允许的不透明块」。
3. 准备真实流量样本（第 5 节分级），写测试。
4. 硬门槛全过（第 2 节），再打分（第 3 节）。
5. 按第 4 节映射 `Status`。P0 只允许 A/B。

一次只做完一个协议。不要把若干协议各写半截然后一起标 `done`。

---

## 2. 硬门槛（任一不过 → 总分记 0，禁止 `done`）

这些不占分，是一票否决。

| 编号 | 门槛 | 判定 |
| --- | --- | --- |
| G1 | 规则文件存在，能被 `ProtocolCatalog.RuleFile` 读到 | `TestProtocolCatalogRuleFilesExist` |
| G2 | 测试调用已交付解析器：`parser.ParseBinary` / `parseRule` / `parseEthernet` | 禁止另写一套解码器当测试 oracle；禁止从 PDU 中段切开再喂解析器（除非测的就是该子节点） |
| G3 | 根节点最后一项不得是无界 `raw`（`MaxUint64` 会 panic） | 剩余数据必须有 `SetMaxLength`、`length-from-field` 或 list 循环退出条件 |
| G4 | 不得破坏已有黄金样例与 ABI | TLS 应用数据仍有 `Payload`；HTTP GET 不得被 TLS 吃成 GGET；TPKT 的 `TPDU` 保持 raw，供 `protocol-impl` 往返 |
| G5 | 至少一份 **L1、L2 或 L3** 样本（第 5 节），测试注释写清出处 | 仅 L4 手工拼包不能作为唯一样本。L3 只能作补充（Traffic ≤ 8）。**A 级**必须另有 L1/L2，且 hex/pcap 含本次断言的具名字段（不能只断言 Type/Length） |
| G6 | 至少一条 **截断 / 错误魔数 / 非法长度** 的 `parseMustFail` | 只有 happy path 不算交付 |
| G7 | 若协议走 TCP/UDP 端口分发，必须有 **Ethernet+IP+TCP/UDP** 整帧测试 | 只测 `parseRule(payload)` 不够 |
| G8 | 路线图与目录一致 | P0 翻 `done` 时 `ProtocolCatalog` 不得仍是 `partial`；`TestP0RoadmapCovered` 必须绿 |

G4 细则：TLS 只在 ContentType==22 时尝试 ClientHello；探测 ContentType 不得先 peek 再失败回退（会把 `GET` 变成 `GGET`）。

---

## 3. 打分（满分 100）

硬门槛通过后，按下表计分。每一项必须写**证据**（测试名、YAML 节点路径、样本出处），没有证据的分视为 0。

### 3.1 Schema 完成度 — 25 分

「完成」指规范 / Wireshark 里**主 PDU 的字段被命名并且测试读到了值**，不是 YAML 里有个同名节点，也不是把 leftover blob 换个类型。

机器封顶（`schemaCeiling`）按下表，**声称分不得高于封顶**：

| 分 | 必须同时满足 |
| --- | --- |
| 0 | 几乎没有具名标量 |
| 8 | 固定头有具名字段；变长区仍是 leftover blob（无界 `*: raw`，或 `Payload\|Body\|Data\|Rest\|Content: string`） |
| 15 | 有 type/command 分发；**或** leftover 已清但仍只有头字段 + 一句 `if`（无 TLV 列表、不足两个 `ProcessByType` 臂） |
| 20 | leftover 合法 **且**（真实 list 循环 **或** ≥2 个 `ProcessByType` 臂）**且** 具名标量足够 |
| 25 | leftover 合法 **且** 真实 TLV/list 循环 **且** type/command 分发 **且** 具名标量 ≥ 8。这是「主路径能当 Wireshark 用」的上限，不是「没有 leftover」的默认分 |

**明确不算 Schema 25 / 甚至仍算 leftover：**

- 把 `Body: raw` / `Payload: raw` / `Data: raw` 改成同名 `string`
- dummy `list: true` 循环一进来就 `break`，或从未 `ele.Process()`
- `raw,0` 占位、从不 `ProcessSubNode`
- `OpaqueRaw: "MsgGlobal opaque"` 给**尚未拆开的结构化字段**开绿灯
- P0 已有同一 YAML 就自动把 P1 卡写成 25

#### 3.1.1 规范不透明 vs 未完成 leftover

允许保持 `raw` 的（必须在计分卡 `OpaqueRaw` **写出字段名**，且该名属于载荷本身）：

- 加密载荷：TLS application-data、SMB3 transform 密文、SSH 加密包体、IPsec ICV/Ciphertext、WireGuard transport 密文
- 规范定义为 opaque 且本层不解码：DNS/NBNS RDATA、DCE/RPC NDR stub、ONC RPC Stub、TPKT TPDU（ABI）、RTMP C1/S1 Random、EAPOL Key Data
- 压缩/隧道内层交给另一个 dissector 的 payload（RTP 媒体 bitstream）

**不算**规范不透明、必须拆成具名字段：

- SNMPv3 HeaderData、LDAP BindRequest DN、DCERPC BindAck Sec Addr、Kerberos pvno/msg-type、AJP FORWARD URI、TDS LOGIN7 HostName、DHCP option 53/12/54
- RADIUS / LLDP / CDP 只解 Type/Length 把 Value 整段留下（除非 OpaqueRaw 标明该 TLV 的 Value 且规范就是不透明）
- HTTP/2 SETTINGS 只解第一对 Identifier/Value
- ClientHello 不读 Extensions
- `operator` 里 `return` 掉了后续字段，树上看不到名字

#### 3.1.2 长度必须约束

`Length` / `Remaining Length` / `Frag Length` 字段必须 `SetMaxLength` 到对应子树。声称的长度大于剩余字节时，应截断到剩余或失败，不得用「配置长度 > 父剩余」把整棵 TLS/RADIUS 解析打崩。

引擎注意：`SetMaxLength` 之后，节点自己的 `GetRemainingSpace()` 返回的是**配置长度**，不是「子字段吃完后的剩余」。列表循环用 `NewElement()` + `ele.GetMaxLength()==0` 退出；可选尾字段用 `this.Length() < this.GetMaxLength()`。

### 3.2 真实流量 — 25 分

| 分 | 标准 |
| --- | --- |
| 0 | 无样本，或只有现场随手构造、无出处 |
| 8 | 仅 L4：规范附录 hex / 本仓库 `protocol-impl` 已有 hex，且注释标明 |
| 15 | 至少一份 L1 或 L2，注释含来源（pcap 名、gopacket 测试文件、RFC 章节），且断言读到**应用层具名字段值** |
| 20 | L1/L2 覆盖请求**和**响应（或协议单向则覆盖主 PDU 两类），两类都有具名断言 |
| 25 | L1/L2 经 Ethernet+IP+L4 整帧解析，且整帧路径上断言打到应用层**命名字段**（不是只断言 Type/Length） |

`ProtocolCatalog.SampleFrom` 必须能回溯到测试里的注释。

### 3.3 测试完备度 — 20 分

测试必须驱动**已交付**的 `parser.ParseBinary`。

| 分 | 必须同时满足 |
| --- | --- |
| 0 | 无测试，或测试不调用 YAML 解析器 |
| 6 | 一条 happy path，断言 ≥1 个关键字段 |
| 12 | happy path + ≥3 条失败路径（空输入、截断、错误魔数/类型） |
| 16 | 12 分 + 端口分发整帧 + 可选字段缺省（长度为 0 的 list / 无 extensions） |
| 20 | 16 分 + 第 6 节分支表每一行都有对应 `t.Run` 或独立 `Test*` |

扣分项（每条 −3，该项最低 0）：

- 从 PDU 中间切开，绕过魔数/长度再喂解析器
- 把构造函数输出当「真实流量」且没有 L1/L2
- 只 `require.NotNil` 根节点，不读字段值
- 失败用例其实会成功（断言反了）

### 3.4 解析分支覆盖 — 20 分

先按第 6 节列出分支，再算覆盖率。

```
覆盖率 = 已有测试的分支数 / 应测分支数
```

| 分 | 覆盖率 | 额外要求 |
| --- | --- | --- |
| 0 | < 30% | — |
| 8 | 30%–59% | 主 PDU 至少 1 条 |
| 14 | 60%–84% | 请求/响应或主要 command 都有 |
| 20 | ≥ 85% | 每个 `panic("…")` 负向分支至少 1 条 `parseMustFail` |

「应测分支」定义见第 6 节，不得把未实现的 PDU 算进分母来做高覆盖率。分母 = **本规则 `operator` 里已经写出的**成功路径 + 显式 `panic` 路径 + 列表 0/1/N。未写的 PDU 记在计分卡「未实现」栏，拉低 Schema 分，不虚增覆盖率。

### 3.5 栈集成与引擎约束 — 10 分

| 分 | 标准 |
| --- | --- |
| 0 | 只能 `parseRule` 孤立 payload |
| 4 | YAML endian / 位宽正确；`TryProcessByType` 的目标是 yaml-root（与 Package 同级），不是 Package 内部嵌套类型 |
| 7 | 已挂到 `transmission_control_protocol.yaml` 或 `user_datagram_protocol.yaml` 的 `typeNameList`（或 L2/L3 的 EtherType/IP proto） |
| 10 | 7 分 + 默认启发式不误伤（例：443 上 TLS 优先于 HTTP；80 上 GET 不能变 TLS；22 上先试 `SSHPacket` 再试 ident） |

---

## 4. 等级 → 路线图状态

| 总分 | 等级 | `ProtocolRoadmap.Status` | `ProtocolCatalog.Status` | P0 | P1 |
| --- | --- | --- | --- | --- | --- |
| 硬门槛失败 | F | 保持 `todo` 或 `partial` | `partial` 或不要标 `new` 当完成 | 禁止 `done` | 禁止 `done` |
| 0–59 | D | `todo` / `partial` | `partial` | 禁止 `done` | 禁止 `done` |
| 60–74 | C | `partial` | `partial` | 禁止 `done` | 禁止 `done` |
| 75–89 | B | `done`，Notes 写清不透明块与未实现 PDU | `new` 或 `stable` | 允许 | **允许（P1 验收线）** |
| 90–100 | A | `done` | `stable` 或 `new` | 允许 | 允许；表示 Wireshark 级主 PDU，不是默认目标 |

P0 验收线：**硬门槛全过 + 总分 ≥ 75（B）**。

P1 验收线与 P0 对齐：**硬门槛全过 + 总分 ≥ 75（B）**。C 及以下一律 `partial`。A 级是质量目标，不是把 leftover 清掉之后的默认分。

翻 `done` 时 Notes 至少写：主 PDU 列表、样本出处、仍为 raw 的字段及原因。

---

## 5. 样本分级（真实流量）

优先级 L1 > L2 > L3 > L4。**交付至少 L2，P0 目标 L1。**

| 级 | 定义 | 例子 | 可否单独作为交付样本 |
| --- | --- | --- | --- |
| L1 | 从真实 pcap / 抓包文件抽出的帧字节，测试里原样保存 | Wireshark SampleCaptures、gopacket `layers/*_test.go` 里标注 pcap 名的 `[]byte`、本仓库 `protocol-impl` 捕获 hex | 是 |
| L2 | 规范或实现文档中的**完整报文 hex**（不是自己按字段拼的），且 hex 里含本次断言的字段 | RFC 1157 SNMP GetRequest community `public`、RFC 6455 §5.7 WebSocket `81 05 Hello`、RFC 2132 option 53 DHCPDISCOVER | 是 |
| L3 | 用 gopacket / 标准库 **Serialize** 生成、字段符合 RFC 的包 | `layers.DHCPv4` Discover | 只能作补充，不能当唯一样本；Traffic ≤ 8 |
| L4 | 测试里 `make([]byte)` / `binary.Put*` 按规范拼的骨架 | SMB2 sync header builder、NTLMSSP 手工 type1 | 只能测分支/失败路径，**不能**当「真实流量」 |

要求：

- 字节数组上方注释必须写：来源文件或 URL、pcap 包序号或 RFC 章节、这是什么 PDU。
- L1 帧应保留以太网头；用 `parseEthernet` 走到 `IP/TCP|UDP/<Proto>`。
- 同一协议至少覆盖一种「对端会看到」的方向（客户端请求或服务器响应）；有握手的协议两边都要有。
- **只有魔数/版本的短 hex 不算 L2 主样本**（例如只断言 `Type==1` 的 4 字节头）。

---

## 6. 解析分支怎么数

对目标 YAML（含它 `import` 的文件）做一张表，一行一个分支。

计入分母的：

1. **PDU 分发**：`if cmd==` / `if ptype==` / `if first==` / `switch` 的每一臂（含 default/`panic`）。
2. **记录类型**：TLS ContentType 20–24、HTTP/2 frame type、MQTT packet type 等。
3. **列表基数**：0 个元素、1 个元素、≥2 个元素（TLV/SETTINGS/DHCP option/SMB dialect）。
4. **可选块**：有/无 Cookie、有/无 Extensions、有/无 plugin name。
5. **传输封装**：UDP 裸 PDU vs TCP 带长度前缀（KerberosTCP、MySQL packet、TDS header）。
6. **负向**：每个 `panic("proto: …")`。
7. **端口分发**：该协议在 TCP/UDP `typeNameList` 里出现的每个端口组合，至少一条整帧。

不计入分母（单独记「未实现」）：

- 规范有、YAML 完全没写的 PDU（例如 SMB2 未写的 command）。
- 加密后不可见的内层。

覆盖判定：该分支有测试**读到了该臂产生的命名字段**，或负向分支 `parseMustFail`。仅仅「包能 parse 成功」不算覆盖了内部臂。

### 6.1 最低分支集合（所有协议）

即使 YAML 很短，也至少要有：

| 分支 | 测试 |
| --- | --- |
| 合法主 PDU | 字段断言 |
| 空输入 | fail |
| 截断到头字段中间 | fail |
| 错误魔数 / 非法 type | fail |
| 长度字段 > 实际剩余 或 < 头长 | fail |
| 整帧 + 端口 | `ipv4TCPFrame` / `ipv4UDPFrame` / 真实以太网帧 |

---

## 7. YAML / 引擎约束（写规则时必须遵守）

这些是历史踩坑，违反会直接打掉 Schema 或集成分。

1. `TryProcessByType("Foo")` 的 `Foo` 必须是 YAML **根级**节点（与 `Package` 兄弟），写在 `Package` 里面会报 `not found root node`。
2. 列表元素模板用 yaml-root 类型（DHCP `Option`、RADIUS `RADIUSAttribute`、HTTP2 `HTTP2Setting`）。
3. 不要在 `SetMaxLength` 之后对**子节点**调用 `GetRemainingSpace()` 判断「还有没有数据」——子节点定长类型在剩余 0 时会 `length over max size 0`。用父节点 `Length() < GetMaxLength()`。
4. TCP 默认启发式是 `TLS, HTTP, SSH, Redis, MQTT, FTP, SMTP`。新协议必须进端口分支，否则非标准端口测不到。
5. 端口 22：先 `SSHPacket` 再 `SSH`，避免 ident 规则吞掉二进制包。
6. 端口 80/8080：HTTP 必须能赢 TLS；端口 443：TLS 优先。
7. `protocol-impl` 依赖的节点形状不要改（TPKT：Version / Reserved / PacketLength / TPDU raw）。
8. 最后一个无界 `raw` 禁止出现在 root。

---

## 8. 单协议计分卡模板

复制到 PR 描述或 `p0_*_test.go` 文件头注释。打分人填「得分 / 证据」。

```
协议：
规则文件：
规范：
优先级：P0 / P1 / …

硬门槛 G1–G8：□全过  □失败（停）

A. Schema /25
  主 PDU 列表：
  已命名字段：
  仍为 raw 的字段及理由：
  得分：

B. 真实流量 /25
  L1：
  L2：
  整帧测试：
  得分：

C. 测试完备 /20
  Test 函数：
  fail 用例：
  得分：

D. 分支覆盖 /20
  应测分支数：
  已覆盖：
  覆盖率：
  未实现 PDU（不进分母）：
  得分：

E. 栈集成 /10
  TCP/UDP 端口或 EtherType：
  yaml-root 类型：
  得分：

总分：  /100    等级：A/B/C/D/F
拟写入 Status：
Notes：
```

### 8.1 分支表模板

| ID | 分支 | YAML 位置 | 测试 | 覆盖? |
| --- | --- | --- | --- | --- |
| B01 | | | | |
| N01 | panic: … | | parseMustFail | |

---

## 9. 示例（RADIUS，仅作格式示范）

不是永久分数，后续改规则要重打。

| 项 | 证据 | 分 |
| --- | --- | --- |
| G1–G8 | `radius.yaml`；`TestRADIUSAccessRequestFromCapture` 走以太网；截断 `pkt[:22]` fail；UDP/1812 | 过 |
| Schema | Code/ID/Length/Authenticator + Attributes **list**（5 个 attr） | 23 |
| 流量 | L1：gopacket `radtest.pcap` Access-Request / Access-Accept | 25 |
| 测试 | `TestRADIUSAndEdges` + 真实帧；非法 code / length<20 | 18 |
| 分支 | code 1/2/3；0 attr / 1 attr / 5 attr；截断 attr | 18 |
| 集成 | UDP 1812/1813/1645 | 10 |
| **合计** | | **94 A** |

扣 2：厂商 VSA 内部结构未展开（外层 Type/Length/Value 已命名，属可接受不透明）。

---

## 10. 与现有文件的关系

| 文件 | 角色 |
| --- | --- |
| `protocol_roadmap.go` | 全量 backlog 与 `done/partial/todo` |
| `protocol_catalog.go` | 已有 YAML 的规则路径、`SampleFrom` |
| `p0_more_test.go` `TestP0RoadmapCovered` | P0 不得残留 partial/todo |
| `p0_real_samples_test.go` | L1/L2 样本应放这里或同风格文件 |
| `protocol_support_test.go` | `parseRule` / `parseEthernet` / 整帧 helper |
| `protocol-impl/` | 仅 TPKT/NTLM/BER 等 ABI 消费者；**不能**代替 YAML 测试 |

新协议默认：YAML 放 `rules/` 或 `rules/application-layer/`，测试放 `p0_*_test.go` 或 `<proto>_test.go`，样本注释写出处，计分卡过线再改 Status。
