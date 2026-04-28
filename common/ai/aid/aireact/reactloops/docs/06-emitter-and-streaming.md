# 06. Emitter 与流式输出

> 回到 [README](../README.md) | 上一章：[05-hooks-and-lifecycle.md](05-hooks-and-lifecycle.md) | 下一章：[07-liteforge.md](07-liteforge.md)

`Emitter` 是专注模式与外部世界（前端 UI、CLI、日志）通信的**唯一**通道。本章讲清楚：

- `Emitter` 的结构与可组合性
- 主要发送方法分类
- 流式机制（`AIResponse` / `chanx.UnlimitedChan` / 节点 ID）
- reactloops 中实际发出的事件清单
- UX 最佳实践

## 6.1 `Emitter` 结构

源码 [common/ai/aid/aicommon/emitter.go:25-58](../../../aicommon/emitter.go)：

```go
type BaseEmitter func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error)
type EventProcesser func(e *schema.AiOutputEvent) *schema.AiOutputEvent

type Emitter struct {
    streamWG              *sync.WaitGroup
    id                    string                  // CoordinatorId（一般是 ReAct runtime UUID）
    baseEmitter           BaseEmitter             // 真正写到外部
    eventProcesserStack   *utils.Stack[EventProcesser]  // 中间件栈
    interactiveEventSaver func(string, *schema.AiOutputEvent)
    streamNodeIdI18nProvider func(nodeId string) *schema.I18n
}
```

**Emitter 不是 interface 而是 struct**。这意味着：

- 你拿到的是同一个对象的不同包装（通过 `PushEventProcesser` 派生新实例）
- 派生时浅拷贝 + 复制 processer 栈，**不共享栈**
- 每个 emit 经过 processer 栈处理后再调 `baseEmitter`

### `EventProcesser` 栈用法

`EventProcesser` 给事件加 metadata。典型场景：

```go
// 给所有事件追加 ProcessId（关联 AI 流程）
emitter.AssociativeAIProcess(process)

// 给所有事件附 model name（异步获取）
emitter.WithAIInfoProvider(func() AIEventMeta { 
    return AIEventMeta{Service: "...", ModelName: "..."} 
})
```

派生出的 emitter 不会污染原 emitter。

## 6.2 主要发送方法分类

`Emitter` 有 60+ 个 emit 方法，按用途分类如下。

### A. 结构化 JSON 事件（`EmitJSON` 系列）

| 方法 | EventType | 用途 |
|------|-----------|------|
| `EmitJSON(typeName, id, data)` | 自定义 | 通用基础方法 |
| `EmitStructured(nodeId, data)` | `STRUCTURED` | UI 结构化数据节点 |
| `EmitStatus(key, value)` | `STRUCTURED` (id="status") | 状态键值对（如 `loadingStatus`） |
| `EmitYakitRisk(id, title, ...)` | `YAKIT_RISK` | 漏洞风险记录 |
| `EmitYakitHTTPFlow(...)` | `YAKIT_HTTPFLOW` | HTTP 流量记录 |
| `EmitYakitExecResult(exec)` | `YAKIT_EXEC_RESULT` | 执行结果 |
| `EmitFocusOn / LoseFocus` | `FOCUS_ON` / `LOSE_FOCUS` | 子 loop 切换通知 |
| `EmitReActFail / Success` | `FAIL_REACT` / `SUCCESS_REACT` | 整个 ReAct 任务结束 |

### B. ReAct 推理三元组

```go
EmitThought(nodeId, thought)      // 思考
EmitAction(nodeId, action, type, args)  // 行动
EmitObservation(nodeId, obs, source)    // 观察
EmitIteration(nodeId, current, max, msg)  // 迭代信息
EmitResult(nodeId, result, success, ...)  // 结果
```

虽然 `reactloops` 内部不**主动**发这些（结构化语义事件），但语义上对应：

