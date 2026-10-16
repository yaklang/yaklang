package aicommon

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestSkillPartsAggregateByPathAndRange(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("demo-skill/SKILL.md", buildSkillMD("demo-skill", "Demo", "FIRST_LINE\nSECOND_LINE\nTHIRD_LINE"))
	vfs.AddFile("demo-skill/notes.md", "SAME_TEXT\nTAIL")
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	bindTestTimeline(mgr, cfg)

	added, err := mgr.LoadForcedSkill("demo-skill")
	require.NoError(t, err)
	require.True(t, added)
	open := RenderTimelineFrozenOpen(tl).Open
	require.Contains(t, open, "[skill demo-skill | metadata]")
	require.Contains(t, open, "[source: user-forced]")
	require.Contains(t, open, "File Tree:")
	require.Contains(t, open, "[skill demo-skill | SKILL.md:")
	require.Contains(t, open, " END]")
	require.Equal(t, 1, strings.Count(open, "[source: user-forced]"))
	require.NotContains(t, open, "=== Skill:")

	tl.FreezeAll()
	promoted := tl.RenderSkillContentForPrompt()
	require.Equal(t, 1, strings.Count(promoted, "=== Skill: demo-skill ==="))
	require.Less(t, strings.Index(promoted, "[metadata]"), strings.Index(promoted, "[SKILL.md:"))
	require.Contains(t, promoted, " END]")

	before := len(tl.idToTimelineItem.Keys())
	_, err = mgr.LoadSkillResource("demo-skill", "notes.md")
	require.NoError(t, err)
	require.Len(t, tl.idToTimelineItem.Keys(), before+1)
	require.NoError(t, mgr.ChangeViewOffset("demo-skill", "notes.md", 2))
	require.Len(t, tl.idToTimelineItem.Keys(), before+2)

	// 相同文本的另一个范围必须保留，不能按正文去重。
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "notes.md", "SAME_TEXT", SkillContentSourceManual, 1, 1, 2))
	require.False(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "notes.md", "SAME_TEXT", SkillContentSourceAuto, 1, 1, 2))

	tl.FreezeAll()
	parts := tl.VisibleSkillParts("demo-skill")
	require.Equal(t, "demo-skill", parts[0].Name)
	var ranges []string
	metadata := 0
	for _, part := range parts {
		require.Equal(t, "demo-skill", part.Name)
		if part.Kind == skillContentKindMetadata {
			metadata++
			continue
		}
		label := part.Path
		if part.EndLine > 0 {
			label += ":" + strconv.Itoa(part.StartLine) + "-" + strconv.Itoa(part.EndLine) + "/" + strconv.Itoa(part.Total)
		}
		if part.ReachedEnd {
			label += " END"
		}
		ranges = append(ranges, label)
	}
	require.Equal(t, 1, metadata)
	require.Contains(t, ranges, "notes.md:1-1/2")
	require.Contains(t, ranges, "notes.md:2-2/2 END")
	names := map[string]bool{}
	for _, part := range tl.VisibleSkillParts("") {
		names[part.Name] = true
	}
	require.Equal(t, map[string]bool{"demo-skill": true}, names)

	summary := mgr.GetSkillViewSummary("demo-skill")
	require.Equal(t, 1, strings.Count(summary, "notes.md:1-1/2"))
	require.Contains(t, summary, "notes.md:2-2/2 END")
	require.Contains(t, summary, "Metadata: loaded")
}

