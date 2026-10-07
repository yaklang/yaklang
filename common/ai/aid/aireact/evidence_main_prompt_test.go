package aireact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// Before freeze, evidence edits remain in Open; after freeze only the aggregate lives in semi.
func TestMainPromptEvidenceDeltasFollowTimelineFreeze(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	tl := react.config.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(react.config.AcquireId(), "BEFORE_EVIDENCE_MAIN")
	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "main-fact", Content: "FIRST_EVIDENCE_MAIN"}})
	tl.PushText(react.config.AcquireId(), "BETWEEN_EVIDENCE_MAIN")
	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "main-fact", Content: "LATEST_EVIDENCE_MAIN"}})
	for _, sealed := range []bool{false, true} {
		if sealed {
			tl.FreezeAll()
		}
		assembled, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "evidence"})
		require.NoError(t, err)
		prompt := assembled.Prompt
		var open string
		if !sealed {
			open = loopPromptSection(t, prompt, "timeline-open")
		} else {
			require.NotContains(t, prompt, aiprojection.CreateTemplate("<|PROMPT_SECTION_timeline-open|>"), "an empty open section is omitted after freeze")
		}
		semi := loopPromptSection(t, prompt, "semi-dynamic-1")
		require.NotContains(t, open, "<|SESSION_EVIDENCE_")
		if !sealed {
			previous := -1
			for _, text := range []string{"BEFORE_EVIDENCE_MAIN", "FIRST_EVIDENCE_MAIN", "BETWEEN_EVIDENCE_MAIN", "LATEST_EVIDENCE_MAIN"} {
				index := strings.Index(open, text)
				require.Greater(t, index, previous)
				previous = index
			}
			require.NotContains(t, semi, "EVIDENCE_MAIN")
		} else {
			require.NotContains(t, open, "EVIDENCE_MAIN")
			require.Contains(t, semi, "LATEST_EVIDENCE_MAIN")
			require.NotContains(t, prompt, "FIRST_EVIDENCE_MAIN")
			require.Equal(t, 1, strings.Count(prompt, "LATEST_EVIDENCE_MAIN"))
		}
	}
}
