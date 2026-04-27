# 07. LiteForge：把不确定塞进确定的盒子

> 回到 [README](../README.md) | 上一章：[06-emitter-and-streaming.md](06-emitter-and-streaming.md) | 下一章：[08-determinism-mechanisms.md](08-determinism-mechanisms.md)

LiteForge 是 reactloops 把"非确定 LLM"变成"确定中间步骤"的关键武器。本章讲清楚：

- LiteForge 是什么、为什么需要它
- 与 `aiforge.ForgeFactory` 的区别
- ReAct 入口的三档优先级
- API 速查
- 在 reactloops 里的典型应用清单
- 端到端实战

## 7.1 定位：单步结构化抽取

源码 [common/aiforge/liteforge.go:49-57](../../../../aiforge/liteforge.go) 的注释说得很清楚：

> LiteForge 被设计只允许提取数据，生成结构化（单步），如果需要多步拆解，不能使用 LiteForge

### 它解决的问题

主循环的 LLM 输出是高熵的：可能是 markdown，可能是 JSON，可能直接说"我不知道"。如果你想要：

- "从用户输入里抽出 URL"
- "把这段日志的关键字段提取成 JSON"
- "判断当前任务的意图属于哪一类"
- "为某个工具调用生成参数"

这些都是**单步、结构化、可以被 JSON Schema 严格约束**的任务。LiteForge 提供：

| 能力 | 实现 |
|------|------|
| 强制 JSON Schema 输出 | `OutputSchema` 渲染进 prompt + 校验 |
| `@action` 字段守门 | `ExtractValidActionFromStream` 必须找到正确 action 名 |
| 失败重试 | `CallAITransaction` 自动重跑 |
| 流式字段 | LLM 还在写 JSON 时就把某字段流到 UI |
| 三档优先级 | speed / quality / 默认 |
| 防注入 | nonce 包裹的 prompt 模板 |

### 它不解决的问题

- 多步推理、需要工具调用 → 用主 ReAct 循环
- 需要思维链 / 长 reasoning → 用普通 LLM call
- 需要 `aiforge.ForgeFactory`（带 RAG / 蓝图、工具栈、Persistence Memory）→ 用大 forge

LiteForge 是**轻量、单步、确定性强**。这就是它叫 "Lite" 的原因。

## 7.2 与 `ForgeFactory` 的区别

| 维度 | LiteForge | ForgeFactory |
|------|-----------|--------------|
| 步数 | 单步 | 多步（有 `Action` / `Plan`） |
| 工具调用 | 不支持 | 支持 |
| 蓝图 / RAG | 不带 | 内置 |
| Memory | 不带 persistent | 带 |
| 输入 schema | 可选（`RequireSchema`） | 工具参数 schema |
| 输出 schema | 必填 | 可选 |
| 适用场景 | 抽取、分类、生成单条结构化数据 | 跑一个完整子任务 |

reactloops 里 99% 的"中间智能步骤"都用 LiteForge，因为：

- 主 ReAct 已经是大 forge 的角色，不能在它里面再嵌套大 forge
- 但是某些步骤需要"调一次 LLM 立即拿到 JSON"，LiteForge 完美匹配

## 7.3 三档优先级

源码 [aireact/invoke_liteforge.go:65-81](../../invoke_liteforge.go)：

```go
func (r *ReAct) InvokeSpeedPriorityLiteForge(ctx, actionName, prompt, outputs, opts) (*aicommon.Action, error)
func (r *ReAct) InvokeQualityPriorityLiteForge(ctx, actionName, prompt, outputs, opts) (*aicommon.Action, error)
func (r *ReAct) InvokeLiteForge(ctx, actionName, prompt, outputs, opts) (*aicommon.Action, error)  // = Quality
```

三档区别就是用哪个 AI callback：

