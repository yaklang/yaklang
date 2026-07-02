package loop_yaklangcode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/schema"
)

type editorSyncCapturedEvents struct {
	mu     sync.Mutex
	events []*schema.AiOutputEvent
}

func (c *editorSyncCapturedEvents) appendEvent(e *schema.AiOutputEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *editorSyncCapturedEvents) byType(eventType schema.EventType) []*schema.AiOutputEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var matched []*schema.AiOutputEvent
	for _, e := range c.events {
		if e.Type == eventType {
			matched = append(matched, e)
		}
	}
	return matched
}

// TestYaklangEditorSync_LiveEmitsForEditDuringLoop verifies that an edit-mode (replace) target is
// delivered to the editor LIVE: the very first modify_code overwrites the file immediately, without
// waiting for the loop to finish. The internal filesystem_pin_filename event stays suppressed, and
// the final flush is a no-op (deduplicated by content).
func TestYaklangEditorSync_LiveEmitsForEditDuringLoop(t *testing.T) {
	capture := &editorSyncCapturedEvents{}
	runtime := mock.NewMockInvoker(context.Background())
	cfg := runtime.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("yaklang-editor-sync-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})

	modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
		runtime,
		loopinfra.WithLoopVarsPrefix("yak"),
		loopinfra.WithActionSuffix("code"),
		loopinfra.WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
		loopinfra.WithFileExtension(".yak"),
	)

	loop, err := reactloops.NewReActLoop(
		"yaklang-editor-sync-test",
		runtime,
		append(modSuite.GetActions(), withYaklangDeferredEditorSync())...,
	)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("task", "test", context.Background(), cfg.Emitter, true)
	loop.SetCurrentTask(task)

	filename := filepath.Join(t.TempDir(), "demo.yak")
	require.NoError(t, os.WriteFile(filename, []byte("a\nold\nc\n"), 0o644))
	loop.Set("editor_file_path", filename)
	loop.Set("filename", filename)
	loop.Set("full_code", "a\nold\nc\n")
	loop.Set("yak_code", "new")

	modifyAction, err := loop.GetActionHandler("modify_code")
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	modifyAction.ActionHandler(loop, mustBuildYaklangAction(t, "modify_code", map[string]any{
		"modify_start_line":  2,
		"modify_end_line":    2,
		"modify_code_reason": "replace middle line",
	}), op)

	// Live delivery: the edit already reached the editor during the loop, before any flush.
	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)
	assert.Empty(t, capture.byType(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME))

	var payload yaklangCodeChangeEvent
	require.NoError(t, json.Unmarshal(events[0].Content, &payload))
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpReplace, payload.Op)
	assert.Equal(t, "a\nnew\nc", payload.Code.Content)
	assert.Equal(t, filename, payload.Code.Path)
	assert.Equal(t, "modify_code", payload.SourceAction)

	// Disk is overwritten live as well.
	disk, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, "a\nnew\nc", string(disk))

	// The loop-end flush must not re-emit the same content.
	flushYaklangDeferredEditorSync(loop)
	assert.Len(t, capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE), 1)
}

func TestYaklangDeferredEditorSync_CreateModePersistsToCodeDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	capture := &editorSyncCapturedEvents{}
	runtime := mock.NewMockInvoker(context.Background())
	cfg := runtime.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("yaklang-editor-sync-create-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})

	loop, err := reactloops.NewReActLoop("yaklang-editor-sync-create-test", runtime)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("task", "test", context.Background(), cfg.Emitter, true)
	loop.SetCurrentTask(task)

	genPath, err := newYaklangGenCodePath()
	require.NoError(t, err)
	loop.Set("filename", genPath)
	loop.Set("full_code", "println(\"create\")")
	loop.Set(yaklangEditorSyncPendingLoopKey, true)

	flushYaklangDeferredEditorSync(loop)

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)

	var payload yaklangCodeChangeEvent
	require.NoError(t, json.Unmarshal(events[0].Content, &payload))
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpCreate, payload.Op)
	assert.Equal(t, "println(\"create\")", payload.Code.Content)
	assert.Equal(t, genPath, payload.Code.Path)

	data, readErr := os.ReadFile(genPath)
	require.NoError(t, readErr)
	assert.Equal(t, "println(\"create\")", string(data))
}

