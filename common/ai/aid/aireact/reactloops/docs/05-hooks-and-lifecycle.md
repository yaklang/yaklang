# 05. Hook 与生命周期

> 回到 [README](../README.md) | 上一章：[04-actions.md](04-actions.md) | 下一章：[06-emitter-and-streaming.md](06-emitter-and-streaming.md)

Hook 是专注模式给"边界事件"留的扩展点。本章覆盖：

- `WithInitTask`：主循环开始前的初始化
- `WithOnPostIteraction`：每轮 + 整循环结束的回调（注意拼写）
- `WithOnLoopInstanceCreated`：拿到 loop 实例引用
- `WithOnTaskCreated` / `WithOnAsyncTaskTrigger` / `WithOnAsyncTaskFinished`：任务级回调
- 典型 Hook 组合模式

## 5.1 生命周期总览

```mermaid
sequenceDiagram
    autonumber
    participant Caller as 调用方
    participant Factory as 工厂(LoopFactory)
    participant Reg as register.go
    participant Loop as ReActLoop 实例
    participant Init as InitHandler
    participant LOOP as 主循环
    participant Post as OnPostIteraction
    participant Async as 异步监控

    Caller->>Reg: CreateLoopByName(name, runtime, opts...)
    Reg->>Factory: factory(runtime, opts...)
    Factory-->>Reg: *ReActLoop
    Reg->>Loop: 触发 onLoopInstanceCreated(loop)
    Loop-->>Caller: 返回实例
    Caller->>Loop: Execute(taskId, ctx, userInput)
    Loop->>Loop: NewStatefulTaskBase
    Loop->>Loop: 触发 onTaskCreated(task)
    Loop->>Init: initHandler(loop, task, op)
    alt op.IsDone
        Init-->>Loop: 早退（return nil）
    else op.IsFailed
        Init-->>Loop: 报错退出 + DirectlyAnswer
    else 默认 Continue
        Loop->>LOOP: 进入主循环
        loop 每轮迭代
            LOOP->>LOOP: 执行 action handler
            LOOP->>Post: 每轮回调（isDone=false）
            alt action 切到异步
                LOOP->>Async: onAsyncTaskTrigger(action, task)
                Async-->>LOOP: 返回（任务异步运行）
                Async->>Post: 异步完成回调链
                Async->>Loop: onAsyncTaskFinished(task)
            end
        end
        LOOP->>Post: 整循环结束（isDone=true）
        Post-->>Loop: ShouldEndIteration / IgnoreError
    end
    Loop-->>Caller: 返回 error
```

## 5.2 `WithInitTask`：初始化阶段

### 签名

```go
func WithInitTask(handler func(loop *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator)) ReActLoopOption
```

`op` 提供：

```go
op.Done()                       // 早退
op.Failed(err)                  // 失败
op.Continue()                   // 默认行为
op.NextAction("a", "b")         // 首轮强制只能选 a/b
op.RemoveNextAction("c")        // 首轮禁用 c
```

### 用途

| 场景 | 推荐做法 |
|------|----------|
| 从用户输入中抽取参数（结构化） | LiteForge 调一次 |
| 引导外部资源（如 fuzz 上下文、数据库连接） | 直接 setup |
| 用户输入实在不够，需要先问用户 | `op.NextAction("ask_for_clarification")` |
| 输入直接就是答案（不需要循环） | 处理后 `op.Done()` |
| 严重错误（如必需库不可用） | `op.Failed(err)`，会触发 `DirectlyAnswer` 给出修复建议 |

### 示例：HTTP Fuzz 的 InitTask

源码 [loop_http_fuzztest/init.go:128-184](../loop_http_fuzztest/init.go) 简化后：

