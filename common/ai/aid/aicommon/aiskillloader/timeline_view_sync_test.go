package aiskillloader

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func newTimelineViewSyncLoader(t *testing.T) SkillLoader {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	body := strings.Repeat("Shared guide line with enough detail to require paging.\n", 1000)
	vfs.AddFile("shared/SKILL.md", buildTestSkillMD("shared", "Shared guide", body))
	for _, path := range []string{"first.md", "second.md", "local.md"} {
		vfs.AddFile("shared/"+path, body)
	}
	vfs.AddFile("shared/run.sh", "#!/bin/sh\necho shared\n")
	loader, err := NewFSSkillLoader(vfs)
	require.NoError(t, err)
	return loader
}

func TestTimelineViewSyncMergesWithoutReplacingLocalWindows(t *testing.T) {
	loader := newTimelineViewSyncLoader(t)
	sink := newTestTimelineSink()
	writer := NewSkillsContextManager(loader)
	writer.BindTimelineSink(sink)
	require.NoError(t, writer.LoadSkill("shared"))
	require.NoError(t, writer.ChangeViewOffset("shared", "first.md", 300))

	readerLoader := &bindingTestLoader{SkillLoader: loader}
	reader := NewSkillsContextManager(readerLoader)
	reader.BindTimelineSink(sink)
	before := sink.appendCount()
	require.True(t, reader.IsSkillLoaded("shared"))
	require.True(t, reader.HasTruncatedViews())
	require.Len(t, reader.GetCurrentSelectedSkills(), 1)
	require.Contains(t, reader.GetSkillViewSummary("shared"), "first.md:300-")
	require.Zero(t, readerLoader.loads, "state queries must not read skill files")
	require.Equal(t, before, sink.appendCount())

	// 翻页直接恢复其它 manager 加载的技能；只追加请求的那一页。
	require.NoError(t, reader.ChangeViewOffset("shared", "", 100))
	require.Equal(t, before+1, sink.appendCount())
	state, ok := reader.stateFor("shared")
	require.True(t, ok)
	firstWindow := state.ViewWindows["first.md"]
	require.NotNil(t, firstWindow)
	require.Equal(t, 300, firstWindow.GetOffset())
	mainWindow := state.ViewWindows[skillMDFilename]
	require.Equal(t, 100, mainWindow.GetOffset())
	_, err := reader.LoadSkillResource("shared", "local.md")
	require.NoError(t, err)
	localWindow := state.ViewWindows["local.md"]
	sink.mu.Lock()
	for _, item := range sink.items {
		if item.path == "local.md" {
			item.deleted = true
		}
	}
	sink.mu.Unlock()

	// 另一侧新增窗口、移动已有文件游标后，同步只补缺失文件。
	require.NoError(t, writer.ChangeViewOffset("shared", "first.md", 500))
	require.NoError(t, writer.ChangeViewOffset("shared", "second.md", 400))
	beforeParts := sink.VisibleSkillParts("")
	before = sink.appendCount()
	_, err = reader.LoadSkillResource("shared", "first.md")
	require.NoError(t, err)
	require.Same(t, firstWindow, state.ViewWindows["first.md"])
	require.Equal(t, 300, firstWindow.GetOffset(), "the reader's existing cursor must survive synchronization")
	require.Same(t, mainWindow, state.ViewWindows[skillMDFilename])
	require.Equal(t, 100, mainWindow.GetOffset())
	require.Same(t, localWindow, state.ViewWindows["local.md"], "a local-only window must survive a union merge")
	require.Equal(t, 400, state.ViewWindows["second.md"].GetOffset())
	require.Equal(t, beforeParts, sink.VisibleSkillParts(""))
	require.Equal(t, before, sink.appendCount(), "merging windows must not replay their bodies")

	// 反向同步也成立，且双方保留各自的窗口对象。
	_, err = writer.LoadSkillResource("shared", "local.md")
	require.NoError(t, err)
	writerState, ok := writer.stateFor("shared")
	require.True(t, ok)
	require.Contains(t, writerState.ViewWindows, "local.md")
	require.NotSame(t, localWindow, writerState.ViewWindows["local.md"])
	require.Equal(t, 500, writerState.ViewWindows["first.md"].GetOffset())
	require.Equal(t, before+1, sink.appendCount(), "explicitly rereading the retired local.md appends only that resource")
}