func TestYaklangDeferredEditorSync_EditorFileWinsOverGenCodePath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	capture := &editorSyncCapturedEvents{}
	runtime := mock.NewMockInvoker(context.Background())
	cfg := runtime.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("yaklang-editor-sync-target-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})

	loop, err := reactloops.NewReActLoop("yaklang-editor-sync-target-test", runtime)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("task", "test", context.Background(), cfg.Emitter, true)
	loop.SetCurrentTask(task)

	editorFile := filepath.Join(base, "123.yak")
	genCodePath, err := newYaklangGenCodePath()
	require.NoError(t, err)

	loop.Set("editor_file_path", editorFile)
	loop.Set("filename", genCodePath)
	loop.Set("full_code", "println(\"target\")")
	loop.Set(yaklangEditorSyncPendingLoopKey, true)

	flushYaklangDeferredEditorSync(loop)

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)

	var payload yaklangCodeChangeEvent
	require.NoError(t, json.Unmarshal(events[0].Content, &payload))
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpReplace, payload.Op)
	assert.Equal(t, filepath.Clean(editorFile), filepath.Clean(payload.Code.Path))

	data, readErr := os.ReadFile(editorFile)
	require.NoError(t, readErr)
	assert.Equal(t, "println(\"target\")", string(data))
}

func TestYaklangDeferredEditorSync_SkipsFlushWhenSeedUnchanged(t *testing.T) {
	capture := &editorSyncCapturedEvents{}
	runtime := mock.NewMockInvoker(context.Background())
	cfg := runtime.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("yaklang-editor-sync-seed-skip", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})

	loop, err := reactloops.NewReActLoop("yaklang-editor-sync-seed-skip", runtime, withYaklangDeferredEditorSync())
	require.NoError(t, err)

	seed := "println(\"from previous session\")"
	editorFile := filepath.Join(t.TempDir(), "shared.yak")
	require.NoError(t, os.WriteFile(editorFile, []byte(seed), 0o644))

	loop.Set("editor_file_path", editorFile)
	loop.Set("full_code", seed)
	loop.Set(loopinfra.LoopVarInitSeedFullCode, seed)
	loop.Set(loopinfra.LoopVarCodeSeededOnly, true)
	loop.Set(yaklangEditorSyncPendingLoopKey, true)

	flushYaklangDeferredEditorSync(loop)

	assert.Empty(t, capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE))

	data, readErr := os.ReadFile(editorFile)
	require.NoError(t, readErr)
	assert.Equal(t, seed, string(data))
}

func TestYaklangDeferredEditorSync_FlushesAfterWriteCodeOnSeed(t *testing.T) {
	capture := &editorSyncCapturedEvents{}
	runtime := mock.NewMockInvoker(context.Background())
	cfg := runtime.GetConfig().(*mock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("yaklang-editor-sync-seed-write", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.appendEvent(e)
		return e, nil
	})

	modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
		runtime,
		loopinfra.WithLoopVarsPrefix("yak"),
		loopinfra.WithActionSuffix("code"),
		loopinfra.WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
		loopinfra.WithFileExtension(".yak"),
	)

	loop, err := reactloops.NewReActLoop(
		"yaklang-editor-sync-seed-write",
		runtime,
		append(modSuite.GetActions(), withYaklangDeferredEditorSync())...,
	)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("task", "test", context.Background(), cfg.Emitter, true)
	loop.SetCurrentTask(task)

	seed := "old session code"
	editorFile := filepath.Join(t.TempDir(), "shared.yak")
	require.NoError(t, os.WriteFile(editorFile, []byte(seed), 0o644))

	loop.Set("editor_file_path", editorFile)
	loop.Set("filename", editorFile)
	loop.Set("full_code", seed)
	loop.Set(loopinfra.LoopVarInitSeedFullCode, seed)
	loop.Set(loopinfra.LoopVarCodeSeededOnly, true)
	loop.Set("yak_code", "println(\"new task code\")")

	writeAction, err := loop.GetActionHandler("write_code")
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	writeAction.ActionHandler(loop, mustBuildYaklangAction(t, "write_code", nil), op)

	flushYaklangDeferredEditorSync(loop)

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)

	var payload yaklangCodeChangeEvent
	require.NoError(t, json.Unmarshal(events[0].Content, &payload))
	assert.Equal(t, "println(\"new task code\")", payload.Code.Content)
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpReplace, payload.Op)

	data, readErr := os.ReadFile(editorFile)
	require.NoError(t, readErr)
	assert.Equal(t, "println(\"new task code\")", string(data))
}

func mustBuildYaklangAction(t *testing.T, actionName string, fields map[string]any) *aicommon.Action {
	t.Helper()
	payload := map[string]any{"@action": actionName}
	for k, v := range fields {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	action, err := aicommon.ExtractAction(string(raw), actionName)
	require.NoError(t, err)
	return action
}
