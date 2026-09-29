package aireact

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestEvidenceFunctionCallToolParamsFrozenAndSemiOneRouting(t *testing.T) {
	sections, err := newFunctionCallToolParamsPrefixBuilder().AssemblePromptPrefix(&aicommon.PromptMaterials{
		TimelineFrozen: "FROZEN_TIMELINE_R2", SessionEvidenceSemiDynamic: "PROMOTED_EVIDENCE_R2",
		OriginalUserInput: "COMPLETE_USER_INPUT_R2", PromotedSemiDynamic1: "STABLE_TIMELINE_R2",
		FunctionCallSchemas: "FIXED_TOOL_TAGS_R2", TaskInstruction: "SELECTED_TOOL_R2",
	})
	require.NoError(t, err)
	require.Contains(t, sections.FrozenBlock, "FROZEN_TIMELINE_R2")
	require.Contains(t, sections.SemiDynamic, "PROMOTED_EVIDENCE_R2")
	require.NotContains(t, sections.FrozenBlock, "PROMOTED_EVIDENCE_R2")
	require.NotContains(t, sections.FrozenBlock, "SELECTED_TOOL_R2")
	require.Contains(t, sections.SemiDynamic, "COMPLETE_USER_INPUT_R2")
	require.NotContains(t, sections.SemiDynamic, "STABLE_TIMELINE_R2")
	require.Contains(t, sections.SemiDynamic2, "FIXED_TOOL_TAGS_R2")
	require.Contains(t, sections.SemiDynamic2, "SELECTED_TOOL_R2")
	require.NotContains(t, sections.SemiDynamic2, "FROZEN_TIMELINE_R2")
}

// R2 uses the same chronological evidence journal as the parent loop. Before
// freeze, edits remain in Open; after freeze only the aggregate lives in semi.
func TestFunctionCallToolParamsEvidenceDeltasFollowTimelineFreeze(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	tl := react.config.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(react.config.AcquireId(), "BEFORE_EVIDENCE_R2")
	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "r2-fact", Content: "FIRST_EVIDENCE_R2"}})
	tl.PushText(react.config.AcquireId(), "BETWEEN_EVIDENCE_R2")
	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "r2-fact", Content: "LATEST_EVIDENCE_R2"}})
	selected := aitool.NewWithoutCallback("selected_reader", aitool.WithStringParam("path"))
	task := aicommon.NewStatefulTaskBase("r2-evidence-task", "TASK_CONTEXT_R2", context.Background(), react.config.GetEmitter())
	for _, sealed := range []bool{false, true} {
		if sealed {
			tl.FreezeAll()
		}
		prompt, err := react.promptManager.GenerateFunctionCallToolParamsPromptForTask(task, selected, aicommon.ToolParamsCallIntent{})
		require.NoError(t, err)
		var open string
		if !sealed {
			open = r2PromptSection(t, prompt, "timeline-open")
		} else {
			require.NotContains(t, prompt, aiprojection.CreateTemplate("<|PROMPT_SECTION_timeline-open|>"), "an empty open section is omitted after freeze")
		}
		semi := r2PromptSection(t, prompt, "semi-dynamic-1")
		require.NotContains(t, open, "<|SESSION_EVIDENCE_")
		if !sealed {
			previous := -1
			for _, text := range []string{"BEFORE_EVIDENCE_R2", "FIRST_EVIDENCE_R2", "BETWEEN_EVIDENCE_R2", "LATEST_EVIDENCE_R2"} {
				index := strings.Index(open, text)
				require.Greater(t, index, previous)
				previous = index
			}
			require.NotContains(t, semi, "EVIDENCE_R2")
		} else {
			require.NotContains(t, open, "EVIDENCE_R2")
			require.Contains(t, semi, "LATEST_EVIDENCE_R2")
			require.NotContains(t, prompt, "FIRST_EVIDENCE_R2")
			require.Equal(t, 1, strings.Count(prompt, "LATEST_EVIDENCE_R2"))
		}
	}
}
