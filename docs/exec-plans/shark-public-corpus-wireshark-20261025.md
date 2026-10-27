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

## 字段过滤的有界缓存、缺失语义及新抓真实流量

上述启动字段过滤问题已修复。[缓存与完整文件汇总](shark-public-corpus-20261025/field-cache-summary.json)
记录实际二进制、测试及未归一化结果。过滤缓存按同一 history、谓词代次、初始 PDU
关联和贡献引用版本失效；版本使用历史到达次序而不是 PDU 数字大小。迟到贡献、
初始 ID 后到、重复 ID 的最老保留绑定变化、细节淘汰都能失效。索引超预算时保守
退回全局版本，版本在求值前读取，避免并发到达被错误吸收入旧结果。稳定刷新和
另一包的无关到达均为 **0 次分配**，不再复制完整字段树。

同时补了正确性边界：历史已淘汰、重复 ID 实际属于另一贡献帧、超过 **128** 个 PDU
的字段并集，以及初次查看时已没有可证明保留的贡献，都不能变成否定谓词的命中。
已学到的贡献 ID 在有界包缓存中保留；缺失和并集截断明确为 indeterminate。
无 history 的调用方可变事件不使用该缓存。每包已有 256 KiB 快照预算保持不变。

实际 EXE `f420860b55bbfbfdb6260eaeccf2d16a25b80eeeebf222af679cc5b9e7427d64`
与冻结生产源码重建哈希一致。完整 **51,328 帧**以 `bgp.type == 4` 启动：195 条可见，
首条原始第 79 帧，无 UI skipped。EOF、字段、字节源、会话、证据、180×48→80×30→180×48、
退出和 CP936 恢复的整套实测耗时 **18.262 秒**；旧版在 90 秒 EOF 期限内未完成。
截图来自原生字符/颜色缓冲，未重排页面。历史细节不全的黄色提示仍保留。

完整 pcaputil / CLI 回归分别 **443.276 / 2.262s** 通过；历史版本与快照一致性 fuzz
**1,579** 次通过。再次回放两个原始大文件，列标签、原生事件状态、端点、stream/domain
分区与上一版最终结果逐项相同：原生 TCP 仍为 **824/840**，预览为 **808/735**，
通用字段集合差异仍为 **90/35**。没有用缓存修复宣称补齐其他协议字段。

### 新抓 Windows TLS/QUIC 与十三种故障变体

[真实流量汇总](shark-public-corpus-20261025/fresh-live-field-cache-summary.json)来自新开的真实
本机 TCP/UDP 服务和 Npcap：**679 帧、1,066,130 字节、45 项应用操作、内核丢包 0**。
实际 QUIC 代理丢弃 8 包、延迟 14 包。端口和证书/密钥均本轮生成，BPF 只抓测试端口；
TShark 的随机端口 Decode-As 仅取自拥有服务的 manifest，明确是补充验收范围。
没有把此抓包或密钥放进 Git。

- 基线 **220/220** 条 TLS 完整记录按 oracle TCP 流、方向端点和贡献帧逐条匹配字节，
  660 项类型/版本/长度字段正确；不是仅比较记录总数。AEAD 验证单独检查。
- **10/10** 条 gRPC 消息的载荷哈希与 Wireshark 及独立应用操作相同；HTTP/3 的
  **12/12** 个路径、**27/27** 段逻辑 DATA 哈希相同。Wireshark 额外显示的 2 次
  STREAM DATA 重传保留在逐包 CSV，逻辑交付只计一次。
- 从本次新抓文件明确派生 13 个故障文件：TCP/QUIC 乱序、丢失、重复、snaplen、
  截断后重传、TLS 密文翻转、缺 ClientHello/Initial、未知 CID、第二代 SYN 缺失。
  每个变体保留原始帧映射与哈希，未伪称这些故障文件本身是独立现场抓包。
- TCP 乱序/重复/截断后重传均保留全部 10 条 gRPC；真丢失或未修复截断时双方均为
  9 条，反方向完整内容仍可解析。TLS 密文翻转没有产出未经验证的明文。
- 13 个变体的完整 TLS 记录按流和方向仍与 oracle 相同；缺 ClientHello 为 203 条、
  TCP 丢失/未修复截断为 213 条，其余为 220 条。缺失记录没有补造。
- QUIC 缺 Initial 时双方 HTTP/3 路径及 DATA 都为 0，诊断保持可见；未知 CID 不泄漏
  旧身份。第二代 SYN 缺失时，RST 后没有继承旧 TLS 连接协议。

逐包列标签数仍不同（包括 ACK、继承和逻辑消息/重传视图），原始计数留在汇总中。
本轮核对限定于上述消息字节、头字段、应用交付、归属和故障恢复，不能等同于所有
TLS/QUIC 特性或全部协议完备。PIM、LDP、IS-IS、L2TPv3、UDP 会话模型和剩余 fuzz
差异继续逐项处理，PR 保持草稿。

## LDP 发现报文：原生分发、完整消息边界与递进字段

[本项汇总](shark-public-corpus-20261025/ldp-discovery-summary.json)保存逐条拒绝/范围外视图、
精确输入/输出/实际二进制哈希及未归一化差额。

旧的 `extended_protocols.LDP` 只检查 PDU 外头，将消息体作为 raw，未接入原生 UDP。
新增显式 `ldp_discovery.LDPDiscoveryFields`，和原生 646/UDP、显式 UDP Decode-As
共用有界 codec；旧入口保持兼容。目录和 roadmap 都明确为 **partial**，没有改封存
语料的历史 todo 评分。该历史差别只有在实际运行本项完整/恶意边界测试后才被允许。

发现报文检查完整版本/长度、LSR/标签空间、每个 Hello 消息长度/ID、首个必选
Common Hello Parameters 和每个 TLV 的边界/固定值长度。越界、残缺或重复必选参数
不能凭端口或前两个字段识别；限制为初始化前 PDU 长度及集合预算。未知 TLV 保留
U/F 和拥有的原始字节，不解释扩展内容或对端接受结果。支持通告的 IPv4/IPv6 传输
地址、配置序列、保持时间、Targeted/Requested 以及 GTSM 位。

