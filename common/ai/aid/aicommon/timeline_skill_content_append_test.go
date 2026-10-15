package aicommon

import (
	"context"
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
			require.True(t, tl.PushSkillFileContent(id, "demo-skill", "SKILL.md", "SAME_BODY", SkillContentSourceForced, 1, 1, 1))
			if frozen {
				tl.FreezeAll()
			}
			before := RenderTimelineFrozenOpen(tl)
			beforeSkills := tl.RenderSkillContentForPrompt()
			beforeFreeze := tl.FreezeSnapshot()

			require.False(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SAME_BODY", SkillContentSourceAuto, 1, 1, 1), "same content must not be appended even from another source")
			require.Equal(t, []int64{id}, tl.idToTimelineItem.Keys())
			require.Equal(t, before, RenderTimelineFrozenOpen(tl))
			require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
			require.Equal(t, beforeFreeze, tl.FreezeSnapshot())

			require.True(t, tl.DeleteSkillContent("demo-skill"))
			require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SAME_BODY", SkillContentSourceManual, 1, 1, 1), "retired content must be readable again")
			require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "SAME_BODY")
		})
	}
}

func TestSkillContentDifferentWindowsStayInHistory(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	firstID := cfg.AcquireId()
	require.True(t, tl.PushSkillFileContent(firstID, "demo-skill", "SKILL.md", "FIRST_WINDOW", SkillContentSourceAuto, 1, 1, 1))
	tl.FreezeAll()
	before := RenderTimelineFrozenOpen(tl)
	beforeSkills := tl.RenderSkillContentForPrompt()

	secondID := cfg.AcquireId()
	require.True(t, tl.PushSkillFileContent(secondID, "demo-skill", "SKILL.md", "SECOND_WINDOW", SkillContentSourceAuto, 1, 1, 1))
	require.Equal(t, before.Frozen, RenderTimelineFrozenOpen(tl).Frozen)
	require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "SECOND_WINDOW")

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	require.Empty(t, snapshot.Items, "skill windows are promoted items, not ordinary history")
	require.Equal(t, []int64{firstID, secondID}, []int64{snapshot.PromotedItems[0].ID, snapshot.PromotedItems[1].ID})

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

// 精确提升态的技能窗口不进入压缩候选, 因此两个历史窗口都必须保留;
// 需要重读旧页时仍可通过显式写入追加新 item。
func TestSkillContentHistoricalWindowsSurviveCompression(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	firstID, secondID := cfg.AcquireId(), cfg.AcquireId()
	require.True(t, tl.PushSkillFileContent(firstID, "demo-skill", "SKILL.md", "EARLIER_WINDOW", SkillContentSourceAuto, 1, 1, 1))
	require.True(t, tl.PushSkillFileContent(secondID, "demo-skill", "SKILL.md", "LATER_WINDOW", SkillContentSourceAuto, 1, 1, 1))
	tl.FreezeAll()
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		// Same identity does not aggregate compression candidates: both
		// original reads and their full bodies are independently selectable.
		require.Equal(t, 2, strings.Count(req.GetPrompt(), "demo-skill:SKILL.md"))
		require.Contains(t, req.GetPrompt(), "EARLIER_WINDOW")
		require.Contains(t, req.GetPrompt(), "LATER_WINDOW")
		require.Contains(t, req.GetPrompt(), "PROMOTED_ENTRIES_")
		return compressionMockSummary("Ordinary history summarized."), nil
	})

	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.NotContains(t, result.RetiredIDs, firstID)
	require.NotContains(t, result.RetiredIDs, secondID)
	skills := tl.RenderSkillContentForPrompt()
	require.Contains(t, skills, "EARLIER_WINDOW")
	require.Contains(t, skills, "LATER_WINDOW")
	require.Less(t, strings.Index(skills, "EARLIER_WINDOW"), strings.Index(skills, "LATER_WINDOW"))
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
			results <- sink.AppendSkillFileContent("demo-skill", "SKILL.md", "CONCURRENT_BODY", SkillContentSourceManual, 1, 1, 1)
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
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "[skill demo-skill | SKILL.md:2-")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, " END]")
	tl.FreezeAll()
	skills := tl.RenderSkillContentForPrompt()
	require.Contains(t, skills, "FIRST_LINE")
	require.Contains(t, skills, "[SKILL.md:2-")
	require.Equal(t, 1, strings.Count(skills, "[SKILL.md:1-"))
	require.Equal(t, 1, strings.Count(skills, "[SKILL.md:2-"))

	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "", 2))
	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "", 1))
	require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs)+1, "rereading either retained window must be idempotent")
	require.Equal(t, skills, tl.RenderSkillContentForPrompt())
}

func TestSkillManagerContentSurvivesCompression(t *testing.T) {
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
			bindCompressionMock(t, tl, func(_ *AIRequest) (string, error) {
				return compressionMockSummary("Skill windows kept exact."), nil
			})
			_, err = tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.True(t, skillVisible(tl, "deploy-app"), "the metadata/tree remains visible")
			beforeIDs := tl.idToTimelineItem.Keys()
			beforeSkills := tl.RenderSkillContentForPrompt()

			if resource {
				_, err = mgr.LoadSkillResource("deploy-app", "scripts/deploy.sh")
				require.NoError(t, err)
				require.Contains(t, tl.RenderSkillContentForPrompt(), "echo deploy")
			} else {
				added, err := mgr.LoadAutoSkill("deploy-app")
				require.NoError(t, err)
				require.False(t, added, "an exact skill must not be appended again")
				require.Contains(t, tl.RenderSkillContentForPrompt(), "Step 1: build.")
			}
			if resource {
				require.Len(t, tl.idToTimelineItem.Keys(), len(beforeIDs))
				require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt(), "an already visible resource must not be appended twice")
			} else {
				require.Equal(t, beforeSkills, tl.RenderSkillContentForPrompt())
			}
		})
	}
}
