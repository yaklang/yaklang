package aireact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestInputPinsAttachedFilesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	other := filepath.Join(dir, "other.yak")
	require.NoError(t, os.WriteFile(file, []byte("report"), 0o600))
	require.NoError(t, os.WriteFile(other, nil, 0o600))
	type pin struct {
		taskID string
		kind   schema.EventType
		path   string
	}
	var pins []pin
	react := &ReAct{
		config:    &aicommon.Config{Ctx: context.Background()},
		taskQueue: NewTaskQueue("input-pins"),
		Emitter: aicommon.NewEmitter("session", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			switch e.Type {
			case schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME, schema.EVENT_TYPE_FILESYSTEM_PIN_DIRECTORY:
				pins = append(pins, pin{e.TaskId, e.Type, e.GetContentJSONPath("$.path")})
			}
			return e, nil
		}),
	}
	id, err := react.enqueueReTaskWithID(&ypb.AIInputEvent{
		IsFreeInput: true, FreeInput: "inspect attachments", AttachedFilePath: []string{file},
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{Type: "file", Key: "file_path", Value: file},
			{Type: "file", Key: "file_path", Value: dir}, // Yakit @folder
			{Type: "file", Key: "directory_path", Value: dir},
			{Type: "file", Key: "file_path", Value: filepath.Join(dir, "missing")},
			{Type: "file", Key: "file_content", Value: other}, // literal, not a path
			{Type: "code", Key: "file_path", Value: other},    // editor target, not an attachment pin
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []pin{
		{id, schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME, file},
		{id, schema.EVENT_TYPE_FILESYSTEM_PIN_DIRECTORY, dir},
	}, pins)

	pins = nil
	_, err = react.enqueueReTaskWithID(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "continue"})
	require.NoError(t, err)
	require.Empty(t, pins)

	id, err = react.enqueueReTaskWithID(&ypb.AIInputEvent{
		IsFreeInput: true, FreeInput: "use the workspace again",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{Type: "file", Key: "directory_path", Value: dir},
			{Type: "file_path", Value: file},
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []pin{
		{id, schema.EVENT_TYPE_FILESYSTEM_PIN_DIRECTORY, dir},
		{id, schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME, file},
	}, pins)
}

func TestNormalizeInputAttachmentsUnifiesFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.txt")
	require.NoError(t, os.WriteFile(path, []byte("reference"), 0o600))
	react := &ReAct{config: &aicommon.Config{
		Ctx:                    context.Background(),
		ContextProviderManager: aicommon.NewContextProviderManager(),
	}, taskQueue: NewTaskQueue("normalized-attachments")}

	legacyEvent := &ypb.AIInputEvent{
		IsFreeInput:      true,
		FreeInput:        "analyze attachment",
		AttachedFilePath: []string{path, path},
	}
	legacyResources, legacyControl := normalizeInputAttachments(legacyEvent)
	require.Len(t, legacyResources, 2)
	require.Equal(t, aicommon.CONTEXT_PROVIDER_TYPE_FILE, legacyResources[0].Type)
	require.Equal(t, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, legacyResources[0].Key)
	require.Equal(t, path, legacyResources[0].Value)

	legacyTask := react.buildReTaskFromEvent(legacyEvent, legacyResources, legacyControl)
	require.Len(t, legacyTask.GetAttachedDatas(), 2)
	require.Equal(t, path, legacyTask.GetAttachedDatas()[0].Value)

	resourceEvent := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "analyze attachment",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
				Value: path,
			},
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
				Value: path,
			},
		},
	}
	resourceResources, _ := normalizeInputAttachments(resourceEvent)
	require.Equal(t, legacyResources, resourceResources)

	require.NoError(t, react.handleFreeValue(legacyEvent))
	require.Empty(t, react.config.ContextProviderManager.Execute(nil, nil), "file paths must be rendered from Task attachments, not ContextProvider")
}

func TestBuildAISessionTaskInputUsesNormalizedAttachments(t *testing.T) {
	react := &ReAct{config: &aicommon.Config{
		Ctx:                 context.Background(),
		PersistentSessionId: "normalized-attachments",
	}}
	event := &ypb.AIInputEvent{
		IsFreeInput:      true,
		FreeInput:        "inspect file",
		AttachedFilePath: []string{"/tmp/legacy.txt"},
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
				Value: "/tmp/resource.txt",
			},
			{
				Type:  aicommon.USER_FREE_INPUT_UUID,
				Key:   "default",
				Value: "input-uuid",
			},
			{
				Type:  aicommon.USER_INPUT_SOURCE,
				Key:   aicommon.USER_INPUT_SOURCE_KEY,
				Value: aicommon.USER_INPUT_SOURCE_SCHEDULE,
			},
			{
				Type:  aicommon.USER_INPUT_SCHEDULE_CONTEXT,
				Key:   aicommon.USER_INPUT_SCHEDULE_UUID,
				Value: "schedule-uuid",
			},
			{
				Type:  aicommon.USER_INPUT_SCHEDULE_CONTEXT,
				Key:   aicommon.USER_INPUT_SCHEDULE_NAME,
				Value: "daily check",
			},
		},
	}
	resources, control := normalizeInputAttachments(event)
	require.Equal(t, "input-uuid", control.userInputUUID)
	require.Equal(t, aicommon.USER_INPUT_SOURCE_SCHEDULE, control.inputSource)
	require.Equal(t, "schedule-uuid", control.schedule.uuid)
	require.Equal(t, "daily check", control.schedule.name)
	task := react.buildReTaskFromEvent(event, resources, control)
	params := react.buildSessionTaskInput(task, resources)
	require.NotNil(t, params)
	require.Len(t, params.Resources, 2)
	require.Equal(t, "input-uuid", params.UserInputUUID)
	require.Equal(t, aicommon.USER_INPUT_SOURCE_SCHEDULE, params.InputSource)
	require.Equal(t, "schedule-uuid", params.ScheduleUUID)
	require.Equal(t, "daily check", params.ScheduleName)
	require.NotNil(t, findAISessionTaskResource(
		params.Resources, aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/legacy.txt",
	))
	require.NotNil(t, findAISessionTaskResource(
		params.Resources, aicommon.CONTEXT_PROVIDER_TYPE_FILE, aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/resource.txt",
	))
}

func TestNormalizeInputAttachmentsKeepsDistinctLiteralFileContent(t *testing.T) {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "inspect file content",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT,
				Value: "",
			},
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT,
				Value: " ",
			},
			{
				Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
				Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT,
				Value: "",
			},
		},
	}
	resources, _ := normalizeInputAttachments(event)
	require.Len(t, resources, 3)
	require.Empty(t, resources[0].Value)
	require.Equal(t, " ", resources[1].Value)
	require.Empty(t, resources[2].Value)
}