| 入口 | 用的 callback | 适用 |
|------|---------------|------|
| `InvokeSpeedPriorityLiteForge` | `r.config.SpeedPriorityAICallback` | 快速分类、感知、抽取，可容忍精度略低 |
| `InvokeQualityPriorityLiteForge` | `r.config.QualityPriorityAICallback` | 关键决策、最终总结、需要精度 |
| `InvokeLiteForge` | 同 Quality | 默认 |

> 这两个 callback 由 ReAct 顶层配置决定。一般生产环境 speed 用快模型（如 gpt-4o-mini / claude-haiku），quality 用慢模型（如 gpt-4o / claude-sonnet/opus）。

### 选哪个？

| 场景 | 推荐 |
|------|------|
| 感知（perception）：每轮都跑 | speed |
| 自旋检测：可能频繁触发 | speed |
| 能力分块匹配：上下文小 | speed |
| HTTP 包提取：上下文小 | speed |
| 任务计划生成：决定后续走向 | quality |
| 最终总结 / 报告 | quality |
| 复杂代码生成 | quality |

不确定时用 `InvokeLiteForge`（= Quality）就好。

## 7.4 API 速查

### 7.4.1 `InvokeXxxLiteForge` 完整签名

```go
action, err := loop.GetInvoker().(*aireact.ReAct).InvokeSpeedPriorityLiteForge(
    ctx,                  // 取消上下文，常用 task.GetContext()
    "actionName",         // schema 里 @action 字段必须等于这个
    prompt,               // 预渲染好的 markdown prompt
    []aitool.ToolOption{  // 输出字段定义
        aitool.WithStringParam("topic", aitool.WithParam_Description("...")),
        aitool.WithStringParam("summary", aitool.WithParam_Description("...")),
        aitool.WithFloatParam("confidence", aitool.WithParam_Description("...")),
    },
    // 可选：流式字段、回调...
    aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "summary"),
    aicommon.WithGeneralConfigStreamableFieldEmitterCallback(
        []string{"summary"}, 
        func(key string, r io.Reader, emitter *aicommon.Emitter) { /* 自定义处理 */ },
    ),
)
```

返回 `*aicommon.Action`：

```go
action.GetString("topic")
action.GetFloat("confidence")
action.GetStringSlice("tags")
action.GetInvokeParams("nested_object")
```

### 7.4.2 输出 schema 的几种构造方式

**方式 A：用 `aitool.ToolOption` 构造（最常用）**

```go
outputs := []aitool.ToolOption{
    aitool.WithStringParam("answer", 
        aitool.WithParam_Required(true),
        aitool.WithParam_Description("回答内容"),
    ),
    aitool.WithIntegerParam("confidence",
        aitool.WithParam_Description("0-100 置信度"),
    ),
    aitool.WithStringSliceParam("tags",
        aitool.WithParam_Description("标签列表"),
    ),
}
```

**方式 B：原始 schema 字符串**

```go
forge, _ := aiforge.NewLiteForge("my_forge",
    aiforge.WithLiteForge_OutputSchemaRaw("my-action", `{
        "type": "object",
        "properties": {
            "@action": {"const": "my-action"},
            "answer": {"type": "string"}
        },
        "required": ["@action", "answer"]
    }`),
)
```

**方式 C：直接拿 `aiforge.LiteForge`（不通过 ReAct invoker）**

参考 reactloops 内部一些直接构造 LiteForge 的代码（如 `perception.go`）。

### 7.4.3 流式字段三种回调

```go
// 1. 默认：流到指定 NodeId
aicommon.WithGeneralConfigStreamableFieldWithNodeId(nodeId, fieldKey)

// 2. 自定义 Emitter 回调
aicommon.WithGeneralConfigStreamableFieldEmitterCallback(
    []string{"summary"},
    func(key string, r io.Reader, emitter *aicommon.Emitter) {
        // 自己决定怎么处理
        bs, _ := io.ReadAll(r)
        emitter.EmitTextMarkdownStreamEvent("custom-node", bytes.NewReader(bs), taskId)
    },
)

// 3. 简单回调（无 emitter）
aicommon.WithGeneralConfigStreamableFieldCallback(
    []string{"summary"},
    func(key string, r io.Reader) { /* ... */ },
)
```

