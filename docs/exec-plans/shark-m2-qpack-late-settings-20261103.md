# M2-82：迟到 SETTINGS 的 QPACK 恢复与拥有字节展示

本项有限关闭 N09.R04 的当前捕获连接恢复合同。SMB 不分析、不修复，N09 整体、R05、
TCP 政策、TLS 高级阶段及 IP 字段/重组仍开放；不声明核心协议完备或与 Wireshark 全部等价。

解析器提交 `4024d77822b78b6b8b6c91a702a686ad23bf227e`，容量边界修正
`7825d249baa5fecbe285585d2d9ffc2e65492fd7`；最终展示/干净构建提交
`05102df8cefdf0b9c360b77c6a39e48449273b16`。最后一步只改变两个 CLI 展示/测试文件，
解析器、原有断言、捕获 ZIP 和 CI 配置保持。

父检查点 `5255658f94dc032ee21bde5d1a62a3b1663d06cb` 已独立全绿：47 个约定检查名、
91 个实际执行检查和两个 statuses 全成功；自动 draft skip 排除。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41542365029)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41542370968)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41542377065)。
官方产物 17008618B，SHA256 `67348be4d8b107d62e2b8830c42daae3a73047619474a342f086af91ab762615`，
checkout/upload/digest/CRC 和 316075 原始 ID/动作已逐项核验；131084 required 全 pass、零 required skip。
macOS 首次未取得 hosted runner 的容量故障及取消记录保留，第二次 41 个任务全部成功。
闭合1607 SHA256 `c841e8d2713b477bce262c8bb76539c4c7cc62c8077220bd77990b5bbd8845cf`。
父成功不认证本项的新提交，最终说明提交还须独立全部 CI。

## 修复与边界

- 区分「没有观察到对端 SETTINGS」与「完整 SETTINGS 已确认容量为零」。前者保留 context-required；
  后者拒绝编码器指令，包括 SetCapacity(0)。原生已观察容量超限和过大表项归为协议错误，
  实际本地预算不足继续 limited/ResourceExceeded。显式声明的 decrypted-stream API 保留原有限额合同。
- 合法 SETTINGS 到达后，只重试当前 owner、方向和 STREAM 已拥有的未解析后缀，不重新消费 QUIC 包、
  包号或已应用的 STREAM。关闭/退休 owner 不恢复；接口/VLAN、连接代次及两方向上下文隔离。
  有效 SETTINGS 后接坏控制帧时，先前有效上下文的观察仍保留，后面的拒绝也不抹掉。
- 缺设置的 QPACK 只等待自己的解码上下文，不再中断同包后续独立已认证 STREAM。一个被拒绝的
  blocked HTTP 字段段不再阻止另一个可恢复流；资源错误继续向外传播。
- 编码器表按批试算，含插入、容量改变和驱逐。失败不留下重复插入前缀；先前单独完成的指令仍保留，
  不宣称整连接回滚。现有会话预算还计入表项数组试算副本的保守空间。
- 后到设置的结果包含指令、容量、状态、原 PDU、上下文 PDU 和原始字节见证。已完成前缀的见证
  按实际消费区间裁剪，不吞入尚不完整后缀的包、父项或字节；原 buffer 和见证预算继续适用，未新增无界密文队列。
- 页面优先展开恢复语义，独立拥有指令字节并精确高亮。来源说明中的同名值不能继承指令字节资格；
  本地及 worker JSON 两条路径都验了正反例。

