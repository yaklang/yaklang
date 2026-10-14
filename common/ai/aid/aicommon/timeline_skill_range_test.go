package aicommon

import (
	"context"
	"strconv"
	"strings"
	"testing"

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

// skillVisible 是测试侧的可见性判断: 由 Timeline 中未退休的技能部分派生。
func skillVisible(tl *Timeline, name string) bool {
	return tl != nil && len(tl.VisibleSkillParts(name)) > 0
}
