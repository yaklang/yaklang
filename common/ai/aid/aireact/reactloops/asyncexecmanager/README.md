# 异步执行管理器接入

`asyncexecmanager` 只依赖 Go 标准库，负责 execution ID、调度、状态、取消和等待。
工具调用、抓包任务、进程等资源负责自己的执行和观察数据。Manager 持有资源的
Inspector，按需读取；不订阅日志事件来重建 stdout、stderr 或 result。

## 最小接入：两个函数

常规接入使用 `NewResource(execute, inspect)`，无需声明新类型或实现接口：

```go
resource, err := asyncexecmanager.NewResource(
    invocation.Execute,
    invocation.Inspect,
)
if err != nil {
    return err
}
receipt, err := manager.Start(asyncexecmanager.Request{
    Resource: resource,
    Metadata: asyncexecmanager.Metadata{Name: "capture"},
})
if err != nil {
    return err
}
// 保存 receipt.ID，后续调用通过同一个 manager 管理这次执行。
```

这里的 `invocation` 是接入方为本次执行准备的对象，方法签名为：

```go
Execute(context.Context) error
Inspect(context.Context) (asyncexecmanager.Observation, error)
```

两个函数也可以直接使用闭包。已有单独的 Inspector 时，把它的 `Inspect` 方法
传给 `NewResource` 即可。构造函数检查两个函数均非 nil，不执行工作，不读取输出，
不创建后台任务。完整可运行示例见 [example_test.go](example_test.go) 中的
`ExampleNewResource`；直接实现接口的示例见 `ExampleManager`。

每次 `Start` 准备一个独立的 invocation 和 resource。Manager 不检测两个不同 ID
是否指向相同的资源；不要把只能运行一次的资源重复提交，也不要让多个闭包共享
可变的“当前调用”字段。

## 已有对象如何接入

保留两个小接口，供已经有执行对象和观察对象的系统直接适配：

```go
type Resource interface {
    Execute(context.Context) error
    Inspector() Inspector
}

type Inspector interface {
    Inspect(context.Context) (Observation, error)
}
```

Resource 可以返回自己作为 Inspector，也可以返回现有工具调用的观察对象。
`Inspector()` 在提交阶段获取，应快速返回稳定、非 nil 的对象；即使提交随后因为
容量或 ID 冲突失败，也不能产生执行副作用。观察器应能读取 pending 阶段的空数据，
以及终态后的最终数据。

不需要额外实现 Stop、Wait、List、状态管理或结果上报接口。停止通过 Execute 的
context 传递，等待和状态由 Manager 管理。

## 执行和观察约定

| 接入函数 | 必须完成的职责 |
| --- | --- |
| Execute | 使用传入的 context 启动并运行工作；响应取消；等待自身 worker 和清理完成后返回 |
| Inspect | 使用本次观察的 context；读取资源自己的并发安全快照；保留终态数据；不消费输出 |

执行函数应直接等待真实工作结束。不能启动一个 goroutine 就返回 nil；也不能仅因
context 取消就提前返回，把仍在运行的工作留在后台。否则 Manager 会误判完成并释放
并发名额。包装已有异步调用时，要等待它的真实完成信号和清理结束。

如果底层 API 需要显式 Close/Stop，适配器负责把 context 取消翻译成这些操作，并
等待停止完成。外部进程也由适配器处理终止和 Wait。Go 的 context 无法强制杀死
不配合取消的 goroutine；此时记录会保持 stopping，占用并发名额，Shutdown 可超时。

使用 `Start` 时，取消可能发生在 Execute 开始之前，因此创建 resource 时应只准备
描述和观察器，把句柄、worker 等放在 Execute 内创建并清理。接管已有任务使用
`Adopt`，交接失败时仍由原调用方负责执行和清理；详见下文接管协议。

Inspect 不能返回仍会被其他 goroutine 修改的切片，尤其是 `json.RawMessage`。
资源应在自己的锁或快照机制下完成复制。Manager 对返回的 JSON 再做防御性复制，
无法修复观察器读取期间的数据竞争。Execute 的 context 结束后，Inspect 仍应使用
独立传入的观察 context 读取最终结果，不要依赖已经取消的执行 context。

`Observation` 字段约定：

| 字段 | 含义 |
| --- | --- |
| stdout / stderr | 资源提供的当前输出快照；输出上限和截断由资源负责 |
| result | 资源当前可提供的结构化结果；非空时应为合法 JSON |
| partial_result | 中间结构化结果；非空时应为合法 JSON |
| truncated | 当前返回的数据是否被截断 |

