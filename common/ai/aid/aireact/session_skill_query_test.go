package aireact

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSessionSkillInventorySyncWithoutLoop(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.Nil(t, react.GetCurrentLoop())
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "load-for-inventory",
		SyncJsonInput: `{"skill_names":["session"]}`,
	}))
	event := &ypb.AIInputEvent{SyncType: aicommon.SYNC_TYPE_SKILL_INVENTORY, SyncID: "inventory"}
	require.NoError(t, react.handleSyncMessage(event))
	var items []aicommon.CapabilityInventoryItem
	events.read(t, "skill_inventory", &items)
	require.Len(t, items, 2)
	byName := make(map[string]aicommon.CapabilityInventoryItem)
	for _, item := range items {
		require.Equal(t, "skill", item.Type)
		byName[item.Name] = item
	}
	require.Equal(t, aicommon.CapabilityInventoryStageLoaded, byName["session"].Stage)
	require.Equal(t, aicommon.CapabilityInventoryStageMetadata, byName["available"].Stage)
}

func TestSessionSkillDetailSyncLoadedAndAvailable(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	request := func(syncID, body string) aicommon.SessionSkillDetail {
		t.Helper()
		require.NoError(t, react.HandleSyncTypeSkillDetailEvent(&ypb.AIInputEvent{
			SyncType: aicommon.SYNC_TYPE_SKILL_DETAIL, SyncID: syncID, SyncJsonInput: body,
		}))
		var detail aicommon.SessionSkillDetail
		events.read(t, "skill_detail", &detail)
		return detail
	}

	available := request("available-detail", `{"skill_name":"available"}`)
	require.False(t, available.Loaded)
	require.Contains(t, available.SkillMDContent, "Available body")
	require.Empty(t, available.PromptContent)
	availableURL, err := url.Parse(available.URL)
	require.NoError(t, err)
	require.Equal(t, aicommon.SessionSkillResourceURLPath, availableURL.Path)
	require.Equal(t, "available", availableURL.Query().Get("skill_name"))

	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "load-session",
		SyncJsonInput: `{"skill_names":["session"]}`,
	}))
	loaded := request("loaded-detail", `{"name":"session"}`)
	require.True(t, loaded.Loaded)
	require.Empty(t, loaded.SkillMDContent)
	require.Contains(t, loaded.PromptContent, "=== Skill: session ===")
	require.Contains(t, loaded.PromptContent, "Session skill instructions")
	require.NotEmpty(t, loaded.URL)
}

func TestSessionSkillDetailSyncErrorsAreStructured(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.NoError(t, react.HandleSyncTypeSkillDetailEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_DETAIL, SyncID: "missing-detail",
		SyncJsonInput: `{"skill_name":"missing"}`,
	}))
	var response map[string]any
	events.read(t, "skill_detail", &response)
	require.Equal(t, "missing", response["name"])
	require.Contains(t, response["error"], "not found")
}

func TestSessionSkillInventoryUsesActiveLoopLoader(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loop-only/SKILL.md", "---\nname: loop-only\ndescription: Loop-only guide\n---\nLOOP_ONLY")
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	loop, err := reactloops.NewReActLoop("custom-inventory", react, reactloops.WithSkillLoader(loader))
	require.NoError(t, err)
	t.Cleanup(loop.Release)
	task := aicommon.NewStatefulTaskBase("custom-inventory-task", "test", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	require.Same(t, loop, react.GetCurrentLoop())

	require.NoError(t, react.HandleSyncTypeSkillInventoryEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_INVENTORY, SyncID: "custom-inventory",
	}))
	var items []aicommon.CapabilityInventoryItem
	events.read(t, "skill_inventory", &items)
	names := make(map[string]bool)
	for _, item := range items {
		names[item.Name] = true
	}
	require.True(t, names["loop-only"])
}

// JSON round-trip guard: the sync payload must be directly decodable by clients.
func TestSessionSkillQueryPayloadRoundTrip(t *testing.T) {
	raw, err := json.Marshal([]aicommon.CapabilityInventoryItem{{
		Name: "demo", Type: "skill", Stage: aicommon.CapabilityInventoryStageMetadata,
	}})
	require.NoError(t, err)
	var decoded []aicommon.CapabilityInventoryItem
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "demo", decoded[0].Name)
}