```go
reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
    invoker := loop.GetInvoker()
    
    bootstrapLoopHTTPFuzzFuzztagContext(loop, config.GetDB())
    
    haveReq := loop.Get("fuzz_request")
    if haveReq == "" {
        bootstrapResult := tryBootstrapFuzzRequestFromUserInput(r, loop, task)
        switch bootstrapResult {
        case "raw":
            loop.Set("bootstrap_source", "user_input_raw")
        case "url":
            loop.Set("bootstrap_source", "user_input_url")
        default:
            if restoreLoopHTTPFuzzSessionContext(loop, r) {
                // 从历史会话恢复
            } else {
                emitter.EmitThoughtStream(task.GetIndex(), 
                    "No HTTP packet/URL found, please provide one.")
                op.Done() // 早退！避免无意义的主循环
                return
            }
        }
    }
    
    action, err := invoker.InvokeSpeedPriorityLiteForge(
        task.GetContext(),
        "http_fuzztest_init_booststrap",
        bootstrapPrompt,
        []aitool.ToolOption{
            aitool.WithStringParam("thought", aitool.WithParam_Description("...")),
        },
        aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "quick_plan"),
    )
    if err == nil {
        invoker.AddToTimeline("http_fuzztest_init_booststrap", "Bootstrap insights: "+action.GetString("thought"))
    }
})
```

四个关键决策：

1. **环境引导**（`bootstrapLoopHTTPFuzzFuzztagContext`）：把 fuzztag 文档加载到 vars，主循环里 reactiveData 就能注入。
2. **早退路径**：用户输入连 URL 都没有 → `op.Done()` 不进主循环。
3. **会话恢复**：如果之前测试过，恢复上下文，不需要重新让用户提供。
4. **LiteForge 抽思路**：跑 LLM 一次得到 fuzz 思路，写进 timeline 给主循环参考。

### 示例：约束首轮 action

```go
reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
    if loop.Get("fuzz_request") == "" {
        op.NextAction("set_http_request") // 首轮必须先设置目标
    }
})
```

主循环首轮 schema 只暴露 `set_http_request` 这一个 action（`generateSchemaString` 里 `mustUseFiltered` 生效），LLM 只能选它。**仅首轮生效**：使用一次后 `initActionApplied=true`，后续轮次回到默认。

## 5.3 `WithOnPostIteraction`：每轮 + 整循环结束

### 签名

```go
func WithOnPostIteraction(fn ...func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *OnPostIterationOperator)) ReActLoopOption
```

参数解读：

| 参数 | 含义 |
|------|------|
| `iteration` | 当前迭代序号（从 1 开始） |
| `task` | 当前任务 |
| `isDone` | `false`=每轮结束；`true`=整循环结束 |
| `reason` | 仅 `isDone=true` 时有值。可能是 `error`（max iter / fail）或者 `string`（exit 原因） |
| `op` | 控制 operator |

### `op` 方法

```go
op.EndIteration(reason ...any)  // 强制结束
op.IgnoreError()                // 静默退出（即便 max iter 也不报错）
op.DeferAfterCallbacks(fn)      // 等所有回调跑完再执行
op.ShouldEndIteration() bool
op.ShouldIgnoreError() bool
```

### 关键约定

1. **`onPostIteration` 是 `[]func` 列表**：可以注册多个 hook，按注册顺序执行。
2. **每轮 `isDone=false`，整循环结束 `isDone=true`**：你必须**先判 `isDone`** 决定行为分支。
3. **整循环结束时，全局 hook（如 `EmitReActFail/Success`）也在列表里**：用 `DeferAfterCallbacks` 等所有 loop-specific hook 跑完再决定。

### 经典模板：finalize fallback

如果 LLM 在循环里没主动给 `directly_answer`（比如 max iter 触发或者一直在做工具），用 `OnPostIteraction` 强制生成总结。

源码 [loop_http_fuzztest/finalize.go:13-28](../loop_http_fuzztest/finalize.go)：

```go
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
    return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
        if !isDone {
            return  // 只关心整循环结束
        }
        persistLoopHTTPFuzzSessionContext(loop, "post_iteration")
        if hasLoopHTTPFuzzFinalAnswerDelivered(loop) || 
           hasLoopHTTPFuzzDirectlyAnswered(loop) || 
           getLoopHTTPFuzzLastAction(loop) == "directly_answer" {
            return  // 已经给过答案，不重复
        }
        finalContent := generateLoopHTTPFuzzFinalizeSummary(loop, reason)
        deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, finalContent)
        if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
            op.IgnoreError() // max iter 不算错误
        }
    })
}
```

