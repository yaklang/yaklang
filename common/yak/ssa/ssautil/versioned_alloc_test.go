package ssautil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var versionedAllocationResult VersionedIF[*dummySSAValue]

func TestVersionedAssignmentStateAndAllocation(t *testing.T) {
	v := NewVersioned[*dummySSAValue](1, "x", true, nil)
	assertAssigned := func(want bool) {
		raw, err := v.MarshalJSON()
		require.NoError(t, err)
		var state struct {
			Assigned bool `json:"is_assigned"`
		}
		require.NoError(t, json.Unmarshal(raw, &state))
		require.Equal(t, want, state.Assigned)
	}
	assertAssigned(false)
	value := &dummySSAValue{id: 1}
	require.NoError(t, v.Assign(value))
	assertAssigned(true)
	require.NoError(t, v.Assign(value))
	require.Same(t, value, v.GetValue())
	allocs := testing.AllocsPerRun(100, func() {
		versionedAllocationResult = NewVersioned[*dummySSAValue](1, "x", true, nil)
	})
	require.Equal(t, float64(1), allocs, "assignment flag must live inside its version")
}
