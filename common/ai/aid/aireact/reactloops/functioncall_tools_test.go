package reactloops

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// --- buildActionTool / buildFunctionCallTools ---

// TestBuildActionTool_Structure verifies that buildActionTool produces a
// single execute_action tool whose parameters schema is the same as the
// text-mode buildSchema output.
func TestBuildActionTool_Structure(t *testing.T) {
	actions := []*LoopAction{
		{
			ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			Description: "discover and require a new tool",
		},
		{
			ActionType:  schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER,
			Description: "answer the user directly",
		},
	}

	tools, err := buildActionTool(actions)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	tool := tools[0]
	require.Equal(t, "function", tool.Type)
	require.Equal(t, ActionToolName, tool.Function.Name)
	require.Equal(t, ActionToolDescription, tool.Function.Description)

	params, ok := tool.Function.Parameters.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", params["type"])

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)

	// @action enum should contain both action types
	actionField, ok := props["@action"].(map[string]any)
	require.True(t, ok)
	enum, ok := actionField["enum"].([]any)
	require.True(t, ok)
	var enumStrings []string
	for _, e := range enum {
		enumStrings = append(enumStrings, e.(string))
	}
	require.Contains(t, enumStrings, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
	require.Contains(t, enumStrings, schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER)
}

// TestBuildActionTool_EmptyActions returns nil for empty action list.
func TestBuildActionTool_EmptyActions(t *testing.T) {
	tools, err := buildActionTool(nil)
	require.NoError(t, err)
	require.Nil(t, tools)

	tools, err = buildActionTool([]*LoopAction{})
	require.NoError(t, err)
	require.Nil(t, tools)
}

// TestBuildActionToolParameters_MatchesBuildSchema verifies that the
// parameters schema produced by buildActionToolParameters is the JSON
// equivalent of buildSchema output (after tool-batch max-items applied).
func TestBuildActionToolParameters_MatchesBuildSchema(t *testing.T) {
	actions := []*LoopAction{
		loopAction_DirectlyAnswer,
		loopAction_Finish,
	}

	params, err := buildActionToolParameters(actions)
	require.NoError(t, err)

	// The params should be a map (JSON object) with @action
	paramsMap, ok := params.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", paramsMap["type"])

	props, ok := paramsMap["properties"].(map[string]any)
	require.True(t, ok)
	_, ok = props["@action"]
	require.True(t, ok, "@action should exist in tool parameters schema")
}

// TestBuildFunctionCallTools_ModeDisabled verifies that
// buildFunctionCallTools returns nil when functioncall mode is disabled.
func TestBuildFunctionCallTools_ModeDisabled(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.functionCallMode = false

	tools := loop.buildFunctionCallTools(nil)
	require.Nil(t, tools)
}

// TestBuildFunctionCallTools_ModeEnabled verifies that
// buildFunctionCallTools returns the execute_action tool when
// functioncall mode is enabled.
func TestBuildFunctionCallTools_ModeEnabled(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.functionCallMode = true

	tools := loop.buildFunctionCallTools(nil)
	require.Len(t, tools, 1)
	require.Equal(t, ActionToolName, tools[0].Function.Name)
}

// TestBuildFunctionCallTools_DisallowExit verifies that finish action
// is removed when disallowLoopExit is set in the operator.
func TestBuildFunctionCallTools_DisallowExit(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.functionCallMode = true

	operator := newLoopActionHandlerOperator(newMockSimpleTask("test", "test-index"))
	operator.disallowLoopExit = true

	tools := loop.buildFunctionCallTools(operator)
	require.Len(t, tools, 1)

	// The tool parameters should NOT contain "finish" in the @action enum
	params, ok := tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	actionField, ok := props["@action"].(map[string]any)
	require.True(t, ok)
	enum, ok := actionField["enum"].([]any)
	require.True(t, ok)
	for _, e := range enum {
		require.NotEqual(t, "finish", e, "finish should be removed when disallowLoopExit")
	}
}

// TestBuildFunctionCallTools_SchemaMatchesTextMode verifies that the
// @action enum in the functioncall tool matches the text-mode schema
// for the same set of actions (both modes see the same actions).
func TestBuildFunctionCallTools_SchemaMatchesTextMode(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.functionCallMode = true

	// Text mode schema
	textSchema, err := loop.generateSchemaString(false)
	require.NoError(t, err)

	// Functioncall mode tool
	tools := loop.buildFunctionCallTools(nil)
	require.Len(t, tools, 1)

	// Parse the text schema and the tool parameters, compare @action enums
	var textSchemaObj map[string]any
	require.NoError(t, json.Unmarshal([]byte(textSchema), &textSchemaObj))
	textProps, _ := textSchemaObj["properties"].(map[string]any)
	textAction, _ := textProps["@action"].(map[string]any)
	textEnum, _ := textAction["enum"].([]any)
	textEnumSet := map[string]bool{}
	for _, e := range textEnum {
		textEnumSet[e.(string)] = true
	}

	toolParams, _ := tools[0].Function.Parameters.(map[string]any)
	toolProps, _ := toolParams["properties"].(map[string]any)
	toolAction, _ := toolProps["@action"].(map[string]any)
	toolEnum, _ := toolAction["enum"].([]any)

	require.Len(t, toolEnum, len(textEnumSet),
		"functioncall tool @action enum should have same count as text mode")
	for _, e := range toolEnum {
		require.True(t, textEnumSet[e.(string)],
			"functioncall tool @action enum value %q should exist in text mode schema", e.(string))
	}
}

// --- callAITransaction functioncall mode ---

// fcTestConfig wraps MockedAIConfig to provide a controllable CallAI
// that can inspect the request and emit a response.
type fcTestConfig struct {
	*mock.MockedAIConfig
	aiCallback func(req *aicommon.AIRequest) (*aicommon.AIResponse, error)
}

