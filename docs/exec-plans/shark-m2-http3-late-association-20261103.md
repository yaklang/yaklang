# M2-83：晚到 HTTP/3 请求的有界关联与递进展示

2026-11-03补记：最终 `e0fb24890448e2c4610b7e7ab1690525066352d5` 已独立全部全绿，
47约定检查名、91实际执行检查、两个statuses、316190精确Traffic ID/动作和官方产物逐项闭合；
131084 required全pass/零required skip，自动draft skip不计成功。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41563235938)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41563240309)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41563245187)。
官方17138032字节产物SHA256 `e0ec347c849ffc0ac22f0268028ed2a4281cf131a3db4a91e98f989865c6849d`；
闭合1718 SHA256 `b9c83c11c3790b0c1da3f3e81fdecd8b8c36abcd93a067deec524c97bd3750cc`。
后续[M2-84 晚到方法正文复验](shark-m2-http3-late-body-20261103.md)使用新运行0bb，不能沿用本次成功。

本项有限关闭 N09.R05 的七个已冻结真实来源变换：响应头先被完整解码，请求头后来完成。
只认证当前捕获连接、域和 STREAM 的已验证头关联，不认证完整消息正文或对端交付。
N09 整体仍开放；新发现的晚到方法正文复验列为 R06，不藏入本项完成范围。SMB 不分析、不修复。

生产实现和干净 Windows 构建为 `f1fdbbed67794aba206605127ff1b59ae2d2c01e`；
后续 `b62510ec0ad49c96b8269697994ce3a83833d9ed` 仅新增一个 fuzz 测试文件，没有生产代码变化。
本地完整运行与新增种子分开记账；最终说明 SHA 的全部 CI 另验，不能沿用父成功。
父检查点 `1505ee1036a6b0d418da0676530c62a35540ca0c` 已独立 47 个约定检查名、
91 个执行检查、两个 statuses 全成功，316141 原始 Traffic ID/动作和官方产物逐项闭合：
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41554533958)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41554539422)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41554544695)。
闭合1678 SHA256 `3396fa4d27c6ed618f9a2a857346523f2e197b6f91c4937749473412c6b78712`。

## 行为与接口

- 原始响应事件、`ResponseTo`、原始 Decode 结果和贡献字节不可变。后来的请求发布
  `HTTP3 Late Associations`，明确区分请求事务起点、头完成观察 PDU、响应头观察 PDU、
  原头字节 PDU 和物理包引用。QPACK 恢复观察号不会冒充原压缩头所在包号。
- 原生投影在私有快照封存 Flow ID、捕获域与 Observed In PDU ID；查询使用私有上下文，
  修改公开 Session、协议标签或旧响应缓存不能制造/重定向关联。元数据行保留私有应用身份。
- Inspector 当前视图对尚在缓存内、唯一 ID、同 Flow/域的响应增加
  `HTTP3 Resolved Associations`。原事件保持不变；不重放密文、PN 或 STREAM，不新增假包。
  每个响应最多64条边，消耗原有字节预算并随原环形条目淘汰；超预算与冲突显式标记不完整。
- `pdu.response_to` 继续表示原观察；新增 `pdu.resolved_response_to` 表示保留视图后来学到的关联，
  允许多 STREAM、多值与精确64位 ID。上下文字段没有 ciphertext range；歧义边不给唯一答案。
  另外提供 `http3.late_association.{request_pdu,request_headers_pdu,response_pdu,streamid}`。
- 受影响的响应包查询 revision 随上下文变化，列表、过滤器、详情不再保留旧缓存。
  请求与响应两侧的概览、字段树和 worker 保留关联摘要；大引用树继续留在包详情。
  已关闭/退休/RESET、不同 STREAM/域/Flow、未验证头和不完整请求不建立新关联。

## 来源与逐包验证

使用仓库外既有真实 QUIC 端点捕获 `QUIC-early-source1490-01/live.pcap`：
SHA256 `2c7bd3871b18bc3fbe0485bc798b20d396b8e1e2ad119278aa90b6cb3a8f35d0`。
两个实际连接复用同一对 UDP socket；1496 独立解密来源早于候选解析器。
七个变体来自这份实抓，不冒充七次独立抓包；keylog、完整 PDML 和所有导出留在仓库外。

`R05-associations1689-01/summary1689.json` SHA256
`088fc305864fb41493df7eaa9089b42ccd8b4058936c766df348f95f376c05fe`：
七个变体全部精确匹配独立请求/响应头、正文长度/哈希；Flow2/STREAM0、头观察号和字节引用正确。
原响应仍 `response_to=0`，当前视图能查询正确的后来请求，旧 SourceBytes 逐值不变。

`R05-final1685-01/summary1685.json` SHA256
`8432986a254dbecd865625f943d0bd4841f5ae62066bc98e87a6fb77959840a3`：
33 次新原生执行、11 次新的完整 TShark 双遍 PDML。TShark 4.6.9、空配置、相同 keylog/端口 Decode As，
二进制 SHA256 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
Wireshark 此输出没有通用 HTTP3 `response_to` 字段；关联按独立连接/STREAM/头来源核对，
不宣称一个不存在的 Wireshark 生成字段已经相等。Wireshark 双遍补原树与本系统不可变观察另加当前视图的差别保留。

