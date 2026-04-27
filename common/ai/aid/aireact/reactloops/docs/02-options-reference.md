# 02. Options 完整参考

> 回到 [README](../README.md) | 上一章：[01-architecture.md](01-architecture.md) | 下一章：[03-prompt-system.md](03-prompt-system.md)

本章是 [options.go](../options.go) 的"使用手册"。所有 `With*` 选项按职责分组，每个给出：

- **签名**：函数原型
- **作用**：写入哪个字段、影响什么
- **默认值**：不设置时的行为
- **典型场景**：什么时候应该用
- **示例**：最小代码片段

## 2.1 基础与生命周期

### `WithMaxIterations(n int)`

```go
func WithMaxIterations(maxIterations int) ReActLoopOption
```

设置 `r.maxIterations`。**默认值 100**。

主循环顶部的硬限制。超过会调 `finishIterationLoopWithError`，除非 `OnPostIterationOperator.IgnoreError()` 被调用，否则返回错误。

```go
WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())) // 跟随 config
WithMaxIterations(20)                                        // 严格限制
```

[loop_http_fuzztest/init.go:48](../loop_http_fuzztest/init.go) 用的是配置驱动。

### `WithVar(key string, value any)` / `WithVars(map[string]any)`

```go
func WithVar(key string, value any) ReActLoopOption
func WithVars(vars map[string]any) ReActLoopOption
```

向 `r.vars` 写初始 KV。运行时 action 可以用 `loop.Get(key)` / `loop.Set(key, value)` 读写。常用于在 `init` 时把外部参数传进 loop。

### `WithNoEndLoadingStatus(b ...bool)`

```go
func WithNoEndLoadingStatus(b ...bool) ReActLoopOption
```

设置 `r.noEndLoadingStatus`。默认 `false`。当为 `true` 时，结束 defer 不会发 `loadingStatus("end")`。

**典型场景**：作为子 loop 被另一个 loop 嵌套调用时，不要让自己发 "end"，否则前端 UI 会以为整个任务结束了。深度意图识别的子 loop 就用了这个：

```go
opts = append(opts, WithOnLoopInstanceCreated(func(l *ReActLoop) {
    intentLoop = l
}), WithNoEndLoadingStatus(true), WithUseSpeedPriorityAICallback(true))
```
[deep_intent.go:45-47](../deep_intent.go)

### `WithLoopPromptGenerator(generator)`

```go
func WithLoopPromptGenerator(generator ReActLoopCoreGenerateCode) ReActLoopOption
```

**几乎不用**。整个 prompt 生成逻辑被你接管。如果你只想改某一段（持久指令、动态数据），用 `WithPersistentInstruction` 等更细粒度的选项。

### `WithPeriodicVerificationInterval(interval int)`

```go
func WithPeriodicVerificationInterval(interval int) ReActLoopOption
```

设置周期性验证的迭代间隔。默认值由 `BasicAICommonConfigOption` 从 config 读取。

更短的间隔 → 更频繁验证 → 成本高、更早发现偏离。

## 2.2 Prompt 构造

### `WithPersistentInstruction(instruction string)`

```go
func WithPersistentInstruction(instruction string) ReActLoopOption
```

最常用的 prompt 注入方式。`instruction` 是一段 Go template 字符串，渲染时会自动获得：

- `Nonce`：当前轮次的唯一标识
- 来自 `getRenderInfo()` 的所有变量（`CurrentTime` / `OSArch` / `WorkingDir` / `Tools` / `AllowPlan` / `AllowKnowledgeEnhanceAnswer` 等）

源码 [options.go:259-267](../options.go)：

```go
func WithPersistentInstruction(instruction string) ReActLoopOption {
    return WithPersistentContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
        _, result, err := loop.getRenderInfo()
        if err != nil {
            return "", utils.Errorf("get basic prompt info failed: %v", err)
        }
        result["Nonce"] = nonce
        return utils.RenderTemplate(instruction, result)
    })
}
```

**典型用法**（embed 模板文件）：

