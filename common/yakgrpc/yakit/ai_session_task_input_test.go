package yakit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newAISessionTaskInputTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISessionTaskInput{}).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func TestAISessionTaskInputSaveQueryAndCurrentResources(t *testing.T) {
	db := newAISessionTaskInputTestDB(t)
	sessionID := "task-input-" + uuid.NewString()
	otherSessionID := "task-input-other-" + uuid.NewString()

	fileResource := AISessionTaskResource{
		Type: "file", Key: "file_path", Value: "/tmp/workspace.txt",
		Scope: AISessionTaskResourceScopeSession, Source: AISessionTaskResourceSourceAttachedFilePath,
	}
	browserA := AISessionTaskResource{
		Type: "browser", Key: "browser_device", Value: `{"deviceId":"a"}`,
		Scope: AISessionTaskResourceScopeSession, Source: AISessionTaskResourceSourceAttachedResourceInfo,
	}
	selectedFirst := AISessionTaskResource{
		Type: "selected", Key: "content", Value: "first",
		Scope: AISessionTaskResourceScopeTurn, Source: AISessionTaskResourceSourceAttachedResourceInfo,
	}
	task1UUID := "task-" + uuid.NewString()
	require.NoError(t, SaveAISessionTaskInput(db, &SaveAISessionTaskInputParams{
		SessionID: sessionID, TaskID: "task-1", TaskUUID: task1UUID,
		UserInputUUID: "input-1", UserInput: "first task", InputSource: "ide",
		AttachedFilePaths:  []string{fileResource.Value, " " + fileResource.Value + " "},
		ExplicitResources:  []AISessionTaskResource{fileResource, browserA, selectedFirst},
		EffectiveResources: []AISessionTaskResource{fileResource, browserA, selectedFirst},
	}))

	selectedSecond := AISessionTaskResource{
		Type: "selected", Key: "content", Value: "second",
		Scope: AISessionTaskResourceScopeTurn, Source: AISessionTaskResourceSourceAttachedResourceInfo,
	}
	require.NoError(t, SaveAISessionTaskInput(db, &SaveAISessionTaskInputParams{
		SessionID: sessionID, TaskID: "task-2", TaskUUID: "task-" + uuid.NewString(),
		UserInputUUID: "input-2", UserInput: "second task",
		ExplicitResources:  []AISessionTaskResource{selectedSecond},
		EffectiveResources: []AISessionTaskResource{browserA, selectedSecond},
	}))

	browserB := AISessionTaskResource{
		Type: "browser", Key: "browser_device", Value: `{"deviceId":"b"}`,
		Scope: AISessionTaskResourceScopeSession, Source: AISessionTaskResourceSourceAttachedResourceInfo,
	}
	task3UUID := "task-" + uuid.NewString()
	require.NoError(t, SaveAISessionTaskInput(db, &SaveAISessionTaskInputParams{
		SessionID: sessionID, TaskID: "task-3", TaskUUID: task3UUID,
		UserInputUUID: "input-3", UserInput: "third task",
		ExplicitResources:  []AISessionTaskResource{browserB},
		EffectiveResources: []AISessionTaskResource{browserB},
	}))
	require.NoError(t, SaveAISessionTaskInput(db, &SaveAISessionTaskInputParams{
		SessionID: otherSessionID, TaskID: "other-task", TaskUUID: "task-" + uuid.NewString(),
		UserInput: "other session",
	}))

	paginator, records, err := QueryAISessionTaskInputs(db, &AISessionTaskInputFilter{
		SessionID: sessionID,
	}, &ypb.Paging{Page: 1, Limit: 2})
	require.NoError(t, err)
	require.Equal(t, 3, paginator.TotalRecord)
	require.Len(t, records, 2)
	require.Equal(t, "task-3", records[0].TaskID)
	require.Equal(t, "task-2", records[1].TaskID)

	latest, err := GetLatestAISessionTaskInput(db, sessionID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, "task-3", latest.TaskID)

	current, err := GetAISessionCurrentResources(db, sessionID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, "task-3", current.TaskID)
	require.Equal(t, []AISessionTaskResource{browserB}, current.TaskResources)
	require.ElementsMatch(t, []AISessionTaskResource{fileResource, browserB}, current.SessionResources)

	require.NoError(t, SaveAISessionTaskInput(db, &SaveAISessionTaskInputParams{
		SessionID: sessionID, TaskID: "task-1", TaskUUID: task1UUID,
		UserInputUUID: "input-1", UserInput: "first task updated",
		ExplicitResources:  []AISessionTaskResource{fileResource, browserA, selectedFirst},
		EffectiveResources: []AISessionTaskResource{fileResource, browserA, selectedFirst},
	}))
	updated, err := GetAISessionTaskInputByTaskUUID(db, task1UUID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "first task updated", updated.UserInput)

	var count int64
	require.NoError(t, db.Model(&schema.AISessionTaskInput{}).
		Where("session_id = ?", sessionID).Count(&count).Error)
	require.EqualValues(t, 3, count)

	require.NoError(t, DeleteAISessionTaskInputsBySessionID(db, sessionID))
	require.NoError(t, db.Model(&schema.AISessionTaskInput{}).
		Where("session_id = ?", sessionID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&schema.AISessionTaskInput{}).
		Where("session_id = ?", otherSessionID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestAISessionTaskInputSchemaUsesNoForeignKeysOrCompositeIndexes(t *testing.T) {
	db := newAISessionTaskInputTestDB(t)

	var foreignKeys []struct {
		ID    int    `gorm:"column:id"`
		Seq   int    `gorm:"column:seq"`
		Table string `gorm:"column:table"`
		From  string `gorm:"column:from"`
		To    string `gorm:"column:to"`
	}
	require.NoError(t, db.Raw(`PRAGMA foreign_key_list("ai_session_task_inputs_v1")`).Scan(&foreignKeys).Error)
	require.Empty(t, foreignKeys)

	var indexes []struct {
		Seq     int    `gorm:"column:seq"`
		Name    string `gorm:"column:name"`
		Unique  int    `gorm:"column:unique"`
		Origin  string `gorm:"column:origin"`
		Partial int    `gorm:"column:partial"`
	}
	require.NoError(t, db.Raw(`PRAGMA index_list("ai_session_task_inputs_v1")`).Scan(&indexes).Error)
	require.NotEmpty(t, indexes)
	for _, index := range indexes {
		var columns []struct {
			Seq  int    `gorm:"column:seqno"`
			CID  int    `gorm:"column:cid"`
			Name string `gorm:"column:name"`
		}
		require.NoError(t, db.Raw(fmt.Sprintf(`PRAGMA index_info("%s")`, index.Name)).Scan(&columns).Error)
		require.LessOrEqual(t, len(columns), 1, "index %s must not be composite", index.Name)
	}
}
