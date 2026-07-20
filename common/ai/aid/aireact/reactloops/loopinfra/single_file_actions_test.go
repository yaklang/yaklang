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

func (r *testRuntimeForSingleFile) wireEmitterCapture(capture *capturedEvents) {
	cfg := r.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("single-file-test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})
}

func newTestTaskForSingleFile(ctx context.Context) *aicommon.AIStatefulTaskBase {
	emitter := aicommon.NewEmitter("single-file-test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	return aicommon.NewStatefulTaskBase("single-file-task", "single file test", ctx, emitter, true)
}

func newLoopAndFactoryWithEvents(t *testing.T, runtime *testRuntimeForSingleFile, opts ...SingleFileModificationOption) (*reactloops.ReActLoop, *SingleFileModificationSuiteFactory, aicommon.AIStatefulTask, *capturedEvents) {
	t.Helper()
	capture := &capturedEvents{}
	runtime.wireEmitterCapture(capture)
	factory := NewSingleFileModificationSuiteFactory(runtime, opts...)
	loop, err := reactloops.NewReActLoop("single-file-actions-test", runtime, factory.GetActions()...)
	require.NoError(t, err)
	task := newTestTaskForSingleFile(context.Background())
	loop.SetCurrentTask(task)
	return loop, factory, task, capture
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

func yaklangSingleFileOpts(extra ...SingleFileModificationOption) []SingleFileModificationOption {
	opts := []SingleFileModificationOption{
		WithActionSuffix("code"),
		WithFileExtension(".yak"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	}
	return append(opts, extra...)
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

// TestWriteAction_EmptyCode_AddToTimelineAndFail covers the empty-write fault tolerance:
// a single empty write_code feeds back a correction and continues (does NOT abort the task);
// only after several consecutive empty writes does it give up and fail. Every empty write still
// records a "No code generated" diagnosis on the timeline.
func TestWriteAction_EmptyCode_AddToTimelineAndFail(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	actionName := factory.GetActionName("write")
	codeVar := factory.GetCodeVariableName()

	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)

	// First empty write: feedback + continue, not terminated.
	loop.Set(codeVar, "")
	firstOp := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, nil), firstOp)

	terminated, failErr := firstOp.IsTerminated()
	assert.False(t, terminated, "single empty write should not abort the task")
	assert.NoError(t, failErr)
	assert.NotEmpty(t, firstOp.GetFeedback().String(), "empty write should feed back a correction")
	assert.True(t, runtime.timelineContains("error"))
	assert.True(t, runtime.timelineContentContains("error", "No code generated"))

	// Consecutive empty writes eventually give up and fail.
	var lastOp *reactloops.LoopActionHandlerOperator
	for i := 0; i < 5; i++ {
		loop.Set(codeVar, "")
		lastOp = reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, actionName, nil), lastOp)
		if done, _ := lastOp.IsTerminated(); done {
			break
		}
	}

	terminated, failErr = lastOp.IsTerminated()
	assert.True(t, terminated, "repeated empty writes should eventually fail")
	assert.Error(t, failErr)
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
	assert.True(t, runtime.timelineContains("file_write"))
	assert.True(t, runtime.timelineContentContains("file_write", "println"))
}

func TestWriteAction_AllowedWhenSeededOnly(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, yaklangSingleFileOpts()...)
	actionName := factory.GetActionName("write")
	fullCodeVar := factory.GetFullCodeVariableName()
	codeVar := factory.GetCodeVariableName()

	seed := "seed from previous session"
	loop.Set(fullCodeVar, seed)
	loop.Set(LoopVarInitSeedFullCode, seed)
	loop.Set(LoopVarCodeSeededOnly, true)
	loop.Set(codeVar, "println(\"replacement\")")
	loop.Set("filename", filepath.Join(runtime.tmpDir, "demo.yak"))

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	require.NotNil(t, ac.ActionVerifier)
	require.NoError(t, ac.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.Equal(t, "println(\"replacement\")", loop.Get(fullCodeVar))
	assert.False(t, isLoopCodeSeededOnly(loop))
	assert.Greater(t, loop.GetInt(loopYaklangCodeVersionKey), 0)
}

