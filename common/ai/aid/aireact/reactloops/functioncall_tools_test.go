package reactloops

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
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
	loop.actions.Set(nativeAdjustTodolistActionName, loopAction_AdjustTodolistNative)
	loop.functionCallMode = true
	textSchema, actionTags, err := loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	require.Empty(t, textSchema)
	require.NotEmpty(t, actionTags)
	require.Contains(t, actionTags, "<|FUNCTION_CALL_ACTION_SCHEMA_adjust_todolist_"+aiprojection.Nonce()+"|>")
	loop.functionCallMode = false
	textSchema, actionTags, err = loop.prepareLoopActionSchemas(nil)
	require.NoError(t, err)
	require.NotEmpty(t, textSchema)
	require.Empty(t, actionTags)
	require.NotContains(t, textSchema, "adjust_todolist")

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
		require.Equal(t, 1, strings.Count(tags, "<|FUNCTION_CALL_ACTION_SCHEMA_"+action.ActionType+"_"+aiprojection.Nonce()+"|>"))
		require.Contains(t, tags, "<|FUNCTION_CALL_ACTION_SCHEMA_END_"+action.ActionType+"_"+aiprojection.Nonce()+"|>")
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

func TestNativeAdjustTodolistKeepsTodoShapeWithoutBusinessSidecars(t *testing.T) {
	action := &LoopAction{ActionType: "todo_action"}
	tools, err := buildActionTools([]*LoopAction{action, loopAction_AdjustTodolistNative}, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	businessParameters := tools[0].Function.Parameters.(map[string]any)
	require.NotContains(t, businessParameters["properties"], "todo_delta")
	parameters := tools[1].Function.Parameters.(map[string]any)
	nativeProperties := parameters["properties"].(map[string]any)
	require.Len(t, nativeProperties, 1)
	native := nativeProperties["todo_delta"].(map[string]any)
	require.Contains(t, parameters["required"], "todo_delta")

	var textSchema map[string]any
	require.NoError(t, json.Unmarshal([]byte(buildSchema(action)), &textSchema))
	text := textSchema["properties"].(map[string]any)["todo_delta"].(map[string]any)
	require.Equal(t, withoutSchemaDescriptions(text), withoutSchemaDescriptions(native))
	require.Contains(t, native["description"], "arguments")
	require.NotContains(t, native["description"], "Open items form the Frontier")
	require.Contains(t, text["description"], "Open items form the Frontier")
	nativeJSON, err := json.Marshal(native)
	require.NoError(t, err)
	textJSON, err := json.Marshal(text)
	require.NoError(t, err)
	t.Logf("todo_delta schema bytes: native=%d text=%d", len(nativeJSON), len(textJSON))
}

func withoutSchemaDescriptions(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, child := range v {
			if key != "description" {
				result[key] = withoutSchemaDescriptions(child)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			result[i] = withoutSchemaDescriptions(child)
		}
		return result
	default:
		return value
	}
}
