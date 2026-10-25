# Shark 公开大捕获差分：已修复项与未通过项

日期：2026-10-25。PR：<https://github.com/yaklang/yaklang/pull/5353>。

**验收状态：未全部通过，PR 保留草稿。** 本轮已经实际下载两份公开大捕获，在本机
Windows 上完整运行 CLI、SDK 和 Wireshark，对比原始帧号、协议列、字段、连接分区和
完整 TLS 记录。不能把“可以打开文件”“标签数量相近”或“已有 YAML”当成全部协议
字段、事务和会话正确。Sharon 集合包含大量畸形/fuzz 输入，不能整体当成完整真实会话。

## 输入、工具与文件隔离

主输入是 [Wireshark SampleCaptures](https://wiki.wireshark.org/SampleCaptures)
链接的 Johannes Weber [The Ultimate PCAP](https://weberblog.net/the-ultimate-pcap/)，
20260710 版本。下载路径中的 2020/02 是服务器 URL，不是本次捕获版本：

| 输入 | 解压字节数 | 原始帧数 | SHA-256 |
|---|---:|---:|---|
| Weber The-Ultimate-PCAP.pcapng | 15,602,892 | 51,328 | `ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4` |
| Sharon ultimate_wireshark_protocols_pcap_220213.pcap | 86,779,688 | 328,127 | `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c` |

[Weber 下载](https://weberblog.net/wp-content/uploads/2020/02/The-Ultimate-PCAP.pcapng.gz)；
[Sharon 下载](https://wiki.wireshark.org/uploads/26c41b5ec1d89343e2979b73ec374bc9/ultimate_wireshark_protocols_pcap_220213.pcap.zip)。
第二个文件虽以后缀 .pcap 命名，实际也是 PCAPNG。主文件有 313 个接口，包含 Ethernet、
Linux SLL、Ethernet mPacket 和 Raw IP；时间跨多年且不单调，存在相同端点重用、多捕获域
和不完整会话。不能简单合并全部同端口数据。

工具为本机 Wireshark/TShark 4.6.9、Go 1.22.12。输入、压缩包、keylog、完整逐帧 JSON/CSV、
二进制和 Windows 控制台快照全部留在 `C:/tmp/yak-shark-public-corpus-20261024`；
测试日志留在 `C:/tmp/yak-shark-pr-20261024`。**这些原始文件均不提交仓库。**
仓库只提交审计源码、独立构造的回归测试和
[汇总统计](shark-public-corpus-20261025/counts.md)。两个完整原始输入合计 **379,455 帧**。

主输入带有一个 TLS Decryption Secrets Block。默认 reader 继续拒绝自动使用捕获内的
秘密。本次通过 editcap 显式提取公开输入附带的 keylog，双边使用同一份 keylog，Yak 指定
`--ignore-embedded-secrets --tls-keylog ...`。忽略 DSB 不代表信任它，也不把密钥转存到输出捕获。

## 对照口径

`public_corpus.py` 使用 TShark `-2 -n -o tcp.reassemble_out_of_order:TRUE`。
没有通过删包、DecodeAs、流量重放或协议标签归一化凑齐结果。所有原始帧都经过 CLI/SDK。
`frame-comparison.csv` 保留原帧号，供本机逐项复查；仓库内 CSV 只包含协议标签与数量。

- Protocol 列标签、完整 PDU 数、一个物理包参与了哪些 PDU，是不同口径。SSHv2/SSH、
  MDNS/mDNS、ISAKMP/IKEv1/IKEv2 等别名原样保留，不能把同名差额全部算成漏识别。
- `tcp_flow_id` 是原生 TCP 核心代际；`stream_id` 是有界 UI 预览，二者分别导出。
  UDP 的 WS `.stream` 是端点元组，不等于 QUIC CID 会话或应用事务 `conversation_id`。
- 同一 WS stream 可以跨接口/VLAN/镜像源；Yak 保持捕获域隔离。分区检查既检查域内拆分，
  也反查一个原生分组是否错误合并不同 WS stream。仅检查一个方向会漏掉连接粘连。
- ICMP 引用的原包不是当前承载流；畸形嵌套 IP 树没有地址时记为 oracle unavailable。
- 字段表只覆盖当前已注册投影中的指定字段。没有输出、值不等、未解密、未支持和
  重组归属差异分别保留，不宣称已比较 Wireshark 的全部字段。

## 已修复的实际问题

1. **输入打不开和链路类型错用。** 主文件原先在 DSB 处退出；混合 PCAPNG 把首接口
   link type 用于其他接口。增加明确的有界 DSB 丢弃策略、逐包 link 解码和逐 link BPF，
   PCAPNG 输出保留多接口/捕获域；经典 PCAP 不可表示时明确拒绝。
2. **mPacket 整批内层丢失。** 新增 complete-express MAC merge 前导与 Ethernet FCS
   校验，拒绝把片段/控制帧当普通 Ethernet。SDK、包列表、字节详情和连接预览共用入口。
   主文件 10,667 个 mPacket 帧不再整体漏掉。补齐 VLAN 802.3 长度形式、IPv6 内 IP、
   有界 GRE、ERSPAN III 的内层解码与稳定镜像源身份；变化时间戳不再拆分连接。
3. **畸形 TCP 导致崩溃。** 大 fuzz 文件原 CLI 在第 142,199 帧前后遇到不完整 TCP
   端点，调用端口格式化时 panic。现在短头不进入预览重组，后续帧继续解析；原始 328,127
   帧完整运行，长度计数保持一致。
4. **连接复用与最后 ACK。** 关闭后最终 ACK 不再产生新的 UI 预览连接，原生代际也保留
   关联但不重喂应用解析器。非对称 VLAN 捕获中的单侧 FIN、关闭后新 SYN-ACK，均可在
   观察到的新 ISN 下隔离下一代；活动连接中的冲突 SYN 仍不能换代。重传和新代际的
   正反例一起验证。
5. **PIM Register 隧道。** 两组 UDP 分区差异定位为 Register 内层未展开。增加 IPv4/IPv6
   同族、multicast 目标、头部/兼容全消息校验和验证及捕获域。Null-Register 的 dummy IP
   不作为应用流量。控制面内容仍未完整解析，不宣称实现了 PIM 路由状态机。
6. **QUIC Retry 与乱序恢复。** RFC 9001 Retry 完整性、原始 DCID、方向和 token 关联后，
   在认证成功时切换 Initial key；支持 Retry 后客户端 Initial 未捕获、服务端 SCID 再变化。
   不回滚 PN 空间，不把公开完整性密钥当成服务端身份认证。有效 coalesced PDU 后的异常
   尾部独立显示，不能覆盖已经成功解析的 PDU 状态。
7. **SSDP、RTSP 与可见性。** 实际 SSDP 多带空行时保留明确字段/符合性警告，仍拒绝正文
   和串接第二个消息。AirPlay GET/POST 扩展补齐规则、原生分发和保留 CSeq 的关联。
   已知完整 SNMPv3 加密信封、NTP 基础头加未支持扩展可以显示已验证的信封和限制，
   不因未解密就退成无说明的 UDP，也不伪造内部字段。
8. **Windows 查询卡顿及分片详情缺失。** 报文/PDU 历史查询加有界索引；索引不足回退
   完整查询，不能漏结果。物理帧查询可找到后续完成重组的 PDU，SDK 精确捕获域查询
   仍保持隔离。内层 IP 必须配内层端口；分片完成消息的逻辑端点和物理来源分别保留。
9. **能力清单虚报（历史问题，现已接入）。** 初轮 AJP/BGP/RTMP 只有 legacy YAML；后续已补原生入口、字段和连接状态，详见下文。初轮曾从
   `NativeProtocolProfiles` 的已实现列表移除。规则目录存在不再被拿来证明抓包入口已支持。

## TLS 与 QUIC 的核验边界

TLS 记录结果见 [独立记录汇总](shark-public-corpus-20261025/weber-tls-records.json)。
审计曾使用 TShark `-j "frame tls"` 精简树，发现它会漏掉嵌套在 RDPUDP/EAP 内的 TLS。
最终改用完整 JSON 树；**早期 2,639 条完全一致不能代表整个文件全部 TLS 已覆盖。**
按 TCP、RDPUDP、EAP/RADIUS 分开报告记录数；TCP 的记录字节/出现次数、导出头字段
另行核对，流分区和解密结果仍是不同验收项。

最终主文件得到 2,735 条完整 TLS 记录：TCP 2,639、RDPUDP 49、EAP/RADIUS 47。
TCP 的 2,639 条原始字节和重复出现次数完全一致。后续修复已接入 PEAP/TTLS 和
可靠 RDPUDP/2，不能将原先的 96 个缺失视图简单复制成 96 个独立 TLS PDU：

| 载体 | 原文件 WS 完整记录视图 | 原生独立 TLS 记录 | 原文件对照结果 |
|---|---:|---:|---|
| TCP | 2,639 | 2,639 | 字节及出现次数全部一致 |
| EAP/RADIUS | 47 | 47 | PEAP 两代、TTLS 一代，逐记录字节及次数一致 |
| RDPUDP2 | 49 | 1,718 | 原文件 48 个独立记录字节一致；另一个视图是同一通道重传；后续 WS 解析失步，见下文 |

完整记录 version/length/wire_type 共 **13,212** 项原生导出检查无差异。
RDPUDP2 的原始第 44,209 帧重传了 44,208 帧的相同通道数据；原生载体事件保留
`Retransmission` 和原载体 PDU 关联，TLS 接收器只消费一次。原文件 WS 又显示一次
TLS Application Data，此后在 44,213 帧起连续出现 `Ignored Unknown Record`。
原始差分仍保留：WS 多一个重复记录视图，原生多 1,670 个后续独立记录，不能称为
原文件全部记录次数一致。

另用独立 Python 工具从原文件的 RDPUDP2 线格式及 WS 通道序号字段提取载荷，按通道
序号检查连续性，确认 **105 次相同载荷重传**，消除重传后生成外部派生 TCP 捕获。
该捕获只用来绕过 WS 的 RDPUDP 重组路径，不替换原始 oracle、不冒充真实 TCP。
派生捕获中的 Wireshark TLS 解析与原生 **1,718 / 1,718** 条记录字节和出现次数完全一致，
两方向原始源块分别为 570、875 个。输入和派生捕获均不提交。
原始结果、派生结果、typed header 检查分别汇总在
[载体 TLS 检查](shark-public-corpus-20261025/carrier-tls-summary.json)。
同一捕获域内的 824 组 TCP stream、3,367 组 UDP 端点分区，在双向分区检查中没有
缺关联、额外拆分或合并。它不证明所有应用会话/事务已经等价，也不要求跨域合并。

IKE 的五项已登记头字段各检查 719 帧，无值差异。通用逐帧字段检查仍保留
本轮登记 RDPUDP 字段并接入 EAP TLS 后有 94 项差异：88 项 TLS record length 的贡献包/完成 PDU 集合归属，以及 6 项 DNS
名字显示差异（四帧空格转义，两帧非 UTF-8 标签）。6 项均保留，不以有损文本替换
消掉差异。原始/捕获长度双文件全部一致；字段缺失与未比较字段仍见 JSON 汇总。

主文件还有 77 组源/目的地址观察层差异：13 个 PIM Null-Register 的 dummy IP，以及
64 个 SSH 相关 GRE/ERSPAN 重组帧。当前物理帧头和 WS 虚拟重组层的端点并非同一层；
这部分需要继续补充分片续传的逻辑端点展示，不能把修改原始捕获端点当成修复。

公开 QUIC 样本存在四组 Retry 后 ClientHello 字节改变，以及 Initial 后附加未受保护的
零尾部。独立检查 TShark 解密出的 CRYPTO 内容后，保留这些诊断。
[RFC 9000 §17.2.5.3](https://www.rfc-editor.org/rfc/rfc9000.html#section-17.2.5.3)
要求 Retry 后保持原握手消息；不能通过清空 CRYPTO 历史消掉冲突。未捕获
NEW_CONNECTION_ID、无 keylog 的短头，不能仅凭原端点就确认新的 CID 仍属同一连接。

## 尚未通过的内容及修复顺序

1. **载体解析的剩余范围。** 本轮实现可靠 RDPUDP/2 的完整包头校验、选版、源/通道序号
   重组、回绕、重传去重、冲突隔离、缺口等待、先数据后 SYN-ACK 的恢复和新代隔离；
   TLS Hello 可跨载体块和 TLS 记录。PEAP/TTLS v0 使用已捕获 Start、State 回显、方法、
   EAP Identifier 与 RADIUS 请求/响应链，处理双向分片、ACK、乱序和重传；未见初始
   RADIUS 事务时不伪造关联。不放宽 NAS/Message-Authenticator 验证，类型 13 保留原有
   严格链，不重复输出。FEC 丢失恢复、lossy DTLS、PEAP/TTLS 内层认证还没有实现。
   RDPUDP 的 cookie、RADIUS authenticator、证书信任均不因此变成已验证。
   可靠接收与线格式依据 [MS-RDPEUDP2](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpeudp2/98f1d4e7-b9ee-4434-97c0-72c88fdfaf27)
   和[接收可靠性控制](https://learn.microsoft.com/de-at/openspecs/windows_protocols/ms-rdpeudp2/a1d1f871-c282-4f97-8f51-f821aca00c8d)；
   EAP TLS 分片参考 [RFC 5216 §2.1.5](https://www.rfc-editor.org/rfc/rfc5216.html#section-2.1.5)
   和 [RFC 5281 §9.2.2](https://www.rfc-editor.org/rfc/rfc5281.html#section-9.2.2)。
2. **真实未接入的协议。** 本轮已为 BGP/AJP/RTMP 接入下述限定范围的原生解析；HSRP/GLBP 和 BFD Control 已在后续接入限定范围；BFD Echo/MPLS、L2TPv3、IS-IS、
   以及部分路由/工业协议都有明确缺口。先按主文件数量与常用性补齐入口、完整消息校验、
   canonical 规则、字段注册和复用边界，逐项通过后才能加回能力清单。
3. **字段、事务与分片展示覆盖。** ClientHello 提供的套件列表与已协商套件不能混为一个上下文字段；
   多记录贡献包的 `tls.record.length` 逐帧集合差异要与完整记录对照并列。DNS 的非 UTF-8
   标签目前保留无损十进制转义，WS 的替代字符表示不同；不为了字符串相等丢失原字节。
   HTTP/SSH/SMB/NBSS 等未输出字段仍需逐帧归因，不以别名解释掉真正的缺失。
4. **fuzz 的严格拒绝与恢复。** 大量非法 IP/TCP 头、只有 ACK/RST 的孤立输入不分配应用
   流状态，WS 却可能建立 stream/暴露前缀字段。因此不能要求通过宽松接纳凑同样的
   stream 数；每类差异还需独立最小复现、隔离和恢复证明。
   TShark 自身也产生 1,150 行诊断，包括 TCP、IS-IS CSNP、TPM2.0 等 dissector assertion；
   它仍导出完整帧列表。这是 oracle 的已知限制，不能拿来一概解释 Yak 的差异。
5. **更完整的基础设施。** 当前注册仍不是统一 dissector/事务框架，尚无全部协议的
   完整字段来源图、完整 Expert Info 和 Wireshark display-filter 兼容。此前 TOP30
   固定正例通过不等于本次公开大捕获全部通过。

## 本机重现与验证

审计工具位于 `common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/`：
`public_corpus.py` 做完整逐帧差分，`tls_record_audit.py` 做独立完整记录对照，
`rdpudp_tls_oracle.py` 做独立有明确标记的派生载荷对照，
`summarize_public_corpus.py` 仅导出白名单汇总。输入和输出目录若在 Git checkout 内，
原始审计会拒绝运行。不要将外部目录整体复制进仓库。

```powershell
go test ./common/bin-parser/... ./common/pcapx/pcaputil ./common/yak/cmd/yakcmds/shark-cli -count=1
python public_corpus.py --capture <外部PCAP> --out <外部输出目录> --tshark <tshark.exe> --yak <yak.exe> --events <events.exe> --keylog <显式keylog>
python tls_record_audit.py <外部PCAP> <外部输出目录> <tshark.exe> <events.jsonl> <显式keylog>
```

Windows 以 CP936、浅色原生控制台启动实际 EXE，回放完整主文件；核对字段/字节来源、
会话/证据、80×30→180×48 缩放、q 退出和代码页恢复。PNG 由原生控制台字符/颜色
缓冲区原样渲染，不是设计稿。最终二进制哈希和数值以链接的 JSON 汇总为准。

验证记录：`corpus-final-full-tests.jsonl` 中 bin-parser 全部包和 pcaputil 通过。
CLI 唯一失败来自原库存测试仍要求三个 catalog-only 协议必须出现在 native 清单；
纠正该测试以核对规则实际存在、同时禁止虚报原生支持后，完整 CLI 重跑
`corpus-final-cli-tests.jsonl` 通过。没有降低任何解码、认证或恢复断言。
主文件和 fuzz 文件的最终 CLI/SDK 退出码均为 0，TLS 独立记录审计退出码为 0；
`windows-tls-final`、`windows-quic-final` 原生页面检查均通过，退出后输入/输出代码页均为 936。

CI：初始提交有一个 Shark QUIC 展示失败，已修复有效 PDU 被后续诊断覆盖的状态；
本机对应测试通过。另有 `common/mcp` 在线插件查询超时，与本轮协议改动分开记录。
不能把本机专项回归称为全仓库、全部 OS 或最新 PR CI 全部通过。

本轮外部工作目录为 `C:/tmp/yak-shark-carriers-20261025`。新增回归覆盖载体/TLS 双层
分片、乱序、16 位回绕、相同/冲突重传、捕获域、换代、资源释放和认证失败后恢复；
TLS 载体 fuzz 运行 23,178 次通过。RDPUDP/2 规则、嵌入 archive、catalog、roadmap、
原生入口与五项 typed 字段已经接通，条目仍标 partial，不冒充完整 RDP 应用协议。

Windows 原生页面复测发现固定 4,096 PDU 历史早于默认 20k 物理包窗口淘汰详情，出现
“TLS 标签仍在、字段只剩 UDP”的不一致。CLI 历史改为最多 80k PDU / 64 MiB，仍有界；
淘汰的已知事件明确显示详情不可用。递进路径保留真实载体，如
`Ethernet › IPv6 › UDP › RADIUS › PEAP › TLSv1.2` 和 `UDP › RDPUDP2 › TLSv1.2`，
TLS 子消息不会被外层优先级盖住，展开字节与重组字节也不会指向虚构的原包偏移。
RDPUDP SYN 线格式切换到 RDPUDP2 是同一载体的选版，不额外画成一层嵌套协议。
索引仍受字节和引用数上限约束；扩大窗口后的普通包关联不再因旧上限而逐包扫描全窗，
索引不足时仍完整回退，域隔离和多 PDU 贡献包查询断言均保留。

本轮分组回归：pcaputil 完整包通过（502.096s），随后新增 FIN、低预算暂存、跨 TLS
记录 Hello 与大窗口索引均通过专项回归；bin-parser 核心完整包通过（149.891s），
其余子包及完整 Shark CLI 也分别通过。冻结 corpus 的 616 项材料快照保持原哈希、
原捕获和精确行数，新增目录共 618 项；完整性测试逐一检查历史名字仍存在，不能因
新增两项就重写冻结 archive 或假装它已有新协议材料。新 RDPUDP 材料是独立 Go
构造边界测试和本报告外部公开捕获验证，不归入旧快照的覆盖率。

最终可执行文件和完整输出以本轮 JSON 汇总哈希为准。Windows 的 PEAP、RDPUDP2
原生页面检查通过；CP936 在退出后恢复。总体仍未全部验收：主文件保留 94 项逐帧字段
集合差异，fuzz 文件保留 39 项（35 项 TLS 长度归属、4 项 DNS 名字显示），大量畸形
输入的流分区差异及前述未接入协议仍需继续逐类分析，没有把它们标为完成。

## BGP / AJP / RTMP 原生接入与实际页面复核

本次追加结果见 [消息与故障汇总](shark-public-corpus-20261025/bgp-ajp-rtmp-summary.json)。
原始、派生捕获、密钥、TShark JSON、逐包导出、可执行文件、控制台快照均在
`C:/tmp/yak-shark-bgp-ajp-rtmp-20261025`，没有加入 Git。以下均为限定范围的
**partial** 能力，没有把这三个协议或 TOP30 整体宣称为完备。

### 接收、字段与连接语义

- **BGP4**：完整 16 字节 marker、消息类型与长度后才建立识别；支持 OPEN、UPDATE、
  NOTIFICATION、KEEPALIVE、ROUTE-REFRESH。校验可选参数、能力 TLV、扩展属性长度、
  AS_PATH 段、普通/扩展/大型 community、IPv4/IPv6 MP_REACH/MP_UNREACH 和 NLRI。
  AS4、扩展消息接收能力、ADD-PATH 按捕获的两个方向协商；缺失 OPEN 时不猜 AS 宽度，
  ADD-PATH 编码有歧义则保留原字节。同一 TCP 代际里重复 OPEN 会隔离协商状态。
  保留未知地址族和属性原值，不能据此宣称任意 AFI/SAFI 或路由策略语义已实现。
- **AJP13**：方向 magic、完整字符串及终止符、编码/文字 HTTP 头、属性、所有常见
  请求/响应/控制消息共同校验。无类型请求正文只在已观察请求或 GetBodyChunk 上下文
  中解释，正文长度开头 `0x0202` 不会误当 Forward Request。每条连接只有一个活动请求；
  EndResponse reuse、Content-Length 余量、拉取正文和 CPing/CPong 参与关联。重叠请求、
  提前正文 EOF 和禁止 reuse 后的新请求都不能借用旧事务。未知属性/编码不会盲猜。
- **RTMP v3**：完整简单握手或可验证的公开 digest 初始块建立识别，回显对照已观察
  的对端块；公开常量摘要不是对端身份认证。两个方向各自维护 chunk size，按 CSID
  保留 fmt0/1/2/3、扩展时间戳、32 位回绕与分块状态，支持交错消息和 Abort。
  完整控制消息后才改变后续分块大小。完整逻辑载荷具有独立的重组字节来源和贡献 PDU，
  命令/响应关联限定在同一 TCP 代际、方向、消息流和非歧义 transaction number 内。
  AMF0 对象/数组/引用/字符串/深度均有界，命令类型需完整解码后才显示 AMF0。
  控制、音视频基本头和 aggregate tag 有独立校验；RTMPE、一般 AMF3、共享对象事件和
  媒体编解码没有冒充完成。未知压缩头上下文不从端口推导。

新增六个显式 canonical 入口与原生共用 codec；旧默认规则仍选择原来的 BGP/AJP/RTMP
入口。完整回归确实发现了“新增 Package 子入口令旧 default 连续解析”的兼容问题，已
给旧默认入口补明确分发。`ParseStructured` 现在实际返回已校验字段 map，而不是仅有
原始字节；原始字段范围、连接观察与逻辑重组字节不混为原包偏移。

### 对照范围与结果

| 范围 | 对照结果 | 字节/字段检查项 | 未解释的字节/字段差异 |
|---|---|---:|---:|
| Weber 原文件 BGP | 原生 315；原始全文件 WS 302 条完整消息均匹配 | 3,242 | 0；另 13 条保留为跨接口重传视图差异 |
| Weber 按原接口隔离的 BGP 补充验证 | 接口 1/60/258 分别 12/12/291，合计 315 对 315 | 3,281 | 0 |
| Sharon 原文件 BGP | WS 303 条完整消息均匹配；原生另 12 条保留接口差异 | 3,245 | 0；另列 4 个无完整 envelope 的 fuzz 视图 |
| nDPI 真实 AJP | 原生 12 条；WS 13 个视图含 1 个重复视图 | 82 | 0 |
| Wiki 真实 RTMP | 4 段握手和 13 条完整逻辑消息逐一匹配 | 88 | 0 |

“检查项”同时包含完整字节/出现次数检查和实际有值的字段观察，不是全部协议字段覆盖率。
BGP 检查包括属性标志/类型/长度、OPEN/能力、AS4、ORIGIN、NEXT_HOP、MED、LOCAL_PREF、
MP AFI/SAFI、IPv6 next hop 和 IPv4/IPv6 NLRI 前缀/长度。接口隔离捕获只用于明确标记的
补充 oracle：原始完整 pcap 的逐帧统计始终保留，没有用分拆结果替换全文件差异。

AJP 补充捕获来自 [nDPI 固定提交](https://github.com/ntop/nDPI/tree/a915c7bceb96a4fbd6331fdbecdb50aedc8fceef/tests/cfgs/default/pcap)，
包含重传、乱序和 8009/8010 两个服务端口。WS 4.6.9 默认没有接入这个样本，因此只在
该补充实验显式使用 `-d tcp.port==8009,ajp13 -d tcp.port==8010,ajp13`；两个大原文件
没有使用 Decode As。AJP 的 wire null remote-host 保留为 null，WS 的空字符串显示单列，
没有为了字符串相同丢失语义。重组完成帧/重复视图差异也单列，不伪装为丢包或字段错误。

真实 RTMP 来自 [Wireshark Wiki 样本归档](https://wiki.wireshark.org/uploads/__moin_import__/attachments/SampleCaptures/rtmp_sample.tgz)，
解压为 8,808 字节、26 帧；nDPI 的 RTMP 文件含同一段交换及追加 fuzz，不能算第二条独立
真实会话。Sharon 的 RTMP 视图从 frame 37,334 才开始，缺少完整握手，不能把端口和
压缩头前缀视图算作已经验证的完整会话。对照使用完整 JSON 与 `--no-duplicate-keys`，
否则多消息包的同名 RTMP JSON 键会被普通 JSON reader 静默丢弃。

### 故障恢复、复用和拒绝

`observed_fault_corpus.py` 从 Wiki 原始报文产生明确标记的离线派生捕获，不发网络流量，
也不声称新抓到了五条独立真实流。新代际的 IP/TCP checksum 重新计算。

| 派生故障 | 原生完整逻辑消息 | WS 对照项 | 结果 |
|---|---:|---:|---|
| 交换原始第 13/14 帧 | 13 | 88 | 恢复，完整字节与字段一致 |
| 重复第 14 帧 | 13 | 88 | 无重复命令，完整字节与字段一致 |
| 丢失第 13 帧 | 8 | 57 | 只保留可完整观察的方向，缺口显式 incomplete |
| 截断第 14 帧，保留线上长度 | 8 | 58 | 不补造正文，缺口显式 incomplete |
| 观察到 RST 后同五元组、新 ISN 再交换 | 26，分属 2 条连接 | 176 | 无跨代际响应关联，完整字节与字段一致 |

独立 Go 测试还覆盖首消息各切分点、TCP 乱序/重复/RST 换代、方向性 ADD-PATH/AS4、
AJP 正文碰撞/复用/提前 EOF、RTMP 交错 CSID/单方向 chunk-size/Abort/扩展时间戳与回绕、
畸形控制后恢复、命令号复用、消息字节与返回字段所有权。状态化与 codec 联合 fuzz
运行 **35,210 次**通过，关闭后保留字节归零；另一次纯 codec fuzz 为 23,717 次。

### Windows 页面发现并修复的两个问题

1. 同一包包含 RTMP 回显、chunk 和 AMF0 命令时，旧详情优先级相同，命令被早期握手
   挤到下面。现在已重组应用消息优先，递进路径为 `Ethernet › IPv4 › TCP › RTMP › AMF0`，
   字段首先显示 connect、消息流、分块流和 transaction，外层和父 PDU 仍可查询。
2. 完整主文件的协议详情估算为 **121,847,203 字节**。60k 物理包窗口仍配固定 64 MiB
   历史时会淘汰 13,026 个早期 PDU，BGP 全部离开历史，造成“详情固定在旧 OPEN，筛选列表
   却为空”。现在启动窗口相应放大历史预算，60k 为 240k PDU/192 MiB，上限为
   256k PDU/256 MiB，默认仍为 80k/64 MiB。冻结列表会随历史版本变化重查迟到的完整 PDU；
   真正淘汰/超出快照预算时，字段筛选显式不可判定，不再显示确定的“无匹配”。

原生控制台验收还增加了“EOF 后必须仍有匹配行”的断言，不能靠旧固定详情蒙混通过。
最终实际页面为 BGP 20 行、AJP Forward Request 2 行、AMF0 connect 1 行；字段、字节来源、
会话/证据、80×30 与 180×48 缩放、q 退出与 CP936 恢复均通过。其他超过快照预算的
报文仍可使字段谓词部分不可判定，已知匹配行与该提示并列，不能宣称全局穷尽。
测试不重写旧冻结捕获的 metadata：这三项历史 `done` 分数与当前明确的 `partial` 范围
分别验证，限定范围必须有完整/畸形 wire 测试，其他状态、来源哈希和旧字段仍严格检查。

本轮完整 pcaputil 包通过（509.305s）；规则核心包在保留旧捕获 metadata 的兼容检查
修正后完整通过（137.509s），其余规则子包均通过。最终历史窗口/筛选修改后，完整 CLI
回归通过（2.222s），新增原生协议、字段、过滤与历史索引专项通过（0.725s）。两份完整
pcap 的最终回放都使用 JSON 汇总中相同哈希的实际 EXE/SDK；4,404 条 TLS 的 13,212 项
头字段检查及独立 RDPUDP 派生记录对照也再次通过，没有用旧二进制结果替代最终版本。

原始 94/39 逐帧字段集合差异已在下节逐条复核；77 项主文件端点观察层差异、大量 fuzz 流分区差异及
其他未接入协议仍未全部验收。以上新增消息校验通过不代表任意协议版本、所有字段或
全部 Wireshark 行为已经一致；PR 继续保留草稿。


## 字段差异逐条复核与路由控制原生接入

本次继续使用同一份 51,328 帧主文件和 328,127 帧 fuzz 文件，没有裁掉困难报文，
也没有把 Decode As 加到主对照。完整 CLI/SDK 均退出 0。
复现目录为 `C:/tmp/yak-shark-field-routing-20261025`；所有捕获、密钥、二进制、原始
JSONL、派生材料及控制台 PNG 留在该外部目录。

### 原始 94 / 39 项字段差异

[逐行证据](shark-public-corpus-20261025/field-differences-reviewed.json)保留每项原始帧号、
记录 SHA256、长度、贡献帧、字节收齐帧和核验方式，133 项均已复核，没有删除原始差异。

- 8 项 DNS 可读性错误已修复：空格和合法 UTF-8 在界面/`dns.qry.name` 可读显示。
  原 `Name` 与新增 `dns.qry.name.escaped` 保留转义；事务仍按展开后的原始名称字节关联。
  非法单字节 `e4` 不与合法 UTF-8 替换字符合并。主文件另 2 项非法 UTF-8 显示差异有意保留，
  避免为了文字一致丢失原始字节。点号、反斜杠、控制字符与 bidi 控制仍无损转义。
- 123 项 TLS `record.length` 差异均有完整记录字节证明，并非把长度改成 oracle 的帧集合。
  普通归属差异按方向、原生流、捕获域、实际贡献帧约束；36724、36744、36764 三帧的
  延迟批量视图还要求一个同向、同流、共享早期贡献帧且跨至目标帧的精确 sibling 记录。
  其中 1 项原始 RDPUDP oracle 中途失去 TLS 解码，用独立派生接收器的字节结果补充证明。
- 新增 `ByteSource.CompletedIn` / `pdu.completed_in`，页面显示消息字节收齐的帧与贡献包数。
  这不是“Wireshark 展示帧”或“后续上下文才可解码的时间”。克隆与 IPC 快照独立拥有引用；
  不完整、受限和无原始消息字节的事件不会声称已收齐。

修复后通用逐帧集合差异为主文件 **90**、fuzz **35**：88/35 项仍是上述 TLS 归属集合，
主文件另 2 项是非法 UTF-8 的无损显示。没有把这些原始数字归零或改写为标签一致率。
TLS 全量复核仍有 4,404 条原生记录、13,212 项 typed 头字段零值差异；TCP 2,639 条原始
完整记录一致。独立 RDPUDP 派生 oracle 仍为 1,718/1,718，缺失/额外均为 0，
105 条相同载体重传未重复送入 TLS。原始 oracle 的 49 条 RDPUDP 视图及其中重复视图仍保留。

### HSRP / GLBP / BFD Control

新增 `routing_control.yaml` 三个显式 canonical 入口，和原生 UDP 接收共用有界 codec。
旧规则入口保持其原有语义。目录、原生清单、端口提示、typed 字段与过滤器均已接通；
三个总体条目明确为 partial。历史 corpus 的 HSRP done、GLBP/BFD todo 评分保持冻结，
仅在独立正/反例验证限定入口之后接受这三个精确状态迁移，没有通用的 partial 豁免。

| 原始文件 | HSRP | GLBP | BFD Control | 字节及已登记字段检查 | 未解释的已选字段/字节错误 |
|---|---:|---:|---:|---:|---:|
| Weber | 941 原生 / 945 WS | 777 / 777 | 135 / 135 | 29,579 | 0 |
| Sharon | 764 原生 / 768 WS | 777 / 777 | 65 / 85 | 26,806 | 0；下列拒绝与未接入载体另列 |

[汇总](shark-public-corpus-20261025/routing-control-summary.json)包含逐字段次数、二进制和
输入哈希及原始例外帧清单。重复 TLV 字段按顺序和次数比较，不能用 set 消掉重复或错序。

- HSRPv1 校验完整 Hello/Coup/Resign，以及 opcode 3 的 Interface State Advertisement：
  其 length=14 包含四字节 TLV 头，保留真实样本的非零 reserved tail。HSRPv2 分别解析
  group/interface/text-auth/MD5 TLV、IPv4/IPv6、不同于 v1 的状态值与毫秒计时器。
  每份文件各 4 条六字节 interface-only 报文缺少先前完整发送者证据，严格拒绝建立新身份；
  已观察发送者后可识别该短报文，但不会继承组号或 VIP。捕获域、发送者、Identifier 和
  两分钟超时隔离，其他接口的未来时间戳不影响当前观察。
- GLBP 校验 owner/group、Hello、forwarder 和认证 TLV 的完整边界，IPv4/IPv6 地址长度、
  权重、虚拟 MAC、MD5/chain 长度；未知 TLV 原样保留。不同组或新 owner 使用独立上下文，
  未知 TLV 不能凭端口建立新身份；forwarder priority 不投影成 gateway priority。
- BFDv1 Control 校验长度、非零 My Discriminator/Detect Multiplier、状态和认证结构。
  反向源端口可以不同，通过端点/捕获域/观察到的 discriminator 对保守关联；Down bootstrap、
  discriminator 改变和超时产生独立代际。只有唯一已观察 Poll 才能关联 Final；中间丢失 Final
  导致重复 Poll 时保留歧义，不强行制造 response-to。历史乱序报文不消费未来 pending 状态。
- Sharon 的 20 条 BFD 差额逐一保留：8 条完整字节视图违反版本、长度、零 discriminator/
  multiplier 或同时 Poll+Final 的约束；2 条 oracle 没有完整协议 raw range；10 条不是 UDP
  接收路径，涉及 MPLS 等未接入载体。这些没有算作成功协议对齐。
  BFD Echo 载荷由实现定义，3785 端口没有冒充已解析的 BFD Control。

状态仅为对方报文中的 advertised 值；没有据此推断路由器选举结果、转发表、链路健康、
TTL 合规、密码校验或认证身份。协议依据：[RFC 2281](https://www.rfc-editor.org/rfc/rfc2281.html)、
[RFC 5880](https://www.rfc-editor.org/rfc/rfc5880.html)及
[Wireshark v4.6.9 官方 dissector](https://gitlab.com/wireshark/wireshark/-/tree/v4.6.9/epan/dissectors)。

### 页面、故障与剩余范围

Windows 用实际 EXE、CP936 和浅色初始控制台回放完整主文件，检查 HSRPv2、GLBP、BFDv1
的 typed 过滤、非空列表、字段/字节来源、会话/证据、80×30→180×48 缩放和退出后恢复 CP936。
递进路径保留真实 VLAN、IPv6、UDP 层。字段先显示状态、组号/VIP、discriminator 和计时器；
状态显示名称和原值，计时器区分秒、毫秒、微秒。重复 TLV 的 Priority/Weight 点击高亮各自
实际字节，内部范围索引不再显示成字段内容。

有界 codec fuzz **27,095** 次、关联隔离 fuzz **6,851** 次通过；另有每字节截断、认证长度、
错误控制头不污染状态、域隔离、标识复用、Poll/Final 丢失/乱序、资源容量和关闭归零测试。
完整包回归结果与本轮实际二进制哈希见汇总，不能把专项通过称为全仓库 CI 通过。

整体仍未全部验收。主文件 77 项端点观察层差异和 fuzz 的 6 组 TCP / 24 组 UDP stream
分区差异仍需独立证明；当前 UDP tuple 统计本来就不等于 Wireshark 的会话超时模型。
fuzz 中无效 SYN+RST 等报文的 flow ID 统计与实际被接收的会话字节也需区分，不能因为
计数相同就允许其改变已识别连接。MPLS/BFD Echo、L2TPv3、IS-IS、LDP 等尚未接入的
范围和既有字段缺失保留在后续工作中，本次没有宣称 TOP30 或所有核心协议完备。


本批完整回归：bin-parser 核心 171.068s、全部子包通过；pcaputil 完整包 481.410s 通过；
最终 CLI 完整包独立重跑 2.544s 通过。长命令开始后新增了页面格式化源文件，Go 最初
枚举的 CLI 文件清单不含它，导致该命令 CLI 构建出现 `undefined routingFieldText`；
最终源码独立重跑已解决，该失败和重跑分别保留在汇总，未宣称原长命令退出 0。
最终 Windows EXE 按相同源码重建，SHA256 与实际页面验收二进制完全一致。

## 重组传输头、递进路径与 Windows 子进程字段页

本批复现目录为 `C:/tmp/yak-shark-flow-endpoint-20261025`。原始大文件、完整 CLI/SDK
JSONL、TShark raw JSON、接口 226 派生文件、密钥、EXE 和控制台图像均留在仓库外。
最终完整主文件及 fuzz 文件分别仍为 51,328 / 328,127 帧；未改写原始 oracle。

- 修复网络重组完成、但当前包没有完整应用 PDU 时，列表仍沿用外层地址或 Fragment 的问题。
  主文件原 77 条端点分歧中的 64 条 GRE/ERSPAN TCP 已修复；其中 53 条原先显示 Fragment
  的 SSH 报文现在沿用已验证的连接身份，并明确说明本段没有完整解码消息。
  原始物理端点单独保留，不将隧道内地址写回原始捕获字节。
- 网络归一化保留有界、实际观察到的协议头路径，递进显示例如
  `Ethernet › IPv6 › GRE › ERSPANIII › Ethernet › IPv6 › TCP › SSH`。
  同时修复重组 DNS/SNMP 等重复追加 `IPv4 › UDP` 的错误，以及列表再次投影时
  丢掉“本包没有完整解码消息”说明的问题。
- 连接预览在原生 TCP 代际确定后接收拥有独立存储的归一化包。没有原生流关联的
  capture-backed 报文不能自行建立预览；被核心拒绝的冲突 SYN 不再提前重置预览。
  预览仍有自己的有界 assembler，此处没有声称已统一所有 TCP 字节状态机。
- Windows 实际运行又发现深度解码进程丢失逻辑 TCP 头，导致字段页回退成 Fragment。
  已补齐 IPC 元数据及独立头字节传递，检查头长/数据偏移及输入预算；快速字段页也使用
  相同观察。字段高亮使用独立逻辑字节来源，绝不伪造它在物理分片中的偏移。

[本批汇总](shark-public-corpus-20261025/transport-observation-summary.json)包含二进制、
输入、oracle、脚本和原生控制台缓冲的哈希，以及每条观察的帧号和头字节哈希。
主文件 **232** 条观察通过 **2,352** 项检查；fuzz 文件 **442** 条观察通过 **5,082** 项检查。
检查包括 TCP/UDP 完整头字节、端点、端口、TCP Seq/Ack/flags/载荷长度、当前贡献帧、
递进前缀及不得重复制造网络后缀，缺失和差异均为 0。fuzz 头专项使用页面修复前一版
CLI 输出；最终版相关头/路径/端点/预览关联投影逐条相等，汇总明确保留两个完整 JSONL
哈希和投影哈希，没有声称两个完整输出相同。

主文件余下 **13** 对端点分歧分别是 5 条 IPv4、8 条 IPv6 PIM Null-Register。
独立验证了外层 PIM 头校验和、Null 位和虚拟头的源/组地址：这些是通告 `(S,G)` 的
控制信息，不是实际传输连接。保留外层流量端点；8 条 IPv6 虚拟头缺少合规的后续 PIM
封装，不能据此创建内层连接。依据 [RFC 7761 §4.9.3](https://www.rfc-editor.org/rfc/rfc7761.html#section-4.9.3)。
PIM 的完整原生控制字段仍未接入，这 13 条没有被算作字段已全部支持。

### 实际 Windows 验收

最终 EXE `8e62f1a977687921d069fddf215b392e75eb21999baae123a415bdf7228783a4`
与冻结源码重建哈希一致。使用完整主文件，在 CP936、浅色初始控制台中定位原始
第 49707 帧：只有一条匹配行，标签 SSH，路径和字段页均保留归一化 TCP。
逻辑字节来源显示真实 32 字节头；其中源/目的端口为 22/47368、Seq 1614849292、
Ack 4237024444，与原始 TShark raw view 相同。物理帧仍为 156 字节，重组观察来自
2 个贡献帧、载荷 1428 字节，二者的范围没有混用。会话/证据、字节源切换、
180×48→80×30→180×48、退出和恢复 CP936 均通过。早期失败的 Windows 验收日志
也保留在外部复现目录，没有用 JSON 导出通过代替实际字段页验收。

完整 pcaputil 包回归 **431.582s** 通过；最终 CLI 完整包 **2.050s** 通过，包含分片/隧道、
借用存储复制、冲突 SYN、缺失原生关联、递进层去重、刷新幂等及 IPC 范围拒绝测试。
本批没有改动 bin-parser codec，也没有重复宣称运行了新的 bin-parser 完整回归。

### fuzz 分区的接收字节证明及新发现

[独立接收证明](shark-public-corpus-20261025/tcp-fuzz-partitions-reviewed.json)核查先前
6 组 fuzz TCP tuple 的全部 74 条原始 oracle 报文：44 条有原生观察关联，实际只接收
起始的 **8 块、544 字节**，并逐块匹配 TShark 原始 TCP 载荷字节与序号。
Wireshark 后续拆流部分进入重组器的新增字节为 **0**。触发控制包包含 SYN+FIN/RST
冲突和改变 ISN 的 SYN；观察 ID 不等于载荷被接收，不能为了统计一致放宽状态变更。
接口 226 单独导出的补充 oracle 仍有先前 6 组分区差异，不能解释成跨接口影响。

最终整文件回放结果也保留：[主文件数量表](shark-public-corpus-20261025/weber-transport-protocol-counts.csv)、
[fuzz 数量表](shark-public-corpus-20261025/sharon-transport-protocol-counts.csv)。
主文件 824 个原生 TCP 流、722 个有载荷的 stream/domain 对和 3367 个 UDP tuple 分组
仍未发现遗漏、拆分或合并；这不等于已验收所有应用会话。通用字段集合差异仍为 **90/35**，
已有 DNS/TLS 逐条解释不变，没有将原始计数清零。

重复回放发现新的稳定性问题：fuzz 原生 TCP 总数从先前运行的 840 变成最终运行的 843，
合并 oracle stream 的原生分区从 6 变成 5。最终端点差异为源 46,380 / 目的 46,309。
流缓存存在 `time.Now()` 壁钟过期路径，可能受离线回放耗时影响；变化原因尚需隔离复现。
独立 probe 的 6 组流 ID 不能直接冒充最终回放中的同号流，汇总明确记录该范围限制。
下一步优先修复离线时间语义及代际复现稳定性，再继续 UDP 会话模型、PIM/LDP/IS-IS/
L2TPv3、MPLS/BFD Echo 与剩余字段覆盖。整体目标保持未完成，PR 继续草稿。

## 离线回放的连接代际与时间语义

已经独立复现并修复上一节的壁钟问题：三条捕获报文间隔只有 1.234 微秒，显式空闲
资源限额为 5 毫秒；旧版仅因为处理暂停 25 毫秒，就将观察 ID 从 1 换成 2。
新版 1/2/4 worker 均保持原代际，实际接收 `ab`，处理速度不再成为 TCP 身份依据。

不能简单把默认 30 秒改成“按捕获时间切连接”。主文件同一原生流中有 **72** 次间隔
达到 30 秒，包括 **30** 次 BGP 保活及 TLS、HTTP、Syslog；这些间隔不是对端关闭的证明。
因此离线默认由 EOF、已观察 TCP 控制/代际和既有容量/字节/页预算收尾。实时抓包默认的
30 秒壁钟资源限额保持原有行为。调用方在离线明确指定正数 `IdleTimeout` 时，才按实际
捕获时间执行该资源限额；不能将资源淘汰解释为 TCP 已由对端关闭。

`ReplayPcap`、文件捕获及 CLI 文件输入自动使用离线策略；手工向 `PacketAnalyzer`
喂入预录报文可用 `WithCaptureReplayClock()`。显式离线空闲限额使用捕获域独立的
水位和索引堆：另一接口/隧道的未来时间不会过期本域；旧报文既不倒退 last-seen，
也不借用未来水位延长它。未知时间不伪造成壁钟观察。时钟条目随流淘汰回收，数量不
超过流缓存；全局容量和现有资源预算没有放开。并行 worker 保持其原有有界队列及流内顺序。

离线字节预览也不再用全局时间跳变提前跨过缺失字节或关闭其他接口的连接。它仍有
自己的有界 assembler，依赖 EOF、真实控制和既有预算收尾；没有宣称已统一核心重组器。
新增测试覆盖另一接口未来时间、先到后段再补前段、未知时间、容量清理，以及快速字段页
和深度解码边界。回放结束及容量淘汰在实际页面保持可见，不冒充完整 TCP 起点或真实 FIN。

### 完整文件的快慢回放证明

[本批汇总](shark-public-corpus-20261025/replay-clock-summary.json)保留输入、实际 EXE、
探针、原始导出和脚本哈希。对 **328,127 帧原始 fuzz 文件**分别直接回放，以及每 256 个
归一化报文额外暂停 50 毫秒回放，输入未裁剪、时间戳未修改。两份完整 TCP 探针输出的
SHA256 都为 `eaf1f48d144bc088ee1b2e7f614ac5b03c857a9894192efe5cbc653e8a35a754`：

- **840** 个原生 TCP 流、**7,700** 次观察回调；ID 和贡献帧/域关联逐项相同。
- **3,798** 个接收块、**2,048,129** 字节；序号、方向、原始贡献和逐块字节哈希逐项相同。
- 未归一化流 ID；分区和接收块有差异的流数均为 **0**。此证明只覆盖原生 TCP，
  不能代替 UDP 应用会话或全部协议字段的验收。

有完整回归负载的回放及最终整文件 CLI/SDK 对照，也都回到主文件 **824**、fuzz **840**
个原生 TCP 流，相同 stream/domain 指标不再受该处理耗时影响。fuzz 仍有 **6** 组 TCP、
**24** 组 UDP tuple 分区差异，未清零。[当前六组接收证明](shark-public-corpus-20261025/tcp-fuzz-partitions-clock-reviewed.json)
重新绑定本版流 ID：仍只接收 8 块、544 字节，oracle 后续拆流部分接收新增字节为 0。

通用字段集合差异仍为 **90/35**；原始列标签计数没有变化。主文件 722 个有载荷的
TCP stream/domain 对及 3367 个 UDP tuple 组仍无已测遗漏、拆分或合并。
BGP 原始完整 oracle 分别 302/303 条匹配，3,242/3,245 项字节及已选字段检查零差异；
仍保留 13/12 条原生消息未在原始跨接口 oracle 显示的差额和既有 fuzz 前缀拒绝。
TCP TLS 重验仍为 2,639/2,639 字节一致；4,404 条原生记录的 13,212 项头字段零差异。
原始 RDPUDP oracle 的 49 视图/48 独立匹配、后续丢失解析和先前独立派生证明的区别保留，
本批没有伪称重新运行了派生接收器。

完整 pcaputil 回归 **468.677s** 通过；最终 CLI 加上预览时钟测试独立运行 **1.977s** 通过；
时钟堆有界/域隔离 fuzz **1,568** 次通过。这些是所述范围的检查，并非所有协议已完备。

### Windows 页面与仍需修复的问题

实际最终 EXE `1869140154ccf1cb8be7c9a05bcf1fc46fb42e2cbbd81e12cc63b64cdd27b1df`
按冻结源码重建哈希一致。完整主文件在 CP936 控制台回放；BGP KEEPALIVE 和原始 SSH
第 49707 帧的字段、逻辑字节、会话/证据、缩放、退出和恢复 CP936 均通过。
BGP 预览的域预算淘汰及中途起点提示仍显示，未把资源限制包装成完整会话。

仍有一个实际页面问题：启动即带 `bgp.type == 4` 时，原始大文件在本轮 **90 秒 EOF**
验收期限内没有完成界面排空；失败输出保留。回放后使用文本筛选的页面路径通过，
这不代表启动字段过滤路径通过。当前字段谓词每次刷新重新读取/投影事件，下一步要
修复其缓存和失效范围，同时保留迟到 PDU、细节淘汰及 indeterminate 的正确语义。
其他原生协议和字段覆盖缺口继续开放，整体目标未完成。
