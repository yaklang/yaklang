package reactloops

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type promptObservationTestInvoker struct {
	*mock.MockInvoker
}

func (i *promptObservationTestInvoker) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	topTool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("run tool a"))
	return "Mock Basic Prompt Template: {{ .Query }}", map[string]any{
		"Query":            "test query",
		"CurrentTime":      "2026-04-01 12:00:00",
		"OSArch":           "darwin/arm64",
		"WorkingDir":       "/tmp/test-project",
		"WorkingDirGlance": "tree:/tmp/test-project",
		"DynamicContext": "<|AUTO_PROVIDE_CTX_[abcd]_START key=test_ctx|>\ncurrent file: main.go\n<|AUTO_PROVIDE_CTX_[abcd]_END|>\n\n" +
			"<|PREV_USER_INPUT_nonceX|>\n# Session User Input History\n- Round 1 | Time: 2026-04-01 11:59:00 | User Input: previous input\n<|PREV_USER_INPUT_END_nonceX|>",
		"AllowPlan":         true,
		"ShowForgeList":     true,
		"AIForgeList":       "- forge-a\n- forge-b",
		"AllowToolCall":     true,
		"ToolsCount":        2,
		"TopToolsCount":     1,
		"TopTools":          []*aitool.Tool{topTool},
		"HasMoreTools":      true,
		"HasLoadCapability": true,
		"Timeline":          "step1\nstep2",
	}, nil
}

func TestGenerateLoopPrompt_RecordsObservation(t *testing.T) {
	invoker := &promptObservationTestInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
	config := invoker.GetConfig()
	loop := &ReActLoop{
		invoker:           invoker,
		loopName:          "prompt-observation-loop",
		config:            config,
		emitter:           config.GetEmitter(),
		actions:           omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:       omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:      omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:       omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:              omap.NewEmptyOrderedMap[string, any](),
		currentMemories:   omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](),
		extraCapabilities: NewExtraCapabilitiesManager(),
	}
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)
	WithPersistentInstruction("persistent instruction")(loop)
	WithReflectionOutputExample("example output")(loop)
	WithReactiveDataBuilder(func(loop *ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
		return "reactive context", nil
	})(loop)

	task := &mockSimpleTask{id: "test-task", index: "test-index"}
	operator := NewActionHandlerOperator(task)

	prompt, err := loop.generateLoopPrompt("nonce1", "raw user input", "memory content", operator)
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	observation := loop.GetLastPromptObservation()
	require.NotNil(t, observation)
	require.Equal(t, "prompt-observation-loop", observation.LoopName)
	require.Equal(t, "nonce1", observation.Nonce)
	require.Equal(t, len(prompt), observation.PromptBytes)
	require.Len(t, observation.Sections, 10)
	require.Greater(t, observation.SectionCount, len(observation.Sections))

	require.Equal(t, "background", observation.Sections[0].Key)
	require.Equal(t, PromptSectionRoleMixed, observation.Sections[0].Role)
	require.NotEmpty(t, observation.Sections[0].Children)
	require.Equal(t, "background.environment", observation.Sections[0].Children[0].Key)
	require.Greater(t, observation.Sections[0].ContentBytes(), 0)
	require.Greater(t, observation.Sections[0].LineCount(), 0)
	require.Empty(t, observation.Sections[0].Content)
	require.Equal(t, "background.dynamic_context", observation.Sections[0].Children[1].Key)
	require.Equal(t, "background.ai_forge_list", observation.Sections[0].Children[2].Key)
	require.Equal(t, "background.tool_inventory", observation.Sections[0].Children[3].Key)
	require.Equal(t, "background.timeline", observation.Sections[0].Children[4].Key)
	require.Equal(t, "background.dynamic_context.auto_provided", observation.Sections[0].Children[1].Children[0].Key)
	require.Equal(t, "background.dynamic_context.prev_user_input", observation.Sections[0].Children[1].Children[1].Key)

	require.Equal(t, "user_query", observation.Sections[1].Key)
	require.Equal(t, PromptSectionRoleUserInput, observation.Sections[1].Role)
	require.False(t, observation.Sections[1].Compressible)
	require.Equal(t, "raw user input", observation.Sections[1].Content)

	require.Equal(t, "session_evidence", observation.Sections[4].Key)
	require.True(t, observation.Sections[4].Compressible)

	require.Equal(t, "reactive_data", observation.Sections[6].Key)
	require.True(t, observation.Sections[6].IsIncluded())
	require.True(t, observation.Sections[6].Compressible)

	require.Equal(t, "schema", observation.Sections[8].Key)
	require.False(t, observation.Sections[8].Compressible)
	require.NotZero(t, observation.Stats.UserInputBytes)
	require.NotZero(t, observation.Stats.RuntimeCtxBytes)
	require.NotZero(t, observation.Stats.SystemPromptBytes)

	report := observation.RenderCLIReport(80)
	t.Logf("prompt observation cli report:\n%s", report)
	require.Contains(t, report, "Prompt Bytes:")
	require.Contains(t, report, "Section Tree")
	require.Contains(t, report, "Background / Environment")
	require.Contains(t, report, "key: background.environment")
	require.Contains(t, report, "key: background.tool_inventory")
	require.Contains(t, report, "meta: role=user_input, mode=fixed, included=yes")
	require.Contains(t, report, "summary: raw user input")
	require.NotContains(t, report, "Unified Capability Loading")

	status := loop.GetLastPromptObservationStatus()
	require.NotNil(t, status)
	require.Equal(t, observation.LoopName, status.LoopName)
	require.Equal(t, observation.Nonce, status.Nonce)
	require.Equal(t, observation.PromptBytes, status.PromptBytes)
	require.NotEmpty(t, status.Sections)
	require.Equal(t, "background", status.Sections[0].Key)
	require.NotEmpty(t, status.Sections[0].Children)
	require.Greater(t, status.Sections[0].Bytes, 0)
	require.Greater(t, status.Sections[0].Lines, 0)
	require.Empty(t, status.Sections[0].Summary)
	require.Equal(t, "background.tool_inventory", status.Sections[0].Children[3].Key)
	require.Contains(t, status.Sections[0].Children[3].Summary, "enabled_tools=2")
	require.Equal(t, "user_query", status.Sections[1].Key)
	require.Equal(t, "raw user input", status.Sections[1].Summary)

	loop.Set("prompt_observation_log", true)
}
