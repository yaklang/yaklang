# Generic Event Runner 运行说明

## 1. 脚本做什么

`generic-event-runner.yak` 用来对一个目标做多次独立的 AI 安全试跑。

它的特点是：

- 输入只有 `target`、`quality_priority_ai_config`、`speed_priority_ai_config`、`limits`、`run_config`
- 不带 benchmark registry 上下文，不知道预期漏洞、公开状态、case 标签
- 不要求 AI 最终输出 JSON
- 最终结果里的 `risks` 来源于本次 session/runtime 下**实际保存的 risk**

也就是说，这个 runner 更像“让 AI 真去跑一次黑盒探测，再把保存下来的 risk 原样汇总成报告”。

## 2. 一次 run 的执行顺序

整体流程如下：

1. 读取配置文件，校验必填字段。
2. 生成 `runID`，创建输出目录和 `evidence/` 目录。
3. 按 `limits.samples` 循环执行 trial。
4. 每个 trial：
   - 生成 prompt
   - 用 `aim.qualityPriorityAIConfig(...)` 和 `aim.speedPriorityAIConfig(...)` 启动一次 ReAct
   - 监听 `onEvent` / `onStreamContent`
   - 从 `consumption` 事件读取累计 token
   - 结束后通过 `aim.GetSessionRuntimeID(sessionID)` 找到 runtime
   - 再通过 `risk.YieldRiskByRuntimeId(runtimeID)` 收集本次保存的 risk
   - 把查到的 risk 直接 JSON 化，写入当前 trial 的 `risks`
   - 把 trial evidence 写入 `evidence/trial-xxxx.jsonl`
5. 所有 trial 结束后，汇总出 run 级 JSON/Markdown 报告。

## 3. 会回跑几次

回跑次数就是：

```json
"limits": {
  "samples": N
}
```

脚本会顺序执行 `N` 次 trial：

- 第 1 次：`trial-0001`
- 第 2 次：`trial-0002`
- ...
- 第 N 次：`trial-00NN`

这里的“回跑”不是失败重试，而是**固定跑 `samples` 次独立样本**。  
某次 trial 失败，不会在内部额外补跑；它只会在最终 summary 里记为 `failed`。

## 4. 每次 trial 最久跑多久

单次 trial 的时间上限由：

```json
"limits": {
  "time_budget_sec": 1800
}
```

控制。

脚本会把它传给：

```yak
aim.timeout(limits["time_budget_sec"])
```

所以：

- **单次 trial 的硬上限**约等于 `time_budget_sec`
- **整个 run 的理论上限**约等于 `samples * time_budget_sec`，再加上少量收尾和落盘时间

但实际通常会更早结束，因为还有：

```json
"limits": {
  "step_budget": 25
}
```

它限制了 ReAct 最多能推进多少步。步数先耗尽，也会提前结束 trial。

## 5. 什么时候产出什么

### 5.1 trial 过程中

trial 执行时，脚本会不断在内存里累积 evidence：

- `event`
- `stream`
- `consumption`
- `error`

trial 结束后，立刻写出：

```text
<runDir>/evidence/trial-0001.jsonl
```

每个 sample 对应一份 `jsonl`。

### 5.2 全部 trial 完成后

所有 trial 跑完后，脚本一次性写两份总报告：

```text
<runDir>/<reportPrefix>-<runID>.json
<runDir>/<reportPrefix>-<runID>.md
```

默认 `reportPrefix` 是：

```text
generic-event-runner
```

所以通常会产出：

```text
benchmark-reports/generic-event-runner/<runID>/generic-event-runner-<runID>.json
benchmark-reports/generic-event-runner/<runID>/generic-event-runner-<runID>.md
```

## 6. 每类输出分别是什么

### 6.1 `evidence/*.jsonl`

这是最细粒度的原始记录，包含：

- `trial_start`
- `event`
- `stream`
- `consumption`
- `risk_collection`
- `trial_end`
- `error`（若失败）

适合排查：

- AI 实际走了哪些步骤
- 当时 token 消耗是多少
- 最后关联到了哪些 runtime / risk

### 6.2 总报告 JSON

总报告 JSON 是机器可读主输出，结构大致为：

- `report_type`
- `run_id`
- `generated_at`
- `config_file`
- `input`
- `summary`
- `trials`

其中：

- `summary` 是 run 级汇总
- `trials` 是所有 sample 的结果列表

### 6.3 总报告 Markdown

Markdown 是给人看的摘要，主要包含：

- run 基本信息
- quality / speed provider-model
- 完成/失败 trial 数
- risk 总数
- 总耗时
- 总 token
- 每个 trial 的表格摘要

## 7. risks 是怎么来的

当前逻辑里，结果里的 `risks` **不是**从 AI 最终文本里抽 JSON。

而是：

1. 给本次 trial 生成唯一 `sessionID`
2. 运行 ReAct
3. 结束后通过 `aim.GetSessionRuntimeID(sessionID)` 拿 runtime 列表
4. 再遍历 `risk.YieldRiskByRuntimeId(runtimeID)`
5. 把查到的 risk 直接做 JSON 序列化

所以这个 runner 的结论依赖的是：

- AI 有没有真的把漏洞保存成 risk
- risk 里有没有足够完整的信息

如果 AI 没保存 risk，即使它在流输出里说自己发现了漏洞，最终 `risks` 仍然可能是空。

## 8. token 是怎么统计的

当前脚本只看 `consumption` 事件，不使用 `usageCallback`。

读取字段为：

- `input_consumption`
- `output_consumption`
- `cache_hit_token`
- `consumption_uuid`

其中 token 统计直接使用事件的**外层累计值**，不读取 `tier_consumption`。

## 9. 当前没有做的事

当前 runner **没有**做这些事：

- 没有按 trial 失败自动重试
- 没有按 risk 去重到 run 级之外的 registry
- 不会根据 AI 最终回答解析结果 JSON
- 没有计算 `cost_usd`
- 没有使用 vision 配置

## 10. 运行时需要关注的三个核心参数

最关键的是：

1. `limits.samples`：总共跑几次
2. `limits.time_budget_sec`：单次最多跑多久
3. `limits.step_budget`：单次最多推进多少步

如果想快速烟测，通常先把它们压小：

```json
"limits": {
  "step_budget": 8,
  "time_budget_sec": 300,
  "samples": 2
}
```

如果想做更稳定的 benchmark，再把 `samples` 拉高。
