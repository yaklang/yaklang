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

取消可能发生在 Execute 开始之前，因此创建 resource 时应只准备描述和观察器。
把需要释放的句柄、worker 等放在 Execute 内创建并清理。如果是接管已有任务，
要另外明确交接失败或执行未开始时的所有权，不能假设 Execute 一定会被调用。

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

本包已实现独立核心和函数适配器，尚未接入旧的工具调用链路或 actions。
接入工具时应复用原调用链路，把原工具执行函数和该次调用的 Inspector 接入
NewResource。需要在原调用实例上补齐 Inspector 时，应从原实例读取已有状态，
不能另外订阅 UI/log 事件并创建第二份输出缓存。

当前 [ExecuteToolWithCapture](../../../aitool/tool_result.go) 在 context 取消时可能
先返回，而内部 callback 还在退出。因此不能仅把这个方法的返回当作真实结束。
后续工具接入必须在原调用实例上暴露 callback 的完成信号，并由适配器等待清理；
已有输出缓冲的读取也要在原调用链路提供并发安全快照，不能直接读取运行中的
`bytes.Buffer`。这些是工具链路需要补齐的能力，本包的函数适配器不会自动生成。

调用实例要在提交前固定 tool、参数、task、emitter 和 call ID 等信息；后台闭包
不能在运行时重新读取 ReAct 的 current task。原链路的参数检查、授权审查和输出
处理仍应生效；异步只是调度方式，pending 回执不代表工具已执行或已获批准。

预期 action 参数保持：单工具 `async_directly_call_tool` 可选 bool；批量
`directly_call_tool_params_group` 每项 `async` 可选 bool，默认均为同步。
`manage_async_execution_runtime` 将各 OP 映射到上表的方法；OP=wait 应有等待时限，
需要结果时在等待后显式调用 Inspect。action、AutoContext 和 UI 使用同一个 owner
Manager 的状态与 Inspector，UI 的持续刷新另由展示层实现。

## 验证

```sh
go test -race ./common/ai/aid/aireact/reactloops/asyncexecmanager
go vet ./common/ai/aid/aireact/reactloops/asyncexecmanager
```

测试覆盖实时观察、终态观察、等待超时、取消与清理、父 context 传播、容量、
并发操作，以及函数适配器的参数检查和 context 传递。示例随测试编译并执行。
