# OSPFv2 接入、逐包字段与 Windows 页面

`cd7af6a67e998ed1a6466a11f2b37c66fad742a8` 接入 OSPFv2 原生观察、共享
bin-parser entry、私有类型化字段和递进页面。两份公共原包中的556条完整报文已从
原字节、隔离 TShark4.6.9、实际 native/CLI 输出三个方向核验；这不是新的 TOP30
全量标签普查，也没有宣称核心协议已经完备。

## 原包和参考范围

仍使用外置 Sharon/Weber 原包，SHA分别为
`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`、
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`。
37输入合计383936个物理记录；两大原包合计379455个。协议消息、CLI结束发布和
物理帧分别统计，没有给结束发布制造帧号。

| OSPFv2 消息 | Sharon 来源/参考/native | Weber 来源/参考/native | 合计 |
|---|---:|---:|---:|
| Hello | 204 / 204 / 204 | 204 / 204 / 204 | 408 |
| Database Description | 29 / 29 / 29 | 29 / 29 / 29 | 58 |
| Link State Request | 7 / 7 / 7 | 7 / 7 / 7 | 14 |
| Link State Update | 28 / 28 / 28 | 28 / 28 / 28 | 56 |
| Link State Acknowledgment | 10 / 10 / 10 | 10 / 10 / 10 | 20 |
| 总计 | 278 / 278 / 278 | 278 / 278 / 278 | 556 |

统计按原始Version/Type和来源帧绑定，不把 Wireshark 的聚合 `OSPF` 标签误当作
全部是v2。15506个完整wire字段中，15276个直接来源字段及28个需拆分E/TOS位的
字段与参考核对，共15332次参考值/范围检查，无差异；202个reserved字段没有
公开参考字段，单独列为参考覆盖缺口。684个opaque字节字段通过长度/hash核验。
Wireshark的标签或未提供的校验结果没有被当作认证、完整性或整个协议正确的证明。

## 实现与错误边界

共享入口为 `ospfv2_observed/OSPFv2ControlFields`，目录状态保持partial，原生
能力清单同时列出v2/v3。旧兼容YAML入口的范围没有被改写成全量语义支持。
按IPv4 protocol89及实际Version选择；UDP端口、ICMP引述和孤立分片不能授权
这条观察路径。字段具有独立PDU范围，修改公开Raw/Session不会改变私有快照。

五类消息以及LSA1/2/3/4/5/7有有界解析。未知完整LSA保留opaque正文，并只按
它声明的边界继续下一项；残缺字段不补零、不创造空LSA。LS Request的四字节
类型与LSA头的一字节类型保持各自宽度。TOS/MT-ID保持原始选择器，不据此宣称
已选拓扑或把Link Data的IPv4位形强认作接口地址。

包checksum、完整LSA Fletcher和LLS checksum使用不同范围。LSA字段0000即使
残数为0也拒绝；包checksum字段为0则可以在数学上有效。DB/Ack只有LSA头，
不能验证缺少的LSA正文。认证digest位于声明的OSPF正文之后、LLS之前；未知认证
方案不猜LLS位置。CA TLV序列/长度符合头部仍只证明字段一致，不证明MAC正确。
正文预算耗尽不会掩盖独立尾部完整性失败，也不会证明尚未读完的CA不存在。

Instance ID和8位AuType依据[RFC6549](https://www.rfc-editor.org/rfc/rfc6549.html#section-2)；
LLS位置、对齐与CA约束依据[RFC5613](https://www.rfc-editor.org/rfc/rfc5613.html)；
LSA非零checksum依据[RFC2328 §12.1.7](https://www.rfc-editor.org/rfc/rfc2328.html#section-12.1.7)。

IP分片乱序/重复完成后只发布一条完整观察；缺片不把后缀认成消息。同Router ID、
不同Instance和后来报文逐个独立读取，未继承旧字段；没有给路由通告制造FlowID、
TCPFlowID、事务、response_to、邻接、LSDB或已安装路由。

## 实际闭合

| 检查 | 结果 |
|---|---|
| 556真实报文及42个边界/预算场景 | 598次direct，16446个wire字段值/范围，0失败 |
| 上述元数据/派生检查 | 4132项，单独统计，不充当wire覆盖 |
| 原生完整对象 | 新增556条来源构造对象；307262条旧完整对象保留 |
| CLI完整行 | 556条来源构造行；383962条其他完整行保留，共384518行 |
| 两份流导出 | 整文件逐字节不变 |
| 不含新增消息的35个输入 | native和CLI整文件逐字节不变 |
| 新增typed字段 | 14620个literal与1600个derived，共16220个，范围/归属/重复顺序正确 |
| 五包完整回归 | 285594通过、24跳过、0失败 |
| 页面顺序及父边界后续检查 | 45通过 |
| OSPFv2 fuzz | 60秒、243255次、0失败 |

新增消息会改变两大包里的生成PDU编号。比较使用一个可注入的旧/新编号账本，
仅允许已有明确schema中的生成引用替换；线上的整数、物理帧、流、会话、数组
顺序和类型不归一化。556个新完整对象及CLI行从独立原字节事实构造，预期在
candidate回放前冻结。完整比较没有意外差异；不能将此结果表述为所有ID均字面不变。

最后另补公开 `OSPFv2Checksum` API的声明长度守卫：附加尾部不参与计算，缺少
声明字节不能提供证明。原生调用原本已经校验n并传入 `w[:n]`，调用路径不变。
新47项相关/字段注册检查通过；重新编译direct探针的598个完整结果与前次逐字节
相同，独立字段审计再次通过。此补充没有冒称重新执行native/CLI/Windows。

## Windows 实际页面

使用三个从Weber提取、原始字节不变的单包回放：原帧25的Hello、849的Network-LSA、
11240的认证LLS。它们是来源可查的检查夹具，未声称新抓流量或完整大包常驻缓存。
默认20000包/64MiB，显示Ethernet/IPv4/OSPFv2层级；字段顺序为头、认证、正文、
LLS、完整性。具体名称包括`2 (Network-LSA)`及`LLS Cryptographic Authentication`。

三页的物理/PDU字节、选中范围和高亮均完整核验，含鼠标tab、缩放与CP936恢复。
主agent亲自查看了三张实际控制台cells渲染；不是Windows Terminal像素截图。
认证页的序列/长度匹配显示true，同时认证仍为false，避免把一致性当作认证成功。

## CI、保留失败与剩余范围

cd7af的[Essential](https://github.com/yaklang/yaklang/actions/runs/40886652682)、
[完整SHA流量](https://github.com/yaklang/yaklang/actions/runs/40886638891)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/40886652650)和
[路径大小写](https://github.com/yaklang/yaklang/actions/runs/40886652809)均成功。
草稿自动流量40886652672为skipped，未计为通过；后续提交的CI另行跟踪。

第一次页面回归发现OSPF父长度拒绝码未被展示层识别，已补v2/v3拒绝分支。
第一次whole检查器把只适用于两大包的引用schema用于其他输入，拒绝HTTP/3的
事务编号；改为对无新增消息的输入要求整文件字面一致，原始预期/构造器/来源
未变，未放宽HTTP/3事务映射。认证LLS第一次滚动位置未显示目标字段，仅调
导航，原值/范围/数据不变。原失败、版本与源摘要都保留。

该轮不证明SA/MAC、重放防护、路由收敛、完整LSDB，也不覆盖全部认证方案或
扩展LSA；真实源没有的Summary/NSSA/可选TOS条目主要由明确人工控制补测。
其他TOP30高级协议、GTP及更多SSH/QUIC边界仍开放。

原包、keylog、完整case/plan/预期/导出/账本/PDML、日志、程序和图片均未入Git。
仅归档43份实际源码及摘要，见[源码清单](../../../common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ospfv2-control42/source-manifest.json)
及[证据索引](ospfv2-cd7af-summary.json)。