无输出的资源可以返回空 Observation，不需要模拟 stdout。大型结果可以用合法 JSON
表达资源引用或摘要，由原资源负责保存内容。Manager 不验证业务结果或 JSON 内容。

Manager 会恢复执行函数、获取观察器、读取观察函数在调用 goroutine 内的 panic。
接入方自行启动的 goroutine 仍需自行处理 panic 和退出。直接获取 Inspector 后调用
其 Inspect 会直接进入资源；需要 Manager 的错误保护时使用 `manager.Inspect`。

## 跨调用的所有权

同一个任务保存同一个 Manager 成员，并用任务的 owner context 创建：

```go
manager, err := asyncexecmanager.New(ownerCtx, asyncexecmanager.Config{})
```

ownerCtx 应覆盖需要保留异步任务的全部 ReAct 轮次。若每轮创建 Manager，或绑定
单次 action 的短期 context，下一轮就无法继续管理同一批执行。owner 取消会传播到
所有子执行，并禁止新的提交；StopAll 只停止该 Manager 已有执行，仍允许后续提交。
单次 Inspect / Wait 使用各自的请求 context，不改变执行的生命周期。

任务释放时可先 Close 发出取消，再用独立、有超时的 context 调用 Shutdown 等待清理。
不要使用已经取消的 ownerCtx 作为等待清理的 context。Manager 是进程内结构，
不提供重启恢复、全局 session 注册或持久化。

共享浏览器/session 等公共资源的所有者应管理其生命周期。提交到这里的是本次操作
或独立后台工作；某次执行取消不应误关闭其他任务仍在使用的共享资源。

## 管理接口与 action 映射

| 操作 | Manager 方法 | 行为 |
| --- | --- | --- |
| 提交 | Start(Request) | 注册后返回 pending 回执，再由 Manager 启动执行 |
| list | List() | 只返回元信息；不读取输出 |
| 查看状态 | Get(id) | 返回单个执行的元信息 |
| 查看输出 | Inspect(ctx, id) | 调用真实 Inspector，返回元信息和 Observation |
| 获取观察器 | Inspector(id) | 返回该资源注册的观察器 |
| stop | Stop(id) | 幂等地请求取消；终态保持不变 |
| wait | Wait(ctx, id) | 等待 Execute 返回；等待超时不取消执行 |
| stop_all | StopAll() | 请求停止本 Manager 的全部执行 |
| 关闭提交 | Close() | 禁止新任务，请求取消全部执行 |
| 关闭并排空 | Shutdown(ctx) | Close 后等待全部执行退出；超时可再次调用 |

Wait 返回的 error 描述“等待是否成功”，执行本身失败要读取 Summary 的 State 和 Error。
Stop 返回 stopping 只表示已发出取消请求；确认退出需继续 Wait。Inspect 失败时仍会
返回可用的生命周期元信息。元信息和观察结果分别采样，不保证是同一个原子时刻。
Summary.Revision 仅描述生命周期变更，不可用于判断 stdout/result 是否发生变化。

默认最多同时运行 4 个执行，保留 128 条记录。容量耗尽返回 ErrCapacity，不排队。
终态记录仍计入 MaxExecutions，ID 在该 Manager 内不能复用；当前没有自动淘汰或
Forget 接口。较长的 owner 生命周期需要配置适当的记录上限，达到上限时给调用方
明确反馈。Metadata 只保存名字和来源标识，不放输出，也不承担授权职责。

## 工具调用接入位置

本包已实现独立核心和函数适配器。directly_call_tool 已接入异步标记的解析，
ToolCaller 已实现 Inspector；工具提交入口和管理 action 已接入，directly_call_tool 的
单调用异步标记和批量每项 async 均已生效，同一批次可混合同步和异步执行。
ReActLoop 提供 `GetAsyncExecutionManager()`：首次调用必须已有 current task，
Manager 绑定该任务 context；后续调用复用同一实例，不随 current task 切换而重建。
Release 会在其他释放回调前 Close，取消执行并禁止新提交，但不阻塞等待清理；
释放后仍可获取已有 Manager 来 Inspect / Wait，尚未创建 Manager 的已释放 loop
不能再创建。需要等待所有清理完成时，显式使用有超时的独立 context 调用 Shutdown。
工具调用使用 `loop.StartAsyncToolCall(name, params, options...)`，不需要手动创建
caller 或 resource。它通过可选的 AsyncToolCallRuntime 准备工具，再注册到 loop
的 Manager；不支持该扩展的 runtime 会明确返回错误，不改动 AIInvokeRuntime 的
公共接口。工具查找、owner 的守卫/参数修正、原审查与执行链路都会保留。
准备阶段可能等待 MCP 工具就绪；提交成功后返回 pending 回执，不等待工具结果。
完成时结果写入准备时捕获的 owner task 和 timeline；Inspector 与任务结果使用同一
个预先分配的 result ID。后台执行不重新读取 runtime 的 current task。

