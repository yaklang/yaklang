package syntaxflow_scan

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestNoSaveTaskSkipsProgressAndTerminalSnapshotWrites(t *testing.T) {
	task, err := createSyntaxflowTaskById(context.Background(), "", uuid.NewString(),
		mustScanConfig(t, ssaconfig.WithNoSaveTask(true), ssaconfig.WithProgramNames("readonly-ir")))
	require.NoError(t, err)
	var opened atomic.Int64
	saver := newTaskSnapshotSaver(func() *gorm.DB { opened.Add(1); return nil }, 0)
	t.Cleanup(saver.stop)
	task.snapshotSaver = saver
	task.markRuleSuccess(3)
	task.setRiskCount(2)
	task.queueTaskSnapshot()
	require.NoError(t, saver.Flush())
	task.status = schema.SYNTAXFLOWSCAN_DONE
	require.NoError(t, task.SaveTask())
	require.Zero(t, opened.Load(), "neither asynchronous progress nor terminal save may touch the IR database")
	require.Equal(t, int64(3), task.taskRecorder.SuccessQuery)
	require.Equal(t, int64(2), task.taskRecorder.RiskCount)
}
