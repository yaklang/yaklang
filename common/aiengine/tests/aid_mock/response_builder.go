package aidmock

import (
	"encoding/json"
	"fmt"
)

// ReactResponseBuilder 是ReAct响应的基础构建器
type ReactResponseBuilder struct {
	action            string
	nextActionType    string
	payload           interface{}
	humanThought      string
	cumulativeSummary string
}

// NewReactResponseBuilder 创建一个新的ReAct响应构建器
func NewReactResponseBuilder() *ReactResponseBuilder {
	return &ReactResponseBuilder{
		action: "object",
	}
}

// WithDirectlyAnswer 设置为直接回答类型
func (b *ReactResponseBuilder) WithDirectlyAnswer(answer string) *ReactResponseBuilder {
	b.nextActionType = "directly_answer"
	b.payload = answer
	return b
}

// WithRequireTool 设置为请求工具类型
func (b *ReactResponseBuilder) WithRequireTool(toolName string) *ReactResponseBuilder {
	b.nextActionType = "require_tool"
	b.payload = toolName
	return b
}

// WithRequestPlanAndExecution 设置为请求计划和执行类型
func (b *ReactResponseBuilder) WithRequestPlanAndExecution(planPayload string) *ReactResponseBuilder {
	b.nextActionType = "request_plan_and_execution"
	b.payload = planPayload
	return b
}

// WithAskForClarification 设置为请求澄清类型
func (b *ReactResponseBuilder) WithAskForClarification(question string, options []string) *ReactResponseBuilder {
	b.nextActionType = "ask_for_clarification"
	b.payload = map[string]interface{}{
		"question": question,
		"options":  options,
	}
	return b
}

// WithHumanThought 设置人类可读的思考过程
func (b *ReactResponseBuilder) WithHumanThought(thought string) *ReactResponseBuilder {
	b.humanThought = thought
	return b
}

// WithCumulativeSummary 设置累积总结
func (b *ReactResponseBuilder) WithCumulativeSummary(summary string) *ReactResponseBuilder {
	b.cumulativeSummary = summary
	return b
}

// Build 构建JSON响应字符串
func (b *ReactResponseBuilder) Build() string {
	response := map[string]interface{}{
		"@action":                b.action,
		"human_readable_thought": b.humanThought,
		"cumulative_summary":     b.cumulativeSummary,
	}

	// 构建 next_action
	nextAction := map[string]interface{}{
		"type": b.nextActionType,
	}

	// 根据类型设置payload字段
	switch b.nextActionType {
	case "directly_answer":
		nextAction["answer_payload"] = b.payload
	case "require_tool":
		nextAction["tool_require_payload"] = b.payload
	case "request_plan_and_execution":
		nextAction["plan_request_payload"] = b.payload
	case "ask_for_clarification":
		nextAction["ask_for_clarification_payload"] = b.payload
	}

	response["next_action"] = nextAction

	// 转换为JSON
	jsonBytes, _ := json.Marshal(response)
	return string(jsonBytes)
}

// CallToolBuilder 工具调用参数构建器
type CallToolBuilder struct {
	params map[string]interface{}
}

// NewCallToolBuilder 创建工具调用参数构建器
func NewCallToolBuilder() *CallToolBuilder {
	return &CallToolBuilder{
		params: make(map[string]interface{}),
	}
}

// WithParam 添加单个参数
func (b *CallToolBuilder) WithParam(key string, value interface{}) *CallToolBuilder {
	b.params[key] = value
	return b
}

// WithParams 批量添加参数
func (b *CallToolBuilder) WithParams(params map[string]interface{}) *CallToolBuilder {
	for k, v := range params {
		b.params[k] = v
	}
	return b
}

// Build 构建JSON响应字符串
func (b *CallToolBuilder) Build() string {
	response := map[string]interface{}{
		"@action": "call-tool",
		"params":  b.params,
	}
	jsonBytes, _ := json.Marshal(response)
	return string(jsonBytes)
}

// VerifySatisfactionBuilder 验证满意度构建器
type VerifySatisfactionBuilder struct {
	satisfied bool
	reasoning string
}

// NewVerifySatisfactionBuilder 创建验证满意度构建器
func NewVerifySatisfactionBuilder() *VerifySatisfactionBuilder {
	return &VerifySatisfactionBuilder{}
}

// WithSatisfied 设置是否满意
func (b *VerifySatisfactionBuilder) WithSatisfied(satisfied bool) *VerifySatisfactionBuilder {
	b.satisfied = satisfied
	return b
}

// WithReasoning 设置原因
func (b *VerifySatisfactionBuilder) WithReasoning(reasoning string) *VerifySatisfactionBuilder {
	b.reasoning = reasoning
	return b
}

