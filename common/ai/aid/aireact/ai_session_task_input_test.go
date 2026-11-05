package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func findAISessionTaskResource(resources []yakit.AISessionTaskResource, typ, key, value string) *yakit.AISessionTaskResource {
	for i := range resources {
		resource := &resources[i]
		if resource.Type == typ && resource.Key == key && resource.Value == value {
			return resource
		}
	}
	return nil
}

func TestBuildAISessionTaskInputParamsCapturesEffectiveResources(t *testing.T) {
	react := &ReAct{config: &aicommon.Config{
		Ctx:                 context.Background(),
		PersistentSessionId: "session-task-input",
	}}
	browser := &ypb.AttachedResourceInfo{
		Type:  aicommon.AttachedResourceTypeBrowser,
		Key:   aicommon.AttachedResourceKeyBrowserDevice,
		Value: `{"deviceId":"device-a"}`,
	}
	firstEvent := &ypb.AIInputEvent{
		FreeInput:        "inspect the workspace",
		FocusModeLoop:    "code_security_audit",
		AttachedFilePath: []string{"/tmp/workspace"},
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			nil,
			browser,
			{Type: aicommon.AttachedResourceTypeSelected, Key: aicommon.AttachedResourceKeyContent, Value: "selection"},
			{Type: aicommon.USER_FREE_INPUT_UUID, Value: "input-uuid"},
			{Type: aicommon.USER_INPUT_SOURCE, Key: aicommon.USER_INPUT_SOURCE_KEY, Value: "schedule"},
			{Type: aicommon.USER_INPUT_SCHEDULE_CONTEXT, Key: aicommon.USER_INPUT_SCHEDULE_UUID, Value: "schedule-uuid"},
		},
	}
	firstTask := react.buildReTaskFromEvent(firstEvent)
	require.NotNil(t, firstTask)
	firstTask.SetFocusMode(firstEvent.GetFocusModeLoop())
	first := buildAISessionTaskInputParams("session-task-input", firstTask, firstEvent)
	require.NotNil(t, first)
	require.Equal(t, "session-task-input", first.SessionID)
	require.Equal(t, firstTask.GetId(), first.TaskID)
	require.Equal(t, firstTask.GetUUID(), first.TaskUUID)
	require.Equal(t, "input-uuid", first.UserInputUUID)
	require.Equal(t, "schedule", first.InputSource)
	require.Equal(t, "schedule-uuid", first.ScheduleUUID)
	require.Equal(t, "code_security_audit", first.FocusModeLoop)
	require.Equal(t, []string{"/tmp/workspace"}, first.AttachedFilePaths)

	fileResource := findAISessionTaskResource(
		first.ExplicitResources, aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/workspace",
	)
	require.NotNil(t, fileResource)
	require.Equal(t, yakit.AISessionTaskResourceScopeSession, fileResource.Scope)
	require.NotNil(t, findAISessionTaskResource(
		first.ExplicitResources, aicommon.AttachedResourceTypeBrowser, aicommon.AttachedResourceKeyBrowserDevice, browser.Value,
	))
	require.NotNil(t, findAISessionTaskResource(
		first.ExplicitResources, aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, "selection",
	))
	require.Nil(t, findAISessionTaskResource(first.ExplicitResources, aicommon.USER_FREE_INPUT_UUID, "", "input-uuid"))

	followUpEvent := &ypb.AIInputEvent{FreeInput: "continue"}
	followUpTask := react.buildReTaskFromEvent(followUpEvent)
	followUp := buildAISessionTaskInputParams("session-task-input", followUpTask, followUpEvent)
	require.NotNil(t, followUp)
	inheritedBrowser := findAISessionTaskResource(
		followUp.EffectiveResources, aicommon.AttachedResourceTypeBrowser, aicommon.AttachedResourceKeyBrowserDevice, browser.Value,
	)
	require.NotNil(t, inheritedBrowser)
	require.Equal(t, yakit.AISessionTaskResourceScopeSession, inheritedBrowser.Scope)
	require.Equal(t, yakit.AISessionTaskResourceSourceInherited, inheritedBrowser.Source)
	require.Empty(t, followUp.ExplicitResources)
}

func TestEnqueueReTaskPersistsAISessionTaskInput(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	require.NoError(t, db.AutoMigrate(&schema.AISessionTaskInput{}).Error)

	const sessionID = "session-task-input-enqueue"
	react := &ReAct{
		config: &aicommon.Config{
			Ctx:                       context.Background(),
			PersistentSessionId:       sessionID,
			BaseCheckpointableStorage: aicommon.NewCheckpointableStorageWithDB("react-test", db),
		},
		taskQueue: NewTaskQueue("session-task-input-test"),
	}
	event := &ypb.AIInputEvent{
		IsFreeInput:      true,
		FreeInput:        "inspect attached resources",
		FocusModeLoop:    "code_security_audit",
		AttachedFilePath: []string{"/tmp/workspace"},
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{Type: aicommon.USER_FREE_INPUT_UUID, Value: "input-uuid"},
			{
				Type:  aicommon.AttachedResourceTypeBrowser,
				Key:   aicommon.AttachedResourceKeyBrowserDevice,
				Value: `{"deviceId":"device-a"}`,
			},
		},
	}
	require.NoError(t, react.enqueueReTask(event))
	require.Equal(t, 1, react.taskQueue.GetQueueingCount())

	latest, err := yakit.GetLatestAISessionTaskInput(db, sessionID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, sessionID, latest.SessionID)
	require.Equal(t, event.GetFreeInput(), latest.UserInput)
	require.Equal(t, event.GetFocusModeLoop(), latest.FocusModeLoop)
	require.Equal(t, "input-uuid", latest.UserInputUUID)

	current, err := yakit.GetAISessionCurrentResources(db, sessionID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.NotNil(t, findAISessionTaskResource(
		current.TaskResources, aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/workspace",
	))
	require.NotNil(t, findAISessionTaskResource(
		current.TaskResources, aicommon.AttachedResourceTypeBrowser, aicommon.AttachedResourceKeyBrowserDevice, `{"deviceId":"device-a"}`,
	))
}
