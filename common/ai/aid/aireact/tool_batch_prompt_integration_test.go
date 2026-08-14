package aireact

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func exactToolCallJSONsFromExamples(t *testing.T, examples string) []string {
	t.Helper()
	const actionStart = "{\n  \"@action\""
	var exacts []string
	for cursor := 0; cursor < len(examples); {
		relativeStart := strings.Index(examples[cursor:], actionStart)
		if relativeStart < 0 {
			break
		}
		start := cursor + relativeStart
		decoder := json.NewDecoder(strings.NewReader(examples[start:]))
		var parsed map[string]any
		require.NoError(t, decoder.Decode(&parsed), "every taught action example must be complete JSON")
		end := start + int(decoder.InputOffset())
		exacts = append(exacts, examples[start:end])
		cursor = end
	}
	require.Len(t, exacts, 2, "each tool action must teach one scalar and one batch example")
	return exacts
}

func toolBatchActionsSchemaForPrompt(t *testing.T, actions ...*reactloops.LoopAction) string {
	t.Helper()
	names := make([]string, 0, len(actions))
	opts := make([]any, 0, len(actions)*4+1)
	for _, action := range actions {
		require.NotNil(t, action)
		names = append(names, action.ActionType)
	}
	opts = append(opts, aitool.WithStringParam(
		"@action",
		aitool.WithParam_EnumString(names...),
		aitool.WithParam_Required(true),
	))
	for _, action := range actions {
		for _, option := range action.Options {
			opts = append(opts, option)
		}
	}
	return aitool.NewObjectSchema(opts...)
}

// This test closes the prompt-placement loop: it takes the registered built-in
// actions, renders their real schema into a complete main-loop prompt, and
// proves that the same executable scalar and batch examples tested by
// loopinfra CI are visible to the model inside the semi-dynamic Schema section.
func TestToolCallExamplesAreInAssembledMainLoopSchema(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			response := i.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	direct, ok := reactloops.GetLoopAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	require.True(t, ok)
	required, ok := reactloops.GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
	require.True(t, ok)
	schemaText := toolBatchActionsSchemaForPrompt(t, direct, required)
	directExamples := exactToolCallJSONsFromExamples(t, direct.OutputExamples)
	requireExamples := exactToolCallJSONsFromExamples(t, required.OutputExamples)
	require.Contains(t, directExamples[0], `"directly_call_tool_calls"`, "direct batch must be taught before the scalar fallback")
	require.Contains(t, directExamples[1], `"directly_call_tool_name"`)
	require.Contains(t, requireExamples[0], `"tool_require_calls"`, "require batch must be taught before the scalar fallback")
	require.Contains(t, requireExamples[1], `"tool_require_payload"`)
	var emittedSchema map[string]any
	require.NoError(t, json.Unmarshal([]byte(schemaText), &emittedSchema))
	properties := emittedSchema["properties"].(map[string]any)
	for _, placement := range []struct {
		field string
		exact string
	}{
		{field: "directly_call_tool_calls", exact: directExamples[0]},
		{field: "directly_call_tool_name", exact: directExamples[1]},
		{field: "tool_require_calls", exact: requireExamples[0]},
		{field: "tool_require_payload", exact: requireExamples[1]},
	} {
		fieldSchema := properties[placement.field].(map[string]any)
		require.Contains(t, fieldSchema["description"].(string), placement.exact,
			"the authoritative schema description must teach the matching exact form")
	}

	result, err := react.promptManager.AssembleLoopPrompt(
		nil,
		&reactloops.LoopPromptAssemblyInput{
			Nonce:           "batch-prompt-ci",
			UserQuery:       "inspect two independent files",
			TaskInstruction: "use the published action schema",
			OutputExample:   "base example",
			Schema:          schemaText,
		},
	)
	require.NoError(t, err)
	require.Contains(t, result.Prompt, "先枚举本轮已经明确、可立即执行的真实工具调用")
	require.Contains(t, result.Prompt, "不得仅为沿用单工具而拆成多轮")
	require.NotContains(t, result.Prompt, "默认单步")
	require.NotContains(t, result.Prompt, "探索 / 上游不确定 / 需要逐步收紧时的默认形态")

	sectionStart := strings.Index(result.Prompt, "<|PROMPT_SECTION_semi-dynamic-2|>")
	sectionEnd := strings.Index(result.Prompt, "<|PROMPT_SECTION_END_semi-dynamic-2|>")
	require.NotEqual(t, -1, sectionStart)
	require.Greater(t, sectionEnd, sectionStart)

	for _, examples := range [][]string{directExamples, requireExamples} {
		for _, exact := range examples {
			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(exact), &parsed), "the taught example itself must be legal JSON")

			encoded, err := json.Marshal(exact)
			require.NoError(t, err)
			// Schema is JSON, so its description necessarily JSON-escapes the inner
			// example. Strip only the outer string quotes before checking the prompt.
			escapedExact := string(encoded[1 : len(encoded)-1])
			exampleIndex := strings.Index(result.Prompt, escapedExact)
			require.Greater(t, exampleIndex, sectionStart)
			require.Less(t, exampleIndex, sectionEnd)
		}
	}
}

func TestLegacyBasePromptUsesTheSameBatchFirstSelectionPolicy(t *testing.T) {
	require.Contains(t, basePrompt, "先枚举本轮已经明确、可立即执行的真实调用")
	require.Contains(t, basePrompt, "优先走独立并发批次")
	require.Contains(t, basePrompt, "“任务属于探索阶段”本身不是拒绝并发的理由")
	require.NotContains(t, basePrompt, "默认单步")
	require.NotContains(t, basePrompt, "探索 / 上游不确定 / 需要逐步收紧时的默认形态")
}
