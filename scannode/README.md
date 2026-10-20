# scannode

Managed AI execution inputs, resource admission, storage and scoped file access
are documented in [Managed input workspaces](../docs/legion-managed-input-workspaces.md).

## Legion professional task source failures

Source preparation runs before AI execution. A failure emits
`source.workspace.failed` with code `source_workspace_materialize_failed`,
followed by `ai.session.failed` with code `ai_session_bind_failed`. Both carry
the same actionable Chinese message; the event names, codes and payload fields
remain compatible with existing Legion consumers.

Messages distinguish recognized DNS, timeout, connection, source-access,
local-permission, capacity, trust, source-identity and invalid-ZIP failures.
For network failures, check connectivity and the configured source proxy from
the executing Node; an operator's browser or Provider connectivity does not
prove that the Node can fetch the source. Uploading a source ZIP can bypass a
Git endpoint connectivity problem. Unknown causes use a generic source-check
message. Raw errors, repository URLs, credentials and local paths are never
copied into either message.

## Legion HIDS

- Linux HIDS packaging, host-readiness checks, desired spec validation, and degraded-host behavior are documented in `../docs/legion-linux-hids-readiness.md`.
- The current Legion smoke entrypoint is `../cmd/legion-smoke-node`, and HIDS support requires building that binary with `-tags hids`.
- 推荐优先使用 `task legion_smoke_node_build_hids_linux_amd64` 产出可部署的 Linux HIDS 二进制；`task legion_smoke_node_build_hids` 更适合已经在 Linux 主机上的原生调试构建。

1. 本项目自带了一个脚本执行引擎，可以支持你想要的功能的编写，可以单独调试脚本，编写完成后复制到服务器即可
2. 分布式脚本编写数据流依赖本系统自带的 mq 框架
3. 现有的分布式指纹识别/爬虫是依赖服务器分发与控制的

但是需要注意的是，虽然这个分布式扫描节点依赖的服务器并不开源，但是并不意味着这一小段代码是不可用的，你可以把它当成 `common/mq` 和 `common/yak` 的典型实用案例。

稍作修改任何人都可以实现分布式服务器

## 启动与配置

1. 节点配置很简单，不需要配置核心服务器位置，只需要配置 MQ 地址即可，通信会根据代码协议进行接受任务与执行，汇报结果
2. 如果需要运行超多节点，请启用 --id 参数作为不同节点的区分

## 配置其他扫描器依赖（功能依赖）

## 编写分布式扫描脚本

### 获取参数

### 上报结果

上报结果分为几种内容:

#### 上报风险

上报风险函数定义:

`reportRisk(riskTitle: string, target: string, details: map[string]interface{}, subCategories: ...string) error`

这个函数用于上报：风险/漏洞，本质上是上报漏洞，但是某些漏洞没有目标，只有扫描风险，所以可以用这个简化设置。

#### 上报漏洞

`reportVul(vul: *assets.Vul | *tools.PocVul) error`

上报漏洞，这个漏洞对象一般是扫描器扫的结果，比如 xray 啥的，或者 pocinvoker 执行的结果，可以直接用于上报。

#### 上报弱口令

`reportWeakPassword(result: *bruteutil.BruteItemResult)`

本系统自带的爆破框架爆破的结果可以直接上报！很方便。

#### 上报指纹

`reportPort`
`reportFp`
`reportFingerprint`

支持本系统扫描指纹直接上报，非常好用了。

## Legion 扫描事件的本地持久化

普通 `DispatchJobCommand` 的事件先写入 `<base-dir>/legion/job-outbox`，
文件及目录权限分别为 `0600`、`0700`。成功入队前会同步文件、原子重命名并同步目录；
发送器按落盘顺序回放原始 protobuf、EventID、CommandID、Attempt 和执行 Session。
资产、风险等事件取得 JetStream 确认后移除；完成事件还要等待 HTTP 心跳中的平台落库回执。
已完成命令的精简回执保留 24 小时，避免重启后的旧命令再次执行。

节点重启时通过 bootstrap 声明已落盘的完成事件。配套 Legion 版本会验证同一安装身份、
原执行绑定和最新 Attempt，并授予最多 15 分钟的投递恢复窗口。重连不能延长这个上限。
恢复窗口由新 Session 的 HTTP 心跳维持，不修改原执行 Session；已有替代 Attempt 的结果会退入
`retired/` 留作诊断。恢复的是事件投递，不会重新启动已结束的脚本。
没有落盘完成事件的中断执行，其旧 claimed/started/progress/规则准备状态只保留为诊断，
避免把已失联的执行重新标成运行中；部分资产、风险仍按原绑定回放，由平台拒绝已被替代的执行。

活动队列上限为 10,000 条、256 MiB protobuf、256 个 Attempt；单条上限为 8 MiB，
实际发送仍受 NATS 服务端的消息大小限制。精简完成回执上限为 100,000 条。
队列损坏会阻止启动；持久化 I/O 错误或容量耗尽会暂停新执行准入并报告 degraded。
`retired/` 诊断文件需运维单独管理，不包含在活动队列容量内。

日志 `job_delivery_backlog` 提供待确认事件数量、字节和最老等待时间，
`job_delivery_receipt` 提供最终处置及从落盘到回执的时间。
需要同时升级配套平台协议和数据库迁移；带未确认事件的节点遇到不支持恢复的旧平台会拒绝继续注册。
这套机制不依赖平台数据库类型，PG 和 DM 使用同一节点实现。
部署时先升级支持上述恢复协议的平台并完成数据库结构迁移，再升级节点；
升级时保留原 `base-dir`，以保留待投递事件和完成回执。
当前运行验收范围为 Linux 本地文件系统与持久 JetStream；物理磁盘丢失、已产生替代 Attempt、
任意外部副作用的恰好一次、AI managed-input 事件及多机房灾难恢复不在该保证内。







