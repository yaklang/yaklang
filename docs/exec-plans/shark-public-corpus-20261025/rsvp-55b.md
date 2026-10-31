# RSVP 基础对象、父层约束与 Windows 展示

生产代码为 `55b29ade06b2201e8be8e40b25920226038df2e1`，前一阶段为 `4dceaf339e49dd235918664ec38c827a0f20cf30`。本阶段接入协议目录、canonical bin-parser、原生 IP46 分发、34 个私有字段别名及递进详情。原始抓包、密钥、完整逐例事实/期望/导出、PDML、日志、console cells/PNG 与二进制均在仓库外；仓库仅保留执行源码、依赖哈希和聚合结果。

本报告没有证明 TOP30 或核心协议全部完备。RSVP 当前是基础 wire observation profile：消息 1–7 的公共头、对象封装，以及部分基础对象字段。SESSION/SENDER 中的地址和端口是通告选择器；RSVP_HOP 的 handle 是通告内容。它们不创建实际 UDP/TCP 流、可信邻接、已安装预约或经过认证的会话。

## 独立来源与参考范围

官方 [Wireshark RSVP PATH/RESV 样本](https://wiki.wireshark.org/uploads/__moin_import__/attachments/SampleCaptures/rsvp-PATH-RESV.pcap) 为 9 个物理帧、3 种不同 PDU 字节串、55 个对象。7 个 Path 帧重复同一 PDU，不能当成 7 种独立过程。39 个基础对象有选中的字段布局；16 个 FLOWSPEC/SENDER_TSPEC/ADSPEC body 保留为 owned opaque bytes。

基础来源模型含 332 个原始字段与 18 个派生位值。与固定 TShark 4.6.9 的参考相比，289 个原始字段和 18 个派生值的数值与范围一致；43 个未渲染项不计作参考通过。对象边界、完整公共头及非零校验和另作来源证明，不把校验和正确当成认证或预约生效。

规范边界来自 [RFC2205](https://www.rfc-editor.org/rfc/rfc2205.html)、[RFC2961](https://www.rfc-editor.org/rfc/rfc2961.html) 和 [RFC2747](https://www.rfc-editor.org/rfc/rfc2747.html)。未知 class/C-Type 的 body 保留为 opaque，后续独立 sibling 可以继续；Bundle 等未选消息只保留公共头与 owned body，不能把它的 body 当作普通对象链。TIME 的单位明确为毫秒；C-Type 的意义按对象 class 区分，没有把所有数值 2 都显示成 IPv6。

## 残包、预算与原生入口

20 个 authored 场景包含 24 个物理帧，另有一次同字节零预算评估。与官方 9 帧合并，direct API 共 34 次评估、33 个唯一物理输入，核对 685 个原始字段、68 个派生值和 34 组错误身份。原生默认预算为 4096，故另建来源期望，33 帧的 Session 为 688 个原始字段、66 个派生值；这些数量不与 direct 或 GetFields 相加冒称额外的唯一 wire 覆盖。

原生和 CLI 对同 33 帧分别实际执行，34 个 descriptor、795 次 private alias occurrence，以及 33 份 SDK/CLI snapshot 与物理来源/层级桥核验通过。官方 9 帧 wire 解码成功，但 Intserv 未选布局使 typed snapshot 保持 limited。authored 24 帧中 12 个 wire 解码、12 个诊断，诊断保留完整捕获的字段前缀，不补造缺失 primitive，也不伪造 checksum 状态。

完整 IP 父已经结束却容不下已选 v1 公共头或声明消息长时，原生入口具有额外的 wrong-length 证据；真实 caplen 小于 original length 时，才使用捕获不完整的判断。裸 wire API 没有这种父层事实，保留自己的 NeedMore/Context 边界。坏校验和、格式与预算证据分别保留，不能互相掩盖。

IPv4/IPv6 乱序分片及重复分片检查要求完整重组后才发布 RSVP 字段；缺首或缺尾不能发布完整 RSVP 消息。通告选择器复用、记录重排/重复的 authored 控制保持零 flow、TCP flow、transaction 和 response-to 归属。这些是离线控制，不冒称新的真实网络丢包现场。

## 大包发现的真实父层误分发

首次 `4dce` 回放出现 937 条父包诊断。逐原始 PCAPNG 复核发现 frame218724 是 IPv4 Protocol43；旧分发把其载荷解成 IPv6 Routing，再借其 Next Header46 进入 RSVP 入口。这个错误 family 归属必须修复，不能纳入新的正确期望。

`55b29ad` 使 Hop-by-Hop、Destination、Routing 的 RSVP 子分发只在实际 IPv6 父中成立，原生、捕获与详情入口一致；AH 的已有 IPv4/IPv6 支持保持原作用域。回归同时覆盖 IPv4 Protocol0/43/60 的完整及无效父、真实 IPv6 正例和原有捕获错误行为。

修后全原包来源模型得到 936 条应保留的 IP 父拒绝：671 条非零 IP 长度超过记录边界，265 条零长度。671 条非空端点逐字节来自原 header；265 条仍未知的端点保持空，不借原始字节填入未经原生入口确认的字段。原 raw-IP46 清单遗漏或独有的集合也单独解释：233 个旧清单独有帧均有坏 options，须更早拒绝。frame279605 是真实 literal46 的空 payload 记录，新增父包诊断有来源依据，不与错误 family 一起删除。

37 输入的原生完整对象核验通过：935 个旧 IP 诊断被完整来源 DTO 替换、279605 新增 1 个父包诊断；其余 306883 个完整原生对象仅在明确的生成编号引用路径作注入式映射后保持。36 个完整 event 导出文件逐字节相同。不得把 wire 数字、帧号、flow ID 或任意错误字段作数值归一化。

两份完整 flow/session 对象及其他统计保持一致。Sharon 的 `stats.DecodeErrors` 从 81595 变为 80660：935 个非空 direct46 坏父不再调用旧的 unsupported child decoder，原生仍明确发布父包 malformed/child-unconfirmed 诊断。这个唯一 counter 差值单列，不能说成修好了 935 个无效 IP，也不能声称整个 flow 文件逐字节不变。

CLI 的完整 37 输入逐行验证也已通过：共 384518 条记录，其中 383936 条物理记录、582 条非物理发布。936 个目标整行按原始物理来源与固定展示合同构造，另 383582 个完整行仅在已审生成编号路径映射外保持；36 份完整 CLI export 逐字节相同。原生与 CLI 各自验证，后者独立重建并绑定已闭合 native 编号账本，字段值、端点、时间、来源与递进层级均未被任意归一化。

## Windows 实际页面与测试

Windows 默认 20000 条 packet cache、64 MiB history，两张页面分别检查官方 frame1 的 TIME_VALUES=30000 和 frame7 的 STYLE=10（Fixed Filter）。它们是精确抽出的单包检查文件，仍保留原帧字节；不是新抓的现场流量，也不是全大包缓存能力证明。

Source0 完整物理字节与 Source1 完整 RSVP PDU 核对；选中 TIME 的 PDU 范围为 [36,40)，实际帧为 [74,78)；STYLE 为 [53,56)，实际帧为 [87,90)。两个帧的 IHL 不同，不能共用起始偏移。鼠标、tabs、缩放及 CP936 恢复通过，root 看过实际选中图。PNG 是实际 console 字符与颜色还原，不是 Windows Terminal 像素截图。两次旧导航失败保留，仅调整滚动位置，原字段名、值及范围期望未改。

初始五包全量运行 285647 项通过、24 项跳过、0 失败；后续长度/诊断页修复 58 项通过，family 修复相关 88 项通过。这些执行有各自冻结源快照，不冒称是最新源码又跑了一遍完整五包。最新 `55b29ad` 的 60 秒双 worker wire-range fuzz 为 168940 次执行、0 失败；旧 `4dce` 的 161692 次另列，不相加作一个新运行。

## 尚未覆盖的语义

每种消息的完整 mandatory-object/procedure grammar、预约安装状态、INTEGRITY/MAC/replay、Bundle/reduction 与完整 Intserv 都未证明。Intserv 已单独取得 16 个对象、3 种 body 的来源审计，字段布局与参考宽范围差别保留；其中 9 个 m=0、2 个 M=0 违反明确参数域约束，不能因 Wireshark 渲染正常就声称广告参数有效。这些新来源目前尚未接入产品解析。

后续继续补选中的 Intserv 布局、保留 IEEE32 原位模式与非有限值分类、加入长度/未知服务/残包控制，再逐字段与参考及 Windows 页面核验。覆盖声明随实际证据扩展。


## CI 与可复核源码

生产源码55b的四项CI全绿：流量测试40927625316、Essential40927636718、Diff40927636795、路径碰撞40927636648。草稿自动跳过的traffic不计通过。前一4d提交曾有SSA引用重复及AI工具WaitGroup计数失败，原日志保留，未改这些源码；失败项重跑后通过。后续归档提交的检查另行跟踪，不借代码提交的结果冒称新HEAD已通过。

[聚合证据](rsvp-55b-summary.json)记录来源、范围、自测、执行与CI的外部proof哈希。
[冻结归档](../../../common/yak/cmd/yakcmds/shark-cli/testdata/live-audit.zip)
内的 `live-audit/rsvp-control47/` 保留47个纯源文件及其逐文件哈希，含旧检查器与来源政策勘误；不含完整逐包事实、期望、编号账本或运行输出。
初版native整对象检查器错误地将已字面相同的fresh-QUIC文件送入仅适用两公共包的ID语义mapper，失败原件保留；02仅在完整export SHA与行数均相同且producer绑定完成时跳过映射，未知transaction的任意变化仍拒绝，未扩大其含义猜测。