```go
receipt, err := loop.StartAsyncToolCall(toolName, params,
    aicommon.WithToolCaller_Reason("本次调用目的"))
// 检查 err 后，用 receipt.ID 在 loop 的 Manager 中 Inspect / Wait / Stop。
```

完整链路见 [入口综合测试](../../invoke_toolcall_async_test.go)：从工具名开始调用，
验证参数修正、非阻塞回执、实时输出、owner 隔离、结果归档与退出取消。
基础的 manager 生命周期见 [ReActLoop 接入测试](../async_execution_test.go)。
接入工具时应复用原调用链路，把原工具执行函数和该次调用的 Inspector 接入
NewResource。需要在原调用实例上补齐 Inspector 时，应从原实例读取已有状态，
不能另外订阅 UI/log 事件并创建第二份输出缓存。

[ToolCaller Inspector](../../../aicommon/toolcall_inspector.go) 直接读取原调用的
线程安全有界缓冲。Result 是调用返回时冻结的 ToolResult JSON 信封，参数字段为
null（默认取消路径中 callback 可能仍在修改参数），运行期间为空；PartialResult
当前为空。观察调用不会消费输出，终态后仍可观察。

工具接入使用 `caller.NewAsyncResource(tool, params)`，无需另外实现接口或手写
取消桥接。caller 必须由 NewToolCaller 创建且尚未调用；创建资源成功后，将它的
执行权交给该资源，不能再直接调用或修改 caller。task、emitter、review 等选项应
提前配置好，资源会复用原验证、审查、执行及产物处理链路。
资源在创建时复制 JSON 参数，工具定义应保持只读；每个 caller 只能绑定一次，
每个资源只能执行一次。构造失败不会占用 caller；手动提交失败时调用
`caller.Cancel()` 并丢弃该资源，不要复用为新的调用。loop 的入口会自动处理
提交失败和 Release 时的 caller 取消。

适配器自动开启 `WithToolCaller_WaitForCallbackOnCancel` 的行为，取消时等待 callback
退出后再关闭输出管道及清理产物。可动态转换的同步调用也使用此行为；未绑定
异步或接管适配器的 ToolCaller 保持原来的及时返回取消行为。
manager 的执行 context 会联动取消该 caller，执行返回后也会取消 caller 的 context；
创建 caller 时应使用所属任务的 context，而不是单次 action 的短期 context。例如：

```go
// caller 通过现有工具调用准备流程创建；每次提交使用一个全新的 caller。
manager, err := loop.GetAsyncExecutionManager()
if err != nil {
    return err
}
resource, err := caller.NewAsyncResource(tool, params)
if err != nil {
    return err
}
receipt, err := manager.Start(asyncexecmanager.Request{
    Resource: resource,
    Metadata: asyncexecmanager.Metadata{Name: tool.Name},
})
if err != nil {
    caller.Cancel()
    return err
}
// 后续用 receipt.ID 调用 Inspect / Wait / Stop。
```

`NewAsyncResource` 可选接收一个结果处理函数，用于调用返回后的 owner task/timeline
归档。该函数只在这次外层调用结束后执行一次，不额外缓存观察数据，也不随审批
修改参数的内部递归重复归档。ReAct 的准备入口会安装原有的 finalizeToolCallResult。

Inspector 直接返回原 caller 的观察器，不增加输出或结果缓存。工具返回错误或
不成功的 ToolResult 会产生 failed 执行；direct_answer 产生 cancelled 状态并通知
所属 loop 回答，其他 nil result 报告 failed，不能误当作完成了工具调用。
context 取消保留取消/超时错误。

适配器验证见 [资源接入综合测试](../../../aicommon/toolcall_resource_test.go)：
参数固定、真实观察器绑定、执行成功/失败、重复执行保护、执行前取消和原参数验证。