```go
//go:embed prompts/persistent_instruction.txt
var instruction string

reactloops.WithPersistentInstruction(instruction)
```

详见 [03-prompt-system.md](03-prompt-system.md)。

### `WithPersistentContextProvider(provider ContextProviderFunc)`

```go
type ContextProviderFunc func(loop *ReActLoop, nonce string) (string, error)
func WithPersistentContextProvider(provider ContextProviderFunc) ReActLoopOption
```

更底层的版本。如果你需要根据 loop 的运行时状态生成不同的指令（不只是模板渲染），用这个。

### `WithReflectionOutputExample(example string)` / `WithReflectionOutputExampleContextProvider(provider)`

```go
func WithReflectionOutputExample(example string) ReActLoopOption
func WithReflectionOutputExampleContextProvider(provider ContextProviderFunc) ReActLoopOption
```

注入"输出示例"段。被渲染到 `<|OUTPUT_EXAMPLE_...|>` 区块。

**特别**：`WithReflectionOutputExample` 内部还会自动遍历 `loop.loopActions` 的所有名字，从 `GetLoopAction(actionName).OutputExamples` 或 `GetLoopMetadata(actionName).OutputExamplePrompt` 中收集每个 action 的示例并拼接：

```go
// options.go:222-256
for _, actionName := range loop.loopActions.Keys() {
    if action, ok := GetLoopAction(actionName); ok && action.OutputExamples != "" {
        rendered, err := utils.RenderTemplate(action.OutputExamples, result)
        // ...append
    } else if meta, ok := GetLoopMetadata(actionName); ok && meta.OutputExamplePrompt != "" {
        // fallback to LoopMetadata
    }
}
```

### `WithReactiveDataBuilder(provider FeedbackProviderFunc)`

```go
type FeedbackProviderFunc func(loop *ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error)
func WithReactiveDataBuilder(provider FeedbackProviderFunc) ReActLoopOption
```

每轮的"动态反应数据"。`feedbacker` 是上一轮 `operator.Feedback(...)` 的累积。

返回值会被填到 prompt 模板的 `<|REFLECTION_...|>` 区块。

[loop_http_fuzztest/init.go:52-92](../loop_http_fuzztest/init.go) 是个完整示例：把 `feedbacker.String()` + `loop.Get(originalRequest)` + `loop.Get(diff_result)` + 最近 action 摘要等所有上下文拼成 Markdown。

## 2.3 Hook 与生命周期

### `WithInitTask(handler)`

```go
func WithInitTask(handler func(loop *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator)) ReActLoopOption
```

主循环开始前的初始化。`op` 提供 `Done` / `Failed` / `NextAction` / `RemoveNextAction`。

典型用法：从用户输入抽参数（用 LiteForge）、引导环境（如 fuzz 的 fuzztag 上下文）、决定首轮必须用某个 action。

### `WithOnPostIteraction(...fns)`

```go
func WithOnPostIteraction(fn ...func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *OnPostIterationOperator)) ReActLoopOption
```

**注意拼写**是 `Iteraction`（历史遗留，不是 `Iteration`）。

回调在两种时机触发：

1. **每轮迭代结束**（`isDone=false`）：用于 per-iteration 的统计、增量 finding 收集等。
2. **整个循环结束**（`isDone=true`）：用于持久化、强制 finalize、生成总结报告。

回调列表，**顺序执行**，多个 hook 都会跑。详见 [05-hooks-and-lifecycle.md](05-hooks-and-lifecycle.md)。

### `WithOnLoopInstanceCreated(fn)`

```go
func WithOnLoopInstanceCreated(fn func(loop *ReActLoop)) ReActLoopOption
```

在 `CreateLoopByName` 创建出实例后立刻回调。常用于"我需要保留这个 loop 的引用以便后面读它的状态"。

[deep_intent.go:45-49](../deep_intent.go) 用它捕获子 loop 引用，等子 loop 跑完后读取 `intentLoop.Get("intent_analysis")` 等字段。

### `WithOnTaskCreated(fn)` / `WithOnAsyncTaskTrigger(fn)` / `WithOnAsyncTaskFinished(fn)`

