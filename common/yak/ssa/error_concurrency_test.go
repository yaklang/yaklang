package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConcurrentUnitErrorsAreRetained(t *testing.T) {
	app := newDeferredBuildTestProgram(t, "concurrent-errors")
	const units, perUnit = 16, 128
	libraries := make([]*Program, units)
	for i := range libraries {
		libraries[i] = app.NewLibrary(fmt.Sprintf("unit%d", i), nil)
	}
	var wg sync.WaitGroup
	for _, lib := range libraries {
		wg.Add(1)
		go func(lib *Program) {
			defer wg.Done()
			for i := 0; i < perUnit; i++ {
				lib.AddError(&SSAError{Message: fmt.Sprintf("%s/%d", lib.Name, i)})
				_ = app.GetErrors()
			}
		}(lib)
	}
	wg.Wait()
	errors := app.GetErrors()
	require.Len(t, errors, units*perUnit)
	require.Len(t, libraries[0].GetErrors(), units*perUnit, "libraries must read the application error list")
	seen := make(map[string]bool, len(errors))
	for _, err := range errors {
		require.NotNil(t, err)
		require.False(t, seen[err.Message], "concurrent appends must not overwrite another unit's error")
		seen[err.Message] = true
	}
	snapshot := app.GetErrorsSnapshot()
	snapshot[0] = nil
	require.NotNil(t, app.GetErrors()[0], "snapshots must be isolated from the program's error slice")
}