严格来源审计1690：21原包、353字面检查、零缺口，SHA256
`201fe527a827146c4215f332fc229170f233449139f7c57dc7146388b76950db`。
H3审计1691：四消息、14头、四正文、八帧、两个原始响应关联正确，SHA256
`d29d32733c9e26e85a405f6767401a7aaebc11487689f662701e2267a8161cd3`。
控制审计1692：13目标/7有效/6 PN顺序拒绝、276检查，SHA256
`e1184166d8bf117e7ac90a45d6ac2d962ba66889c5cec9ce825adc26a84924b3`。

## 回归、fuzz 与 Windows

完整 `R05-full1686-01` 对 f1fdbbed6 执行 parser/pcapx/CLI/scripts-ci：
291084 pass、25历史可选 skip、0 fail，655.422s；stdout SHA256
`4fb7d6df74af6e1f4a5426795598210c86383a08014485d731f62b1eba982ccc`。
后续只新增 fuzz 文件，六种子加父共七个 ID 在1697全部通过。严格合并集合291116唯一 ID，
291091 pass/25历史 skip/0 fail；父291067 ID/动作全保留，49新 ID均pass。
这不是 b625 一次全模块本地运行；最终 SHA 的远端完整运行须独立验证。
原有测试断言、两个捕获ZIP、协议期望与 CI 配置均未修改。

`FuzzHTTP3LateHeaderAssociation` 经原生 STREAM 消费者检查分片、丢失后缀、非法头值、
退休、RESET、重复提交、旧响应不可变和连接内存界限；保护层/捕获域由另有原生集成测试验收。
插桩运行1699：六种子全完成、21109执行、0 fail，62.079s；单CPU、768MiB job、
最多两个同时进程，另有第三进程被拒绝的正控，不覆盖内部 worker 标志。峰值278941696B。
五个生命周期内总进程包含 worker 重启，不是五个同时进程。binary SHA256
`92d49072b8e6fb99f2a8f184ba2e30cc91c16baedf8e961da2102a7a154325af`。

实际 Windows 控制台1695共36状态：请求 PDU29/物理19、响应 PDU24/物理16 两侧关联树，
80×30/180×48、鼠标/Tab、CP936恢复、worker、所有进程退出通过。
选中上下文不产生假 wire 高亮；右侧原捕获 PDU 与独立1496摘要及原物理帧包含关系精确一致。
闭合 SHA256 `cef0c3f2035e4e3b4296312212f825f85d1af246e155f93834743afa03022e38`。
根线程另看实际响应截图 `Windows-R05-1695-01/response/console/01-selected-field.png`。
1694首轮校验错误地假定右侧默认是整物理帧，实际标题为 captured PDU；失败保留。
1695改成独立来源 PDU 字节、唯一物理帧包含关系和无假高亮三项同时验证，没有修改产品来迎合测试。

两个大型公共捕获共379455物理记录、306855 SDK事件、379926 CLI记录（含471 publication）。
六份完整回放1693全部逐字节等于父导出，SHA256
`d5b7080d778e2151b0faedbe16d5236990a8abbb7432319b87756204ece013d0`。
这是这些既有输出的回归保持，不是全部协议、字段、会话已与 Wireshark 等价。
完整性闭合1700 SHA256 `71bb2c86f14d87f8a3acaad6d69113b6910dd267bdac9a65a03ab105f32e89a1`。

## 保留失败与下一项

原生1668两个关联正例失败、一个负例通过；1671公开关联字段修改影响查询的六套件/版本失败保留，
修复私有查询上下文后1682为966 pass，UI1683另六项pass。
1681七个 fail action 来自新增元数据测试误用不存在的布尔协议字段；改成现有 `protocol` 字段，原关联断言保留。
外部 draft overlay1649的两个正例失败/一个负例通过及其单独 vet 兼容设置不改变仓库/CI默认 vet。

新增 N09.R06：响应正文先完成，HEAD 请求后来才完整时，自动状态仍未重新检查原响应。
仓库外 authored overlay1696为两个pass/两个fail action（含父），没有声称真实抓包已覆盖。
同一保留 body 状态手动 `checkBody` 会得到“DATA content forbidden”，但原生晚到方法路径未发布/保留该响应错误。
后续应有界地重验旧方向正文、区分后来语义诊断与当前请求有效性，并补 HEAD/CONNECT、FIN、缺包、复用、来源与实际页。

当前83父项73有限完成、7进行中、3 SMB搁置，共10未关闭；非SMB17包13有限完成/4开放。
计数不重复把 R06 算成新父项；N09.R05有限关闭、R06明确追加到既有开放N09。
QUIC/H3完整生命周期、六TCP政策fuzz、TLS高级阶段、IP字段/重组仍需验收。
非SMB粗估18–36 AI实施小时，因新增R06重新估算；不含CI/构建/回放等待或新问题。
最终说明SHA须独立316190原始ID（316165pass/25历史skip）、131084 required零skip及全部CI。
