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
9. **能力清单虚报。** AJP/BGP/RTMP 目前只有 legacy YAML，没有原生会话入口，已从
   `NativeProtocolProfiles` 的已实现列表移除。规则目录存在不再被拿来证明抓包入口已支持。

## TLS 与 QUIC 的核验边界

TLS 记录结果见 [独立记录汇总](shark-public-corpus-20261025/weber-tls-records.json)。
审计曾使用 TShark `-j "frame tls"` 精简树，发现它会漏掉嵌套在 RDPUDP/EAP 内的 TLS。
最终改用完整 JSON 树；**早期 2,639 条完全一致不能代表整个文件全部 TLS 已覆盖。**
按 TCP、RDPUDP、EAP/RADIUS 分开报告记录数；TCP 的记录字节/出现次数、导出头字段
另行核对，流分区和解密结果仍是不同验收项。

最终主文件得到 2,735 条完整 TLS 记录：TCP 2,639、RDPUDP 49、EAP/RADIUS 47。
TCP 的 2,639 条原始字节和重复出现次数完全一致，记录 version/length/wire_type 共
7,917 项导出检查无差异；**另两种载体缺失的 96 条仍未解决**，没有混进成功比例。
同一捕获域内的 824 组 TCP stream、3,367 组 UDP 端点分区，在双向分区检查中没有
缺关联、额外拆分或合并。它不证明所有应用会话/事务已经等价，也不要求跨域合并。

IKE 的五项已登记头字段各检查 719 帧，无值差异。通用逐帧字段检查仍保留
89 项差异：83 项 TLS record length 的贡献包/完成 PDU 集合归属，以及 6 项 DNS
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

1. **RDPUDP/2、EAP/RADIUS 中的 TLS。** 必须先实现载体可靠重组、分片/重传及代际，
   再接 TLS；不能搜索载荷里的五字节 TLS 头冒充完成。本次完整树新增的缺失记录直接
   保留在汇总里。
2. **真实未接入的协议。** BGP/AJP/RTMP、HSRP/GLBP、BFD、L2TPv3、RDPUDP、IS-IS、
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