```go
func WithOnTaskCreated(fn func(task aicommon.AIStatefulTask)) ReActLoopOption
func WithOnAsyncTaskTrigger(fn func(action *LoopAction, task aicommon.AIStatefulTask)) ReActLoopOption
func WithOnAsyncTaskFinished(fn func(task aicommon.AIStatefulTask)) ReActLoopOption
```

任务生命周期 hook。`OnAsyncTaskTrigger` 在切到异步模式时调用（如 plan / forge action）。

## 2.4 Action 注册

### `WithRegisterLoopAction(name, desc, opts, verifier, handler)`

```go
func WithRegisterLoopAction(
    actionName string,
    desc string,
    opts []aitool.ToolOption,
    verifier LoopActionVerifierFunc,
    handler LoopActionHandlerFunc,
) ReActLoopOption
```

最常用的 action 注册方式。`opts` 是参数 schema。

### `WithRegisterLoopActionWithStreamField(name, desc, opts, fields, verifier, handler)`

```go
func WithRegisterLoopActionWithStreamField(
    actionName string,
    desc string,
    opts []aitool.ToolOption,
    fields []*LoopStreamField,
    verifier LoopActionVerifierFunc,
    handler LoopActionHandlerFunc,
) ReActLoopOption
```

带流式字段。当 LLM 输出 JSON 中某字段开始流时，会立即推送到指定 NodeId 的事件流。

### `WithRegisterLoopActionFromTool(tool *aitool.Tool)`

```go
func WithRegisterLoopActionFromTool(tool *aitool.Tool) ReActLoopOption
```

把一个 `aitool.Tool` 包装成 `LoopAction`。等价于：

```go
// options.go:127-139
action := ConvertAIToolToLoopAction(tool)
r.actions.Set(name, action)
```

详见 [04-actions.md](04-actions.md) 来源 3。

### `WithOverrideLoopAction(action *LoopAction)`

```go
func WithOverrideLoopAction(action *LoopAction) ReActLoopOption
```

直接覆盖一个已注册 action。最常见的场景是覆盖默认的 `directly_answer` 来加自己的 finalize 逻辑：

```go
reactloops.WithOverrideLoopAction(loopActionDirectlyAnswerHTTPFuzztest)
```
参考 [loop_http_fuzztest/action_directly_answer.go](../loop_http_fuzztest/action_directly_answer.go)。

### `WithActionFactoryFromLoop(name string)`

```go
func WithActionFactoryFromLoop(name string) ReActLoopOption
```

把另一个已注册的 loop（`RegisterLoopFactory` 注册过的）作为本 loop 的一个 action。运行时如果 LLM 选了这个 action，就会创建子 loop 并对**同一个 task** 调 `ExecuteWithExistedTask`。

**典型用途**：把高频意图识别 / 知识查询作为子能力嵌入主 loop。

源码 [options.go:288-298](../options.go) + [action_from_loop.go:11-69](../action_from_loop.go)。

### `WithActionFilter(filter func(*LoopAction) bool)`

```go
func WithActionFilter(filter func(action *LoopAction) bool) ReActLoopOption
```

在 `generateSchemaString` 渲染 schema 时过滤 actions。返回 `false` 的不出现在 schema 里，但仍在 `r.actions` 中（**所以已经注入的 action 还能被代码内部触发**）。

## 2.5 流式字段

### `WithAITagField(tagName, variableName string)`

```go
func WithAITagField(tagName, variableName string) ReActLoopOption
```

注册 AI tag 字段。当 LLM 输出包含 `<TAG>...</TAG>`（实际语法是 `<|TAG_nonce|>...<|TAG_END_nonce|>`），内容会被提取并存入 `loop.Get(variableName)`。

### `WithAITagFieldWithAINodeId(tagName, variableName, nodeId, contentType...)`

```go
func WithAITagFieldWithAINodeId(tagName, variableName, nodeId string, contentType ...string) ReActLoopOption
```

