<|SELF_REFLECTION_TASK_{{.Nonce}}|>
# 自我反思任务

对刚执行的操作进行简要反思分析。反思结果将保存到长期记忆中用于改进未来决策。

## 操作执行详情

<|ACTION_DETAILS_{{.Nonce}}|>
**操作类型**: {{.ActionType}}
**迭代轮次**: {{.IterationNum}}
**执行时间**: {{.ExecutionTime}}
**执行结果**: {{.ResultStatus}}
{{if .ErrorMessage}}**错误信息**: {{.ErrorMessage}}{{end}}
<|ACTION_DETAILS_END_{{.Nonce}}|>

{{if .EnvironmentalImpact}}<|ENVIRONMENTAL_IMPACT_{{.Nonce}}|>
## 环境影响

**状态变化**: {{.EnvironmentalImpact.StateChanges}}
**副作用**: {{.EnvironmentalImpact.SideEffects}}
**正面影响**: {{.EnvironmentalImpact.PositiveEffects}}
**负面影响**: {{.EnvironmentalImpact.NegativeEffects}}
<|ENVIRONMENTAL_IMPACT_END_{{.Nonce}}|>{{end}}

{{if .RelevantMemories}}<|RELEVANT_MEMORIES_{{.Nonce}}|>
## 相关历史记忆

{{.RelevantMemories}}
<|RELEVANT_MEMORIES_END_{{.Nonce}}|>{{end}}

{{if .PreviousReflections}}<|PREVIOUS_REFLECTIONS_{{.Nonce}}|>
## 之前的反思

{{.PreviousReflections}}
<|PREVIOUS_REFLECTIONS_END_{{.Nonce}}|>{{end}}

{{if .SpinDetection}}<|SPIN_DETECTION_{{.Nonce}}|>
## SPIN 检测分析

检测到连续 {{.SpinDetection.ConsecutiveCount}} 次执行相同的 Action 类型（{{.SpinDetection.ActionType}}）。请分析是否发生了 SPIN 情况。

**SPIN 定义**：AI Agent 反复做出相同或相似的决策，没有推进任务。

### 最近的 Action 执行历史

{{.SpinDetection.RecentActionsText}}

{{if .SpinDetection.TimelineContent}}### Timeline 上下文

{{.SpinDetection.TimelineContent}}{{end}}

**请分析**：
1. 这些 Action 是否在重复执行相同的逻辑，没有推进任务？
2. 如果发生了 SPIN，请说明原因
3. 如果发生了 SPIN，请提供打破循环的具体建议（这些建议应整合到 `suggestions` 中）

<|SPIN_DETECTION_END_{{.Nonce}}|>{{end}}

<|ANALYSIS_REQUIREMENTS_{{.Nonce}}|>
## 分析要求

请对本次操作进行简要反思，**按需提供以下内容**（所有字段均为可选）：

1. **建议**：针对类似情况的改进建议（如有）。{{if .SpinDetection}}**如果检测到 SPIN，请将打破循环的建议整合到此字段。**{{end}}
{{if .SpinDetection}}2. **SPIN 检测**：如果提供了 SPIN 检测数据，请判断是否发生 SPIN，并说明原因{{end}}

**注意**：如果是常规操作且无特殊情况，可以只返回最基本的信息。保持简洁，避免冗余。{{if .SpinDetection}}**如果同时进行了 SPIN 检测，请将 SPIN 相关建议整合到 `suggestions` 中。**{{end}}
<|ANALYSIS_REQUIREMENTS_END_{{.Nonce}}|>

<|OUTPUT_SCHEMA_{{.Nonce}}|>
## 输出格式

使用以下 JSON schema 返回结果：

```jsonschema
{{.Schema}}
```
<|OUTPUT_SCHEMA_END_{{.Nonce}}|>

**提示**：保持简洁，按需填写字段，避免产生过多噪声。
<|SELF_REFLECTION_TASK_END_{{.Nonce}}|>

