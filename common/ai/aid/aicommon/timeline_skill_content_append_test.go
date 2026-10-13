package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestSkillContentDuplicateIsCheckedBeforeAppend(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		t.Run(fmt.Sprintf("frozen=%t", frozen), func(t *testing.T) {
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
			tl := cfg.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			id := cfg.AcquireId()
			require.True(t, tl.PushSkillContent(id, "demo-skill", "", "SAME_BODY", SkillContentSourceForced))
			if frozen {
				tl.FreezeAll()
			}
			before := RenderTimelineFrozenOpen(tl)
			beforeSkills := tl.RenderSkillContentForPrompt()
			beforeFreeze := tl.FreezeSnapshot()

			require.False(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "SKILL.md",
				"SAME_BODY", SkillContentSourceAuto), "same content must not be appended even from another source")
			require.Equal(t, []int64{id}, tl.idToTimelineItem.Keys())
			require.Equal(t, before, RenderTimelineFrozenOpen(tl))
			require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
			require.Equal(t, beforeFreeze, tl.FreezeSnapshot())

			require.True(t, tl.DeleteSkillContent("demo-skill"))
			require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "SKILL.md",
				"SAME_BODY", SkillContentSourceManual), "retired content must be readable again")
			require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "SAME_BODY")
		})
	}
}

func TestSkillContentDifferentWindowsStayInHistory(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	firstID := cfg.AcquireId()
	require.True(t, tl.PushSkillContent(firstID, "demo-skill", "SKILL.md", "FIRST_WINDOW", SkillContentSourceAuto))
	tl.FreezeAll()
	before := RenderTimelineFrozenOpen(tl)
	beforeSkills := tl.RenderSkillContentForPrompt()

	secondID := cfg.AcquireId()
	require.True(t, tl.PushSkillContent(secondID, "demo-skill", "SKILL.md", "SECOND_WINDOW", SkillContentSourceAuto))
	require.Equal(t, before.Frozen, RenderTimelineFrozenOpen(tl).Frozen)
	require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "SECOND_WINDOW")

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 2)
	require.Equal(t, firstID, snapshot.Items[0].ID)
	require.True(t, snapshot.Items[0].Frozen)
	require.Contains(t, snapshot.Items[0].PromptText, "FIRST_WINDOW")
	require.Equal(t, secondID, snapshot.Items[1].ID)
	require.False(t, snapshot.Items[1].Frozen)
	require.Contains(t, snapshot.Items[1].PromptText, "SECOND_WINDOW")

	tl.FreezeAll()
	skills := tl.RenderSkillContentForPrompt()
	require.Contains(t, skills, "FIRST_WINDOW")
	require.Contains(t, skills, "SECOND_WINDOW")
	require.Less(t, strings.Index(skills, "FIRST_WINDOW"), strings.Index(skills, "SECOND_WINDOW"))
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)

	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, skills, restored.RenderSkillContentForPrompt())
}

func TestSkillContentCompressionSelectsHistoricalWindows(t *testing.T) {
	for _, retainBoth := range []bool{false, true} {
		t.Run(fmt.Sprintf("retainBoth=%t", retainBoth), func(t *testing.T) {
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
			tl := cfg.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			firstID, secondID := cfg.AcquireId(), cfg.AcquireId()
			require.True(t, tl.PushSkillContent(firstID, "demo-skill", "SKILL.md", "EARLIER_WINDOW", SkillContentSourceAuto))
			require.True(t, tl.PushSkillContent(secondID, "demo-skill", "SKILL.md", "LATER_WINDOW", SkillContentSourceAuto))
			tl.FreezeAll()
			bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
				require.Contains(t, req.GetPrompt(), "EARLIER_WINDOW")
				require.Contains(t, req.GetPrompt(), "LATER_WINDOW")
				retained := fmt.Sprint(firstID)
				if retainBoth {
					retained = fmt.Sprintf("%d-%d", firstID, secondID)
				}
				body, err := json.Marshal(map[string]any{
					"@action": "timeline-summary", "summary": "Skill reading history.",
					"ratain_timeline_item_range": retained, "memory_entities": []any{},
				})
				return string(body), err
			})

			result, err := tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.Contains(t, result.RetainedIDs, firstID, "an earlier window must be independently selectable")
			skills := tl.RenderSkillContentForPrompt()
			require.Contains(t, skills, "EARLIER_WINDOW")
			if retainBoth {
				require.Contains(t, result.RetainedIDs, secondID)
				require.Empty(t, result.RetiredIDs)
				require.Contains(t, skills, "LATER_WINDOW")
			} else {
				require.Equal(t, []int64{secondID}, result.RetiredIDs)
				require.NotContains(t, skills, "LATER_WINDOW")
				require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "SKILL.md",
					"LATER_WINDOW", SkillContentSourceAuto))
				require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "LATER_WINDOW")
				require.Equal(t, skills, tl.RenderSkillContentForPrompt(), "reloading must not rewrite the frozen skill block")
			}
		})
	}
}

