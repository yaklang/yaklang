package sfvm

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleWorkBudgetReportsUsageAndCancelsOnce(t *testing.T) {
	var cancelCalls atomic.Int32
	budget := NewRuleWorkBudget(2, func() {
		cancelCalls.Add(1)
	})

	require.False(t, budget.EnterWork())
	require.False(t, budget.EnterWork())
	require.True(t, budget.EnterWork())
	require.True(t, budget.Exceeded())
	require.Equal(t, int64(3), budget.Visited())
	require.Equal(t, int64(2), budget.Limit())
	require.Equal(t, int32(1), cancelCalls.Load())

	// Callers may race or re-check after cancellation. The cancel callback is
	// still exactly-once, while the usage counter remains observable.
	require.True(t, budget.EnterWork())
	require.Equal(t, int64(4), budget.Visited())
	require.Equal(t, int32(1), cancelCalls.Load())
}

func TestRuleWorkBudgetDisabledDoesNotCount(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		budget := NewRuleWorkBudget(limit, func() {
			t.Fatal("disabled budget must not cancel")
		})
		require.False(t, budget.EnterWork())
		require.False(t, budget.Exceeded())
		require.Zero(t, budget.Visited())
		require.Equal(t, limit, budget.Limit())
	}
}
