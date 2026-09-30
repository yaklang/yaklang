package aireact

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"strings"
	"testing"
)

func TestPlanPromptLayersMainLoopCacheAndObservationAcrossModes(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, light := range []bool{false, true} {
			t.Run(map[bool]string{true: "native", false: "text"}[native]+"/"+map[bool]string{true: "light", false: "full"}[light], func(t *testing.T) {
				react, err := NewTestReAct()
				require.NoError(t, err)
				tl := react.config.GetTimeline()
				definition := tl.WrapPlanReferenceForPrompt("PLAN_DEFINITION", `{"plan_version":"v1","goal":"complete both nodes"}`)
				input := &reactloops.LoopPromptAssemblyInput{Nonce: "turn1", UserQuery: "original query", FunctionCallMode: native, Lightweight: light, Schema: `{"type":"object"}`, TodoSnapshot: "NODE_TODO_ONE", PlanContext: aicommon.PlanPromptContext{
					Version: "v1", Definition: definition, RuntimeState: tl.WrapPlanReferenceForPrompt("PLAN_RUNTIME_STATE", `{"plan_version":"v1","current_task_id":"node1","status":"processing"}`), ExecutionRules: "FRAMEWORK_PLAN_RULES",
				}, FrozenPartitions: []aicommon.FrozenBlockPartition{{ID: "plan_facts", Title: "Facts", Content: "FIXED_PLAN_FACT", Order: 100}, {ID: "plan_document", Title: "Document", Content: "FIXED_PLAN_DOCUMENT", Order: 110}}}
				first, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				frozenStart := aiprojection.CreateTemplate("<|AI_CACHE_FROZEN_semi-dynamic|>")
				frozenEnd := aiprojection.CreateTemplate("<|AI_CACHE_FROZEN_END_semi-dynamic|>")
				cached := func(prompt string) string {
					start, end := strings.Index(prompt, frozenStart), strings.Index(prompt, frozenEnd)
					require.GreaterOrEqual(t, start, 0)
					require.Greater(t, end, start)
					return prompt[start : end+len(frozenEnd)]
				}
				beforeFrozen := cached(first.Prompt)
				require.Contains(t, beforeFrozen, definition)
				require.Contains(t, beforeFrozen, "FIXED_PLAN_FACT")
				require.Contains(t, beforeFrozen, "FIXED_PLAN_DOCUMENT")
				require.Contains(t, planLayerTestSection(t, first.Prompt, "semi-dynamic-2"), "FRAMEWORK_PLAN_RULES")
				open := planLayerTestSection(t, first.Prompt, "timeline-open")
				require.Less(t, strings.Index(open, "# Plan Runtime State"), strings.Index(open, "NODE_TODO_ONE"))
				require.NotContains(t, open, "# Current Time")
				require.Contains(t, planLayerTestSection(t, first.Prompt, "dynamic_turn1"), "# Current Time")
				observations := mustLoopPromptSections(t, first.Sections)
				var definitionObserved, runtimeObserved, rulesObserved bool
				for _, section := range observations {
					for _, child := range section.Children {
						switch child.Key {
						case "section.frozen_block.partition.plan_definition":
							definitionObserved = true
							require.Contains(t, child.Content, definition)
						case "section.timeline_open.plan_runtime_state":
							runtimeObserved = true
							require.Equal(t, input.PlanContext.RuntimeState, child.Content)
						case "section.semi_dynamic_2.plan_execution_rules":
							rulesObserved = true
							require.Equal(t, input.PlanContext.ExecutionRules, child.Content)
						}
					}
				}
				require.True(t, definitionObserved)
				require.True(t, runtimeObserved)
				require.True(t, rulesObserved)
				input.Nonce = "turn2"
				input.UserQuery = "next query"
				input.TodoSnapshot = "NODE_TODO_TWO"
				input.PlanContext.RuntimeState = tl.WrapPlanReferenceForPrompt("PLAN_RUNTIME_STATE", `{"plan_version":"v1","current_task_id":"node2","status":"processing"}`)
				second, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Equal(t, beforeFrozen, cached(second.Prompt))
				require.Equal(t, planLayerTestSection(t, first.Prompt, "semi-dynamic-1"), planLayerTestSection(t, second.Prompt, "semi-dynamic-1"))
				require.Equal(t, planLayerTestSection(t, first.Prompt, "semi-dynamic-2"), planLayerTestSection(t, second.Prompt, "semi-dynamic-2"))
				require.NotEqual(t, open, planLayerTestSection(t, second.Prompt, "timeline-open"))
				input.PlanContext.Version = "v2"
				input.PlanContext.Definition = tl.WrapPlanReferenceForPrompt("PLAN_DEFINITION", `{"plan_version":"v2","goal":"new goal"}`)
				replanned, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.NotEqual(t, beforeFrozen, cached(replanned.Prompt))
			})
		}
	}
}

func planLayerTestSection(t *testing.T, prompt, name string) string {
	t.Helper()
	startTag := aiprojection.CreateTemplate("<|PROMPT_SECTION_" + name + "|>")
	endName := "END_" + name
	if turn, dynamic := strings.CutPrefix(name, "dynamic_"); dynamic {
		endName = "dynamic_END_" + turn
	}
	endTag := aiprojection.CreateTemplate("<|PROMPT_SECTION_" + endName + "|>")
	start := strings.Index(prompt, startTag)
	require.GreaterOrEqual(t, start, 0, "missing %s", name)
	start += len(startTag)
	end := strings.Index(prompt[start:], endTag)
	require.GreaterOrEqual(t, end, 0, "missing end %s", name)
	return prompt[start : start+end]
}
