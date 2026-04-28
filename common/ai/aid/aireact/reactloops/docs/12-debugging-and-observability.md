# 12. 调试与可观测性

> 回到 [README](../README.md) | 上一章：[11-case-studies.md](11-case-studies.md) | 下一章：[13-yak-focus-mode.md](13-yak-focus-mode.md)

reactloops 内置了多层调试基础设施，让你可以**事后**查看每一轮的 prompt、action、感知、意图识别快照。本章梳理：

- workspace debug 总开关
- 调试产物目录结构
- prompt observation 与 prompt_profile 事件
- action 执行落盘
- timeline vs emitter 事件
- 测试基建
- 实战调试技巧

## 12.1 总开关：`YAKIT_AI_WORKSPACE_DEBUG`

源码 [workspace_debug.go:15-29](../workspace_debug.go)：

```go
const (
    envAIWorkspaceDebugPrimary   = "YAKIT_AI_WORKSPACE_DEBUG"
    envAIWorkspaceDebugSecondary = "AI_WORKSPACE_DEBUG"
)

func IsAIWorkspaceDebugEnabled() bool {
    // 任何一个环境变量被设置为真值（true / 1 / yes / on 等）即开启
}
```

### 开启方式

```bash
# 方式 1：导出环境变量
export YAKIT_AI_WORKSPACE_DEBUG=1
yak ./common/ai/aid/aireact/...

# 方式 2：单次执行
YAKIT_AI_WORKSPACE_DEBUG=1 yak test ./common/ai/aid/aireact/reactloops/...

# 方式 3：兼容老变量
AI_WORKSPACE_DEBUG=true yak ...
```

### 影响范围

开启后会：

1. 落盘 prompt 到 `<workdir>/debug/prompt/`
2. 落盘 perception 快照到 `<workdir>/debug/perception/`
3. 落盘 intent 识别结果到 `<workdir>/debug/intent/`
4. action 执行记录会包含完整 prompt（默认只有参数）
5. 控制台打 prompt section build report

## 12.2 调试产物目录结构

`<workdir>` 由 `cfg.GetOrCreateWorkDir()` 决定，一般为 `~/yakit/aiworkspace/<runtime_id>/`。

```text
<workdir>/
├── task_<task_index>/
│   ├── loop_<loop_name>_action_calls/
│   │   ├── 1_set_http_request.md       # 第 1 轮的 action 调用记录
│   │   ├── 2_fuzz_path.md
│   │   └── 3_directly_answer.md
│   └── ... 
└── debug/
    ├── prompt/
    │   └── prompt_<timestamp>_<nanos>.md
    ├── perception/
    │   └── perception_epoch_3_<timestamp>_<nanos>.md
    └── intent/
        └── intent_<timestamp>_<nanos>.md
```

### Action 调用记录（每轮 1 个文件）

源码 [exec.go:1120-1169](../exec.go) `emitActionExecutionRecord` + `buildActionExecutionMarkdown`：

```markdown
# Action Call Record

## Action

- Name: fuzz_path
- Human Readable Thought: 我准备对路径做模糊测试

## Params

```json
{
  "path_payload": "../../etc/passwd",
  "method": "GET"
}
```

## Prompt    （只在 debug 模式下包含）

```
（完整的本轮 prompt）
```
```

### Perception 快照

源码 [workspace_debug.go:162-210](../workspace_debug.go)：

```markdown
# Perception Debug

- Generated At: 2024-01-01 10:00:00
- Loop Name: http_fuzztest
- Epoch: 3
- Trigger: post_action
- Changed: true
- Confidence: 0.8500

## Summary

User is testing SQL injection on the login endpoint

## Topics

sql_injection, login, authentication

## Keywords

login, password, sql, injection

## Capability Search Input

Query: sql injection login

## Capability Search Results

（完整的 markdown 报告）
```

### Intent 识别快照

源码 [workspace_debug.go:112-160](../workspace_debug.go)：包含 IntentAnalysis、Recommended Tools/Forges、Context Enrichment、Matched Names 等。

## 12.3 Prompt Observation 与 `prompt_profile` 事件

