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
| 错误恢复指导 | `AddToTimeline` / 限长 `operator.Feedback` | 用户可见 Action 日志 |
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

`loop_http_flow_analyze` 的 `match_flows` / `match_flows_simple` 已采用此写法。

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

注意：`EmitProgress` 要求 `total > 0`；传 `0` 时函数直接返回，不会产生任何输出。`loop_http_flow_analyze` 的匹配 Action 目前仍有此问题，见下文「仍待收敛」第 1 项。

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

### 6. 误删 timeline 中的错误恢复指导

坏例子：

```go
// 原来 timeline 有“可能原因 / 立即行动 / 下一步”，迁移时被压成一行
invoker.AddToTimeline("read_file_not_found", fmt.Sprintf("file not found: %s", filePath))
```

问题：

- timeline 是下一轮 ReAct 的重要上下文，不等同于用户可见 stream。
- 删除结构化恢复指导后，模型只知道“失败了”，不知道下一步该 `list_files`、改绝对路径，还是重新反编译。
- 输出收敛的目标是减少前端刷屏，不是把所有可恢复信息删掉。

好例子：

```go
log.Warnf("[read_java_file] file not found: %s", filePath)
reactloops.EmitStatus(loop, "读取失败 / Read Failed")
invoker.AddToTimeline("read_file_not_found", fmt.Sprintf(`【文件不存在】指定的Java文件未找到：%s

【可能原因】：
1. 文件路径拼写错误
2. 使用了相对路径但工作目录不对
3. 文件已被删除或移动

【立即行动】：
1. 使用 list_files 列出目录中的所有Java文件
2. 检查文件路径的拼写
3. 尝试使用绝对路径

【下一步】：使用 list_files 查看实际存在的文件`, filePath))
operator.Fail("failed to read file: " + err.Error())
```

要求：

- timeline 可以保留结构化“可能原因 / 立即行动 / 下一步”。
- timeline 不应塞完整源码、完整 diff、完整 HTTP 包或内部堆栈。
- 需要引用大内容时，先 `SaveAndPinFile`，timeline 只写文件路径和短 preview。

### 7. 在各 action 上分散声明 reason / summary 类参数

坏例子：

```go
// 不同 action 各自声明同义说明字段，并挂到 re-act-loop-thought
aitool.WithStringParam("rewrite_reason", ...),  // rewrite_java_file
aitool.WithStringParam("read_reason", ...),     // read_java_file — 同名 schema 会互相覆盖
[]*reactloops.LoopStreamField{{
    FieldName: "rewrite_reason",
    AINodeId:  "re-act-loop-thought",
}},
```

问题：

- 同一 loop 内所有 action 共用一份 schema，多个 `*_reason` / `summary` 字段会互相覆盖或让模型填多遍相似内容。
- 框架已在 `buildSchema` 提供共用的 `human_readable_thought`；再挂 per-action 说明字段会**重复刷 thought 通道**。

好例子：

```go
// 各 action 只保留业务参数；调用说明写在 JSON 的共用字段 human_readable_thought 中
return reactloops.WithRegisterLoopAction(
    "rewrite_java_file",
    desc,
    []aitool.ToolOption{
        aitool.WithStringParam("file_path", ...),
        aitool.WithIntegerParam("rewrite_start_line", ...),
    },
    verifier, handler,
)
```

`loop_plan/action_recon.go` 不为每个工具单独声明 `request`；`loop_java_decompiler` 已去掉 `rewrite_reason`，finish 示例也不再使用多余的 `summary` 参数。

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
rewrite_reason
summary
operator.Feedback
AddToTimeline
ai_node_id_i18n.go
```

逐个判断它属于“用户可见状态、Action 摘要、最终答案、文件、调试日志、模型观察、schema 共用字段”中的哪一类。

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

### 第六步：保留 timeline 错误恢复指导

迁移时逐条检查 `AddToTimeline`。处理原则：

- **保留**：路径错误、权限错误、文件不存在、语法错误、diff 失败等可恢复场景的结构化指导。
- **压缩**：成功结果、扫描列表、源码内容、diff 内容、HTTP request/response 等大体量材料。
- **迁移**：完整材料写入 `SaveAndPinFile`，timeline 只保留文件路径、计数、短 preview 和下一步建议。
- **删除或改 log**：内部 fallback、性能数据、堆栈、开发调试碎片。

推荐 timeline 模板：

```text
【问题/结果】一句话说明

