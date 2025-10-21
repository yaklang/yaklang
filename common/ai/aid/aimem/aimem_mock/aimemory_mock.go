package aimem_mock

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aicommon_mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// MockInvoker 实现 AIInvokeRuntime 接口用于测试
type MockInvoker struct {
	ctx    context.Context
	config aicommon.AICallerConfigIf
}

func NewMockInvoker(ctx context.Context) *MockInvoker {
	return &MockInvoker{
		ctx:    ctx,
		config: aicommon_mock.NewMockedAIConfig(ctx),
	}
}

func (m *MockInvoker) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return "Mock Basic Prompt Template: {{ .Query }}", map[string]any{
		"Query": "test query",
	}, nil
}

func (m *MockInvoker) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption) (*aicommon.Action, error) {
	log.Infof("mock InvokeLiteForge called with action: %s", actionName)

	if actionName == "memory-triage" {
		// 构造mock的返回数据
		mockResponseJSON := `{
			"@action": "memory-triage",
			"memory_entities": [
				{
					"content": "用户在实现一个复杂的AI记忆系统，使用C.O.R.E. P.A.C.T.框架进行记忆评分",
					"tags": ["AI开发", "记忆系统", "C.O.R.E. P.A.C.T."],
					"potential_questions": [
						"如何实现AI记忆系统？",
						"什么是C.O.R.E. P.A.C.T.框架？",
						"如何评估记忆的重要性？"
					],
					"t": 0.8,
					"a": 0.7,
					"p": 0.9,
					"o": 0.85,
					"e": 0.6,
					"r": 0.75,
					"c": 0.65
				},
				{
					"content": "系统需要支持语义搜索、按分数搜索和按标签搜索功能",
					"tags": ["搜索功能", "AI开发"],
					"potential_questions": [
						"如何实现语义搜索？",
						"什么是按分数搜索？",
						"如何按标签过滤记忆？"
					],
					"t": 0.7,
					"a": 0.8,
					"p": 0.6,
					"o": 0.9,
					"e": 0.5,
					"r": 0.8,
					"c": 0.7
				}
			]
		}`

		// 使用ExtractAction从JSON字符串创建Action
		action, err := aicommon.ExtractAction(mockResponseJSON, "memory-triage")
		if err != nil {
			return nil, utils.Errorf("failed to extract action: %v", err)
		}
		return action, nil
	}

	return nil, utils.Errorf("unexpected action: %s", actionName)
}

func (m *MockInvoker) ExecuteToolRequiredAndCall(name string) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (m *MockInvoker) AskForClarification(question string, payloads []string) string {
	return ""
}

func (m *MockInvoker) DirectlyAnswer(query string, tools []*aitool.Tool) (string, error) {
	return "", nil
}

func (m *MockInvoker) EnhanceKnowledgeAnswer(ctx context.Context, s string) (string, error) {
	return "", nil
}

func (m *MockInvoker) VerifyUserSatisfaction(query string, isToolCall bool, payload string) (bool, error) {
	return true, nil
}

func (m *MockInvoker) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
}

func (m *MockInvoker) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (m *MockInvoker) AddToTimeline(entry, content string) {
}

func (m *MockInvoker) GetConfig() aicommon.AICallerConfigIf {
	return m.config
}

func (m *MockInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return ""
}

func (m *MockInvoker) EmitResultAfterStream(any) {
}

func (m *MockInvoker) EmitResult(any) {
}
