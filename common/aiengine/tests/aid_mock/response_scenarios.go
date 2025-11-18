package aidmock

import (
	"bytes"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestScenarioDefinition 测试场景定义
// 用于描述特定测试场景中的AI响应序列
type TestScenarioDefinition struct {
	Name        string   // 场景名称
	Description string   // 场景描述
	Steps       []string // 响应步骤（按顺序）
}

// TestScenarios 定义常见测试场景
var TestScenarios = map[string]*TestScenarioDefinition{
	// 场景1: 简单问答流程
	"simple_qa": &TestScenarioDefinition{
		Name:        "simple_qa",
		Description: "Simple question and answer flow",
		Steps: []string{
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Here is the answer to your question."}, "human_readable_thought": "Understanding the question", "cumulative_summary": "Question answered"}`,
		},
	},

	// 场景2: 单个工具调用流程
	"single_tool_call": &TestScenarioDefinition{
		Name:        "single_tool_call",
		Description: "Single tool call workflow",
		Steps: []string{
			// 步骤1: 请求工具
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "sleep"}, "human_readable_thought": "Need to use sleep tool", "cumulative_summary": "Requesting tool"}`,
			// 步骤2: 生成工具参数
			`{"@action": "call-tool", "params": {"seconds": 0.1}}`,
			// 步骤3: 验证满意度
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Tool executed successfully"}`,
		},
	},

	// 场景3: 多工具调用流程
	"multi_tool_call": &TestScenarioDefinition{
		Name:        "multi_tool_call",
		Description: "Multiple tool call workflow",
		Steps: []string{
			// 工具1
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "search"}, "human_readable_thought": "Searching first", "cumulative_summary": "Search initiated"}`,
			`{"@action": "call-tool", "params": {"query": "test"}}`,
			// 工具2
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "analyze"}, "human_readable_thought": "Analyzing results", "cumulative_summary": "Analysis initiated"}`,
			`{"@action": "call-tool", "params": {"data": "search_results"}}`,
			// 验证
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "All tools executed"}`,
		},
	},

	// 场景4: 计划和执行流程
	"plan_and_execute": &TestScenarioDefinition{
		Name:        "plan_and_execute",
		Description: "Plan and execute workflow",
		Steps: []string{
			// 请求计划
			`{"@action": "object", "next_action": {"type": "request_plan_and_execution", "plan_request_payload": "Create and execute plan"}, "human_readable_thought": "Planning task", "cumulative_summary": "Plan requested"}`,
			// 验证满意度
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Plan executed successfully"}`,
		},
	},

	// 场景5: 请求澄清流程
	"clarification_flow": &TestScenarioDefinition{
		Name:        "clarification_flow",
		Description: "Clarification request workflow",
		Steps: []string{
			// 请求澄清
			`{"@action": "object", "next_action": {"type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "Which option?", "options": ["A", "B", "C"]}}, "human_readable_thought": "Need clarification", "cumulative_summary": "Awaiting user input"}`,
			// 收到答复后继续
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Based on your choice, here is the result."}, "human_readable_thought": "Processing user choice", "cumulative_summary": "Choice processed"}`,
		},
	},

	// 场景6: 错误处理和重试
	"error_and_retry": &TestScenarioDefinition{
		Name:        "error_and_retry",
		Description: "Error handling and retry workflow",
		Steps: []string{
			// 首次尝试
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "risky_tool"}, "human_readable_thought": "Trying tool", "cumulative_summary": "First attempt"}`,
			`{"@action": "call-tool", "params": {"retry": false}}`,
			// 错误响应
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Error occurred, will retry."}, "human_readable_thought": "Handling error", "cumulative_summary": "Error detected"}`,
			// 重试
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "risky_tool"}, "human_readable_thought": "Retrying", "cumulative_summary": "Retry attempt"}`,
			`{"@action": "call-tool", "params": {"retry": true}}`,
			// 成功
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Retry successful"}`,
		},
	},

	// 场景7: 带内存的对话流程
	"memory_conversation": &TestScenarioDefinition{
		Name:        "memory_conversation",
		Description: "Conversation with memory context",
		Steps: []string{
			// 第一轮对话
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "First response"}, "human_readable_thought": "Initial answer", "cumulative_summary": "First interaction recorded"}`,
			// 第二轮对话（使用记忆）
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Second response based on context"}, "human_readable_thought": "Using previous context", "cumulative_summary": "Context-aware response"}`,
			// 第三轮对话（深度使用记忆）
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Third response with full context"}, "human_readable_thought": "Full context utilization", "cumulative_summary": "Complete conversation context maintained"}`,
		},
	},

	// 场景8: 代码生成和验证
	"code_generation": &TestScenarioDefinition{
		Name:        "code_generation",
		Description: "Code generation and validation workflow",
		Steps: []string{
			// 请求代码生成工具
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "write_yaklang_code"}, "human_readable_thought": "Writing code", "cumulative_summary": "Code generation started"}`,
			// 生成代码参数
			`{"@action": "call-tool", "params": {"code": "println(\"test\")", "language": "yaklang"}}`,
			// 验证代码
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "check_syntax"}, "human_readable_thought": "Validating syntax", "cumulative_summary": "Syntax check initiated"}`,
			`{"@action": "call-tool", "params": {"code": "println(\"test\")"}}`,
			// 完成
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Code generated and validated"}`,
		},
	},

	// 场景9: 风险创建流程
	"risk_creation": &TestScenarioDefinition{
		Name:        "risk_creation",
		Description: "Security risk creation workflow",
		Steps: []string{
			// 请求创建风险
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "create_risk"}, "human_readable_thought": "Creating risk entry", "cumulative_summary": "Risk creation initiated"}`,
			// 提供风险参数
			`{"@action": "call-tool", "params": {"target": "http://test.com", "title": "SQL Injection", "severity": "high", "type": "sqli"}}`,
			// 验证
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Risk created successfully"}`,
		},
	},

	// 场景10: 知识库查询和回答
	"knowledge_query": &TestScenarioDefinition{
		Name:        "knowledge_query",
		Description: "Knowledge base query and answer workflow",
		Steps: []string{
			// 查询知识库
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "query_knowledge_base"}, "human_readable_thought": "Searching knowledge", "cumulative_summary": "Knowledge query initiated"}`,
			`{"@action": "call-tool", "params": {"query": "How to use yaklang?"}}`,
			// 基于知识库回答
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Based on knowledge base: Yaklang is used for..."}, "human_readable_thought": "Providing knowledge-based answer", "cumulative_summary": "Answer from knowledge base"}`,
		},
	},

	// 场景11: 蓝图选择和执行
	"blueprint_execution": &TestScenarioDefinition{
		Name:        "blueprint_execution",
		Description: "Blueprint selection and execution workflow",
		Steps: []string{
			// 选择蓝图
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "blueprint"}, "human_readable_thought": "Selecting blueprint", "cumulative_summary": "Blueprint selection"}`,
			`{"@action": "call-tool", "params": {"blueprint_name": "web_scanner"}}`,
			// 修改参数
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "modify_blueprint_params"}, "human_readable_thought": "Adjusting parameters", "cumulative_summary": "Parameters modified"}`,
			`{"@action": "call-tool", "params": {"target": "http://example.com", "depth": 3}}`,
			// 执行
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Blueprint executed"}`,
		},
	},

	// 场景12: 任务队列管理
	"task_queue_management": &TestScenarioDefinition{
		Name:        "task_queue_management",
		Description: "Task queue management workflow",
		Steps: []string{
			// 任务入队
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task enqueued"}, "human_readable_thought": "Adding to queue", "cumulative_summary": "Task queued"}`,
			// 调整优先级
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Priority adjusted"}, "human_readable_thought": "Jumping queue", "cumulative_summary": "Priority changed"}`,
			// 任务出队执行
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task dequeued and executing"}, "human_readable_thought": "Processing task", "cumulative_summary": "Task executing"}`,
		},
	},

	// 场景13: MCP协议工具调用
	"mcp_protocol": &TestScenarioDefinition{
		Name:        "mcp_protocol",
		Description: "MCP protocol tool usage workflow",
		Steps: []string{
			// 请求MCP工具
			`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "mcp_tool"}, "human_readable_thought": "Using MCP tool", "cumulative_summary": "MCP invoked"}`,
			// 提供MCP参数
			`{"@action": "call-tool", "params": {"protocol": "mcp", "action": "fetch_resource"}}`,
			// 验证结果
			`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "MCP tool executed successfully"}`,
		},
	},

	// 场景14: 自我反思和优化
	"self_reflection": &TestScenarioDefinition{
		Name:        "self_reflection",
		Description: "Self-reflection and optimization workflow",
		Steps: []string{
			// 初始响应
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Initial approach"}, "human_readable_thought": "First attempt", "cumulative_summary": "Initial response"}`,
			// 自我反思
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "After reflection, better approach"}, "human_readable_thought": "Reflecting on approach", "cumulative_summary": "Improved solution"}`,
			// 最终优化
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Optimized final solution"}, "human_readable_thought": "Optimization complete", "cumulative_summary": "Best solution provided"}`,
		},
	},

	// 场景15: 任务取消和清理
	"task_cancellation": &TestScenarioDefinition{
		Name:        "task_cancellation",
		Description: "Task cancellation and cleanup workflow",
		Steps: []string{
			// 开始任务
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task started"}, "human_readable_thought": "Starting task", "cumulative_summary": "Task initiated"}`,
			// 接收取消请求
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Cancelling task"}, "human_readable_thought": "Processing cancellation", "cumulative_summary": "Cancellation in progress"}`,
			// 完成清理
			`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Task cancelled and cleaned up"}, "human_readable_thought": "Cleanup complete", "cumulative_summary": "Task cancelled"}`,
		},
	},
}

