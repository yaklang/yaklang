package aireact

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type sessionSkillTestEvents struct {
	mu      sync.Mutex
	payload map[string][]byte
}

func (c *sessionSkillTestEvents) read(t *testing.T, node string, target any) {
	t.Helper()
	c.mu.Lock()
	raw := append([]byte(nil), c.payload[node]...)
	c.mu.Unlock()
	require.NotEmpty(t, raw, "missing event %s", node)
	require.NoError(t, json.Unmarshal(raw, target))
}

func newSessionSkillTestRuntime(t *testing.T, invokerOnly bool) (*ReAct, *sessionSkillTestEvents) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	captured := &sessionSkillTestEvents{payload: make(map[string][]byte)}
	emitter := aicommon.NewEmitter("session-skills", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event != nil {
			captured.mu.Lock()
			captured.payload[event.NodeId] = append([]byte(nil), event.Content...)
			captured.mu.Unlock()
		}
		return event, nil
	})
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("session/SKILL.md", "---\nname: session\ndescription: Session guide\n---\n"+
		strings.Repeat("Session skill instructions with enough detail for pagination.\n", 1000))
	vfs.AddFile("available/SKILL.md", "---\nname: available\ndescription: Available guide\n---\nAvailable body.")
	opts := []aicommon.ConfigOption{
		aicommon.WithContext(ctx),
		aicommon.WithEmitter(emitter),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableMemoryTriage(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithSkillsFS(vfs),
	}
	if invokerOnly {
		runtime, err := BuildReActInvoker(ctx, opts...)
		require.NoError(t, err)
		return runtime.(*ReAct), captured
	}
	runtime, err := NewTestReAct(opts...)
	require.NoError(t, err)
	return runtime, captured
}

type sessionSkillLoadResponse struct {
	Loaded        []string          `json:"loaded"`
	Failed        map[string]string `json:"failed"`
	AlreadyLoaded []string          `json:"already_loaded"`
	Error         string            `json:"error"`
}

func TestSessionSkillManualLoadWithoutLoop(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	require.Nil(t, react.GetCurrentLoop())
	event := &ypb.AIInputEvent{
		IsSyncMessage: true, SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "manual-load",
		SyncJsonInput: `{"skill_names":["session","missing"]}`,
	}
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(event))
	var response sessionSkillLoadResponse
	events.read(t, "load_skill", &response)
	require.Empty(t, response.Error)
	require.Equal(t, []string{"session"}, response.Loaded)
	require.Contains(t, response.Failed, "missing")
	sessionMgr := react.config.GetSkillsContextManager()
	require.True(t, sessionMgr.IsSkillLoaded("session"))
	require.Contains(t, react.DumpTimeline(), "User forced load: session")

	// 无 loop 也推送已加载状态、正文 token 数和仍可加载的技能目录。
	var snapshot aicommon.SessionSnapshot
	events.read(t, aicommon.SessionSnapshotNodeID, &snapshot)
	skills := make(map[string]aicommon.CapabilityInventoryItem)
	for _, item := range snapshot.Capabilities {
		if item.Type == "skill" {
			skills[item.Name] = item
		}
	}
	require.Equal(t, aicommon.CapabilityInventoryStageLoaded, skills["session"].Stage)
	expectedTokens := sessionMgr.GetCurrentSelectedSkills()[0].TokenCount
	require.Equal(t, float64(expectedTokens), skills["session"].Data.(map[string]any)["token_count"])
	require.Contains(t, skills, "available")
	require.NotEqual(t, aicommon.CapabilityInventoryStageLoaded, skills["available"].Stage)

	var legacy aicommon.CapabilityInventoryPayload
	events.read(t, aicommon.CapabilityInventoryNodeID, &legacy)
	require.Len(t, legacy.Fixed.Skills, 2)
	require.Equal(t, "session", legacy.Fixed.Skills[1].Name)
	require.Equal(t, expectedTokens, legacy.Fixed.Skills[1].TokenCount)

	before := react.config.GetTimeline().VisibleSkillParts("")
	event.SyncJsonInput = `{"skill_names":["session"]}`
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(event))
	response = sessionSkillLoadResponse{}
	events.read(t, "load_skill", &response)
	require.Empty(t, response.Loaded)
	require.Empty(t, response.Failed)
	require.Equal(t, []string{"session"}, response.AlreadyLoaded)
	require.Equal(t, before, react.config.GetTimeline().VisibleSkillParts(""))

	// 后创建的真实 loop 保留独立 manager，并能直接翻阅 session 加载的内容。
	loop, err := reactloops.NewReActLoop("session-skill-test", react)
	require.NoError(t, err)
	t.Cleanup(loop.Release)
	loopMgr := loop.GetSkillsContextManager()
	require.NotNil(t, loopMgr)
	require.NotSame(t, sessionMgr, loopMgr)
	require.True(t, loopMgr.HasTruncatedViews())
	require.Equal(t, before, react.config.GetTimeline().VisibleSkillParts(""))
	require.NoError(t, loopMgr.ChangeViewOffset("session", "", 100))
	require.Contains(t, sessionMgr.GetSkillViewSummary("session"), "SKILL.md:100-")
	require.True(t, sessionMgr.UnloadSkill("session"))
	require.False(t, loopMgr.IsSkillLoaded("session"))
	require.False(t, loopMgr.HasTruncatedViews())
}