func TestWriteAction_RejectedWhenExistingCodeNotSeedOnly(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, _ := newLoopAndFactory(t, runtime, yaklangSingleFileOpts()...)
	actionName := factory.GetActionName("write")
	fullCodeVar := factory.GetFullCodeVariableName()

	loop.Set(fullCodeVar, "already edited code")
	loop.Set(LoopVarInitSeedFullCode, "seed from disk")
	loop.Set(LoopVarCodeSeededOnly, false)

	action := mustBuildAction(t, actionName, nil)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	require.NotNil(t, ac.ActionVerifier)
	verifyErr := ac.ActionVerifier(loop, action)
	require.Error(t, verifyErr)
	assert.Contains(t, verifyErr.Error(), "code already exists")
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
		WithFileChanged(func(loop *reactloops.ReActLoop, content string, operator *reactloops.LoopActionHandlerOperator) (string, bool) {
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

func TestModifyAction_ContinuesAfterSyntaxClean(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithExitAfterWrite(false),
		WithExitWhenSyntaxClean(true),
	)
	filename := filepath.Join(runtime.tmpDir, "clean.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nb\nc")
	loop.Set(factory.GetCodeVariableName(), "B")

	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	}), op)

	// modify 操作不再自动退出：AI 需要通过 finish 主动退出循环
	terminated, failErr := op.IsTerminated()
	assert.False(t, terminated)
	assert.NoError(t, failErr)
	assert.Equal(t, "true", loop.Get(factory.GetLintStatusVariableName()))
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
	assert.True(t, runtime.timelineContains("file_modify"))
	assert.True(t, runtime.timelineContentContains("file_modify", "-b"))
	assert.True(t, runtime.timelineContentContains("file_modify", "+B"))
}

func TestModifyAction_CodeLineBase_AbsoluteLineNumbers(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "sel.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "urlDecoded, err := codec.DecodeUrl(urlEncoded)\ndie(err)\nyakit.Info(\"old\")")
	loop.Set(LoopVarCodeLineBase, 27)
	loop.Set(factory.GetCodeVariableName(), "urlDecoded, err := codec.DecodeUrl(urlEncoded)\ndie(err)\nyakit.Info(\"new\")")

	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 28,
		"modify_end_line":   30,
	}), op)

	require.False(t, op.IsContinued())
	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), `yakit.Info("new")`)
	assert.True(t, runtime.timelineContains("modify_success"))
}

func TestNormalizeActionLineNumber(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, _ := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	fullCodeVar := factory.GetFullCodeVariableName()
	loop.Set(fullCodeVar, "a\nb\nc\nd")
	loop.Set(LoopVarCodeLineBase, 27)

	assert.Equal(t, 1, NormalizeActionLineNumber(loop, fullCodeVar, 28))
	assert.Equal(t, 3, NormalizeActionLineNumber(loop, fullCodeVar, 30))
	assert.Equal(t, 2, NormalizeActionLineNumber(loop, fullCodeVar, 2))
	assert.Equal(t, 99, NormalizeActionLineNumber(loop, fullCodeVar, 99))
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
		"modify_start_line": 10,
		"modify_end_line":   10,
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_warning"))
	assert.NotEmpty(t, op.GetFeedback().String())
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
	assert.True(t, runtime.timelineContains("file_insert"))
	assert.True(t, runtime.timelineContentContains("file_insert", "+b"))
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
	assert.True(t, runtime.timelineContains("file_delete"))
	assert.True(t, runtime.timelineContentContains("file_delete", "-b"))
}

func TestWriteAction_Yaklang_CodeChangeEventMatchesDiskOverwrite(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task, capture := newLoopAndFactoryWithEvents(t, runtime, yaklangSingleFileOpts()...)
	actionName := factory.GetActionName("write")
	code := "println(\"overwrite\")"
	loop.Set(factory.GetCodeVariableName(), code)

	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, nil), op)

	filename := loop.Get(factory.GetFilenameVariableName())
	require.NotEmpty(t, filename)

	diskContent, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, code, string(diskContent))

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)
	payload := parseYaklangCodeChangeEvent(t, events[0])
	assert.Equal(t, loopYaklangCodeEventOpCreate, payload.Op)
	assert.Equal(t, code, payload.Code.Content)
	assert.Equal(t, filename, payload.Code.Path)
	assert.Equal(t, actionName, payload.SourceAction)
	assert.Equal(t, string(diskContent), payload.Code.Content)
}