func TestTimelineViewSyncConcurrentManagers(t *testing.T) {
	loader := newTimelineViewSyncLoader(t)
	sink := newTestTimelineSink()
	writer := NewSkillsContextManager(loader)
	writer.BindTimelineSink(sink)
	require.NoError(t, writer.LoadSkill("shared"))
	reader := NewSkillsContextManager(loader)
	reader.BindTimelineSink(sink)

	managers := []*SkillsContextManager{writer, reader}
	paths := []string{"first.md", "second.md"}
	errs := make(chan error, 6)
	var wg sync.WaitGroup
	for i, mgr := range managers {
		wg.Add(1)
		go func(mgr *SkillsContextManager, path string) {
			defer wg.Done()
			for _, offset := range []int{100, 200, 300} {
				errs <- mgr.ChangeViewOffset("shared", path, offset)
				mgr.HasTruncatedViews()
				mgr.GetCurrentSelectedSkills()
			}
		}(mgr, paths[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for i, mgr := range managers {
		_, err := mgr.LoadSkillResource("shared", paths[i])
		require.NoError(t, err)
		state, ok := mgr.stateFor("shared")
		require.True(t, ok)
		for _, path := range paths {
			require.Contains(t, state.ViewWindows, path)
		}
		require.Equal(t, 300, state.ViewWindows[paths[i]].GetOffset())
		require.Len(t, sink.VisibleSkillParts("shared"), 8, "metadata, main document and all six resource ranges survive")
	}
}

func TestTimelineViewSyncLoadEntryPoints(t *testing.T) {
	tests := []struct {
		name string
		load func(*SkillsContextManager) error
	}{
		{"manual", func(m *SkillsContextManager) error { return m.LoadSkill("shared") }},
		{"forced", func(m *SkillsContextManager) error {
			_, err := m.LoadForcedSkill("shared")
			return err
		}},
		{"auto", func(m *SkillsContextManager) error {
			_, err := m.LoadAutoSkill("shared")
			return err
		}},
		{"resource", func(m *SkillsContextManager) error {
			_, err := m.LoadSkillResource("shared", "first.md")
			return err
		}},
		{"script", func(m *SkillsContextManager) error {
			_, err := m.LoadSkillResourceAsScript("shared", "run.sh", func(string, string, any) string {
				return "/artifacts/run.sh"
			})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := newTimelineViewSyncLoader(t)
			sink := newTestTimelineSink()
			writer := NewSkillsContextManager(loader)
			writer.BindTimelineSink(sink)
			require.NoError(t, writer.LoadSkill("shared"))
			require.NoError(t, writer.ChangeViewOffset("shared", "first.md", 300))
			require.NoError(t, writer.ChangeViewOffset("shared", "first.md", 500))
			before := sink.appendCount()
			reader := NewSkillsContextManager(loader)
			reader.BindTimelineSink(sink)
			require.NoError(t, test.load(reader))
			state, ok := reader.stateFor("shared")
			require.True(t, ok)
			require.Equal(t, 500, state.ViewWindows["first.md"].GetOffset(),
				"a new window starts at the furthest visible range")
			if test.name == "script" {
				require.Equal(t, before+1, sink.appendCount(), "only the script reference is new")
			} else {
				require.Equal(t, before, sink.appendCount(), "already visible pages must not be duplicated")
			}
		})
	}
}

func TestTimelineViewSyncDoesNotReplayRetiredContent(t *testing.T) {
	loader := newTimelineViewSyncLoader(t)
	sink := newTestTimelineSink()
	writer := NewSkillsContextManager(loader)
	writer.BindTimelineSink(sink)
	require.NoError(t, writer.LoadSkill("shared"))
	require.NoError(t, writer.ChangeViewOffset("shared", "first.md", 300))
	// 模拟压缩后只留下资源窗口，主文档及元信息已退休。
	sink.mu.Lock()
	for _, item := range sink.items {
		if item.path != "first.md" {
			item.deleted = true
		}
	}
	sink.mu.Unlock()
	before := sink.appendCount()
	parts := sink.VisibleSkillParts("")
	require.Len(t, parts, 1)
	reader := NewSkillsContextManager(loader)
	reader.BindTimelineSink(sink)
	_, err := reader.LoadSkillResource("shared", "first.md")
	require.NoError(t, err)
	require.Equal(t, before, sink.appendCount())
	require.Equal(t, parts, sink.VisibleSkillParts(""), "restoring resource state must not restore retired SKILL.md")

	require.True(t, writer.UnloadSkill("shared"))
	require.False(t, reader.IsSkillLoaded("shared"))
	require.False(t, reader.HasTruncatedViews(), "cached windows must not make an unloaded skill visible")
	require.Empty(t, reader.GetCurrentSelectedSkills())
	require.Empty(t, reader.GetSkillViewSummary("shared"))
	require.Equal(t, before, sink.appendCount(), "read-only queries must not resurrect cached content")

	_, err = reader.LoadSkillResource("shared", "first.md")
	require.NoError(t, err)
	require.Len(t, sink.VisibleSkillParts("shared"), 1, "an explicit reread restores only the requested resource")
}

func TestTimelineViewSyncMissingSourcesDoNotBreakQueries(t *testing.T) {
	sink := newTestTimelineSink()
	require.True(t, sink.AppendSkillFileContent("unavailable", "guide.md", "retained body", SkillContentSourceManual, 10, 20, 100))
	mgr := NewSkillsContextManager(newTimelineViewSyncLoader(t))
	mgr.BindTimelineSink(sink)
	before := sink.appendCount()
	require.True(t, mgr.IsSkillLoaded("unavailable"))
	require.True(t, mgr.HasTruncatedViews())
	require.Contains(t, mgr.GetSkillViewSummary("unavailable"), "guide.md:10-20/100")
	require.Len(t, mgr.GetCurrentSelectedSkills(), 1)
	require.Error(t, mgr.ChangeViewOffset("unavailable", "guide.md", 21))
	require.Equal(t, before, sink.appendCount())

	// 非文件窗口和已失效资源不阻止可用文件继续翻页。
	require.NoError(t, mgr.LoadSkill("shared"))
	require.True(t, sink.AppendSkillFileContent("shared", "missing.md", "old resource", SkillContentSourceManual, 1, 1, 1))
	require.True(t, sink.AppendSkillFileContent("shared", "script_ref:run.sh", "script reference", SkillContentSourceManual, 1, 1, 1))
	require.True(t, sink.AppendSkillFileContent("shared", "grep:shared", "search result", SkillContentSourceManual, 1, 1, 1))
	before = sink.appendCount()
	require.NoError(t, mgr.ChangeViewOffset("shared", "", 100))
	require.Equal(t, before+1, sink.appendCount())
	require.Contains(t, mgr.GetSkillViewSummary("shared"), "missing.md")
}