四个步骤模式：

1. **过滤 isDone=false 的调用**：`if !isDone { return }`。
2. **持久化必要状态**：`persistLoopHTTPFuzzSessionContext`。
3. **判断是否已经给过答案**：避免重复 emit。
4. **生成 + emit + 标记 + IgnoreError**。

### 增强模式：用 LiteForge 生成 finalize

[loop_http_flow_analyze/finalize.go:13-37](../loop_http_flow_analyze/finalize.go) 是更强版本——不只是拼接 markdown，而是调用 LLM **再生成**一次结构化总结：

```go
contextMaterials := collectFinalizeContextMaterials(loop, reason)
deliverFinalAnswerFallback(loop, invoker, contextMaterials)
```

`deliverFinalAnswerFallback` 内部调 `InvokeSpeedPriorityLiteForge`，给 LLM 喂"context materials"，要求生成最终报告。带流式回调，前端实时显示。

源码 [loop_http_flow_analyze/finalize.go:114-215](../loop_http_flow_analyze/finalize.go)。

### 每轮统计模式（isDone=false）

如果 hook 想在每轮做增量收集（不等到结束），就利用 `isDone=false`：

```go
WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
    if isDone {
        // 整循环结束的逻辑
        return
    }
    // 每轮：收集 finding、统计、累积
    collectIterationFindings(loop)
})
```

[loop_http_flow_analyze/finalize.go:16-23](../loop_http_flow_analyze/finalize.go) 就是双分支：每轮收集 findings，结束时强制 finalize。

### 多个 hook 的执行顺序

```go
preset := []ReActLoopOption{
    WithOnPostIteraction(hookA),  // 先注册先跑
    WithOnPostIteraction(hookB),
    WithOnPostIteraction(hookC),
}
```

跑完 A、B、C 后，调用 `op.RunDeferredFuncs()` 跑所有 `DeferAfterCallbacks` 注册的延迟函数。

**全局 hook 几乎一定要用 DeferAfterCallbacks**：因为它们想看的是"所有 loop-specific hook 都跑完后的最终状态"，否则可能在 IgnoreError 设置之前就读到 `false`。

## 5.4 `WithOnLoopInstanceCreated`：拿引用

### 签名

```go
func WithOnLoopInstanceCreated(fn func(loop *ReActLoop)) ReActLoopOption
```

调用时机：`CreateLoopByName` 里 `factory(runtime, opts...)` 拿到 `*ReActLoop` 后**立即**调用，**早于** `Execute`。

源码 [register.go:110-122](../register.go)：

```go
func CreateLoopByName(name string, invoker aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
    factoryCreator, ok := loops.Get(name)
    if !ok {
        return nil, utils.Errorf("reactloop[%v] not found", name)
    }
    loopIns, err := factoryCreator(invoker, opts...)
    if err != nil {
        return nil, utils.Wrap(err, "failed to create loop instance")
    }
    if loopIns.onLoopInstanceCreated != nil {
        loopIns.onLoopInstanceCreated(loopIns)
    }
    return loopIns, nil
}
```

### 用途

**最典型用法**：在父 loop 调子 loop 之前，捕获子 loop 引用，等子 loop 跑完后读取它内部状态。

[deep_intent.go:45-49](../deep_intent.go) 实战：

```go
var intentLoop *reactloops.ReActLoop
opts = append(opts, 
    reactloops.WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
        intentLoop = l
    }),
    reactloops.WithNoEndLoadingStatus(true),
    reactloops.WithUseSpeedPriorityAICallback(true),
)

err := reactloops.RunReactLoop(ctx, "loop_intent", invoker, userInput, opts...)
if err == nil && intentLoop != nil {
    intent := intentLoop.Get("intent_analysis")
    // 用子 loop 的状态
}
```

## 5.5 任务级 Hook

### `WithOnTaskCreated(fn)`

```go
func WithOnTaskCreated(fn func(task aicommon.AIStatefulTask)) ReActLoopOption
```

每次 `Execute` 创建新 task 后调用。常用于把 task 注册到外部状态机、记录开始时间。

