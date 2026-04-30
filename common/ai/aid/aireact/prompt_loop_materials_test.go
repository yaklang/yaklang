package aireact

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestPromptManager_AssembleLoopPrompt_SectionOrder(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	react.promptManager.cpm.Register("provider-one", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "auto ctx body", nil
	})
	react.config.SetUserInputHistory([]schema.AIAgentUserInputRecord{
		{Round: 1, Timestamp: time.Date(2026, 4, 29, 10, 0, 0, 0, time.Local), UserInput: "previous input"},
	})
	react.AddToTimeline("test", "timeline content")

	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:           "n123",
		UserQuery:       "current user query",
		TaskInstruction: "follow task rules",
		OutputExample:   "example output",
		Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		SkillsContext:   "<|SKILLS_CONTEXT_skills_context|>\nloaded skill\n<|SKILLS_CONTEXT_END_skills_context|>",
		ReactiveData:    "reactive state",
		InjectedMemory:  "memory content",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Sections, 4)

	require.Equal(t, "section.high_static", result.Sections[0].Key)
	require.Equal(t, "section.semi_dynamic", result.Sections[1].Key)
	require.Equal(t, "section.timeline", result.Sections[2].Key)
	require.Equal(t, "section.dynamic", result.Sections[3].Key)

	prompt := result.Prompt
	traitsIdx := strings.Index(prompt, "<|TRAITS|>")
	workspaceIdx := strings.Index(prompt, "# Workspace Context")
	timelineIdx := strings.Index(prompt, "# Timeline Memory")
	currentTimeIdx := strings.Index(prompt, "# Current Time")
	toolInventoryIdx := strings.Index(prompt, "# Tool Inventory")
	skillsIdx := strings.Index(prompt, "<|SKILLS_CONTEXT_skills_context|>")
	schemaIdx := strings.Index(prompt, "<|SCHEMA|>")
	userQueryIdx := strings.Index(prompt, "<|USER_QUERY_n123|>")
	autoCtxIdx := strings.Index(prompt, "<|AUTO_PROVIDE_CTX_[n123_provider_one]_START key=provider-one|>")
	prevUserInputIdx := strings.Index(prompt, "<|PREV_USER_INPUT_n123|>")

	require.NotEqual(t, -1, traitsIdx)
	require.NotEqual(t, -1, workspaceIdx)
	require.NotEqual(t, -1, timelineIdx)
	require.NotEqual(t, -1, currentTimeIdx)
	require.NotEqual(t, -1, toolInventoryIdx)
	require.NotEqual(t, -1, skillsIdx)
	require.NotEqual(t, -1, schemaIdx)
	require.NotEqual(t, -1, userQueryIdx)
	require.NotEqual(t, -1, autoCtxIdx)
	require.NotEqual(t, -1, prevUserInputIdx)
	require.Less(t, traitsIdx, toolInventoryIdx)
	require.Less(t, toolInventoryIdx, skillsIdx)
	require.Less(t, skillsIdx, schemaIdx)
	require.Less(t, schemaIdx, timelineIdx)
	require.Less(t, traitsIdx, timelineIdx)
	require.Less(t, timelineIdx, currentTimeIdx)
	require.Less(t, currentTimeIdx, workspaceIdx)
	require.Less(t, currentTimeIdx, userQueryIdx)
	require.Less(t, userQueryIdx, autoCtxIdx)
	require.Less(t, autoCtxIdx, prevUserInputIdx)
	require.Contains(t, prompt, "<|PERSISTENT|>")
	require.Contains(t, prompt, "<|OUTPUT_EXAMPLE|>")
	require.Contains(t, prompt, "<|SCHEMA|>")
	require.NotContains(t, prompt, "<|SCHEMA_n123|>")

	require.Len(t, result.Sections[1].Children, 3)
	require.Equal(t, "section.semi_dynamic.tool_inventory", result.Sections[1].Children[0].Key)
	require.Equal(t, "section.semi_dynamic.skills_context", result.Sections[1].Children[1].Key)
	require.Equal(t, "section.semi_dynamic.schema", result.Sections[1].Children[2].Key)
	require.GreaterOrEqual(t, len(result.Sections[2].Children), 3)
	require.Equal(t, "section.timeline.timeline", result.Sections[2].Children[0].Key)
	require.Equal(t, "section.timeline.current_time", result.Sections[2].Children[1].Key)
	require.Equal(t, "section.timeline.workspace", result.Sections[2].Children[2].Key)
	require.GreaterOrEqual(t, len(result.Sections[3].Children), 3)
	require.Equal(t, "section.dynamic.user_query", result.Sections[3].Children[0].Key)
	require.Equal(t, "section.dynamic.auto_context", result.Sections[3].Children[1].Key)
	require.Equal(t, "section.dynamic.user_history", result.Sections[3].Children[2].Key)
}

func TestPromptManager_RenderLoopSemiDynamicSection_Order(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	rendered, err := react.promptManager.renderLoopSemiDynamicSection(&reactloops.PromptPrefixMaterials{
		ToolInventory:  true,
		ToolsCount:     2,
		TopToolsCount:  1,
		TopTools:       []*aitool.Tool{aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))},
		HasMoreTools:   true,
		ForgeInventory: true,
		AIForgeList:    "* `forge-a`: forge a desc",
		SkillsContext:  "<|SKILLS_CONTEXT_demo|>\nskill body\n<|SKILLS_CONTEXT_END_demo|>",
		Schema:         `{"type":"object","properties":{"@action":{"type":"string"}}}`,
	})
	require.NoError(t, err)

	toolIdx := strings.Index(rendered, "# Tool Inventory")
	forgeIdx := strings.Index(rendered, "# AI Blueprint Inventory")
	skillsIdx := strings.Index(rendered, "<|SKILLS_CONTEXT_demo|>")
	schemaIdx := strings.Index(rendered, "<|SCHEMA|>")
	require.NotEqual(t, -1, toolIdx)
	require.NotEqual(t, -1, forgeIdx)
	require.NotEqual(t, -1, skillsIdx)
	require.NotEqual(t, -1, schemaIdx)
	require.Less(t, toolIdx, forgeIdx)
	require.Less(t, forgeIdx, skillsIdx)
	require.Less(t, skillsIdx, schemaIdx)
}

func TestPromptManager_AssemblePromptPrefix(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	base, err := react.promptManager.GetLoopPromptBaseMaterials([]*aitool.Tool{tool}, "pfx123")
	require.NoError(t, err)

	prefix, err := react.promptManager.AssemblePromptPrefix(
		react.promptManager.NewPromptPrefixMaterials(base, &reactloops.LoopPromptAssemblyInput{
			Nonce:           "pfx123",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, prefix)
	require.Len(t, prefix.Sections, 3)
	require.Contains(t, prefix.Prompt, "<|TRAITS|>")
	require.Contains(t, prefix.Prompt, "<|SCHEMA|>")
	require.Equal(t, "section.high_static", prefix.Sections[0].Key)
	require.Equal(t, "section.semi_dynamic", prefix.Sections[1].Key)
	require.Equal(t, "section.timeline", prefix.Sections[2].Key)
}