### 数据结构

每轮 prompt 渲染后，`prompt_observation.go` 把 prompt 拆段统计：

| 段落 | 来源 |
|------|------|
| Background | `getRenderInfo` 系统注入 |
| UserQuery | `task.GetUserInput()` |
| PersistentContext | `WithPersistentInstruction` 渲染结果 |
| ReactiveData | `WithReactiveDataBuilder` 渲染结果 |
| InjectedMemory | memory pool 拉取 |
| Schema | actions 自动生成 |
| OutputExample | `WithReflectionOutputExample` |
| ExtraCapabilities | `ExtraCapabilitiesManager.Render` |
| SessionEvidence | verification 留存的 evidence |
| SkillsContext | skills loader 注入 |

每段计算字节数和 token 数。

### 事件发出

```go
status := observation.BuildStatus(1 * 1024)
r.SetLastPromptObservationStatus(status)
r.emitPromptObservationStatus(status)
// → emitter.EmitPromptProfile(status)
// → emit EVENT_TYPE_PROMPT_PROFILE 到前端
```

### 前端用途

前端可以可视化每段 prompt 的 token 占比，帮助：

- 找出膨胀段（比如 timeline 太长）
- 优化 prompt 控制成本
- 看 ExtraCapabilities 是否正确注入

### CLI 报告

debug 模式下还会在控制台打印：

```text
prompt section build report:
+--------------------+--------+--------+
| Section            | Bytes  | Tokens |
+--------------------+--------+--------+
| Background         |   1024 |    256 |
| UserQuery          |    256 |     64 |
| PersistentContext  |   2048 |    512 |
| ReactiveData       |   3072 |    768 |
| Schema             |   4096 |   1024 |
| ...                |        |        |
| TOTAL              |  16384 |   4096 |
+--------------------+--------+--------+
```

源码 [prompt.go:243-248](../prompt.go)。

## 12.4 Timeline vs Emitter 事件

两套机制经常被混淆，要区分清楚：

| 维度 | Timeline | Emitter Event |
|------|----------|---------------|
| 调用 | `invoker.AddToTimeline(name, data)` | `emitter.EmitXxx(...)` |
| 持久化 | 注入到下一轮 prompt 的 Background 段 | 通过 baseEmitter 发到 UI / 数据库 |
| 用途 | LLM 上下文记录 | UI 实时反馈 / 持久化历史 |
| 受众 | LLM 自己 | 用户 / 数据库 |
| 形态 | 字符串（被 markdown 化） | 结构化事件（含 NodeId / type / contentType） |

### 何时用 Timeline

- LLM 需要"记住自己做过什么"：`AddToTimeline("anomaly_found", data)`
- action 失败要让 LLM 反思：`AddToTimeline("error", reason)`
- 关键状态变化：`AddToTimeline("phase_switched_to_2", "from_phase_1")`

### 何时用 Emitter Event

- 用户需要看到的进度：`EmitThoughtStream`
- 关键产物：`EmitFileArtifactWithExt`
- 状态指示：`EmitStatus("loadingStatus", "...")`
- 最终结果：`EmitResultAfterStream`

### 一般同时用

```go
invoker.AddToTimeline("found_vulnerability", evidence)        // 给 LLM 看
invoker.GetEmitter().EmitYakitRisk(id, title, ...)            // 给用户看
invoker.EmitFileArtifactWithExt("vuln_report", ".md", report) // 给用户看
```

## 12.5 调试产物的命名规则

源码 [workspace_debug.go:78-98](../workspace_debug.go) `sanitizeAIWorkspaceDebugName`：

```go
// 把名字小写化、空格 / 路径分隔符 / 特殊字符替换为 _
name = strings.ReplaceAll(name, " ", "_")
name = strings.ReplaceAll(name, "/", "_")
// ... 等等
name = strings.Trim(name, "_")
```

文件名格式：`<prefix>_<YYYYMMDD>_<HHMMSS>_<nanos>.md`

示例：`perception_epoch_3_20240101_100000_1704067200000000000.md`

时间戳的纳秒精度避免同秒内多次写入冲突。

## 12.6 Test 基建