func TestModifyAction_Yaklang_CodeChangeEventMatchesDiskOverwrite(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task, capture := newLoopAndFactoryWithEvents(t, runtime, yaklangSingleFileOpts(WithExitAfterWrite(false))...)
	actionName := factory.GetActionName("modify")

	filename := filepath.Join(runtime.tmpDir, "overwrite.yak")
	require.NoError(t, os.WriteFile(filename, []byte("a\nold\nc"), 0644))
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nold\nc")
	loop.Set(factory.GetCodeVariableName(), "new")

	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
		"modify_start_line":  2,
		"modify_end_line":    2,
		"modify_code_reason": "replace middle line",
	}), op)

	expected := "a\nnew\nc"
	diskContent, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, expected, string(diskContent))
	assert.Equal(t, expected, loop.Get(factory.GetFullCodeVariableName()))

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)
	payload := parseYaklangCodeChangeEvent(t, events[0])
	assert.Equal(t, loopYaklangCodeEventOpReplace, payload.Op)
	assert.Equal(t, expected, payload.Code.Content)
	assert.Equal(t, filename, payload.Code.Path)
	assert.Equal(t, actionName, payload.SourceAction)
	assert.Equal(t, "replace middle line", payload.Reason)
	assert.Equal(t, string(diskContent), payload.Code.Content)
}

func TestWriteAction_NonYaklangContentType_DoesNotEmitCodeChange(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task, capture := newLoopAndFactoryWithEvents(t, runtime, WithActionSuffix("code"), WithFileExtension(".yak"))
	loop.Set(factory.GetCodeVariableName(), "println(\"plain\")")

	ac, err := loop.GetActionHandler(factory.GetActionName("write"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("write"), nil), op)

	filename := loop.Get(factory.GetFilenameVariableName())
	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "println(\"plain\")", string(content))
	assert.Empty(t, capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE))
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

func TestWriteAction_DeferDiskWrite_SkipsFileCreation(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithFileExtension(".yak"),
		WithDeferDiskWrite(true),
	)
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
	assert.Equal(t, "println(\"ok\")", loop.Get(factory.GetFullCodeVariableName()))
	if _, statErr := os.Stat(filename); statErr == nil {
		data, readErr := os.ReadFile(filename)
		require.NoError(t, readErr)
		assert.Empty(t, data)
	}
	assert.True(t, runtime.timelineContains("write_success"))
	assert.True(t, runtime.timelineContentContains("write_success", "disk write deferred"))
}

func TestModifyAction_DeferDiskWrite_PreservesExistingFile(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	filename := filepath.Join(runtime.tmpDir, "existing.yak")
	require.NoError(t, os.WriteFile(filename, []byte("a\nold\nc"), 0644))

	loop, factory, task := newLoopAndFactory(t, runtime,
		WithActionSuffix("code"),
		WithDeferDiskWrite(true),
	)
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nold\nc")
	loop.Set(factory.GetCodeVariableName(), "new")

	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	})
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	diskContent, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a\nold\nc", string(diskContent))
	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), "new")
	assert.True(t, runtime.timelineContentContains("modify_success", "disk write deferred"))
}

func TestModifyAction_OldSnippet_UniqueMatch(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "snippet.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "yakit.AutoInitYakit()\nx = 1\ny = 2\n")
	loop.Set(factory.GetCodeVariableName(), "x = 42")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"old_snippet": "x = 1",
	})
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), "x = 42")
	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "x = 42")
	assert.True(t, runtime.timelineContains("modify_success"))
}

func TestModifyAction_OldSnippet_NormalizedUniqueMatch(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "snippet_normalized.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "before\r\nx = 1  \r\nafter\r\n")
	loop.Set(factory.GetCodeVariableName(), "before\nx = 42\nafter")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"old_snippet": "before\nx = 1\nafter",
	})
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.Equal(t, "before\nx = 42\nafter\r\n", loop.Get(factory.GetFullCodeVariableName()))
	assert.True(t, runtime.timelineContains("modify_success"))
}

func TestModifyAction_OldSnippet_NotFound_Continue(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "snippet2.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a = 1\n")
	loop.Set(factory.GetCodeVariableName(), "a = 2")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"old_snippet": "missing",
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_snippet_not_found"))
	assert.Contains(t, op.GetFeedback().String(), "CURRENT_CODE")
	assert.Contains(t, op.GetFeedback().String(), "Cursor Patch")
}

func TestModifyAction_OldSnippet_Ambiguous_Continue(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "snippet3.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "x=1\nfoo\nx=1\n")
	loop.Set(factory.GetCodeVariableName(), "x=9")

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"old_snippet": "x=1",
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_snippet_ambiguous"))
}