升级版：还会在内容流式产生时，实时推送到指定 NodeId（前端可以挂载到该节点显示）。`contentType` 用于告诉 UI 怎么渲染（`http_flow`、`text_markdown`、`yak_code` 等）。

[loop_http_fuzztest/init.go:42-43](../loop_http_fuzztest/init.go)：

```go
reactloops.WithAITagFieldWithAINodeId("GEN_PACKET", generatedPacketContentField, "http_flow", aicommon.TypeCodeHTTPRequest),
reactloops.WithAITagFieldWithAINodeId("GEN_MODIFIED_PACKET", modifiedPacketContentField, "http_flow", aicommon.TypeCodeHTTPRequest),
```

## 2.6 能力开关

每个开关都有两种形式：

```go
WithAllowXxx(b ...bool)         // 简化形式
WithAllowXxxGetter(f func() bool) // 完整形式（运行时可变）
```

| 选项 | 作用 |
|------|------|
| `WithAllowRAG` | 是否启用 RAG（决定是否注册 `knowledge_enhance` action） |
| `WithAllowAIForge` | 是否允许 AI 蓝图（`require_ai_blueprint`） |
| `WithAllowPlanAndExec` | 是否允许任务规划与执行（`request_plan_execution`） |
| `WithAllowToolCall` | 是否允许工具调用（`require_tool` / `directly_call_tool`） |
| `WithAllowUserInteract` | 是否允许向用户提问（`ask_for_clarification`） |

`Getter` 形式的好处：可以在运行时动态切换。如异步任务期间禁用 plan：

```go
WithAllowPlanAndExecGetter(func() bool {
    return !r.HasRunningAsyncTask()
})
```

### `WithToolsGetter(getter func() []*aitool.Tool)`

```go
func WithToolsGetter(getter func() []*aitool.Tool) ReActLoopOption
```

在 prompt 的 `Tools` / `TopTools` 段渲染工具列表时使用。

## 2.7 记忆

### `WithMemoryTriage(triage aicommon.MemoryTriage)`

注入记忆检索器。运行时 `memoryTriage.SearchMemory(task, sizeLimit)` 异步运行，结果通过 `r.PushMemory` 加入到 `currentMemories`。

### `WithMemoryPool(pool *omap.OrderedMap[string, *aicommon.MemoryEntity])`

直接共享一个外部记忆池。多个 loop 可以共用记忆。

### `WithMemorySizeLimit(sizeLimit int)`

记忆池字节上限。**默认 10 KB**（`if sizeLimit <= 0`）。

## 2.8 反思与自旋

### `WithEnableSelfReflection(enable ...bool)`

```go
func WithEnableSelfReflection(enable ...bool) ReActLoopOption
```

启用自我反思。默认不启用。开启后每次 action 执行后按策略触发 0~5 级反思。详见 [08-determinism-mechanisms.md](08-determinism-mechanisms.md)。

### `WithSameActionTypeSpinThreshold(n int)`

设置同 action type 自旋阈值。**默认 3**：连续 3 次相同 type 触发简单自旋检测。

### `WithSameLogicSpinThreshold(n int)`

设置 AI 深度自旋检测阈值。**默认 3**：达到此阈值后用 LiteForge 调 AI 判断是否真的是逻辑层面自旋（同 type 不同参数可能不算）。

### `WithMaxConsecutiveSpinWarnings(n int)`

最大允许的连续自旋警告数，超过则强退。**默认 3**。设为 0 禁用强退。

### `WithUseSpeedPriorityAICallback(b ...bool)`

让主循环用 `config.CallSpeedPriorityAI` 而不是 `config.CallAI`。子 loop 通常用这个降低延迟。

## 2.9 技能（Skills）

### `WithSkillLoader(loader, managerOpts...)`

```go
func WithSkillLoader(loader aiskillloader.SkillLoader, managerOpts ...aiskillloader.ManagerOption) ReActLoopOption
```

设置技能加载器，自动创建 `SkillsContextManager`，并自动配置：

```go
r.allowSkillLoading = mgr.HasRegisteredSkills
r.allowSkillViewOffset = mgr.HasTruncatedViews
```

