package aid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPlanContextFromText_UsesAITagParser(t *testing.T) {
	input := `prefix
<|FACTS_nonce1|>
## Facts
- a
<|FACTS_END_nonce1|>

middle

<|EVIDENCE_nonce2|>
## Evidence
- b
<|EVIDENCE_END_nonce2|>
suffix`

	require.Equal(t, "## Facts\n- a", extractPlanFactsFromText(input))
	require.Equal(t, "## Evidence\n- b", extractPlanEvidenceFromText(input))
}

func TestStripPlanContextBlocks_RemovesAITagBlocksOnly(t *testing.T) {
	input := `before

<|FACTS_nonce1|>
## Facts
- a
<|FACTS_END_nonce1|>

between

<|PLAN_EVIDENCE_nonce2|>
## Evidence
- b
<|PLAN_EVIDENCE_END_nonce2|>

after`

	require.Equal(t, "before\n\nbetween\n\nafter", stripPlanContextBlocks(input))
}