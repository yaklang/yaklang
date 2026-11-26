package syntaxflow_scan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// An unreachable shared IR database leaves the process-global SSA handle nil:
// initYakitDatabase runs once, so a failed init is never retried in that
// process. Scan bookkeeping must degrade instead of dereferencing the nil
// handle, otherwise a source-only inspect scan dies before its first rule with
// "invalid memory address or nil pointer dereference".
func TestScanTaskBookkeepingWithNilSSADatabase(t *testing.T) {
	oldDB := ssadb.GetDB()
	require.NotNil(t, oldDB, "test process must own an initialized SSA database")
	ssadb.SetDB(nil)
	t.Cleanup(func() { ssadb.SetDB(oldDB) })

	cfg, err := NewConfig(ssaconfig.WithProgramNames("program-a"))
	require.NoError(t, err)
	manager := &scanManager{
		Config: cfg,
		ctx:    context.Background(),
		status: schema.SYNTAXFLOWSCAN_DONE,
		// captureTaskSnapshot reads only these counters.
		processMonitor: &processMonitor{},
	}

	require.NotPanics(t, func() { manager.setScanBatch() })
	require.EqualValues(t, 1, manager.taskRecorder.ScanBatch)

	require.NotPanics(t, func() {
		snapshot := manager.captureTaskSnapshot(true)
		require.NotNil(t, snapshot)
		require.Zero(t, snapshot.HighCount)
	})

	// Resume/status cannot read the task row without the database; it must
	// report that instead of panicking inside gorm too.
	require.NotPanics(t, func() {
		require.Error(t, manager.RestoreTask())
	})
}
