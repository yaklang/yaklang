# OSPFv3 边界与 d677 实测

本页只记录 `d6772965c669e288705f7e0d0e25c62972226832` 已闭合的执行。
未知完整 LSA 现在保留 opaque body 并继续读取后续已知 LSA；独立包校验和错误不会
再被 unsupported/context 或本地集合预算盖住。短 LSA 只保留已完整捕获的字段，
不创建空子项，不越过错误长度寻找下一条。AT、LLS、地址族和缺失 IPv6 最终目的
地址的校验范围分别标明；不会从校验通过推断发送方认证、邻接或 LSDB。

20 个明确标记为人工构造的边界控制中，17 个实际走 native/CLI，逐项核验283个
wire leaves、259个私有字段 occurrence；另外3个直接调用共享 parser，核验60个
leaves、准确范围和 context/budget/integrity 的 `errors.Is` 身份。两者不混作20次
native 执行。完整未知 LSA、后续已知 LSA、损坏 Fletcher、独立包校验和、预算、
AT/LLS、Options/下一条 LSA 的短头均在控制范围内。

TShark4.6.9 的94个原字节/字段检查没有差异，但保留两项校验适用范围差异及10个
短头标量遗漏。已核对参考字段的真实祖先与字节范围，以及官方 v4.6.9 代码，
没有为了名称一致而改写源值。LLS 位于 OSPF 包外，不能混入基础包 checksum；
认证尾存在时不能把非零 checksum 当已适用的基础完整性结论。
依据：[RFC5613](https://www.rfc-editor.org/rfc/rfc5613.html)、
[RFC7166](https://www.rfc-editor.org/rfc/rfc7166.html)。

| 实际执行 | 结果 |
|---|---|
| 五包完整回归 | 285564通过、24跳过、0失败 |
| 新 OSPFv3 fuzz | 60秒、167349次、0失败 |
| 37输入原生回放 | 307262完整事件、2份完整flow，与636逐字面保持 |
| 37输入 CLI 回放 | 384518完整行，与685逐字面保持 |
| 6项既有 TLS/QUIC 恢复控制 | 8053完整事件保持；不重编号、不映射ID |

Windows 使用默认20000/64MiB预算实际运行。大包选中 frame48906/PDU40097，物理
334字节和独立OSPF240字节来源正确，选中前缀8字节准确高亮。人工短头页原始
122/捕获116字节，只显示第二条 LSA 的6个完整标量，Type 的物理范围100..102
正确；没有臆造 Length 或 Body。主 agent 已亲自查看两张实际控制台 cells/属性
渲染图；它们不是 Windows Terminal 像素截图。原始proof中的visual-review-pending
标志没有被重写；本页另行记录后续人工查看。全局部分历史提示保留，未抬高预算。

该提交的 [Essential](https://github.com/yaklang/yaklang/actions/runs/40854905634)、
[显式完整SHA流量](https://github.com/yaklang/yaklang/actions/runs/40854898830)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/40854905576) 和
[路径大小写](https://github.com/yaklang/yaklang/actions/runs/40854905558) 全部通过。
草稿自动流量任务40854905602是 skipped，不算通过。最初无效checksum fixture、
外部checker对field coverage的错误假设及较早回归均保留各自结果。

具体计数、源码和外部证据摘要见 [证据索引](ospfv3-boundaries-d677-summary.json)。
归档28个原始控制器源码；捕获、密钥、完整case/plan/期望/导出、日志、图片和程序
均在仓库外。OSPFv2、其他高级边界和 TOP30 的整体完备性仍未因此关闭。
