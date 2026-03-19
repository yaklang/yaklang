package loopinfra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type skillResourceTestRuntime struct {
	*mock.MockInvoker
	tmpDir            string
	timelineMu        sync.Mutex
	timelineLogs      []timelineCall
	emittedArtifacts  []emittedArtifact
	emittedArtifactMu sync.Mutex
}

type emittedArtifact struct {
	Name string
	Ext  string
	Data []byte
	Path string
}

func newSkillResourceTestRuntime(t *testing.T) *skillResourceTestRuntime {
	t.Helper()
	return &skillResourceTestRuntime{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		tmpDir:      t.TempDir(),
	}
}

func (r *skillResourceTestRuntime) GetYakExecutablePath() string {
	return "/usr/local/bin/yak"
}

func (r *skillResourceTestRuntime) EmitFileArtifactWithExt(name, ext string, data any) string {
	r.emittedArtifactMu.Lock()
	defer r.emittedArtifactMu.Unlock()

	var dataBytes []byte
	switch v := data.(type) {
	case []byte:
		dataBytes = v
	case string:
		dataBytes = []byte(v)
	}

	outPath := filepath.Join(r.tmpDir, name+ext)
	_ = os.WriteFile(outPath, dataBytes, 0644)

	r.emittedArtifacts = append(r.emittedArtifacts, emittedArtifact{
		Name: name, Ext: ext, Data: dataBytes, Path: outPath,
	})
	return outPath
}

func (r *skillResourceTestRuntime) AddToTimeline(entry, content string) {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	r.timelineLogs = append(r.timelineLogs, timelineCall{Entry: entry, Content: content})
}

func (r *skillResourceTestRuntime) hasTimelineEntry(entry string) bool {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	for _, item := range r.timelineLogs {
		if item.Entry == entry {
			return true
		}
	}
	return false
}

func (r *skillResourceTestRuntime) timelineEntryContains(entry, needle string) bool {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	for _, item := range r.timelineLogs {
		if item.Entry == entry && strings.Contains(item.Content, needle) {
			return true
		}
	}
	return false
}

func buildTestSkillMD(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body
}

func registerLoadSkillResourcesAction() reactloops.ReActLoopOption {
	a := loopAction_LoadSkillResources
	return reactloops.WithRegisterLoopActionWithStreamField(
		a.ActionType, a.Description, a.Options, a.StreamFields,
		a.ActionVerifier, a.ActionHandler,
	)
}

