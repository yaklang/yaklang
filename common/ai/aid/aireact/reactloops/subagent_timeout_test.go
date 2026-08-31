package reactloops

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveSubAgentJobTimeout(t *testing.T) {
	require.Equal(t, time.Duration(0), resolveSubAgentJobTimeout(SubAgentJob{}, SubAgentOptions{}))
	require.Equal(t, 30*time.Minute, resolveSubAgentJobTimeout(
		SubAgentJob{}, SubAgentOptions{DefaultJobTimeout: 30 * time.Minute}))
	require.Equal(t, 10*time.Minute, resolveSubAgentJobTimeout(
		SubAgentJob{Timeout: 10 * time.Minute},
		SubAgentOptions{DefaultJobTimeout: 30 * time.Minute}))
}
