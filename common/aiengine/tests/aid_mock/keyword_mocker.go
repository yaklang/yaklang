package aidmock

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// PromptMatcherFunc 是自定义的prompt匹配函数类型
// 接收prompt字符串，返回是否匹配
type PromptMatcherFunc func(prompt string) bool

// KeywordScenarios 是一个基于关键词匹配的AI场景生成器
// 当请求的prompt中包含特定关键词时，返回预定义的AI响应内容
// 也支持使用自定义的匹配函数进行更灵活的匹配
type KeywordScenarios struct {
	// responses 存储关键词到响应内容的映射
	// key: 响应的名称（唯一标识）
	// value: ScenarioResponse结构
	responses map[string]*ScenarioResponse
}

// ScenarioResponse 表示一个场景响应配置
type ScenarioResponse struct {
	Name        string            // 响应的名称
	Keywords    []string          // 需要匹配的关键词列表（AND关系，所有关键词都要出现）
	Matcher     PromptMatcherFunc // 自定义匹配函数（如果设置，优先使用此函数）
	Response    string            // AI响应内容（JSON格式）
	Description string            // 响应描述
}

// KeywordResponse 是 ScenarioResponse 的别名，保持向后兼容
type KeywordResponse = ScenarioResponse

// NewKeywordScenarios 创建一个新的KeywordScenarios实例，并加载内置的响应配置
func NewKeywordScenarios() *KeywordScenarios {
	scenarios := &KeywordScenarios{
		responses: make(map[string]*ScenarioResponse),
	}
	// 加载内置响应
	// scenarios.loadBuiltInResponses()
	return scenarios
}

// AddResponse 添加自定义的关键词响应配置
func (s *KeywordScenarios) AddResponse(name string, keywords []string, response string, description string) {
	s.responses[name] = &ScenarioResponse{
		Name:        name,
		Keywords:    keywords,
		Matcher:     nil, // 使用关键词匹配
		Response:    response,
		Description: description,
	}
}

// AddResponseWithMatcher 添加使用自定义匹配函数的响应配置
// matcher: 自定义的匹配函数，接收prompt返回是否匹配
// response: AI响应内容（JSON格式）
// description: 响应描述
func (s *KeywordScenarios) AddResponseWithMatcher(name string, matcher PromptMatcherFunc, response string, description string) {
	s.responses[name] = &ScenarioResponse{
		Name:        name,
		Keywords:    nil, // 使用自定义匹配函数
		Matcher:     matcher,
		Response:    response,
		Description: description,
	}
}

// RemoveResponse 移除指定名称的响应配置
func (s *KeywordScenarios) RemoveResponse(name string) {
	delete(s.responses, name)
}

// GetResponse 获取指定名称的响应配置
func (s *KeywordScenarios) GetResponse(name string) *KeywordResponse {
	return s.responses[name]
}

// ListResponses 列出所有已注册的响应名称
func (s *KeywordScenarios) ListResponses() []string {
	names := make([]string, 0, len(s.responses))
	for name := range s.responses {
		names = append(names, name)
	}
	return names
}

// GetAICallbackType 实现AIScenario接口，返回AI回调函数
func (s *KeywordScenarios) GetAICallbackType() aicommon.AICallbackType {
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()

		// 遍历所有响应配置，查找匹配的响应
		for _, resp := range s.responses {
			// 优先使用自定义匹配函数
			if resp.Matcher != nil {
				if resp.Matcher(prompt) {
					// 找到匹配的响应，返回对应的AI响应
					rsp := config.NewAIResponse()
					rsp.EmitOutputStream(bytes.NewBufferString(resp.Response))
					rsp.Close()
					return rsp, nil
				}
			} else if s.matchKeywords(prompt, resp.Keywords) {
				// 使用关键词匹配
				rsp := config.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(resp.Response))
				rsp.Close()
				return rsp, nil
			}
		}

		// 没有找到匹配的响应，返回默认响应
		return s.defaultResponse(config, prompt)
	}
}

