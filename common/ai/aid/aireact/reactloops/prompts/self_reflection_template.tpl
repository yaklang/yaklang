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

<|ANALYSIS_REQUIREMENTS_{{.Nonce}}|>
## 分析要求

请对本次操作进行简要反思，**按需提供以下内容**（所有字段均为可选）：

1. **学习点**：从本次执行中学到的重要经验（如有）
2. **未来建议**：针对类似情况的改进建议（如有）
3. **影响评估**：操作的实际影响（如有特殊影响需要记录）
4. **效果评级**：评估操作效果（可选）

**注意**：如果是常规操作且无特殊情况，可以只返回最基本的信息。保持简洁，避免冗余。
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