## 7.5 在 reactloops 中的典型应用清单

下面是从代码 grep 出来的应用，按场景分组。

### A. 感知层（perception）

[perception.go](../perception.go) 每隔几轮跑一次：抽取当前任务的 topic / keywords / summary / confidence，注入下一轮 prompt 的 `<|REFLECTION_<nonce>|>` 段。

```go
action, err := r.invoker.InvokeSpeedPriorityLiteForge(ctx, "perception",
    promptForPerception,
    []aitool.ToolOption{
        aitool.WithStringSliceParam("topics"),
        aitool.WithStringSliceParam("keywords"),
        aitool.WithStringParam("summary"),
        aitool.WithFloatParam("confidence"),
        aitool.WithBoolParam("changed"),
    },
)
state.Topics = action.GetStringSlice("topics")
state.OneLinerSummary = action.GetString("summary")
```

### B. 能力分块匹配（capability_search）

[capability_search.go](../capability_search.go) 把候选能力分块（防超长 prompt），每块用 LiteForge 让 LLM 选 top-N。

### C. 自旋 AI 检测

[spin_detection.go](../spin_detection.go) 第二层：当同 type action 触发 N 次后，跑 LiteForge 让 LLM 判断"是否真的是死循环"。

### D. 技能加载冲突仲裁

[loopinfra/action_loading_skills.go](../loopinfra/action_loading_skills.go) 加载多个技能时，用 LiteForge 判断哪个最相关、是否冲突。

### E. 计划生成

[loop_plan/generate_document_and_plan.go](../loop_plan/generate_document_and_plan.go) / [loop_plan/facts.go](../loop_plan/facts.go) 一系列 LiteForge 步骤生成正式的多步任务计划。

### F. HTTP fuzz 初始化

[loop_http_fuzztest/init.go:162](../loop_http_fuzztest/init.go) 在 init 阶段提取测试要点；[init.go:210](../loop_http_fuzztest/init.go) 从用户原始输入抽 raw HTTP 请求或 URL。

```go
action, err := invoker.InvokeSpeedPriorityLiteForge(task.GetContext(), 
    "http_fuzztest_init_booststrap", prompt,
    []aitool.ToolOption{
        aitool.WithStringParam("thought", aitool.WithParam_Description("...")),
    }, 
    aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "quick_plan"),
)
```

注意流式字段 `quick_plan`：LLM 还在写 thought 时，前端 `quick_plan` NodeId 已经开始接收，用户体验上不会有"卡 5 秒"的感觉。

### G. HTTP 流量分析最终总结

[loop_http_flow_analyze/finalize.go](../loop_http_flow_analyze/finalize.go) 在 `OnPostIteraction(isDone=true)` 时如果 LLM 没主动 `directly_answer`，用 LiteForge 强制生成一份 markdown 总结。

### H. 意图识别（loop_intent / deep_intent）

[deep_intent.go](../deep_intent.go) 用 LiteForge 提取深度意图，并把候选能力（capabilities）一起返回给上层。

### I. 报告生成 / SyntaxFlow 规则 / yak 代码

各个领域 loop 在 `init.go` / `finalize.go` 里大量用 LiteForge 做"决定主循环走向"的智能判断。

## 7.6 实战 1：在自定义 action 里加一个 LiteForge 抽取

假设我们要写一个 action：用户给一段日志，我们调 LLM 抽出关键字段。

