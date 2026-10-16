package aiskillloader

import (
	"fmt"
	"strings"
	"testing"
)

// fakeSkillSink 是可控的 Timeline 替身: 记录追加次数并模拟显式移除。
type fakeSkillSink struct {
	appends  int
	visible  map[string]bool
	lastBody map[string]string
	lastSrc  map[string]string
	bodies   map[string]map[string]bool
	parts    []SkillContextPart
}

func newFakeSkillSink() *fakeSkillSink {
	return &fakeSkillSink{
		visible:  map[string]bool{},
		lastBody: map[string]string{},
		lastSrc:  map[string]string{},
		bodies:   map[string]map[string]bool{},
	}
}

func (f *fakeSkillSink) AppendSkillMetadata(name, content, source string) bool {
	return f.appendPart(name, SkillContentKindMetadata, content, source, 0, 0)
}

func (f *fakeSkillSink) AppendSkillFileContent(name, path, content, source string, startLine, endLine, total int) bool {
	return f.appendPart(name, path+"\x00"+fmtRange(startLine, endLine, total), content, source, startLine, endLine)
}

func fmtRange(start, end, total int) string {
	return fmt.Sprintf("%d:%d:%d", start, end, total)
}

func (f *fakeSkillSink) appendPart(name, path, content, source string, start, end int) bool {
	if !f.visible[name] {
		f.bodies[name] = map[string]bool{}
	}
	key := path + "\x00" + content
	if f.bodies[name][key] {
		return false
	}
	f.bodies[name][key] = true
	f.appends++
	f.visible[name] = true
	f.lastBody[name] = content
	f.lastSrc[name] = source
	f.parts = append(f.parts, SkillContextPart{Name: name, Path: path, StartLine: start, EndLine: end,
		TokenCount: MeasureStringTokens(content, nil)})
	return true
}

func (f *fakeSkillSink) VisibleSkillParts(name string) []SkillContextPart {
	var parts []SkillContextPart
	for _, part := range f.parts {
		if !f.visible[part.Name] {
			continue
		}
		if name == "" || part.Name == name {
			parts = append(parts, part)
		}
	}
	return parts
}

func (f *fakeSkillSink) DeleteSkillContent(name string) bool {
	if !f.visible[name] {
		return false
	}
	f.visible[name] = false
	return true
}

func (f *fakeSkillSink) DeleteSkillCatalog() bool { return true }

func (f *fakeSkillSink) HasSkillContentHistory(name string) bool {
	_, found := f.bodies[name]
	return found
}

func (f *fakeSkillSink) RenderSkillContentForPrompt() string { return "" }

// After its body is explicitly removed, loading the same skill again must append
// a fresh item instead of being short-circuited by the loader-side cache.
func TestSkillReloadAfterSummarized(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newFakeSkillSink()
	mgr.BindTimelineSink(sink)

	const skillName = "deploy-app"
	if err := mgr.LoadSkill(skillName); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	firstAppends := sink.appends
	if firstAppends == 0 {
		t.Fatal("loading a skill must append its body to Timeline")
	}

	// Second load while still visible: no duplicate append.
	if err := mgr.LoadSkill(skillName); err != nil {
		t.Fatalf("second LoadSkill failed: %v", err)
	}
	if sink.appends != firstAppends {
		t.Fatalf("visible skill must not be re-appended: %d -> %d", firstAppends, sink.appends)
	}

	// Simulate explicit removal retiring the body.
	sink.visible[skillName] = false

	if err := mgr.LoadSkill(skillName); err != nil {
		t.Fatalf("reload after explicit removal failed: %v", err)
	}
	if sink.appends <= firstAppends {
		t.Fatalf("removed skill must be re-readable, appends=%d", sink.appends)
	}
	if !strings.Contains(sink.lastBody[skillName], "deploy") {
		t.Fatalf("reloaded body must carry the skill content, got %q", sink.lastBody[skillName])
	}
}

// A removed skill must not be reported as "already loaded and visible".
func TestSummarizedSkillIsNotVisible(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newFakeSkillSink()
	mgr.BindTimelineSink(sink)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if mgr.IsSkillLoadedAndUnfolded("deploy-app") != true {
		t.Fatal("freshly loaded skill must be visible")
	}

	sink.visible["deploy-app"] = false
	if mgr.IsSkillLoadedAndUnfolded("deploy-app") != false {
		t.Fatal("removed skill must not short-circuit as visible")
	}
}

// 用户指定加载只改变来源标记, 不再有独立渲染块; 三条加载路径内容同构。
func TestForcedSkillOnlyMarksSource(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newFakeSkillSink()
	mgr.BindTimelineSink(sink)

	added, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadForcedSkill failed: %v", err)
	}
	if !added {
		t.Fatal("first LoadForcedSkill should report newly loaded")
	}
	if got := sink.lastSrc["deploy-app"]; got != SkillContentSourceForced {
		t.Fatalf("forced load must mark source=%q, got %q", SkillContentSourceForced, got)
	}
	if !mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("forced skill must be visible as loaded")
	}

	// 重复加载不再重复追加。
	appends := sink.appends
	added2, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("second LoadForcedSkill err: %v", err)
	}
	if added2 {
		t.Fatal("second LoadForcedSkill should report already visible")
	}
	if sink.appends != appends {
		t.Fatalf("already visible skill must not be re-appended: %d -> %d", appends, sink.appends)
	}

	// unload 退休 Timeline 中的正文。
	if !mgr.UnloadSkill("deploy-app") {
		t.Fatal("UnloadSkill should retire the timeline item")
	}
	if mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("unloaded skill must not be visible")
	}
}

// 自动加载与普通加载同构: 只是来源不同, 渲染块与可见性语义一致。
func TestAutoAndManualSkillShareTimelineSemantics(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newFakeSkillSink()
	mgr.BindTimelineSink(sink)

	if _, err := mgr.LoadAutoSkill("code-review"); err != nil {
		t.Fatalf("LoadAutoSkill failed: %v", err)
	}
	if got := sink.lastSrc["code-review"]; got != SkillContentSourceAuto {
		t.Fatalf("auto load must mark source=%q, got %q", SkillContentSourceAuto, got)
	}
	if !mgr.IsAutoSkillLoadedAndUnfolded("code-review") {
		t.Fatal("auto skill must be visible as loaded")
	}
}

// 清空/隐藏技能目录必须移除之前写入的目录 item, 且不影响技能正文。
func TestClearCatalogRemovesTimelineItem(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	mgr.SetCatalogSkills(loader.AllSkillMetas())
	if rendered := mgr.RenderStable(); !strings.Contains(rendered, "Relevant Skills") {
		t.Fatalf("catalog should be visible before clearing:\n%s", rendered)
	}

	mgr.SetCatalogSkills(nil)
	rendered := mgr.RenderStable()
	if strings.Contains(rendered, "Relevant Skills") {
		t.Fatalf("cleared catalog must not be rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatalf("clearing the catalog must not remove skill bodies:\n%s", rendered)
	}
}

func (f *fakeSkillSink) AppendSkillCatalog(rendered string) bool {
	f.appends++
	f.visible["__skill_catalog__"] = true
	return true
}