func TestVisibleSkillPartsTokenCountsTrackRetainedContent(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	metadata, first, updated, last := "技能说明与文件树", "first page body", "updated first page", "last page body"
	require.True(t, tl.PushSkillMetadata(1, "demo-skill", metadata, SkillContentSourceForced))
	require.True(t, tl.PushSkillFileContent(2, "demo-skill", "SKILL.md", first, SkillContentSourceManual, 1, 4, 8))
	require.True(t, tl.PushSkillCatalog(3, strings.Repeat("recommended skills ", 100)))
	mgr := aiskillloader.NewSkillsContextManager(nil)
	mgr.BindTimelineSink(NewSkillContentTimelineAdapter(tl, func() int64 { return tl.GetMaxID() + 1 }))

	parts := tl.VisibleSkillParts("demo-skill")
	require.Len(t, parts, 2)
	require.Equal(t, MeasureTokens(metadata), parts[0].TokenCount)
	require.Equal(t, MeasureTokens(first), parts[1].TokenCount)
	selected := mgr.GetCurrentSelectedSkills()
	require.Len(t, selected, 1)
	require.Equal(t, "demo-skill", selected[0].Name, "Timeline-only skills need no loader")
	require.Equal(t, MeasureTokens(metadata)+MeasureTokens(first), selected[0].TokenCount)
	require.Empty(t, tl.VisibleSkillParts("missing-skill"))
	require.False(t, tl.PushSkillFileContent(4, "demo-skill", "SKILL.md", first, SkillContentSourceAuto, 1, 4, 8))
	require.Equal(t, parts, tl.VisibleSkillParts("demo-skill"), "a rejected duplicate does not increase size")

	freeze := tl.FreezeAll()
	require.Equal(t, parts, tl.VisibleSkillParts("demo-skill"), "moving to promotedState preserves body size")
	require.True(t, tl.PushSkillFileContent(5, "demo-skill", "SKILL.md", updated, SkillContentSourceManual, 1, 4, 8))
	require.True(t, tl.PushSkillFileContent(6, "demo-skill", "SKILL.md", last, SkillContentSourceManual, 5, 8, 8))
	parts = tl.VisibleSkillParts("demo-skill")
	require.Len(t, parts, 3)
	require.Equal(t, MeasureTokens(first)+MeasureTokens(updated), parts[1].TokenCount, "different bodies of one range both remain visible")
	require.Equal(t, MeasureTokens(last), parts[2].TokenCount)
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	expected := MeasureTokens(metadata) + MeasureTokens(first) + MeasureTokens(updated) + MeasureTokens(last)
	require.Equal(t, expected, mgr.GetCurrentSelectedSkills()[0].TokenCount, "count the mixed Open and frozen view")
	require.Contains(t, mgr.GetSkillViewSummary("demo-skill"), "Timeline content: "+strconv.Itoa(expected)+" tokens")
	require.Equal(t, parts, tl.VisibleSkillParts(""), "catalog entries are excluded from all-skills queries")
	parts[0].TokenCount = -1
	require.Equal(t, MeasureTokens(metadata), tl.VisibleSkillParts("demo-skill")[0].TokenCount, "query results are detached")
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "size observation must not mutate journal or projections")
	require.Equal(t, freeze.Version, tl.FreezeSnapshot().Version)

	tl.FreezeAll()
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, tl.VisibleSkillParts("demo-skill"), restored.VisibleSkillParts("demo-skill"))
	mgr.BindTimelineSink(NewSkillContentTimelineAdapter(restored, func() int64 { return restored.GetMaxID() + 1 }))
	require.Equal(t, expected, mgr.GetCurrentSelectedSkills()[0].TokenCount, "rebind to restored Timeline derives the same sizes")
	require.True(t, restored.DeleteSkillContent("demo-skill"))
	require.Empty(t, mgr.GetCurrentSelectedSkills())
	require.Empty(t, mgr.GetSkillViewSummary("demo-skill"))
	mgr.BindTimelineSink(NewSkillContentTimelineAdapter(tl, func() int64 { return tl.GetMaxID() + 1 }))
	require.Equal(t, expected, mgr.GetCurrentSelectedSkills()[0].TokenCount, "removal in the restored Timeline does not affect the original")
}

func TestVisibleSkillPartsTokenCountsFollowFrozenHashDeduplication(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	stamp := time.Now()
	for i, body := range []string{"FIRST_BODY", "SECOND_BODY", "SECOND_BODY"} {
		payload := skillContentPayload{Name: "demo-skill", Kind: skillContentKindContent,
			Path: "SKILL.md", Content: body, Range: &skillContentRange{StartLine: 1, EndLine: 4, Total: 8}}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		id := int64(i + 1)
		importFreezeItem(tl, id, stamp.Add(time.Duration(i)*time.Millisecond), &PromotableTimelineItem{
			ID: id, Kind: TimelinePromotedKindSkillContent, TargetSection: TimelinePromotedTargetSemiDynamic1,
			Key: skillPartIdentity(payload), Operation: TimelinePromotedOperationUpsert, Payload: string(raw),
		})
	}
	parts := tl.VisibleSkillParts("demo-skill")
	require.Len(t, parts, 1)
	require.Equal(t, MeasureTokens("FIRST_BODY")+2*MeasureTokens("SECOND_BODY"), parts[0].TokenCount,
		"imported duplicate items remain visible separately in Open")
	tl.FreezeAll()
	parts = tl.VisibleSkillParts("demo-skill")
	require.Len(t, parts, 1)
	require.Equal(t, MeasureTokens("FIRST_BODY")+MeasureTokens("SECOND_BODY"), parts[0].TokenCount,
		"promoted hash deduplication also applies to the visible body size")
	require.Equal(t, 1, strings.Count(tl.RenderSkillContentForPrompt(), "SECOND_BODY"))
	require.Len(t, tl.idToTimelineItem.Keys(), 3, "size observation does not clean up the journal")
}

// skillVisible 是测试侧的可见性判断: 由 Timeline 中未退休的技能部分派生。
func skillVisible(tl *Timeline, name string) bool {
	return tl != nil && len(tl.VisibleSkillParts(name)) > 0
}