Inspector 联动例子见 [综合测试](../../../aicommon/toolcall_inspector_test.go)：
正常完成、实时输出、结果快照隔离、取消期间的清理等待和最终输出都在一个测试里。

调用实例要在提交前固定 tool、参数、task、emitter 和 call ID 等信息；后台闭包
不能在运行时重新读取 ReAct 的 current task。原链路的参数检查、授权审查和输出
处理仍应生效；异步只是调度方式，pending 回执不代表工具已执行或已获批准。

action Schema：单工具 `async_directly_call_tool` 可选 bool；批量
`directly_call_tool_params_group` 每项 `async` 可选 bool，默认均为同步。
解析兼容大小写和前后空白的 true/t/1/yes/on 字符串，以及数字 1；其他值（包括
缺省、null、无法识别的类型）回退到 false。批量只读取每项 async，多余的顶层
单调用标记会被忽略。单调用标记保存在该 action 的执行状态中，不进入工具业务参数。
两套协议和 root/next_action 形式由 action_direct_tool_async_test.go 的 mock 测试覆盖。

单调用标记为 true 时，文本/native 两条 action 入口都通过 loop.StartAsyncToolCall
提交到所属 Manager，返回人类可读的 execute_id/tool_name/state 回执并继续主循环。
缺省或 false 仍同步等待结果。文本 verifier 仍可流式读取工具名；handler 在决定
调度方式前等待完整 action（含 AITAG），因此调用卡片要等完整解析后才创建，避免
遗漏晚于参数出现的 async 标记，也不把尚未完整的参数交给后台任务。文本异步路径
复用原有的归一化、AITAG 合并和校验；native 路径使用 verifier 已校验的参数。
资源准备继续执行原 owner 的 guard/mutator，工具审批和执行由 ToolCaller 完成。

回执只表示成功注册，不表示工具已执行或已完成；提交失败立即反馈，不降级为同步。
异步提交不计入已完成工具次数，不触发满意度验证；后台结果仍归档到原 task/timeline，
状态和输出通过 AutoContext / manage_async_execution_runtime 查看。loop Release 会
请求取消自身后台执行，因此结束任务前应先 wait 确认所需执行结束。

验证见 [单调用派发综合测试](../../../aireact/invoke_toolcall_async_action_test.go)：
文本 root/next_action 与 native handler、默认同步、提交立即返回、真实 Inspector
输出与结果、wait/stop、执行失败、guard 拒绝和参数校验失败；不依赖 AI 服务。

批量逐项读取 async：缺省/false 的项沿用同步执行，true 的项用原批量 ToolCaller
构造 Resource，接入同一个 owner Manager。原有 guard、参数 mutator、顺序审批、
批次执行屏障、并发上限、call ID 和 artifact 元信息均保留；全同步路径不创建 Manager。
同步项优先取得批次执行名额，避免长期异步项占满名额而使同步项无法启动。

批次返回条件是所有同步项结束，且所有异步项已提交或提交失败。每个提交成功的
异步项返回 submitted + execute_id，不附加伪造的结果，也不计入已完成工具次数。
反馈按原数组下标列出调用及 identifier，使用可读的文本/Dump 展示 ID；提交后的
最终状态、stdout/stderr 和结果通过 Manager/原 Inspector 获取。批量状态展示也
区分后台已提交与调用已完成，全异步不会误报整批失败。

一项 guard 拒绝或 Manager 容量不足只报告该项失败，其他项继续，不自动降级为同步。
后台项结束时归档到原 task/timeline，不修改已返回的批次回执；异步结果按完成时间
归档，同步结果仍按输入顺序归档。批次 context 在全部后台执行和清理结束后释放，
action 返回不会取消它；owner 取消或 loop Release 会取消后台项。后台审批收到
direct_answer 时仍通过原屏障取消兄弟调用，避免未审批完就开始执行。

验证见 [批量混合派发综合测试](../../invoke_toolcall_batch_async_test.go)：两套 handler
与文本 root/next_action、全同步、同步/异步混用、全异步、并发名额优先级、逐项 ID
与参数、真实 Inspector、部分提交失败、执行失败及 owner 取消后的清理。

