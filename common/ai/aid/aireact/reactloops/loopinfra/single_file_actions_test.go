package loopinfra

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type timelineCall struct {
	Entry   string
	Content string
}

type testRuntimeForSingleFile struct {
	*mock.MockInvoker
	tmpDir       string
	forcedEmit   string
	timelineMu   sync.Mutex
	timelineLogs []timelineCall
}

func newTestRuntimeForSingleFile(t *testing.T) *testRuntimeForSingleFile {
	t.Helper()
	tmpDir := t.TempDir()
	return &testRuntimeForSingleFile{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		tmpDir:      tmpDir,
	}
}

func (r *testRuntimeForSingleFile) EmitFileArtifactWithExt(name, ext string, data any) string {
	if r.forcedEmit != "" {
		return r.forcedEmit
	}
	return filepath.Join(r.tmpDir, name+ext)
}

func (r *testRuntimeForSingleFile) AddToTimeline(entry, content string) {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	r.timelineLogs = append(r.timelineLogs, timelineCall{Entry: entry, Content: content})
}

func (r *testRuntimeForSingleFile) timelineContains(entry string) bool {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	for _, item := range r.timelineLogs {
		if item.Entry == entry {
			return true
		}
	}
	return false
}

func (r *testRuntimeForSingleFile) timelineContentContains(entry, needle string) bool {
	r.timelineMu.Lock()
	defer r.timelineMu.Unlock()
	for _, item := range r.timelineLogs {
		if item.Entry == entry && strings.Contains(item.Content, needle) {
			return true
		}
	}
	return false
}

func newTestTaskForSingleFile(ctx context.Context) *aicommon.AIStatefulTaskBase {
	emitter := aicommon.NewEmitter("single-file-test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	return aicommon.NewStatefulTaskBase("single-file-task", "single file test", ctx, emitter, true)
}

func mustBuildAction(t *testing.T, actionName string, fields map[string]any) *aicommon.Action {
	t.Helper()
	payload := map[string]any{
		"@action": actionName,
	}
	for k, v := range fields {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	action, err := aicommon.ExtractAction(string(raw), actionName)
	require.NoError(t, err)
	return action
}

func newLoopAndFactory(t *testing.T, runtime *testRuntimeForSingleFile, opts ...SingleFileModificationOption) (*reactloops.ReActLoop, *SingleFileModificationSuiteFactory, aicommon.AIStatefulTask) {
	t.Helper()
	factory := NewSingleFileModificationSuiteFactory(runtime, opts...)
	loop, err := reactloops.NewReActLoop("single-file-actions-test", runtime, factory.GetActions()...)
	require.NoError(t, err)
	task := newTestTaskForSingleFile(context.Background())
	loop.SetCurrentTask(task)
	return loop, factory, task
}

func TestWriteAction_EmptyCode_AddToTimelineAndFail(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	actionName := factory.GetActionName("write")
	codeVar := factory.GetCodeVariableName()
	loop.Set(codeVar, "")

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	terminated, failErr := op.IsTerminated()
	assert.True(t, terminated)
	assert.Error(t, failErr)
	assert.True(t, runtime.timelineContains("error"))
	assert.True(t, runtime.timelineContentContains("error", "No code generated"))
}

func TestWriteAction_NoFilename_EmitsNewFileAndWrites(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithFileExtension(".yak"))
	actionName := factory.GetActionName("write")
	codeVar := factory.GetCodeVariableName()
	filenameVar := factory.GetFilenameVariableName()
	loop.Set(codeVar, "println(\"ok\")")

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	filename := loop.Get(filenameVar)
	require.NotEmpty(t, filename)
	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "println(\"ok\")", string(content))
	assert.True(t, runtime.timelineContains("write_success"))
}

func TestWriteAction_WriteError_AddToTimelineAndFail(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	runtime.forcedEmit = runtime.tmpDir // writing file into a directory path should fail
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	actionName := factory.GetActionName("write")
	codeVar := factory.GetCodeVariableName()
	loop.Set(codeVar, "println(\"x\")")

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	terminated, failErr := op.IsTerminated()
	assert.True(t, terminated)
	assert.Error(t, failErr)
	assert.True(t, runtime.timelineContains("write_failed"))
}

func TestWriteAction_OnFileChanged_BlockingDisallowExit(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithFileChanged(func(content string, operator *reactloops.LoopActionHandlerOperator) (string, bool) {
			return "lint: blocking issue", true
		}),
	)
	actionName := factory.GetActionName("write")
	codeVar := factory.GetCodeVariableName()
	loop.Set(codeVar, "println(\"x\")")

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.GetDisallowLoopExit())
	assert.Contains(t, op.GetFeedback().String(), "lint: blocking issue")
	assert.True(t, runtime.timelineContains("lint-message"))
}

func TestWriteAction_ExitBehavior(t *testing.T) {
	t.Run("default exit", func(t *testing.T) {
		runtime := newTestRuntimeForSingleFile(t)
		loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
		loop.Set(factory.GetCodeVariableName(), "a")
		ac, _ := loop.GetActionHandler(factory.GetActionName("write"))
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("write"), nil), op)
		terminated, failErr := op.IsTerminated()
		assert.True(t, terminated)
		assert.NoError(t, failErr)
	})

	t.Run("no exit after write", func(t *testing.T) {
		runtime := newTestRuntimeForSingleFile(t)
		loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
		loop.Set(factory.GetCodeVariableName(), "a")
		ac, _ := loop.GetActionHandler(factory.GetActionName("write"))
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("write"), nil), op)
		terminated, failErr := op.IsTerminated()
		assert.False(t, terminated)
		assert.NoError(t, failErr)
	})
}

