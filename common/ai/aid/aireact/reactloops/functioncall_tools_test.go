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
