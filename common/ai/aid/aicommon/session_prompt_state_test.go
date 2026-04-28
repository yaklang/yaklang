package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionPromptState_AppendReasoningTextTrimsOldContent(t *testing.T) {
	state := NewSessionPromptState()
	first := strings.Repeat("A", 3000)
	second := strings.Repeat("B", 3000)

	state.AppendReasoningText(first)
	state.AppendReasoningText(second)

	rendered := state.GetSessionReasoningRendered()
	require.NotEmpty(t, rendered)
	require.LessOrEqual(t, len([]rune(rendered)), sessionReasoningRuneBudget)
	require.Contains(t, rendered, "B")
	require.NotEqual(t, first+"\n\n---\n\n"+second, rendered)
}