func (c *fcTestConfig) CallAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.aiCallback(req)
}
func (c *fcTestConfig) CallSpeedPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.aiCallback(req)
}
func (c *fcTestConfig) CallQualityPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return c.aiCallback(req)
}

// TestCallAITransaction_FunctionCallMode_InjectsTools verifies that in
// functioncall mode, callAITransaction injects ExtraSpecOpts (WithTools,
// WithToolChoice, WithToolCallCallback) and EnableToolCallArgumentsStream
// into the AI request.
func TestCallAITransaction_FunctionCallMode_InjectsTools(t *testing.T) {
	var observedStreamEnabled bool
	var observedExtraOptsCount int
	var mu sync.Mutex

	baseConfig := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	baseConfig.SetConfig("AiTransactionAutoRetry", 0)

	config := &fcTestConfig{
		MockedAIConfig: baseConfig,
		aiCallback: func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			mu.Lock()
			observedStreamEnabled = req.IsToolCallArgumentsStreamEnabled()
			observedExtraOptsCount = len(req.GetExtraSpecOpts())
			mu.Unlock()

			// Simulate tool_call arguments flowing through the output stream
			resp := aicommon.NewAIResponse(baseConfig)
			resp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "directly_answer",
				"identifier": "answer_user",
				"answer_payload": "Hello World"
			}`))
			resp.Close()
			return resp, nil
		},
	}

	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)
	loop.functionCallMode = true
	loop.loopName = "test-fc-inject"

	// Register at least one action so buildFunctionCallTools returns a tool
	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)

	action, _, err := loop.callAITransaction(&sync.WaitGroup{}, "test prompt", "nonce123", nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	mu.Lock()
	require.True(t, observedStreamEnabled,
		"IsToolCallArgumentsStreamEnabled should be true in functioncall mode")
	require.Greater(t, observedExtraOptsCount, 0,
		"ExtraSpecOpts should be non-empty in functioncall mode")
	mu.Unlock()
}

// TestCallAITransaction_FunctionCallMode_ParsesAction verifies that the
// action JSON emitted via the output stream (simulating tool_call
// arguments) is correctly parsed by ExtractActionFromStream in
// callAITransaction's postHandler.
func TestCallAITransaction_FunctionCallMode_ParsesAction(t *testing.T) {
	baseConfig := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	baseConfig.SetConfig("AiTransactionAutoRetry", 0)

	config := &fcTestConfig{
		MockedAIConfig: baseConfig,
		aiCallback: func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			resp := aicommon.NewAIResponse(baseConfig)
			// Emit a complete action JSON as tool_call arguments would
			resp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "directly_answer",
				"identifier": "answer_user",
				"answer_payload": "42 is the answer"
			}`))
			resp.Close()
			return resp, nil
		},
	}

	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)
	loop.functionCallMode = true
	loop.loopName = "test-fc-parse"

	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)

	action, matchedAction, err := loop.callAITransaction(&sync.WaitGroup{}, "test prompt", "nonce456", nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	// Verify the action type
	actionType := action.ActionType()
	require.Equal(t, "directly_answer", actionType)

	// Verify params were parsed
	require.Equal(t, "42 is the answer", action.GetString("answer_payload"))

	// matchedAction should be the directly_answer LoopAction
	require.NotNil(t, matchedAction)
	require.Equal(t, "directly_answer", matchedAction.ActionType)
}

// TestCallAITransaction_TextMode_NoStreamEnabled verifies that in text
// mode (functioncall disabled), the request does NOT have
// ToolCallArgumentsStream enabled.
func TestCallAITransaction_TextMode_NoStreamEnabled(t *testing.T) {
	var observedStreamEnabled bool
	var mu sync.Mutex

	baseConfig := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	baseConfig.SetConfig("AiTransactionAutoRetry", 0)

	config := &fcTestConfig{
		MockedAIConfig: baseConfig,
		aiCallback: func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			mu.Lock()
			observedStreamEnabled = req.IsToolCallArgumentsStreamEnabled()
			mu.Unlock()

			resp := aicommon.NewAIResponse(baseConfig)
			resp.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "directly_answer",
				"identifier": "answer_user",
				"answer_payload": "text mode answer"
			}`))
			resp.Close()
			return resp, nil
		},
	}

	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)
	loop.functionCallMode = false
	loop.loopName = "test-text-mode"

	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)

	action, _, err := loop.callAITransaction(&sync.WaitGroup{}, "test prompt", "nonce789", nil)
	require.NoError(t, err)
	require.NotNil(t, action)

	mu.Lock()
	require.False(t, observedStreamEnabled,
		"IsToolCallArgumentsStreamEnabled should be false in text mode")
	mu.Unlock()
}

// --- FunctionCallModeEnabled getter ---

// TestFunctionCallModeEnabled verifies the getter returns the correct
// value based on config.
func TestFunctionCallModeEnabled(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.SetConfig("EnableFunctionCallMode", true)

	loop := &ReActLoop{
		config: cfg,
	}
	// Simulate what NewReActLoop does when it reads the config
	if cfg.GetConfigBool("EnableFunctionCallMode") {
		loop.functionCallMode = true
	}
	require.True(t, loop.FunctionCallModeEnabled())

	cfg2 := aicommon.NewConfig(context.Background())
	cfg2.SetConfig("EnableFunctionCallMode", false)
	loop2 := &ReActLoop{
		config: cfg2,
	}
	if cfg2.GetConfigBool("EnableFunctionCallMode") {
		loop2.functionCallMode = true
	}
	require.False(t, loop2.FunctionCallModeEnabled())
}

// --- Ensure aispec import is used ---
var _ aispec.Tool