| 三元组 | reactloops 实际事件 |
|--------|---------------------|
| Thought | 流式 `re-act-loop-thought` 节点（来自 LLM reasoning） |
| Action | `EVENT_TYPE_STRUCTURED` (action 信息) + `EmitFileArtifact` |
| Observation | timeline + `Feedback` 注入下一轮 prompt |

### C. 流式事件（核心！）

| 方法 | 典型 NodeId | 用途 |
|------|-------------|------|
| `EmitStreamEvent(nodeId, time, reader, taskIdx, ...)` | 任意 | 流式输出（默认 markdown） |
| `EmitStreamEventEx(...)` | 同上 + `disableMarkdown` 控制 |
| `EmitStreamEventWithContentType(nodeId, reader, taskIdx, contentType, ...)` | 同上 | 指定 ContentType（http_flow / yak_code / text_markdown 等） |
| `EmitTextMarkdownStreamEvent` | `re-act-loop-answer-payload` 等 | 便捷的 markdown 流 |
| `EmitDefaultStreamEvent` | 任意 | 不做 markdown 处理的纯流 |
| `EmitYaklangCodeStreamEvent` | `yaklang_code_editor` | yak 代码流 |
| `EmitHTTPRequestStreamEvent` | `http_flow` | HTTP 请求流 |

`EmitStreamEvent` 内部会把整个 `io.Reader` 拆成 chunk 一边读一边推。reader 关闭时自动 emit `STREAM` 完成事件。

### D. 思考流（thought）

```go
EmitThoughtStream(taskId, fmtTpl, item...)         // 字符串
EmitThoughtStreamReader(taskId, reader, finished)  // 流（内部调 EmitStreamEvent("re-act-loop-thought", ...)）
```

NodeId 固定 `re-act-loop-thought`。reactloops 主循环在 `callAITransaction` 内部把 LLM reasoning 流自动接到这里。

### E. 文件 / 引用资料

| 方法 | EventType |
|------|-----------|
| `EmitFileArtifactWithExt(name, ext, content)` | 落盘 + emit |
| `EmitPinFilename(path)` | `FILESYSTEM_PIN_FILENAME` |
| `EmitPinDirectory(path)` | `FILESYSTEM_PIN_DIRECTORY` |
| `EmitTextReferenceMaterial(eventId, content)` | `REFERENCE_MATERIAL` |
| `EmitTextReferenceMaterialWithFile(...)` | 同上 + 落盘 |

### F. 工具调用相关

```go
EmitToolCallStart / Status / Done / Error / UserCancel
EmitToolCallSummary / Decision / Result / Param / LogDir
```

工具调用走完整生命周期事件。

### G. 任务/流程

```go
EmitJSON(EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC, ...)  // 异步切换
EmitJSON(EVENT_TYPE_FOCUS_ON_LOOP, ...)              // 切到子 loop
EmitJSON(EVENT_TYPE_LOSE_FOCUS_LOOP, ...)            // 离开子 loop
EmitPromptProfile(profileData)                        // prompt 段落 token 占比
```

### H. 资源 / 风险 / 知识

```go
EmitYakitRisk / EmitYakitHTTPFlow / EmitYakitExecResult
EmitJSON(EVENT_TYPE_KNOWLEDGE, ...)
```

### I. 终态结果

```go
EmitResultAfterStream(content)
```

**特别**：这是 **`Emitter` 上没有，但 invoker（AIInvokeRuntime）上有** 的方法。在 `directly_answer` action handler 里典型用法：

```go
invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
invoker.EmitResultAfterStream(payload)
```

`EmitResultAfterStream` 在所有当前的流式事件结束后再发结果，避免乱序。

## 6.3 `AIResponse` 流（reactloops 主循环依赖的底层）

主循环每轮调 `callAITransaction(streamWg, prompt, nonce)`，内部用 `aicommon.AIResponse` 包装 LLM 流式输出。结构：