```go
import (
    "github.com/yaklang/yaklang/common/ai/aid/aireact"
    "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
    "github.com/yaklang/yaklang/common/ai/aid/aitool"
    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

reactloops.WithRegisterLoopAction(
    "extract_log_fields",
    "从日志片段抽取关键字段",
    []aitool.ToolOption{
        aitool.WithStringParam("log_snippet", 
            aitool.WithParam_Required(true),
            aitool.WithParam_Description("待分析的日志片段"),
        ),
    },
    func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
        if action.GetString("log_snippet") == "" {
            return fmt.Errorf("log_snippet is required")
        }
        return nil
    },
    func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
        invoker := loop.GetInvoker()
        react, ok := invoker.(*aireact.ReAct)
        if !ok {
            op.Fail("invoker is not *ReAct")
            return
        }
        snippet := action.GetString("log_snippet")
        prompt := fmt.Sprintf("从下面的日志中抽取关键字段：\n\n```\n%s\n```", snippet)

        ctx := loop.GetCurrentTask().GetContext()
        result, err := react.InvokeSpeedPriorityLiteForge(ctx,
            "log-extract",
            prompt,
            []aitool.ToolOption{
                aitool.WithStringParam("level", aitool.WithParam_Description("日志等级 INFO/WARN/ERROR")),
                aitool.WithStringParam("timestamp", aitool.WithParam_Description("时间戳")),
                aitool.WithStringParam("source", aitool.WithParam_Description("源 / 模块")),
                aitool.WithStringParam("message", aitool.WithParam_Description("消息体")),
                aitool.WithStringSliceParam("error_keywords"),
            },
            aicommon.WithGeneralConfigStreamableFieldWithNodeId("thought", "message"),
        )
        if err != nil {
            invoker.AddToTimeline("extract-failed", fmt.Sprintf("liteforge failed: %v", err))
            op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
            op.Continue()
            return
        }

        invoker.AddToTimeline("extract-log-fields", map[string]any{
            "level":     result.GetString("level"),
            "timestamp": result.GetString("timestamp"),
            "source":    result.GetString("source"),
            "keywords":  result.GetStringSlice("error_keywords"),
        })
        loop.Set("last_log_fields", result.ActionParams)
        op.Feedback(map[string]any{
            "extracted_log_fields": result.ActionParams,
        })
        op.Continue()
    },
)
```

要点：

1. **从 invoker 拿到 `*aireact.ReAct`**：这是入口
2. **prompt 构造**：清晰、有上下文、用代码块包裹用户数据
3. **outputs 设计**：每个字段都有 description，LLM 才知道怎么填
4. **流式字段**：让 `message` 实时流到 UI
5. **失败 fallback**：不直接 Fail loop，而是 `Continue` + `Critical` reflection，让主 LLM 反思下一步
6. **结果落地**：`AddToTimeline` + `loop.Set` + `op.Feedback` 三处都写

### 7.6.1 `AddToTimeline` vs `loop.Set` vs `op.Feedback`

| 方法 | 时长 | 用途 |
|------|------|------|
| `AddToTimeline(name, data)` | 整个 task 生命周期 | 历史记录，会渲染进下一轮 prompt 的 timeline |
| `loop.Set(key, val)` | loop 实例生命周期 | 跨 action 状态共享，prompt 可读 |
| `op.Feedback(any)` | 仅下一轮 | 强行注入 reactiveData，prompt 优先级最高 |

**最佳实践**：抽取的关键事实写 timeline + loop.Set，关键反馈消息写 Feedback。

## 7.7 实战 2：用 LiteForge 写 finalize fallback

参考 [loop_http_flow_analyze/finalize.go:174-191](../loop_http_flow_analyze/finalize.go)：

```go
func deliverFinalAnswerFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string) {
    react, ok := invoker.(*aireact.ReAct)
    if !ok {
        return
    }
    ctx := loop.GetCurrentTask().GetContext()

    prompt := buildFinalSummaryPrompt(loop, contextMaterials)

    taskID := loop.GetCurrentTask().GetIndex()
    result, err := react.InvokeQualityPriorityLiteForge(ctx,
        "http-flow-analyze-summary",
        prompt,
        []aitool.ToolOption{
            aitool.WithStringParam("summary",
                aitool.WithParam_Required(true),
                aitool.WithParam_Description("markdown 总结"),
            ),
        },
        aicommon.WithGeneralConfigStreamableFieldEmitterCallback(
            []string{"summary"},
            func(key string, r io.Reader, emitter *aicommon.Emitter) {
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
            },
        ),
    )
    if err != nil {
        log.Errorf("fallback summary failed: %v", err)
        return
    }
    invoker.EmitResultAfterStream(result.GetString("summary"))
}
```

