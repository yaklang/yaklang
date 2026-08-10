package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunDeferredBuildsForUnitsWithUnitCallback verifies that afterUnit
// is called exactly once per unitKey when all tasks for that unit complete,
// even if tasks from different units interleave.
func TestRunDeferredBuildsForUnitsWithUnitCallback(t *testing.T) {
	prog := NewTmpProgram("test-unit-callback")

	// Register deferred builds for two units: "unit-a" has 3 tasks, "unit-b" has 2.
	// Tasks are registered in interleaved order to test that afterUnit fires
	// only when a unit's remaining count hits zero.
	// BeginCompileUnit sets currentCompileUnit, which RegisterDeferredBuild uses.

	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a1", func() {})
	prog.BeginCompileUnit("unit-b")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "b1", func() {})
	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a2", func() {})
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a3", func() {})
	prog.BeginCompileUnit("unit-b")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "b2", func() {})
	prog.EndCompileUnit()

	var unitCompletedOrder []string
	var mu sync.Mutex
	var afterEachCount int

	ok := prog.RunDeferredBuildsForUnitsWithUnitCallback(
		[]string{"unit-a", "unit-b"},
		func(index int, total int) bool {
			afterEachCount++
			return true
		},
		func(unitKey string) bool {
			mu.Lock()
			unitCompletedOrder = append(unitCompletedOrder, unitKey)
			mu.Unlock()
			return true
		},
	)

	require.True(t, ok, "should complete all tasks")
	require.Equal(t, 5, afterEachCount, "afterEach should be called for each task")
	require.Equal(t, []string{"unit-a", "unit-b"}, unitCompletedOrder,
		"afterUnit should fire once per unit in completion order: unit-a (3 tasks) finishes before unit-b (2 tasks)")
}

// TestRunDeferredBuildsForUnitsWithUnitCallbackCancellation verifies that
// returning false from afterUnit stops execution.
func TestRunDeferredBuildsForUnitsWithUnitCallbackCancellation(t *testing.T) {
	prog := NewTmpProgram("test-cancel")

	prog.BeginCompileUnit("unit-a")
	for i := 0; i < 3; i++ {
		prog.RegisterDeferredBuild(DeferredBuildKindHelper, "cancel-"+string(rune('a'+i)), func() {})
	}
	prog.EndCompileUnit()

	callCount := 0
	ok := prog.RunDeferredBuildsForUnitsWithUnitCallback(
		[]string{"unit-a"},
		nil, // no afterEach
		func(unitKey string) bool {
			callCount++
			return false // cancel after first unit completion
		},
	)

	require.False(t, ok, "should return false when afterUnit returns false")
	require.Equal(t, 1, callCount, "afterUnit should be called once before cancellation")
}

// TestRunDeferredBuildsForUnitsBackwardCompat verifies that the original
// RunDeferredBuildsForUnits (without afterUnit) still works.
func TestRunDeferredBuildsForUnitsBackwardCompat(t *testing.T) {
	prog := NewTmpProgram("test-backward")

	prog.BeginCompileUnit("unit-x")
	for i := 0; i < 3; i++ {
		prog.RegisterDeferredBuild(DeferredBuildKindHelper, "compat-"+string(rune('a'+i)), func() {})
	}
	prog.EndCompileUnit()

	callCount := 0
	ok := prog.RunDeferredBuildsForUnits(
		[]string{"unit-x"},
		func(index int, total int) bool {
			callCount++
			return true
		},
	)

	require.True(t, ok)
	require.Equal(t, 3, callCount)
}
