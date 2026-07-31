# Timeline 优化任务指导

## 背景

Timeline 是 ReAct Agent 的核心记忆载体，所有历史动作、工具调用结果、用户交互、反思都会 push 到 timeline，并随 prompt 一起送入 LLM。当前 timeline 存在两类冗余问题：

1. **调用层重复 add**：同一次迭代/工具调用会产生多条信息重叠的 timeline item，占用了不必要的 token。
2. **dump 层渲染冗余**：单条 timeline item 渲染时含有重复 prefix、无意义字段、对 LLM 无信息量的标记行，纯消耗 token。

优化原则：**不改变 timeline 的数据结构与缓存机制，只在调用方减少冗余 push、在渲染方去除单条内的冗余内容。**

---

## 一、调用层：减少冗余 timeline add

> 目标：识别并消除/合并"同一次操作产生的信息高度重叠的多条 AddToTimeline 调用"。
> 注意：不同迭代的历史条目必须保留，不能跨条目合并（那会破坏缓存稳定性）。

### 1.1 已识别的冗余 add 点

#### A. `[DIRECT_CALL_PARAMS]` 与 ToolResult 的参数重复

**位置**：`common/ai/aid/aireact/reactloops/loopinfra/action_directly_call_tool.go:527`
```go
reportStatus(formatDirectlyCallToolParamsTimeline(toolName, params, mergedBlockParams))
```
**问题**：directly_call_tool 路径会先 `AddToTimeline("[DIRECT_CALL_PARAMS]", ...)` 打印完整参数，随后 `PushToolResult(result)` 时 ToolResult 自身的 `dumpTimelineParams` 又会输出一遍 param。虽然已有 `OmitParamsInTimeline` 标志位（在 `invoke_toolcall.go:308` 设置），但仅对 directly_call 路径生效，且 fallback 分支会重置为 false（`toolcall.go:726`），导致参数仍可能重复出现。

**方案**：
- 确保 directly_call_tool 成功路径的 `OmitParamsInTimeline` 始终为 true，不因 fallback 而重置（fallback 路径已产生新的 params timeline item，ToolResult 的 param 应省略）。
- 复核 `WithToolCaller_OmitResultParamsInTimeline` 在所有 directly_call 入口都正确传入。

#### B. `intent_init` / `init_done` 两条低信息量条目

**位置**：
- `common/ai/aid/aireact/reactloops/loop_intent/init.go:167` → `AddToTimeline("intent_init", "Intent recognition loop initialized for deep analysis")`
- `common/ai/aid/aireact/reactloops/exec.go:638` → `AddToTimeline("init_done", "ReActLoop[...] init handler completed task early")`

**问题**：这两条都是纯状态标记，对 LLM 决策无信息增量——"意图识别循环已初始化"和"init handler 提前完成"对后续推理没有帮助。

**方案**：
- `intent_init`：降级为 log.Infof，不再 AddToTimeline（意图识别的结果已有 `intent_analysis` / `intent_recommended_tools` 等条目承载实质内容）。
- `init_done`：降级为 log.Infof；若需保留早退信号，合并到下一条 `current task user input` 条目的内容中作为前缀注释，而非独立条目。

#### C. `intent_context_enrichment` 无信息量条目

**位置**：`common/ai/aid/aireact/reactloops/deep_intent.go:118`
```go
r.AddToTimeline("intent_context_enrichment", "已补充能力上下文")
```
**问题**：内容固定为"已补充能力上下文"，不携带任何实质信息（补充了什么上下文？），对 LLM 无用。

**方案**：
- 如果有实质 enrichment 内容，改为输出实际内容摘要。
- 如果无实质内容可展示，直接移除该 AddToTimeline，降级为 log.Infof。

#### D. `iteration` 条目的冗余尾巴

**位置**：`common/ai/aid/aireact/reactloops/exec.go:1013, 1046`
```go
r.GetInvoker().AddToTimeline("iteration",
    fmt.Sprintf("[%v]ReAct Iteration Done[%v] max:%v continue to next iteration", loopName, iterationCount, maxIterations))
```
**问题**：每次迭代结束都会额外 push 一条 `ReAct Iteration Done ... continue to next iteration`。下一条迭代开头的 `ReAct iteration N+1` 已经隐含了"上一轮已结束"。

