package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallReplaceValueTouchesOnlyMatchingSlots(t *testing.T) {
	call := &Call{
		anValue:   NewValue(),
		Method:    10,
		Args:      []int64{1, 2, 1},
		ArgMember: []int64{1, 3, 4},
		Binding:   map[string]int64{"a": 1, "b": 5},
	}
	from := NewConst(1)
	from.SetId(1)
	to := NewConst(9)
	to.SetId(9)

	call.ReplaceValue(from, to)

	require.Equal(t, int64(10), call.Method)
	require.Equal(t, []int64{9, 2, 9}, call.Args)
	require.Equal(t, []int64{9, 3, 4}, call.ArgMember)
	require.Equal(t, int64(9), call.Binding["a"])
	require.Equal(t, int64(5), call.Binding["b"])
}
