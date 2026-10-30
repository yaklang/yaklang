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

func TestSessionSkillListsSyncWithoutLoop(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.Nil(t, react.GetCurrentLoop())
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "load-for-inventory",
		SyncJsonInput: `{"skill_names":["session"]}`,
	}))
	require.NoError(t, react.handleSyncMessage(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_LOADED, SyncID: "loaded",
	}))
	require.NoError(t, react.handleSyncMessage(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_AVAILABLE, SyncID: "available",
	}))
	var loaded aicommon.SessionSkillList
	events.read(t, "skill_loaded", &loaded)
	require.Equal(t, 1, loaded.Total)
	require.Equal(t, "session", loaded.Skills[0].Name)
	require.Equal(t, aicommon.CapabilityInventoryStageLoaded, loaded.Skills[0].Stage)

	var available aicommon.SessionSkillAvailablePage
	events.read(t, "skill_available", &available)
	require.Equal(t, 1, available.Total)
	require.Equal(t, "available", available.Skills[0].Name)
	require.Equal(t, aicommon.CapabilityInventoryStageMetadata, available.Skills[0].Stage)
}

func TestSessionSkillInventorySyncPagination(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.NoError(t, react.HandleSyncTypeSkillAvailableEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_AVAILABLE, SyncID: "available-page-2",
		SyncJsonInput: `{"page":2,"page_size":1}`,
	}))
	var inventory aicommon.SessionSkillAvailablePage
	events.read(t, "skill_available", &inventory)
	require.Equal(t, 2, inventory.Page)
	require.Equal(t, 1, inventory.PageSize)
	require.Equal(t, 2, inventory.Total)
	require.Len(t, inventory.Skills, 1)
	require.False(t, inventory.HasMore)
}

func TestSessionSkillLoadedSyncReturnsAllWithoutPagination(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "load-loaded",
		SyncJsonInput: `{"skill_names":["session"]}`,
	}))
	require.NoError(t, react.HandleSyncTypeSkillLoadedEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_LOADED, SyncID: "loaded",
		SyncJsonInput: `{"page":99,"page_size":1}`,
	}))
	var inventory aicommon.SessionSkillList
	events.read(t, "skill_loaded", &inventory)
	require.Equal(t, 1, inventory.Total)
	require.Len(t, inventory.Skills, 1)
	require.Equal(t, "session", inventory.Skills[0].Name)
	require.Equal(t, aicommon.CapabilityInventoryStageLoaded, inventory.Skills[0].Stage)
}

func TestSessionSkillAvailableSyncSearchPagination(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "load-before-search",
		SyncJsonInput: `{"skill_names":["session"]}`,
	}))
	require.NoError(t, react.HandleSyncTypeSkillAvailableEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_AVAILABLE, SyncID: "search-available",
		SyncJsonInput: `{"keyword":"guide","page":1,"page_size":1}`,
	}))
	var page aicommon.SessionSkillAvailablePage
	events.read(t, "skill_available", &page)
	require.Equal(t, "guide", page.Query)
	require.Equal(t, 1, page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 1, page.PageSize)
	require.Len(t, page.Skills, 1)
	require.Equal(t, "available", page.Skills[0].Name)
	require.Equal(t, aicommon.CapabilityInventoryStageMetadata, page.Skills[0].Stage)
	require.False(t, page.HasMore)
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

	require.NoError(t, react.HandleSyncTypeSkillAvailableEvent(&ypb.AIInputEvent{
		SyncType: aicommon.SYNC_TYPE_SKILL_AVAILABLE, SyncID: "custom-inventory",
	}))
	var inventory aicommon.SessionSkillAvailablePage
	events.read(t, "skill_available", &inventory)
	names := make(map[string]bool)
	for _, item := range inventory.Skills {
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