**方案**：
- 移除 `continueIter()` 中的这条 AddToTimeline。
- 保留开头的 `ReAct iteration N` 条目（携带 Reason/Next-Step，有信息量）。

#### E. `reflection` 与 `logic_spin_warning` 的内容重叠

**位置**：
- `common/ai/aid/aireact/reactloops/reflection.go:509` → `AddToTimeline("logic_spin_warning", ...)` 含 Suggestions
- `common/ai/aid/aireact/reactloops/reflection_memory.go:96` → `AddToTimeline("reflection", ...)` 也含 MANDATORY RECOMMENDATIONS（同样的 Suggestions）

**问题**：当 SPIN 检测触发时，`logic_spin_warning` 和随后的 `reflection` 会输出几乎相同的 Suggestions 列表（从 timeline 示例可见 6 条建议重复出现两次）。

**方案**：
- SPIN 场景下，`reflection` 条目只保留执行结果摘要（action / iteration / 耗时 / level），不再重复 Suggestions（已由 `logic_spin_warning` 承载）。
- 可在 `addReflectionToTimeline` 中判断 `reflection.IsSpinning`，若 true 则跳过 Suggestions 部分。

### 1.2 待排查项

- `perception` 条目（`perception.go:938`）每次 post-action 都触发，需确认频率是否过高。
- `intent_recommended_forges`（`deep_intent.go:114`）在无 forge 匹配时是否仍输出空条目。

---

## 二、Dump 层：渲染时去除单条冗余内容

> 目标：在 `renderTimelineEntry` / `ToolResult.String()` 等渲染函数中，去除单条 item 内的重复 prefix、无意义字段。
> **红线：禁止对工具自身的 result data 做任何清洗/截断/裁剪——工具产出的内容由工具自己负责，timeline dump 只处理"框架拼装的包装层"。**

### 2.1 已识别的渲染冗余

#### A. ToolResult 重复 prefix：`data:` / `COMBINED OUTPUT:` / `RESULT:`

**位置**：`common/ai/aid/aitool/tool_invoke_result.go:101-179` (`dumpTimelineResult`)

**问题**：ToolResult.Data 经过 `normalizeToolResultData`（`toolcall_artifact.go:451`）已被包装为：
```
COMBINED OUTPUT:
{工具原始输出}

RESULT:
{结果}

HINT:
{artifacts 路径提示}
```
这是一层框架包装。当 `dumpTimelineResult` 渲染时，又在外层加 `data:\n` 前缀：
```
data:
COMBINED OUTPUT:
{工具原始输出}
...
```
`data:` 和 `COMBINED OUTPUT:` 语义重复，都是"这是工具输出"。

**方案**：
- 在 `dumpTimelineResult` 中，当 Data 为 string 且以 `COMBINED OUTPUT:` 开头时，直接输出该 string，不再追加 `data:\n` 前缀。
- 这只是去掉框架自己的重复包装，不触碰 `COMBINED OUTPUT:` 下面的工具原始内容。

#### B. HINT 段落的冗余路径行

**位置**：`common/ai/aid/aicommon/toolcall_artifact.go:401-408` (`toolArtifactHint`)

**问题**：HINT 段落输出 4 行 artifact 路径：
```
HINT:
Complete tool output is stored in artifacts:
- combined output: /long/path/.../combined_output.txt
- stdout:          /long/path/.../stdout.txt
- stderr:          /long/path/.../stderr.txt
- result:          /long/path/.../result.txt
Use grep first, or read_file(file="/long/path/.../combined_output.txt", ...).
Do not load or cat the complete artifact unless necessary.
```
- stdout/stderr 路径对 LLM 几乎无用（combined_output 已含两者）。
- 路径完整展开非常占 token，且 `read_file` 示例又重复了一遍 combined_output 路径。