func newSkillResourceLoop(t *testing.T, runtime *skillResourceTestRuntime, loader aiskillloader.SkillLoader) (*reactloops.ReActLoop, aicommon.AIStatefulTask) {
	t.Helper()
	mgr := aiskillloader.NewSkillsContextManager(loader)
	loop, err := reactloops.NewReActLoop(
		"skill-resource-test",
		runtime,
		registerLoadSkillResourcesAction(),
		reactloops.WithSkillsContextManager(mgr),
	)
	require.NoError(t, err)

	emitter := aicommon.NewEmitter("skill-resource-test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	task := aicommon.NewStatefulTaskBase("skill-resource-task", "skill resource test", context.Background(), emitter, true)
	loop.SetCurrentTask(task)
	return loop, task
}

func newSkillResourceLoopNoManager(t *testing.T, runtime *skillResourceTestRuntime) (*reactloops.ReActLoop, aicommon.AIStatefulTask) {
	t.Helper()
	loop, err := reactloops.NewReActLoop(
		"no-mgr-test",
		runtime,
		registerLoadSkillResourcesAction(),
	)
	require.NoError(t, err)

	emitter := aicommon.NewEmitter("test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	task := aicommon.NewStatefulTaskBase("task", "test", context.Background(), emitter, true)
	loop.SetCurrentTask(task)
	return loop, task
}

// --- Verifier tests ---

func TestLoadSkillResources_Verifier_MissingResourcePath(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.NoError(t, ac.ActionVerifier(loop, action))
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

func TestLoadSkillResources_Verifier_InvalidResourceType(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	vfs.AddFile("s1/file.md", "content")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1/file.md",
		"resource_type": "invalid_type",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.NoError(t, ac.ActionVerifier(loop, action))
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

func TestLoadSkillResources_Verifier_DefaultsToDocument(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	vfs.AddFile("s1/file.md", "content")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1/file.md",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.NoError(t, ac.ActionVerifier(loop, action))
	assert.Equal(t, "document", loop.Get("_load_resource_type"))
}

func TestLoadSkillResources_Verifier_ScriptTypeAccepted(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	vfs.AddFile("s1/run.sh", "#!/bin/bash")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1/run.sh",
		"resource_type": "script",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.NoError(t, ac.ActionVerifier(loop, action))
	assert.Equal(t, "script", loop.Get("_load_resource_type"))
}

func TestLoadSkillResources_Verifier_MissingFilePath(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.NoError(t, ac.ActionVerifier(loop, action))
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

// --- Document handler tests ---

func TestLoadSkillResources_Document_Success(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("docskill/SKILL.md", buildTestSkillMD("docskill", "Doc skill", "body"))
	vfs.AddFile("docskill/guide.md", "# Guide\nStep 1\nStep 2\nStep 3")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@docskill/guide.md",
		"resource_type": "document",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_resource_loaded"))
	assert.True(t, runtime.timelineEntryContains("skill_resource_loaded", "guide.md"))
	assert.Contains(t, op.GetFeedback().String(), "loaded successfully")
}

func TestLoadSkillResources_Document_NotFound(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("docskill/SKILL.md", buildTestSkillMD("docskill", "Doc skill", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@docskill/nonexistent.md",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	_ = ac.ActionVerifier(loop, action)

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_resource_load_failed"))
	assert.Contains(t, op.GetFeedback().String(), "Failed to load")
}

// --- Script handler tests ---

func TestLoadSkillResources_Script_VirtualFS_Materializes(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("scriptskill/SKILL.md", buildTestSkillMD("scriptskill", "Script skill", "body"))
	vfs.AddFile("scriptskill/tools/scan.py", "#!/usr/bin/env python3\nprint('scan')")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@scriptskill/tools/scan.py",
		"resource_type": "script",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_script_resource_loaded"))
	assert.True(t, runtime.timelineEntryContains("skill_script_resource_loaded", "absolute path"))
	assert.True(t, runtime.timelineEntryContains("skill_script_resource_loaded", "materialized"))

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "loaded successfully")
	assert.Contains(t, feedback, "Absolute path:")
	assert.Contains(t, feedback, "materialized")

	runtime.emittedArtifactMu.Lock()
	assert.Len(t, runtime.emittedArtifacts, 1)
	assert.Equal(t, ".py", runtime.emittedArtifacts[0].Ext)
	assert.Contains(t, string(runtime.emittedArtifacts[0].Data), "scan")
	runtime.emittedArtifactMu.Unlock()
}

func TestLoadSkillResources_Script_LocalFS_ResolvesPath(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "localskill")
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755))

	skillMD := buildTestSkillMD("localskill", "Local skill", "body")
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))
	scriptContent := "#!/bin/bash\necho deploy"
	scriptFile := filepath.Join(skillDir, "scripts", "deploy.sh")
	require.NoError(t, os.WriteFile(scriptFile, []byte(scriptContent), 0755))

	loader, err := aiskillloader.NewLocalSkillLoader(tmpDir)
	require.NoError(t, err)

	runtime := newSkillResourceTestRuntime(t)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@localskill/scripts/deploy.sh",
		"resource_type": "script",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_script_resource_loaded"))

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, scriptFile)
	assert.NotContains(t, feedback, "materialized")

	runtime.emittedArtifactMu.Lock()
	assert.Len(t, runtime.emittedArtifacts, 0, "should not materialize for local FS")
	runtime.emittedArtifactMu.Unlock()
}

func TestLoadSkillResources_Script_NotFound(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("scriptskill/SKILL.md", buildTestSkillMD("scriptskill", "Script skill", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@scriptskill/nonexistent.sh",
		"resource_type": "script",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	_ = ac.ActionVerifier(loop, action)

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_script_resource_load_failed"))
	assert.Contains(t, op.GetFeedback().String(), "Failed to load")
}

func TestLoadSkillResources_Script_FuzzyMatch(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("fuzzskill/SKILL.md", buildTestSkillMD("fuzzskill", "Fuzzy", "body"))
	vfs.AddFile("fuzzskill/deep/nested/run.yak", "println(\"hello\")")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@fuzzskill/run.yak",
		"resource_type": "script",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_script_resource_loaded"))
	assert.Contains(t, op.GetFeedback().String(), "fuzzy matched")
	assert.True(t, runtime.timelineEntryContains("skill_script_resource_loaded", "Recommended command: /usr/local/bin/yak"))
	assert.True(t, runtime.timelineEntryContains("use_script", "/usr/local/bin/yak"))
}

func TestLoadSkillResources_Script_SkillNotFound(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@nonexistent/run.sh",
		"resource_type": "script",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	_ = ac.ActionVerifier(loop, action)

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_script_resource_load_failed"))
}

// --- No skills context manager ---

func TestLoadSkillResources_Verifier_NoSkillsContextManager(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	loop, _ := newSkillResourceLoopNoManager(t, runtime)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@skill/file.md",
	})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	assert.Error(t, ac.ActionVerifier(loop, action))
}

