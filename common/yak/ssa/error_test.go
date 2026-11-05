package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newErrorTestProgram(t *testing.T) *Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName("error-copy"))
	require.NoError(t, err)
	return NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 1)
}

// Sub programs report to the application's error list, so copying errors from a
// library back onto the application must not duplicate anything. The ts2ssa
// prehandler used to do this per built file; with a shared owner every file
// doubled the list and a large project died of quadratic memory growth.
func TestCopyErrorsFromSharedOwnerIsNoop(t *testing.T) {
	app := newErrorTestProgram(t)
	lib := app.NewLibrary("lib.js", []string{"lib.js"})
	require.NotNil(t, lib)
	require.Same(t, app, lib.errorOwner(), "library errors must live on the application")

	app.AddError(&SSAError{Message: "first"})
	require.Len(t, app.GetErrors(), 1)

	for i := 0; i < 10; i++ {
		app.CopyErrorsFrom(lib)
		require.Len(t, app.GetErrors(), 1, "shared-owner copy must not grow the error list")
	}
}

// Separate owners still receive the source errors exactly once.
func TestCopyErrorsFromSeparateOwnerTransfersOnce(t *testing.T) {
	app := newErrorTestProgram(t)
	other := newErrorTestProgram(t)
	require.NotSame(t, app.errorOwner(), other.errorOwner())

	other.AddError(&SSAError{Message: "from-other"})
	app.CopyErrorsFrom(other)
	require.Len(t, app.GetErrors(), 1)
	require.Equal(t, "from-other", app.GetErrors()[0].Message)

	// A second copy legitimately duplicates: the helper transfers the current
	// list, it does not dedupe across calls.
	app.CopyErrorsFrom(other)
	require.Len(t, app.GetErrors(), 2)
}

func TestGetErrorsReturnsLiveViewWithoutClone(t *testing.T) {
	app := newErrorTestProgram(t)
	app.AddError(&SSAError{Message: "first"})

	first := app.GetErrors()
	second := app.GetErrors()
	require.Len(t, first, 1)
	require.True(t, &first[0] == &second[0], "GetErrors must not clone the backing array")

	snapshot := app.GetErrorsSnapshot()
	require.Len(t, snapshot, 1)
	require.False(t, &first[0] == &snapshot[0], "snapshot must own a separate backing array")
}

func TestCopyErrorsFromNilIsSafe(t *testing.T) {
	app := newErrorTestProgram(t)
	require.NotPanics(t, func() {
		app.CopyErrorsFrom(nil)
		(*Program)(nil).CopyErrorsFrom(app)
	})
}