// Build 构建JSON响应字符串
func (b *VerifySatisfactionBuilder) Build() string {
	response := map[string]interface{}{
		"@action":        "verify-satisfaction",
		"user_satisfied": b.satisfied,
		"reasoning":      b.reasoning,
	}
	jsonBytes, _ := json.Marshal(response)
	return string(jsonBytes)
}

// 便捷函数：直接创建常用响应

// BuildDirectlyAnswer 构建直接回答响应
func BuildDirectlyAnswer(answer, thought, summary string) string {
	return NewReactResponseBuilder().
		WithDirectlyAnswer(answer).
		WithHumanThought(thought).
		WithCumulativeSummary(summary).
		Build()
}

// BuildRequireTool 构建请求工具响应
func BuildRequireTool(toolName, thought, summary string) string {
	return NewReactResponseBuilder().
		WithRequireTool(toolName).
		WithHumanThought(thought).
		WithCumulativeSummary(summary).
		Build()
}

// BuildRequestPlanAndExecution 构建请求计划执行响应
func BuildRequestPlanAndExecution(planPayload, thought, summary string) string {
	return NewReactResponseBuilder().
		WithRequestPlanAndExecution(planPayload).
		WithHumanThought(thought).
		WithCumulativeSummary(summary).
		Build()
}

// BuildAskForClarification 构建请求澄清响应
func BuildAskForClarification(question string, options []string, thought, summary string) string {
	return NewReactResponseBuilder().
		WithAskForClarification(question, options).
		WithHumanThought(thought).
		WithCumulativeSummary(summary).
		Build()
}

// BuildCallTool 构建工具调用参数响应
func BuildCallTool(params map[string]interface{}) string {
	return NewCallToolBuilder().
		WithParams(params).
		Build()
}

// BuildVerifySatisfaction 构建验证满意度响应
func BuildVerifySatisfaction(satisfied bool, reasoning string) string {
	return NewVerifySatisfactionBuilder().
		WithSatisfied(satisfied).
		WithReasoning(reasoning).
		Build()
}

// 常用工具参数构建器

// BuildSleepToolParams 构建sleep工具参数
func BuildSleepToolParams(seconds float64) string {
	return BuildCallTool(map[string]interface{}{
		"seconds": seconds,
	})
}

// BuildSearchToolParams 构建搜索工具参数
func BuildSearchToolParams(query string, maxResults int) string {
	return BuildCallTool(map[string]interface{}{
		"query":       query,
		"max_results": maxResults,
	})
}

// BuildRiskToolParams 构建创建风险工具参数
func BuildRiskToolParams(target, title, severity, riskType string) string {
	params := map[string]interface{}{
		"target": target,
		"title":  title,
	}
	if severity != "" {
		params["severity"] = severity
	}
	if riskType != "" {
		params["type"] = riskType
	}
	return BuildCallTool(params)
}

// BuildCodeToolParams 构建代码工具参数
func BuildCodeToolParams(code, description, language string) string {
	params := map[string]interface{}{
		"code": code,
	}
	if description != "" {
		params["description"] = description
	}
	if language != "" {
		params["language"] = language
	}
	return BuildCallTool(params)
}

// BuildBlueprintToolParams 构建蓝图工具参数
func BuildBlueprintToolParams(blueprintName string, additionalParams map[string]interface{}) string {
	params := map[string]interface{}{
		"blueprint_name": blueprintName,
	}
	for k, v := range additionalParams {
		params[k] = v
	}
	return BuildCallTool(params)
}

// 用于调试的辅助函数

// PrettyPrintResponse 格式化打印响应（用于调试）
func PrettyPrintResponse(response string) string {
	var obj interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		return response
	}
	prettyBytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return response
	}
	return string(prettyBytes)
}

// ValidateResponse 验证响应格式是否正确
func ValidateResponse(response string) error {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	action, ok := obj["@action"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid @action field")
	}

	switch action {
	case "object":
		if _, ok := obj["next_action"]; !ok {
			return fmt.Errorf("object action requires next_action field")
		}
	case "call-tool":
		if _, ok := obj["params"]; !ok {
			return fmt.Errorf("call-tool action requires params field")
		}
	case "verify-satisfaction":
		if _, ok := obj["user_satisfied"]; !ok {
			return fmt.Errorf("verify-satisfaction action requires user_satisfied field")
		}
		if _, ok := obj["reasoning"]; !ok {
			return fmt.Errorf("verify-satisfaction action requires reasoning field")
		}
	default:
		return fmt.Errorf("unknown action type: %s", action)
	}

	return nil
}
