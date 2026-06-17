# 17. Stream Helpers 使用指导

> 适用范围：Go 实现的专注模式，尤其是 `loop_http_flow_analyze` 这类会查询、匹配、加载和保存大量中间材料的循环。

`stream_helpers.go` 提供了一层更窄的前端输出 API，用来约束专注模式的可见输出。它的目标不是替代所有 emitter 能力，而是把常见输出收敛到三种稳定形态：

1. 瞬时状态：告诉用户当前正在做什么。
2. 累积 Action 日志：给每个 Action 留下简短、可读、有限的过程记录。
3. 文件产物：把大体量材料落盘并 pin 到前端，避免刷屏。

## API 速查

### `EmitStatus`

```go
reactloops.EmitStatus(loop, "查询流量中 / Querying Flows...")
```

用于状态栏覆盖显示。状态会被后续状态替换，所以只放“现在正在做什么”，不要放详情、原因链、调试数据或完整结果。

要求：

- 文案必须中英双语，格式建议为 `中文 / English`。
- 内容应短，推荐不超过一行。
- 适合阶段切换：准备、执行中、收集结果、生成报告、完成、失败。
- 不要把 `human_readable_thought`、prompt、SQL、HTTP 原文或异常堆栈放进状态。

### `EmitProgress`

```go
reactloops.EmitProgress(loop, current, total, "匹配进度", "Matching")
```

用于已知总数的进度状态，输出格式为：

```text
匹配进度 42% (420/1000) / Matching 42% (420/1000)
```

注意：`total <= 0` 时函数会直接返回。未知总量的流式扫描不要传 `0` 伪装进度，应改用节流后的 `EmitStatus`，例如每 100 条覆盖一次：

```go
if totalCount > 0 && totalCount%100 == 0 {
    reactloops.EmitStatus(loop, fmt.Sprintf(
        "已扫描 %d 条流量 / Scanned %d Flows",
        totalCount, totalCount,
    ))
}
```

### `EmitActionLog`

```go
reactloops.EmitActionLog(loop, "http-flow-query", "查询 keyword=login, limit=30")
reactloops.EmitActionLog(loop, "http-flow-query", "完成: 找到 18 条流量", summary)
```

用于输出某个 Action 的累积日志。它会通过 `EmitDefaultStreamEvent` 写到指定 `nodeId`，并可把大段详情作为 reference material 关联到本次流。

推荐模式是“两段式”：

1. Action 开始时发一条短摘要，说明用户可理解的操作对象和参数。
2. Action 完成时发一条短结果，并把完整摘要作为 `reference` 传入。

要求：

- `nodeId` 必须稳定，使用 kebab-case，例如 `http-flow-query`、`http-flow-match`、`http-flow-detail`、`fuzz-test`。
- `lines` 是用户可见日志，必须短而确定；不要逐条 flow 输出，不要发循环内 debug。
- `reference` 可放较长的查询结果、匹配摘要、证据材料，但仍应经过长度控制或文件化。
- 不要把 `human_readable_thought` 直接拼进 `lines`。LLM reasoning 已由框架处理，Action 日志只记录事实动作。

### `SaveAndPinFile`

```go
filename := filepath.Join(loop.GetLoopContentDir("data"), "match_result.txt")
if err := reactloops.SaveAndPinFile(loop, filename, []byte(fullSummary)); err != nil {
    log.Warnf("failed to save match result: %v", err)
}
```

用于把完整内容保存到文件，并通过 `EmitPinFilename` 让前端展示可点击文件。

适用场景：

- HTTP flow 列表、匹配详情、原始请求响应、fuzz 结果等可能很长的材料。
- 需要给模型保留摘要，但不希望前端直接被大段文本刷屏。
- 后续用户可能需要打开完整材料复查。

推荐做法：

- 完整内容写文件。
- 前端和 `operator.Feedback` 只放压缩预览、计数和文件路径。
- 文件名包含业务前缀、迭代号或时间，便于追踪。

## `loop_http_flow_analyze` 的推荐输出结构

每个 Action 按以下顺序组织输出：

```go
reactloops.EmitActionLog(loop, nodeId, startLine)
reactloops.EmitStatus(loop, "执行中 / Running...")

// 执行业务逻辑，技术细节只写 log.Infof/Warnf/Errorf。

if len(fullSummary) > maxHTTPFlowSummaryBytes {
    summary = buildPreviewWithFilename(fullSummary, filename)
}
_ = reactloops.SaveAndPinFile(loop, filename, []byte(fullSummary))

reactloops.EmitStatus(loop, "完成 / Complete")
reactloops.EmitActionLog(loop, nodeId, finishLine, summary)
operator.Feedback(feedbackMsg)
```

对照现有代码：

- `query_http_flows`：`http-flow-query`，开始输出查询参数，结束输出命中数量，详情写文件。
- `match_flows` / `match_flows_simple`：`http-flow-match`，开始输出匹配条件，结束输出匹配比例，详情写文件。
- `get_http_flow_detail`：`http-flow-detail`，开始输出定位符，结束输出请求/响应大小，详情作为 reference。
- `dispatch_fuzz_test`：`fuzz-test`，开始输出目标 flow 与漏洞类型，结束输出结果摘要，完整 fuzz 结果合并到 evidence。

## 什么时候不要用这些 helper

- 最终答案流：继续使用 `StreamField`、`AITagField`、`EmitResultAfterStream` 或 finalize 中的 markdown stream。
- 特殊内容类型：HTTP 包、Yak 代码、Markdown 正文等需要特定 `ContentType` 的流，仍使用专门的 stream field 或 emitter 方法。
- 技术日志：数据库错误、内部 fallback、模型 prompt 长度、执行耗时等只写 `log.*`。
- 模型下一轮观察：`operator.Feedback` 是给 ReAct 下一轮看的，不是前端展示通道；内容也要限长。

## 审查清单

新增或修改专注模式 Action 时逐项检查：

- 是否只有 1 条开始 Action 日志和 1 条完成 Action 日志。
- 状态是否中英双语、短句、可覆盖。
- 是否没有把 `human_readable_thought`、prompt、原始调试信息输出到前端。
- 循环中是否没有无节制 emit；需要进度时是否有节流。
- 大内容是否通过 `SaveAndPinFile` 文件化，前端只展示摘要和引用。
- 技术排查信息是否进入 `log.*`，而不是用户可见 stream。