// TestModifyAction_OldSnippet_EmptyCode_SingleRetry verifies that a single empty
// GEN_CODE block does NOT fail the task — it feeds back a correction hint (guiding
// toward line-range mode) and continues, giving the AI a chance to retry.
func TestModifyAction_OldSnippet_EmptyCode_SingleRetry(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "empty_snippet.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a = 1\nb = 2\n")
	loop.Set(factory.GetCodeVariableName(), "") // empty code

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"old_snippet": "a = 1",
	})
	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	terminated, failErr := op.IsTerminated()
	assert.False(t, terminated, "single empty old_snippet modify should NOT abort")
	assert.NoError(t, failErr)
	assert.True(t, op.IsContinued(), "should continue for retry")
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "GEN_CODE")
	assert.Contains(t, feedback, "modify_start_line", "feedback should suggest line-range alternative")
	assert.True(t, runtime.timelineContains("error"))

	// full_code must remain unchanged
	assert.Equal(t, "a = 1\nb = 2\n", loop.Get(factory.GetFullCodeVariableName()))
}

// TestModifyAction_OldSnippet_EmptyCode_EscalatesToLineRangeHint verifies that
// after 3+ consecutive empty GEN_CODE blocks, feedback escalates to a stronger
// warning insisting the AI switch to line-range mode.
func TestModifyAction_OldSnippet_EmptyCode_EscalatesToLineRangeHint(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "escalate.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a = 1\nb = 2\n")

	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	actionName := factory.GetActionName("modify")

	// Fire 3 consecutive empty old_snippet modifies
	for i := 0; i < 3; i++ {
		loop.Set(factory.GetCodeVariableName(), "")
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
			"old_snippet": "a = 1",
		}), op)
		terminated, _ := op.IsTerminated()
		assert.False(t, terminated, "attempt %d should not abort", i+1)
		assert.True(t, op.IsContinued())
	}

	// 4th attempt should have escalated feedback
	loop.Set(factory.GetCodeVariableName(), "")
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
		"old_snippet": "a = 1",
	}), op)
	terminated, _ := op.IsTerminated()
	assert.False(t, terminated, "4th attempt should still not abort")
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "改用行号模式", "escalated feedback should insist on line-range mode")
}

// TestModifyAction_OldSnippet_EmptyCode_EventuallyFails verifies that after
// maxEmptySnippetRetry (5) consecutive empty attempts, the action does Fail
// to prevent infinite loops.
func TestModifyAction_OldSnippet_EmptyCode_EventuallyFails(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "fail.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a = 1\nb = 2\n")

	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	actionName := factory.GetActionName("modify")

	var lastOp *reactloops.LoopActionHandlerOperator
	for i := 0; i < 10; i++ {
		loop.Set(factory.GetCodeVariableName(), "")
		lastOp = reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
			"old_snippet": "a = 1",
		}), lastOp)
		if done, _ := lastOp.IsTerminated(); done {
			break
		}
	}

	terminated, failErr := lastOp.IsTerminated()
	assert.True(t, terminated, "should eventually fail after max retries")
	assert.Error(t, failErr)
	assert.Contains(t, failErr.Error(), "modify_start_line", "final error should suggest line-range alternative")
}

// TestModifyAction_OldSnippet_EmptyCode_ResetAfterSuccess verifies that a
// successful old_snippet modify resets the empty-code retry counter, so
// a later empty attempt gets a fresh set of retries.
func TestModifyAction_OldSnippet_EmptyCode_ResetAfterSuccess(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "reset.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a = 1\nb = 2\n")

	ac, _ := loop.GetActionHandler(factory.GetActionName("modify"))
	actionName := factory.GetActionName("modify")

	// 2 consecutive empty attempts
	for i := 0; i < 2; i++ {
		loop.Set(factory.GetCodeVariableName(), "")
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
			"old_snippet": "a = 1",
		}), op)
		terminated, _ := op.IsTerminated()
		assert.False(t, terminated)
	}

	// Successful modify resets the counter
	loop.Set(factory.GetCodeVariableName(), "a = 42")
	successOp := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
		"old_snippet": "a = 1",
	}), successOp)
	terminated, failErr := successOp.IsTerminated()
	assert.False(t, terminated)
	assert.NoError(t, failErr)
	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), "a = 42")

	// After success, empty counter should be reset; next empty should not immediately fail
	loop.Set(factory.GetCodeVariableName(), "")
	retryOp := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
		"old_snippet": "a = 42",
	}), retryOp)
	terminated, failErr = retryOp.IsTerminated()
	assert.False(t, terminated, "after a successful modify, the counter should be reset")
	assert.NoError(t, failErr)
	assert.True(t, retryOp.IsContinued())
}