func TestModifyAction_Verifier_InvalidParams(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, _ := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 0,
		"modify_end_line":   1,
	})
	verifyErr := ac.ActionVerifier(loop, action)
	assert.Error(t, verifyErr)
}

func TestModifyAction_Success_ReplacesLines(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "m.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc")
	loop.Set(factory.GetCodeVariableName(), "B")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	})
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.Equal(t, "a\nB\nc", loop.Get(factory.GetFullCodeVariableName()))
	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a\nB\nc", string(content))
	assert.True(t, runtime.timelineContains("modify_success"))
}

func TestModifyAction_PrettifyMismatch_ContinueAndTimeline(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithCodePrettify(func(code string) (int, int, string, bool) {
			return 1, 1, "X", true
		}),
	)
	filename := filepath.Join(runtime.tmpDir, "m2.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc")
	loop.Set(factory.GetCodeVariableName(), "1 | X")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_warning"))
}

func TestModifyAction_Spinning_ReflectionAndFeedback(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithSpinDetection(func(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string) {
			return true, "same range repeatedly"
		}),
		WithReflectionPrompt(func(startLine, endLine int, reason string) string {
			return "please reflect before retry"
		}),
	)
	filename := filepath.Join(runtime.tmpDir, "spin.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc")
	loop.Set(factory.GetCodeVariableName(), "B")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.Equal(t, reactloops.ReflectionLevel_Deep, op.GetReflectionLevel())
	assert.Contains(t, op.GetFeedback().String(), "please reflect before retry")
	assert.True(t, runtime.timelineContains("spinning_detected"))
}

func TestInsertAction_VerifierAndSuccess(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	ac, err := loop.GetActionHandler(factory.GetActionName("insert"))
	require.NoError(t, err)

	invalid := mustBuildAction(t, factory.GetActionName("insert"), map[string]any{"insert_line": 0})
	assert.Error(t, ac.ActionVerifier(loop, invalid))

	filename := filepath.Join(runtime.tmpDir, "insert.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nc")
	loop.Set(factory.GetCodeVariableName(), "b\n")
	valid := mustBuildAction(t, factory.GetActionName("insert"), map[string]any{"insert_line": 2})
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, valid, op)

	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a\nb\nc", string(content))
	assert.True(t, runtime.timelineContains("insert_success"))
}

func TestInsertAction_InsertError_AddToTimeline(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	filename := filepath.Join(runtime.tmpDir, "insert_err.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a")
	loop.Set(factory.GetCodeVariableName(), "b")
	action := mustBuildAction(t, factory.GetActionName("insert"), map[string]any{"insert_line": -1})
	ac, _ := loop.GetActionHandler(factory.GetActionName("insert"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	terminated, failErr := op.IsTerminated()
	assert.True(t, terminated)
	assert.Error(t, failErr)
	assert.True(t, runtime.timelineContains("insert_failed"))
}

func TestDeleteAction_VerifierAndSuccess(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	ac, err := loop.GetActionHandler(factory.GetActionName("delete"))
	require.NoError(t, err)

	invalid := mustBuildAction(t, factory.GetActionName("delete"), map[string]any{"delete_start_line": 0})
	assert.Error(t, ac.ActionVerifier(loop, invalid))

	filename := filepath.Join(runtime.tmpDir, "del.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc")
	valid := mustBuildAction(t, factory.GetActionName("delete"), map[string]any{"delete_start_line": 2})
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, valid, op)

	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a\nc", string(content))
	assert.True(t, runtime.timelineContains("delete_success"))
}

func TestDeleteAction_DeleteRangeAndErrorTimeline(t *testing.T) {
	t.Run("range success", func(t *testing.T) {
		runtime := newTestRuntimeForSingleFile(t)
		loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
		filename := filepath.Join(runtime.tmpDir, "del_range.yak")
		loop.Set(factory.GetFilenameVariableName(), filename)
		loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc\nd")
		ac, _ := loop.GetActionHandler(factory.GetActionName("delete"))
		action := mustBuildAction(t, factory.GetActionName("delete"), map[string]any{
			"delete_start_line": 2,
			"delete_end_line":   3,
		})
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, action, op)
		content, readErr := os.ReadFile(filename)
		require.NoError(t, readErr)
		assert.Equal(t, "a\nd", string(content))
	})

	t.Run("delete error", func(t *testing.T) {
		runtime := newTestRuntimeForSingleFile(t)
		loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
		filename := filepath.Join(runtime.tmpDir, "del_err.yak")
		loop.Set(factory.GetFilenameVariableName(), filename)
		loop.Set(factory.GetFullCodeVariableName(), "a")
		ac, _ := loop.GetActionHandler(factory.GetActionName("delete"))
		action := mustBuildAction(t, factory.GetActionName("delete"), map[string]any{"delete_start_line": 99})
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, action, op)
		terminated, failErr := op.IsTerminated()
		assert.True(t, terminated)
		assert.Error(t, failErr)
		assert.True(t, runtime.timelineContains("delete_failed"))
	})
}