func TestSessionSkillHotloadWithoutLoop(t *testing.T) {
	for _, invokerOnly := range []bool{false, true} {
		name := "react"
		if invokerOnly {
			name = "invoker"
		}
		t.Run(name, func(t *testing.T) {
			react, events := newSessionSkillTestRuntime(t, invokerOnly)
			require.Nil(t, react.GetCurrentLoop())
			capability := aicommon.EnabledCapability{Name: "session", Type: aicommon.EnabledCapabilityTypeSkill}
			require.NoError(t, aicommon.WithEnabledCapabilities(capability)(react.config))
			mgr := react.config.GetSkillsContextManager()
			require.True(t, mgr.IsSkillLoaded("session"))
			require.Contains(t, react.DumpTimeline(), "User forced load: session")
			require.NoError(t, aicommon.WithDisabledCapabilities(capability)(react.config))
			require.False(t, mgr.IsSkillLoaded("session"))
			require.False(t, mgr.HasTruncatedViews())
			var inventory aicommon.CapabilityInventoryPayload
			events.read(t, aicommon.CapabilityInventoryNodeID, &inventory)
			for _, skill := range inventory.Fixed.Skills {
				require.Equal(t, aicommon.CapabilityInventorySkillLoadMetadata, skill.SkillLoadState)
				require.Zero(t, skill.TokenCount)
			}
		})
	}
}

func TestSessionSkillManualLoadPreservesLoopLoader(t *testing.T) {
	react, events := newSessionSkillTestRuntime(t, false)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loop-only/SKILL.md", "---\nname: loop-only\ndescription: Loop-specific guide\n---\nSpecialized body.")
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	loop, err := reactloops.NewReActLoop("custom-skill-loader", react, reactloops.WithSkillLoader(loader))
	require.NoError(t, err)
	t.Cleanup(loop.Release)
	task := aicommon.NewStatefulTaskBase("custom-loader-task", "test", react.config.GetContext(), nil, true)
	loop.SetCurrentTask(task)
	require.Same(t, loop, react.GetCurrentLoop())
	require.NoError(t, react.HandleSyncTypeLoadSkillEvent(&ypb.AIInputEvent{
		IsSyncMessage: true, SyncType: aicommon.SYNC_TYPE_LOAD_SKILL, SyncID: "loop-load",
		SyncJsonInput: `{"skill_names":["loop-only"]}`,
	}))
	var response sessionSkillLoadResponse
	events.read(t, "load_skill", &response)
	require.Equal(t, []string{"loop-only"}, response.Loaded)
	require.Empty(t, response.Failed, "an active loop's specialized loader must remain usable")
	require.True(t, react.config.GetSkillsContextManager().IsSkillLoaded("loop-only"),
		"the session sees the timeline content even when its own loader lacks this skill")
}
