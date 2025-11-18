package aidmock

// ResponseTemplates 提供了各种AI响应的模板
// 可以用于创建自定义的响应内容

const (
	// DirectlyAnswerTemplate 直接回答模板
	DirectlyAnswerTemplate = `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "%s"}, "human_readable_thought": "%s", "cumulative_summary": "%s"}`

	// RequireToolTemplate 请求工具模板
	RequireToolTemplate = `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "%s"}, "human_readable_thought": "%s", "cumulative_summary": "%s"}`

	// CallToolTemplate 调用工具模板
	CallToolTemplate = `{"@action": "call-tool", "params": %s}`

	// VerifySatisfactionTemplate 验证满意度模板
	VerifySatisfactionTemplate = `{"@action": "verify-satisfaction", "user_satisfied": %t, "reasoning": "%s"}`

	// RequestPlanAndExecutionTemplate 请求计划执行模板
	RequestPlanAndExecutionTemplate = `{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "%s"}, "human_readable_thought": "%s", "cumulative_summary": "%s"}`

	// AskForClarificationTemplate 请求澄清模板
	AskForClarificationTemplate = `{"@action": "object", "next_action": {"type": "ask_for_clarification", "ask_for_clarification_payload": %s}, "human_readable_thought": "%s", "cumulative_summary": "%s"}`
)

// ResponseTemplate 响应模板结构
type ResponseTemplate struct {
	ActionType  string
	Template    string
	Description string
}

// GetAllTemplates 获取所有预定义的响应模板
func GetAllTemplates() map[string]*ResponseTemplate {
	return map[string]*ResponseTemplate{
		"directly_answer": {
			ActionType:  "directly_answer",
			Template:    DirectlyAnswerTemplate,
			Description: "Direct answer template with answer_payload, thought, and summary",
		},
		"require_tool": {
			ActionType:  "require_tool",
			Template:    RequireToolTemplate,
			Description: "Require tool template with tool name, thought, and summary",
		},
		"call_tool": {
			ActionType:  "call-tool",
			Template:    CallToolTemplate,
			Description: "Call tool template with JSON parameters",
		},
		"verify_satisfaction": {
			ActionType:  "verify-satisfaction",
			Template:    VerifySatisfactionTemplate,
			Description: "Verify satisfaction template with boolean and reasoning",
		},
		"request_plan_and_execution": {
			ActionType:  "request_plan_and_execution",
			Template:    RequestPlanAndExecutionTemplate,
			Description: "Request plan and execution template with payload, thought, and summary",
		},
		"ask_for_clarification": {
			ActionType:  "ask_for_clarification",
			Template:    AskForClarificationTemplate,
			Description: "Ask for clarification template with question and options",
		},
	}
}

// CommonResponses 常见的完整响应内容
var CommonResponses = map[string]string{
	// 简单直接回答
	"simple_answer": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "This is a simple answer."}, "human_readable_thought": "Providing straightforward response", "cumulative_summary": "Simple answer provided"}`,

	// 复杂直接回答
	"complex_answer": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "This is a detailed answer with multiple considerations and nuances."}, "human_readable_thought": "Analyzing multiple factors before answering", "cumulative_summary": "Complex analysis completed and answer provided"}`,

	// 请求sleep工具
	"tool_sleep": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "sleep"}, "human_readable_thought": "Need to use sleep functionality", "cumulative_summary": "Requesting sleep tool"}`,

	// 请求搜索工具
	"tool_search": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "search"}, "human_readable_thought": "Need to search for information", "cumulative_summary": "Initiating search"}`,

	// 请求文件操作工具
	"tool_file_operation": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "file_operation"}, "human_readable_thought": "Need to perform file operations", "cumulative_summary": "File operation requested"}`,

	// sleep工具参数
	"params_sleep": `{"@action": "call-tool", "params": {"seconds": 0.1}}`,

	// 搜索工具参数
	"params_search": `{"@action": "call-tool", "params": {"query": "test query", "max_results": 10}}`,

	// 满意度验证 - 正面
	"satisfaction_yes": `{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Task completed successfully"}`,

	// 满意度验证 - 负面
	"satisfaction_no": `{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "Task needs improvement"}`,

	// 计划执行请求
	"plan_exec_simple": `{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "Execute simple task"}, "human_readable_thought": "Creating execution plan", "cumulative_summary": "Plan created"}`,

	// 请求澄清 - 单选
	"clarify_single": `{"@action": "object", "next_action": {"type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "Which option do you prefer?", "options": ["option1", "option2", "option3"]}}, "human_readable_thought": "Need user input", "cumulative_summary": "Requesting clarification"}`,

	// 请求澄清 - 开放式
	"clarify_open": `{"@action": "object", "next_action": {"type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "Please provide more details about your requirement.", "options": []}}, "human_readable_thought": "Need more information", "cumulative_summary": "Requesting details"}`,

	// 错误响应
	"error_response": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "An error occurred during processing."}, "human_readable_thought": "Error detected", "cumulative_summary": "Error handled"}`,

	// 带内存上下文的回答
	"memory_based_answer": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Based on previous context, here is the answer."}, "human_readable_thought": "Retrieving from memory", "cumulative_summary": "Memory-based response provided"}`,

	// 创建风险工具请求
	"tool_create_risk": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "create_risk"}, "human_readable_thought": "Creating security risk entry", "cumulative_summary": "Risk creation initiated"}`,

	// 风险参数
	"params_risk": `{"@action": "call-tool", "params": {"target": "http://example.com", "title": "Test Risk", "severity": "medium"}}`,

	// Yaklang代码编写
	"tool_write_code": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "write_yaklang_code"}, "human_readable_thought": "Writing Yaklang code", "cumulative_summary": "Code generation started"}`,

	// 代码参数
	"params_code": `{"@action": "call-tool", "params": {"code": "println(\"Hello World\")", "description": "Simple hello world"}}`,

	// 文档查询
	"tool_query_doc": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "query_document"}, "human_readable_thought": "Searching documentation", "cumulative_summary": "Document query initiated"}`,

	// 蓝图工具
	"tool_blueprint": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "blueprint"}, "human_readable_thought": "Selecting blueprint", "cumulative_summary": "Blueprint selection"}`,

	// 多工具序列开始
	"multi_tool_start": `{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "first_tool"}, "human_readable_thought": "Starting tool sequence", "cumulative_summary": "Multi-tool execution initiated"}`,

	// 任务取消
	"task_cancel": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task cancelled as requested."}, "human_readable_thought": "Cancelling task", "cumulative_summary": "Task cancellation processed"}`,

	// 任务完成
	"task_complete": `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task completed successfully."}, "human_readable_thought": "Task finished", "cumulative_summary": "Task completion confirmed"}`,
}

// GetCommonResponse 获取常见响应
func GetCommonResponse(name string) (string, bool) {
	resp, ok := CommonResponses[name]
	return resp, ok
}

// GetTemplateResponse 使用模板生成响应
func GetTemplateResponse(templateName string, args ...interface{}) string {
	templates := GetAllTemplates()
	if template, ok := templates[templateName]; ok {
		return formatTemplate(template.Template, args...)
	}
	return ""
}

// formatTemplate 格式化模板
func formatTemplate(template string, args ...interface{}) string {
	// 简单的字符串格式化
	// 在实际使用中，可以使用更复杂的模板引擎
	return template // 实际实现中需要根据args进行格式化
}

