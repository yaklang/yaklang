# UDP参考层级与原始位置关联：2026-11-02

M2-66关闭N11原报告24组UDP tuple分区差异的来源归因，修正差分基础设施。
运行工具及干净构建为`976e64817308633a8577e7468ec6c18f5b5a6d02`；仅新增
`internal/trafficoracle`及CI中的工具反例测试，未修改原生解析器、TUI或fixture ZIP。
SMB仍搁置，仍只提交M2 PR #5371。本项不是全部UDP应用会话或校验和验收。

## 原报告为何有24组

原clock报告将同一物理包中`udp.stream`的全部出现值，合并到native外层tuple。
内层解码产生的stream因而被误算成外层会话复用。1003–1008独立定位全部9066原帧：
每组只有一个外层IPv4/UDP tuple，且与旧参考第一个外层stream一一对应；452帧有多值UDP stream。
不能由此一般化成“取第一个字段”：其他承载路径可能选择内层，必须逐项关联位置和实际字节。

原24组及原始报告字节保持，另给新来源账本和范围结论，不改写旧计数掩盖差异。
这些9066包的当前SDK均无应用FlowID/TCPFlowID；CLI也无应用conversation/connection identity。
9018包有字面UDP长度矛盾，native明确MalformedMessage；48包只有长度范围一致，仍ProtocolNotRecognized。
因此原24组不证明产品合并了UDP应用会话，48包也不能称“有效协议已识别”。

## 可复用实现及真实参考

[工具和输入约定](../../internal/trafficoracle/README.md)从流式PDML读取每个协议出现位置，
按同一物理frame/interface/captured length、IPv4地址位置及完整UDP四字段的pos/size/value选唯一层。
相同端口、出现顺序或某个stream编号均不足以加入。内层参考层保留，不能混入父层stream集合。
当前只支持独立来源已选定的未分片IPv4→UDP；不支持路径明确报错，不暗中跳过。

命令同时保留来源section/encapsulation、完整捕获帧SHA和三份输入指纹；source domain由独立捕获账本提供，
PDML interface直接核对。缺帧、重复、歧义、错范围/字节、接口错误均失败；整范围通过后才创建新输出，
已有文件不覆盖。限制64MiB captured evidence、100000选定帧、1GiB流式PDML；不能靠超限静默少测。

1010用TShark4.6.9对完整原Sharon捕获重新双遍解码，空配置、IP/TCP乱序重组、明确TSO开启，
用事前冻结的24个外层stream筛选输出；所有9066原帧在输出中完整出现，零额外帧。
capture SHA256仍`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
122942607B PDML的SHA256为`928462909eda4f33dd757f04db8093a51d0596e733274497d4efb0379520ad75`。
整个捕获中的TCP/ITS等参考dissector警告、原stderr和默认TSO政策均保留，exit0不豁免警告。

1025实际运行提交后的Windows命令，逐包匹配原始地址、端口、UDP长度、所有位置及原帧哈希，
24外层tuple各只对应一个新参考stream；9018个`_ws.malformed`头部诊断完整保留。
参考会把这些诊断proto嵌在UDP长度field内，新读取器保留诊断存在，不把其子字段混作UDP头。
1032再以真实1702B DNS应用字节构造直接、VLAN100同接口、VLAN100另一接口三包正控，
五次实际reference/工具运行核对来源，错VLAN偏移、错接口、缺帧三个负控明确失败且无部分成功文件。
这些是声明外层变换的来源控制，不是新peer抓包，也不代表DNS正文全语义。

## 测试、Windows与范围

新增42原始测试ID全部pass，其中四个fuzz种子；完整受影响`./scripts/ci ./internal/trafficoracle/...`
为43pass/0skip。包含同端口内层先出现、字节/范围/域错误、缺checksum/stream、重复父头/帧、
损坏XML、诊断嵌套、真实命令缺帧/已有输出保护。既有断言未删改，现有traffic CI直接执行这些检查。
1030最终覆盖插桩fuzz完成四种子和382630次执行，硬1CPU/768MiB job/最多2同时进程，所有后代关闭。

保留失败：1004将未加引号的TSV当CSV、1005猜字段数，1006改为核对原manifest及每行列数；
1009显示过滤集合语法不符安装版，1010修正并冻结命令。1020真实PDML暴露嵌套诊断兼容问题，
修正后补两反例并重跑全部9066包；1028未插桩的fuzz不计验收，1029重建带覆盖指引并由1030实际重跑。

生产解析器和Windows TUI源码与3d2完全相同，先前完整两捕获六导出、三个原包六实际页面的
网络运行代码仍为a9031；本批没有宣称重新运行那些页面或改变了协议识别。
新增Windows验证是提交后的差分命令、9066包及五次来源控制运行。
完整捕获、PDML、原始frame evidence、密钥、截图和fuzz corpus均在仓库外；两个fixture ZIP逐字节不变。

## 闭合证据和剩余工作

外部证据根`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`：

| 闭合 | SHA256 |
|---|---|
| UDP-oracle-runtime1025-01/summary1025.json | `bcbd05b2a819af02510937fa7c2b5f58993e186055206aeca37a7f274f6b17c8` |
| UDP-source-join1006-01/outer-header-ledger1006.jsonl | `48e7448ef9448204914d4fcd57befb75ac5fa2b85bc6f36a6e483be4719582e6` |
| UDP-oracle-qualification1033-01/summary1033.json | `6db3228cafa376de8e80c9afd4101efb7a1453fa4aebb24f72dabd279712bf4a` |

已发布父`3d2ac689d21b6831f34ff46c24e82557584ad6c4`已独立47约定检查、91实际执行、2statuses全绿；
[traffic](https://github.com/yaklang/yaklang/actions/runs/41389720953)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41389724818)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41389728070)。314308 raw ID与事前清单一致：
314283pass/25历史optional skip，42新增及131084 required均pass/0skip；产物SHA256
`e84373d8a01eb332d9fc2c7df40c90869312905155039ceb2876501c1159469a`，综合闭合1015为
`27eaedbf8f564691cadb99eb6c1376f19f8caeafc5f53b98b1fe1621104bd902`。
这不认证976e工具及随后说明。新最终head须事前冻结314350 raw ID及终态，保留全部父动作，
42新增全部pass；预期314325pass/25历史optional skip及131084 required pass/0skip，再独立闭合全部CI。

N11仅关闭原24组混层报告差异，不承诺所有UDP协议会话正确。非SMB17包现13有限完成/4开放：
N09 QUIC缺握手、N10六TCP政策、N13完整TLS阶段/协商、N14 IP来源/字段/transport恢复。
当前66父项中56有限完成、7进行中、3 SMB搁置，仍10未关闭；四包粗估16–32 AI实施小时，
构建/CI/回放等待和新问题另计。不是全部核心协议或全部Wireshark正确性一致。