### `WithOnAsyncTaskTrigger(fn)`

```go
func WithOnAsyncTaskTrigger(fn func(action *LoopAction, task aicommon.AIStatefulTask)) ReActLoopOption
```

调用时机：handler 选了一个 `AsyncMode=true` 的 action，或者 handler 内部调了 `op.RequestAsyncMode()`。

参数：触发异步的那个 action 实例 + task。

### `WithOnAsyncTaskFinished(fn)`

```go
func WithOnAsyncTaskFinished(fn func(task aicommon.AIStatefulTask)) ReActLoopOption
```

异步任务完成时调用。**不是** `OnPostIteraction(isDone=true)`——异步任务可能不通过主循环结束。

## 5.6 典型 Hook 组合模式

### 模式 A：纯执行（极简）

```go
preset := []ReActLoopOption{
    WithMaxIterations(20),
    WithAllowToolCall(true),
    WithPersistentInstruction(instruction),
}
```

没有 hook，靠默认行为。适合只是工具编排的场景。

### 模式 B：标准 finalize（推荐）

```go
preset := []ReActLoopOption{
    WithMaxIterations(maxIter),
    WithAllowToolCall(true),
    WithPersistentInstruction(instruction),
    WithReactiveDataBuilder(myReactiveDataBuilder),
    WithReflectionOutputExample(outputExample),
    WithInitTask(myInitHandler),
    BuildOnPostIterationHook(invoker), // 整循环结束时 finalize
}
```

`Init` 引导 + 主循环 + `Post` 收尾。绝大部分专注模式都是这个套路。

### 模式 C：双 hook（增量 + 终态）

```go
preset := []ReActLoopOption{
    WithOnPostIteraction(func(loop, it, task, isDone, reason, op) {
        if !isDone {
            collectIterationFindings(loop) // 每轮增量
            return
        }
        // 终态
        finalize(loop, invoker, reason, op)
    }),
}
```

适合需要持续累积数据的场景（如代码审计、流量分析）。

### 模式 D：父子 loop 协作

```go
var subLoop *reactloops.ReActLoop
opts := []ReActLoopOption{
    WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
        subLoop = l
    }),
    WithNoEndLoadingStatus(true),
    WithUseSpeedPriorityAICallback(true),
}

err := reactloops.RunReactLoop(ctx, "loop_intent", invoker, userInput, opts...)
intent := subLoop.Get("intent_analysis")
```

参考 [deep_intent.go](../deep_intent.go)。子 loop 跑完后**父逻辑**继续读 vars。

### 模式 E：异步任务追踪

```go
preset := []ReActLoopOption{
    WithOnAsyncTaskTrigger(func(action *LoopAction, task aicommon.AIStatefulTask) {
        log.Infof("loop async start: action=%s task=%s", action.ActionType, task.GetId())
        registerAsyncJob(task)
    }),
    WithOnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
        log.Infof("loop async done: task=%s", task.GetId())
        completeAsyncJob(task)
    }),
}
```

UI 用这两个 hook 显示异步任务运行状态。

## 5.7 小结：什么时候用什么 Hook

| 想做的事 | Hook |
|----------|------|
| 用户输入抽参 | `WithInitTask` + LiteForge |
| 首轮强制某 action | `WithInitTask` + `op.NextAction(...)` |
| 用户输入不够直接退出 | `WithInitTask` + `op.Done()` |
| 主循环结束补 markdown | `WithOnPostIteraction(isDone=true)` |
| 每轮收集数据 | `WithOnPostIteraction(isDone=false)` |
| max iter 不报错 | `op.IgnoreError()` |
| 拿子 loop 的状态 | `WithOnLoopInstanceCreated` |
| 异步任务监控 | `WithOnAsyncTaskTrigger/Finished` |

## 5.8 进一步阅读

- [02-options-reference.md](02-options-reference.md)：每个 Hook option 的签名细节
- [04-actions.md](04-actions.md)：Hook 与 Action 的协作
- [11-case-studies.md](11-case-studies.md)：每个 loop_xxx 用了哪些 hook
