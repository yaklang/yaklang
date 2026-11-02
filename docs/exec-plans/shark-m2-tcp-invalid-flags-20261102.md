# TCP异常开连接标志的物理诊断与Windows页面：2026-11-02

M2-67关闭一个限定展示缺口：完整TCP头中的SYN+FIN/RST已经被重组核心拒绝，页面却没有
说明未接纳原因。生产代码及干净SDK/flow/Windows CLI构建为
`469ac09110215d84b67e762a3706b9c643434bb1`。仍只提交M2 PR #5371，SMB继续搁置。
本项不改原生重组政策，不关闭N10的完整连接代次/应用关联或全部TCP正确性。

## 原因和修复

依赖解码器接受这些语法完整的头，不创建ErrorLayer；原物理TCP诊断入口只在ErrorLayer存在时执行。
例如原Sharon145238：TCP标志包含SYN和RST，核心在触碰tuple缓存前拒绝；旧页面仍只是普通TCP及端口提示。
新入口对已解码的异常标志做低成本候选检查，再独立核对当前物理字节、所选IPv4/IPv6父头、范围及TCP选项。
只有字面标志证明SYN与FIN/RST同时存在，才显示具体拒绝；元数据不能代替原始标志。

列表、详情、quick和packet worker统一显示`TCP SYN combined with FIN or RST`、已观察头边界及
未进入重组的captured payload。保留Source0头字段/范围及未验证checksum，清除不适用的旧TLS/gRPC
连接展示；不生成新PDU、连接或Source1。正常SYN、SYN-ACK/ECN、FIN-ACK等不走该路径。
其他头错误保留既有优先级；未选择的隧道/扩展路径仍由其owner处理，不借内层字段作为父层证据。

这是解释产品既有拒绝政策，而不是声称实现了端点完整TCP状态机。接收端的序号、SYN及状态处理依据见
[RFC9293 §3.10.7](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.10.7)；被动捕获缺失的信息
不能仅由stream编号补出。

## 完整捕获和逐包参考

1053使用最终干净源码的三种入口重新完整回放Weber/Sharon，共六个导出、379455物理包。
两份SDK和两份TCP flow均与a9031逐字节一致。Weber51328行及29独立发布全部保持；
Sharon327812行及442独立发布完整字段保持，只有315个字面异常标志包改变展示。
1056逐对象检查，无PDU编号或任意数值映射；仅`info/protocol_stack/protocol_status/protocol_scope/
protocol_evidence/visibility`六种展示键改变，地址、端口、来源、PDU/字段和其他元数据保持。

1058再对完整原Sharon捕获做TShark4.6.9双遍解码，空配置、IP/TCP乱序重组、显式默认TSO，
输出全部315变化包。1059完成4725个字段检查：原frame/interface/caplen、IP地址/原始长度、
TCP端口/seq/ack/header length/flags/window/checksum/urgent pointer的位置及原值逐项一致。
TCP flags的逻辑掩码与unmasked word分别核对，不把bitfield显示值当完整两字节。
其中167包IP Total Length字面为零，参考默认TSO推断与原字面值分列；没有修补原包或证明真实TSO。
所有参考警告和原stderr保留；不声称checksum正确或所有Wireshark状态解释一致。

## 实际Windows和测试

1047只做独立worker预检；1048的旧/新版控制台随后分别从同一原包PCAP运行。
原145238完整241B未变、派生frame1保留interface226。新页明确错误和184B未进重组，
Source0[46,48)精确高亮`50 fe`，逻辑flags显示`0x0fe`并注明reserved bits排除。
两个实际页面共40状态：完整原帧读回、字段/字节选择、mouse/Tab、80×30/180×48恢复和CP936恢复通过。
1054主线程看旧/新实际PNG；这是原生控制台单元检查，不是全部Terminal字体/DPI/IME验收。

25个新增精确ID全部pass，覆盖三种非法组合×IPv4/零长度观察/VLAN/IPv6，合法标志、物理包不符、
UDP和父层拒绝负控，以及陈旧连接展示、直接/quick/worker一致与精确范围。
1043真实红测12pass/13fail；1044相关110pass/0skip，1046完整受影响CLI/CI脚本1481pass/2历史optional skip。
原生解析器未改，最终完整traffic CI仍另验，不以本地受影响测试代替全部CI。

保留验证失败：1052将命令行source误当模板字面常量，执行前失败；1055把JSON省略的零端口当缺证据，
1056按schema默认零逐项与wire核对后重新完整运行。没有删断言、少测包或弱化字段比较。
两份fixture ZIP字节不变；原捕获、keylog、完整导出、参考、原始字节及截图全部在仓库外。

## N10继续开放

1039以当前父源码完整原包重新取TCP回调：原44观察帧、8段544B及全部原字节SHA保持，未确认
新opening后无额外接纳。1040独立遍历全部328127原记录，六tuple实际有74个候选，而旧52只是见证集合。
新增22候选包括不完整头、碎片及其他未接纳输入，不能称74个有效TCP包；1041已经补新参考输出。
完整头/可观察前缀里未见相互对应的SYN/SYN-ACK，也不证明真实端点不存在未捕获握手。
原stream分区/政策差异、完整代次/应用关联与真实端点状态仍归N10，SMB应用内容未重新分析。

## 证据和CI边界

外部根`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`：

| 闭合 | SHA256 |
|---|---|
| TCP-flags-public-check1056-01/summary1056.json | `b44eb95557ec6ee52d867beb7efe4d906eb78d5a152d473c388589dc77676c2f` |
| TCP-flags-reference-check1059-01/summary1059.json | `6a4169fc21af8bcae1c28f97d3298d5f172d28eec3e8f353d53ade24a9611162` |
| TCP-flags-qualification1060-01/summary1060.json | `088e3b138cd952cbcbfcda6207e1749a40c6b5ed83373d165b1ad9fd3f545444` |

已发布父`220d52898ac06f9951f2d82e53aab46c559d21b8`已独立47约定检查/91实际执行/2statuses全绿：
[traffic](https://github.com/yaklang/yaklang/actions/runs/41393130244)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41393134157)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41393138594)。314350 raw ID逐项与清单一致，
314325pass/25历史optional skip，42新增与131084 required均pass/0skip；官方产物SHA256
`802a2028e392ff8397cea2fe3fbafb6179bb2cd7fedb9084a0f847e32aab87fb`，综合闭合1051为
`acc2314be9a9e186461f90c3c8de091b2af870169939739947c73a3d0f13491f`。

父成功不认证本批469及随后说明。新最终head须事前冻结314375 raw ID/终态，保留父全部动作，
25新增全部pass；预期314350pass/25历史optional skip及131084 required pass/0skip，再独立闭合全部CI。
当前67父项57有限完成、7进行中、3 SMB搁置，10未关闭；非SMB17包13有限完成/4开放，
N09/N10/N13/N14仍约16–32 AI实施小时，构建/CI/回放等待与新问题另计，不代表核心协议全部完备。
