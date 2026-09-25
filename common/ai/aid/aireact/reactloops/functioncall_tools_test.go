package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Other ReAct tests use this configurable mock caller.
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

func TestBuildActionToolsPromptTags(t *testing.T) {
	loop := makeSchemaStabilityTestLoop(aicommon.NewConfig(context.Background()))
	loop.functionCallMode = true
	textSchema, actionTags, err := loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	require.Empty(t, textSchema)
	require.NotEmpty(t, actionTags)
	loop.functionCallMode = false
	textSchema, actionTags, err = loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	require.NotEmpty(t, textSchema)
	require.Empty(t, actionTags)

	actions := []*LoopAction{
		{ActionType: "find_files", Options: []aitool.ToolOption{aitool.WithStringParam("pattern")}},
		{ActionType: "grep_text", Options: []aitool.ToolOption{aitool.WithStringParam("query")}},
	}
	tools, err := buildActionTools(actions, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	require.Len(t, tools, len(actions))
	tags, err := renderFunctionCallSchemaTags(tools)
	require.NoError(t, err)
	for _, action := range actions {
		require.Equal(t, 1, strings.Count(tags, "<|FUNCTION_CALL_ACTION_SCHEMA_"+action.ActionType+"|>"))
		require.Contains(t, tags, "<|FUNCTION_CALL_ACTION_SCHEMA_END_"+action.ActionType+"|>")
	}
	require.NotContains(t, tags, "<|FUNCTION_CALL_SCHEMAS_")
}

func TestBuildActionToolsDirectAnswerUsesArgumentsForLongContent(t *testing.T) {
	tools, err := buildActionTools([]*LoopAction{loopAction_DirectlyAnswer}, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	tool := tools[0]
	require.NotContains(t, tool.Function.Description, "FINAL_ANSWER")
	parameters, ok := tool.Function.Parameters.(map[string]any)
	require.True(t, ok)
	properties, ok := parameters["properties"].(map[string]any)
	require.True(t, ok)
	answer, ok := properties["answer_payload"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, answer["description"], "Complete answer")
	require.Contains(t, parameters["required"], "answer_payload")
	require.Contains(t, buildSchema(loopAction_DirectlyAnswer), "FINAL_ANSWER")
}

func TestBuildActionToolsUsesNativeDescription(t *testing.T) {
	textAction := &LoopAction{
		ActionType:        "inspect_result",
		Description:       "Emit JSON with @action and then an AITAG block.",
		NativeDescription: "Inspect the result using the query argument.",
		Options:           []aitool.ToolOption{aitool.WithStringParam("query")},
	}
	plainAction := &LoopAction{ActionType: "search_notes", Description: "Search saved notes."}
	registered := withNativeActionDescription(plainAction)
	require.Equal(t, "Search saved notes.", registered.NativeDescription)
	require.Empty(t, plainAction.NativeDescription)

	tools, err := buildActionTools([]*LoopAction{textAction, plainAction, {ActionType: "empty_description"}}, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	require.Equal(t, textAction.NativeDescription, tools[0].Function.Description)
	require.Equal(t, plainAction.Description, tools[1].Function.Description)
	require.Equal(t, "Run the empty_description action using its tool arguments.", tools[2].Function.Description)
	require.NotContains(t, tools[0].Function.Description, "@action")
}
