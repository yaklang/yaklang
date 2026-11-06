package reactloops

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordRecoverableTimelineEventUsesSingleSharedFormat(t *testing.T) {
	loop, invoker, _ := newActionExecutionTestLoop(t)

	errorText := loop.recordRecoverableTimelineEvent(recoverableTimelineEvent{
		phase:             "execute_ai_output",
		action:            "write_file",
		iteration:         2,
		err:               errors.New("permission denied"),
		retryCount:        1,
		sideEffectUnknown: true,
		nextStep:          "inspect the file first",
	})

	require.Equal(t, "permission denied", errorText)
	entry := strings.Join(invoker.entries, "\n")
	require.Contains(t, entry, recoverableTimelineEntry+":")
	require.Contains(t, entry, "phase=execute_ai_output")
	require.Contains(t, entry, "action=write_file")
	require.Contains(t, entry, "iteration=2")
	require.Contains(t, entry, "error=permission denied")
	require.Contains(t, entry, "side_effect_started=unknown")
	require.Contains(t, entry, "retry_count=1")
	require.Contains(t, entry, "next_step=inspect the file first")
}
