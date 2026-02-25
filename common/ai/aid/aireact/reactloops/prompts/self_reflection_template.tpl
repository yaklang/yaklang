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
## SPIN Detection Analysis (Warning #{{.SpinDetection.SpinWarningCount}} — Escalation Level {{.SpinDetection.EscalationLevel}})

Detected {{.SpinDetection.ConsecutiveCount}} consecutive executions of the same action type '{{.SpinDetection.ActionType}}'.
This is spin warning #{{.SpinDetection.SpinWarningCount}}. Previous warnings have been IGNORED by the agent.

**SPIN definition**: The AI Agent repeatedly makes the same or similar decisions without advancing the task.

### Recent Action History

{{.SpinDetection.RecentActionsText}}

{{if .SpinDetection.TimelineContent}}### Timeline Context

{{.SpinDetection.TimelineContent}}{{end}}

### Required Analysis

1. Are these actions repeating the same logic without progress? (Almost certainly YES at this escalation level)
2. If SPIN is confirmed, identify the ROOT CAUSE — why is the agent unable to choose a different action?
3. Your suggestions MUST include CONCRETE alternative action types (not vague advice).
4. Your suggestions MUST name specific tool/action that DIFFERS from '{{.SpinDetection.ActionType}}'.

{{if ge .SpinDetection.EscalationLevel 3}}### ⛔ CRITICAL: SWOT-Based Suggestion Generation

This is escalation level {{.SpinDetection.EscalationLevel}}. The loop will be FORCE-TERMINATED soon.
Generate suggestions using SWOT analysis:
- **Strengths**: What does the agent already know that it keeps re-discovering?
- **Weaknesses**: What misconception causes the agent to repeat '{{.SpinDetection.ActionType}}'?
- **Opportunities**: List ALL alternative actions the agent has NOT tried.
- **Threats**: State clearly that repeating '{{.SpinDetection.ActionType}}' will cause task failure.

Your suggestions MUST include: "IMMEDIATELY use action [X] instead of '{{.SpinDetection.ActionType}}'" where [X] is a concrete alternative.
{{else if ge .SpinDetection.EscalationLevel 2}}### ⚠️ ESCALATED: S.M.A.R.T Suggestion Generation

This is escalation level {{.SpinDetection.EscalationLevel}}. Previous suggestions were IGNORED.
Generate suggestions using S.M.A.R.T framework:
- **Specific**: Name the exact alternative action type and its parameters
- **Measurable**: Define what success looks like for the alternative action
- **Achievable**: Confirm the alternative action is available in the tool set
- **Relevant**: Explain how the alternative directly advances the task
- **Time-bound**: The alternative must complete in a single iteration

Your suggestions MUST be action-oriented, not advisory. Say "DO X" not "consider X".
{{else}}### Analysis Required

Answer these questions in your suggestions:
1. Why is '{{.SpinDetection.ActionType}}' being repeated?
2. What SPECIFIC different action should be used instead?
3. What information does the agent already have that makes repeating '{{.SpinDetection.ActionType}}' unnecessary?
{{end}}

<|SPIN_DETECTION_END_{{.Nonce}}|>{{end}}

<|ANALYSIS_REQUIREMENTS_{{.Nonce}}|>
## Analysis Requirements

{{if .SpinDetection}}**⚠ SPIN DETECTED — Escalation Level {{.SpinDetection.EscalationLevel}}**

This is NOT a routine reflection. The agent is in a confirmed SPIN state.

MANDATORY output:
1. Set `is_spinning` to `true`
2. Provide `spin_reason` with the root cause
3. Provide `suggestions` with CONCRETE, ACTIONABLE break-out steps:
   - Each suggestion MUST name a specific action type different from '{{.SpinDetection.ActionType}}'
   - Each suggestion MUST be imperative ("DO X", "USE Y", "SWITCH TO Z")
   - Do NOT suggest "consider" or "try" — use direct commands
{{if ge .SpinDetection.EscalationLevel 3}}
4. **CRITICAL**: This is the FINAL escalation. If your suggestions do not break the loop, the task will be force-terminated as UNSUCCESSFUL.
{{end}}
{{else}}Perform a brief reflection on this action. All fields are optional — provide only what's relevant.

1. **Suggestions**: Improvement advice for similar situations (if any).

**Note**: For routine operations, return minimal information. Keep it concise.
{{end}}
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

