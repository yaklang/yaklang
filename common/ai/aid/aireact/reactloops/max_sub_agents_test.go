package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestNormalizeDispatchJobs_RespectsMaxSubAgents(t *testing.T) {
	jobs := []SubAgentJob{
		{Goal: "scan a"},
		{Goal: "scan b"},
		{Goal: "scan c"},
	}
	_, err := NormalizeDispatchJobs(jobs, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 2")

	// Under the limit, loop_name registration is checked next; with no factories
	// registered in this package test, expect the factory error rather than the
	// MaxSubAgents error.
	_, err = NormalizeDispatchJobs(jobs[:2], 2)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "at most 2")
	require.Contains(t, err.Error(), "not registered")
}

func TestNormalizeMaxSubAgents(t *testing.T) {
	require.Equal(t, int64(aicommon.DefaultMaxSubAgents), aicommon.NormalizeMaxSubAgents(0))
	require.Equal(t, int64(aicommon.DefaultMaxSubAgents), aicommon.NormalizeMaxSubAgents(-1))
	require.Equal(t, int64(5), aicommon.NormalizeMaxSubAgents(5))
	require.Equal(t, int64(aicommon.AbsoluteMaxSubAgents), aicommon.NormalizeMaxSubAgents(aicommon.AbsoluteMaxSubAgents+10))
}