```go
type AIResponse struct {
    output            *chanx.UnlimitedChan[*OutputChunk]  // 主输出 chan
    reasoning         *chanx.UnlimitedChan[*OutputChunk]  // reasoning 流（可选）
    // ...
}
```

**关键设计**：用 `chanx.UnlimitedChan` 而不是有界 channel，避免 LLM 输出 burst 时阻塞 transport。

`GetOutputStreamReader(nodeId, isReasoning, emitter)` 把 chan 包成 `io.Reader`：

```go
// emitter 在收到流时会自动 EmitStreamEvent(nodeId, ...) 推送
output := response.GetOutputStreamReader("answer", false, emitter)
reasoning := response.GetOutputStreamReader("re-act-loop-thought", true, emitter)
```

源码 [aicommon/response.go:397-440](../../../aicommon/response.go)。

### Fan-out 给字段提取器

reactloops 的 `callAITransaction` 内部还会把同一份 reader **fan-out** 给：

1. **JSON 字段流提取器**：扫描 `streamFields` 配置，遇到 JSON 字段开流时实时调用 callback。
2. **AITag 流提取器**：扫描 `aiTagFields`，遇到 `<|TAG_<nonce>|>` 开始时切到 tag 内容流。
3. **Action 解析器**：完整 JSON 解析，提取 `@action` 等字段。

所以 LLM 输出**只读一遍流**，多个消费者并行处理。

## 6.4 reactloops 中实际发出的事件清单

下面是从代码 grep 出来的事件清单，按主循环阶段排列。

### 任务开始

| 事件 | NodeId / Key | 时机 | 来源 |
|------|--------------|------|------|
| `STRUCTURED` (key=status) | `re-act-loading-status-key` | `r.loadingStatus("初始化...")` | [exec.go:382-397](../exec.go) |
| 自定义 thought 流 | `quick_plan` 等 | InitTask 阶段 LiteForge 流式 | 各 loop init |

### 每轮迭代

| 事件 | NodeId | 时机 | 来源 |
|------|--------|------|------|
| `STRUCTURED` | `react_task_mode_changed` | 切到异步模式 | exec.go 异步路径 |
| `STREAM` (reasoning) | `re-act-loop-thought` | LLM 思考流 | callAITransaction |
| `STREAM` (answer) | 各 action 注册的 NodeId | LLM 输出 stream field | StreamField 配置 |
| `STREAM` (AITag) | 同上 | AITag 内容流 | AITagField 配置 |
| `STRUCTURED` | `prompt_profile` | 每轮 prompt 段落统计 | `emitPromptObservationStatus` |
| `STRUCTURED` | `http_fuzz_request_change` | HTTP fuzz 改包 | loop_http_fuzztest |
| `STREAM` | `yaklang_code_editor` | yak 代码生成 | loop_yaklangcode |
| `STREAM` | `directly_call_tool_params` | 工具参数流 | loopinfra |

### 子 loop 切换

| 事件 | EventType |
|------|-----------|
| `FOCUS_ON_LOOP` | 进入子 loop |
| `LOSE_FOCUS_LOOP` | 离开子 loop |

源码 [action_from_loop.go:51-58](../action_from_loop.go)。

### 工具调用

完整 8 个事件：`tool_call_start` → `tool_call_status` → `tool_call_param` → `tool_call_log_dir` → `tool_call_done` / `tool_call_error` → `tool_call_summary` → `tool_call_result` → `tool_call_decision`。

### 任务结束

| 事件 | EventType |
|------|-----------|
| `RESULT` | `EmitResultAfterStream` |
| `FILESYSTEM_PIN_FILENAME` | `EmitFileArtifact` 落盘 |
| `REFERENCE_MATERIAL` | finalize fallback 时 |
| `STRUCTURED` (status="end") | `loadingStatus("end")` |
| `SUCCESS_REACT` / `FAIL_REACT` | 全局 hook 决定 |

### 中断 / 异常

