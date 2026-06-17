# 18. 专注模式输出行为改造规范

> 目标：把已有专注模式中无节制的“思考”、调试信息和大段中间结果，改造成有限、结构化、可追踪的用户可见输出。

本规范以 `common/ai/aid/aireact/reactloops/stream_helpers.go` 为主要落点。改造时优先使用 `EmitStatus`、`EmitActionLog`、`EmitProgress`、`SaveAndPinFile`，减少直接操作 emitter 的随意输出。

## 输出分层

专注模式里的每段信息先按用途归类，再决定放哪里。

| 信息类型 | 应该去哪里 | 不应该去哪里 |
|---|---|---|
| 当前阶段 | `EmitStatus` | Action 日志、最终答案 |
| Action 开始/完成摘要 | `EmitActionLog` | thought stream、逐条 debug stream |
| 已知总量进度 | `EmitProgress` | 每条数据一条 stream |
| 未知总量进度 | 节流后的 `EmitStatus` | `EmitProgress(total=0)` |
| 大体量结果 | `SaveAndPinFile` + 简短 preview | 直接 `EmitDefaultStreamEvent` |
| 最终答案 | `StreamField` / `AITagField` / `EmitResultAfterStream` | Action 日志 |
| 模型下一轮观察 | `operator.Feedback`，且必须限长 | 前端 stream |
| 工程调试信息 | `log.Infof/Warnf/Errorf` | 用户可见 stream |

## 不良输出行为

### 1. 把 `human_readable_thought` 暴露给用户

坏例子：

```go
thought := action.GetString("human_readable_thought")
line1 := fmt.Sprintf("匹配 source=%s, %s | %s", sourceQuery, matcherDesc, thought)
reactloops.EmitActionLog(loop, nodeId, line1)
```

问题：

- `human_readable_thought` 是模型给 ReAct 过程的思路字段，不是稳定的用户界面文案。
- 内容可能冗长、重复、带调试味或泄漏 prompt 决策过程。
- 框架已经有 reasoning/thought 通道，不需要业务 Action 再拼一次。

好例子：

```go
line1 := fmt.Sprintf("匹配 source=%s, %s", sourceQuery, matcherDesc)
reactloops.EmitActionLog(loop, nodeId, line1)
```

### 2. 循环内逐条输出

坏例子：

```go
for _, flow := range flows {
    reactloops.EmitActionLog(loop, nodeId, fmt.Sprintf("checking flow #%d", flow.ID))
}
```

好例子：

```go
for flow := range flowStream {
    totalCount++
    if knownTotal > 0 && totalCount%100 == 0 {
        reactloops.EmitProgress(loop, totalCount, knownTotal, "匹配进度", "Matching")
    } else if knownTotal == 0 && totalCount%100 == 0 {
        reactloops.EmitStatus(loop, fmt.Sprintf(
            "已扫描 %d 条流量 / Scanned %d Flows",
            totalCount, totalCount,
        ))
    }
}
```

### 3. 把大段结果直接推到前端

坏例子：

```go
reactloops.EmitActionLog(loop, "http-flow-match", fullSummary)
```

好例子：

```go
fullSummary := builder.String()
summary := fullSummary
if len(fullSummary) > maxHTTPFlowSummaryBytes {
    preview := utils.ShrinkTextBlock(fullSummary, 2000)
    summary = fmt.Sprintf(
        "结果过长，已保存到文件。\n\n预览:\n%s\n\n文件: %s",
        preview, filename,
    )
}
_ = reactloops.SaveAndPinFile(loop, filename, []byte(fullSummary))
reactloops.EmitActionLog(loop, "http-flow-match", finishLine, summary)
```

### 4. 直接使用 emitter 发送普通 Action 日志

坏例子：

```go
emitter := loop.GetEmitter()
emitter.EmitDefaultStreamEvent("thought", strings.NewReader(debugText), taskID)
```

好例子：

```go
reactloops.EmitStatus(loop, "搜索规则样例 / Searching Rule Examples...")
reactloops.EmitActionLog(loop, "syntaxflow-rule-search", "搜索规则样例: include/golang")
```

只有需要特殊 `ContentType`、最终答案流、AITag/StreamField 回调或 reference material 时，才保留直接 emitter 调用。

### 5. 把错误堆栈和 fallback 细节展示给用户

坏例子：

```go
reactloops.EmitActionLog(loop, nodeId, fmt.Sprintf("query failed: %+v", err))
```

好例子：

```go
log.Errorf("[query_http_flows] query failed: %v", err)
reactloops.EmitStatus(loop, "查询失败 / Query Failed")
operator.Fail(fmt.Sprintf("query http flows failed: %v", err))
```

用户需要知道“失败了”和“下一步如何处理”，不需要看到内部堆栈或调试上下文。

