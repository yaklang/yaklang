package aid

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/loop_coordinator"
)

func TestCoordinatorRecoveryResetsOnlyAffectedDependencies(t *testing.T) {
	s := loop_coordinator.Snapshot{Finished: true, Attempts: map[string]loop_coordinator.Attempt{
		"source":      {Task: loop_coordinator.Task{ID: "source", Index: "1"}, ID: 1, State: loop_coordinator.Accepted, Seen: true},
		"dependent":   {Task: loop_coordinator.Task{ID: "dependent", Index: "2", DependsOn: []string{"source"}}, ID: 2, State: loop_coordinator.Accepted, Seen: true},
		"independent": {Task: loop_coordinator.Task{ID: "independent", Index: "3"}, ID: 3, State: loop_coordinator.Accepted, Seen: true},
	}}
	require.NoError(t, resetCoordinatorRecovery(&s, "1"))
	require.False(t, s.Finished)
	require.Equal(t, loop_coordinator.Pending, s.Attempts["source"].State)
	require.Equal(t, loop_coordinator.Pending, s.Attempts["dependent"].State)
	require.Zero(t, s.Attempts["dependent"].ID)
	require.Equal(t, loop_coordinator.Accepted, s.Attempts["independent"].State)
	require.Error(t, resetCoordinatorRecovery(&s, "unknown"))
}