【关键信息】：
- 数量 / 路径 / 文件引用

【可能原因】：
1. ...

【下一步】：
1. ...
```

不要为了减少前端输出而把 timeline 改成 `"failed: err"` 这类不可恢复的一行文本。

### 第七步：注册 `EmitActionLog` 的 nodeId i18n

为 Action 选用稳定的 `nodeId`（kebab-case）后，在 `common/schema/ai_node_id_i18n.go` 的 `nodeIdMapper` 中补充条目：

```go
"java-decompile-jar": {
    Zh: "JAR 反编译",
    En: "Decompile JAR",
},
"java-list-files": {
    Zh: "列出 Java 文件",
    En: "List Java Files",
},
```

要求：

- 中英文都要填，与 `EmitStatus` 的双语习惯一致。
- `nodeId` 与代码里 `EmitActionLog(loop, nodeId, ...)` 的字符串完全一致。
- 改造 PR 应同时包含 loop 代码与 i18n 映射，避免前端分区标题显示原始 id。

### 第八步：共用 schema 下收敛调用说明字段

排查 loop 内所有 action 的 `ToolOption` 与 `LoopStreamField`，把各类 reason/summary/explanation 同义参数**删到只留 schema 里那一个共用字段**（框架默认为 `human_readable_thought`）：

- Prompt / `output_example` 不再示范 per-action 的 `*_reason` 或多余的 `summary`。
- 不为说明类字段再注册指向 `re-act-loop-thought` 的 `LoopStreamField`。
- Handler 不把共用说明字段拼进 `EmitActionLog` 或 `operator.Feedback`（见上文不良行为第 1 条）。

## `loop_http_flow_analyze` 参考实现

以 `common/ai/aid/aireact/reactloops/loop_http_flow_analyze` 为样板时，可按「已完成 / 仍待收敛」对照改造。

### 已完成

| 区域 | 文件 | 做法 |
|---|---|---|
| 查询 / 匹配 | `action_query_flows.go`、`action_match_flows.go`、`action_match_flows_simple.go` | 开始/完成各一条 `EmitActionLog`；阶段切换走 `EmitStatus`；大结果 `SaveAndPinFile` 后只把 preview 放进 reference material |
| 流详情 | `action_get_flow_detail.go` | 两行 `EmitActionLog`；完整 request/response 作为 reference material，不逐段 stream |
| Fuzz 子循环 | `action_dispatch_fuzz.go` | 多段 `EmitStatus` 覆盖准备→执行→收集；结果写入 `http_flow_analysis_evidence`，完成日志只报摘要 |
| 证据记录 | `action_output_findings.go` | `record_http_flow_evidence` 通过 `StreamField`（`http-flow-analysis-evidence`）流式交付 Markdown，handler 只合并状态 |
| 直接答复 | `action_directly_answer.go` | `AITagField` / `StreamField` + `EmitResultAfterStream` |
| 退出兜底 | `finalize.go` | AI 摘要走 `EmitStreamEventWithContentType` + `EmitTextReferenceMaterial`；落定 `EmitResultAfterStream` |
| 思考字段 | 各 Action handler | Action 日志未拼接 `human_readable_thought`（该字段仅保留在 prompt 示例中供模型内部使用） |

### 仍待收敛

按优先级处理：

1. **未知总量进度无效** — `action_match_flows.go`、`action_match_flows_simple.go` 在扫描循环里调用 `EmitProgress(loop, totalCount, 0, ...)`。`EmitProgress` 在 `total <= 0` 时直接返回，等于没有进度输出。应改为每 100 条节流的 `EmitStatus`，例如 `已扫描 %d 条流量 / Scanned %d Flows`（参见上文「循环内逐条输出」好例子）。

2. **`operator.Feedback` / timeline 仍塞全文** — `query_http_flows`、`match_flows`、`match_flows_simple` 的 `feedbackMsg` 和 `AddToTimeline` 仍附带完整 `summary` 字符串。大结果已文件化后，Feedback 应只保留查询名、命中数、文件路径和短 preview。

3. **`init_task.go` 文件 pin 未统一** — `formatAttachedHTTPFlowsDetailed` 与 `inlineOrSpillAttachedText` 仍直接 `os.WriteFile` / `consts.TempAIFileFast` + `emitter.EmitPinFilename`。应统一为 `SaveAndPinFile`，并避免把完整 `detailedInfo` 写入 timeline（只写条数 + 文件路径 + preview）。

4. **`get_http_flow_detail` Feedback 未限长** — `operator.Feedback(summary)` 和 `AddToTimeline` 仍传入完整 request/response 摘要。超长时应先文件化，Feedback 只留 flow 元信息与文件引用。

5. **死代码清理** — `findings.go` 中 `emitHTTPFlowEvidenceMarkdown`（直接 `EmitTextMarkdownStreamEvent`）已无调用方；证据展示已由 `record_http_flow_evidence` 的 `StreamField` 承担，可删除该函数避免误用。

6. **`dispatch_fuzz_test` reference 体积** — 完成日志的 `EmitActionLog(..., fuzzResult)` 可能携带完整子循环报告。应对 `fuzzResult` 做 `ShrinkTextBlock` 或文件化后再作 reference material。

## `loop_java_decompiler` 参考实现

| 区域 | 文件 | 做法 |
|---|---|---|
| 输出分层 | 各 `action_*.go` | 开始/完成 `EmitActionLog` + 阶段 `EmitStatus`；大结果 `SaveAndPinFile`；`Feedback` / timeline 限长 |
| 错误恢复 | 各 `action_*.go` | 校验失败、读写失败、语法错误、diff 错误等 timeline 保留“可能原因 / 立即行动 / 下一步” |
| nodeId i18n | `common/schema/ai_node_id_i18n.go` | `java-decompile-jar`、`java-list-files`、`java-read-file` 等 6 个 nodeId 已注册 |
| 调用说明 | `action_rewrite_java_file.go`、prompts | 已删除 `rewrite_reason`；finish 示例去掉 `summary`；只保留共用 `human_readable_thought` |

## 验收标准

一次输出行为改造完成后，必须满足：

- 前端不会出现无节制“思考”或调试碎片。
- 每个 Action 最多保留开始和完成两类用户可见日志。
- 长结果都有文件化路径，界面只显示摘要。
- 状态栏文案是短句、中英双语、可覆盖。
- 技术日志只在 `log.*` 中出现。
- 最终答案仍通过最终答案通道流式展示，并调用 `EmitResultAfterStream` 落定。
- `operator.Feedback` 和 timeline 没有塞入不必要的大段原文。
- timeline 中的可恢复错误指导没有被删成无上下文的一行错误。
- 新增 `EmitActionLog` 的 `nodeId` 已在 `ai_node_id_i18n.go` 注册中英标签。
- 各 action 未重复声明 reason/summary 类参数；调用说明只走 schema 共用字段。

## Code Review 提问清单

Review 专注模式输出相关 PR 时，直接问：

- 这条输出是给用户看的，还是给开发者排查的？
- 这条信息是否会在循环中按数据量增长？
- 如果返回 5000 条 flow，前端会不会被刷屏？
- `human_readable_thought` 有没有被拼进用户可见文案？
- 大内容有没有 `SaveAndPinFile` 和 preview？
- 最终答案有没有走 `StreamField` / `AITagField` / `EmitResultAfterStream`？
- 直接 emitter 调用是否真的需要特殊 stream 能力？
- timeline 中“可能原因 / 立即行动 / 下一步”的恢复指导有没有被误删？
- 新增的 `nodeId` 有没有在 `ai_node_id_i18n.go` 里补 i18n？
- 有没有在多个 action 上分散声明 reason/summary 类参数，导致 schema 冲突或 thought 双写？
