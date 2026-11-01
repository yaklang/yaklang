# SSH 字段范围与单棵递进树：92620d72b0

`b586dda3c8` 将现有解码器持有的位置传到字段与字节来源；`92620d72b0` 去掉同一 PDU 的重复展示副本，并补齐旧封存样本的范围预期。完整字段、捕获字节、协商上下文及外部导出没有因展示去重而改变。聚合证据见 [summary](ssh-ranges-926-summary.json)，SHA256 `170319b9da1bbc49e9aaac51771a839f5a4705e46258220ff33b4b15b6317711`。

范围是 `ProtocolEvent.Raw` 所属完整 PDU 的半开字节区间，不是捕获帧偏移。每个嵌套 owner 使用自己的范围；没有范围的子 map 清空继承作用域。相同内容的算法表、主机密钥和签名不通过搜索字节定位。当前 KEXINIT offers 与连接上选出的算法分开；携带的 identification、选择结果、指纹及验证结论不借用当前字段位置。零宽值保留 API 事实，不高亮邻居字节。

去重只发生在同一事件的 `Fields` 与 `Session.Initial Packet Fields` 完整带类型相等时。父进程先判断，再移除 outgoing display copy；worker 不在 JSON 折叠数值类型后重新判等。原对象、其他 PDU、不同范围/字段/字节尾均保留。源码测试覆盖两种 worker 入口、类型折叠、跨 PDU 来源和无位置字节的独立提取来源。

## 独立来源与恢复

- source49 的 148 物理帧与 source50 的 115 帧仍是作者控制，不冒称新抓的 SSH 流量。中立计划在候选输出前冻结 1,487 个 binary literal spans、42 个 identification 行和 178 个现有私有别名范围；旧字段值和旧事实不改。两组实际 SDK/CLI 全对象检查通过，GetFields owner 范围分别 890/681；其中含行字段，不能与 Session 副本、别名重复相加为 wire 覆盖。
- TCP 实际重组路径覆盖 eager/deferred、1/2 workers、乱序后缀先到、重复后缀、缺失中间段和同四元组新 SYN。缺口明确报错，不能由重传后缀补造字段；完整重组保留原 contributor refs，后续新连接不继承旧方法。错误 sender 的完整边界仍允许后续正确消息恢复；其 GetFields 保持拒绝，只保留当前 message byte 的事实。
- 三项 60 秒 SSH fuzz 全通过：FuzzSSHClientRoleNegotiationIsolation 48417, FuzzSSHNativeRefusalFieldIsolation 35828, FuzzSSHPlaintextEdwardsBoundaries 165709。构建时间单列，没有把总耗时当作 fuzz 时间。

## Wireshark 对照范围

重新执行隔离配置的 TShark 4.6.9，共 8 个自然/显式 SSH reference 进程。source49 每种模式有 762 个精确 primitive value/span、48 个零宽、24 个 identification 内容检查；source50 有 671 个 primitive、36 个零宽。aggregate owners、未呈现叶子与未映射字段单列，不冒充逐叶匹配。

参考仍会在部分未知/忽略的方法上显示标准布局，在 4 个已知错误 sender 包上未给角色拒绝，其中 2 个显示 host-key 字段。自然模式的方向标签还存在与 SYN 证据不同的情况。这里保留独立原字节与 TCP 角色约束，没有为追平标签制造方法或身份。

## 公共输入与 Windows

37 个公共输入重新执行：307,819 个完整 native 事件、384,518 条 CLI 行及 2 份完整 flow 文件。50→53 的冻结保存计划只允许 60 native 行中的 296 处位置元数据、41 CLI 物理行中的 74 个别名范围增加；所有其他字段、ID、refs、Raw 严格不变。53→54 再完整执行并核对全部输出，逐字节一致，没有 ID 重映射或 metadata stripping。这证明旧解释保留，不替代独立协议事实。

新版 Windows 三个实际控制台场景：主机格式 `[14,25)`、签名格式 `[105,116)` 的相同字符串各高亮 11 字节；选出的 cipher 不产生当前 offer 的高亮。完整 246/214-byte 捕获帧、192/160-byte PDU、来源切换、tabs/mouse/resize、CP936 和默认 20,000 packets/64 MiB 保留。图片由实际控制台 cells/attributes 还原，不冒称 Windows Terminal 像素截图。完整 worker 测试证明只画一棵报文树，视口中没有重复副本。

## 检查与历史

当前源码五包全量 **285,911 pass /24 skip /0 fail**，专项 **1,772 pass**。四项精确源码 CI 首轮全部成功：[Essential](https://github.com/yaklang/yaklang/actions/runs/40974723731)、[Diff](https://github.com/yaklang/yaklang/actions/runs/40974723739)、[case](https://github.com/yaklang/yaklang/actions/runs/40974723805)、[traffic](https://github.com/yaklang/yaklang/actions/runs/40974716776)。

最初 b586 全量有 4 个旧 SSH line fixture 加父项失败，流量 CI artifact 校验下载 digest/CRC 后确认相同失败。926 没改原压缩样本，而是核对 fixture 原 raw hex 的 identification/Text/Version 后明确补充范围，完整 Session/GetFields 仍 JSONEq。最初 Windows closure 的来源循环、滚动视口与 Unicode 选择符假设错误也逐版保留；所有 source 字节和实际截图未改。

18 份原始范围控制器源码共 121,962 bytes 归档于 `ssh-wire-ranges53`，只含源码和依赖哈希。抓包、keylog、原始字段事实/完整预期、SDK/CLI/PDML、日志、运行 receipt、二进制及图片均在仓库外。

新的 [TOP30 原始标签分布](label-cohorts-b586.md) 检查两个大包共 379,455 个原始帧；名称、版本、加密可见性及保守拒绝的差异仍需逐帧解释。尚未证明所有核心协议完备；已发现的 SMB1 多协议协商导致后续 SMB2 请求丢失，以及其他未覆盖协议和字段，继续单独处理。
