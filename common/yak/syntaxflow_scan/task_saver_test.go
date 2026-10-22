package syntaxflow_scan

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestTaskSnapshotLatestWriteWinsAndStaysQueued(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowScanTask{}).Error)
	saver := newTaskSnapshotSaver(func() *gorm.DB { return db }, 0)
	t.Cleanup(saver.stop)

	first := uuid.NewString()
	second := uuid.NewString()
	saver.Enqueue(&schema.SyntaxFlowScanTask{TaskId: first, Status: schema.SYNTAXFLOWSCAN_EXECUTING, SuccessQuery: 1})
	saver.Enqueue(&schema.SyntaxFlowScanTask{TaskId: first, Status: schema.SYNTAXFLOWSCAN_EXECUTING, SuccessQuery: 4})
	saver.Enqueue(&schema.SyntaxFlowScanTask{TaskId: second, Status: schema.SYNTAXFLOWSCAN_EXECUTING, SuccessQuery: 2})
	_, err = schema.GetSyntaxFlowScanTaskById(db, first)
	require.Error(t, err, "coalesced snapshots are not durable before an explicit flush")

	require.NoError(t, saver.Flush())
	stored, err := schema.GetSyntaxFlowScanTaskById(db, first)
	require.NoError(t, err)
	require.Equal(t, int64(4), stored.SuccessQuery)
	other, err := schema.GetSyntaxFlowScanTaskById(db, second)
	require.NoError(t, err)
	require.Equal(t, int64(2), other.SuccessQuery)

	require.NoError(t, saver.Save(&schema.SyntaxFlowScanTask{
		TaskId: first, Status: schema.SYNTAXFLOWSCAN_DONE, SuccessQuery: 9,
	}))
	stored, err = schema.GetSyntaxFlowScanTaskById(db, first)
	require.NoError(t, err)
	require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, stored.Status)
	require.Equal(t, int64(9), stored.SuccessQuery)
	var rows int
	require.NoError(t, db.Model(&schema.SyntaxFlowScanTask{}).Where("task_id = ?", first).Count(&rows).Error)
	require.Equal(t, 1, rows, "task_id upsert must replace the row, not insert another")
}

func TestTaskSnapshotSaveErrorIsReturned(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.Close())
	saver := newTaskSnapshotSaver(func() *gorm.DB { return db }, 0)
	t.Cleanup(saver.stop)
	err = saver.Save(&schema.SyntaxFlowScanTask{TaskId: uuid.NewString(), Status: schema.SYNTAXFLOWSCAN_DONE})
	require.Error(t, err)

	task, err := createSyntaxflowTaskById(context.Background(), "", uuid.NewString(),
		mustScanConfig(t, ssaconfig.WithProgramNames("batch-save-error")),
	)
	require.NoError(t, err)
	task.snapshotSaver = saver
	task.markRuleSuccess(3)
	err = task.SaveTask()
	require.Error(t, err)
	_, err = schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), task.TaskId())
	require.Error(t, err, "a failed batch must not look saved through another database")
}

func TestTaskSnapshotConcurrentRulesPersistFinalCount(t *testing.T) {
	task, err := createSyntaxflowTaskById(context.Background(), "", uuid.NewString(),
		mustScanConfig(t, ssaconfig.WithProgramNames("batch-save-concurrent")),
	)
	require.NoError(t, err)
	const updates = 40
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task.markRuleSuccess(1)
			task.queueTaskSnapshot()
		}()
	}
	wg.Wait()
	task.status = schema.SYNTAXFLOWSCAN_DONE
	require.NoError(t, task.SaveTask())
	stored, err := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), task.TaskId())
	require.NoError(t, err)
	require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, stored.Status)
	require.Equal(t, int64(updates), stored.SuccessQuery)
	require.Equal(t, task.GetRiskCount(), stored.RiskCount)
}

func TestTerminalCallbackSeesDurableTask(t *testing.T) {
	taskID := uuid.NewString()
	var saw string
	var sawSuccess int64
	task, err := createSyntaxflowTaskById(context.Background(), "", taskID,
		mustScanConfig(t,
			ssaconfig.WithProgramNames("batch-save-terminal"),
			WithScanResultCallback(func(result *ScanResult) {
				stored, loadErr := schema.GetSyntaxFlowScanTaskById(ssadb.GetDB(), taskID)
				require.NoError(t, loadErr)
				saw = stored.Status
				sawSuccess = stored.SuccessQuery
				require.Equal(t, taskID, result.TaskID)
				require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, result.Status)
			}),
		),
	)
	require.NoError(t, err)
	task.markRuleSuccess(3)
	task.status = schema.SYNTAXFLOWSCAN_DONE
	require.NoError(t, task.SaveTask())
	task.notifyDone()
	require.Equal(t, schema.SYNTAXFLOWSCAN_DONE, saw)
	require.Equal(t, int64(3), sawSuccess)
}

func mustScanConfig(t *testing.T, opts ...ssaconfig.Option) *Config {
	t.Helper()
	cfg, err := NewConfig(opts...)
	require.NoError(t, err)
	return cfg
}