`manage_async_execution_runtime` 是核心 action，始终注册，不依赖工具、MCP 或 forge
开关；普通 action 过滤和单轮 must-use/disabled 约束不会隐藏它，宿主 runtime action
policy 仍生效。文本和 native function call 协议共享同一个 verifier/handler。
参数 `op` 支持 `list / inspect / stop / wait / stop_all`，省略时默认为 list；
inspect、stop、wait 必须提供 `execute_id`。OP 和 ID 会去除前后空白，OP 忽略大小写。
`timeout_seconds` 仅用于 wait，默认 10 秒，允许 0–60 秒（支持数字字符串）；0 表示
立即查询。wait 超时返回当前元信息和 `wait_timed_out: true`，不取消执行、不触发重试。
list/stop_all 返回 `executions` 数组，inspect 返回带 Observation 的 `execution`，
stop/wait 返回元信息 `execution`。错误放在反馈的 `error` 字段，继续 loop，未知 ID
不会中止整个任务。stop 只请求取消，需 wait 确认清理结束；需要结果时再显式 inspect。
这些反馈直接来自 Manager/Inspector，不另建输出缓存；每个 action 的验证参数独立
保存，避免同一 native batch 内多个管理调用覆盖彼此。

```json
{"@action":"manage_async_execution_runtime","identifier":"observe_capture","op":"inspect","execute_id":"exec-..."}
```

接入验证见 [管理 action 综合测试](../action_manage_async_execution_runtime_test.go)：
mock 模型通过真实文本/native 解析链路执行全部 OP，验证始终可用、实时观察、等待
超时不取消、正常完成、取消、stop_all 后继续提交以及非法参数保护。
action、AutoContext 和 UI 使用同一个 owner
Manager 的状态与 Inspector，UI 的持续刷新另由展示层实现。

## AI 上下文展示

每轮主循环构造 prompt 时，读取该 loop 已有 Manager 的 List，将只读摘要放入
AutoContext 动态段。没有 Manager 或没有执行记录时不展示，也不会为展示创建 Manager。
摘要通过本次 prompt 的输入显式传递，组装时不根据 runtime 可变的 current task
重新查找 Manager；多个 loop 使用同一配置时，仍只展示各自拥有的执行。

摘要使用人类可读的 YAML 风格 Dump，例如：

```yaml
executions:
  - execute_id: "exec-demo"
    name: "抓取流量"
    state: running
    elapsed_seconds: 12
omitted: 0
```

资源文本使用带引号的字符串，换行、引号及 prompt 分隔符会转义，避免混入新字段。
摘要优先展示尚未结束的执行，再展示按结束时间排序的最近终态记录：最多 8 条，
其中终态最多 4 条，整块最多 4096 字节。每条包含 execute_id、名称、状态、耗时秒数
和简短错误；名称最多 64 字符，错误最多 160 字符，截断时附加省略号。
超过 256 字节的 ID 不在摘要中截断展示，完整 ID 仍可通过 list 获取。
`omitted` 表示未展示的记录数。耗时从开始时间计算，尚未开始时使用创建时间，终态
使用结束时间，避免已结束执行的耗时继续增长。

展示只读取元信息，不调用 Inspector，不复制或缓存 stdout、stderr 或结果；详情由
manage_async_execution_runtime 的 inspect 按需获取。文本/native 和轻量模式都使用
同一动态输入，已有 AutoContext provider 内容会保留；摘要也出现在已有上下文成分
面板的 Auto Context 项中。UI 的实时执行卡片仍待后续接入。

验证见 [展示综合测试](../async_execution_context_test.go)：实际主循环 prompt 构造、
跨轮次状态变化、取消清理、任务隔离、近期终态选择、长度限制、Dump 转义和不读取输出。

## 后续待办

- 子 agent 资源协作：保持默认隔离，由主 agent 显式共享指定资源引用，默认只读，
  控制权限单独授权；AutoContext 只展示自身拥有及被授权共享的资源。本次不实现。

## 验证

```sh
go test -race ./common/ai/aid/aireact/reactloops/asyncexecmanager
go vet ./common/ai/aid/aireact/reactloops/asyncexecmanager
```

测试覆盖实时观察、终态观察、等待超时、取消与清理、父 context 传播、容量、
并发操作，以及函数适配器的参数检查和 context 传递。示例随测试编译并执行。

## ReAct 工具的三种异步入口

三种入口最终都由工具所属 ReActLoop 的 manager 管理，模型使用同一个
`manage_async_execution_runtime` action 的 `list / inspect / wait / stop / stop_all`。
执行 ID 的协议字段是 `execute_id`。提交回执不代表工具完成或业务成功；`wait`
等待执行和清理结束，读取输出和退出结果用 `inspect`。等待超时不取消工具。