### Mock AI Callback

reactloops 测试用 `aicommon.NewAIResponseFromMockedContent` 构造 AI 响应：

```go
import (
    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/ai/aid/aireact"
    _ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"
)

aiCallback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
    return aicommon.NewAIResponseFromMockedContent(`{"@action":"finish","human_readable_thought":"done"}`), nil
}

react, _ := aireact.NewReAct(
    aireact.WithAICallback(aiCallback),
    aireact.WithEnterFocusMode("default"),
    aireact.WithDisableIntentRecognition(true),  // 测试中关闭意图识别（共享 callback 会消耗 mock）
)

err := react.Invoke(ctx, "test query")
```

### 用 callCount 区分轮次

```go
callCount := 0
aiCallback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
    callCount++
    switch callCount {
    case 1:
        return aicommon.NewAIResponseFromMockedContent(`{"@action":"step1"}`), nil
    case 2:
        return aicommon.NewAIResponseFromMockedContent(`{"@action":"finish"}`), nil
    default:
        return aicommon.NewAIResponseFromMockedContent(`{"@action":"finish"}`), nil
    }
}
```

### 检查 Timeline / 状态

```go
summary := react.GetTimelineSummary()
if !strings.Contains(summary, "anomaly_found") {
    t.Error("expected anomaly_found in timeline")
}
```

### 检查 Emitter Event

捕获 emitter 事件：

```go
var events []*schema.AiOutputEvent
react, _ := aireact.NewReAct(
    aireact.WithEventCallback(func(e *schema.AiOutputEvent) {
        events = append(events, e)
    }),
    aireact.WithAICallback(aiCallback),
)
// 检查 events
```

参考真实测试：
- [common/ai/aid/aireact/reactloops/reactloop_test.go](../reactloop_test.go)
- [common/ai/aid/aireact/reactloops/loop_http_fuzztest/directly_answer_test.go](../loop_http_fuzztest/directly_answer_test.go)
- [common/ai/aid/aireact/reactloops/perception_test.go](../perception_test.go)

## 12.7 实战调试技巧

### 技巧 1：先关意图识别再测主 loop

测试中 `DisableIntentRecognition=true` 避免子 loop 消耗 mock 响应：

```go
aireact.WithDisableIntentRecognition(true)
```

否则 `loop_default → loop_intent → loop_default` 的链路会让 mock 顺序错位。

### 技巧 2：用 desc / log 看运行时状态

在 action handler 里：

```go
ActionHandler: func(loop *ReActLoop, action *aicommon.Action, op *LoopActionHandlerOperator) {
    log.Infof("[debug] action params: %+v", action.GetParams())
    log.Infof("[debug] loop state: %s", loop.Get("my_key"))
    log.Infof("[debug] timeline: %s", loop.GetInvoker().GetTimelineSummary())
    // ...
}
```

### 技巧 3：开 workspace debug 看 prompt 完整内容

```bash
YAKIT_AI_WORKSPACE_DEBUG=1 go test -run TestMyLoop ./common/ai/aid/aireact/reactloops/loop_xxx/...
```

测试结束后看：

```bash
ls ~/yakit/aiworkspace/*/task_*/loop_xxx_action_calls/
cat ~/yakit/aiworkspace/*/task_*/loop_xxx_action_calls/1_*.md
```

### 技巧 4：临时强制反思看 LLM 自评

```go
op.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
op.Continue()
```

下一轮 prompt 会包含 LLM 自己写的反思 → 复盘哪里出错。

### 技巧 5：用 timeline 替代 print

reactloops 的 timeline 是结构化的、被 LLM 看到、被持久化的"日志"。**不要用 print 调试，用 timeline**：

```go
invoker.AddToTimeline("[DEBUG]", map[string]any{
    "checkpoint": "before fuzz",
    "current_request": currentRequest,
    "iteration": loop.GetCurrentIterationIndex(),
})
```

后续 LLM 推理时就能看到这个调试信息，更有利于产生正确决策。

### 技巧 6：用 `EmitPromptProfile` 看 token 分布

如果某 loop 总是超 context，开启 debug 模式看每段占比，找出膨胀段裁剪。

