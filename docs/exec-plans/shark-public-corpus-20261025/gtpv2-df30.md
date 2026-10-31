# GTPv2-C：来源、解析、原生字段与页面

本阶段补齐一个明确标为 partial 的 EPC GTPv2-C observation profile。它提供有界的条件头、最多两个指示消息、TLIV/group93 和选定 Recovery、EBI、Cause、F-TEID 字段；并不证明 mandatory IE、完整 procedure、NAS、订户、承载/TEID 所有权或对端认证。不能据此宣布全部核心协议或 TOP30 完备。

原有 37 输入中的 GTP 端口普查有 5538 条提示，其中 5535 条的 UDP 声明范围超出完整捕获的 IPv4 父包；剩余 3 条也不是合法 GTP。该组不能作为应增加几千条 GTP 标签的正样本。六个原始 Sharon 代表帧及其 section/interface/DLT 另有来源账本，未修写长度或捕获字节。

另从 Wireshark 官方 issue 15724 获得六帧完整 GTPv2-C：32×1、95×1、133×2、128×1、129×1，共 56 个顶层及 8 个 group93 子 IE。独立头/TLIV 逐值与范围检查 292 次，参考差异为零。三个参考 expert warning 保留；没有把未知 IE 的 opaque 内容计为已理解的语义。

[ETSI TS 129 274 V18.6.0](https://www.etsi.org/deliver/etsi_ts/129200_129299/129274/18.06.00_60/ts_129274v180600p.pdf) 是 EPC 头和 IE 边界依据。S101/S121、Sv 另有各自规范；共享 v2 header 不能证实 EPC IE 命名空间。未选接口保留共同头和 owned opaque body，IEs absent，返回 context；不可用未解释字段的否定筛选证明不存在。官方有效捕获来自 [Wireshark issue 15724](https://gitlab.com/wireshark/wireshark/-/issues/15724)。

30 个冻结 direct 用例来自上述六帧及 20 个场景的 24 个 authored 物理帧。原始 source facts/oracle 均不随候选输出修改。当前 direct30 与之前 direct30 全部输出逐字节相同，SHA256 为 `22cd7fabb3864cf33b17e21232913973dd034e96610c8a5dae85e4402cdd88b2`，独立逐叶检查零差异。这是 direct codec 验证，不能替代 native、CLI 或 Windows ingress 的验证。

实现保留 TEID=0；MP 未激活或 Echo 的 spare octet 不制造优先级；第二消息使用自己的长度和 sequence，第三消息及未指示尾部不递归接纳；组 IE 保持父界限和全局预算。UDP 包独立观察，重复、乱序及跨对端同 TEID 不继承字段或制造 flow/session。IP 分片必须完成 UDP 重组，缺失尾片不能识别完整 GTP。

实际发现并修复了 GTP 外形抢占已有完整 IKE envelope、reserved178 当作已选类型、Echo 错误激活 MP 优先级、零 IE 预算丢头，以及低预算字段树在页面被 4096 默认预算重解读而失去 physical prefix 的问题。前两次 red 执行证明了 IKE 抢占和零预算丢头；页面预算另有实际 red/green。其他修复有对应检查，不冒称每条都先运行了失败版本。

Native 私有字段保留本消息的值、次数和 UDP-PDU 范围；对公开 Raw/Session/Protocol/Status/Source 的修改不改变既有字段证据。masked bits 是 derived，没有伪造整数字节跨度。物理 IP/UDP source 与逻辑协议 source 分开，worker/quick/detail 共用同一 canonical tree 与来源复现检查。

PCAP、keylog、完整 per-case facts/plan/expected、native/CLI 导出、PDML、二进制、日志、控制台 cells 和图片均留在仓库外。入仓仅解析/测试源码、纯控制器及聚合计数和 hash。


## 原生、CLI 与实际前后差别

旧版 `cd7af6` 与新版 `df30fc4` 均实际运行同一份官方六帧、相同关闭 embedded secret 的参数。旧 SDK 六帧都是 TURN ChannelData / context-required（没有 ChannelBind），旧 CLI 六行均为 UDP；新版六帧均为 decoded GTPv2-C，逐字段/范围证据和具体消息名已核对。不能将旧状态说成六个成功解码的 TURN 会话。

| 模式 | 物理行数 | 完整 GTPv2 | 显式诊断（仍显示 UDP） | 拒绝 GTP 身份 | private typed 值 |
|---|---:|---:|---:|---:|---:|
| 官方自动 | 6 | 6 | 0 | 0 | 422 |
| authored 自动 | 24 | 13 | 0 | 11（含1 carrier拒绝） | 1051 |
| authored 显式 | 24 | 13 | 10 | 1（carrier拒绝） | 1209 |

模式复用同30帧，不是54个独立样本。54条 SDK/CLI snapshot 桥及30个 descriptor(name/type/bits/origin)全过。官方 Session 与 GetFields 各307 literal/178 derived/70 names，是两份 API 检查，不能相加作614个独立 wire 叶。显式诊断保留 Session 部分字段；没有将 GetFields 未确认内容、opaque bytes 或未知 metadata 冒称完整语义。

## 接收长度与证据边界

ETSI §7.7.7、Table8.1、§8.4/8.5/8.8/8.22的复核修正：EBI 和 F-TEID 保留可证前缀及 owned 忽略扩展；F-TEID V4/V6至少一位，最小value为9/21/25；短字段、无地址类型和Cause非法n（仅允许2或6）有明确错误。Recovery表项称Variable Length而正文规定1 octet；n>1保留counter和owned未选尾并返回Context，未伪称规范必然要求Format。IPv6与双地址扩展有精确半开范围和独立所有权检查。

旧长短IE处理有实际失败执行（11个子例失败、1个子例通过，父测试也失败）；修后相关59项及额外IPv6/双地址1项全过。中间一次native测试误用了未发布的底层decoder，故私有alias尚未封存；随后改用真实auto/explicit捕获发布入口，没有降低预期。完整五包285612通过/24跳过属于f1068基线；不冒称最新df30又跑了一遍完整五包。

## 完整回放、Windows 与 fuzz

df30用新构建二进制实际跑37输入，307818 native完整对象、384518 CLI完整行及两份whole flow均与f1068逐字节相同；没有字段/ID规范化。它证明这组导出保留，不能证明所有未变解释都正确。

Windows默认20k/64MiB，两张实际页面来自官方frame2与authored双消息frame5，均为精确抽出的单包检查文件，不是新现场抓包或全大包缓存能力声明。Source0完整物理字节、Source1完整PDU、第二消息自身MessageType范围、鼠标/tabs/resize/CP936恢复与实际cells/attrs核验通过；root看过两张选中图。图片是实际console字符/颜色还原，不是Windows Terminal像素截图。

首次旧helper把WriteFile尚未写完的request读成空名而失败，原件保留；现helper忽略空request并在完整JSON关闭后发布完成ack。只改helper binding，原byte/name/range及导航均未改。闭合checker随后修正waitShot别名对ack的机械对应：必须匹配已完成candidate的精确JSON hash，不改内容预期或重新制造图片。

Fuzz实际运行和精确log/receipt/hash见聚合JSON。PCAP、完整facts/expected/exports、PDML、logs、cells/PNG与exe均外置。仓库仅有纯执行源码、来源依赖清单和聚合报告。

## 尚未完成

本阶段未证明全部IE、GTP-U、NAS/订户/承载状态或TOP30完整性。RSVP下一阶段已取得9帧/3种PDU、55对象的独立来源，39基础对象可核对，16 Intserv对象body仍opaque；这不是RSVP产品实现成果。全套hash、范围与逐例事实仍在外部保留。