// matchKeywords 检查prompt是否包含所有指定的关键词
func (s *KeywordScenarios) matchKeywords(prompt string, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}

	promptLower := strings.ToLower(prompt)
	for _, keyword := range keywords {
		if !strings.Contains(promptLower, strings.ToLower(keyword)) {
			return false
		}
	}
	return true
}

// defaultResponse 返回默认的AI响应（当没有匹配到任何关键词时）
func (s *KeywordScenarios) defaultResponse(config aicommon.AICallerConfigIf, prompt string) (*aicommon.AIResponse, error) {
	rsp := config.NewAIResponse()
	defaultResp := `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "I don't have a mocked response for this prompt."}, "human_readable_thought": "No matching keywords found", "cumulative_summary": "Default response"}`
	rsp.EmitOutputStream(bytes.NewBufferString(defaultResp))
	rsp.Close()
	return rsp, nil
}

// loadBuiltInResponses 加载内置的响应配置
// 这些响应是从common/ai/aid/aireact目录下的测试文件中提取的常见模式
// 使用Builder模式创建响应，更加灵活和易于维护
func (s *KeywordScenarios) loadBuiltInResponses() {
	// 1. 直接回答类型响应
	s.AddResponse(
		"directly_answer",
		[]string{"directly_answer", "next_action"},
		BuildDirectlyAnswer(
			"This is a mocked answer.",
			"Providing a direct answer",
			"Direct answer provided",
		),
		"Basic directly answer response",
	)

	// 2. 工具调用 - require_tool (sleep)
	s.AddResponse(
		"require_tool_sleep",
		[]string{"directly_answer", "request_plan_and_execution", "require_tool"},
		BuildRequireTool(
			"sleep",
			"I need to use the sleep tool",
			"Requesting sleep tool",
		),
		"Request to use a tool (sleep example)",
	)

	// 3. 工具参数生成 - call-tool (sleep params)
	s.AddResponse(
		"call_tool_params",
		[]string{"You need to generate parameters for the tool", "call-tool"},
		BuildSleepToolParams(0.1),
		"Generate parameters for tool calling",
	)

	// 4. 验证满意度 - verify-satisfaction (positive)
	s.AddResponse(
		"verify_satisfaction_positive",
		[]string{"verify-satisfaction", "user_satisfied", "reasoning"},
		BuildVerifySatisfaction(true, "The task has been completed successfully"),
		"Verify user satisfaction - positive",
	)

	// 5. 请求计划和执行 - request_plan_and_execution
	s.AddResponse(
		"request_plan_and_execution",
		[]string{"directly_answer", "request_plan_and_execution", "require_tool"},
		BuildRequestPlanAndExecution(
			"Execute the planned task",
			"Need to create and execute a plan",
			"Planning and execution requested",
		),
		"Request plan and execution",
	)

	// 6. 询问澄清 - ask_for_clarification
	s.AddResponse(
		"ask_for_clarification",
		[]string{"directly_answer", "request_plan_and_execution", "require_tool", "ask_for_clarification"},
		BuildAskForClarification(
			"Could you please provide more details?",
			[]string{"option1", "option2", "option3"},
			"Need clarification from user",
			"Requesting clarification",
		),
		"Ask for user clarification",
	)

	// 7. 带记忆的直接回答
	s.AddResponse(
		"directly_answer_with_memory",
		[]string{"directly_answer", "cumulative_summary"},
		BuildDirectlyAnswer(
			"Answer based on memory context",
			"Using memory to provide answer",
			"Memory-based answer provided",
		),
		"Direct answer with memory context",
	)

	// 8. 工具调用 - 创建风险
	s.AddResponse(
		"require_tool_create_risk",
		[]string{"require_tool", "create_test_risk"},
		BuildRequireTool(
			"create_test_risk",
			"I need to create a test risk",
			"Creating test risk",
		),
		"Request to create a test risk",
	)

	// 9. 创建风险工具参数
	s.AddResponse(
		"call_tool_risk_params",
		[]string{"generate parameters", "create_test_risk", "call-tool"},
		BuildRiskToolParams(
			"http://test.example.com",
			"Test Risk",
			"", // severity 留空
			"", // type 留空
		),
		"Generate parameters for risk creation",
	)

	// 10. 蓝图选择响应
	s.AddResponse(
		"blueprint_selection",
		[]string{"blueprint", "select"},
		BuildRequireTool(
			"blueprint_tool",
			"Selecting appropriate blueprint",
			"Blueprint selected",
		),
		"Blueprint selection response",
	)

	// 11. 代码编写响应
	s.AddResponse(
		"write_yaklang_code",
		[]string{"write", "yaklang", "code"},
		BuildRequireTool(
			"write_code",
			"Writing Yaklang code",
			"Code writing initiated",
		),
		"Write Yaklang code response",
	)

	// 12. 自我反思响应
	s.AddResponse(
		"self_reflection",
		[]string{"reflection", "analyze"},
		BuildDirectlyAnswer(
			"After reflection, I believe the approach is correct.",
			"Performing self-reflection",
			"Self-reflection completed",
		),
		"Self-reflection response",
	)

	// 13. 查询文档响应
	s.AddResponse(
		"query_document",
		[]string{"query", "document", "knowledge"},
		BuildRequireTool(
			"query_knowledge_base",
			"Querying knowledge base",
			"Knowledge query initiated",
		),
		"Query document/knowledge base",
	)

	// 14. 错误处理和修正
	s.AddResponse(
		"error_correction",
		[]string{"error", "fix", "correct"},
		BuildDirectlyAnswer(
			"I've identified the error and will correct it.",
			"Correcting the error",
			"Error correction in progress",
		),
		"Error correction response",
	)

	// 15. 多次工具调用
	s.AddResponse(
		"multiple_tool_calls",
		[]string{"multiple", "tools", "sequence"},
		BuildRequireTool(
			"first_tool",
			"Starting multiple tool calls sequence",
			"Multi-tool execution planned",
		),
		"Multiple tool calls response",
	)

	// 16. 任务取消响应
	s.AddResponse(
		"cancel_task",
		[]string{"cancel", "task", "abort"},
		BuildDirectlyAnswer(
			"Task has been cancelled.",
			"Cancelling task",
			"Task cancelled",
		),
		"Cancel task response",
	)

	// 17. 队列跳转响应
	s.AddResponse(
		"jump_queue",
		[]string{"jump", "queue", "priority"},
		BuildDirectlyAnswer(
			"Adjusting task priority.",
			"Jumping queue",
			"Priority adjusted",
		),
		"Jump queue response",
	)

	// 18. MCP工具调用
	s.AddResponse(
		"mcp_tool_call",
		[]string{"mcp", "tool", "protocol"},
		BuildRequireTool(
			"mcp_tool",
			"Using MCP protocol tool",
			"MCP tool invoked",
		),
		"MCP tool call response",
	)

	// 19. 工具调用失败重试
	s.AddResponse(
		"tool_call_retry",
		[]string{"retry", "tool", "failed"},
		BuildRequireTool(
			"retry_tool",
			"Retrying tool call after failure",
			"Tool retry initiated",
		),
		"Tool call retry response",
	)

	// 20. 知识库答案
	s.AddResponse(
		"answer_with_knowledge",
		[]string{"answer", "knowledge", "base"},
		BuildDirectlyAnswer(
			"Based on knowledge base: [answer content]",
			"Answering from knowledge base",
			"Knowledge-based answer provided",
		),
		"Answer with knowledge base",
	)
}

// CreateCustomScenarios 创建一个空的KeywordScenarios，不加载内置响应
// 适用于需要完全自定义响应的场景
func CreateCustomScenarios() *KeywordScenarios {
	return &KeywordScenarios{
		responses: make(map[string]*ScenarioResponse),
	}
}

// PrintAllResponses 打印所有已注册的响应信息（用于调试）
func (s *KeywordScenarios) PrintAllResponses() {
	fmt.Println("=== Registered Responses ===")
	for name, resp := range s.responses {
		fmt.Printf("Name: %s\n", name)
		fmt.Printf("  Keywords: %v\n", resp.Keywords)
		fmt.Printf("  Description: %s\n", resp.Description)
		fmt.Printf("  Response: %s\n\n", utils.ShrinkString(resp.Response, 100))
	}
	fmt.Printf("Total: %d responses\n", len(s.responses))
}
