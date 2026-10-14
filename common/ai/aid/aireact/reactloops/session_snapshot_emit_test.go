package reactloops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestBuildSessionSnapshot_AlwaysEmitsFullPayload(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))
	snapshot := BuildSessionSnapshot(cfg, nil, nil)
	require.NotNil(t, snapshot)

	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Contains(t, payload, "revision")
	require.Contains(t, payload, "updated_at")
	require.Contains(t, payload, "execution")
	require.Contains(t, payload, "perception")
	require.Contains(t, payload, "capabilities")
	require.Contains(t, payload, "background_processes")

	var execution map[string]any
	require.NoError(t, json.Unmarshal(payload["execution"], &execution))
	require.Equal(t, "processing", execution["status"])
	require.Contains(t, execution, "execution_rounds")
}

func TestBuildSessionSnapshot_ExecutionRoundsFromLoop(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))
	loop := NewMinimalReActLoop(cfg, nil)
	task := aicommon.NewStatefulTaskBase("rounds-task", "input", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	loop.currentIterationIndex = 4

	snapshot := BuildSessionSnapshot(cfg, loop, task)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Execution)
	require.Equal(t, 4, snapshot.Execution.ExecutionRounds)
}

func TestEmitSessionSnapshotLoadedSkillTokenCounts(t *testing.T) {
	captured := map[string][]byte{}
	emitter := aicommon.NewEmitter("coordinator", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event != nil && (event.NodeId == aicommon.SessionSnapshotNodeID || event.NodeId == aicommon.CapabilityInventoryNodeID) {
			captured[event.NodeId] = append([]byte(nil), event.Content...)
		}
		return event, nil
	})
	cfg := aicommon.NewConfig(context.Background(),
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithEmitter(emitter))
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loaded-skill/SKILL.md", "---\nname: loaded-skill\ndescription: Loaded guide\n---\nBODY_IN_TIMELINE")
	vfs.AddFile("metadata-skill/SKILL.md", "---\nname: metadata-skill\ndescription: Available guide\n---\nUNLOADED_BODY")
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	bindSkillsManagerToTimeline(mgr, cfg)
	require.NoError(t, mgr.LoadSkill("loaded-skill"))
	loop := NewMinimalReActLoop(cfg, nil)
	loop.skillsContextManager = mgr
	expected := mgr.GetCurrentSelectedSkills()[0].TokenCount

	EmitSessionSnapshot(cfg, loop, nil)
	require.Contains(t, captured, aicommon.SessionSnapshotNodeID)
	require.Contains(t, captured, aicommon.CapabilityInventoryNodeID)
	var snapshot aicommon.SessionSnapshot
	require.NoError(t, json.Unmarshal(captured[aicommon.SessionSnapshotNodeID], &snapshot))
	skills := map[string]aicommon.CapabilityInventoryItem{}
	for _, item := range snapshot.Capabilities {
		if item.Type == "skill" {
			skills[item.Name] = item
		}
	}
	require.Equal(t, float64(expected), skills["loaded-skill"].Data.(map[string]any)["token_count"])
	require.NotContains(t, skills["metadata-skill"].Data.(map[string]any), "token_count")
	var legacy aicommon.CapabilityInventoryPayload
	require.NoError(t, json.Unmarshal(captured[aicommon.CapabilityInventoryNodeID], &legacy))
	require.Len(t, legacy.Fixed.Skills, 2)
	require.Equal(t, "loaded-skill", legacy.Fixed.Skills[0].Name)
	require.Equal(t, expected, legacy.Fixed.Skills[0].TokenCount)
	require.Equal(t, "metadata-skill", legacy.Fixed.Skills[1].Name)
	require.Zero(t, legacy.Fixed.Skills[1].TokenCount)
}