func TestSkillContentConcurrentDuplicateLoadsAppendOnce(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	sink := NewSkillContentTimelineAdapter(tl, cfg.AcquireId)
	results := make(chan bool, 16)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- sink.AppendSkillContent("demo-skill", "SKILL.md", "CONCURRENT_BODY", SkillContentSourceManual)
		}()
	}
	wg.Wait()
	close(results)
	appended := 0
	for added := range results {
		if added {
			appended++
		}
	}
	require.Equal(t, 1, appended)
	require.Len(t, tl.idToTimelineItem.Keys(), 1)
	require.Equal(t, 1, strings.Count(RenderTimelineFrozenOpen(tl).Open, "CONCURRENT_BODY"))
}

func TestSkillManagerPagingPreservesPreviousWindow(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("demo-skill/SKILL.md", buildSkillMD("demo-skill", "Demo", "FIRST_LINE\nSECOND_LINE\nTHIRD_LINE"))
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	bindTestTimeline(mgr, cfg)
	require.NoError(t, mgr.LoadSkill("demo-skill"))
	tl.FreezeAll()
	beforeSkills := tl.RenderSkillContentForPrompt()
	beforeIDs := tl.idToTimelineItem.Keys()

	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "", 2))
	require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs)+1)
	require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "Current Offset: 2")
	tl.FreezeAll()
	skills := tl.RenderSkillContentForPrompt()
	require.Contains(t, skills, "FIRST_LINE")
	require.Contains(t, skills, "Current Offset: 2")
	require.Equal(t, 2, strings.Count(skills, "File: SKILL.md (Skill: demo-skill)"))

	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "", 2))
	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "", 1))
	require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs)+1, "rereading either retained window must be idempotent")
	require.Equal(t, skills, tl.RenderSkillContentForPrompt())
}

func TestSkillManagerReloadsRetiredContentDespiteOtherVisibleItems(t *testing.T) {
	for _, resource := range []bool{false, true} {
		t.Run(fmt.Sprintf("resource=%t", resource), func(t *testing.T) {
			loader, err := aiskillloader.NewFSSkillLoader(buildSkillsVFS())
			require.NoError(t, err)
			mgr := aiskillloader.NewSkillsContextManager(loader)
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
			tl := cfg.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			bindTestTimeline(mgr, cfg)
			require.NoError(t, mgr.LoadSkill("deploy-app"))
			if resource {
				_, err = mgr.LoadSkillResource("deploy-app", "scripts/deploy.sh")
				require.NoError(t, err)
			}
			mainID := tl.idToTimelineItem.Keys()[0]
			bindCompressionMock(t, tl, func(_ *AIRequest) (string, error) {
				body, err := json.Marshal(map[string]any{
					"@action": "timeline-summary", "summary": "Skill windows summarized.",
					"ratain_timeline_item_range": fmt.Sprint(mainID), "memory_entities": []any{},
				})
				return string(body), err
			})
			_, err = tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.True(t, tl.HasVisibleSkillContent("deploy-app"), "the metadata/tree remains visible")
			beforeIDs := tl.idToTimelineItem.Keys()
			beforeSkills := tl.RenderSkillContentForPrompt()

			if resource {
				_, err = mgr.LoadSkillResource("deploy-app", "scripts/deploy.sh")
				require.NoError(t, err)
				require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "echo deploy")
			} else {
				added, err := mgr.LoadAutoSkill("deploy-app")
				require.NoError(t, err)
				require.True(t, added, "a remaining tree must not block reloading a retired window")
				require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "Step 1: build.")
			}
			require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs)+1)
			require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
			if resource {
				_, err = mgr.LoadSkillResource("deploy-app", "scripts/deploy.sh")
				require.NoError(t, err)
			} else {
				added, err := mgr.LoadAutoSkill("deploy-app")
				require.NoError(t, err)
				require.False(t, added)
			}
			require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs)+1, "same visible content must not be appended twice")
		})
	}
}
