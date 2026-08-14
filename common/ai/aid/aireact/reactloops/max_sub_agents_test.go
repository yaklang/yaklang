package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestNormalizeDispatchJobs_RespectsAbsoluteMax(t *testing.T) {
	jobs := make([]SubAgentJob, int(aicommon.AbsoluteMaxSubAgentConcurrency)+1)
	for i := range jobs {
		jobs[i] = SubAgentJob{Goal: "scan"}
	}
	_, err := NormalizeDispatchJobs(jobs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most")
}

func TestResolveSubAgentConcurrency(t *testing.T) {
	require.Equal(t, int(aicommon.DefaultMaxSubAgentConcurrency), ResolveSubAgentConcurrency(0, 10))
	require.Equal(t, 5, ResolveSubAgentConcurrency(0, 10))
	require.Equal(t, 6, ResolveSubAgentConcurrency(6, 10))
	require.Equal(t, 3, ResolveSubAgentConcurrency(6, 3))
	require.Equal(t, int(aicommon.AbsoluteMaxSubAgentConcurrency), ResolveSubAgentConcurrency(int(aicommon.AbsoluteMaxSubAgentConcurrency)+5, 100))
}

func TestGetMaxSubAgentsDefaults(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	require.Equal(t, int64(aicommon.DefaultMaxSubAgentConcurrency), cfg.GetMaxSubAgents())
	require.Equal(t, int64(5), cfg.GetMaxSubAgents())

	cfg = aicommon.NewConfig(context.Background(), aicommon.WithMaxSubAgents(0))
	require.Equal(t, int64(5), cfg.GetMaxSubAgents())

	cfg = aicommon.NewConfig(context.Background(), aicommon.WithMaxSubAgents(100))
	require.Equal(t, int64(aicommon.AbsoluteMaxSubAgentConcurrency), cfg.GetMaxSubAgents())
}
