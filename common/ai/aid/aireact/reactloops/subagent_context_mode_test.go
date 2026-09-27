package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackgroundSubAgents_InvalidContextModeDoesNotStartJobs(t *testing.T) {
	loop, task := backgroundSubAgentFixture(t, 1)
	_, err := loop.SubmitSubAgents(task, []SubAgentJob{{ContextMode: "clean"}}, SubAgentOptions{}, "bad-mode")
	require.ErrorContains(t, err, "context_mode")
	require.Nil(t, loop.GetSubAgentManager())
}