依据 [RFC 9204 §3.2.3](https://www.rfc-editor.org/rfc/rfc9204.html#section-3.2.3)、
[§4.3.1](https://www.rfc-editor.org/rfc/rfc9204.html#section-4.3.1) 和
[RFC 9114 SETTINGS](https://www.rfc-editor.org/rfc/rfc9114.html#section-7.2.4)。
未观察到设置时，不借另一捕获连接的旧容量推断当前连接；完整跨连接 0-RTT remembered SETTINGS
和全部 QPACK/QUIC 生命周期仍属于开放 N09。

## 实抓来源、逐包参考与实际页面

沿用独立来源先于两个解码器的 Windows loopback 实抓1490：相同 UDP sockets 的两个 full/resumed
HTTP/3 连接，19 个物理帧、21 个保护包。原 PCAP SHA256
`2c7bd3871b18bc3fbe0485bc798b20d396b8e1e2ad119278aa90b6cb3a8f35d0`。
十个控制是其冻结字节/顺序变换，不冒充十次新抓包。TShark 4.6.9、独立配置目录、两遍解析，
显式 keylog 且 ignore embedded secrets。

在 delayed-early-after-server-response 中，原 PDU 28 / frame 18 的 server encoder stream 7
先到，peer SETTINGS 在 PDU 29 / frame 19 才出现。现恢复逻辑偏移 1、长度 3 的 `3f e1 1f`，
解析为 Set Dynamic Table Capacity 4096。字节及原保护包 SHA 与独立 AEAD 来源一致；
新参考的 `http3.qpack.encoder.opcode.dtable_cap` 及 `.val` 也分别是相同字节和 4096。
Byte Source 的贡献仍是 frame 18 / PDU 28，frame 19 只提供上下文。

Wireshark 两遍解析把结果补到原 frame 18；本系统保留不可变的原 context-required 诊断，
并在后到上下文的 HTTP/3 树中公布恢复结果及原来源。两者展示时机有明确差别，
不通过改写旧事件假装首遍成功，也不将此差别当字段/来源已经全部一致。

最终 UI 构建重新执行 33 个 native 命令、11 个完整新参考；33 份 SDK/flow/CLI 小源导出
逐字节等于解析器检查点。原始 21 包全部按独立来源/新参考核对，353 个 QUIC 字面检查；
13 个变换目标为 7 个有效、6 个包号顺序拒绝，276 个字面检查。四个 HTTP/3 消息、14 个头值、
四个正文、八个帧 envelope/stream 范围及原始两条请求响应关联保持。

最终 Windows 实际运行 18 个状态，3 字节独立源/高亮、80×30 和 180×48、鼠标/Tab、
CP936 恢复及全部子进程关闭通过。根线程实际看过最终 selected-field PNG。
全部原始抓包、keylog/私钥、完整导出/PDML、控制台/PNG 和 fuzz 产物留在仓库外。

## 测试分母与证据

外部证据根 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`：

| Profile | 实际范围 | 证据 |
| --- | --- | --- |
| 设置未知/已知零、迟到重试 | 双方向、默认/显式零、非法/残缺设置、部分指令、批原子、资源传播、关闭/退休 | 1599 原型 8 ID 中 6 fail；最终新增原生/CLI 合计 66 ID 全 pass |
| 真实保护包与域隔离 | v1/v2、三套件、双方向、DTO 变更、接口/VLAN、重复包、同包其他流 | 解析器检查点950相关pass；后续完整CLI见下，旧断言保持 |
| 完整协议回归 | 7825：291064 ID，291039 pass / 25 历史 optional skip / 0 fail | full1633 stdout SHA256 `26fc08eea4b16cbab1988eb3a30414d7916ff6306fb72ca78ec7d3781d212afb` |
| 后续完整 CLI 包 | 当前0510内容：1685 pass / 2历史optional skip / 0 fail；只改变两个CLI文件 | UI-full1651 stdout `3684f90c983acbafea85caa2e53a2afde925ea808605999c17f54147f898f8e2` |
| 合并本地测试集合 | 291067唯一ID：291042 pass / 25历史skip / 0 fail；父291001动作保持，66新ID均pass | 不是0510的一次全模块运行；最终SHA完整CI另验 |
| 新插桩 fuzz | 71334次、4种子、1 CPU / 768 MiB / 最多2同时进程，第三进程拒绝控制通过 | fuzz1638，stderr `124c405e1f8da88afce197da6de71863cd0bbb57fe0de297b01d256ea2f0d02d` |
| 小源及字段 | 33 native / 11新参考、21原包、13控制、4 H3消息/14头/4正文/8帧 | final1653、source1658、H3-1659、controls1660、QPACK proof1654/reference1655 |
| Windows | 最终0510，18状态，3fe11f精确独立源 | Windows-UI1656，闭合 SHA `9aa848c771107e60570b7e184c4d0449083e024434d242e984f183b1f720f108` |
| 大捕获/原四域 | 解析器7825的两大捕获六完整导出与父逐字节一致；四个未改strict checker全0 | public1636 SHA `c9669d4a548ac593db58ba5f4998bdb88da9b24b47c495deec275ab03f23b6cb`；domains1639/check1645 |

最后的 CLI 资格收窄不改 native、fuzz 或 JSON 导出路径；解析器/大捕获证据明确归属7825，
不冒充最终展示提交的新六次完整回放。最终干净构建、小源新参考和实际页面归属0510。
资格1662检查这两个 CLI 文件的唯一差分、当前全部输入哈希、原解析器代码不变、33个新输出等值，
以及全部父测试动作和两个 ZIP；SHA256
`ae33639be8036036e8dc408951355d8f700a5e0967cea4c384ac4045d3922c01`。

失败保留：1598为错误零容量假设/测试 helper 错误，不作协议证据；1599规范校正后确实重现；
1611测试未走emit就读CompletedIn、1613旧显式API分类合同、1615新测试误写状态字符串均保留后修正。
插桩1628在771次后发现新fuzz构造把65字节表项当作能装入64字节；保留最小输入，
补所有切分位置的明确拒绝回归，并让合法随机分支使用足够容量，原有断言未删改。
1650本地/worker来源说明继承错误范围的三个失败均先重现，最终常规CLI回归含修复后断言。
1661资格脚本误把目标数组当数值，未产出通过证书；1662纠正读取形状并保持所有门。

## 剩余工作与最终 CI

本项后为82父项、72有限完成、7进行中、3 SMB搁置，共10未关闭；非SMB仍17包、13有限完成、4开放。
R04当前连接有限恢复关闭，N09完整生命周期、R05七个晚到请求关联、N10六个TCP政策fuzz、
N13完整角色/阶段/协商依赖和N14 IPv6/其他封装/字段/缺片冲突预算恢复继续开放。
剩余非SMB粗估17–34 AI实施小时，执行等待和新问题另计，不按协议/测试数量换算完成率。

R05在最终解析器源上重核仍有七个 profile，字面头/正文一致但已发布响应的response_to仍0。
1649仅仓库外command-local overlay固定两个正例失败/一个错误请求负例通过，不属于本项完成分母；
Go1.22 vet无法打开不存在于磁盘的虚拟新增文件，首次1648零测试失败不算功能复现；
仅该外部待修overlay关闭vet以执行断言，仓库测试和全部CI保持默认vet。

最终说明提交必须独立冻结并验证316141个Traffic原始ID/动作（316116 pass、25历史optional skip），
保持父316075动作、全部66新增pass、131084 formal required全pass/零required skip，再核全部约定CI。
在证据未闭合前维持M2-11进行中、PR draft，不借父CI或自动draft skip通过。
