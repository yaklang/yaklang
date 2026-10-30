# M2-28：CI暴露的Yak入口/include顺序问题

此修复只进入M2，M1 PR5353及其验收提交不变。来源为60c87b9的
[SSA frontend CI失败](https://github.com/yaklang/yaklang/actions/runs/41006818329/job/123904909146)：
原有`TestParseProject/parse_project_with_entry`期望一个包含3的b值，却发表两个Const3。
本地原断言40轮中5轮失败，日志SHA256
`9b75e490f56fc8581a55986fee916e5a05e0224a67519e932db1ff3c966cf22e`。
失败记录保留，没有删断言、把重复值去重后再断言或靠重跑碰成功。

## 行为与修复范围

Yak include使用独立子程序，但原AST入口读取application的外层editor来检查/记录
FileList。这把被include的源与调用方混淆。只修editor仍不足：独立源码根和include的
执行作用域不同，源码完成顺序还会决定重复定义或空模块。该中间候选40轮仍9轮失败，
包括Undefined-b，日志SHA256
`4b2ea2080757b4909fb71caad181498bb1da9b7c958e8aaba844b8112f891d0e`。

- builder使用自身editor。应用根的去重只作用于应用根；include子程序由自身加载缓存
  和作用域负责，避免把另一个执行作用域的已读源码误当作已构建模块。
- 显式Yak入口现在选择**可执行根**，其他源码继续保留供include。此前新的项目编译引擎
  没有读取这个选项；不能把此次行为描述成仅改标签或旧的“先入口再执行所有根”排序不变。
  无入口时仍编译所有可处理源码，其他语言行为不变。
- units和legacy共同筛选根，非入口源码仍注册editor，但不能通过额外helper的
  `PopEditor(true)`冒充已执行。include目标可用与FileList/visited状态分开。
- 自动识别Yak、多个/逗号分隔/重复入口、缺失入口都有明确控制。路径统一斜杠，
  先按文件系统根相对路径精确匹配，再按ProgramPath相对路径匹配。
  匹配遵从扫描名称的大小写；未声称覆盖大小写不敏感文件系统的全部别名。

原40轮在完整候选通过，80个父/叶action，0fail；日志SHA256
`7c9c13c8b7e0f434d4439b64c15e9298ee98a9912929b0cabe51e987269865b1`。
初版41项合同也通过，包含两编译路径×1/3 workers、两种路径先后、自动语言、
ProgramPath相对入口、无入口、两个不同include及无效入口。该结果不能替代后续
新增的后台editor/visited/FileList与路径歧义控制，也不能替代完整受影响回归和最终CI。

## 本地回归的独立问题

第一次完整受影响矩阵保留672 pass/11历史skip/2fail，日志SHA256
`0fef6f87ae3c0544cd5bbd8d90b193df9d0619c7ecf0f67433590a8d9e21cf24`。
完整Yak frontend已通过；两个失败均在ssaapi测试夹具：

- headroom决策测试继承执行器的`GOMEMLIMIT=2GiB`，因此正确阻止自动GC调节却与测试
  的32GiB模型预期冲突。测试显式隔离配置，再检查显式2GiB仍阻止自动调节。
  只调用决策函数，不改变`runtime/debug`活动限制，实际Go执行仍限制2GiB。
- Windows ZIP清理只尝试直接的ZipFS，遗漏UnifiedFS包装。测试现在解包并断言关闭
  实际ZipFS成功；原临时目录清理和文件系统语义断言保留，不修改生产关闭行为。

第二轮完整矩阵的ssaapi/yak2ssa通过，新增来源断言却用FS相对键查询以
`/programName/path`为合同的registry，产生16个叶/5个父失败action。
该错误测试结果保留（661 pass/11 skip/21 fail），日志SHA256
`b87fa4a34ac59017fe371df003b8770ac50a0e82788864b590ed55613ba95460`。
修正为声明的源URL后，editor内容、未visited和FileList缺席三项均保留；没有改生产
registry来迎合错误键。完整Yak前端再跑281 pass/4历史skip/0fail，日志SHA256
`24adc76cf2e830a10dff7a93ce163b61db37b51d8e98ca3a55ecd31eff0078b0`。

受影响包的最终本地闭合为682 pass/11历史skip/0fail，693个ID各有唯一终止action；
ssaapi/yak2ssa接受第二轮通过结果，Yak前端使用最终完整复跑，期间唯一Go变动是该
前端测试的URL查询。生产源码/其余测试哈希保持，不重复计算失败轮次为成功。
闭合摘要SHA256 `7fe16ef90bc3b892313c89ab4cecdb1d8063e909dd1d6050766f939a27f64500`。
11个skip原样保留（7个可选后端/子进程入口及4个既有Yak限制项），不作为通过。
最终Go源码冻结后，原失败用例又独立40轮通过，80个父/叶action均有平衡终止；
日志SHA256 `072cb2a94b5f0067de08c52cd5b7eae8d68162d6a9b2b46c2ce7e38ff9fcb650`。

最终精确提交CI尚待闭合，归M2-11；此处只接受有限本地合同，不借用60或dd63的CI。
此SSA合同与SMB字段、会话、TLS/QUIC恢复验收分开，不能增加协议正确性覆盖数字。
