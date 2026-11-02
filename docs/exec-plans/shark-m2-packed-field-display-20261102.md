# 位字段值与真实字节来源：2026-11-02

M2-68有限关闭按需字段树的raw位字段展示缺口。代码和干净Windows构建为
`ddd4024d47bdfcd56f73755b052b5278013b3b01`。仍在M2 PR #5371，SMB继续搁置。

TCP规则的Flags宽12位。解析器raw合同是完整八位组加右对齐的末尾不足八位组；
例如线上控制字`50fe`的Flags值是`0x0fe`，raw返回`{0x0f,0x0e}`。
原页面把该容器显示为“2 bytes · HEX 0f 0e”，容易与原始字节或整数混淆。
本项不修改解析器raw合同、SDK字段、重组或会话政策。

新展示以原节点的raw类型、确切bit span、结果长度及末组padding共同限定；
不能凭字段名猜测。显示位宽、逻辑HEX和BITS；超过128位只显示有界BITS预览，
不把预览冒称完整HEX。未知/错误元数据及普通对齐字节保留原行为。字段选择仍高亮
包含该位字段的真实来源字节，不把逻辑位容器变成新的Source。

## 实际验证

证据根目录为仓库外`C:\tmp\yak-shark-ESP-SSH-UI-20261028`，捕获/密钥/PDML/完整导出/
原帧和页面均未入库或fixture ZIP。

| 验收项 | 实际结果与证据 |
| --- | --- |
| 原红测 | `field-bits-red1065-01`保留原位字段失败；1064是命令缺完整Go路径的执行前失败，不计测试 |
| 新回归 | 145个新增精确ID全部pass；真实TCP规则、独立bit cursor覆盖八种起点及14位宽，含跨字节、保留位、长预览、元数据拒绝、IPv4/VLAN/IPv6/loopback和worker |
| 完整受影响测试 | 1068：1626 pass、2历史optional skip、0 fail；stdout SHA256 `63a4994c4d59aad178721c0bd4b34c33ab71a56d3153bf92383ddae790a0923d`；测试输入与ddd源码文件哈希一致 |
| 双完整捕获 | 1070重新完整SDK/flow/CLI六导出，379455物理帧，全部逐字节相同；该证明是未改变导出，不是全协议语义正确性证明。summary SHA256 `cbfcfa90706930c16fd8321fef3a8a67c243941867c93543c14724ea83ba38b5` |
| 按需字段树 | 1071预先固定17物理引用/10不同原帧字节，排除SMB端口；1073实际34个旧/新worker执行，仅Flags文本变化，所有其他字段及范围/来源完全保持 |
| 新Wireshark参考 | 1072在两份完整原捕获上重新运行TShark4.6.9双遍、空配置、显式defrag/out-of-order/TSO；1074逐一核对17个Flags的逻辑值、unmasked原字、位置/长度、帧号及接口，含真实保留位；不判断checksum/端点状态 |
| 实际Windows | 原Sharon195146、接口226、57字节（派生文件明确为frame1），旧469与新ddd共40状态；新值`12 bits · HEX b00 · BITS 101100000000`、Source0 [46,48)的`5b00`完全一致；读回57字节、80×30/180×48、Tab/鼠标、CP936恢复和自有进程关闭均通过 |
| 主线程目视 | 主线程实际查看`Windows-field-bits-native1080-{old,new}-01/console/01-selected-field.png`两图；为实际console cell渲染，不冒称所有Terminal/DPI/IME已验收 |
| 综合本地闭合 | `field-bits-qualification1083-01/summary1083.json` SHA256 `6da59f0f072bf0d9804a42bf164b5c8aed0b92a86d256d22a6302dab5a97f996`；两个fixture ZIP哈希保持，空闲41744736256B，高于8GiB底线 |

1076实际UI失败因为真实回放附带观察节点，worker预选索引没有将Flags滚进可见区域。
1080增加有界实际滚动，再要求确切字段值和原字节高亮；原失败记录保留，没有注入DTO代替UI。

## 新发现和未关闭范围

`N14.U02`明确追加：1067的带合法IPv4 options控制中，通用IP规则没有先消费IHL声明的
选项，导致从选项位置读取TCP、出现错误字段或解析失败。该项继续修复，不算68已完成。
68的worker矩阵隔离普通无options头，原故障日志保留；完整IP字段/片段/预算/IPv6/承载范围
仍属于N14。`N14.U03`是本项有限位字段展示合同。

父检查点`422c9a49aacd9232d8db413059884910fda033b4`已独立47约定检查、91实际执行、2状态
全成功；314375原始ID/动作及131084 required全核对，25新增pass/零required skip。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41399204766)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41399208355)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41399212461)。官方产物13120848481，
17053205B，SHA256 `0658106ee228c619ffe2e517320c90529eb8d08ec4325c876873fa9d7f261b3d`；
综合1079 SHA256 `f4608c81889151a5918eb1033a132e2ee0042cac81e793e1ef4aa37f83c4b652`。
本次最终head必须另验全部CI和原始ID，不能借用父版本成功。

当前68父项58有限完成，7进行中/3 SMB搁置，10未关闭；非SMB仍17包13有限完成/4开放。
新U02粗估增加1–3小时，已知非SMB剩余17–35 AI实施小时，构建/回放/CI等待和新问题另计。