// GetScenario 获取指定场景的响应序列
func GetScenario(name string) (*TestScenarioDefinition, bool) {
	scenario, ok := TestScenarios[name]
	return scenario, ok
}

// ListScenarios 列出所有可用的场景名称
func ListScenarios() []string {
	names := make([]string, 0, len(TestScenarios))
	for name := range TestScenarios {
		names = append(names, name)
	}
	return names
}

// SequentialScenarios 顺序响应场景生成器
// 用于按顺序返回预定义的响应序列，适合测试多步骤流程
type SequentialScenarios struct {
	responses []string
	index     int
}

// NewSequentialScenarios 创建一个顺序响应场景生成器
func NewSequentialScenarios(responses []string) *SequentialScenarios {
	return &SequentialScenarios{
		responses: responses,
		index:     0,
	}
}

// NewSequentialScenariosFromScenario 从场景创建顺序响应场景生成器
func NewSequentialScenariosFromScenario(scenarioName string) *SequentialScenarios {
	if scenario, ok := GetScenario(scenarioName); ok {
		return NewSequentialScenarios(scenario.Steps)
	}
	return NewSequentialScenarios([]string{})
}

// GetAICallbackType 实现AIScenario接口
func (s *SequentialScenarios) GetAICallbackType() aicommon.AICallbackType {
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()

		if s.index < len(s.responses) {
			response := s.responses[s.index]
			s.index++
			rsp.EmitOutputStream(bytes.NewBufferString(response))
		} else {
			// 如果超出范围，返回默认响应
			defaultResp := `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "No more responses in sequence"}, "human_readable_thought": "Sequence completed", "cumulative_summary": "End of sequence"}`
			rsp.EmitOutputStream(bytes.NewBufferString(defaultResp))
		}

		rsp.Close()
		return rsp, nil
	}
}

// Reset 重置序列索引
func (s *SequentialScenarios) Reset() {
	s.index = 0
}

// AddResponse 添加响应到序列
func (s *SequentialScenarios) AddResponse(response string) {
	s.responses = append(s.responses, response)
}

// GetCurrentIndex 获取当前索引
func (s *SequentialScenarios) GetCurrentIndex() int {
	return s.index
}