常见膨胀源：

- timeline 没有 prune（每轮都加，永不清）
- skill loader 加载了太多 SKILL.md
- ExtraCapabilities 没限制 50 个工具

### 技巧 7：spin 触发了但应用层不清楚？

```go
// 在 ActionHandler 之外的地方
isSpin, result := loop.IsInSpin()
if isSpin {
    log.Warnf("spin detected: %+v", result)
}
```

或者主动触发 perception 强制更新：

```go
loop.ForcePerceptionUpdate("manual_check")
```

### 技巧 8：观察事件流过滤特定 NodeId

```go
react, _ := aireact.NewReAct(
    aireact.WithEventCallback(func(e *schema.AiOutputEvent) {
        if e.NodeId == "re-act-loop-thought" {
            fmt.Printf("[THOUGHT] %s\n", string(e.StreamDelta))
        }
    }),
)
```

只看思考流，过滤掉所有其他噪声。

### 技巧 9：把 debug markdown 提交到 issue

实际遇到 LLM 跑偏，把：

1. `prompt_<...>.md` 完整文件
2. 几个 `<iter>_<action>.md`
3. timeline summary

打包发给团队，能极大加速排错。

### 技巧 10：用 `desc` 自省（运行时）

```go
desc(loop)         // 看 loop 的所有方法和字段
desc(loop.GetInvoker())  // 看 invoker
```

注：这是 yak 脚本里的特性。Go 测试里用 reflect 或 print struct。

## 12.8 常见问题排查

### Q1：LLM 总是不按 schema 输出

- 看 `iter_<N>.md` 里的 prompt，确认 schema 段确实存在
- 看 LLM 原始输出（debug 模式 emitter 会打印）
- 检查 `WithReflectionOutputExample` 的示例是否清楚
- 试试更换 model（speed → quality）

### Q2：流式输出卡住

- 检查 `LoopAITagField` 的 TagName 是否和 prompt 里的 `<|TAG_<nonce>|>` 模板一致
- 检查 `LoopStreamField.FieldName` 拼写
- 看 `chanx.UnlimitedChan` 是否被关闭（一般是上游异常）

### Q3：finalize fallback 没触发

- 确认 `OnPostIteraction` 注册了
- 确认 `isDone == true` 才执行
- 确认 deliver 状态守门变量名拼写正确（`loop.Get("xxx_delivered")`）

### Q4：spin 检测过于敏感

- 调高 `WithSameActionTypeSpinThreshold`
- 关注 `MaxConsecutiveSpinWarnings` 阈值

### Q5：感知（perception）总是不更新

- `WithDisableLoopPerception(false)` 确认开启
- 看 `state.LastTrigger` 是否触发
- `state.Changed` 为 false 时不会更新（topics 哈希一样）
- 主动 `ForcePerceptionUpdate` 跳过节流

### Q6：测试用 mock callback 报"insufficient mock responses"

- 数 mock 响应数量是否覆盖所有 LLM 调用
- 加 `WithDisableIntentRecognition(true)` 减少子 loop 调用
- 加 `WithEnableSelfReflection(false)` 减少反思调用

### Q7：debug 文件没生成

- 确认环境变量正确：`echo $YAKIT_AI_WORKSPACE_DEBUG`
- 确认 `cfg.GetOrCreateWorkDir()` 返回非空
- 看日志：`failed to create ai workspace debug dir`

## 12.9 进一步阅读

- [01-architecture.md](01-architecture.md)：主循环 → 知道每个调试点对应哪个执行阶段
- [03-prompt-system.md](03-prompt-system.md)：prompt observation 详解
- [05-hooks-and-lifecycle.md](05-hooks-and-lifecycle.md)：怎么在 hook 里加调试
- [08-determinism-mechanisms.md](08-determinism-mechanisms.md)：感知 / 反思 / 自旋的产物在 debug 目录哪里
- 源码：
  - [workspace_debug.go](../workspace_debug.go)
  - [prompt_observation.go](../prompt_observation.go)
  - [exec.go](../exec.go)（`emitActionExecutionRecord`）