func TestLoadSkillResources_Handler_NoSkillsContextManager(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	loop, task := newSkillResourceLoopNoManager(t, runtime)

	loop.Set("_load_resource_mode", "load")
	loop.Set("_load_resource_skill", "test")
	loop.Set("_load_resource_path", "file.md")
	loop.Set("_load_resource_raw", "@test/file.md")
	loop.Set("_load_resource_type", "document")

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@test/file.md",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	terminated, failErr := op.IsTerminated()
	assert.True(t, terminated)
	assert.Error(t, failErr)
}

// --- Grep verifier tests ---

func TestLoadSkillResources_Verifier_PatternAndResourcePathBothEmpty(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

func TestLoadSkillResources_Verifier_PatternAndResourcePathBothSet(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	vfs.AddFile("s1/file.md", "content")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1/file.md",
		"pattern":       "test",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

func TestLoadSkillResources_Verifier_PatternOnly(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern": "search_term",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "grep", loop.Get("_load_resource_mode"))
	assert.Equal(t, "search_term", loop.Get("_grep_pattern"))
}

func TestLoadSkillResources_Verifier_PatternInvalidRegex(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern": "[invalid(regex",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "validation_failed", loop.Get("_load_resource_skip"))
}

func TestLoadSkillResources_Handler_InvalidParamsSoftFails(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{})
	ac, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, err)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_resource_validation_failed"))
	assert.Contains(t, op.GetFeedback().String(), "requires either 'resource_path' or 'pattern'")
}

func TestLoadSkillResources_Verifier_PatternWithSkillName(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern":    "search",
		"skill_name": "s1",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "grep", loop.Get("_load_resource_mode"))
	assert.Equal(t, "s1", loop.Get("_grep_skill_name"))
}

// --- Grep handler tests ---

func TestLoadSkillResources_Grep_Success(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("grepskill/SKILL.md", buildTestSkillMD("grepskill", "Grep test", "body with FINDME"))
	vfs.AddFile("grepskill/doc.md", "# Doc\nLine 1\nFINDME in the middle\nLine 3")
	vfs.AddFile("grepskill/other.md", "# Other\nNo match here")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern": "FINDME",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_grep_completed"))
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "Grep completed")
	assert.Contains(t, feedback, "SKILLS_CONTEXT")
}

func TestLoadSkillResources_Grep_NoMatch(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("grepskill/SKILL.md", buildTestSkillMD("grepskill", "Grep test", "body"))
	vfs.AddFile("grepskill/doc.md", "# Doc\nNothing here")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern": "ZZZZNONEXISTENTZZZZ",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_grep_completed"))
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "No matches found")
}

func TestLoadSkillResources_Grep_SpecificSkill(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("skill-a/SKILL.md", buildTestSkillMD("skill-a", "A", "body"))
	vfs.AddFile("skill-a/doc.md", "# A\nTARGET in A")
	vfs.AddFile("skill-b/SKILL.md", buildTestSkillMD("skill-b", "B", "body"))
	vfs.AddFile("skill-b/doc.md", "# B\nTARGET in B")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, task := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"pattern":    "TARGET",
		"skill_name": "skill-a",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.hasTimelineEntry("skill_grep_completed"))
	assert.True(t, runtime.timelineEntryContains("skill_grep_completed", "skill 'skill-a'"))
}

// --- Backward compatibility: existing resource_path tests should still pass ---

func TestLoadSkillResources_Verifier_ResourcePathStillWorks(t *testing.T) {
	runtime := newSkillResourceTestRuntime(t)
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("s1/SKILL.md", buildTestSkillMD("s1", "desc", "body"))
	vfs.AddFile("s1/file.md", "content")
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	loop, _ := newSkillResourceLoop(t, runtime, loader)

	action := mustBuildAction(t, schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES, map[string]any{
		"resource_path": "@s1/file.md",
	})
	ac, _ := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES)
	err := ac.ActionVerifier(loop, action)
	assert.NoError(t, err)
	assert.Equal(t, "load", loop.Get("_load_resource_mode"))
	assert.Equal(t, "document", loop.Get("_load_resource_type"))
}