**方案**（只改框架拼接，不改工具产出）：
- 只保留 combined_output 路径与 result 路径（省去 stdout/stderr 两行）。
- `read_file(file=...)` 中的路径与上一行 `- combined output:` 的路径重复，改为用相对引用：`read_file(file=<combined output path above>, ...)` 或直接省去重复路径，只写 `read_file(file=同上, mode="lines", offset=..., lines=...)`。

#### C. `RESULT: (empty)` 无信息量行

**位置**：`common/ai/aid/aicommon/toolcall_artifact.go:445-449`

**问题**：当 result 为空时输出 `RESULT:\n(empty)`，占两行但信息量为零。

**方案**：
- 当 `resultText == ""` 时，不输出 `RESULT:` 段落（省去 `RESULT:\n(empty)` 两行）。
- `combined == resultText` 时已有 `[duplicate of COMBINED OUTPUT omitted]` 逻辑，可同样跳过 RESULT 段落。

#### D. `call_expectations` 字段冗余输出

**位置**：`common/ai/aid/aitool/tool_invoke_result.go:62-64`
```go
if t.CallExpectations != "" {
    fmt.Fprintf(buf, "call_expectations: %s\n", t.CallExpectations)
}
```
**问题**：`call_expectations` 是工具调用前的期望描述（如 "~3s, fallback to require_tool if params are uncertain"），在工具结果已完成后对 LLM 无决策价值。

**方案**：
- 在 timeline 渲染路径（`DumpTimelineItem`）中跳过 `call_expectations` 输出。
- 保留在 ToolResult 结构体中（审计/调试用），仅 timeline dump 不展示。

#### E. `shrink_result:` / `shrink_similar_result:` 标记行

**位置**：`common/ai/aid/aitool/tool_invoke_result.go:104-107`

**问题**：压缩后的 ToolResult 渲染时输出 `shrink_result: "..."`，该 prefix 标记对 LLM 无意义。

**方案**：
- 当使用 ShrinkResult 时，直接输出 shrink 内容本身，不加 `shrink_result: ` 前缀。

### 2.2 红线

- **禁止**在 dump 层对 `ToolResult.Data`（工具原始产出）做截断、清洗、去重。`shrinkBodyWithStats` / `enforceCanonicalToolResultLimit` 等保护性处理属于工具结果 finalize 阶段，不在 dump 渲染职责内。
- **禁止**跨条目合并或折叠 timeline item（破坏缓存）。
- 渲染优化只针对：框架包装层（`data:` 前缀、`COMBINED OUTPUT:`/`RESULT:` 标签、HINT 段落、`call_expectations`、`shrink_result:` 标记等），不针对工具内容本身。

---

## 三、实施顺序

1. **dump 层优化先行**（低风险、不改变数据流）：
   - [ ] `dumpTimelineResult`：去掉与 `COMBINED OUTPUT:` 重复的 `data:` 前缀
   - [ ] `toolArtifactHint`：精简 HINT 段落，去除 stdout/stderr 路径与重复路径
   - [ ] `normalizeToolResultData`：result 为空时跳过 RESULT 段落
   - [ ] `DumpTimelineItem`：跳过 `call_expectations` 输出
   - [ ] `dumpTimelineResult`：shrink 内容不加 `shrink_result:` 前缀

2. **调用层优化跟进**（需逐点验证信息流）：
   - [ ] `intent_init` 降级为 log
   - [ ] `init_done` 降级为 log
   - [ ] `intent_context_enrichment` 移除或输出实质内容
   - [ ] `continueIter()` 移除 `ReAct Iteration Done` 条目
   - [ ] SPIN 场景 `reflection` 不重复 Suggestions
   - [ ] 复核 `OmitParamsInTimeline` 在 fallback 路径的行为

3. **验证**：
   - [ ] 跑一轮 CTF / HTTP fuzz 场景，对比优化前后 timeline dump 的 token 量
   - [ ] 确认缓存命中率未下降（StableNonce / StableKey 不受影响）
   - [ ] 确认工具结果完整性未被改变（diff 工具原始输出）