| 事件 | EventType |
|------|-----------|
| `AI_TASK_SWITCHED_TO_ASYNC` | 异步切换 |
| `REQUIRE_USER_INTERACTIVE` | `ask_for_clarification` |

## 6.5 流式输出的两种模式

### 模式 A：JSON 字段流

LLM 输出标准 JSON，里面某字段开始流时实时推送：

```go
// 注册
reactloops.WithRegisterLoopActionWithStreamField(
    "my_action",
    "my action description",
    options,
    []*reactloops.LoopStreamField{
        {
            FieldName:   "long_explanation",
            AINodeId:    "my-action-node",
            ContentType: aicommon.TypeTextMarkdown,
        },
    },
    verifier, handler,
)
```

LLM 还在写 `"long_explanation": "..."` 时，前端就能看到 `my-action-node` 节点流。

### 模式 B：AITag 流

LLM 输出 JSON 之外的 `<|FINAL_ANSWER_<nonce>|>...<|FINAL_ANSWER_END_<nonce>|>`：

```go
// 注册
reactloops.WithAITagFieldWithAINodeId(
    "FINAL_ANSWER",
    "tag_final_answer",
    "re-act-loop-answer-payload",
    aicommon.TypeTextMarkdown,
)
```

LLM 输出（伪示例）：

```text
{"@action": "directly_answer", "human_readable_thought": "give final answer"}

<|FINAL_ANSWER_aB3x|>
## 分析报告

发现以下问题：
...
<|FINAL_ANSWER_END_aB3x|>
```

提取的 markdown 内容存到 `loop.Get("tag_final_answer")`，并实时流到 `re-act-loop-answer-payload`。

### 选哪个？

| 场景 | 推荐 |
|------|------|
| 字段是简单字符串、json escape 没问题 | StreamField |
| 字段是长 markdown / 代码 / HTTP packet | AITag |
| 一个内容字段两种都给（互斥） | 两个都注册 |
| 多段独立内容 | 多个 AITagField |

详细对比见 [04-actions.md](04-actions.md)。

## 6.6 NodeId 命名约定

NodeId 决定前端把流挂在哪个 UI 元素上。reactloops 用了一些固定的 NodeId：

| NodeId | 含义 |
|--------|------|
| `re-act-loop-thought` | 思考流（reasoning） |
| `re-act-loop-answer-payload` | 最终答案 markdown 流 |
| `re-act-loading-status-key` | 加载状态 key |
| `directly_call_tool_params` | 工具参数流 |
| `prompt_profile` | prompt 段落开销 |
| `http_flow` | HTTP 请求/响应 |
| `yaklang_code_editor` | yak 代码 |
| `quick_plan` | 快速规划思路 |
| `reference_material` | 引用资料 |

ContentType 决定渲染方式：

| ContentType | 含义 |
|-------------|------|
| `text_markdown` | Markdown 渲染 |
| `text_plain` | 纯文本 |
| `code_yaklang` | yak 代码高亮 |
| `code_http_request` | HTTP 包格式化 |
| `default` | 默认 |

## 6.7 UX 最佳实践

### 1. 总是把"思考"流出来

LLM 思考阶段沉默几秒钟用户会觉得卡住。reactloops 自动把 reasoning 流到 `re-act-loop-thought`，但你**也可以**主动 EmitThoughtStream：

```go
emitter.EmitThoughtStream(task.GetIndex(), "Initialized fuzz request from extracted HTTP packet.")
```

[loop_http_fuzztest/init.go:144](../loop_http_fuzztest/init.go) 在 init 阶段先 emit 一句"我已经从输入提取到 packet"，让用户立刻有反馈。

### 2. 关键 action 后立刻 EmitFileArtifact

不要等到结束才一次性 emit。比如代码生成 action：

```go
// 在 ActionHandler 里
yakCode := generate(...)
invoker.EmitFileArtifactWithExt("generated_code", ".yak", yakCode)
loop.Set("last_generated_code", yakCode)
operator.Continue()
```

