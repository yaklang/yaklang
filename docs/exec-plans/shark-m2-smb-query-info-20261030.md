# M2 SMB QUERY_INFO：字面字段、请求上下文与验收边界

本批继承 M1 `39729081a3300381154d96cf1b4866f23976260b`，只进入 M2 分支。
上一阶段 `dd63fc08b8cb8c396519c3dad0c611dc0bf9aeb6` 的 47 个 checks、两个 commit
status 全部成功；[完整 traffic](https://github.com/yaklang/yaklang/actions/runs/41001690849)
实际 checkout 和 v191 执行已核对：312117 pass、25 历史 skip、0 fail，131084 个必需
测试无缺失/未通过/跳过。本批代码仍须自己的精确提交 CI，不能借用 dd63 的通过。

## 本批实现

- `stream_parser.DecodeSMB2QueryInfoBody` 同时供原生会话和 canonical 规则使用。
  请求要求完整 40-byte fixed body/StructureSize 41，响应要求 8-byte fixed body/9；
  校验非空输入、输出缓冲的命令局部 offset/length，先宽化运算，不借后一个 compound
  命令的字节。零长度缓冲不解引用 offset，padding/trailing bytes 保留拥有的原字节。
- `STATUS_BUFFER_OVERFLOW` 走部分输出布局，其余错误走完整 ERROR/ByteCount 边界。
  零 SID/TID 的部分输出不准借用“会话丢失错误”的唯一 MID 回退；真正完整 ERROR
  保留原回退合同。残缺正文不会消耗 pending。
- 每个请求保留 InfoType、FileInfoClass、AdditionalInformation、Flags、输出上限；
  仅同一捕获域/连接代际内、反方向且匹配 MID/session/tree 或已观察 AsyncId 的响应
  可以发布独立 `Request …` 元数据。这些元数据没有伪造的响应 wire range。
  实际输出与请求上限分开；类型化内容暂未解码时明确显示 opaque。
- canonical 头分别选择请求 ChannelSequence/Reserved、响应 Status，以及同步
  ProcessId/TreeId、异步 AsyncId。读取 flags 的临时探测会回滚，不发表临时节点。
  Header/Body 的 Node 范围、原生拥有字节、调用者输入复用后再读均逐项断言。
- 页面保留 Header→Body→observed context；请求的 raw File ID 与 SDK 兼容 hex FileId
  仅在类型、完整值、同一命令的确切字节位置均被证明后消重。原 SDK 两种表示均保留；
  非法值/范围、另一命令、IPC 类型折叠不能授权消重。本地及两种 worker 验证字节来源。
- pending 记录计入新增 selector 的内存，测试保留超限拒绝，并增加恰差一字节的拒绝；
  不提高生产预算以容纳测试。状态正文 fuzz 增加 QUERY_INFO 已建请求入口。

canonical 的旧 `Compound Tail` 仍是 raw；本批共享正文 codec 对首命令有独立边界，
原生路径逐命令解码。不能据首命令通过宣称 canonical 已展开全部 compound 尾部。
七种 selector 对应的高级内容、quota/EA 输入、完整 ERROR contexts、取消/晚到恢复等仍开放。

## 来源先于候选结果

外部 Weber 捕获 SHA256 为
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`。
仓库外 `M2-SMB-query-info-source62-01` 在读取新候选结果前冻结 93 个完整 PDU、
113 个命令（56 请求/57 响应）、731 个固定字面字段及输入/输出缓冲的原字节和范围。
32 个独立 checker 合成控制通过。facts SHA256
`0e182897e8fdb484a346ddad10b08ae4f99230b19e7d6bf57b7fe3da4b696a3f`；
contract SHA256 `cf41b8a9afbdfa916dc406c3c6c754d8ccb29bbafec6f3d4ed4dfdbf731dc6af`。
同一冻结 checker 对 dd63 实际导出的基线为 93/93 来源、0 实际值错误、113 个正文
未发布缺口；新候选尚须独立执行，不能把 checker 控制或基线来源匹配算成产品验收。

请求全部 VLAN81，响应全部 VLAN80。原完整 Wireshark 参考中的 112 个响应 selector
装饰来自跨域请求，不是响应字面字段；即使 PDML 带 pos/size，也不能伪造对应范围。
真实响应在缺同域请求时应保留 opaque。Go 中另有同域正例、跨 VLAN/接口负控、异步
最终响应、同向/错误 SID/TID/零 ID 部分返回和已完成 MID 复用控制。

M2-04.19 的独立补充正文 oracle 也已冻结：原索引8–19 的 12 个单头完整 PDU，
307 字段（127 body），13 次物理观察中40022为唯一完整重复；269+12=281，无重叠/遗漏。
这12个不是原107恢复 focus，不能增加恢复指标。独立完整性程序通过，产品验收仍为 false。
最终 facts SHA256 `36439f8c8f031dc387258e0447279453f43cbf699fe2db6ed28670f9dc9ffd26`；
closure SHA256 `a3a4db3717544edaf67796bb646a1d183b177ff75dda45718101d82daa4b0c2c`。
两个 TREE_CONNECT PDML 坐标越出物理帧、六个 preauth 派生坐标单列为参考边界；
规范和实际原字节优先，未据此把产品差异归零。

外部捕获、keylog、物理/PDU 原始包、完整 JSONL/PDML 和详细字段审计继续留在仓库外。
M1 中央样本 ZIP、manifest 和 corpus index 不变。

## 本地验证与待闭合门

首次完整本地 traffic 矩阵为312239 pass/25历史skip/3 fail actions，日志 SHA256
`7f740ce94a00ed02a8807ee01632a21a096430ef77bb241c5abda794fb323d24`。
真实失败均保留：PCAPDB Windows 检查点共享锁（叶与父两个 action）和新异步头测试误把
未发表模板节点当作字段。后者改为同时检查 Result/NodeToMap 的实际输出和正确 AsyncId
范围；没有移除无伪 TreeId/ProcessId 的断言。共享锁修复见 M2-27，WAL/事务/导出断言不变。

修复后定向412项通过，日志 SHA256
`7b62aa2a13b7f26605baa4bcd66225e3eea4dee4bbe73b530da2157d85c499cb`。
后续请求头 union 调整曾在新测试中暴露临时节点寻址错误，修正为已成功解析 trial 的
Flags 值后，SMB/canonical/embed 矩阵319项通过，日志 SHA256
`2f30cf536719a99ab588f340cf0592d471f4d170a235aa745c6800aa98e89188`。
最后 YAML 调整后完整 CLI 再跑为994 pass/2历史skip，日志 SHA256
`f77780f164bb6413397c0490873fc0f1e9778c849bad3b6b538c0bfe6c0eacee`；
原调整前994项结果亦保留。最终提交仍需新 CI，不作最新提交完整通过的替代。

待闭合：本批精确 SHA CI；新构建大包113命令/731字段和原281来源门；新的 Windows
QUERY_INFO body 选择/高亮；新增 fuzz 入口的实际覆盖引导执行。来源资料、单测和旧 SHA
成功均不能替代这些实际执行。M2-04 的完整字段、连接/会话、TLS/QUIC 高级恢复继续开放。

规范：[MS-SMB2 QUERY_INFO request](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/d623b2f7-a5cd-4639-8cc9-71fa7d9f9ba9)、
[response](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/3b1b3598-a898-44ca-bfac-2dcae065247f)、
[部分返回的服务器行为](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/d64e0451-64a2-425a-848e-52a7dddab7b9)。