## 标准改造流程

### 第一步：定位所有用户可见输出

在目标 loop 下搜索：

```text
EmitDefaultStreamEvent
EmitTextMarkdownStreamEvent
EmitStreamEventWithContentType
EmitThoughtStream
LoadingStatus
EmitStatus
EmitActionLog
EmitPinFilename
human_readable_thought
operator.Feedback
AddToTimeline
```

逐个判断它属于“用户可见状态、Action 摘要、最终答案、文件、调试日志、模型观察”中的哪一类。

### 第二步：替换普通过程输出

普通 Action 过程统一改成：

```go
nodeId := "business-action"
startLine := "执行动作: 参数摘要"
reactloops.EmitActionLog(loop, nodeId, startLine)
reactloops.EmitStatus(loop, "执行中 / Running...")

// business logic

reactloops.EmitStatus(loop, "完成 / Complete")
reactloops.EmitActionLog(loop, nodeId, finishLine, referenceSummary)
```

要求：

- 开始日志只说明“做什么”和关键参数。
- 完成日志只说明“结果数量、命中比例、文件位置”等事实。
- 不拼接模型思考。
- 不在业务循环里不断追加日志。

### 第三步：文件化大内容

满足任一条件就文件化：

- 内容可能超过几 KB。
- 内容包含 HTTP request/response、flow 列表、fuzz payload、扫描明细。
- 内容主要用于复查，不需要作为界面主叙事。
- 内容可能重复出现，容易淹没最终答案。

文件化后前端只保留：

- 总数、命中数、失败数。
- 文件路径。
- 最多一段压缩 preview。
- 必要时作为 `EmitActionLog` 的 reference material。

### 第四步：保留最终答案的专用通道

不要用 `EmitActionLog` 交付最终答案。最终答案继续走：

- action `StreamField`，适合 JSON 字段里的短到中等 Markdown。
- loop `AITagField`，适合长 Markdown、代码、HTTP 包。
- `invoker.EmitResultAfterStream`，用于流结束后落定最终结果。
- `EmitTextReferenceMaterial`，用于把最终答案和上下文材料关联。

### 第五步：收紧 `operator.Feedback`

`operator.Feedback` 会进入下一轮 ReAct 上下文，不能当作无限制日志缓存。

要求：

- Feedback 放机器可用摘要，不放完整大文件内容。
- 大内容先 `SaveAndPinFile`，Feedback 只写文件路径和 preview。
- 错误反馈写可恢复信息，不写堆栈。

## `loop_http_flow_analyze` 的当前改造方向

这个 loop 已经有良好雏形：

- `query_http_flows`、`match_flows`、`match_flows_simple` 使用 `EmitActionLog` 输出开始/完成两段。
- `get_http_flow_detail` 使用 `EmitActionLog` 加 reference 展示详情。
- `dispatch_fuzz_test` 使用 `EmitStatus` 展示阶段切换，并把结果合并到 evidence。
- `finalize` 用专门的 markdown stream 和 `EmitResultAfterStream` 交付最终答案。

后续继续收敛时优先处理：

- 移除 Action 日志中拼接 `human_readable_thought` 的代码。
- 将 `EmitProgress(loop, totalCount, 0, ...)` 改为未知总量场景的节流 `EmitStatus`。
- `init_task.go` 中直接 `os.WriteFile` + `EmitPinFilename` 的路径可替换为 `SaveAndPinFile`，保持文件 pin 逻辑一致。
- `findings.go` 中直接 `EmitTextMarkdownStreamEvent` 仅在确实需要独立 Markdown 证据流时保留；普通 evidence 更新应优先走状态、Action 日志或最终报告引用。

## 验收标准

一次输出行为改造完成后，必须满足：

- 前端不会出现无节制“思考”或调试碎片。
- 每个 Action 最多保留开始和完成两类用户可见日志。
- 长结果都有文件化路径，界面只显示摘要。
- 状态栏文案是短句、中英双语、可覆盖。
- 技术日志只在 `log.*` 中出现。
- 最终答案仍通过最终答案通道流式展示，并调用 `EmitResultAfterStream` 落定。
- `operator.Feedback` 和 timeline 没有塞入不必要的大段原文。

## Code Review 提问清单

Review 专注模式输出相关 PR 时，直接问：

- 这条输出是给用户看的，还是给开发者排查的？
- 这条信息是否会在循环中按数据量增长？
- 如果返回 5000 条 flow，前端会不会被刷屏？
- `human_readable_thought` 有没有被拼进用户可见文案？
- 大内容有没有 `SaveAndPinFile` 和 preview？
- 最终答案有没有走 `StreamField` / `AITagField` / `EmitResultAfterStream`？
- 直接 emitter 调用是否真的需要特殊 stream 能力？