### 3. 长输出用 AITag + StreamField 两路

`directly_answer` action 同时注册 `answer_payload`（StreamField）和 `FINAL_ANSWER`（AITag）。LLM 自由选择短答案或长答案。前端不需要关心，因为 NodeId 一致，事件流落到同一个 UI 元素。

### 4. 不要在静默路径里偷偷消耗模型

很多 hook（`OnPostIteraction` 之类）调 LiteForge **没有流式输出**，用户不知道在跑什么。所以：

- 调 LiteForge 时**带 NodeId 流回调**，让用户看到生成过程
- 或者 `EmitThoughtStream` 一句话告诉用户"系统正在生成总结"

参考 [loop_http_flow_analyze/finalize.go:174-191](../loop_http_flow_analyze/finalize.go)：

```go
aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{"summary"}, func(key string, r io.Reader, emitter *aicommon.Emitter) {
    if event, _ := emitter.EmitStreamEventWithContentType(
        "re-act-loop-answer-payload",
        utils.JSONStringReader(r),
        taskID,
        aicommon.TypeTextMarkdown,
        func() {},
    ); event != nil {
        streamId := event.GetStreamEventWriterId()
        emitter.EmitTextReferenceMaterial(streamId, contextMaterials)
    }
}),
```

不仅流出，还把 context materials 作为 reference material 关联——用户可以查看"这个总结基于哪些原始数据"。

### 5. `loadingStatus` 的语义

主循环每个阶段都会调 `r.loadingStatus(...)`：

- `"初始化 / initializing..."`
- `"执行初始化函数 / execute init handler..."`
- `"记忆快速装载中 / waiting for fast memories to load..."`
- `"执行中... / executing..."`
- `"[action_name]执行中 / executing action..."`
- `"end"`（除非 `noEndLoadingStatus=true`）

UI 一般在底部状态栏显示。`WithNoEndLoadingStatus(true)` 让子 loop 不发 `"end"`，避免错误结束信号。

### 6. `EmitPinFilename` 给可下载的产物

落盘后调 `EmitPinFilename(path)`，前端会显示一个"打开文件"链接。`EmitFileArtifactWithExt` 内部已经做了。

## 6.8 实战：自定义流式 action

```go
reactloops.WithRegisterLoopActionWithStreamField(
    "generate_report",
    "生成 markdown 报告",
    []aitool.ToolOption{
        aitool.WithStringParam("report_content", aitool.WithParam_Required(true)),
    },
    []*reactloops.LoopStreamField{
        {
            FieldName:   "report_content",
            AINodeId:    "report-stream",
            ContentType: aicommon.TypeTextMarkdown,
        },
    },
    verifier,
    func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
        report := action.GetString("report_content")
        invoker := loop.GetInvoker()
        invoker.EmitFileArtifactWithExt("report", ".md", report)
        invoker.EmitResultAfterStream(report)
        op.Exit()
    },
)
```

LLM 输出 `report_content` 字段时，前端 `report-stream` 节点就开始接收。结束后落盘 + `EmitResultAfterStream` 发最终结果事件。

## 6.9 进一步阅读

- **[14-streaming-ux.md](14-streaming-ux.md)**：流式输出与 UX 实战 —— 本章是手册，14 章是「专注模式作者写流式怎么做对」的实战指南（yak / Go 双侧、ContentType / NodeId 命名规范、终局三连、八条踩坑）
- [03-prompt-system.md](03-prompt-system.md)：prompt 模板里的 nonce / TAG 防注入
- [04-actions.md](04-actions.md)：StreamField vs AITagField 的取舍
- [12-debugging-and-observability.md](12-debugging-and-observability.md)：`EmitPromptProfile` 调试
- [13-yak-focus-mode.md](13-yak-focus-mode.md)：用 yak 脚本写专注模式（包含 `__AI_TAG_FIELDS__` / `stream_fields` 在 yak 侧的精确字段名）
