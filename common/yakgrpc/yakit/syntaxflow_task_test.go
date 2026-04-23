package yakit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQuerySyntaxFlowScanTask_OrderByUpdatedAtUsesIDDESCAsTieBreaker(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowScanTask{}).Error)

	programName := "prog-" + uuid.NewString()
	oldTaskID := "task-old-" + uuid.NewString()
	newTaskID := "task-new-" + uuid.NewString()

	oldTask := &schema.SyntaxFlowScanTask{
		TaskId:   oldTaskID,
		Programs: programName,
		Status:   schema.SYNTAXFLOWSCAN_DONE,
		Kind:     schema.SFResultKindScan,
	}
	newTask := &schema.SyntaxFlowScanTask{
		TaskId:   newTaskID,
		Programs: programName,
		Status:   schema.SYNTAXFLOWSCAN_DONE,
		Kind:     schema.SFResultKindScan,
	}

	require.NoError(t, db.Create(oldTask).Error)
	require.NoError(t, db.Create(newTask).Error)
	require.Greater(t, newTask.ID, oldTask.ID)

	sameUpdatedAt := time.Unix(2000, 0)
	require.NoError(t, db.Model(&schema.SyntaxFlowScanTask{}).Where("task_id = ?", oldTaskID).UpdateColumn("updated_at", sameUpdatedAt).Error)
	require.NoError(t, db.Model(&schema.SyntaxFlowScanTask{}).Where("task_id = ?", newTaskID).UpdateColumn("updated_at", sameUpdatedAt).Error)

	paginator, tasks, err := QuerySyntaxFlowScanTask(db, &ypb.QuerySyntaxFlowScanTaskRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "updated_at",
			Order:   "desc",
		},
		Filter: &ypb.SyntaxFlowScanTaskFilter{
			Programs: []string{programName},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, paginator)
	require.Len(t, tasks, 2)
	require.Equal(t, newTaskID, tasks[0].TaskId)
	require.Equal(t, oldTaskID, tasks[1].TaskId)
}
