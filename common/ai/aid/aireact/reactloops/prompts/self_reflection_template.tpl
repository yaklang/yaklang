<|SELF_REFLECTION_TASK_{{.Nonce}}|>
# 自我反思任务

针对刚执行的操作做一次简短反思,产物会落到时间线供下一轮决策吸收.
保持简洁: 字段全部可选, 没有结论就不填.

## 操作执行详情

<|ACTION_DETAILS_{{.Nonce}}|>
- 操作类型: {{.ActionType}}
- 工具名称: {{if .ToolName}}{{.ToolName}}{{else}}(无, 非工具调用类 action){{end}}
- 迭代轮次: {{.IterationNum}}
- 执行时间: {{.ExecutionTime}}
- 执行结果: {{.ResultStatus}}
{{if .ErrorMessage}}- 错误信息: {{.ErrorMessage}}{{end}}
<|ACTION_DETAILS_END_{{.Nonce}}|>

{{if .SpinDetection}}<|SPIN_DETECTION_{{.Nonce}}|>
## 死循环 (SPIN) 检测

- 检测到连续 {{.SpinDetection.ConsecutiveCount}} 次执行同一个 ActionType "{{.SpinDetection.ActionType}}"{{if .SpinDetection.ToolName}} + ToolName "{{.SpinDetection.ToolName}}"{{end}}.
- 当前是第 {{.SpinDetection.SpinWarningCount}} 次 SPIN 警告, 升级等级 {{.SpinDetection.EscalationLevel}}.
- 定义: SPIN 指 Agent 反复做出相同或相似决策, 但任务没有推进.

### 最近的 action 历史

{{.SpinDetection.RecentActionsText}}

{{if .SpinDetection.TimelineContent}}### Timeline 上下文

{{.SpinDetection.TimelineContent}}{{end}}

### 判定要求

请独立判断是真 SPIN 还是正常推进:
- 如果参数/目标实质不同(例如 URL 不同、查询不同), 不是 SPIN. 设置 `is_spinning=false`、`is_task_progressing=true`.
- 如果在反复执行近似相同逻辑、没有新增信息收益, 是 SPIN. 设置 `is_spinning=true`, 并填写 `spin_reason` 和 `suggestions`.

{{if ge .SpinDetection.EscalationLevel 3}}### 严重升级 (SWOT 分析)

升级等级 {{.SpinDetection.EscalationLevel}}, 即将强制退出循环. 用 SWOT 简要分析后给建议:
- Strengths: agent 已经掌握但反复重新发现的信息.
- Weaknesses: 导致重复 "{{.SpinDetection.ActionType}}" 的认知偏差.
- Opportunities: 还没尝试过的具体备选 action.
- Threats: 继续重复将导致任务失败.

`suggestions` 必须包含: "立即改用 [X] 而非 '{{.SpinDetection.ActionType}}'", X 为具体 action.
{{else if ge .SpinDetection.EscalationLevel 2}}### 升级 (SMART 分析)

升级等级 {{.SpinDetection.EscalationLevel}}, 之前的建议已被忽略. 用 SMART 框架生成新建议:
- Specific: 指出具体的替代 action 类型和参数.
- Measurable: 定义替代 action 的成功标志.
- Achievable: 确认该替代 action 在工具集中可用.
- Relevant: 解释替代 action 如何直接推进任务.
- Time-bound: 替代 action 必须在一次迭代内可完成.

`suggestions` 必须命令式 (DO X), 不要建议式 (consider X).
{{else}}### 五问分析

围绕这五个问题给出建议:
1. 为什么 "{{.SpinDetection.ActionType}}" 被反复执行?
2. 应该改用哪个具体的不同 action?
3. agent 已经掌握的什么信息让继续重复变得不必要?
4. 时间线最近一桶里有没有被忽略的反馈?
5. 是否需要先 ask_for_clarification 再继续?
{{end}}
<|SPIN_DETECTION_END_{{.Nonce}}|>{{end}}

<|OUTPUT_SCHEMA_{{.Nonce}}|>
## 输出格式

请按以下 JSON schema 返回结果:

```jsonschema
{{.Schema}}
```
<|OUTPUT_SCHEMA_END_{{.Nonce}}|>

提示: 保持简洁, 按需填写. 如果不是 SPIN, 把 `is_task_progressing` 设为 true, `suggestions` 留空即可.
<|SELF_REFLECTION_TASK_END_{{.Nonce}}|>