### `WithSkillsContextManager(mgr)`

直接传入已配置好的 manager（用于多个 loop 共享技能上下文）。

## 2.10 Extra Capabilities / Perception

### `WithExtraCapabilities(ecm *ExtraCapabilitiesManager)`

```go
func WithExtraCapabilities(ecm *ExtraCapabilitiesManager) ReActLoopOption
```

注入自定义的 `ExtraCapabilitiesManager`。不设置时 `NewReActLoop` 会自动创建一个 `MaxExtraTools=50` 的默认实例。

详见 [09-capabilities.md](09-capabilities.md)。

### `WithDisableLoopPerception(disable ...bool)`

```go
func WithDisableLoopPerception(disable ...bool) ReActLoopOption
```

关闭本 loop 的感知层（`r.perception = nil`）。

**专用于轻量子 loop**（如 `loop_intent`），它们应该不做感知评估。区别于 `aicommon.WithDisablePerception`（config 级全局开关）。

### `WithToolCallIntervalReviewExtraPrompt(prompt string)`

```go
func WithToolCallIntervalReviewExtraPrompt(prompt string) ReActLoopOption
```

工具长时间运行期间的间隔审查 prompt 中加额外指令。会写入 `aicommon.Config.ConfigKeyToolCallIntervalReviewExtraPrompt`。

## 2.11 批量配置

### `BasicAICommonConfigOption(c *aicommon.Config) []ReActLoopOption`

```go
func BasicAICommonConfigOption(c *aicommon.Config) []ReActLoopOption {
    return []ReActLoopOption{
        WithMemoryTriage(c.MemoryTriage),
        WithMemoryPool(c.MemoryPool),
        WithPeriodicVerificationInterval(int(c.PeriodicVerificationInterval)),
        WithMemorySizeLimit(int(c.MemoryPoolSize)),
        WithEnableSelfReflection(c.EnableSelfReflection),
    }
}
```

便捷函数：从一个 `*aicommon.Config` 一次性导出 5 个常用选项。在 loop 工厂里：

```go
preset := append(reactloops.BasicAICommonConfigOption(cfg), preset...)
```

## 2.12 LoopMetadata 选项（注册时）

下面这些**不是** `ReActLoopOption` 而是 `LoopMetadataOption`，在 `RegisterLoopFactory` 第三个参数处使用。源码 [register.go:25-75](../register.go)。

```go
WithLoopDescription(string)        // 英文描述（给 AI 选择 loop 用）
WithLoopDescriptionZh(string)      // 中文描述（前端展示）
WithLoopOutputExample(string)      // 该 loop 作为 action 时的输出示例
WithLoopUsagePrompt(string)        // 在 schema 的 x-@action-rules 里覆盖默认描述
WithLoopIsHidden(bool)             // 是否对用户隐藏
WithVerboseName(string)            // 英文展示名
WithVerboseNameZh(string)          // 中文展示名
```

[loop_http_fuzztest/init.go:120-160](../loop_http_fuzztest/init.go) 是完整示例：

```go
reactloops.WithLoopDescription("HTTP request fuzzing and response diff analysis for security testing"),
reactloops.WithLoopDescriptionZh("HTTP 请求模糊测试与响应差异分析"),
reactloops.WithVerboseName("HTTP Fuzz Testing"),
reactloops.WithVerboseNameZh("HTTP 模糊测试"),
reactloops.WithLoopUsagePrompt("Use this when ..."),
reactloops.WithLoopOutputExample(`...`),
```

## 2.13 进一步阅读

- [03-prompt-system.md](03-prompt-system.md)：`WithPersistentInstruction` / `WithReactiveDataBuilder` 注入到哪里
- [04-actions.md](04-actions.md)：所有 `WithRegisterLoopAction*` 的展开使用
- [05-hooks-and-lifecycle.md](05-hooks-and-lifecycle.md)：`WithInitTask` / `WithOnPostIteraction` 的实战
- [10-build-your-own-loop.md](10-build-your-own-loop.md)：把这些选项组合起来