### 关键设计

1. **用 quality 模型**：finalize 是关键决策
2. **流到 `re-act-loop-answer-payload`**：用户看到的"最终回答"位置
3. **Reference Material**：把 contextMaterials（基于哪些原始数据）作为引用资料同步发出
4. **`EmitResultAfterStream`**：等流结束后再发结果事件，避免顺序错乱
5. **失败兜底**：日志即可，不要再抛错

## 7.8 LiteForge 的 prompt 模板

LiteForge 内部用了一个固定模板（[liteforge.go:242-276](../../../../aiforge/liteforge.go)）：

```text
# Preset
你现在在一个任务引擎中，是一个输出JSON的数据处理和总结提示小助手...

<background_<NONCE>>
{你传入的 prompt}
</background_<NONCE>>

<timeline_<NONCE>>
{自动从 ContextProvider 拿到的 timeline}
</timeline_<NONCE>>

# 牢记
{自动从 ContextProvider 拿到的 PersistentMemory}

<params_<NONCE>>
{你传入的 params（一般是 query=prompt）}
</params_<NONCE>>

# Output Formatter

请你根据下面 SCHEMA 构建数据 ...

# SCHEMA

<schema_<NONCE>>
{自动渲染的 OutputSchema}
</schema_<NONCE>>
```

所以你**不需要**在 prompt 里手动写 schema 或要求"返回 JSON"，LiteForge 自己处理。

## 7.9 常见陷阱

### 1. 拿不到 `*aireact.ReAct`

如果 `loop.GetInvoker()` 不是 `*aireact.ReAct`，说明在测试环境或者非标准调用栈。**永远要做类型 assertion**：

```go
react, ok := invoker.(*aireact.ReAct)
if !ok {
    op.Fail("invoker not *aireact.ReAct")
    return
}
```

### 2. action 名字写错

`actionName` 必须和 `OutputActionName`（默认 = actionName）一致，否则 `ExtractValidActionFromStream` 会失败、走重试。

### 3. 流式字段没接 callback

注册了流式字段但没接 callback，会**默认 io.Discard**（实际有 stdout debug，但生产看不到）。一定要接 NodeId 或自定义 callback。

### 4. 用 LiteForge 跑多步任务

**不要这样做**：循环里调 LiteForge 5 次。这种场景应该用 `aiforge.ForgeFactory` 或在主 ReAct 里走多 action。

### 5. prompt 太长 / 不裁剪

LiteForge 不会自动 shrink。`perception.go` 里有 `ShrinkTextBlockByTokens` 可以参考。一般给 LiteForge 的 prompt 控制在 8k token 内。

### 6. 忘记 `aitool.WithParam_Required(true)`

LLM 会"省略"它觉得不重要的字段，结果 `action.GetString("xxx")` 返回空。关键字段一定要 Required。

## 7.10 进一步阅读

- [05-hooks-and-lifecycle.md](05-hooks-and-lifecycle.md)：在 InitTask / OnPostIteraction 用 LiteForge
- [08-determinism-mechanisms.md](08-determinism-mechanisms.md)：感知 / 反思 / 自旋都基于 LiteForge
- [09-capabilities.md](09-capabilities.md)：capability_search 用 LiteForge 做分块匹配
- 源码：
  - [common/aiforge/liteforge.go](../../../../aiforge/liteforge.go)
  - [common/ai/aid/aireact/invoke_liteforge.go](../../invoke_liteforge.go)
  - [common/aiforge/extra_general_config.go](../../../../aiforge/extra_general_config.go)（`WithGeneralConfigStreamableFieldXxx`）
