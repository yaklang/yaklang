package loopinfra

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type yaklangCodeChangeEventPayload struct {
	Op           string `json:"op"`
	Reason       string `json:"reason,omitempty"`
	SourceAction string `json:"source_action,omitempty"`
	Code         struct {
		Content string `json:"content"`
		Path    string `json:"path,omitempty"`
		Summary string `json:"summary,omitempty"`
		Version int    `json:"version"`
	} `json:"code"`
}

type capturedEvents struct {
	mu     sync.Mutex
	events []*schema.AiOutputEvent
}

func (c *capturedEvents) appendEvent(e *schema.AiOutputEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *capturedEvents) byType(eventType schema.EventType) []*schema.AiOutputEvent {
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

func newYaklangFactory(t *testing.T, runtime *testRuntimeForSingleFile) *SingleFileModificationSuiteFactory {
	t.Helper()
	return NewSingleFileModificationSuiteFactory(runtime,
		WithActionSuffix("code"),
		WithFileExtension(".yak"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	)
}

func newLoopWithCapturedEvents(t *testing.T, runtime *testRuntimeForSingleFile, factory *SingleFileModificationSuiteFactory) (*reactloops.ReActLoop, *capturedEvents, aicommon.AIStatefulTask) {
	t.Helper()
	capture := &capturedEvents{}
	runtime.wireEmitterCapture(capture)
	loop, err := reactloops.NewReActLoop("yaklang-code-state-test", runtime, factory.GetActions()...)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("yaklang-code-state-task", "yaklang code state test", context.Background(), runtime.GetConfig().GetEmitter(), true)
	loop.SetCurrentTask(task)
	return loop, capture, task
}

func parseYaklangCodeChangeEvent(t *testing.T, e *schema.AiOutputEvent) yaklangCodeChangeEventPayload {
	t.Helper()
	require.Equal(t, schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, e.Type)
	require.Equal(t, loopYaklangCodeChangeEventNode, e.NodeId)
	require.True(t, e.IsJson)

	var payload yaklangCodeChangeEventPayload
	require.NoError(t, json.Unmarshal(e.Content, &payload))
	return payload
}

func TestApplyLoopYaklangCodeChange_NonYaklangContentType_NoOp(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := NewSingleFileModificationSuiteFactory(runtime, WithActionSuffix("code"))
	loop, capture, _ := newLoopWithCapturedEvents(t, runtime, factory)

	result, err := factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      "println(\"x\")",
		Path:         "/tmp/demo.yak",
		SourceAction: "write_code",
		EmitEvent:    true,
	})
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Empty(t, capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE))
}

func TestApplyLoopYaklangCodeChange_UpdatesLoopStateAndVersion(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := newYaklangFactory(t, runtime)
	loop, capture, _ := newLoopWithCapturedEvents(t, runtime, factory)

	filename := runtime.EmitFileArtifactWithExt("demo", ".yak", nil)
	content := "println(\"hello\")"

	result, err := factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      content,
		Path:         filename,
		SourceAction: "write_code",
		ChangeReason: "initial write",
		EventOp:      loopYaklangCodeEventOpReplace,
		EmitEvent:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.CurrentState)

	assert.Equal(t, content, loop.Get(factory.GetFullCodeVariableName()))
	assert.Equal(t, filename, loop.Get(factory.GetFilenameVariableName()))
	assert.Equal(t, 1, result.CurrentState.Version)
	assert.Equal(t, "write_code", result.CurrentState.SourceAction)
	assert.Equal(t, "initial write", result.CurrentState.ChangeReason)

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)
	payload := parseYaklangCodeChangeEvent(t, events[0])
	assert.Equal(t, loopYaklangCodeEventOpReplace, payload.Op)
	assert.Equal(t, content, payload.Code.Content)
	assert.Equal(t, filename, payload.Code.Path)
	assert.Equal(t, 1, payload.Code.Version)
	assert.Equal(t, "initial write", payload.Reason)
	assert.Equal(t, "write_code", payload.SourceAction)
}

func TestApplyLoopYaklangCodeChange_VersionIncrements(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := newYaklangFactory(t, runtime)
	loop, _, _ := newLoopWithCapturedEvents(t, runtime, factory)

	first, err := factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      "v1",
		Path:         "/tmp/v.yak",
		SourceAction: "write_code",
		EmitEvent:    false,
	})
	require.NoError(t, err)
	require.Equal(t, 1, first.CurrentState.Version)

	second, err := factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      "v2",
		Path:         "/tmp/v.yak",
		SourceAction: "modify_code",
		EmitEvent:    false,
	})
	require.NoError(t, err)
	require.Equal(t, 2, second.CurrentState.Version)
	assert.Equal(t, "v1", second.PreviousState.Content)
	assert.Equal(t, "v2", second.CurrentState.Content)
}

func TestApplyLoopYaklangCodeChange_SummaryTruncation(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := newYaklangFactory(t, runtime)
	loop, capture, _ := newLoopWithCapturedEvents(t, runtime, factory)

	longContent := strings.Repeat("x", 250)
	_, err := factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content:      longContent,
		Path:         "/tmp/long.yak",
		SourceAction: "write_code",
		EmitEvent:    true,
	})
	require.NoError(t, err)

	events := capture.byType(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE)
	require.Len(t, events, 1)
	payload := parseYaklangCodeChangeEvent(t, events[0])
	assert.Equal(t, longContent[:200]+"...", payload.Code.Summary)
}

func TestApplyLoopYaklangCodeChange_InvalidInput(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	factory := newYaklangFactory(t, runtime)
	loop, _, _ := newLoopWithCapturedEvents(t, runtime, factory)

	_, err := factory.applyLoopYaklangCodeChange(nil, &loopYaklangCodeChange{
		Content: "x", SourceAction: "write_code",
	})
	assert.Error(t, err)

	_, err = factory.applyLoopYaklangCodeChange(loop, nil)
	assert.Error(t, err)

	_, err = factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content: "", SourceAction: "write_code",
	})
	assert.Error(t, err)

	_, err = factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content: "x", SourceAction: "",
	})
	assert.Error(t, err)

	_, err = factory.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
		Content: "x", SourceAction: "write_code", EventOp: "merge",
	})
	assert.Error(t, err)
}