保持时间明确是通告值，0 的缺省和 65535 的无限值分别解释；GTSM 通告位不等于
已实施 TTL 防护，Targeted Hello 下该位的忽略语义保持可见。Hello 不创建 TCP 会话，
相同 UDP tuple 换 LSR/标签空间或省略地址时不继承旧参数，残包也不借用先前身份。
依据 [RFC 5036](https://www.rfc-editor.org/rfc/rfc5036.html) 和
[RFC 6720](https://www.rfc-editor.org/rfc/rfc6720.html) 的发现报文定义；没有把此范围
扩张为完整 LDP 邻接、标签库或路由器状态机。

两份完整大文件分别匹配 **290/290** 条原始发现 PDU：每份 **4,930** 项完整字节、
有序关键字段及贡献域检查，再加 **290** 项 CLI 原始帧和递进路径检查，合计每份
**5,220** 项，额外原生 PDU 为 0。每份列标签只发生 `UDP -290 / LDP +290`；原有
TCP/其他协议数量、824/840 原生 TCP 流及 90/35 通用字段集合差异保留。
主文件另外 **16** 条、fuzz 文件另外 **615** 条 TCP LDP 视图仍是下一项原生缺口。
fuzz 文件保留 **387** 条 UDP 拒绝：380 条 PDU 长度不完整/不一致、7 条超出所选
发现字节边界。这些都没有算作成功匹配或静默删除。

实际 Windows 完整主文件以 `ldp.msg.type == 0x0100` 启动，290 条可见、首帧 #5，
列表显示 LSR、Link/Targeted Hello、通告保持时间和传输地址；层级为 Ethernet ›
IPv4 › UDP › LDP。原生字符/颜色缓冲曾发现重复的 Packet Name 摘要，已修复并重跑。
字段/字节源、会话/证据、缩放、退出及 CP936 恢复通过；嵌套消息和 TLV 的高亮范围
使用各自的完整 PDU 坐标，有单独回归测试。认证和已建立会话没有从 Hello 推断。

最终 Windows EXE `2bab63d8ec2ad3bdf05aca661585307e8f55ed49816ad4694beb6d92f4870c32`
与源码重建一致。完整文件 SDK 的解析版本也与最终源码重建哈希一致；整文件列计数
来自修摘要前的 CLI，最后这项摘要修复由最终 Windows 全帧页面和 CLI 测试验收，
两种二进制的哈希均保留，未混写为同一次运行。完整 bin-parser **154.243s** 及全部
子包、pcaputil **448.619s**、CLI **2.051s** 通过；最后新增的 IPv6/缺省/Targeted GTSM、
显式选择及展示测试又分别在三个包 **0.661/0.605/0.620s** 通过。
发现报文边界 fuzz **17,167** 次通过。TCP LDP 的完整消息重组、观察到的 Init/KeepAlive、
连接复用隔离及 FEC/标签字段是下一项工作，不能用发现报文的验收替代。

## TCP LDP：完整 PDU、连接观察与逐消息递进展示

[TCP LDP 汇总](shark-public-corpus-20261025/ldp-session-summary.json)补上了上述基础 TCP 范围。
目录、显式 `ldp_session.LDPSessionFields`、原生流分发及类型化字段共用有界完整报文解析：
Initialization、KeepAlive、Notification、Address/Withdraw、Label Mapping/Request/Withdraw/
Release/Abort 的消息和 TLV 边界，以及 IPv4/IPv6 Prefix FEC、Generic Label、地址、路径、
状态码和引用消息字段。未知消息、未知必选参数及未选择的 FEC/ATM/FR 内容保留原字节，
不会宣称已理解语义。无握手的完整中途 PDU 可以解析，但不会凭端口或标签消息补造协商。

连接状态只取当前已接收 TCP 代次：两个方向的 Initialization 和 KeepAlive 都被观察后，
才发布共同最大 PDU 长度与保持时间。发送/接收 LSR 标识冲突、重复 Initialization 会隔离
协商；fatal Notification 关闭协议上下文。完整 PDU 内的坏语义可以明确拒绝后继续检查下一个
已知 PDU 边界，不会跨过 TCP 缺口扫描伪造恢复点；原生重传接收器负责修补真实缺失字节。
这仍不是完整的标签数据库、标签请求响应状态机、认证邻居或已安装转发表。

本次实际发现并修复了三个问题：

- 首个中途 PDU 超过通用 64 字节探测窗口时会提前拒识。现在只在服务提示和合法有界长度下
  延长等待，身份仍须完整消息及必选 TLV 验证；64/80/末字节等切分有失败前和修复后日志。
- `ldp.msg.tlv.fec.pfval` 原先误投影为 CIDR，Wireshark 此字段只含地址。现在地址、前缀长度
  和页面 CIDR 各自正确，地址与标签的高亮均指向自身所属完整 PDU 中的原始范围。
- 完整坏 PDU 的错误原先缺少消息作用域，导致对端后续已知边界也停止解析。现保留拒绝和
  协商隔离，同时允许检查后续完整消息；新的 TCP 代次不能继承这些状态。

### 原始大文件与接口补充证明

最终两个完整文件均从 290 个 UDP 发现报文增加到 **322** 个 LDP 列报文，变化严格为
**TCP -32 / LDP +32**；原有 TCP 流 **824/840**、预览 **808/735**、通用字段集合差异
**90/35** 以及全文件流分区指标均与上一批相同。没有用新协议标签消除原有未覆盖项。

原始跨接口 Wireshark oracle 每个文件的 16 个完整合法 TCP PDU 都逐字节、逐出现次数匹配，
每个文件 **420** 项已注册有序字段/域/连接归属检查零差异。原生各为 32 个 PDU、4 个流；
完整 oracle 仍受跨接口去重影响，额外 16 个原生 PDU 的原始差额没有删除。另从真实接口
**1 和 60** 导出补充文件，各 38 帧、16 PDU、2 个连接，各有 420 项检查；32 个补充 PDU
全部按字节哈希、原始贡献帧及接口绑定回原始文件的 32 个原生 PDU 和 4 个流域映射。
补充证明与原始计数分开列出，不把两个接口的结果替换原始 oracle。

Sharon 的 615 个 TCP LDP 列视图中，Wireshark 展开了 **1,540** 个 PDU 视图。除上述 16 个
完整合法匹配，剩余 **1,524** 个逐条保留：1,028 个没有完整选定 v1 PDU 外框，472 个消息
长度越过完整 PDU，24 个是未选择的消息类型。独立审计先检查边界和必选结构再归类，
并保留首次出现的差异输出；24 个范围外视图不被称为全部已修复或全部恶意报文。

### 丢失、乱序、残包、复用与实际 Windows 页面

从真实路由器的单接口 SYN 起始连接明确派生 **7** 个离线故障控制文件，所有修改、原帧
映射、时钟策略和哈希留在外部 manifest。基线/切分乱序/重复/截断后重传均匹配 **9** 个
完整 PDU；真实缺中段和未修复 snaplen 均匹配 **6** 个，并保留两个原生 incomplete 诊断，
反向完整内容不受影响。RST 后同 tuple、新 ISN、相同消息 ID 的两代连接匹配 **18** 个，
首 PDU 没有旧协商字段。共 **2,276** 项字节和已选字段检查通过，匹配连接的双向流分区
无拆分/合并。这些是原始真实字节的离线变体，未伪称新抓到的现场 LDP。

最终 Windows EXE `63c466e071176315373735172bad3fc43550dea748df4c60c6ade4164f71e856`
与冻结生产源码重建哈希相同。完整 51,328 帧以 `ldp.msg.type == 0x0400` 启动，4 条可见、
首帧 #979，无 UI skipped。实际字段页逐层显示 Ethernet › IPv4 › TCP › LDP、Address 和
多个 Label Mapping、FEC 及标签；标签 3 明确标注为 **Implicit NULL (advertised)**，
不冒充已安装转发标签。顶部图表标成 `capture protocols / last 128`，表明来自最近捕获帧，
避免和筛选后的列表占比混淆。字段、逻辑字节源、会话/证据、180×48→80×30→180×48、
退出和 CP936 恢复通过，截图由实际原生字符/颜色缓冲渲染。

完整 bin-parser 及子包测试通过（主包 **154.922s**）；最终完整 pcaputil **450.900s**、
CLI **2.023s** 通过。另有实际 FEC/Generic Label 字节范围和特殊标签展示回归；初始 codec
fuzz **20,436** 次、扩展到半/全协商、关闭、隔离及新代次的最终状态 fuzz **17,119** 次通过。
PIM、IS-IS、L2TPv3、RIPng、BFD Echo/MPLS、UDP 会话和原有 fuzz 差异继续开放，整体仍未完备。

## PIMv2 原生控制字段与真实 IPv6 分片恢复

[PIM 汇总](shark-public-corpus-20261025/pim-control-summary.json)把目录、显式
`pim_control.PIMControlFields`、原生 IP protocol 103 分发、原生能力登记和字段展示接到
同一套有界解析器。基础范围覆盖 Hello、Register/Stop、Join/Prune、Bootstrap、Assert、
Graft/Ack、Candidate-RP 和 State Refresh 的完整消息/选项/编码地址边界。版本、掩码、
数量和长度逐层检查；未知地址编码及扩展不扫描猜宽度。参数是通告，不能冒充已建立邻接、
选举结果、已安装多播路由或已拼齐的 Bootstrap RP-set。PIMv1 和扩展仍未覆盖，目录保持
partial，冻结的抓包归档没有修改。[PIM-SM 格式](https://www.rfc-editor.org/rfc/rfc7761.html)、
[Bootstrap 范围](https://www.rfc-editor.org/rfc/rfc5059.html)和
[Dense Mode 格式](https://www.rfc-editor.org/rfc/rfc3973.html)分别限定所述字段含义。

校验和使用实际外层 IP 地址；IPv6 Register 的伪头长度固定为 8，并区分完整消息的
兼容校验范围。无外层地址的 stateless 入口不宣称校验通过。Null-Register 的内层源/组
地址明确标为通告，不能创建 TCP/UDP 流；旧式零版本 dummy 头依赖明确的外层地址族。
IPv6 dummy 后没有 PIM 头时会如实显示，不把任意尾部四字节认作 dummy PIM。
真实 Register 的载荷继续走原有受限隧道归一化，内层已识别应用不会被控制事件遮住。

### 原始文件逐条对比与拒绝证明

主文件 Wireshark 的 **333** 个 PIMv2 视图均有相同原帧、接口及原始字节的原生事件；
其中 **315** 条完整合法消息通过 **8,394** 项有序字段、独立校验和、真实外层端点和
无虚构传输流的检查，已选字段零差异。其余 **18** 条明确拒绝，并独立检查原始字节：

- 9 条 Register 只有 8 字节公共头/flags，没有 IP 载荷，不能当作完整 Register。
- 9 条 Register-Stop 的组地址是 `0.0.0.0/32`，不属于多播。源地址全零可以表示 wildcard，
  本项没有把合法的 source wildcard 错当作错误；问题在 group。

较早的 324 条/8,520 项中包含了后一组无效组地址，最终严格验收已将它们移至拒绝表，
不是为了计数一致而放宽条件。原始 **51** 条 PIMv1 视图仍是未覆盖项。

Register 内 IP 的源/组地址在 Wireshark 属于 IP 字段，不能随意借用 `pim.source/group`
的字段契约。原生保留 `pim.observed.register.source/group` 供查询，并维持实际编码地址
的 Wireshark 字段名。编码地址外层包装和内层叶子重复展示有单独记录，只有在树结构明确
重复同一字段时按一个逻辑值核对，原始重复条目未丢弃。

Sharon 原始结果仍是 **非零验收返回码**：622 个可配对原始视图中，127 个精确匹配的是
拒绝/未选编码，158 个是未选版本，337 个没有原生完整匹配；原生没有产出任何 decoded PIM。
另有四个原生拒绝诊断没有对应 oracle 视图。没有把零成功解析写成“全字段一致”。
337 条逐项复查原始物理帧字节，**334** 条存在明确 IP 头长度、捕获长度或分片准入边界；
逻辑/重组数据源的偏移不冒充物理帧偏移。剩余 **3** 条仍是 UDP/MPLS/AYIYA 封装工作，
原始差异全部保留，334 条也没有从计数中删除。

两个大文件的通用字段集合差异仍为 **90/35**，TCP 流仍为 **824/840**、预览 **808/735**，
原有双向分区及端点指标逐项相同。主文件列标签只变化 **UDP -10 / PIMv2 +10**，来自此前
只显示通用 UDP 的 Register 包；Sharon 列计数不变。13 条 Null-Register 仍保留外层流量
端点，其通告的 `(S,G)` 现在有明确的递进字段，不会为了追平 dummy IP 的显示而伪造连接。

### 补充真实样本与两项接收恢复修复

另使用 [Wireshark 官网的 pim-reg.cap](https://wiki.wireshark.org/PIM)补充验证 IPv6：
20 条真实消息（2 Hello、17 Register、1 Join/Prune）全部匹配，**263** 项检查零差异。
其他基础控制布局另有作者编写的完整字节/逐切分/数量/预算测试，不能把这些测试冒称为
新抓到的全部高级路由交换。没有在用户网络上主动进行路由通告。

从这份真实文件明确派生 7 个离线 IPv6 故障控制，保持 PIM 字节和端点不变，外层分片、
时钟、截断和顺序修改均有 manifest。实际对比发现并修复了两项基础接收问题：

- 完全相同的 IPv6 分片重复原先会触发重叠隔离。现在只丢弃相同范围、相同字节且结束
  标记一致的重复，不扩展证据预算/贡献列表；offset-zero 的头也必须一致。冲突、部分
  重叠、变化的结束标记/首头仍拒绝并隔离。这采用了
  [RFC 8200 4.5 允许的重复分片处理](https://www.rfc-editor.org/rfc/rfc8200.html)。
- 原始捕获长度小于线上长度能证明本地 snaplen 截断。现在报告 incomplete、丢弃该次
  不完整观察并保留其他完整范围，让完整重传修补；不把本地捕获缺字节当作恶意线上
  片段隔离整个 ID。声明线上长度本身不合法的情况仍按坏包拒绝，不作此例外。

基线、分片、乱序、完全重复和截断后完整重传均与 Wireshark 交付同一原始 PIM；真丢中段
及未修复 snaplen 双方均为零完整消息，不补造字节。5 个正向控制有 **60** 项字段/字节
检查，两个负向控制也明确检查没有完整 PIM。分片重传/重复/捕获域排列 fuzz **8,765** 次
通过；重复 200 次不增长保留预算，冲突封包和关闭后预算释放有回归。PIM codec 边界 fuzz
另 **12,551** 次通过。这些是离线变体，不是重新现场抓到的路由流量。

### 实际 Windows 页面与最终回归

最终 Windows EXE `975e1e8ad302f4f257f7ce16e4dd47f3cfae7990cdf492bacaec0515da372c53`
与源码重建一致，SDK `25fc6b58fc5245afb3ad8a975bcc9c7dc1c5a9c2dd84de1ff63c4330615536e1`
也独立重建一致，全部最终大文件回放绑定这对二进制。完整 51,328 帧的两个页面验收分别
显示 **13** 条 Null-Register（首帧 #48202）与 **8** 条 IPv6 Null-Register（首帧 #48643）。
实际页面能查看外层端点、通告源/组、dummy 标记、缺省 dummy PIM 头状态和本 PDU 字节。
原生颜色/字符缓冲已渲染检查；缩放、退出和 CP936 恢复通过。

页面另外修复了三个可见问题：原生事件不再重复追加捕获层已有的 PIMv2（真实隧道的重复
IP 层保留）；控制名称和通告地址优先展示，完整载荷可在字节页查看，不让大段 base64
挡住字段；仅部分包不能求值时，提示改为 **Field filter partial: some packets unavailable**，
避免把仍有 13 个已验证匹配的筛选说成整体不可用。未解析/超预算字段不能变成否定谓词
的命中；预算拒绝仍保留实际消息长度和受限字节计数，不报告零字节。

完整 bin-parser 及子包通过（主包 **154.984s**），完整 pcaputil **439.959s**、CLI **3.342s**
通过；末尾能力登记/预算检查 **0.341s**、筛选提示最终 CLI 全包 **2.968s** 另行通过。
原始拒绝、未选编码、PIMv1、剩余封装、L2TPv3、IS-IS、RIPng、BFD Echo/MPLS 及既有
流/会话 fuzz 差异继续开放。没有声称核心协议已全部完备或与 Wireshark 全功能等价。

## L2TPv3 原生封装、控制字段和 Windows 实页

本轮补齐此前只有旧版 UDP L2TPv2 入口、没有实际原生 IP115 接收器的缺口。
共享 codec、`l2tp3.yaml` 的 IP/UDP 两个 canonical 入口、能力登记和字段展示已经对齐。
完整控制消息检查 IP 零 Session ID 前缀、T/L/S、16 位序号、32 位接收方 Control
Connection ID、精确 Length、每个 AVP 边界及基础消息所需的 AVP。未知、vendor、hidden
值保留原字节；计数和字节均有上限。算法切换允许的两个相邻 Message Digest 不再被误当
成重复冲突，摘要不冒称已验证；Physical Channel 是 AVP25，不能错用保留的 AVP67。

数据提供独立的显式 receiver-context codec，校验 0/4/8 字节 cookie、default sublayer、
24 位序号位、sequencing0/1/2 及选定 Ethernet4/5 布局，不搜索“像以太网”的字节来猜偏移。
四字节全零 cookie 与没有 cookie 是两回事。原生自动 UDP 入口仍要求完整控制语法，
只匹配端口或两三个头字段的数据包不能建连；显式 Decode-As 的依据单独标注。
cookie 与内层字节保留自有副本，测试明确覆盖了输入缓冲区复用后解析结果不变。
显式数据 codec 也检查完整 VLAN tag 和集合预算。hidden/unknown/vendor AVP 即使 M bit
为零，或参数重复/消息语义未选定，字段集合仍标为 limited；不把“看不到”变成否定谓词
命中。这里不限制 PW4 必须有 VLAN tag，RFC4719 允许接入侧先做 tag 适配。

当前原生入口**尚未实现完整控制协商/会话状态机**，因此不会把手工 context codec 的能力
冒称为自动协商或真实伪线交付。接下来仍需按接收方 CID/SID、双向 Ns/Nr、重传/缺口、
SCCRQ/RP/CN 与 ICRQ/RP/CN（及 outgoing 对应交换）建立受限关联，隔离连接和 SID 复用，
再将有根据的伪线交给内层解析器。没有这个证据，中途捕获的数据保持明确的封装层和未知
载荷。L2TP roadmap/catalog 已从旧的笼统完成项改成 partial；冻结历史归档不改写，例外
绑定可执行 canonical 完整/残包测试。

### 原始大文件逐包核验

Weber 的 **706/706** 个原始封装全部匹配，**4,556** 项完整 wire、字段、贡献帧、接口和
端点检查零差异：666 数据、20 HELLO、20 ZLB 确认。这些流量确实没有控制/会话建立协商；
Wireshark 默认也只展示数据的 Session ID 和 generated unknown pseudowire type0。
原生同样没有猜 cookie/sublayer/Ethernet，更没有因此产生 TCP 或 L2TP 会话身份。
合法 IP115 控制的零 Session ID 标记也保留，不能因它不是数据 SID 就删掉真实字段。
v3 的 `l2tp.sid` 为 32 位，不借用 v2 的 16 位 `l2tp.session`；旧 v2 字段与字节范围仍有回归。

Sharon 保留原始 **非零验收**：1,355 个精确 wire 匹配中，707 个有效封装、648 个明确拒绝，
选定字段和端点零差异；**23** 个 raw missing、**8** 个额外原生拒绝诊断仍保留。
另有 34 个 inner 视图位于不合法的父 UDP 长度下、17 个未建立所选原始 carrier 的视图。
不能把这些计数归零来宣称总体协议一致。独立复核 23 个 raw missing：

- 8 个没有完整重组的 IP 分片；首片上的 Wireshark 解码不等于完整消息交付。
- 11 个 IPv4 option 的长度/边界不合法。
- 4 个原始 Ethernet IPv4 EtherType 下实际 version6 的头；不做跨类型猜测。

八个额外诊断全部有独立的原始 IP115 长度错误/截断证明，且没有 decoded 身份。
本轮实际修掉了 **IPv4 TotalLength0** 的接收漏洞：底层库会把原始零长度替换为捕获剩余
长度；原生现在查原始头字节，不能据此认定完整 L2TP。较早阶段有 8 个这类 fuzz 包曾被
接收为 data envelope，最终全部移入明确拒绝，未删除原始证据。

两个完整文件的列标签变化分别仅 **IPv4 -706 / L2TPv3 +706** 和
**IPv4 -707 / L2TPv3 +707**。每个文件新增了此前缺失的 40 个 `l2tp.flags` 与 40 个
`l2tp.length` 的真实字段值；Sharon 剩余 fuzz 字段缺失没有冒称已补完。
原有通用字段集合差异仍为 **90/35**，TCP 流 **824/840**、预览 **808/735**，双向分区、
已有 endpoint 指标均与上一完整 PIM 版本逐项相同。

### 故障恢复、实际页面和验证范围

从原始真实 router frame#9 保留完整 L2TP 字节及端点，明确派生 7 个离线 IPv4 控制。
基线、分片、乱序、完全重复、snaplen 后完整重传双方均得到同一完整封装；缺中段和
未修复截断双方均为零完整消息。五个正向控制共 20 项字节、SID、端点和不伪造会话检查，
另有两个负向控制。这验证封装重组，不是控制协商/连接复用的替代验收。
bounded codec 定向 fuzz 前阶段 **38,619** 次、最终 opaque/VLAN 边界版本 **26,953** 次通过。

Windows 在完整 51,328 帧上实际运行，60,000 包缓存保证 EOF 后仍能查看前段的 706 个
L2TPv3 包，首包 #9；另验证 20 个 HELLO 的字段与 AVP 字节。页面显示
`Ethernet › MPLS › IPv4 › L2TPv3`，明确 receiver context unavailable 和 opaque payload
长度，保留 PDU 字节，不让一大段 base64 挡住字段。
发现并修复了通用解码器重复追加 `incomplete/undecoded` 的误导提示：只有原生已验证的
实际 IP115 PDU、长度及 SHA256 与该网络载荷完全匹配，才能覆盖那个明确的 unsupported
decoder 错误；不同字节的事件不能覆盖，真实 snaplen 提示仍保留。原生控制台颜色/字符
缓冲、字段定位、字节源、缩放、CP936 恢复和退出均检查。

完整 bin-parser 及子包通过（主包 **138.049s**），pcaputil **442.873s**、CLI **2.024s**
通过；实际页面修复后的完整 CLI **2.020s** 另行通过，末尾原生准入/字段、hidden 可选 AVP、VLAN 和 v2 范围回归 **2.067s**，最终完整 CLI **3.101s** 另行通过。总体仍为 draft：自动 L2TPv3 协商/伪线复用及其故障矩阵、PIMv1/
extensions、其他原生协议和既有 fuzz 流/会话差异继续处理，未宣称核心协议完备。

最终冻结二进制：CLI `6670c6efb2992963fecb42538a821d62cd3b0bce55481fc939fc22e5a4bb78fe`，SDK `3d84dad067bf255533a7fdb890a2f3b1e4b1f51b6ae4ded74b1717bf4faacf17`。两个完整大文件、七项离线分片控制与原生 Windows 数据/HELLO AVP 页面均使用此版本；汇总证据见 `shark-public-corpus-20261025/l2tp3-wire-summary.json`。控制页实际选中 Message Type6，定位本 PDU 的 offset22、2字节 `00 06`。

## 2026-10-25 后续：L2TPv3 真实协商、接收方参数和内层分发

前一阶段的 midstream wire-envelope 支持现在增加了有界的被动控制/会话观察；目录仍为
partial。新增真实样本来自 [Wireshark issue7711](https://gitlab.com/wireshark/wireshark/-/issues/7711)
的 9 个公开附件，共 285 帧，包含 IP115、多隧道、双方/单方动态 UDP 端口、不同长度
cookie、会话结束和替换。原始 pcap、派生故障 pcap、TShark JSON、SDK JSONL、可执行文件
和控制台截图均留在仓库外；提交的是审计代码、回归控制和聚合证据。

### 本轮实际修复

- 离线 plain-UDP 快速路径原先直接进入 datagramFields，绕过 L2TPv3 观察器。现在完整
  控制语法或已观察到的接收方 ID 能进入同一原生路径；SCCRP 选择动态响应端口后，后续
  控制/数据保持同一观察上下文。单凭端口、短头或不透明数据不自动创建会话。
- 控制消息按双向 Ns 有界排队并处理依赖；缺 SCCCN/ICRP 时不从后续 Nr 推造参数。
  完整重传可恢复，Nr/digest 改变的相同参数重传不重置状态；ACK/ZLB 不推进 Ns。
  冲突的保留序号或无法区分的控制 ID 复用进入隔离。
- 只有两组三报文的完整控制/会话建立证据，才选用接收方公布的 SID、cookie、子层和
  sequencing 要求。每个数据观察保留 6 个建立 PDU 的 ID、原始 SHA256 和贡献帧。
  cookie 校验遵循 [RFC3931](https://www.rfc-editor.org/info/rfc3931/) 的接收方向，四字节
  全零 cookie 也不是缺省。保留已关闭接收方历史，允许可区分 cookie 的会话 ID 复用；
  相同/前缀兼容 cookie 不据此猜代次。
- Ethernet PW4/5 的完整帧交给既有内层网络/TCP/UDP 分发；逻辑域带控制/会话代次，
  原始物理帧及接口不改写，内层消息保留父 PDU。内层 IP 分片也保留全部父 PDU。
  24 位有意义序号支持回绕、缺口/乱序和相同重复观察；相同序号不同载荷会隔离，
  sequence0 不声称序号有效。不把被动观察说成实际端点投递或认证成功。
- 真实旧实现把 ICCN 的新电路状态位保留为 1；这违反
  [RFC4719](https://www.rfc-editor.org/info/rfc4719/)，但不改变 cookie/子层字节边界。
  现在保留实际位并给出规范警告，不因此丢掉后续完整帧。旧 Framing/Bearer 能力、
  Bearer Type18、Framing Type19 和 ACCM 也有明确宽度及字段，未知 mandatory 参数仍拒绝
  学习上下文。控制摘要不把观察到的 A 位说成电路已正常工作。
- Windows 实际页面发现并修复了内层 ICMP/ARP 标签配外层 IPv4 地址、漏掉内层头路径的
  不一致。列表现在使用内层地址，另保留 physical/message 端点，路径保留重复的实际头，
  如 `Ethernet › IPv4 › UDP › L2TPv3 › Ethernet › IPv6 › IPv6HopByHop › ICMPv6`。
  接收方上下文被拒绝仍显示已证明的 L2TPv3 外层、opaque 原因以及期望 cookie/实际前缀。
- 新的边界测试复现了完整 Ethernet 封装中的 IPv4 TotalLength0、缺失尾部和版本错误仍
  产生 ICMP 观察的问题；现在核对原始内层 IP 字节后才分发。外层 UDP 入口也核对原始
  IPv4 长度，不能借底层库对零长度的替换建立 L2TP 上下文。

### 逐包证据和保留的差异

9 份样本的原始 TShark 与显式声明 UDP Decode-As 的补充视图分别保留。补充视图共
**6,022** 项所选字段、完整 carrier、物理引用、六 PDU 建立证据和接收方布局检查，
没有所选差异。原始结果仍有 **53** 个未提供的原生消息视图和 **28** 个 sublayer-S
字段差异：原始动态端口接续失败使参考端丢失部分协商上下文。这些未归零，也没有把
显式 Decode-As 的补充验收算成默认自动识别准确率。

`lns-eth` 的 **20** 个和 `lns-eth-ip` 的 **12** 个 Ethernet 数据都有独立从原始控制
AVP 重建的接收方 cookie、子层及内层字节证明。另 **36** 个真实数据包的实际 cookie
与接收方公布值不符：参考端使用 cookie 长度继续解码，原生明确拒绝。没有为凑齐标签
改用发送方 cookie，也没有把这些数据说成已认证或已解析的内层协议。PPP/PW7 的数据
保持 context-required，不能把 Ethernet 的解法推广到全部 PW。

从真实 `lns-eth` 完整前缀派生 **10** 个明确标记的离线故障控制，共 **48** 项直接验收：
SCCRP 先到、重复建立、缺 SCCCN、补回缺口、数据先于修复、snaplen、完整重传、相同 UDP
元组的新控制/会话 ID、相同控制 ID 的冲突 bootstrap。缺建立控制时原生零内层投递；
补回后仅接收后续完整数据，新 ID 产生独立逻辑代次，歧义复用不继承旧参数。参考端在
缺 SCCCN/snaplen/冲突复用时仍显示的内层计数和逆序 SCCRP 时的漏解码均原样保留。

冻结生产源码 `b2a03407bc` 再跑完整 **51,328 / 328,127** 帧，通用字段集合差异仍为
**90/35**，TCP 流 **824/840**、预览 **808/735**；既有标签、所选字段、端点和双向会话
分区指标与上一版本一致。Weber 的完整 CLI/SDK 导出字节相同。Sharon 的原始导出保留
**780** 个变化并逐项复核：**656** 个拒绝观察的 `pdu.length` 从错误的 0 改为实际保留的
wire 字节数；**2** 个已有 incomplete UDP 现在有明确 L2TP 截断诊断，仍没有解码字段；
**122** 行 IP 片段关闭诊断的遍历顺序变化，其源引用/域/原因多重集合完全相同。
没有删除这些原始行来声称导出逐字节一致。

### 回归和当前边界

完整 bin-parser 及子包通过（主包 **171.594s**），完整 pcaputil **514.887s**、CLI
**2.084s** 通过；末尾原生头/展示和 IP 边界修改后另跑完整 CLI **2.248s**、L2TP 原生
回归 **0.963s**，冻结源码的末次完整 CLI **2.025s** 另行通过，并保留先失败后修复的内层长度测试。控制/会话恢复定向 fuzz
**10,131** 次，bounded wire fuzz **41,654** 次通过。有真实快路径/公开回调一致性、
内层 DNS/MQTT、内层乱序分片、物理帧数量、所有权、预算关闭释放、序号回绕、冲突和
接收方向测试；不以测试自己拼的固定样本替代上述真实文件核验。末尾新增的拒绝观察 pdu.length 回归断言亦通过（0.354s）。

当前生产二进制的原生 Windows QA 覆盖 20 个有接收方上下文的数据、cookie 拒绝页、
实际字段/字节源、会话消息/证据、80 列缩放、恢复、CP936 和退出。图片在本地
`C:/tmp/yak-shark-l2tp-session-20261025/windows-b2a03407-observed.png` 和
`windows-b2a03407-refusal.png`，不是设计图。

冻结 CLI `c6ed25121afd81f0bcccf54bdd301bc224b167ac5db251950c2413bd14e5f81b`，
SDK `f9bbb9a48482f0cb0ad1ac515789b6742b94482e3d6713f522455249dd49a670`。
聚合证据为 `shark-public-corpus-20261025/l2tp3-session-summary.json`。
本轮没有证明控制消息认证、任意/厂商 AVP、PPP/全部伪线、无可区分 ID 的代次复用；
IP115 的专用会话页面、PIMv1/extensions、其他原生入口和既有 fuzz 流/字段覆盖差异仍继续
处理。PR 保持 draft，不能宣称核心协议已经完备，也没有把本地通过当成当前 head 的 CI。


## 2026-10-25 后续：IP/UDP 会话页与紧凑建立证据

修复 IP115 的已观察 L2TPv3 会话未出现在会话页、带错误但仍有字段的 PDU 被跳过，
以及详情回收后动态 UDP 端点被后来的服务端口覆盖的问题。协议会话键包含捕获域、
传输类型和原生 FlowID；无 FlowID 的中途包不能建立虚假连接。IP/UDP 保留消息边界，
每条消息从偏移 0 展示，不冒充连续 TCP 字节流；TCP 专用按钮和范围文案同时收紧。

Windows CP936 实际回放 `lns-eth-ip.pcap`、查询 `fields l2tp.sid == 1921849866`，
发现完整字段树挤掉了 6 条建立证据中的 5 条。现预留 32 KiB 保存紧凑的建立观察：
逐条核对原生 PDU 的传输类型、捕获域和完整字节 SHA-256 后，只保留帧号、名称和哈希。
缺失、被回收、哈希冲突和预算耗尽各自明确显示，不把哈希一致当成对端认证。
修复后实际页面同时显示 SCCRQ/SCCRP/SCCCN/ICRQ/ICRP/ICCN 六条证据；64 条数据
挤满消息详情预算的回归也仍保留六条紧凑观察。缩放、恢复、退出均通过。
完整 CLI 测试 2.098 秒，原生 L2TPv2/v3 相关检查 0.917 秒。

[本轮汇总](shark-public-corpus-20261025/l2tp3-association-display-summary.json)
保留二进制哈希、9 份真实捕获的原始与补充解码检查和完整文件指标。补充范围仍为
6,022 项检查、零选定字段差异；原始 53 个未选择参考视图和 28 个字段差异未删除。
两份大捕获共 379,455 帧，协议计数、字段检查及缺失数、90/35 个原始字段集合差异、
824/840 个 TCP 核心流及原始分区结果保持一致。Weber CLI/SDK 导出逐字节相同。
Sharon SDK 116 条变化全部为关闭时 IP 分片诊断的 map 遍历顺序，完整诊断多重集合一致；
CLI 另有 17 帧的 PDU 详情保留/回收差异，说明关闭顺序确实能影响有界历史的尾部，
**后续需要固定关闭/过期诊断顺序**，不能把这个现象仅按协议计数相同结案。

额外发现 `lns-eth-ip` 的一个方向并非所有内层 IPv6 都能按所观察的接收方布局解码：
ICRP 未带 AVP69，接收方默认 sublayer0；线上包却在看似 Ethernet 头前带四个零。
本实现保留接收方声明，不复制反方向 sublayer1 或试探跳过四字节。12 条数据消息的
接收方参数已验证，不等于 12 条内部 IPv6 字段都已一致；当前仅 6 条具有该内层结果。
这项互通/诊断边界仍保留，需要独立检查并改善说明，不能通过猜测布局消除差额。


## 2026-10-26 后续：STP 家族、严格载体与可复现导出

本轮为 IEEE STP/RSTP/标准 MSTP 和 Cisco PVST+/RPVST+ 接通共享的 bounded wire
codec、bin-parser 规范入口、原生入口、类型字段和递进页面。以原始 802.3 长度限制
BPDU，分别核对 LLC SAP/UI 或 Cisco SNAP OUI/PID，排除 padding。修复快路径 VLAN
长度头没有 LLC layer 时漏掉 2,037 个消息的问题；不再依赖底层是否恰好生成该 layer。
单播/组播 MAC、端口提示和上一条通告都不能替代完整载体与消息检查。

共同字段包含完整 root/bridge/port、flags 和精确 1/256 秒计时；保留原始 ticks，秒的
类型导出为精确十进制字符串。标准 MST 明确解析配置名、revision、未验证 digest、
CIST 和全部有界 MSTI；Cisco 通告解析 originating-VLAN TLV，冲突 VLAN、缺字段和
不完整集合不被接受。未知有界 TLV 可以保留，但字段投影显式不完整。所有这些都是
独立 BPDU 通告，没有虚构 TCP/UDP 会话、实际选举结果、安装端口状态或对端认证。
丢失一条通告不影响后续完整通告，TCN 不继承之前的 root/flags/MSTI。

特别修正了一次过严判断：冻结样本 `pr5023-gen-rstp` 的 Version2/Type0/35 字节
不能被标为 RSTP，但也不能仅因版本不匹配就一律说非法。
[Open vSwitch 接收实现](https://github.com/openvswitch/ovs/blob/main/lib/rstp-state-machines.c)
的 validate_received_bpdu 按配置/TCN 的 type 和长度选择布局；
[Wireshark4.6.9 BPDU 实现](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-bpdu.c)
也保留这种 STP 配置显示。当前保留 literal Version2，以 Type0 展示 **STP Configuration**
并给出 compatibility warning，不产生 rapid/multiple 字段。冻结捕获和历史基线均未改写，
另有 SHA 固定的回归证明原样本与独立 Type2/36 字节控制的区别。

### 原始逐包比较

| 原始捕获 | 原始 STP 视图 | 原生完整匹配 | 所选 wire/字段检查 | 所选差异/漏入 |
| --- | ---: | ---: | ---: | ---: |
| Weber 全部 51,328 帧 | 2,612 | 2,612：RPVST+ 2,205、RSTP 407 | 77,139 | 0 / 0 |
| Sharon 全部 328,127 帧 | 3,079 | 2,612：RPVST+ 2,205、RSTP 407 | 77,139 | 0 / 0 |
| Wireshark Wiki 真实 stp.pcap | 96 | 96 STP | 2,016 | 0 / 0 |

两大捕获共享部分原始通告，不能把相同内容说成独立流量多样性。Sharon 额外 **467**
个原始参考子视图仍保留：GRE/PPP 406、EtherIP 24、UDP/MPLS 22、IP/Ethernet 8、
UDP/MSRCP 5、Remote WLAN 2。它们没有在原始帧对应偏移处提供本轮选择的完整 Ethernet
长度/LLC/SNAP 载体；部分偏移来自重建数据源。不能直接用这些偏移扫描物理帧来补标签，
也没有把这些子树全部判成合法或非法 BPDU。相关载体和 fuzz 边界仍需后续处理。

17 组明确声明的离线控制共 **32** 物理帧、**72** 项直接验收和 **742** 项所选 wire/
字段检查：真实字节、重复/乱序通告、缺通告、snaplen 后完整修复、短 body、TCN 不继承、
错误 SSAP/OUI、未知 Type2 版本、Type0 兼容布局、缺/冲突/未知 VLAN TLV，以及独立构造的
一/二 MSTI 和损坏后修复。未发现所选差异。尚未找到真实正向标准 MST 捕获；MST 正向控制
明确为 authored，不能说真实 MST 已充分验证。legacy MST 编码和 SPTv4 仍未选择。

### 稳定性和 Windows

实际重复导出发现并修复三处时序差异：IP 分片超时/关闭诊断按完整 key 排序；TCP
关闭时对相同 endpoint hash 的不同捕获域增加 capture-local Index 平局规则；流式 CLI
的字段在物理包发布时冻结有界快照，增加 `pdu_scope=observed_at_packet`。后续重组带来的
旧贡献帧注解仍通过 SDK/TUI 历史查询提供，流式行不再因消费者速度和详情回收而改变。
每包 256 KiB/128 PDU、64 包输出队列以及显式 limit marker 限制快照；不完整投影、未知
扩展和回收不能令负字段过滤误判为“字段不存在”。unsigned64 的序列化快照保留 exact_value。

最终源码 **7d9fdba8c1** 再跑两份完整捕获，CLI/SDK 分别重复导出均 **逐字节一致**，
SDK 额外重跑也与完整逐包检查的原始导出相同。通用字段检查/缺失、端点、原始 **90/35**
字段集合差异、TCP 流 **824/840**、预览 **808/735** 和双向分区指标与上一批保持一致。
没有把快照口径变化或新协议标签当成旧差异归零。

完整 bin-parser/子包通过（主包136.388s），pcaputil532.940s（aff88c1833）；新增最终
TCP 平局排序及快照/片段定向回归0.457s、最终 CLI 全套3.868s。STP codec 定向 fuzz
**40,439** 次/47.067s 通过；该 codec 在最终排序提交中没有变化。不是当前 PR head 的
远程 CI 通过声明。

最终 Windows CP936 实际回放主文件，严格核对 EOF、51,328 包全部入窗口、实际查询、
选中行的 RPVST+ 与对应帧字段标题；字段/字节源、缩放、恢复和退出通过。路径为
`Ethernet › Dot1Q › LLC › SNAP › RPVST+`。另在最终版本实际验证 Version2/Type0 的 STP
兼容页。字段历史有界：VLAN121 的 SDK 总匹配为169，页面有证据可查的80条，底部明确
提示 partial；物理包保留数不等于完整字段历史。没有把80说成全部匹配，也没有默默提高
内存上限掩盖回收。截图及所有捕获/原始导出仅在本地临时目录。

[本轮聚合证据](shark-public-corpus-20261025/stp-wire-summary.json) 保存二进制/原始审计
哈希、逐协议计数、字段检查、重复稳定性和剩余载体边界。整体协议/字段验收仍未完成，
目录保持 partial，PR 继续 draft。


## 2026-10-26 后续：RIPng 原生入口和逐条下一跳作用域

补齐已有 YAML 却没有原生接收入口的 RIPng。保留历史 `RIPng` 叶节点入口，另提供
`RIPngFields`：它与原生 IPv6 UDP 入口使用同一 bounded codec，并注册全部原始 header/
RTE 关键字段、地址类型过滤和递进页面。端口521只提供选择提示，自动识别还要求真实
IPv6 UDP 载体、完整 Version1 Request/Response 及至少一个完整20字节条目。只有4字节
header 不足以自动确定身份；显式 Decode-As 也不能把 IPv4 变成 IPv6 或绕过坏条目。
完整未知版本、残条目、非法 metric/prefix length 不继承之前同元组的识别。

按 [RFC2080](https://www.rfc-editor.org/rfc/rfc2080.html) 区分普通 route/query 和
metric255 的 Next Hop 条目。advisory 只影响当前报文其后的条目，每个 datagram 重置为
originator。Next Hop 的非零 tag/prefix-length 按接收规则忽略，原始值和说明都保留；
非 link-local advisory 保留原地址，并明确回退 originator。请求中的 metric16 不冒充
不可达通告，whole-table request 单独显示。保留原始 IPv6 前缀与派生 masked prefix 的
区别；128位字段中的 IPv4-mapped IPv6 不会在类型导出或 CIDR 过滤中坍缩成 IPv4。
multicast/link-local 路由目的保留明确接收警告，不宣称被路由器采纳。没有创建虚构
FlowID、查询应答关联、跨包下一跳、已安装路由表或已验证邻居/跳数。

### 原始比较、控制和回归

| 捕获 | 原始 RIPng 视图 | 原生匹配 | 所选完整 wire/字段集合检查 | 所选差异/缺失 |
| --- | ---: | ---: | ---: | ---: |
| Weber 全部51,328帧 | 124 | 124 | 1,240 | 0 / 0 |
| Sharon 全部328,127帧 | 137 | 124 | 1,240 | 0 / 0 |
| Wiki ipv6-ripng 真实捕获 | 20 | 20 | 200 | 0 / 0 |

Sharon 的额外13个原始视图均为 IPv4 载体，header 也不符合所选 v1 请求/响应；原始帧号、
字节数、header/hash 与零原生 RIPng 观察单列保留，没有仅靠521端口补标签。两个大文件
的124条共享原始内容，不能当成独立多样性。Wiki 文件只做无损 gzip 解压，原始压缩文件
SHA为0339544cbf5491283f2e5c8c9f6d33830bd6c4a68416f26edcdaddef0a83610e，解压后
20,264字节；包含5个请求、15个响应、930个完整RTE。上述字段集合检查保留每个重复字段
的出现次数，并核对真实 UDP 长度、物理贡献帧与完整消息字节，不只比较协议计数。
原始样本见 [Wireshark Wiki](https://wiki.wireshark.org/uploads/__moin_import__/attachments/SampleCaptures/ipv6-ripng.gz)。

15组明确声明的离线控制共26帧、47项直接验收和190项 wire/字段集合检查：真实字节、
重复/乱序通告、缺中间通告、snaplen 后完整修复、坏 metric、完整载体内残条目、未知版本、
四字节 header、逐报文 Next Hop 重置、被忽略参数保留、非 link-local advisory、whole-table
query、mapped IPv6，以及不能把相邻两个 UDP datagram 拼成完整消息。全部通过。真实
大包和 Wiki 样本都没有 Next Hop 特殊条目；该分支目前是离线控制与原始参考 dissector
的独立对照，不能说已经抓到真实 Next Hop。此轮未在实际网络生成 RIPng 路由流量。

完整回归初次发现两处兼容问题：目录改用全-map入口使冻结样本丢失历史 Command叶节点；
旧归档 todo 元数据与现在 partial 状态冲突。已保留旧叶树，并在执行新旧入口边界测试后
只允许 RIPng 这一项有依据的历史状态转换。冻结 manifest、捕获、哈希及历史评分没有修改。
最终完整 bin-parser/子包通过（主包159.201s，1fc25a4a）；pcaputil483.297s通过（02d4fa39，
原生 wire/filter 代码不再变化，最终 SDK PE 与该版逐字节相同）；最终 UI 摘要修正后完整
CLI2.164s通过。定向 bounded wire fuzz21,434次/46.883s通过。首次组合回归命令因上述
bin-parser问题失败，不能把其中原生包通过说成该整条命令通过；远程CI亦未作保证。

### 页面与完整文件复验

最终生产源码 e1cab606dc 重跑全部379,455帧，唯一原始列标签变化是每个文件124条
UDP提升为RIPng：Weber UDP3724→3600，Sharon UDP109835→109711。已有通用字段检查和
缺失、端点、90/35项原始字段集合差异、TCP流824/840、预览808/735及分区指标不变。
两个完整文件的最终 CLI 重复导出逐字节一致；三次原生SDK导出亦逐字节一致，SDK二进制
本身也相同。没有用新增字段掩盖旧协议的缺口。

最终 Windows CP936 实测修掉摘要重复的 Packet Name/Command，列表直接显示 request/
response、条目数与首个前缀/metric。字段按条目类型、IPv6地址、prefix length、tag、metric
递进；frame4004所选 metric1 精确对应逻辑 captured-PDU 的0x17字节，派生 prefix/advisory
不伪造 byte range。路径为 `Ethernet › Dot1Q › IPv6 › UDP › RIPng`；严格 EOF/窗口入满/
实际查询/选中协议与对应详情帧、字节源、缩放恢复和退出通过。字段历史仍有界：查询
`fields ripng.cmd == 2` 在SDK共123条响应，页面证明保留的61条，明确提示 partial，不能
把61宣称全部匹配。截图只在本地临时目录。

[本轮聚合证据](shark-public-corpus-20261025/ripng-wire-summary.json) 包含捕获/二进制哈希、
重复结果、13条原始差异和15组控制范围。目录仍为partial：当前没有运行 RIPng路由器、
做真实邻居/跳数验证、建立全局路由表或实现未来版本。IS-IS等原生入口、剩余载体/fuzz
与字段历史体验继续处理；整体目标仍未完成。

## 2026-10-26 后续：IS-IS 原生入口、字段关联和参考工具复核

**已纠正 ES TLV3 的早期错误结论。** 真实 frame24231 的四字节空 ES 列表合法。先前
跟随 Wireshark/tcpdump 的每邻居十字节布局，将它标成 malformed，判断不准确。此前
677 decoded/1 malformed、57,550项检查、36组参考差异和“真实坏 ES TLV”页面证据已撤回，
由下述重新执行的结果替代。原始抓包和 Wireshark 输出仍保留，没有修改参考值来制造一致。

补齐原来已有 YAML、回放仍显示 LLC 的 IS-IS。历史 `ISIS` 叶树保持兼容，新增
`ISISFields` 与原生入口共用 bounded codec。当前范围为六字节 System ID、完整
IEEE802.3 长度、FE/FE LLC UI 与 NLPID83，支持四层 VLAN 和已验证 express FCS 的
mPacket。沿用载体解码，不能任意去掉八字节前缀。STP/IS-IS 只选择第一层 Ethernet
的明确入口，不在未证明的 MPLS/pseudowire 内寻找另一个 Ethernet 特征。

LAN/P2P Hello、L1/L2 LSP、CSNP 和 PSNP 展示具体协议名称、固定头、重复 TLV、所选
邻居/前缀条目和真实 PDU 字节范围。SNPA/MAC 与 System ID 分开；SNP Source ID 的
六字节 System ID 和一字节 circuit 分开。序列号、LSP fragment、P2P advertised state
是逐报文观察，不升级为邻接、LSDB、路由、DIS 选举或应答关联；FlowID/ResponseTo 为零。
未知扩展保持原始字节和 partial 范围，残条目不生成假的邻居或跨包字段。

按 [RFC3358](https://datatracker.ietf.org/doc/html/rfc3358) 分开完整性和语法：活动 LSP 的
Fletcher 检查排除可变 lifetime；purge/零值保留 Wire Checksum、不宣称验证成功。
可选 TLV12 只在 IIH/CSNP/PSNP 接受，重复/错误值拒绝，零值接受但未验证。
无 checksum 明确显示未验证。Fletcher 成功和 TLV 失败可同时出现，不能据此认证发送者。
HMAC-MD5 只解析有界结构；与非零可选 checksum 同时出现时保留组合违规提示。

旧式 IPv4 default metric 高位按 up/down 分发语义处理。可选 metric 原始 unsupported
bit 与易读 Supported 布尔值分开。区域地址同时保留带长度字节的参考字段和易读地址。
扩展 IPv4/IPv6 前缀保留编码字节、零扩展原值与 masked canonical prefix，派生完整
地址不伪造四/十六字节物理范围。依据
[RFC5305](https://www.rfc-editor.org/rfc/rfc5305.html)、
[RFC5308](https://datatracker.ietf.org/doc/html/rfc5308) 和固定版 tvbuff 核对。

### ES 邻居必须按共享 metric 分组

[RFC1142 §9.8 的公开 ISO 草案格式](https://www.rfc-editor.org/rfc/rfc1142.html) 明确
ES TLV3 为四字节 metric 加六字节 ID 列表；同一列表内邻居共享成本。该文现为历史文档，
不把它称为现行互联网标准；另以
[固定 Quagga 接收源码](https://github.com/Quagga/quagga/blob/4f593574c41deb598cbbefde5c6d938799e81d5a/isisd/isis_tlv.c)
交叉检查：先推进一次四字节 metric，再按六字节 ID 推进。该接收实现的 ID 集合仍有 FIXME，
这里只使用其布局证据，不据此宣称实际路由安装已验证。

原生树现在按 `ES Neighbor Count → Shared Metrics → Neighbors[]` 递进。metric 只投影
一次真实范围；每个 ID 有自己的六字节范围，不能复制 metric 制造多个物理字段。零 ID
合法，count 为派生值、不伪造 byte range；不同成本用不同 TLV。三字节/非4+6n长度才是
损坏结构。TLV3 仅按 L1 ES 上下文解释，L2 的不同上下文继续 opaque。

### 完整原始文件逐包与关联核验

| 捕获 | 原始含 IS-IS 的帧 | 匹配 root 消息 | wire/字段/条目关联检查 | 未处置所选差异 / 缺失 |
| --- | ---: | ---: | ---: | ---: |
| Weber 全部51,328帧 | 678 | 678 | 57,577 | 0 / 0 |
| Sharon 全部328,127帧 | 3,414 | 678 | 57,577 | 0 / 0 |
| Wiki P2P adjacency sync 真实捕获 | 15 | 15 | 928 | 0 / 0 |

每份大文件678条均 decoded：559 Hello、111 CSNP、7 LSP、1 PSNP。两份文件共享同一组
root PDU，不能当成独立多样性。官网
[P2P 捕获](https://wiki.wireshark.org/uploads/cc35c2f0b03269774fb4dd501d510c55/isis_p2p_adjacency_sync.pcapng)
12,040字节，SHA256 d344b2aacf60c32731b9996f62c3430d6140456fac991f39f2c26fcef32027e0；
含7 P2P Hello、4 L2 CSNP、2 L2 LSP、2 L2 PSNP。

检查每个重复字段的次数及值/范围成对关系。每份大文件含22,160项独立原始字节关联
检查，官网含334项：metric/地址/掩码/邻居/共享成本/SNPA/SNP lifetime/sequence/checksum
必须归属正确条目，不能靠字段多重集合相同通过。没有 Decode-As、去帧或标签归一化。

每份大文件保留42组原始参考差异，涵盖旧式 metric 错偏移/unsupported bit、ES 共享范围
和合法空列表被报 short_clv。Wireshark4.6.9 的非 default metric 及部分 supported 字段
从 default 字节读取，范围亦错。核对
[固定 Wireshark 源码](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-isis-lsp.c)
与独立 wire 布局；[tcpdump 固定提交](https://github.com/the-tcpdump-group/tcpdump/blob/94a3708f00f658994293a440fbbb7e71e2047a0f/print-isoclns.c)
只用于独立 metric 偏移检查，它的 ES 每记录布局也不正确，不能作该语法的权威。
聚合保留双方值和范围，不以“PK 全相同”掩盖参考工具错误。另5组/10条 IPv4 前缀为
12字节记录与实际4字节地址的有效范围粒度区别，单独记录；不称为解析错误。

### 边界、Windows 与重新验证

37组明确离线控制共58帧，192项直接验收和2762项 wire/字段/关联检查通过。覆盖重复、
乱序、缺中间 LSP、同 MAC 不同 System ID、fragment/P2P state 不粘连、snaplen 后完整
修复、坏 checksum/purge、真正损坏 ES TLV、合法空列表/共享多个 ID/不同成本分组、
错误 LLC/NLPID、长度矛盾、未知上下文、可选 checksum 零/非零/错误/重复/错误 PDU、
HMAC 结构和组合、非整字节 IPv4/IPv6 prefix、非零 circuit，拒绝拼接相邻残 datagram。
原始参考拒绝/差异保留；附加控制证明 attached-default 的 bit3 与 partition-repair 的
bit7 不能混用。非空 ES 列表、非零 optional checksum 和部分 sub-TLV 正例仍来自作者
控制，不宣称新抓到真实路由器流量；没有在本机网络发送 IS-IS 通告或验证收敛。

生产 ES 修正源码 **0208c1d1ff** 重新运行全部379,455帧，CLI/SDK 各自重复导出逐字节
相同。相对 RIPng 阶段，每份文件只有678条 LLC 提升为具体 IS-IS 子类型；通用字段检查/
缺失、原始90/35项差异、端点、TCP流824/840、预览808/735及分区指标不变。

ES 修正后 bounded-wire fuzz11,425次/46.479s通过。最新统一源码 **778d338cdb** 完整
bin-parser/子包、pcaputil 和 CLI 回归通过：主包 136.218s、原生 445.054s、CLI 2.137s。
此前0208组合命令的 bin-parser 和 native 包虽通过，CLI因运行期间 API 编辑混用旧
编译产物而失败；不把那条组合命令说成全通过。冻结 capture/manifest/历史评分未修改，
不据此宣称远程 CI 已通过。

Windows CP936 重新实测真实 frame24231：EOF/51,328包入满、正确查询和对应详情通过，
现为合法 L1 LSP，明确 `ES Neighbor Count: 0`、共享 Default Metric0，并高亮实际0x41
字节，无 Malformed 标记。同 LSP ID 的另一 L2 消息独立显示。真正的 length3 损坏 TLV
另用声明的离线两帧控制检查：Malformed TLV 仍在摘要前部可见，不再冒充真实异常流量。
字节源、80x30缩放/180x48恢复、退出和 code-page 恢复通过。此前 Hello 的281条 L1
查询与 frame24184 Priority64/0x13证据保留。字段历史仍有界，这些页面不认证全局可查。

[修正后聚合证据](shark-public-corpus-20261025/isis-wire-summary.json) 记录替代旧结论的
原因、源码/二进制/原始审计哈希、重复结果、原始参考差异、控制与真实控制台记录哈希。
捕获、派生 PCAP、keylog、可执行文件、控制台单元格和图片仅在本地临时目录。
IS-IS 仍为 partial：其他 ID 宽度、合法隧道入口、部分别名和路由操作尚未认证。
整体协议/字段/流/会话目标仍未全部完成，PR 保持 draft。

### 继续复核2736个载体/fuzz帧：不能仅凭 NLPID 确认协议

进一步逐条核查上述2736帧的**全部5381个原始 Wireshark TVB 视图**，保留同一 GRE/PPP
帧内的多个重复/重叠视图。原始 `isis_raw` 字节中，没有一个满足当前支持的 v1、0/6
System ID width、已知 PDU type 与对应固定头长度这一必要公共头签名。这项结论只针对
选定 profile，不把其他宽度或未来版本宣称为不存在，也不把未证明的 TVB 偏移说成物理
帧偏移。

| 原始载体组 | 帧 | 原始视图 | 满足选定必要公共头签名 |
| --- | ---: | ---: | ---: |
| IP/OSI，IPv4 protocol80 | 1688 | 1688 | 0 |
| UDP/MPLS | 772 | 772 | 0 |
| GRE/PPP | 262 | 2907 | 0 |
| EtherIP | 12 | 12 | 0 |
| 其他 | 2 | 2 | 0 |

IP/OSI 中另有677帧的 IPv4 total length 小于 header，1009帧的声明 datagram 长度越过
捕获边界，只有2帧在这项长度检查上完整；这2帧的 IS-IS 必要公共头仍不成立。不能因
protocol80/NLPID83 或参考工具尝试拆解的字段，就把这些帧提升为已确认 IS-IS。

同时核对最终完整 SDK 与 CLI：这2736帧没有被本系统确认为带完整固定头的 IS-IS，也没有
被列为已确认的 IS-IS 协议列。三方文件 SHA 绑定后再检查，避免拿另一捕获或不同时点的
导出来证明拒绝。[拒绝复核聚合证据](shark-public-corpus-20261025/isis-rejected-reference-summary.json)
保留每组原因计数、全部帧号、输入/原始详细结果哈希；完整逐视图表仍在本地临时目录。
原始3414帧/5381个补充视图的数量没有归一化为678。这解决的是这些原始视图的确认边界，
没有声称合法 IP/OSI、MPLS、GRE/PPP、EtherIP 的原生正例入口已经完成。

## 2026-10-26 后续：部分字段的可证明过滤结果

共享过滤器此前一见 limited/unknown extension/字段预算即拒绝整个包，连已经保留的正向
字段也无法查询。源码 **778d338cdb** 改为三值判断：true、false、unknown。已观察到的
值可以证明存在/等值/范围/contains/CIDR；未观察到的值在部分投影中保持 unknown。
`!=` 保留既有“存在且所有值都不等”的语义，缺少重复值不能证明它为 true；发现一个
相等值则能证明 false。NOT/AND/OR 按三值组合，不能把 unknown 取反后变成匹配。

`DisplayFilterResult.Complete` 表示谓词结果已确定，新增 `Partial` 明确指出字段集合
仍不完整。TUI 可以保留被真实字段证明的匹配行，同时继续显示 partial 原因。已淘汰的
PDU不从占位字段制造证据；损坏/不可表示的快照值被跳过并保留未知。包级字段 union
预算保持4096，正向证据无需抬高预算；缺失字段和超出128个PDU的范围仍不能证明不存在。
精确64位还原失败不能回退成近似值。旧的私有 incomplete 标记、JSON导出和负过滤保护保留。

定向原生/CLI测试以及上述完整统一回归通过。控制覆盖未知字段与确定字段的布尔组合、
`!=`/NOT、重复值、精确数值失败、PDU顺序、淘汰/union预算、缓存失效和页面partial提示；
每个确定结果还核对添加后来字段后不变。此变更改善保留证据的查询，不恢复已淘汰详情，
也不把 parser namespace 提升为已认证的应用协议或宣称所有字段已解析。

## 2026-10-26 后续：跨协议 PDU 长度和 partial 页面纠错

继续检查真实导出，发现共享 datagram 完成路径虽然保留了 Raw，却未给没有 UDP 构造器的
L2/network 观察初始化 Length。导致 Weber 3623条、Sharon 3421条非空 PDU 的 `pdu.length`
为0：各含2612 STP、678 IS-IS，另333/131 PIM。实际协议字节和原来的字节统计已存在，
问题在事件元数据和由它驱动的过滤/导出。

源码 **83e2a69e6b** 在共享完成路径仅补原来为0的观察长度，已有显式长度继续保留，
包括资源受限、未保留 body 和短 preview；不能用 retained 字节覆盖原始记录长度。
完整及 deferred 的实际 STP/VLAN、IS-IS 和 PIM 检查事件长度、逻辑字节与精确64位 JSON
快照/过滤一致。定向原生0.978s、CLI0.310s通过。前一统一源码778d338cdb的完整回归证据
仍保留，不把它重新命名成83e2的完整套件。

新版重跑两个完整文件的379,455帧，新增逐条 SDK/CLI差分审计直接验证上述3623/3421个
长度全部等于实际 Raw 字节数，两个出口完全一致。每一个其他 JSON 值、顺序、帧、PDU
数、协议字段、原始字节、方向、来源和关联保持原样；不把长度之外的字段删掉来通过。
物理帧数51328/328127、SDK事件39283/245615；通用字段、90/35原始差异、端点、824/840
TCP流、808/735预览和分区指标不变。IS-IS的57577项核验仍通过，42组原始参考差异保留。

Windows另检查明确的两帧离线控制：24字节 Hello 含未知 TLV，另一完整 Hello 为20字节。
查询 `fields pdu.length == 24` 正确保留第一行，41字节物理帧与24字节逻辑PDU有明确
载体区别。旧页面把所有 limited 都解释成历史不可用，并提示“some packets unavailable”，
即使匹配行全部可见。现在改为 `incomplete field evidence · protocol field coverage limited`；
真正 eviction 仍提示 retained history unavailable。实际 EOF/包数/查询/所选详情、字节源、
缩放恢复、退出和 code-page 恢复通过；unknown TLV 没有被说成完整解析。

[长度及页面聚合证据](shark-public-corpus-20261025/pdu-length-summary.json) 保存完整输入/
脚本/二进制/原始审计哈希。PCAP、原始JSONL和原生控制台图片继续只在本地临时目录。

## 2026-10-26 后续：NetFlow v9 原生模板接收、真实逐包与 Windows 页面

协议目录原来列有 NetFlow v9，但旧 bin-parser 入口只有静态外层，原生 UDP 分发没有
模板接收能力。本轮补齐共享 `NetFlowV9Fields`、目录/生成文档、原生接收、类型化字段与
逐级页面；原来 `extended_protocols.NetFlowV9` 的历史入口与 round-trip 兼容性保留。
目录仍标为 **Partial**，没有用一个 v9 标签宣称所有 NetFlow/IPFIX 版本完备。

### 实际解析与连接边界

源码 **65e8624b2c / c5696f7c1c / a444885038** 检查完整20字节头、FlowSet边界、普通/
Options模板、独立 scope 命名空间、字段宽度、数据记录与计数。选定0–3字节padding；
非零padding保留并提示 SHOULD 不符合，不把 SHOULD 误当 MUST。短记录通过总记录数
求唯一边界，有歧义则拒绝。System scope type1 的显式零长度保留为无字节的范围语义，
普通零宽字段和全零宽数据记录不能进入循环或冒充解析。相同PDU中模板可在数据之后，
冲突模板拒绝；调用方提供的模板上下文也验证宽度、ID和预算，出错不提交半个新状态。

按 **capture domain + 有方向的源/目的地址端口 + SourceID** 隔离模板，最多128个上下文，
使用本地捕获时间的5分钟有界租约。v9序号是导出包计数，连续序号和正向/回绕uptime才
可沿用；丢失、乱序、冲突重复、时钟倒退、畸形/截断和复用边界隔离旧模板，完整新模板
可恢复。完全相同的紧邻重复使用原来的 before-context，不续租。无捕获时间不缓存。
这是本系统保守接收策略，不能保证 UDP 序号连续就能证明没有攻击或隐藏的导出器变化。

实际发现并修复：NetFlow模板之后，同一UDP tuple被一个有效MAVLink消息占用时，较早
分发器先接走MAVLink，旧模板原本没有失效。现在在任何UDP协议认领前观察复用边界，
使用独立mutex，避免借其他UDP状态锁。完整网络包回归先证明旧实现会错误解码下一条
Data，再证明新版要求新模板。没有把未到达接收入口的任意坏IP包都算作模板边界证明。

完整Data Set保留实际模板包号、capture domain、源PDU与定义SHA256、定义偏移/长度/
序号；模板字节绝不冒充当前Data PDU的范围。导出IPv4/IPv6、端口、包/字节计数、TCP
flags、uptime、方向、flowId和firewallEvent与当前捕获端点/连接ID严格分开。148/233
按已观察到的v9扩展解析，IANA含义和Wireshark源码交叉核对，不说它们都由原始RFC3954
定义。未知字段保留literal bytes并报告limited，不凭两三个字段猜出完整应用。

主要依据：[RFC3954](https://www.rfc-editor.org/rfc/rfc3954.html)、
[IANA IPFIX注册表](https://www.iana.org/assignments/ipfix/ipfix.xhtml)、
[Wireshark4.6.9 NetFlow源码](https://raw.githubusercontent.com/wireshark/wireshark/v4.6.9/epan/dissectors/packet-netflow.c)。
IANA的兼容说明与较新元素的定义范围保留；不把IPFIX记录序号规则套到v9导出包。

### 两个完整大文件与独立样本

| 输入 | 原始参考v9 / 原生v9 | 原生状态 | 已解Data记录 | 独立字段/字节/来源/时间线检查 |
| --- | ---: | --- | ---: | ---: |
| Weber 51,328帧 | 196 / 196 | 193 decoded，3 context-required | 3,733 | 544,027，选定差异0 |
| Sharon 328,127帧 | 305 / 196 | 193 decoded，3 context-required | 3,733 | 544,027，选定差异0；109原始视图另列 |
| Scapy公开样本5帧 | 5 / 5 | 3模板 decoded，2 context-required | 0 | 241，选定差异0；25参考Data记录保留为拒绝 |

主文件中9个template-only消息共72个定义，184个Data消息共3733条记录。开始3个Data包
发生在首个已观察模板之前，Wireshark的原始两遍参考解出了20/21/20，共 **61** 条Data。
本系统不从未来模板反向制造此前已观察到的上下文，这61条保留头/FlowSet和明确原因。
所以“196个都识别为v9”和“所有参考记录都被解码”不是同一验收项。

Scapy固定来源为提交 `22cd7670eb7603fb156e3eb2e97d9d24ee9b51a2` 的
[netflowv9.pcap](https://raw.githubusercontent.com/secdev/scapy/22cd7670eb7603fb156e3eb2e97d9d24ee9b51a2/test/pcaps/netflowv9.pcap)，
14072字节，SHA256 `fc3368101cb41877013e82a5e0209ff0ba478471ec07a8040873b94ab64cb160`。
两个导出器模板到Data之间分别缺42/24个导出包，Wireshark继续使用已知模板解出24+1条；
本系统会隔离可能被缺包隐藏的定义更换。这 **25条差异保留**，没有降低接收条件来凑数量。

Sharon原始CFLOW版本计数保持：v9=305、v8=198、v10=159、v5=41、v7=24、v1=16。
额外109个v9视图逐个复核：17个UDP视图不满足选定完整语法；92个TCP视图不在本轮UDP
入口范围，其中1个满足基本FlowSet语法，其余91个不满足。不能把它们全部说成合法正例
缺失，也不能删去原始视图后声称305/305。v1/v5/v7/v8和IPFIXv10没有在本轮新增原生入口。

独立审计从原始TVB/物理字节重新读取头、集合/模板/记录边界、字段顺序、unsigned64与
地址值、精确范围、scope、padding、模板物理来源/定义摘要、capture-time租约、期间
连续序号/uptime和模板替换证据；按显式声明的别名契约检查，不能从当前程序恰好输出
哪些别名反过来决定验收。6个反向控制分别改错值、值范围、定义摘要、定义偏移、来源
包号、类型化别名，全部被独立审计拒绝。首次真实对比还发现重复遍历记录树消耗16k
投影预算而漏掉1018项别名；a444885038改为一次有界遍历建立本次投影索引，保持4096
值与遍历预算，200记录/400字段回归保留所有头和所选别名。

15组明确的离线派生控制共有 **62帧 / 66,086项** 检查，选定差异0：基线、缺模板、丢包后
恢复、乱序后恢复、完全重复、同序号不同内容、同tuple协议切换、畸形模板后恢复、
SourceID隔离、源端口隔离、序号回绕、uptime回绕/倒退、实际snaplen截断、租约过期。
字节来自公开原始帧20548/20549，序号/时间/内容修改和插入均标为离线控制，不称为新抓
真实故障。TShark保持原始启发式/宽松模板策略；拒绝和参考侧解出的记录数逐帧保留。

### Windows 实际发现的页面错误与修复

源码 **6992aa1700** 修复实际原生控制台暴露的第二层问题：常见21记录×17字段的NetFlow
消息被拿整个parser私有证据大小去扣仅保存scalar projection的256KiB队列，导致CLI
JSON把完整字段整条替换成预算占位；TUI又用同样过小的完整详情预算，字段筛选显示0条。
新增回归先在旧实现失败，再证明冻结的字段输出与真实详情页都保留该普通记录批次。

现在scalar队列按实际序列化输出收费，保留原来的256KiB/128PDU限制；所选物理包的
完整详情使用独立4MiB上限，仍有4096节点/32层展示限制和明确limited notice，没有
无限保留或把丢失字段当作完整。两个完整文件各修复 **193** 个CLI `pdus`字段快照；与
a444885038相比，每个其他CLI键和值保持不变，SDK全量导出byte-identical。

Windows原生CP936程序实际打开真实大文件的20545–20570共26帧片段，查询
`fields cflow.version == 9 and pdu.completed_in == 5`，完成EOF后保留1条实际匹配行；
片段#5对应原文件#20549。协议列/详情都是NetFlow v9，breadcrumb为Ethernet›IPv6›UDP›
NetFlow v9，逐级展开首条记录的 `Reported Flow ID:60767`，实际选择PDU逻辑偏移
0x46–0x4d的8字节。模板信息和导出器方向不冒充本地TCP会话或认证证据。字段和字节源、
80×30缩窄、180×48恢复、退出、code page恢复均通过。截图来自实际console cells，
没有用模型图替代；完整379455帧验证单独完成，不用短片段声称全量历史均已保留。

### 验证与仍未覆盖的内容

a444885038统一完整回归通过：bin-parser主包139.068s、子包、原生pcaputil453.098s、
CLI2.220s；后续6992aa1700仅改页面与冻结输出，完整CLI回归2.119s通过。最新相同原生
源码完成 **162,103次 / 46.872s** 有界wire fuzz（2 workers，1GiB）。这些是本地结果，
不等同当前PR远端CI通过。

最终两个完整文件的CLI分别重复导出，byte-identical；SDK的两次完整导出也相同。相对
此前长度修复版本，各增加196个NetFlow v9列、减少196个UDP列，未改变原始字段差异
**90/35**、现有字段检查/缺失计数、端点差异、TCP流 **824/840**、预览 **808/735**及
双向/域内会话分区指标。模板记录不会增加捕获连接，也没有用参考输出覆盖本地结果。

仍未完成：所有企业/扩展IE语义、绝对uptime推导时间、TCP flags逐bit别名、其他版本/
载体，以及前述原始字段/端点/会话/fuzz差异。当前保留原始flags和uptime、模板真实来源
字段，未解析的Wireshark别名及逐项数量在聚合中列明；这不是所有关键字段已一致。

[NetFlow v9聚合证据](shark-public-corpus-20261025/netflow-v9-summary.json) 绑定输入、原始
oracle、脚本、二进制、重复输出、字段/来源、拒绝、故障控制和实际页面的哈希与数量。
PCAP、派生PCAP、密钥、二进制、完整JSONL、原生console buffers和PNG继续留在本地临时
目录；仓库只增加实现、作者编写的回归/审计源代码和聚合结果。整体目标未完成，PR仍草稿。
## 2026-10-26 后续：NetFlow v5 原生入口、完整固定记录与畸形时间展示

发现目录中的 NetFlow v5 已有完整静态 bin-parser 规则，但没有原生 UDP 分发。本轮新增
共享 `NetFlowV5Fields`、原生入口、所有固定记录/采样参数/unused padding 的字节范围、
`cflow.*`字段与递进页；旧 `application-layer.NetFlowV5` 叶子、边界和错误行为保留。
目录仍为 Partial，选定24字节头、1–30条48字节记录；没有把v5功能扩展成所有版本。

v5不依赖模板，每个完整datagram独立解析全部记录。丢包、乱序、回绕、重复和引擎复用
都不能借用另一报文的值，报告流量不创建本地TCP流、transaction或响应关联。序号累计
的是**已报告流记录数**，不能套用v9“导出包序号连续+1”的规则。IPv4地址、接口、AS、
端口、计数和TCP flags均标为Reported，与当前导出器/收集器的真实捕获端点分开。

主要依据：[Cisco v5记录/序号说明](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/iproute_pi/configuration/15-s/nf-15-s-book/cfg-nflow-data-expt.html)
及 [Wireshark4.6.9实现/采样位域](https://raw.githubusercontent.com/wireshark/wireshark/v4.6.9/epan/dissectors/packet-netflow.c)。
两个采样位域共用实际16位storage范围，值分别为高2位/低14位。未分配采样模式、非零
unused padding和大于32的掩码保留原值及warning，不把255推导成合法IPv4 prefix。
自动识别仍需完整语法和服务提示；只有version/count前缀或端口不确认协议。

### 公开正例、原始坏包与独立差分

使用 Akvorado 固定提交 `ac68c5970e2c8eb1444483814e6a7b1f5c1956e3` 的
[nfv5.pcap](https://raw.githubusercontent.com/akvorado/akvorado/ac68c5970e2c8eb1444483814e6a7b1f5c1956e3/outlet/flow/decoder/netflow/testdata/nfv5.pcap)：
1498字节，SHA256 `6ae061846c7d85120215a140072f259726bd823d7852b72c08a370b503b47307`。
这是项目公开抓包样本，未把它说成本机新抓流量或独立证明原始生成方式。它使用UDP9990，
原始默认Wireshark和本系统都不认领；两边显式Decode-As后，1个完整报文的29条记录通过
**1832项**原始字段、精确范围、reported/captured区分、采样、时间与PDU元数据检查。
原始默认参考0条的事实保留，没有改端口或改原始参考来充当默认识别成功。

Sharon原始41个v5视图保持20个UDP、21个TCP。17个UDP的count/body不符合选定布局，
另2个同时有坏nanoseconds，1个看似完整72字节/单记录但有坏nanoseconds；21个TCP
不在本轮UDP入口范围，且count/body也不满足完整布局。**这些20个UDP的实际载体也
未通过独立完整性复核**，不能只检查被参考工具截出的应用TVB后就确认协议。

特别保留原始 **#89636**：114字节捕获/线上帧，IPv4总长100，实际UDP可用80字节，
UDP头却声明24593；其中CFLOW TVB为72字节，count1，nanoseconds1182905864。
它不仅时间坏，UDP载体也不可能完整。此前处理中先看应用TVB，误以为只坏时间，复核
原始物理字节后撤回该判断；原始帧继续拒绝v5确认。没有通过重写UDP长度来让它通过。
聚合保留全部原始版本数量、未选帧号、载体/布局原因与该纠正。当前共享诊断仍将此类
矛盾写成incomplete，页面原因也不够直接；这项共享UDP诊断工作留在下一步。

### 完整载体的畸形字段与页面

在独立完整UDP载体的控制中，若全部头和48字节记录完整、仅残余纳秒越界，过去直接
丢掉所有已知字段。源码 **4642c38bd7** 保留已知字段并返回时间错误；原生事件标为
`malformed`，`Export Timestamp Valid:false`及明确failure，仍可检查每一条记录和
原始纳秒；不推导或悄悄归一化绝对时间。旧静态规则/新的无状态规则仍返回错误。
自动确认该畸形观察需要真实完整UDP外框，不能绕过上面的坏载体拒绝。

Windows实际CP936程序检查公开单帧样本：`fields cflow.version == 5`，显式UDP9990
selector，实际EOF/所选行/详情#1一致。NetFlow v5面包屑为Ethernet›IPv4›UDP›NetFlow v5；
首条 `Reported Source IPv4:161.202.212.212`选择PDU逻辑0x18–0x1b四字节，捕获源端点
则为10.19.144.41，二者不混用。纳秒等头字段已放在长记录列表前。

另用明确标注的离线nanoseconds-overflow控制验证畸形页：表格仍显示NetFlow v5，
Info首先给出Malformed export timestamp，详情显示时间有效性false，选择原始
1000000000的0xc–0xf四字节，29条已知记录保留。该控制不是原始#89636。
字段/字节源、80×30及180×48缩放恢复、退出与code-page恢复均通过。控制台PNG和cells
留在本地，未用设计图代替真实程序。

共享字段帮助原本仍把 `cflow.version`等只归到v9。源码 **ba674b4c65** 新增可选、独立
所有权的 `Protocols`列出实际共享协议，同时保留兼容的 `Protocol`主标签；JSON schema
版本保持1的追加字段，text listing同时显示v5/v9。对原始返回slice的修改不能污染注册表。
该修复也使原本共享的HTTP/gRPC等binding如实列出支持事件名，未新增其协议解析声明。

### 故障、分片与验收范围

18组明确的离线控制共 **24帧 / 29379项** 检查，选定差异0：完整固定记录、非默认端口
显式选择、序号跳跃/乱序/回绕、完全重复、引擎复用、30记录边界、无效采样模式/掩码/
padding警告、空记录头、两字段前缀、0/31/count-body矛盾、纳秒越界、末记录截断、
多余尾字节和实际snaplen截断。共436条decoded报告记录，另29条畸形时间记录保留，
二者分开计数。坏/短包和端口提示不能变成decoded；显式selector不绕过语法。

另外5组IPv4分片控制共 **11帧**：按序、倒序、完全重复首片均只重组出1条完整消息、
29条报告记录，共 **5514项** 字段/原始来源检查；来源包号、完成包号和原始fragment bytes
均独立核验。完全重复首片不制造第二份导出记录。缺尾片只有ip/incomplete，冲突重叠
只有ip/malformed，不确认应用。重组字段对应实际UDP TVB的逻辑范围，不冒充某个物理
帧的连续字节；独立重组会检查同key捕获区间内被参考工具丢弃的重复/重叠片。

冲突重叠的原始Wireshark仍以尽力策略解出29条记录，本系统拒绝这些不确定字节。这
**29条参考记录差异保留**，没有因选定字段检查为0差异就声称冲突包全部对齐。
8个审计反向控制故意破坏地址、值范围、source hash、类型化别名、纳秒、record index、
字段/别名存在性，全部被独立审计拒绝；不能从程序恰好输出哪些字段反推验收契约。

统一源码4642c38bd7完整回归通过：bin-parser主包125.621s及子包、原生427.231s、
CLI2.213s；最新相同源码 **15866次 / 46.339s** wire fuzz通过（2 workers，1GiB）。
两个完整大文件379455帧重放，当前CLI和SDK各再独立重复，byte-identical；并且全量
输出与此前v9阶段逐字节相同。原始41个v5坏/未选视图没有为了数量一致被认领。

既有v9每份544027项检查仍通过；通用90/35原始字段差异、检查/缺失、端点、TCP流
824/840、预览808/735和会话分区指标保持原样。这里没有宣称这些既有差异已全部解决。
IPFIXv10、v1/v7/v8、v5 TCP、全部扩展/时间推导/flags bit别名及共享UDP诊断仍未完成。

[v5聚合证据](shark-public-corpus-20261025/netflow-v5-summary.json) 绑定固定来源、真实默认
遗漏、补充参考、全部重复输出、原始拒绝、故障与分片差异、测试、脚本/二进制和真实
Windows页哈希。只提交实现、作者编写的审计/测试及聚合；抓包、派生抓包、密钥、二进制、
完整JSONL和原生页面图继续放在仓库外。整体目标未完成，PR仍草稿。

## 2026-10-26 共享UDP载体、IPv6扩展头与新实测流量

本节更新前述v5阶段排队的共享UDP诊断问题；历史聚合保留其原始源码快照。
源码703c75fefc、ec13807f64及b9242dc9c3依次修复字面长度、早期L2TPv3分发和隧道
解码之前的校验。真实捕获缺字节保持`incomplete/NeedMore`，UDP长度小于8或超过
完整IP所能容纳的空间则为`malformed/MalformedMessage`。IPv4 TotalLength0不能因
底层以捕获大小补值就成为UDP应用识别证据。原始SDK、冻结JSON行和Windows详情均
保留具体原因；坏载体不导出应用字段，公开缓存修改也不能把拒绝改成解码成功。

这遵循[UDP长度定义](https://www.rfc-editor.org/rfc/rfc768)和
[IPv6载荷长度包含扩展头](https://www.rfc-editor.org/rfc/rfc8200)的原始字节边界。
底层gopacket1.3.1在移除Hop-by-Hop头后仍使用未扣除扩展头的Payload Length，会把
完整包误标截断、或带入填充。私有原生适配层现按原始IPv6包界裁剪，并保留HBH层、
真实截断反馈；复用解码器会清除前一个包的HBH和端点。正常、填充、TCP/UDP及RAW
分片重组入口共用修复，观察者原始视图不改写。零长度IPv6/jumbogram未因此被宣称支持。

早于通用UDP分发的L2TPv3入口也使用同一分类，并在协议切换/坏载体时退休原NetFlow
模板。新增测试先复现“同元组经过L2TP后仍用旧v9模板解码”的错误，修复后要求新模板。
VXLAN/Geneve/GTP不能先凭UDP端口进入隧道再掩盖坏外框；读取器的捕获截断标记也会
阻止内层应用确认。有效、已选VXLAN/Geneve配置仍能解析内层DNS，有效GTP内层归一化
仍明确受限。字段覆盖范围与畸形/缺上下文结果分开，页面不再把已知部分字段误说成投影
预算或类型转换失败；重复错误前缀也已精简。

### 原始样本逐包结果

完整两份大文件仍为51328/328127帧。原始Sharon #89636的UDP声明24593字节，IP只
容纳80字节；现在为malformed且列仍为UDP，未以CFLOW外观认领v5。#38024的早期
L2TPv3同样给出字面长度矛盾，不能再笼统称抓包截断。#75002、#97159、#98754三个
GTP坏包从`ip/limited`改为明确UDP malformed，恢复实际外层端点；UDP声明长度分别
41226/259/5140，实际IP UDP容量119/174/78。它们没有因此成为GTP或内层应用成功解析。

| 原始文件 | 参考筛选帧 | 选定语义核验帧 | 核验项 | 选定差异 | 未选视图 | 另无选定语义断言帧 |
|---|---:|---:|---:|---:|---:|---:|
| Weber | 141 | 131 | 665 | 0 | 10 | 0 |
| Sharon | 105503 | 93571 | 1134304 | 0 | 11225 | 707 |

Sharon选定范围包括93502个明确畸形UDP载体和69个完整HBH/ICMPv6基础视图，其中2个
UDP拒绝保留已选L2TPv3解码器诊断但无应用字段。11225个未选视图中，11215个是多IP/
引用视图，2个缺可证明的物理UDP头，7个是零IPv6长度/jumbogram配置，1个UDP位于
未接入EtherIP/XIP嵌套载体内。它们与另707个无本节选定语义断言的帧均不计入协议
字段通过；逐帧清单保留在本地。参考导出使用未修改Wireshark4.6.9，只裁去无关应用
输出子树，保留原始头、偏移、物理帧和错误。大JSON按包流式读取，未把整个字段森林
加载进内存。原始参考字段/列差异没有为此归一化。

撤销了1670条畸形Geneve和3条畸形VXLAN UDP产生的封装域，防止端口触发的假域进入
后续关联。Sharon相同参考UDP stream/domain对仍为10100，域内缺失/拆分从13降至10；
参考端点差异source/destination从46380/46309降至46377/46306。Weber原13/13差异
保留；这些数值不能当作端点全部一致。TCP流824/840、预览808/735保持，原始通用字段
差异仍为90/35，checked/missing字段覆盖计数保持。既有v9每份544027项选定检查继续
通过。最新Sharon SDK两次独立全文件输出逐字节相同。

### 新Windows真实socket流量与HTTP/3差异

此次在Windows Npcap Loopback新抓747帧、1066696字节，45次自有应用操作；不是离线
拼包。包含TLS1.2/1.3、HTTP/1.1、HTTP/2、gRPC、HTTP/3、DNS TCP/UDP和TLS/Redis
元组复用。代理实际丢9个包、延迟15个包，Npcap丢包为0。只捕获自有临时端口；代理
故障与捕获丢包分开记录。本次自有非标准端口在补充Wireshark参考中显式选择协议，
双方使用同一份本次会话密钥，TCP/TLS重组配置由compare.py记录。它不替代上面的原始
大文件默认参考指标，密钥不入库。

239个完整TLS记录按参考TCP流、方向端点和原始记录逐条匹配，717项原生头检查通过；
10条gRPC消息原始字节、12条HTTP/3路径一致。直接HTTP/3 DATA展示计数为原生27/
参考24，**此差异保留**。独立审计绕开HTTP/3展示，读取同一未修改参考已解密的QUIC
STREAM字节，按连接/方向/stream重组，验证重复/重叠一致性、洞和FIN，再独立读变长
整数与DATA帧。3个连接、24个双向流方向、150个STREAM视图得到27条DATA，与原生
顺序和payload完全一致，另逐帧核对150个STREAM的端点、stream ID、offset、length、字节和FIN，共10398项检查，重复重叠8454字节未制造重复投递。两份原生
输出与本次应用返回hash也对应。参考HTTP/3展示少了3个DATA观察，不能通过删原生
结果强行凑24；独立重组也不能证明QPACK和所有高层字段都完备。

### 故障、反向控制和实际页面

54组声明为离线控制的66帧通过612项检查：IPv4/IPv6、HBH/Destination组合、UDP
短/零/越界长度、真正捕获截断、截断且长度矛盾、链路填充、正常和畸形隧道、IPv6 HBH
分片正序/倒序/完全重复/缺尾/冲突重叠。正常和倒序只得到一份DNS，原始消息及id/name/
type逐项核验；引用包号、完成包号和重复首片来源也检查。冲突重叠只给ip/malformed，
缺尾只给ip/incomplete；参考仍尽力展示的DNS、截断部分字段和未选GTP配置差异逐项
保留。8个载体审计反向控制、11个HTTP/3反向控制均被拒绝，包括改状态、伪造字段、
删拒绝原因、错参考端口，以及改DATA、方向、stream、长度、认证、来源、删除/重复DATA，以及STREAM offset/length/FIN。

最新源码完整原生441.219s、CLI1.989s回归通过。IPv6包界fuzz在703c75fefc完成26036次/
46.457s，2 workers、1GiB；本节没有把这一小范围fuzz说成所有协议fuzz完备。封存的
原始样本与哈希保持，只在独立字面事实核验后更新旧错误分类断言，没有重写捕获字节。

Windows CP936实程序分别打开原始#89636、#75002单帧摘录，`fields status == malformed`
的EOF、所选UDP行、详情和帧号一致，列表能同时看到声明长度与IP容量；详情解释应用
未解析，物理IP/UDP与字节页仍可查看。两种尺寸恢复、字节源、退出及code page恢复
均通过。使用实际控制台cells渲染并查看页面，不是示意图；完整cells/PNG仍在本地。

[本轮聚合证据](shark-public-corpus-20261025/udp-carrier-and-fresh-live-summary.json)绑定源码、
二进制/审计源码哈希、完整原始对照、保留差异、故障、反向控制和实际页面。IPFIX等
未接入协议、剩余90/35通用字段差异、嵌套载体、扩展语义和QUIC/HTTP3跨包字段来源/
高亮范围仍需继续检查；不宣称核心协议已全部完备。只提交实现、作者编写的验证程序
及聚合，原始/派生pcap、密钥、程序、完整JSONL与页面图不入库，整体目标仍进行中。


## 显式UDP选择优先级与Syslog二进制视图（2026-10-26）

发现真实解析路径错误：RFC5424允许的二进制MSG-ANY可与旧VXLAN端口解码重叠。
即使用户显式选择UDP4789为Syslog，旧路径仍先拆出伪内层DNS。修复使显式UDP选择
先于旧隧道端口分发，继续验证实际Syslog语法和UDP载体；没有改默认端口策略，也没有
把所有碰巧像Syslog的内容自动认成Syslog。IPv4/IPv6、观察器开关、普通/残DNS共24项
及二进制Syslog完整/分片正序/倒序共12项原生回归通过。

独立作者控制使用正确IP/UDP校验和，六组十帧；IPv6包含HBH，分片正序和倒序只输出
一条Syslog，消息81字节及MSG64字节与同配置Wireshark逐字节相同，引用来源也保留。
这些是明确声明的离线控制，不是真实网络捕获。补充参考双方显式选Syslog；默认参考
仍可能显示DNS，其原始列单独保存，未改写大文件参考计数。

源码6b67bae4ac完整原生416.457s、CLI1.864s通过。两个原始大文件重新完整默认回放：
CLI51328/328127行、SDK39283/245615行，四份输出与b9242dc9证据逐字节相同；原始
字段90/35、流及端点差异因此保持，没有声称新增完整参考字段审计。CLI页面修复另经
完整CLI1.951s回归，直接事件及worker JSON往返都验证保留字节所有权。

Windows实际程序在CP936打开离线控制并核对EOF、当前proto:syslog查询、所选帧和
详情。Message Bytes显示64 bytes并能点击，右侧为明确标注的Syslog MSG提取字节源，
0..64全段高亮；不伪装为物理帧连续偏移。NIL头显示not provided，替代<nil>。
尺寸切换、退出及CP936恢复通过，cells/PNG在本地。[聚合证据](shark-public-corpus-20261025/udp-selector-and-syslog-summary.json)
保留源/程序/日志/图像哈希。pcap、程序、密钥、cells和图像均不入Git；IPFIX原生解析等
工作仍未完成，整个PR保持draft。


## IPFIX原生UDP/TCP模板接入与实际捕获（2026-10-26）

新增共享IPFIXFields与UDP/TCP原生路径，保留旧IPFIX叶树。目录、规则归档、原生分发、
字段注册与递进详情对应。按[RFC7011](https://www.rfc-editor.org/rfc/rfc7011)与
[IANA类型登记](https://www.iana.org/assignments/ipfix/ipfix.xhtml)分别处理数据记录序号、
Options作用域、企业E位/PEN命名空间、可变长字段及合法短填充。模板按Set顺序生效，
后来的模板不反解更早的未知数据；未知IE保留原字节，不冒充标准字段。TCP Flags支持
16位，IPv6 Flow Label按32位类型处理，毫秒绝对时间保留8字节，区别于v9布局。

UDP模板按方向端点、捕获域与Observation Domain隔离，使用明确的本地300秒捕获时间
租约。数据记录序号缺口、倒退、坏载体、过期和元组换协议清退旧模板，完整新模板恢复。
模板消息不增加数据序号；精确相邻重复不刷新租约；UDP撤销明确忽略。TCP缓存绑定
捕获连接代次/方向/Observation Domain，处理拆分/合并消息与有序撤销；未经撤销的
不同重定义持续隔离，重复冲突定义不能解封。完整坏消息不吞掉后续消息边界。
正确识别不意味着认证；仅数据序号无法发现模板消息单独丢失，这个不确定性明确保留。

补查拒绝空Template/Options Set、空Data Set和只有填充没有记录的Set；合法的无Set
完整消息头与它们分开。自动识别仍需更多完整语法证据，端口/版本不能单独确认身份。
调用方模板在遍历/复制前先计入上限，全部原生缓存也计入现有会话预算。

| 核验范围 | 结果及保留差异 |
|---|---|
| 两份原始大文件 | CLI51328/328127行、SDK39283/245615行四份完整输出与b9242dc9逐字节相同；原始字段90/35与既有流/端点差异保持。 |
| 原始v10参考视图 | Weber0、Sharon159；UDP122长度矛盾+1坏Set，TCP34长度矛盾+1短头，其他载体1长度矛盾；无一通过必要头/Set边界，不补159个假确认标签。 |
| 固定上游项目正例 | Akvorado6065985版本，1608字节两帧。默认参考仍为UDP/data；补充双方显式选择UDP2057。2消息/18记录，1868项字节、字段、投影范围、模板来源与时间检查通过；36个未知语义值保留字节，不计语义通过。 |
| 离线故障控制 | 16组35帧221项；丢失/乱序/重复/序号回绕/租约/元组复用/撤销/重定义/未来模板/残缺与空Set。5处参考旧模板解码与原生拒绝的标量差异保留。 |
| 审计反向控制 | 12处字段值/偏移/IE/企业位/模板哈希/条数/协议/来源帧/删增消息/投影值与范围篡改全部被发现。 |
| 新抓Windows实际socket | Npcap Loopback52帧，捕获丢包0；自有UDP代理实际丢1条、延迟1条，9条到达收集端；3个新TCP连接，消息分段发送，收集端哈希与发送端一致。它是自建导出器实验，非生产路由器流量。 |
| 新抓逐包对照 | 16条完整消息，独立按TCP seq/方向重组原始字节并核对全部贡献包号，258项通过。原生7记录/参考10记录，UDP#3/#6/#7旧模板差异明确拒绝；后续模板恢复。无模板的新TCP连接不继承别的连接字段，3个连接ID保持独立。 |

源码63c9b3c71c完整原生417.689s、CLI1.905s通过，另共享解码器/bin-parser完整回归通过；
同源码IPFIX fuzz108280次/40.504s、2 workers、1GiB通过。早期169478次结果仅对应
1cbca9c47d实现，不拿它替代最新空Set边界验证。

Windows CP936实际程序在上游两帧里用`fields cflow.packets == 9`选第2帧；列表IPFIX/
18条记录与详情一致。递进展开到Records[0]/Fields[0]，Reported Source IPv4是
203.0.113.235，区别于捕获发送端192.0.2.1。点击高亮真实PDU20..24的cb0071eb四字节，
模板源包引用保留且不伪造当前数据偏移。头部按线上顺序，Export Time显示UTC与原始秒数，
标准IE不显示默认企业编号0。EOF/当前查询/所选行/详情帧、尺寸、退出及code page恢复
通过，cells/PNG留本地。[聚合证据](shark-public-corpus-20261025/ipfix-summary.json)
绑定源、程序、审计源码与原始结果哈希。

IPFIX仍是限定profile：结构化list、全部企业字段语义、SCTP入口、认证身份及时间era
推断未认证。借此检查HTTP/3实际#492又发现5776字节DATA在详情被当作7704字节Base64，
且未连接到完整重组载荷字节源；这是后续展示修复，不在本IPFIX通过范围。整体协议/流/
会话/关键字段一致性目标仍未完成，PR保持draft。仅实现、作者审计程序与聚合入库，pcap、
派生捕获、密钥、EXE、完整JSONL和页面图不入库。


## HTTP/3实际DATA字节视图修复（2026-10-26）

在前述747帧真实socket捕获的#492逐层展开，列表DATA5776 bytes正确，详情却显示
Payload7704 bytes，并停在加密的QUIC线上包。原因是worker JSON的Base64字串被当作
载荷文本，且HTTP/3重组载荷未接入详情字节源。原生载荷本身与此前独立STREAM重组
相同；当前原生SDK整份747帧输出也与b9242dc9审计输出逐字节相同（0be78bed…）。

修复在已认证HTTP/3帧上下文提供完整DATA与QPACK块的独立字节源，绑定实际所属stream，
明确为重组载荷。顶部完成包号明确描述QUIC线上包；不是把跨包DATA的5776字节说成
全部来自#492。原始物理帧823字节、QUIC PDU791字节，最后交付STREAM块offset5096/
764字节，与重组载荷范围分开。未知/未认证载荷不能获得这个明文字节源。线上字节列表
也按严格八位数值收为单个可查看字段，避免几百个Protected Payload[n]挤占字段树；
负数、越界、小数等拒绝转换。字节源名放在面板标题，右侧仅显示选区，避免重复长名称
挤掉范围。gRPC原有选中字节源身份断言保留。

源码67d2bc463d完整CLI1.923s通过；直接与worker JSON、内外不同stream ID、QPACK、
无认证/CANCEL_PUSH隔离、所有权与4096字节紧凑显示回归通过。Windows实际程序再开
完整747帧，用proto:http3 info:5776选#492/stream8，DATA字段5776 bytes与右侧重组
字节源一致，0x0–0x168f/5776 bytes完整选区可见；EOF、行/详情、resize、退出与CP936
恢复通过。两份原始大文件的当前CLI完整输出与已核验b9242dc9逐字节相同，原字段/
端点/会话差异保持。[本轮聚合](shark-public-corpus-20261025/http3-payload-view-summary.json)
保留前后cells、实页PNG、源与程序哈希，原始文件留本地。

原生27/参考24的HTTP/3 DATA展示差异仍保留。此前独立STREAM重组/10398项检查的
范围保持；本节确认当前SDK输出等价及实际展示，不冒充重新跑过独立参考审计。完整
应用帧的贡献PDU清单、流偏移来源与更广QPACK语义仍待补齐；本节不宣称核心协议或
所有跨包字段来源已完备。


## QUIC/HTTP3跨包来源与实际帧头选择（2026-10-26）

补查 STREAM 重叠/乱序时发现两个来源错误：原始 Stream Data 仍为整块，却把 Byte Source
Offset 写成新交付后缀；补洞触发的交付也只列当前 PDU，漏掉早先乱序缓存的贡献者。
现在原始、连续交付、完整应用帧和应用载荷各有明确的坐标及来源。每个连接/stream/方向
保存有界、经重组验证的字节见证；冲突重叠不能进入证据，应用前缀消费后及时清理。
它是足以覆盖字节的见证集合，不承诺枚举所有重传、对端收到或给密文伪造物理偏移。
范围最多min(maxElements,1024)条，每条最多64个物理引用；不足显示limited。

HTTP/3保留线上原始Type/Length varint、各自字节范围、完整帧SHA256、流ID/偏移、整帧
及载荷来源；合法非最短varint也不被重新编码。新增http3.frame_type/frame_length投影
保持64位精度，不把它们的范围硬贴到加密QUIC Raw上。QPACK恢复后保留原HEADERS流ID
和原消息字节来源，而不是编码器流ID/后来触发包。这里验证帧字节来源，未声称提供完整
动态表或解码头字段的语义依赖来源。[QUIC STREAM](https://www.rfc-editor.org/rfc/rfc9000.html#section-19.8)、
[HTTP/3帧](https://www.rfc-editor.org/rfc/rfc9114.html#section-7.1)与
[QPACK阻塞恢复](https://www.rfc-editor.org/rfc/rfc9204.html#section-2.2.1)范围分开。

| 核验 | 当前结果 |
|---|---|
| 747帧实际socket捕获重新回放 | 3连接、57应用帧（6 SETTINGS/24 HEADERS/27 DATA）；从原始pcap核对物理帧、端点、捕获域，再独立按参考STREAM重组。406来源范围、24736项检查通过。13个合包的参考连接信息仅在DCID无歧义时继承。 |
| 独立载荷检查重跑 | 150/150原始STREAM视图、8454重叠字节、24方向、10398项检查；独立27 DATA与原生一致。原生27/参考HTTP3显示24的差异继续保留。 |
| 审计反向控制 | 29种来源范围/PDU/包号/域/端点/认证/覆盖标志、类型/长度/哈希/头选区/投影、增删帧、原始参考字节及合包DCID歧义篡改全部拒绝；正例先通过。 |
| 独立字节存在模型fuzz | 38424次/41.347s，2 workers、1GiB；任意丢失、乱序、重叠与3流双向交付，不借其他流/方向的见证。此为认证后算法验证，不代替线上AEAD证明。 |
| 两份原始大文件完整复验 | CLI51328/328127行、SDK39283/245615行四份输出与b924基线逐字节一致；90/35原始字段差异、824/840流及808/735预览等全部指标保持。最新UI程序也再次完整回放两文件，CLI仍逐字节一致。 |

原生源码5f18dcd9bd完整404.539s通过；新增来源/QPACK恢复回归0.363s与CLI完整2.154s
通过。新来源树曾挤满256KiB会话预算、使已有:path回归失败；已改为单独的消息摘要快照，
按原stream保存全部头节，完整字节和来源树保留在包详情。预算不提高，历史不被改写，
原有会话断言保持。详情先展示帧和载荷，再展示来源，4096节点限制下内容仍可见。

Windows实际CP936程序重开完整747帧，proto:http3 info:5776选#492。递进显示STREAM
8/offset5096/764字节，以及从流offset81重组的HTTP3帧5779字节/DATA5776字节；两者
不会混为一包。Type0在独立头005690中选0x0一个字节，Length5776选0x1–0x2的5690，
完整DATA选0x0–0x168f。查询/EOF/行详情、字节源、会话、80x30/180x48、退出和codepage
恢复检查通过。实页cells/PNG留本地。[本轮聚合](shark-public-corpus-20261025/quic-http3-source-summary.json)
绑定代码、程序、审计程序和原始结果哈希。

总体仍未完成：更广HTTP3字段语义、QPACK动态依赖及全部协议/会话、原始90/35差异和
未支持载体继续保留。所有捕获、密钥、EXE、原始导出与图片不入库，PR保持draft。


## HTTP3字段语义、同包多流隔离与Windows标题（2026-10-26）

QPACK能解码字段，不足以证明它们构成合法HTTP消息。新增20个先失败的控制覆盖缺失/
重复/错位伪头、请求响应方向、非法字段名/值、连接字段、TE、CONNECT与trailers。
按[RFC9114字段与控制数据](https://www.rfc-editor.org/rfc/rfc9114.html#section-4.2)做限定
base profile检查；未知伪头扩展明确unsupported。保留原字段、QPACK块及完整帧来源，
错误流保留拒绝状态，新流继续使用；坏请求不成为已确认的响应关联。
字段语法/基本消息形态通过不等于完整HTTP语义通过，范围说明随HEADERS一起展示。

补查发现一个真实影响多路复用的问题：字段错误使applyFrames提前退出，同一认证QUIC
包里的另一条STREAM被漏掉。现在HTTP字段错误归到单流，逐流保存原因，继续处理同包
其他STREAM及已证明边界的后续包；内存预算仍执行，连接级QUIC错误继续拒绝。
会话摘要也保留同包所有头节和各自流号，错误信息不会使正常流的头字段消失。
原QPACK组件例子只有authority/path，native恢复测试补上必需method/scheme后验证完整
请求，冻结捕获和原始组件示例不被重写。

| 新抓Windows实际socket实验 | 原始结果与核验范围 |
|---|---|
| 独立aioquic1.7.0原始QUIC双方 | 同一对端口复用3代连接，69次请求和69次响应字节/哈希在收发端一致；实际丢8个、延迟28个UDP报文。双方有意不执行HTTP服务器策略，用于声明的畸形控制，不冒充正常HTTP服务。 |
| 捕获完整性 | 271帧；以nonce探针被TShark实际观测作为启动屏障。旧固定sleep试验漏掉首代连接，capture01不作完整正例，认证只使用capture02。 |
| 字段与流 | 147个头节，13756项；93个base-shape-valid、51个malformed、3个unsupported。93包含坏消息较早的合法形态头节，不能说成93条完整合法消息。3代原生/参考连接独立，9个同包混合结果PDU逐流保留。所有所选原始字段与参考相同。 |
| 帧与来源 | 独立参考STREAM重组222帧（6 SETTINGS/147 HEADERS/69 DATA），720个来源范围/33729项通过。引用绑定原始pcap物理字节、端点、域和所属连接。6个独立expert-only JSON块被明确区分为注释，不能借它们猜连接。 |
| 原始Wireshark显示计数 | 6 SETTINGS/150 HEADERS/72 DATA；多出3+3视图，分别在#88/#90、#189/#192、#213/#216，连接/方向/流/完整帧哈希相同。原始计数与重复清单保留，没有改成零差异。 |
| 校验器反向控制 | 15种原生字段、类型、认证、ALPN、连接、流号、增删帧、错误与关联篡改全部拒绝；参考字段篡改另产生明确保留差异，共16项检出。 |

原生3240252e5a完整398.354s、CLI1.964s及字段隔离fuzz134236次/41.384s（2 workers/
1GiB）通过。fuzz是认证后算法测试，不代替线上AEAD。原747帧实际捕获重新跑SDK及两个
独立审计：57帧/24736项来源检查与27 DATA/10398项载荷检查继续通过，来源审计29种
篡改也全部拒绝。两份原始大pcap的当前CLI/SDK四份完整输出与b924逐字节相同，原始
90/35、TCP824/840流与808/735预览及其他参考指标保持。

实页又发现两处展示不一致：原生已确认h3，但消息字段错误使列表降成UDP；修复列表后，
worker因不持有私有认证依据，详情标题仍是UDP。现在从原生私有AEAD/所选ALPN快照
读取观察身份，公开Session/Profile/协议字串不能伪造它；消息状态仍是malformed或
unsupported。父进程保留发布时的身份供详情标题使用，不从worker公开快照重新猜测。
拒绝原因和所属流号放在大字段/来源树之前。API定向0.275s、CLI完整2.003s与worker
修复CLI完整1.951s通过。

实际CP936程序完整读271帧，fields http3.streamid ==84选#76/PDU81：列表、路径和
详情标题均为HTTP/3，开头明确错误属于stream80；同包stream84仍完整展开HEADERS。
它的73字节帧包含70字节QPACK块，点击Payload Length70准确高亮独立头014046中的
4046（0x1–0x2）。当前6匹配行、包/详情对应、字节源、会话、resize、退出及codepage
恢复均检查通过。[聚合证据](shark-public-corpus-20261025/http3-field-conformance-summary.json)
绑定源、程序、原始结果及实页cells/PNG哈希，原始资料留本地。

HTTP3仍有边界：完整URI/authority语义、Content-Length与DATA一致性、扩展CONNECT
协商、informational FIN、更多帧放置与QPACK动态依赖尚待覆盖。整体协议/流/会话/关键
字段一致性目标未完成，PR保持draft；pcap、密钥、EXE、原始导出和页面图不入库。


## HTTP3正文长度、FIN与实际双向丢包重组（2026-10-26）

原生d24e490d07新增按方向的正文状态：声明长度保留任意精度十进制，不经浮点或
uint64截断；等价重复值采用明确的接收端归一策略，原始字段照常保留。DATA超过声明
立即拒绝；不足只能在FIN最终位置和所有缺口排空后拒绝，相等前缀仍是pending-FIN。
HEAD/304表示长度、204、1xx、CONNECT隧道/失败响应和trailer分别判断；未观察请求
方法时，不凭空判定空HEAD响应长度错误。错误只隔离所属方向，同包其他流继续解析。
规则依据[RFC9110 §8.6](https://www.rfc-editor.org/rfc/rfc9110.html#section-8.6)与
[RFC9114 §4.1.2](https://www.rfc-editor.org/rfc/rfc9114.html#section-4.1.2)。

独立aioquic原始双方通过实际UDP捕获188帧，28种声明场景×3代同端点连接，84对请求/
响应、168方向；实际丢8/延迟20个报文，启动nonce观测屏障通过。双方刻意不执行HTTP
服务器合法性策略，畸形报文是声明控制。参考STREAM重组、实际收发字节、原生逐帧
类型/varint/字段/DATA/完整哈希和物理来源完成43720项检查：117个有效方向与51个
畸形方向按声明处理，3代连接互不污染。26种长度、FIN、归一、身份、头PDU、域、方向、
连接、字段、帧与参考原包篡改全部拒绝。此范围无所选原生差异，不代表全协议正确。

| 原始视图/逻辑帧 | SETTINGS | HEADERS | DATA | 其他 |
|---|---:|---:|---:|---:|
| Wireshark原始视图 | 6 | 202 | 173 | 2个type13921 |
| 原生交付逻辑帧 | 6 | 174 | 141 | 0 |

计数不强行归零。6个畸形请求的DATA尾帧在显式拒绝后不再做应用语义解析，原始
STREAM字节完整保留。参考#80/#147的type13921/length11369字节头76616c69实际为
"vali"，独立收发定位到stream104 offset69，位于完整HEADERS[0,96)内部；参考深层
视图错位，不能复制成原生未知帧。其余参考额外视图尚未全部逐一分类，原始数量保留。
另6条QUIC诊断是aioquic包后追加的未保护零字节，不能称已认证PADDING。
[RFC9000 §14.1](https://www.rfc-editor.org/rfc/rfc9000.html#section-14.1)允许Initial与
无效包合并后丢弃无效部分；当前拒绝尾部正确，但Admission文字仍过宽，另行修复。

完整原生392.905s/CLI2.007s和111962次正文隔离fuzz/40.437s通过；fuzz是认证后算法
测试。原747帧实际捕获重新执行，57应用帧/24736项来源和27 DATA/10398项继续通过。
两份原始大pcap四份完整CLI/SDK输出与b924逐字节相同，90/35等历史指标保持。

Windows实际CP936程序读完188帧，fields pdu.id ==58选择原包#53：HTTP/3身份、
UDP→QUIC→HTTP/3路径、声明18000/观测18000、Drained FIN=true、matched直接位于
详情开头；包/详情对应、字节源、会话、resize、退出/codepage恢复通过。
[聚合证据](shark-public-corpus-20261025/http3-body-conformance-summary.json)绑定源、
原始结果及实页哈希。pcap、密钥、EXE、原始导出与页面图留本地。

尚未完成：晚到请求方法对先前未验证响应的回溯、完整URI/authority、扩展协商、
更多帧放置与QPACK依赖。所有协议/流/会话一致性目标仍未完成，PR保持draft。


## QUIC未验证字节范围与完整大样本Windows复核（2026-10-26）

承接前节发现，原生982b02dcc9修正Admission提前设为已认证的问题：只有对应完整
包边界且Session明确通过认证的字节才保留authenticated-wire-and-observed-CID。
无法解析的范围、CID不一致、未认证候选和已观察的保护载荷分别标注。原始错误、
字节、来源与连接状态不丢弃；后续有效包仍沿原连接正常识别。Retry完整性通过仍不
等同包AEAD/对端身份认证，其独立Retry Integrity Verified字段保持。

| 完整输入 | SDK事件 | 实际变化 | 其余输出 |
|---|---:|---|---|
| 新抓188帧 | 203 | 6个包后零字节范围的Admission/摘要 | 逐字段完全相同 |
| 新抓747帧 | 636 | 0 | 完整SDK和CLI哈希均相同 |
| Weber原始51328帧 | 39283 | 10个未解析范围摘要/Admission，4个Retry的Admission | 字段、流、会话、端点和来源逐字段相同 |
| Sharon原始328127帧 | 245615 | 0 | 完整SDK和CLI哈希均相同 |

Weber的10个范围包括6个有效包后的尾部、4个offset0整份拒绝报文，不能全部称尾帧。
其PCAPNG原始块、接口26、IPv6/UDP字节范围完成415167项断言；此数字包含容器
断言，不是415167个协议字段。完整CLI只改变10行相应PDU摘要/Info/证据，其中
6行decoded改为partial/mixed datagram outcomes，4行继续malformed。其余379445行
与两套SDK的非Admission/摘要字段保持原值，原90/35、824/840 TCP流、808/735预览
未变。原生403.744s、最终CLI2.060s完整回归通过。两个范围审计分别9种篡改全拒绝，
HTTP3正文审计26种仍全拒绝。

HTTP3参考视图进一步全部分类：原始6 SETTINGS/202 HEADERS/173 DATA/2未知仍保留；
323个视图对应唯一真实应用帧，另58个重复视图（30 HEADERS/28 DATA），4个真实大帧
缺少深层参考视图（2 HEADERS/2 DATA，3代中的后2代stream104请求）。另外2个
未知视图仍精确定位到HEADERS内部的"vali"，不是真帧头。独立STREAM重组/实际收发
仍完整；原生6个显式拒绝后的DATA后缀保持未做语义解析，不混入零差异口径。新正文
审计44497项通过，原生321逻辑帧与参考原始383视图不是同一计量。

Windows实跑又修复诊断裁切：长摘要里的未验证尾缀会被详情栏裁掉；现在短提示
"QUIC remainder #…: … bytes · authentication not verified"放在字段树开头，Info
也以未验证范围开头。有效Initial自身仍明确AEAD通过，整份报文不再说全部验证。
最终4ff1100050在CP936下完整读51328帧，query fields pdu.id ==37385选#48060，
显示657字节未验证范围/offset543；188帧控制显示680字节/offset520。另选#53/PDU58
仍显示HTTP/3、18000声明/观测字节和完整FIN/matched。新EOF、查询、包/详情对应、
字节源、会话、resize、退出与codepage恢复均通过。大文件明确显示全局字段历史
partial提示，不能把已淘汰证据当成字段不存在。

[聚合证据](shark-public-corpus-20261025/quic-admission-scope-summary.json)绑定各源、
完整输出、范围/反向控制、程序与实页cells/PNG哈希。所有pcap、密钥、EXE、原始导出
和页面图继续只在本地。下一项已核实的边界是：FIN完全排空且末HTTP3帧被截断时，
应按[RFC9114 §7.1](https://www.rfc-editor.org/rfc/rfc9114.html#section-7.1)报告
H3_FRAME_ERROR连接级协议问题，当前仍返回NeedMore；尚未修复，不能宣布完备。


## HTTP3完整FIN后的残帧、同包恢复与连接级展示（2026-10-26）

原生ed3bf34ffe修复[RFC9114 §7.1](https://www.rfc-editor.org/rfc/rfc9114.html#section-7.1)
边界：已认证QUIC STREAM的最终位置与所有缺口排空后，末帧类型varint、长度varint
或负载仍不完整，应为H3_FRAME_ERROR(0x106)连接级错误，不能继续NeedMore。缺失前缀
尚未补齐、开放流和中途RESET不作该判定。新finding保留首PDU/流/方向/帧位置，
只增加128字节私有连接状态，不推测对端已关闭；后续单帧可解析不清除连接违规。

旧错误还会终止同一QUIC包的后续STREAM处理。现在保持传输/其他流观察进度，
不会把连接错误伪装为仅消息流错误；已解析帧、残帧原始字节、SHA与独立流字节
来源都保留。残帧的正文检查明确unverified-frame-layout，失败请求不再关联响应。
新增http3.connection.error_code/error_pdu/error_streamid/error_direction字段，
历史PDU用64位精确整数，原生投影使用私有快照；连接历史不是当前密文字节范围。

独立aioquic原始双方实际捕获161帧，12声明场景×3代同端点连接，36对请求/响应/72
方向，收发字节逐一相同，实际丢8/延迟17个UDP报文。双端刻意不执行HTTP端点合法性
策略，作为明确声明的畸形控制。参考QUIC STREAM重组、原始pcap物理字节与端口/域、
源PDU范围及实际收发核验32409项：30个方向有声明的帧错误，42个方向无残帧；
6个PDU同时保留新错误和其他已解码流。3代首错误分别PDU12/58/122，不能跨代继承。
21种错误码/范围/方向/字节/来源/会话/字段别名/增删帧/错误关联篡改全部拒绝。

| 指标 | 修复前 | 修复后 | 原始Wireshark/TShark视图 |
|---|---:|---:|---:|
| STREAM观察 | 227 | 227 | 参考STREAM逐字节检查 |
| 缺失原生STREAM来源 | 24 | 0 | 来源逐包绑定 |
| 完整应用帧（含控制） | 60 | 84 | 118个原始视图，非同一计量 |
| 完整HEADERS | 42 | 66 | 95原始视图 |
| 完整DATA | 12 | 12 | 17原始视图 |
| SETTINGS | 6 | 6 | 6原始视图 |
| 包列HTTP/3（参考拼写HTTP3） | 18 | 78 | 26 |
| 包列QUIC | 141 | 81 | 135 |
| 包列UDP | 2 | 2 | 0 |

原始计数不归零：TShark在已拥有端口上显式DecodeAs QUIC，2个启动探针也被标成QUIC；
原生按字节/上下文不承认探针为QUIC。85个原始列名差异含21个仅HTTP3/HTTP/3拼写
差异。另有应用重组显示层级策略差异；其中5个参考HTTP3/原生QUIC行
#72/#73/#125/#127/#129是没有新连续字节的STREAM重传，应用身份在提前跳过处丢失，
后续修复，不能说已全对齐。该参考运行没有H3_FRAME_ERROR专家消息；缺少专家消息
不能证明合法，残帧结论以已捕获FIN、实际字节和RFC为依据。

完整原生405.239s/CLI2.013s，新FIN/缺口fuzz107646次/40.408s通过。实时/会话
修复3eb430a2fc的CLI完整2.028s、worker修复aa252cab24的CLI完整2.056s通过。
两份原始大pcap在最终CLI与SDK的四份完整输出与前节逐字节相同，90/35、824/840流
和808/735预览未变；188/747帧此前实抓的SDK/CLI完整输出也不变。

Windows又实测修了3处不一致：实时增量列表绕过批量警告、会话摘要丢掉连接错误、
worker对新scope仍回退UDP标题。最终CP936程序读完161帧，typed query
fields http3.connection.error_code ==0x106匹配148行，首行#7/PDU12，列表/路径/详情
均为HTTP/3；后续ACK仍是QUIC但带同一连接警告。字段与会话页均在大树之前显示连接
违规。EOF、当前查询、包/详情匹配、字节源、会话、resize、退出/codepage恢复通过。
[聚合证据](shark-public-corpus-20261025/http3-fin-frame-summary.json)绑定各源、原始
结果/程序/实页哈希，所有pcap、密钥、EXE、原始导出与图片继续留本地。

仍待处理：超大声明长度在已完整FIN时的预算/格式错误优先级、上述5个重传行的粘性
展示、其他连接级帧/流创建错误、晚到方法、完整URI/authority、扩展协商及QPACK依赖。
整体目标未完成，PR保持draft。


## HTTP3重传身份、超大残帧与混合包字段归属（2026-10-27）

原生03b3a81a7a先在已认证STREAM重组后保留已知HTTP3流的重传/待缺口范围身份。
只在该流已有请求、控制、QPACK或已观察错误上下文时生效，不能从其他流或旧连接借用；
分类retransmission/buffered-range/mixed，http3.wire_only明确无新应用帧。正文计数、
逻辑帧、事务和来源不重放。开放流/新流缺前缀仍按实际证据保留，不能按ALPN给所有
任意密文贴HTTP3标签。原来#72/#73/#125/#127/#129五条退回QUIC的重传现均保留HTTP3。

同一版将已排空FIN的62位超大声明长度先按可见剩余字节检查，再执行资源限制和int
转换。声明(1<<60)+7但没有负载的DATA是H3_FRAME_ERROR，不能先ResourceExceeded
退役整个密码上下文并拒绝其他流。新实抓14场景×3代同端点、42对/84方向、161物理帧，
实际丢8/延迟17，双端字节与参考STREAM重组一致。旧程序3个资源超限导致122次退役CID
上下文拒绝；新版不再发生该退役链，36残帧方向和48无残帧方向均保留，90个完整消息帧
加6个SETTINGS。247次原始STREAM出现逐一对应，不只比较字节并集。

完整比较又发现一处真实错误：后处理的重传STREAM会覆盖同包新解析应用帧的包级
Stream ID，正文检查仍指向原流。原生0f40769b3b保留新帧的ID/类型，重传自己的ID留在
逐流明细；请求/控制及前后顺序回归均覆盖。原始实抓新PDU109的流20不再被重传流44
覆盖，前轮实抓PDU172的流20也不再被重传流40覆盖。比较程序最初确实拒绝了这些输出，
没有放宽检查掩盖覆盖。d746610ae5另修QUIC认证字段：HTTP3/DoQ细化仍导出底层
quic.packet.authenticated；原生用私有认证快照，公开字段修改不能伪造或清除认证；
认证是派生事实，不是当前密文偏移。Windows残帧Base64也解码到独立可选明文字节源。

| 复验范围 | 原始STREAM出现 | 重传/待缺口范围观察 | 独立检查 | 完整逻辑应用帧 |
|---|---:|---:|---:|---:|
| 新14场景/161帧 | 247 | 54/39 | 35,203 | 90消息+6控制 |
| 原12场景/161帧 | 227 | 46/39 | 34,653 | 78消息+6控制 |
| 原188帧正文实抓 | 保留 | 42个PDU细化 | 44,497正文检查 | 原321帧不变 |
| 原747帧多协议实抓 | 150 | 23个PDU细化 | 24,736来源+10,398 DATA检查 | 原57帧不变 |

33种独立审计篡改全部拒绝，含认证别名、原始STREAM增删、重传分类/标签、应用重放、
摘要流归属、连接错误及来源。三份旧实抓完整SDK/CLI再以有界字段归属比较，除声明的
身份/摘要和新字段外，原始字节、逻辑帧、来源、正文计数、端点、流、事务与关联相同。
原12场景包列HTTP3由78增至133：新增55个仅线格式观察，不是新增55个HTTP消息；
原188帧76→118、原747帧128→151同理。原始参考列计数和重复/部分应用视图仍保留，
不能用新的标签把总差异宣称归零。新实抓参考SETTINGS/HEADERS/DATA视图为6/115/17。

最终0f40769b3b完整原生419.292s、CLI1.972s通过。首版线格式修复完整原生461.414s、
CLI1.956s，超大长度fuzz110459次/40.481s、重传fuzz110347次/40.390s；fuzz属于
认证后的重组/应用输入，不是AEAD模糊测试。两个原始大pcap的最终四份完整导出与
上一节逐字节相同，90/35原始集合差异、824/840 TCP流和808/735预览未改。

最终Windows CP936实测三页：747帧#558/PDU451的纯重传列表/路径/详情保持HTTP3且
明确不重放应用；新161帧#10/PDU15的9字节残帧显示00d000000000000007独立来源；
#99/PDU109摘要和正文检查均为流20，连接违规仍在最前。全部实际EOF、typed查询、
行/详情同帧、字节源、会话、resize、q及936码页恢复通过。
[完整聚合证据](shark-public-corpus-20261025/http3-wire-identity-summary.json)绑定源码、
日志、实抓、参考、程序和实际控制台结果哈希；原始文件继续全部留本地。

未完成：其他帧位置/流创建错误、晚到方法重核、完整URI/authority/扩展/QPACK依赖、
一般QUIC未知上下文恢复、其他协议/字段/fuzz完整性。实页还发现QUIC CID以IPC Base64
显示，作为后续展示问题保留；本轮不声称所有核心协议完备，PR继续draft。

## 2026-10-27：NTP 时间版本、控制分片与错误分发

本轮补的是原生解析、字段来源、控制请求归属和实际页面。目录中的 NTP 仍标记为
partial，不能据此声称 NTS、全部扩展、私有模式或时间同步算法已经实现。
代码、审计程序和聚合结果入库；输入及完整逐帧证据继续留在
`C:/tmp/yak-shark-ntp-20261026`，不提交 pcap、密钥、EXE、原始 JSONL 或控制台图片。

### 实际修复

- 原入口仅覆盖 v3/v4 client/server 的 48 字节布局。现在接入 v1 独立布局、v2-v4
  模式 1..5 和服务选择下的 v2-v4 mode6。控制头是 12 字节，不再被当成时间戳头。
  规则目录、嵌入规则包、原生分发和注册字段使用同一个实际 `NTP` 入口。
- 按 [RFC1059](https://www.rfc-editor.org/rfc/rfc1059.html)，v1 低三位是保留位，
  不能显示为现代 Mode；其距离为 unsigned16.16、漂移为 unsigned0.32。
  [RFC1119](https://www.rfc-editor.org/rfc/rfc1119.pdf) 的 v2 距离/离散度也使用
  unsigned16.16，分别展示。现代有符号 root delay 不再套到旧版本上。
  四组时间戳保留原始 8 字节及 seconds/fraction，不猜测 era、UTC 或时钟同步状态。
- 按 [RFC9327](https://www.rfc-editor.org/rfc/rfc9327.html) 核对控制头、计数、零填充、
  状态字和系统变量查询返回选中 peer ID 的例外。请求中的保留状态字不再显示为服务器
  时钟状态；时钟状态字保留高/低字节，不猜测未协商变体的硬件状态。
- 控制分片支持尾片先到、缺口后补、相同重叠和重传。冲突重叠、最终长度变化、片间
  状态/关联号变化会拒绝重组。完整字节与语义有效性分别记录；read-status 关联列表
  的格式错误或集合预算不足不会被写成完整字段解析。
- 请求归属按捕获域、双向端点、版本和 sequence 建立，再核对 opcode/association。
  30 秒是明确标注的本地 capture-time 租约。重复请求不延长租约；时钟倒退/缺失、
  完成后复用序号、内容变化和先响应后请求均拒绝借用旧所有者。捕获期间保留隔离历史，
  耗尽预算后拒绝给未跟踪身份建立新归属。损坏请求也不能借用旧请求的错误响应。
- 实抓暴露了 UDP 事件 ID 在发布时才分配的问题：原先请求所有者和重组来源写入了 0。
  现在在推进控制状态前预留最终 ID。普通与 deferred 完整抓包入口均有回归。
- 第二份公开大包的 #140530 还暴露了旧 NTP 分发绕过新准入的问题：非 123 端口的
  780 字节数据报仅凭旧前缀检查被标为 NTP。现在移除该 UDP 旧路径。独立参考链为
  `eth:ethertype:ip:udp:mpls:tcp:smtp:data-text-lines`；本轮不因此宣称已验证这条
  fuzz 链的全部内层语义，原始 UDP 和来源仍保留。
- 递进页把当前片段与重组正文分成不同字节源，保留逐片 PDU/物理帧来源和覆盖范围。
  未知尾部显示为独立的 unselected trailer，只有选中长度的认证信封才显示为未验证的
  authenticator。完整控制头/计数数据加未知尾部保留 NTP 头部身份及限制说明；损坏的
  count 不提升为已验证协议消息。

### 原始大包逐条对照

| 口径 | Weber | Sharon |
|---|---:|---:|
| 原始物理帧 | 51,328 | 328,127 |
| 参考 NTP filter 行 | 1,363 | 1,900 |
| 参考物理数据源中的 NTP 视图 | 1,331 | 1,891 |
| 参考派生数据源中的视图，单列保留 | 0 | 2 |
| 没有 root raw 的引用/嵌套视图，单列保留 | 32 | 7 |
| 原生匹配的选中视图 | 1,331 | 1,213 |
| 其中新增对称模式 / 控制头 | 38 / 104 | 38 / 104 |
| 匹配消息之外的原生 NTP 多报 | 0 | 0 |
| 已完成控制响应 / 等待缺口或尾片 / 请求等非组装消息 | 40 / 24 / 40 | 40 / 24 / 40 |
| 独立字节、字段、来源与归属检查 | 456,720 | 2,106,620 |

Sharon 的 678 条未选择物理视图逐条保留，原因合计为：不支持版本 302、v1 非零保留位
277、未选择 mode 32、控制 LI 非零 49、短时间头 5、短控制头 2、未选择的保留 opcode 1、
零 sequence 自动准入未选择 3、stratum 超出当前版本档位 3、外层 UDP 长度超出捕获 3、
UDP 控制档位之外的 TCP 布局 1。没有未解释的选中档位漏项；这些拒绝和未支持项没有
被改写成匹配。两条派生源视图也没有把其逻辑偏移冒充原始帧偏移。

审计同时核对物理 PCAPNG Section/Interface、VLAN、IP 隧道和 express mPacket 的
前导/FCS。其中每个公开文件均有 599 条选中 NTP 经 mPacket 承载；格式对照
[Wireshark packet-fpp.c](https://gitlab.com/wireshark/wireshark/-/raw/v4.6.9/epan/dissectors/packet-fpp.c)，
独立程序用原始字节和 CRC 校验，不仅比较原生端公开的来源标签。

SDK 完整导出仍分别为 39,283 / 245,615 行，CLI 保留全部 51,328 / 328,127 帧。
Weber 新增 142 条 NTP；Sharon 新增 142 条并移除上述 1 条旧误报。逐记录不变约束检查
要求非 NTP 记录、物理/网络/传输列、PDU 身份、捕获域和原有连接数据保持一致，
没有通过协议标签归一化凑数。原先的 90/35 字段集合口径差异也没有被此次修复抹去。

### 两轮实际 Windows UDP 收发

作者控制对端分别实际捕获 205 和 251 帧，第二轮还加入 v1/v2 高位定点数、返回选中
peer、片间 peer 变化以及损坏请求之后的错误响应。每轮代理实际丢弃 3 个报文、延迟
3 个报文；顺序、重传、缺口补齐、复用、冲突、端点隔离和未知尾部均由真实 socket
收发及接收回执核对，不能把离线删改 pcap 说成网络实验。零/保留 sequence/opcode
只在这些已声明的测试端口通过双边一致 DecodeAs 检查；公开大包没有 DecodeAs。

测试对端不是 ntpd。对 Windows 现有 `127.0.0.1:123` 的单独查询两次均在 2 秒内没有
响应，不能把它们说成成功的生产服务交换。生产时间/控制报文来自上述公开原始捕获。
独立校验器还分别拒绝 31 / 35 种被故意损坏的导出，包括字段、符号/缩放、请求所有者、
事务、来源父 PDU/帧/域/字节缺口、重放正文、认证断言、旧版本误释及选中 peer 归属。

Windows 实际 CP936 控制台检查包括 EOF 后查询、当前选中行/详情一致、字段高亮、
独立重组字节源、会话/证据、80×30/180×48 调整、退出和码页恢复。#47/PDU47 在补到
中间片段时展示完整 9 字节正文，同时保留当前片段的 3 字节；#72 的请求关联号 0 与
响应关联号 7 分开；#10 的 v1 保留位/距离/漂移不冒充现代 Mode/root dispersion。
未知尾部的列表身份另有完整头与损坏计数的正反例。

### 验证与边界

原生字段/重组版本完整测试 419.569s、CLI 2.181s，bin-parser 116.035s、parser 5.828s、
base 0.525s、stream_parser 4.941s 通过。入口去误报后的完整原生 434.307s、CLI 1.969s
通过；控制头展示修正再通过完整 CLI 1.959s。控制重组 fuzz 91,902 次/40.423s，
线格式范围/语义隔离 fuzz 16,092 次/41.409s 通过。历史冻结输入及答案哈希未改写；
原本按旧档位拒绝的两个合法 v2 输入有单列的新增范围断言，保留原始历史记录。

仍未覆盖：时间消息的事务归属和时钟纪律、时间扩展/NTS、私有 mode7、控制认证算法、
全部文本变量语义、TCP 控制承载和其他派生源载体。可见 MAC、状态字、关联列表、
重组完成或请求 tuple 都不证明对端身份、实际交付或时钟同步。总体目标仍未全部达成，
PR 保持 draft。复现入口为 `ntp_control_live.py`、`ntp_wire_audit.py`、
`ntp_negative_controls.py` 与 `ntp_export_invariants.py`；原始逐条审计文件保留在本地。

最终标识前置版本 8f7976af42 通过完整 CLI 2.087s。两个原始大包的最终 CLI 与入口
修正后逐字节相同；两轮小捕获各仅有 3 个完整控制头加未知尾部的列表行提升为 NTP，
所有 PDU、字段、来源和其他行保持相同。CP936 实页再次检查 #55/PDU55，操作和
序号可见，尾部限制单列；没有通过把损坏 count 改成 NTP 来清除诊断。
[NTP 聚合证据](shark-public-corpus-20261025/ntp-wire-summary.json) 绑定源码版本、
独立校验、完整导出不变约束、原始拒绝原因、日志及实际控制台快照哈希。

## 2026-10-27：QUIC 二进制字段、零 CID 连接复用与 Windows 实页

源码 `2b7441c8a2` 修复规则/快速路径将 DCID、SCID、Token 当 UTF-8 字符串的损失：
Weber 的 22 个成功规则视图中，22 个 DCID、21 个 SCID 原本无法经 JSON 还原原始字节。
现保留自有二进制数组，IPC 用 Base64 无损传输，实际页面用十六进制展示。
已固定的历史输入及答案未改写；测试只对字节绑定的旧损失字符串和新增范围作单列审阅。
长头 Version、CID 长度/CID、v1 包类型有独立原生类型和逻辑 PDU 范围；短头只展示
已知接收端上下文中的 DCID，明确标记 Version/SCID/长度未携带。
空长头 CID 表示确实存在的零字节字段，不虚构高亮；已解密包号属于派生值，
必须有私有原生认证事实，不能将密文包号位置当作明文包号的值范围。
协议目录明确保持 partial：v2 的选定规则布局测试不等于 v2 原生入口/密码支持，
更宽 Token varint 档位也未宣称完整。

新 Windows 实际 UDP 捕获共 101 帧，三个相同端点代际依次使用 0/0/20 字节本地 CID，
实际 6 次丢包、12 次延迟，9 次请求/响应均有收包记录。测试端为明确声明的 aioquic
1.7.0 原始 QUIC 对端：零 CID 档位省略非法的 NEW_CONNECTION_ID，并为第一个客户端
Initial 保留随机 8 字节 DCID；它不代表未经修改的生产 HTTP/3 服务。
两轮先前失败/不合规端点控制保留，不作为正确输入证明。
这些约束来自 [RFC9000 §5.1](https://www.rfc-editor.org/rfc/rfc9000.html#section-5.1)、
[§7.2](https://www.rfc-editor.org/rfc/rfc9000.html#section-7.2) 和
[§19.15](https://www.rfc-editor.org/rfc/rfc9000.html#section-19.15)。

真实重放找出两个归属错误，已由 `ce45da597a` / `a035f1101d` 修正：
旧零 CID 连接收到关闭后仍参与新服务器 Initial 的 tuple 匹配；旧关闭包丢失时，
同一 tuple 的旧零 CID 又使新 20 字节 CID 短头被当作歧义而退回未知 UDP。
现在先收集竞争的 CID/tuple 上下文，通过不改变包号、密钥代际或 STREAM 状态的
保护校验选出唯一归属，再正常交付；未知密钥、损坏包或多个相同密钥上下文
不会借用旧 FlowID/事务。迟到旧包可检查，已关闭连接不会因此续租密钥状态。

| 实际捕获结果 | 修复前 | 最终 |
| --- | ---: | ---: |
| 未知 UDP PDU | 48 | 2 |
| QUIC / HTTP3 PDU | 59 / 7 | 89 / 25 |
| 误用旧上下文导致的保护校验失败 | 22 | 0 |
| 与参考精确匹配的包视图 | 66 | 114 |
| 已认证包号与连接归属检查 | 部分 | 106 / 106 |

最终头部审计 6,270 条检查，字段值/范围差异为 0；三个独立连接的 33 个 HTTP/3 帧
（6 SETTINGS、18 HEADERS、9 DATA）及 216 条来源范围通过 14,445 条独立检查。
34 个特意损坏原始字节、摘要、域、PDU 重复/缺失、CID、范围、包号或连接归属的
控制全部被拒绝。未保护尾部的 8 段零字节仍单列诊断；参考另外两个 QUIC 候选
是强制 DecodeAs 后的抓包就绪 nonce，没有已解密包号，原生保留为未知 UDP。
这两个参考候选未被归并到匹配成功数。

两个原始大包完整重放 379,455 帧，全部记录/流/事务/正文及来源不变约束通过：
Weber 39,283 个 SDK 记录中仅 78 个新增头字段/无损二进制投影，51,328 个 CLI 行中
仅 54 行包含新增字段；Sharon 的 245,615 个 SDK 记录和 328,127 个 CLI 行逐字节不变。
既有 161/747/188 帧真实捕获也重新完整重放，所有其他输出保持一致。
完整原生 439.800s / CLI 1.966s 通过，新增归属探针 fuzz 108,838 次/40.421s 通过；
先前同一头字段源码的完整规则/基础解析器检查与 85,678 次范围 fuzz 结果保持适用。

实际 Windows CP936 实页分别检查空长 CID、HTTP3 短头 20 字节 CID、字段/字节选择、
来源切换、会话/证据、80×30/180×48 缩放、退出和码页恢复。头字段组已前置，
HTTP3 递进路径保持 `Loopback › IPv4 › UDP › QUIC › HTTP/3`。
PNG 从实际控制台字符/属性缓冲区渲染，原始快照及图片只保存在本地。

本轮不是整体 QUIC 验收通过：Weber 仍有 4 个 CRYPTO 冲突整数据报诊断（参考拆为
8 个视图，其中 4 个 Initial 有包号）及另外 13 个保护/头候选待核实；Sharon 的
1,027 个未选参考视图包含大量畸形/fuzz/旧档位候选，另有 10 个派生来源视图，
不能将随机长头位置的四字节数都计作有效 QUIC 版本。Native v2 仍未支持。
后续逐项检查继续保留这些差异；公开的
[QUIC 头部聚合证据](shark-public-corpus-20261025/quic-header-summary.json)
绑定源码、输入/导出/审计器哈希及所有范围边界。抓包、密钥、原始 JSON/JSONL、
可执行文件和截图均未提交。

## 2026-10-27：Retry 身份、冲突重传与独立 Initial 解密

前一节待核实的四个 Weber CRYPTO 冲突已逐个独立解密：Python 使用 RFC9001 的
HKDF、AES-GCM 和头保护，验证 22 个 Initial、4 个公开 Retry 完整性标签和实际
CRYPTO 字节，未调用 Yak 解码器。#48062/#48074/#48096/#48109 与各自初始
ClientHello 在相同 offset 上分别改变 280/279/280/279 个字节，包含随机数。
[RFC9000 §17.2.5.3](https://www.rfc-editor.org/rfc/rfc9000.html#section-17.2.5.3)
要求 Retry 后沿用握手消息；这四处属于已核实的协议不合规，不能通过清空重组/TLS
状态接受新 ClientHello 来消除差异。其余 13 个未选参考包均没有已解密包号及连接
归属；没有从 UDP tuple 或密文前缀推断新的 CID/FlowID。

同时确实修复了三个实现错误（`273e8f5578`）：Retry SCID 与原始 DCID 相同也可被
接受；Retry 后客户端可换初始 SCID；已拒绝的 CRYPTO 包再次出现时，原来的
duplicate 快速路径会返回成功。现在严格校验两个 CID 约束，按方向/包号空间在
128 项观测窗口内保留拒绝原因，计入状态内存预算；重复包返回独立错误副本，
不会重交付帧或让外部修改返回错误后污染后续诊断。冻结样本及答案没有改写。

新增两轮各 136 帧的 Windows 实际 UDP Retry 捕获，均复用同一对 socket 三代，
每轮 3 次 Retry、8 次实际丢包、16 次延迟、9 组请求/响应收包记录。

| 对端控制 | Retry 后 ClientHello | 原生及独立校验结果 |
| --- | --- | --- |
| 未修改 aioquic 1.7.0 | 三代均改变 280 字节 | 3 个实际协议冲突，保持拒绝；收到应用响应不等于合规 |
| 声明保留原始 TLS 状态/ClientHello | 三代逐字节相同 | 140 个已保护包号/归属、33 个应用帧、18 组头字段通过 |

正例是明确声明的对端适配，不冒充未经修改的生产 HTTP/3 端点。原生头部、
STREAM 来源和字段分别有 8,800、18,009、10,790 条检查。实际发出的请求/响应
与参考 STREAM 重组逐字节一致；参考 HTTP3 有 2 个头字段视图不可用和 2 个
12400 类型视图，保留这些差异，没有替换成原生解析来宣称参考字段一致。
独立密码检查另外通过 11 个特意损坏的密文、头保护、AAD、Retry 标签、原始 CID
复用、包号、CRYPTO、拒绝诊断及缺失 transcript 控制。为单独检验密码层而重绑的
证书哈希明确标为测试输入，不作为原始物理抓包证书。

实际坏包的离线复制控制抽取原捕获 #3/#4/#5/#5，保留每个物理包原始字节，
明确不是新增真实流量。旧版本把最后一次冲突 Initial 记为 decoded duplicate；
新版本仍为 malformed/CRYPTO 冲突。另有直接 UDP 入口、错误副本隔离和 140 次
拒绝后淘汰的测试；拒绝状态 fuzz 97,356 次/40.418s 通过。

`290d68368a` 去掉 Retry/Version Negotiation 上由内部零值误带的 Initial 包号空间；
二者不携带包号，Retry 的公开完整性校验也不等于包保护或对端身份认证。
Version Negotiation 缺字段在单元层校验，原生 VN 入口/兼容版本协商仍未宣称支持。
`7aa727b978` 修复纯拒绝行将同一句 Info 重复显示两次的问题，混合数据报的范围
警告仍保留。完整两个大包重放并逐项约束后，Weber SDK 仅有 4 个 Retry 元数据
删除、CLI 仅有对应 4 个冲突行的重复文字删除；Sharon 全部输出逐字节不变。
流、会话、原始字节、所有其他字段及来源不变。实际正反例/零 CID 捕获也完成
相同完整导出约束。旧的 69 操作字段审计默认档位另通过 13,756 条回归检查。

原生功能版本完整测试 400.217s / CLI 2.020s 通过；最后缺字段修正通过所有 QUIC
及 FirstBatchT03 测试 1.488s、完整 CLI 2.262s；纯展示修正完整 CLI 2.171s。
Windows 实际 CP936 检查包括 Retry 的 false 包保护/false 对端身份与独立状态、
复制坏包的持续拒绝、以及完整 51,328 帧大包中的原始 #48062/PDU37387：查询、
当前行/详情、字节源、会话、缩放、退出及码页恢复通过。原始字符/属性缓冲区
中的协议列、IPv6 地址和错误文字再次核对；截图为这些真实缓冲区的渲染。

[Retry 聚合证据](shark-public-corpus-20261025/quic-retry-summary.json) 绑定各源码版本、
输入/导出/审计器/Windows 快照哈希及未选范围。原生 v2、兼容 VN、一般包号复用/
stateless reset、缺失 CID/密钥上下文、其他载体/fuzz 和整体协议完备性仍需后续工作。

## 2026-10-27：原生 QUIC v2、跨版本代际和独立密钥更新验算

`5be60cad5e` 增加固定版本 QUIC v2 原生入口：按
[RFC9369 §3](https://www.rfc-editor.org/rfc/rfc9369.html#section-3)
使用真实版本字 `0x6b3343cf`、v2 长头类型、Initial salt、HKDF 标签和 Retry
完整性参数，三个 AEAD 套件及密钥更新均保留版本边界。整数 `2` 不是该线上的
版本值。公开 RFC Initial、服务器 Initial、Retry、ChaCha20 包及下一代 secret
向量直接通过原生入口；同一 TLS secret 不能借用另一版本的包保护状态。
兼容版本协商尚未实现，已建立代际中的版本变化明确拒绝，不能宣称 RFC9369
端点或 `version_information` 全面合规。

同时修复 Retry 的归属探针与实际解包路径不一致：缺失客户端 post-Retry Initial
时，服务器 Initial 可以在不改变活连接的探针中使用已观测 Retry 密钥；随后真正
消费时再提交状态。post-Retry 服务器采用零 CID 时，后续客户端 Initial 也能验证
已观测的服务器 CID 和 token。同一 tuple 上竞争的旧零 CID 代际不会吞掉新 Retry；
公开 Retry 标签仅选择已观测上下文，不被标作包保护或对端身份认证。
这些场景有原生 UDP 入口、旧代际仍活跃、客户端 Initial 丢失、双版本和零 CID 测试。

新增三轮 Windows 实际 UDP 捕获；均有三次 Retry、三代相同端口、12 组逐字节
请求/响应收包记录、三种 AEAD 和实际 key phase 变化。零 CID、保留首个 ClientHello
及 v2 更新标签的对端适配均在 manifest 中声明，没有修改安装的 aioquic 库。

| 实际捕获 | 帧数 | 实际丢包 / 延迟 | 独立 1-RTT 解密 | 结果 |
| --- | ---: | ---: | ---: | --- |
| v2，测试端保留原始更新标签 | 107 | 6 / 12 | 91 | 7 个包仅错误的 v1 KU 标签可解密，原生保持拒绝 |
| v2，声明使用 RFC9369 更新标签 | 123 | 6 / 14 | 107 | 全部按 RFC 解密，125 个保护包号与参考一致 |
| 同一 tuple 依次 v1 / v2 / v1 | 122 | 7 / 14 | 106 | 三代独立，124 个保护包号与参考一致 |

新增独立 Python 1-RTT 校验器只使用已绑定的物理包、独立 Initial 解密得到的
ClientHello random/ServerHello suite、观察到的 CID 和 TLS keylog，独立完成
HKDF、AES/ChaCha20 头保护及 AEAD。负例中的 7 包确实使用 `quic ku` 而非
`quicv2 ku`；收到响应不能证明这种密钥更新符合 RFC。原生没有增加错误标签回退。
10 个特意损坏 tag、HP、AAD/CID、包号、原生保护标记、版本、CID 长度或 secret
的独立控制全部拒绝。v1/v2 头字段控制分别拒绝 37 个损坏项。

两个正例分别验证全部 42 个 HTTP3 帧（6 SETTINGS、24 HEADERS、12 DATA）、
246 段字节来源及 24 组有序字段，连接、来源、请求关联与真实收包字节一致。
参考原始 HTTP3 仍有重传视图和两个 12400 类型视图；两个正例各有 2 组参考
头字段视图不可用，其余可比的有序原始字段差异为 0。未用原生结果补成参考成功。
强制 DecodeAs 的三个
就绪 nonce 保留为未选 UDP；未保护的零尾部仍为诊断，不能归入已认证 STREAM。

`d608c424c8` 对齐参考的版本专用字段：v1 的 `quic.long.packet_type` 与 v2 的
`quic.long.packet_type_v2` 分开，二者均绑定真实首字节。短头不凭连接记忆制造
`Version`/SCID 字段。Windows 详情显示 `QUIC v2 (0x6b3343cf)`；实际 CP936 检查
空长 CID 和更新密钥后的 HTTP3，请求递进路径、当前行/详情、字节源、会话、
缩放、退出与码页恢复均通过。页面图片来自未经改写的实际控制台字符/属性缓冲区。

两个原始大包完整重跑 379,455 帧，SDK 的 39,283 / 245,615 条记录及 CLI 的
51,328 / 328,127 行与已推送 Retry 基线逐字节一致，包含流、事务、字节和字段。
这证明此次 v2 扩展未引入现有输出回归，不等于剩余差异全部解决。Weber 的四个
真实 CRYPTO 冲突和其他未选参考包、Sharon 的 1,027 个未选/10 个派生参考视图、
原有 90/35 项逐帧归属/字段范围差异仍如实保留。

原生完整测试 400.229s、CLI 1.985s；版本隔离 fuzz 101,789 次/40.461s；
字段别名后的全部 QUIC/FirstBatchT03 测试 1.468s、最终完整 CLI 1.971s 通过。
新增独立校验器另对既有 v1 正例验证 122 个 1-RTT 包及原有 33 帧来源图。
[QUIC v2 聚合证据](shark-public-corpus-20261025/quic-v2-summary.json) 绑定源码、
抓包/导出/审计器及 Windows 缓冲区哈希。pcap、keylog、证书、原始导出和图片均
只存本地。greased fixed bit、票据/0-RTT 上下文、一般包号复用/stateless reset、
兼容协商、完整帧角色规则、宽 token varint 规则投影、其他载体和协议仍未全部覆盖。

## 2026-10-27：长 Initial token、可选字节范围与目录计数复核

`ae39f07afc` 补齐前一节的 token 规则投影缺口。Initial 的 1/2/4/8 字节长度编码
均能解出语义长度及完整二进制 token；按
[RFC9000 §16](https://www.rfc-editor.org/rfc/rfc9000.html#section-16)
接受非最短整数编码。现有一字节字段形状保持不变，宽编码另外保留两位编码前缀。
显式 `QUICInitialTokenFields` 入口拒绝缺失长度、截断长度/内容和超出所选 20 KiB
上限的值；历史 `QUIC` 薄封装的失败回退仍是显式不透明区域，不代表成功解密。
原生入口在投影之前验证完整头部，token 内容本身仍不证明地址、票据或身份。

新增 `quic.token_length`、`quic.token`、`quic.retry_token`、
`quic.retry_integrity_tag`，分别来自私有原生观察和确切 PDU 字节范围。长度保留
无损整数表示，空 token 的存在与字段缺失分开；短头和不相符包型不会继承这些字段。
Initial token 与 Retry 的无长度前缀 token 分别呈现，公开 Retry tag 不等于包保护。
SDK/worker JSON 使用无损字节，Windows 显示十六进制并选择原始字节，未把旧封装
中名为 Protected Payload 的不透明兼容区域宣称为已认证明文或精确密文边界。

新一轮实际 Windows 捕获 106 帧，同端口依次 v1/v2/v1、三种 AEAD、零/20 字节
本地 CID、3 次 Retry、6 次实际丢包、11 次延迟以及 12 组逐字节请求/响应收包记录。
token 为 61/189/585 字节，包含一/二字节长度编码。测试端明确声明为已发行 token
附加不透明随机后缀；只接受完全相同的已发行外壳，再调用原 handler 验证内部的
地址/CID 绑定。没有用宽容解析器替代对端校验，4/8 字节和非最短编码覆盖来自
独立字节控制及原生保护单元，未冒充此次实际网络采用的编码。

新捕获的 121 个参考包视图精确匹配，107 个保护包号一致；独立验算 12 个
Initial、3 个公开 Retry tag 和 89 个 1-RTT 包。42 个应用帧、237 段来源、24 组
有序字段及原始收包字节一致，可比参考原始字段差异为 0，另有 1 组参考头字段
视图不可用，继续单列。19 个 token/范围/编码/精度/包型串用控制及 37 个通用
头部损坏控制均拒绝。旧版有 4 条宽 token 封装回退，现已正确拆出；完整导出
检查只允许新增 token 事实/字段范围和这项明确的边界修正。

两个大包再次完整重跑 379,455 帧。Weber 中实际发现 #48066/#48078/#48100/#48114
四条 101 字节 token 原来未接入规则投影，现已与参考两字节长度及内容对齐。
其 SDK 仅 22 条记录新增 token 事实/投影（其中 4 条修正旧封装边界），CLI 仅
22 行新增相应字段；其余流、事务、应用内容、字节来源和原有字段完全相同。
Sharon 的全部 SDK/CLI 导出逐字节相同。旧的 CRYPTO 冲突、未选/fuzz/派生参考
视图及此前归属口径差异仍保留，不据此宣称所有协议已完成。

规则目录新增显式 token 入口，QUIC 路线图由旧 `done` 改为 `partial`，生成文档
同步更新。冻结清单的 `ndpi-quic-length-boundary` 仍保留原来状态和原包哈希；
`601a84aa29` 只在验证当前可执行范围后接受该条历史状态迁移。完整语料继续是
552 个抓包、58,533 帧和 341 个映射协议，原有门槛没有降低。

完整原生 425.040s、CLI 2.073s、规则库最终全量 130.994s 通过；token extent/
隔离 fuzz 4,078 次/41.385s 通过。规则另检查所有 8 个全局位偏移和后继字段，
原始字节可完整回写。实际 Windows CP936 查看同一 585 字节 token 的 Retry 和
Initial，长度编码/本体范围、详情、字节源、会话、缩放、退出与码页恢复均通过。
图片来自实际字符/属性缓冲区，未进入 Git。

[token 聚合证据](shark-public-corpus-20261025/quic-token-summary.json) 绑定源码、
输入/导出/校验器/本地可执行文件/Windows 缓冲区哈希。
[当前计数](shark-public-corpus-20261025/current-counts.md) 与
[计数聚合](shark-public-corpus-20261025/current-counts-summary.json) 刷新本次完整重放的
原始列标签，原始基线表保留并明确链接新版。标签相同不证明正确，大小写或子协议
不同也不直接构成漏识别；字段/重组/流及未选范围仍以各项证据为准。

## 2026-10-27：已认证 QUIC 违规帧、连接归属和诊断一致性

`8b024ca521` 将包保护成功与协议内容有效分开。按
[RFC9000 §12.4/12.5](https://www.rfc-editor.org/rfc/rfc9000.html#section-12.4)、
[§19.7](https://www.rfc-editor.org/rfc/rfc9000.html#section-19.7) 和
[§19.20](https://www.rfc-editor.org/rfc/rfc9000.html#section-19.20)，补齐所选包型/方向的
帧限制、空 NEW_TOKEN、空保护载荷、保留位和非最短帧类型编码校验。整数参数仍可使用
非最短编码；帧类型的最短编码规则单独执行。未选择的规范编码扩展保持
`UnsupportedFeature`，不冒充已解码，也不一概归为格式错误。

资源拒绝继续是 `limited`：PING 后的 PADDING 不能绕过帧预算，连续 padding 仍压缩
成一个观察；token 解析器超过所选 20 KiB 上限保留资源错误。该 token 控制没有证明
超上限的新候选一定进入原生 QUIC 会话。格式违规与未支持载荷可保留有界的已认证
明文、摘要和明确的“未交付协议状态”说明，不更新 TLS、STREAM、CID 或密钥阶段。
格式违规使用原有 128 个不同 PN 的窗口；重复观察不会挤掉其他包或变成成功交付。

新的实际 Windows UDP 捕获为 151 帧，同一端口依次 v1/v2/v1，三种 AEAD、8/0/20
字节本地 CID、3 次 Retry、6 次实际丢包、12 次延迟和 15 组逐字节请求/响应收包记录。
每代另实际发送 9 个声明的控制数据报，共 27 包；接收 socket 按精确 wire digest
确认收到并主动排空，之后再执行真实恢复请求。**排空控制不证明对端接受违规帧。**
既有保留首个 ClientHello、v2 KU 标签及宽 token 的对端控制继续在 manifest 中声明。

| 控制内容 | 旧原生结果 | 本次结果 | 参考原始视图 |
| --- | --- | --- | --- |
| 客户端 HANDSHAKE_DONE / NEW_TOKEN、服务器空 NEW_TOKEN、空载荷 | 共 12 包误当成功 | 认证事实保留，全部拒绝协议内容 | 前三类展开帧；空载荷报密文过短 |
| 非零保留位 | 3 包丢失认证事实 | 认证成功与格式违规同时保留 | 展开 PING，保留非零位 |
| 未选择的 `0x1f` / `0x40` 扩展 | 6 包统称 malformed | 明确未支持，不提交帧 | 保留对应类型；`0x40` 有 Unknown Frame Type |
| 非最短 PING 类型、合法服务器 token | 拒绝 / 成功 | 对应边界继续成立 | 保留原始类型视图 |

`c3e5dcd8c4` 修复真实控制捕获额外发现的连接归属问题：零 CID 复用 tuple 的 ownership
probe 原来把保留位有效性当作认证条件。现只用唯一 AEAD 匹配确定已捕获代际，再由
消费路径拒绝协议违规；probe 不提交 PN 或密钥。未知/损坏 tag 和多个可认证代际仍
不猜连接。三代全部 24 个拒绝控制和 3 个正向对照保持各自 flow，之后的实际请求均
正常递进到 HTTP/3。

独立检查 12 个 Initial、3 个公开 Retry tag、135 个 1-RTT 包；167 个原始参考包视图
精确匹配。150 个可比的保护 PN 一致，另外 3 条空载荷包的参考 PN 不可用，单列保留。
51 个 HTTP3 帧（6 SETTINGS、30 HEADERS、15 DATA）、261 段字节来源和 30 组有序
字段与实际收包一致；可比原始字段差异为 0，仍有 1 组参考头字段视图不可用。
未把 Wireshark 的无专家提示等同于 RFC 符合性，也未把这些参考缺口记作比较通过。

`1a665e4f4c`/`97314ee256` 修复实际页面里的 24 个重复摘要，前置包级错误，并显示
“AEAD verified; not delivered”的明文来源。短头的零长度 DCID 明确来自观察到的连接，
没有范围证据时显示上下文不可用；零字节载荷不高亮无关捕获字节。实际 CP936 检查
违规包、空载荷和恢复请求的字段/QPACK 来源，4 组检查均包含会话、缩放、退出及
代码页恢复。图片来自未经改写的真实字符/属性缓冲区，未提交仓库。

同一最终 EXE 另完整打开原始 51,328 帧 Weber 文件，在实际页面查询 PDU #37387 /
原帧 #48062，确认冲突包的未解析余量没有继承邻包认证。字段、字节源、会话、
80×30/180×48 缩放、退出及 CP936 恢复均通过；该第五组原始缓冲区也绑定到聚合证据。

还发现 CLI 精简 PDU 导出只保留 malformed/incomplete UDP 的诊断，导致未支持扩展、
缺上下文及其他载体的原因消失。现按原生观察冻结 summary/error/expert，并继续计入
原有预算。两份原始大包全量回放 379,455 帧：SDK 的 39,283 / 245,615 条记录逐字节
一致；最终 CLI 分别只给 4,083 / 116,306 个 PDU 补充上述诊断，所有新增字符串均与
独立 SDK 导出一致。逐条检查其余物理行、PDU、字段、来源、流、事务和摘要完全相同。
旧计数保持不变，原有 CRYPTO 冲突、未选和 fuzz 差异仍保留。

最终完整原生 422.067s、CLI 1.990s 通过；帧角色/版本/密钥隔离 fuzz 106,248 次，
连接归属 fuzz 111,395 次通过。19 类原生保护控制覆盖 v1/v2 和三种 AEAD，格式拒绝
各重复 140 次后仍可恢复；独立诊断/保护审计另拒绝 45 项故意损坏的证据。
[帧约束聚合证据](shark-public-corpus-20261025/quic-frame-conformance-summary.json)
绑定源码、原始输入/导出、审计器、可执行文件和 Windows 缓冲区哈希。

TLS 会话恢复/0-RTT、票据、完整 TLS/transport parameter/endpoint 状态约束、一般不同内容
复用 PN、未选择扩展及此前协议范围仍未全部覆盖。早期数据方向/帧规则来自显式密钥
单元控制，不能冒充此次真实网络完成了 0-RTT 恢复。整体验收仍未完成，PR 保持草稿。

## 2026-10-27：SSH 角色证据、原始提议与协商值

`318920f77e` 修复 SSH 在缺失 TCP 发起方证据时仍按默认方向选择算法的问题。
[RFC4253 §7.1](https://www.rfc-editor.org/rfc/rfc4253.html#section-7.1)
要求按客户端偏好选择，先捕获的一端并不能证明是客户端。缺少角色时继续解析
可证明的标识、完整初始包和双方提议，明确显示上下文缺失，不制造 selected 字段、
特定交换布局或签名/身份验证结论。新增可查询的 `ssh.negotiation.context`。

KEXINIT 的六项方向性密码/MAC/压缩列表改为 `Offered ...`，原始值不再被随后推导的
协商值覆盖。已有选定算法字段在观察到角色和双方提议后才产生。重复初始 KEXINIT
在更新提议之前拒绝；140 次拒绝不覆盖原状态。原有协商正例改为显式提供其所假定的
客户端方向，另加未知角色、服务端先到、方向互换和分片控制，冻结输入与答案均未改。

新抓取的实际 Windows TCP 流量为 376 帧。Windows OpenSSH 与独立 Paramiko 完成
五组连接：Curve25519、P-256 ECDH、固定 DH、DH-GEX 和默认协商，各逐字节接收
128 KiB，并触发加密重协商。声明的 loopback 代理先转发真实服务端标识，再转发
客户端数据，不改写 SSH 字节。客户端偏好 AES-128/HMAC-SHA256，服务端列表相反；
两个实际端点均确认选用了 AES-128/HMAC-SHA256。私钥、moduli 和流量只在本机。

由原始抓包生成明确标记的去 SYN、捕获乱序、重复段、缺段和 snaplen 截断控制，
这些是回放控制，**不冒充实际网络发生了丢包**。去 SYN 后，旧版在五条连接的
27 个 PDU 中传播了错误协商结果；新版保留原始提议和角色限制，不猜选定算法。

| 输入 | 帧数 | 原生完整初始 PDU | 逐字节参考匹配 | 行为 |
| --- | ---: | ---: | ---: | --- |
| 实际原始捕获 | 376 | 42 | 42 | 162 项选定算法与实际端点一致 |
| 去 SYN | 366 | 42 | 42 | 协商上下文缺失，保留 SSH 和提议 |
| 捕获顺序交换 | 376 | 42 | 42 | 重组后解析，不猜后到内容 |
| 重复段 | 381 | 42 | 42 | 完整消息不重复交付 |
| 缺段 | 371 | 26 | 26 | 不跨缺口补造提议或协商 |
| snaplen 截断 | 376 | 26 | 26 | 不把残片当完整 KEXINIT |

独立审计从 PCAP/TCP 字节重组连续前缀，验证长度、padding、十个 name-list、NEWKEYS
及每段物理贡献范围，绑定参考原始 frame bytes，再比较完整参考 PDU/字段和端点日志。
五条连接保持独立；所有可比初始视图一致。加密后的重协商、登录与应用内容没有因此
被解密或认证。24 项字节、字段、角色、引用、流和导出损坏控制全部拒绝。

两份原始大包再次全量回放 379,455 帧。SDK 的 39,283 / 245,615 条记录仅有
6 / 4 条 KEXINIT 调整上述提议投影；所有原字节、字段树、PDU、流与事务保持不变。
Weber 的 CLI 仅 #49653 补充缺失角色说明和字段，Sharon CLI 逐字节一致。
与原文件参考另外核对 25 / 17 个原生初始 PDU、73 / 49 项字段，包含全部
6 / 4 个原生 KEXINIT；可比差异为 0。

仍有一个实际兼容性缺口：两个输入各有 9 个参考明文视图未进入原生成功树，均来自
参考 stream 12 的 `SSH-2.0-Cisco-1.25` 连接，原帧为 #5213/#5217/#5221/#5222/
#5224/#5227/#5247/#5249/#5250。软件名中的连字符违反
[RFC4253 §4.2](https://www.rfc-editor.org/rfc/rfc4253.html#section-4.2)，当前严格路径
在标识处停止。不能把这九条解释成全部已经支持；后续应补带明确符合性警告的兼容
观察，并保持完整性/连接证据约束，不能静默放宽格式后直接宣称标准符合性。

完整原生 401.998s、CLI 2.212s 通过，角色/分片 fuzz 30,679 次/40.431s 通过。
实际 Windows CP936 分别检查有角色时的 AES-128 选择和去 SYN 后的上下文限制，
协议、字段、字节源、会话、缩放、退出及码页恢复均通过。
同一最终 EXE 再完整打开 51,328 帧 Weber 文件，在 #49653 / PDU #38760 的实际
页面确认新角色限制说明；字段、字节源、会话、缩放及 CP936 恢复也通过。
[SSH 角色聚合证据](shark-public-corpus-20261025/ssh-role-evidence-summary.json)
绑定源码、原始/派生输入、独立审计、完整导出与未经改写的原生控制台缓冲区。
本轮不宣称完成所有 SSH 字段范围、方法数学、加密载荷、签名验证或协议整体完备性。

## 2026-10-27 SSH 兼容观察、字段一致性和 Windows 迟到视图

原生与 CLI 源码 `0edee7f77b`，独立审计源码 `1a129b5ebc`。前节两个输入各缺的
9 个明文视图已补齐，原始捕获、派生捕获、密钥、导出和控制台图像仍全部留在仓库外。

`SSH-2.0-Cisco-1.25` 的软件名违反
[RFC4253 §4.2](https://www.rfc-editor.org/rfc/rfc4253.html#section-4.2) 的字符约束。
严格校验函数继续拒绝它；原生兼容路径只有在已观察到合法对端标识，或同向有完整、
独立验证的 KEXINIT 后才接纳这种有界偏差。原始字节保持不变，字段和摘要持续显示
`nonconformant` 及具体原因。半个标识、半个 KEXINIT、控制字符和不支持的版本均不能
据此确认 SSH。警告也保留到 NEWKEYS 后和关闭会话后才发布的 TCP 缺口诊断。

更深的字段检查发现，旧原生 `GetFields` 仍可能把共享的消息号 30 当固定 DH，并把
Curve25519 二进制值转成有损字符串。现在原生入口先解析通用完整封包，再根据实际
协商方法选用已有的有界初始解析器；`GetFields`、`Decode` 和会话树使用同一份受保护
字段结果。角色/双边提议不足时返回不透明的方法载荷，不套用 DH 或 ECDH。修改公开
字段缓存不能改写后续查询的原始字段或已观察协商上下文。

独立 Python 游标按 [RFC4253](https://www.rfc-editor.org/rfc/rfc4253.html)、
[RFC4419](https://www.rfc-editor.org/rfc/rfc4419.html)、
[RFC5656](https://www.rfc-editor.org/rfc/rfc5656.html) 和
[RFC8709](https://www.rfc-editor.org/rfc/rfc8709.html) 的字节结构核对完整封包、长度、
padding、十个提议列表、DH/GEX/ECDH、公钥和签名容器，再与精确消息字节和连接方向
相同的 Wireshark 视图对照；没有把签名、公钥字节解析称为签名或身份验证。

新抓取使用真实 Windows OpenSSH 和独立 Paramiko，明确让测试服务端发送上述标识，
不声称它是真实 Cisco 设备。五组连接分别完成 128 KiB 的逐字节收发，并包含加密重协商。
后者仅证明端点成功通信，本轮原生字段验证仍止于各方向初次 NEWKEYS。

| 输入/控制 | 物理帧 | 完整初始消息 | 与参考字节/主字段相符 |
|---|---:|---:|---:|
| 新真实捕获 | 375 | 42 | 42 |
| 去掉 SYN | 365 | 42 | 42 |
| 捕获顺序交换 | 375 | 42 | 42 |
| 重复段 | 380 | 42 | 42 |
| 缺段 | 370 | 26 | 26 |
| snaplen 截断 | 375 | 26 | 26 |

控制只改变明确声明的回放记录，不宣称在真实网络注入了丢包。完整原始流量另核对
32 棵初始封包字段树和 168 项参考字段。乱序中有 5 个视图的 Wireshark 展示帧是后到的
重组触发帧，原生记录则指向真正携带字节的帧；独立 PCAP/TCP 审计证明字节、连接、
方向和每段贡献区间，证书分别保留这两种帧归属。未具备充分方法上下文的发布仍保留
通用载荷，不能将后见信息伪装成当时已经观察到的协商结果。

| 原始大捕获 | 物理帧 | 原生消息 | 明文初始视图 | SSHPacket 字段树 | 修正的原有字段树 |
|---|---:|---:|---:|---:|---:|
| Weber | 51,328 | 39,293 | 34/34 | 26 | 18 |
| Sharon | 328,127 | 245,625 | 26/26 | 20 | 12 |

每份新增 8 个完整封包和 2 个不透明加密诊断，原标识拒绝变为带符合性警告的标识视图。
全部 8 / 6 个 KEXINIT 及已发布字段树均与原字节/可用参考相符，两个输入原先缺的
九个视图均无剩余。原生生成的 PDU 编号通过精确字节、端点、域及贡献帧建立一一映射；
只重映射明确的 PDU 引用，包号、流号、线上的事务号、序号和长度保持字面值。
其他协议的字段与流/会话关联保持一致。CLI 每份 226 个变化行都限于原 SSH 连接，
其余连接一致；原始显示标签计数不变，原生 SSH 消息计数各增加 10。

Windows 原生 CP936 实测三组：真实标识、完整 Weber 文件早期 GEX 重组和加密阶段。
发现并修复列表摘要不随迟到识别刷新的问题；列表、字段和过滤结果现在共同跟随历史
修订，贡献包注明消息完成帧。还确认包列表容量与协议详情历史是不同的预算；新增
`--protocol-history-mib 1024` 可为长回放显式扩大详情缓存，默认预算保持有界。完整
51,328 帧回放后早期记录仍可查看。字段、字节源、会话、证据、80×30/180×48 缩放、
退出与 CP936 恢复均通过。未覆盖字段的过滤提示仍明确保留。

最终完整原生 448.136s、CLI 2.068s 通过，识别 fuzz 101,728 次/40.402s 通过。
53 项字节/字段、角色/警告、编号/来源/关联损坏控制全部拒绝。
[本轮聚合证据](shark-public-corpus-20261025/ssh-compatibility-summary.json) 与
[当前原始标签计数](shark-public-corpus-20261025/current-counts.md) 绑定源码和完整导出。

仍需继续：乱序控制中另有 5 个方法消息在发布时缺少充分上下文，保持通用载荷，尚未
产生基于后续证据的派生细化视图；Ed25519 主机公钥/签名的原生细节展开；旧的直接无状态
`application-layer.ssh` 规则入口单独复核；加密后内容、方法数学、签名/身份验证和
更多字段范围。Sharon 中 24 个参考行使用本轮未选择的非 IP/TCP 承载，单独列出，
不据此宣称原生 SCTP-SSH 已支持。协议整体及其余公共 fuzz 边界仍未完成。