1. 模型在 `directly_call_tool` 的根参数中设置 `async_directly_call_tool: true`。
   批量调用对应每项的 `async: true`。业务参数中的同名字段不会改变调度。
2. 工具默认异步：Go 工具用 `aitool.WithDefaultAsync(true)`；Yak AI 工具在脚本
   元数据中声明 `__ASYNC__ = true`。模型省略调度字段时使用工具默认值，显式
   `false` 只覆盖本次初始调度，不禁止工具之后主动 Detach。
3. 已运行的同步工具可以在 interval review 中选择 `decision: "detach"`，或由
   Yak 工具脚本主动调用 `ai.Detach()`。转换保留原来的 ToolCaller、执行栈、
   输出管道和清理逻辑，不重新执行脚本、不重新审批、不复制执行结果。

长驻工具可以在完成准备工作后主动释放前台：

```yak
__DESC__ = "后台服务示例"

// 在真实工具中先绑定监听端口、初始化服务，确认准备成功。
println("service ready")
executionID = ai.Detach()~
println("execution: " + executionID)

// Detach 之后仍处于原脚本，原来的 CTX、defer、stdout 都继续有效。
defer func() {
    println("service cleanup")
}()
for {
    sleep(1)
}
```

`ai.Detach()` 返回 `(string, error)`；`~` 是 Yak 的错误传播写法。并发或重复
调用返回同一个 ID，已经预先异步启动的工具也返回原 ID。绑定仅属于本次脚本
调用，不能通过共享的 `ai` 导出表串到其他脚本。脱离 AI 工具运行环境调用会
返回错误。容量不足、owner 已关闭或调用已完成时，转换失败；容量失败不会
取消、重启原调用或释放其前台等待责任。脚本可自行处理该错误并继续同步运行。

## 接管正在运行的工作

新工作仍使用 `Start`。已经运行的工作使用 `Adopt`，二者不能互换：`Start`
可以在执行前取消，从而完全不调用 `Resource.Execute`；`Adopt` 即使在登记后
立即取消，也必须进入 Execute，将取消传给原工作并等待它和清理真正结束。
`aicommon.ToolCallExecution` 实现了这一接管协议，并对完成与 Detach 的竞争
做唯一归属判定：结果只进入一次 task/Timeline，前台不会把回执计为工具完成。
批量同步子调用转后台后释放前台调用槽位，实际 worker 和取消作用域保留到清理
结束；已返回的批次回执不会被后台改写。

只有仍可转换的前台调用会向 interval review 暴露 Detach 能力。它们即使持续
产生输出也可进入 review，避免长驻服务永远无法转后台；已在后台的调用恢复
原有的“输出变化时跳过辅助模型”策略。工具自行调用 `ai.Detach()` 不依赖
interval review 是否开启，也不依赖辅助模型是否启用。

后台执行成功后仍更新最近使用工具 Schema 缓存。缓存按工具定义维护；异步
执行列表按调用实例维护，不能用最近工具缓存代替执行登记表。后台审批的
`direct_answer` 是 owner loop 的控制信号：停止接收新后台执行、取消相关工具、
唤醒 wait，并由主循环在 action 边界统一回答，不在后台复用旧 action operator。

## 所有权和测试边界

manager 是 ReActLoop 的实例字段，首次创建时绑定所属任务的 context。父 agent
和每个 subagent 各有独立 manager；子 agent 的工具不会自动转移给父 agent。
子 loop 退出会请求取消其工具。`Release/Close` 不等待清理；需要确认结束应在
退出前 `wait`，或由宿主使用有界 `Shutdown`。context 取消仍是协作式取消，不能
强制杀死不配合的 Go 回调。

回归测试覆盖：文本/原生 action、默认策略与显式覆盖、同步运行中转换、真实
Yak 引擎与 defer 清理、重复/并发 Detach、完成竞争、容量拒绝、接管后立即取消、
多于前台槽位的混合调度、后台审批控制信号、真实 subagent 的 manager 隔离。
对应测试位于 `aicommon/toolcall_detach_test.go`、
`aireact/invoke_toolcall_detach_test.go`、`reactloops/async_execution_control_test.go`、
`reactloops/async_subagent_test.go`、本目录 `adopt_test.go`，以及
`common/yak/ai_detach*_test.go`。