func TestModifyAction_Verifier_AllowsPatchOnlyParams(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, _ := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_code_reason": "apply patch",
	})
	require.NoError(t, ac.ActionVerifier(loop, action))
}

func TestModifyAction_Patch_Success_FullCodeHasNoPatchMarkers(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, yaklangSingleFileOpts(WithExitAfterWrite(false))...)
	filename := filepath.Join(runtime.tmpDir, "patch_ok.yak")
	orig := "a = 1\nb = 2\nc = 3\n"
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	loop.Set(factory.GetCodeVariableName(), `*** Begin Patch
*** Update File: patch_ok.yak
@@ replace b
 a = 1
-b = 2
+b = 42
 c = 3
*** End Patch`)

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_code_reason": "bump b",
	})
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	full := loop.Get(factory.GetFullCodeVariableName())
	assert.Equal(t, "a = 1\nb = 42\nc = 3\n", full)
	assert.NotContains(t, full, "*** Begin Patch")
	assert.True(t, runtime.timelineContains("modify_success"))

	disk, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a = 1\nb = 42\nc = 3\n", string(disk))
	assert.NotContains(t, string(disk), "*** Begin Patch")

	state := getLoopYaklangCodeState(loop, factory.GetFullCodeVariableName(), factory.GetFilenameVariableName())
	require.NotNil(t, state)
	assert.Equal(t, full, state.Content)
	assert.NotContains(t, state.Content, "*** Begin Patch")
}

func TestModifyAction_Patch_ApplyFailed_LeavesFileUnchanged(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "patch_fail.yak")
	orig := "a = 1\n"
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	_ = os.WriteFile(filename, []byte(orig), 0o644)
	loop.Set(factory.GetCodeVariableName(), `*** Begin Patch
@@ miss
-missing_line
+x
*** End Patch`)

	action := mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_code_reason": "will fail",
	})
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_patch_apply_failed"))
	assert.Contains(t, op.GetFeedback().String(), "CURRENT_CODE")
	assert.Equal(t, orig, loop.Get(factory.GetFullCodeVariableName()))
	disk, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, orig, string(disk))
}

func TestModifyAction_RejectsLargeNonPatchCode(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "reject_full_file.yak")
	orig := "yakit.AutoInitYakit()\n\nbeforeRequest = func(req) {\n    return req\n}\n"
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	require.NoError(t, os.WriteFile(filename, []byte(orig), 0o644))
	loop.Set(factory.GetCodeVariableName(), `yakit.AutoInitYakit()

beforeRequest = func(req) {
    reqStr = string(req)
    modified = re.ReplaceAll(reqStr, "a=1111", "a=2222")
    yakit.Info("replaced")
    return []byte(modified)
}`)

	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_code_reason": "replace query value",
	}), op)

	assert.True(t, op.IsContinued())
	assert.True(t, runtime.timelineContains("modify_non_patch_full_code"))
	assert.Contains(t, op.GetFeedback().String(), "*** Begin Patch")
	assert.Equal(t, orig, loop.Get(factory.GetFullCodeVariableName()))
	disk, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, orig, string(disk))
}

func TestModifyAction_LegacyLineRange_StillWorksWithoutPatch(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "legacy_line.yak")
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), "a\nold\nc")
	loop.Set(factory.GetCodeVariableName(), "new")

	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 2,
		"modify_end_line":   2,
	}), op)

	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), "new")
	assert.NotContains(t, loop.Get(factory.GetFullCodeVariableName()), "*** Begin Patch")
	assert.True(t, runtime.timelineContains("modify_success"))
}

// Short whole-file line-range rewrite (syntaxflow-style) must still be allowed without a patch.
func TestModifyAction_LegacyLineRange_AllowsShortFullRewrite(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("rule"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "rule.sf")
	orig := `rule("test-rule")
desc(
	title: "Test Rule"
	type: audit
	level: info
`
	fixed := `rule("test-rule")
desc(
	title: "Test Rule Fixed"
	type: audit
	level: info
)`
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	loop.Set(factory.GetCodeVariableName(), fixed)

	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line": 1,
		"modify_end_line":   6,
	}), op)

	assert.False(t, runtime.timelineContains("modify_non_patch_full_code"))
	assert.Contains(t, loop.Get(factory.GetFullCodeVariableName()), "Test Rule Fixed")
	assert.True(t, runtime.timelineContains("modify_success"))
}
