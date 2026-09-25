package reactloops

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// promptCompareInvoker uses the real shared semi-dynamic-2 template while
// leaving the rest of the AI runtime mocked. No provider request is made.
type promptCompareInvoker struct{ *mock.MockInvoker }

func (i *promptCompareInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	materials := &aicommon.PromptMaterials{
		FunctionCallMode:    input.FunctionCallMode,
		TaskInstruction:     input.TaskInstruction,
		OutputExample:       input.OutputExample,
		Schema:              input.Schema,
		FunctionCallSchemas: input.FunctionCallSchemas,
	}
	section, err := aicommon.RenderPromptTemplate(
		"loop-prompt-compare-semi-dynamic-2",
		aicommon.SharedTaskInstructionSchemaExampleTemplate,
		materials.SemiDynamic2Data(),
	)
	if err != nil {
		return nil, err
	}
	return &aicommon.LoopPromptAssemblyResult{
		Prompt: "<|PROMPT_SECTION_semi-dynamic-2|>\n" + section + "\n<|PROMPT_SECTION_END_semi-dynamic-2|>",
	}, nil
}

func generateComparedLoopPrompt(t *testing.T, functionCallMode bool) string {
	t.Helper()
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx)
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.loopName = "prompt-compare"
	loop.functionCallMode = functionCallMode
	loop.persistentInstructionProvider = func(*ReActLoop, string) (string, error) {
		return "Choose an available action.", nil
	}
	loop.outputExampleProvider = func(*ReActLoop, string) (string, error) {
		return `{"@action":"compare"}`, nil
	}
	loop.invoker = &promptCompareInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	prompt, err := loop.generateLoopPrompt("compare", "compare action schemas", "", nil, "", nil)
	require.NoError(t, err)
	loop.WaitForInflightObservation()
	return prompt
}

func parseComparedSemiDynamic2(t *testing.T, prompt string) string {
	t.Helper()
	parsed, err := aitag.SplitViaTAG(prompt, "PROMPT_SECTION")
	require.NoError(t, err)
	require.Equal(t, prompt, parsed.String())
	sections := parsed.GetTaggedBlocks()
	require.Len(t, sections, 1)
	require.Equal(t, "semi-dynamic-2", sections[0].Nonce)
	return sections[0].Content
}

func TestExecLoopPromptCompare_TextAndFunctionCallSchemas(t *testing.T) {
	textPrompt := generateComparedLoopPrompt(t, false)
	functionPrompt := generateComparedLoopPrompt(t, true)
	require.NotEqual(t, textPrompt, functionPrompt)

	textSection := parseComparedSemiDynamic2(t, textPrompt)
	require.Contains(t, textSection, `{"@action":"compare"}`)
	require.NotContains(t, textSection, "<|FUNCTION_CALL_ACTION_SCHEMA_")
	const schemaTag = "<|SCHEMA|>"
	require.Equal(t, 2, strings.Count(textSection, schemaTag))
	_, afterOpen, ok := strings.Cut(textSection, schemaTag)
	require.True(t, ok)
	schemaBlock, _, ok := strings.Cut(afterOpen, schemaTag)
	require.True(t, ok)
	schemaBlock = strings.TrimSpace(schemaBlock)
	require.True(t, strings.HasPrefix(schemaBlock, "```jsonschema"))
	require.True(t, strings.HasSuffix(schemaBlock, "```"))
	schemaJSON := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(schemaBlock, "```jsonschema"), "```"))
	var textSchema map[string]any
	require.NoError(t, json.Unmarshal([]byte(schemaJSON), &textSchema))
	properties, ok := textSchema["properties"].(map[string]any)
	require.True(t, ok)
	actionSelector, ok := properties["@action"].(map[string]any)
	require.True(t, ok)
	actionNames, ok := actionSelector["enum"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, actionNames)

	functionSection := parseComparedSemiDynamic2(t, functionPrompt)
	require.NotContains(t, functionSection, `{"@action":"compare"}`)
	require.Contains(t, functionSection, "通过原生工具调用选择 action")
	require.NotContains(t, functionSection, schemaTag)
	parsedTools, err := aitag.SplitViaTAG(functionSection, "FUNCTION_CALL_ACTION_SCHEMA")
	require.NoError(t, err)
	require.Equal(t, functionSection, parsedTools.String())
	toolBlocks := parsedTools.GetTaggedBlocks()
	require.Len(t, toolBlocks, len(actionNames))
	seen := make(map[string]bool, len(toolBlocks))
	for _, block := range toolBlocks {
		require.Equal(t, "FUNCTION_CALL_ACTION_SCHEMA", block.TagName)
		var tool aispec.Tool
		require.NoError(t, json.Unmarshal([]byte(block.Content), &tool))
		require.Equal(t, "function", tool.Type)
		require.Equal(t, block.Nonce, tool.Function.Name)
		parameters, ok := tool.Function.Parameters.(map[string]any)
		require.True(t, ok)
		toolProperties, ok := parameters["properties"].(map[string]any)
		require.True(t, ok)
		require.NotContains(t, toolProperties, "@action")
		seen[block.Nonce] = true
	}
	for _, name := range actionNames {
		require.True(t, seen[name.(string)], "missing action tool %q", name)
	}
}

func TestExecLoopPromptCompare_HighStaticProtocol(t *testing.T) {
	render := func(functionCallMode bool) string {
		t.Helper()
		materials := &aicommon.PromptMaterials{FunctionCallMode: functionCallMode}
		prompt, err := aicommon.RenderPromptTemplate(
			"loop-prompt-compare-high-static",
			aicommon.SharedPlanAndExecHighStaticTemplate,
			materials.HighStaticData(),
		)
		require.NoError(t, err)
		return prompt
	}
	textPrompt := render(false)
	functionPrompt := render(true)
	require.NotEqual(t, textPrompt, functionPrompt)
	require.Contains(t, textPrompt, "caller 每轮给 JSON SCHEMA")
	require.Contains(t, textPrompt, "## NONCE 与 AITAG")
	require.NotContains(t, textPrompt, "本轮使用原生 function call")
	require.Contains(t, functionPrompt, "本轮使用原生 function call")
	require.Contains(t, functionPrompt, "模型输出的 AITAG 不参与 function call 的 action 解析")
	require.NotContains(t, functionPrompt, "caller 每轮给 JSON SCHEMA")
	require.NotContains(t, functionPrompt, "## NONCE 与 AITAG")
}
