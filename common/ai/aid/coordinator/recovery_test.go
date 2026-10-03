package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorRecoveryResetsOnlyAffectedDependencies(t *testing.T) {
	s := Snapshot{Finished: true, Attempts: map[string]Attempt{
		"source":      {Task: Task{ID: "source", Index: "1"}, ID: 1, State: Accepted},
		"dependent":   {Task: Task{ID: "dependent", Index: "2", DependsOn: []string{"source"}}, ID: 2, State: Accepted},
		"independent": {Task: Task{ID: "independent", Index: "3"}, ID: 3, State: Accepted},
	}}
	require.NoError(t, resetCoordinatorRecovery(&s, "1"))
	require.False(t, s.Finished)
	require.Equal(t, Pending, s.Attempts["source"].State)
	require.Equal(t, Pending, s.Attempts["dependent"].State)
	require.Zero(t, s.Attempts["dependent"].ID)
	require.Equal(t, Accepted, s.Attempts["independent"].State)
	require.Error(t, resetCoordinatorRecovery(&s, "unknown"))
}
