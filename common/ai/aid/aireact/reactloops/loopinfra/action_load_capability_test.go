package loopinfra

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
)

// testInvoker wraps MockInvoker with configurable behavior for each dispatch branch.
type testInvoker struct {
	*mock.MockInvoker

	mu                       sync.Mutex
	toolCallName             string
	toolCallCalled           bool
	toolCallResult           *aitool.ToolResult
	toolCallDirectly         bool
	toolCallErr              error
	forgeCallName            string
	forgeCalled              bool
	forgeOnFinish            func(error)
	executeLoopCalled        bool
	executeLoopName          string
	executeLoopResult        bool
	executeLoopErr           error
	executeLoopCallback      func(string, aicommon.AIStatefulTask) (bool, error)
	verifySatisfactionResult *aicommon.VerifySatisfactionResult
	currentTask              aicommon.AIStatefulTask
	timelineEntries          []string
}

func newTestInvoker(ctx context.Context) *testInvoker {
	return &testInvoker{
		MockInvoker:              mock.NewMockInvoker(ctx),
		verifySatisfactionResult: aicommon.NewVerifySatisfactionResult(true, "", ""),
	}
}

func (t *testInvoker) ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCallCalled = true
	t.toolCallName = name
	return t.toolCallResult, t.toolCallDirectly, t.toolCallErr
}

func (t *testInvoker) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.forgeCalled = true
	t.forgeCallName = forgeName
	t.forgeOnFinish = onFinish
}

func (t *testInvoker) ExecuteLoopTaskIF(taskTypeName string, task aicommon.AIStatefulTask, options ...any) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.executeLoopCalled = true
	t.executeLoopName = taskTypeName
	if t.executeLoopCallback != nil {
		return t.executeLoopCallback(taskTypeName, task)
	}
	return t.executeLoopResult, t.executeLoopErr
}

func (t *testInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.verifySatisfactionResult, nil
}

func (t *testInvoker) GetCurrentTask() aicommon.AIStatefulTask {
	return t.currentTask
}

func (t *testInvoker) GetCurrentTaskId() string {
	return "test-task-id"
}

func (t *testInvoker) AddToTimeline(entry, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timelineEntries = append(t.timelineEntries, entry+": "+content)
}

func (t *testInvoker) getTimelineString() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var result string
	for _, e := range t.timelineEntries {
		result += e + "\n"
	}
	return result
}

// mockForgeFactory implements aicommon.AIForgeFactory for testing.
type mockForgeFactory struct {
	forges map[string]*schema.AIForge
}

func (m *mockForgeFactory) Query(ctx context.Context, opts ...aicommon.ForgeQueryOption) ([]*schema.AIForge, error) {
	return nil, nil
}

func (m *mockForgeFactory) GetAIForge(name string) (*schema.AIForge, error) {
	if forge, ok := m.forges[name]; ok {
		return forge, nil
	}
	return nil, fmt.Errorf("forge %q not found", name)
}

func (m *mockForgeFactory) GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error) {
	return "", nil
}

func (m *mockForgeFactory) GenerateAIJSONSchemaFromSchemaAIForge(forge *schema.AIForge) (string, error) {
	return "", nil
}

func mustNewTool(name string, opts ...aitool.ToolOption) *aitool.Tool {
	t, err := aitool.New(name, opts...)
	if err != nil {
		panic(fmt.Sprintf("failed to create tool %q: %v", name, err))
	}
	return t
}

func newToolManagerWithTool(tool *aitool.Tool) *buildinaitools.AiToolManager {
	return buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{tool} },
		buildinaitools.WithExtendTools([]*aitool.Tool{tool}, true),
	)
}

func newTestTask(ctx context.Context) *aicommon.AIStatefulTaskBase {
	emitter := aicommon.NewEmitter("test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	return aicommon.NewStatefulTaskBase("test-task", "test user input", ctx, emitter, true)
}

func buildAction(identifier string) *aicommon.Action {
	action, _ := aicommon.ExtractAction(
		fmt.Sprintf(`{"@action": "load_capability", "identifier": "%s"}`, identifier),
		"load_capability",
	)
	return action
}

// --- Verifier Tests ---

func TestLoadCapability_Verifier_EmptyIdentifier(t *testing.T) {
	action := buildAction("")
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	assert.Error(t, err, "verifier should reject empty identifier")
}

func TestLoadCapability_Verifier_ResolvesToTool(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool("test-tool", aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return "ok", nil
	}))
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	action := buildAction("test-tool")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, "test-tool", loop.Get("_load_cap_identifier"))
	assert.Equal(t, string(aicommon.ResolvedAs_Tool), loop.Get("_load_cap_resolved_type"))
}

func TestLoadCapability_Verifier_ResolvesToForge(t *testing.T) {
	ctx := context.Background()
	forgeMgr := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"test-forge": {ForgeName: "test-forge", Description: "A test forge"},
		},
	}
	cfg := &aicommon.Config{AiForgeManager: forgeMgr}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	action := buildAction("test-forge")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, "test-forge", loop.Get("_load_cap_identifier"))
	assert.Equal(t, string(aicommon.ResolvedAs_Forge), loop.Get("_load_cap_resolved_type"))
}

func TestLoadCapability_Verifier_ResolvesToUnknown(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	action := buildAction("nonexistent-thing")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, "nonexistent-thing", loop.Get("_load_cap_identifier"))
	assert.Equal(t, string(aicommon.ResolvedAs_Unknown), loop.Get("_load_cap_resolved_type"))
}

// --- Handler Tests: Tool Branch ---

func TestLoadCapability_Handler_Tool_Success(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool("my-tool", aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return "ok", nil
	}))
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "my-tool")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Tool))

	invoker.toolCallResult = &aitool.ToolResult{Data: "tool result data"}

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("my-tool")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.toolCallCalled, "tool should be called")
	assert.Equal(t, "my-tool", invoker.toolCallName)

	terminated, err := op.IsTerminated()
	assert.True(t, terminated, "should exit on satisfied")
	assert.NoError(t, err)
}

func TestLoadCapability_Handler_Tool_Error(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task
	invoker.toolCallErr = fmt.Errorf("tool execution failed")

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "broken-tool")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Tool))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("broken-tool")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.toolCallCalled)
	assert.True(t, op.IsContinued(), "should continue on tool error to allow retry")
	assert.Contains(t, op.GetFeedback().String(), "execution failed")
}

func TestLoadCapability_Handler_Tool_NilResult(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task
	invoker.toolCallResult = nil

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "nil-result-tool")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Tool))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("nil-result-tool")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.toolCallCalled)
	assert.True(t, op.IsContinued(), "should continue when tool returns nil result")
}

// --- Handler Tests: Forge/Blueprint Branch ---

func TestLoadCapability_Handler_Forge_AsyncMode(t *testing.T) {
	ctx := context.Background()
	forgeMgr := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"my-forge": {ForgeName: "my-forge"},
		},
	}
	cfg := &aicommon.Config{AiForgeManager: forgeMgr}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "my-forge")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Forge))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("my-forge")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.forgeCalled, "forge async execute should be called")
	assert.Equal(t, "my-forge", invoker.forgeCallName)
	assert.True(t, op.IsAsyncModeRequested(), "should request async mode for forge")
	assert.NotNil(t, invoker.forgeOnFinish, "forge callback should be set")
}

// --- Handler Tests: Skill Branch ---

func TestLoadCapability_Handler_Skill_NoManager(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "some-skill")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Skill))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("some-skill")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, op.IsContinued(), "should continue when skill manager is nil")
	assert.Contains(t, op.GetFeedback().String(), "skills context manager is not available")
}

// --- Handler Tests: Focus Mode Branch ---

func TestLoadCapability_Handler_FocusMode_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "vuln_verify")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("vuln_verify")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled, "ExecuteLoopTaskIF should be called")
	assert.Equal(t, "vuln_verify", invoker.executeLoopName)
	assert.True(t, op.IsContinued(), "should continue after focus mode completes")
	assert.Contains(t, op.GetFeedback().String(), "SUCCESSFULLY")
}

func TestLoadCapability_Handler_FocusMode_Error(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopErr = fmt.Errorf("focus loop crashed")
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "bad-mode")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("bad-mode")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled)
	assert.True(t, op.IsContinued(), "should continue on focus mode error")
	assert.Contains(t, op.GetFeedback().String(), "FAILED")
}

func TestLoadCapability_Handler_FocusMode_NotOk(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = false
	invoker.executeLoopErr = nil
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "partial-mode")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("partial-mode")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled)
	assert.True(t, op.IsContinued(), "should continue when focus mode returns not-ok")
	assert.Contains(t, op.GetFeedback().String(), "UNSUCCESSFUL")
}

// --- Handler Tests: Unknown -> Intent Fallback ---

func TestLoadCapability_Handler_Unknown_IntentFallback(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	invoker.executeLoopCallback = func(loopName string, task aicommon.AIStatefulTask) (bool, error) {
		assert.Equal(t, schema.AI_REACT_LOOP_NAME_INTENT, loopName, "should invoke intent loop")
		return true, nil
	}
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "totally-unknown-thing")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("totally-unknown-thing")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled, "intent loop should be invoked as fallback")
	assert.Equal(t, schema.AI_REACT_LOOP_NAME_INTENT, invoker.executeLoopName)
	assert.True(t, op.IsContinued(), "should continue after intent fallback")
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "totally-unknown-thing")
	assert.Contains(t, feedback, "was NOT found")
}

func TestLoadCapability_Handler_Unknown_IntentFallback_Error(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopErr = fmt.Errorf("intent loop failed")
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "unknown-thing")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("unknown-thing")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled)
	assert.True(t, op.IsContinued(), "should continue on intent fallback failure")
	assert.Contains(t, op.GetFeedback().String(), "intent recognition FAILED")
}

func TestLoadCapability_Handler_Unknown_IntentFallback_NotOk(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = false
	invoker.executeLoopErr = nil
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "unknown-no-results")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("unknown-no-results")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled)
	assert.True(t, op.IsContinued(), "should continue when intent loop returns not-ok")
	assert.Contains(t, op.GetFeedback().String(), "produced NO useful results")
}

// --- Handler Tests: Empty Identifier ---

func TestLoadCapability_Handler_EmptyIdentifier(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "")

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	terminated, err := op.IsTerminated()
	assert.True(t, terminated, "should fail on empty identifier")
	assert.Error(t, err)
}

// --- Registration Test ---

func TestLoadCapability_IsRegistered(t *testing.T) {
	action, ok := reactloops.GetLoopAction(schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY)
	assert.True(t, ok, "load_capability should be registered in global action registry")
	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY, action.ActionType)
}

// --- Operator Dynamic Async Mode Test ---

func TestOperator_RequestAsyncMode(t *testing.T) {
	task := newTestTask(context.Background())
	op := reactloops.NewActionHandlerOperator(task)

	assert.False(t, op.IsAsyncModeRequested(), "should not be async by default")
	op.RequestAsyncMode()
	assert.True(t, op.IsAsyncModeRequested(), "should be async after RequestAsyncMode")
}

// --- End-to-End Verifier+Handler Flow Tests ---

func TestLoadCapability_E2E_ToolFlow(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool("e2e-tool", aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return "e2e result", nil
	}))
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	invoker := newTestInvoker(ctx)
	invoker.toolCallResult = &aitool.ToolResult{Data: "e2e result"}
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildAction("e2e-tool")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, string(aicommon.ResolvedAs_Tool), loop.Get("_load_cap_resolved_type"),
		"verifier should resolve as tool")

	op := reactloops.NewActionHandlerOperator(task)
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.toolCallCalled, "tool should be executed")
	assert.Equal(t, "e2e-tool", invoker.toolCallName)
	assert.False(t, op.IsAsyncModeRequested(), "tool should not request async mode")
}

func TestLoadCapability_E2E_ForgeFlow(t *testing.T) {
	ctx := context.Background()
	forgeMgr := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"e2e-forge": {ForgeName: "e2e-forge", Description: "E2E test forge"},
		},
	}
	cfg := &aicommon.Config{AiForgeManager: forgeMgr}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildAction("e2e-forge")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, string(aicommon.ResolvedAs_Forge), loop.Get("_load_cap_resolved_type"),
		"verifier should resolve as forge")

	op := reactloops.NewActionHandlerOperator(task)
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.forgeCalled, "forge should be invoked")
	assert.Equal(t, "e2e-forge", invoker.forgeCallName)
	assert.True(t, op.IsAsyncModeRequested(), "forge should request async mode")
}

func TestLoadCapability_E2E_UnknownFallbackFlow(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildAction("e2e-unknown-identifier")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, string(aicommon.ResolvedAs_Unknown), loop.Get("_load_cap_resolved_type"),
		"verifier should resolve as unknown")

	op := reactloops.NewActionHandlerOperator(task)
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled, "intent loop should be invoked")
	assert.Equal(t, schema.AI_REACT_LOOP_NAME_INTENT, invoker.executeLoopName)
	assert.True(t, op.IsContinued(), "should continue after fallback")
	assert.False(t, op.IsAsyncModeRequested(), "unknown fallback should not be async")
}

func TestLoadCapability_E2E_FocusModeFlow(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	testLoopName := "__test_load_cap_focus_mode__"
	_ = reactloops.RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return nil, nil
		},
		reactloops.WithLoopDescription("Test focus mode for load_capability"),
	)

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildAction(testLoopName)
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, string(aicommon.ResolvedAs_FocusedMode), loop.Get("_load_cap_resolved_type"),
		"verifier should resolve as focused mode")

	op := reactloops.NewActionHandlerOperator(task)
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.executeLoopCalled, "focus mode loop should be executed")
	assert.Equal(t, testLoopName, invoker.executeLoopName)
	assert.True(t, op.IsContinued(), "should continue after focus mode")
	assert.False(t, op.IsAsyncModeRequested(), "focus mode should not be async")
}

// --- Verifier: Whitespace-padded Identifier ---

func TestLoadCapability_Verifier_WhitespaceIdentifier(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool("trimmed-tool", aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
		return "ok", nil
	}))
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	action := buildAction("  trimmed-tool  ")
	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)

	assert.Equal(t, "trimmed-tool", loop.Get("_load_cap_identifier"),
		"verifier should trim whitespace from identifier")
}

// --- Verifier: Repeated Attempt Detection ---

func TestLoadCapability_Verifier_RepeatedAttemptTracking(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	action := buildAction("repeated-id")

	err := loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)
	assert.Equal(t, "1", loop.Get("_load_cap_attempt_repeated-id"),
		"first attempt should record count=1")

	err = loopAction_LoadCapability.ActionVerifier(loop, action)
	require.NoError(t, err)
	assert.Equal(t, "2", loop.Get("_load_cap_attempt_repeated-id"),
		"second attempt should increment count to 2")

	assert.Contains(t, invoker.getTimelineString(),
		"REPEATED_ATTEMPT",
		"timeline should warn about repeated attempt")
}

// --- Handler Tests: Forge Rejected When Already Async ---

func TestLoadCapability_Handler_Forge_RejectedWhenTaskAlreadyAsync(t *testing.T) {
	ctx := context.Background()
	forgeMgr := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"my-forge": {ForgeName: "my-forge"},
		},
	}
	cfg := &aicommon.Config{AiForgeManager: forgeMgr}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	task.SetAsyncMode(true)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "my-forge")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Forge))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("my-forge")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.False(t, invoker.forgeCalled, "forge should NOT be called when task is already async")
	assert.False(t, op.IsAsyncModeRequested(), "should NOT request async mode")
	assert.True(t, op.IsContinued(), "should continue with rejection")
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "REJECTED")
	assert.Contains(t, feedback, "already running in async mode")
	assert.Contains(t, invoker.getTimelineString(),
		"FORGE_REJECTED",
		"timeline should record forge rejection")
}

func TestLoadCapability_Handler_Forge_AllowedWhenTaskNotAsync(t *testing.T) {
	ctx := context.Background()
	forgeMgr := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"my-forge": {ForgeName: "my-forge"},
		},
	}
	cfg := &aicommon.Config{AiForgeManager: forgeMgr}
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "my-forge")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Forge))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("my-forge")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	assert.True(t, invoker.forgeCalled, "forge should be called when task is NOT async")
	assert.True(t, op.IsAsyncModeRequested(), "should request async mode")
}

// --- Handler Tests: Focus Mode Timeline Feedback ---

func TestLoadCapability_Handler_FocusMode_SuccessTimelineFeedback(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "success-mode")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("success-mode")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "SUCCESSFULLY")
	assert.Contains(t, invoker.getTimelineString(),
		"FOCUS_MODE_DONE",
		"timeline should record success")
}

func TestLoadCapability_Handler_FocusMode_ErrorTimelineFeedback(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopErr = fmt.Errorf("loop crashed")
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "crash-mode")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("crash-mode")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "FAILED")
	assert.Contains(t, feedback, "Do NOT retry")
	assert.Contains(t, invoker.getTimelineString(),
		"FOCUS_MODE_FAILED",
		"timeline should record failure with details")
}

func TestLoadCapability_Handler_FocusMode_UnsuccessfulTimelineFeedback(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = false
	invoker.executeLoopErr = nil
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "unsat-mode")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_FocusedMode))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("unsat-mode")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "UNSUCCESSFUL")
	assert.Contains(t, feedback, "Do NOT retry")
	assert.Contains(t, invoker.getTimelineString(),
		"FOCUS_MODE_UNSUCCESSFUL",
		"timeline should record unsuccessful outcome")
}

// --- Handler Tests: Unknown Blocked on Repeat ---

func TestLoadCapability_Handler_Unknown_BlockedOnRepeat(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "repeat-unknown")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))

	op1 := reactloops.NewActionHandlerOperator(task)
	action := buildAction("repeat-unknown")
	loopAction_LoadCapability.ActionHandler(loop, action, op1)

	assert.True(t, invoker.executeLoopCalled,
		"first attempt should trigger intent recognition")

	invoker.mu.Lock()
	invoker.executeLoopCalled = false
	invoker.mu.Unlock()

	loop.Set("_load_cap_identifier", "repeat-unknown")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))
	op2 := reactloops.NewActionHandlerOperator(task)
	loopAction_LoadCapability.ActionHandler(loop, action, op2)

	assert.False(t, invoker.executeLoopCalled,
		"second attempt with same identifier should be BLOCKED without intent recognition")
	feedback := op2.GetFeedback().String()
	assert.Contains(t, feedback, "BLOCKED")
	assert.Contains(t, feedback, "already been tried")
	assert.Contains(t, feedback, "Do NOT call load_capability")
	assert.Contains(t, invoker.getTimelineString(),
		"UNKNOWN_BLOCKED",
		"timeline should record blocked attempt")
}

// --- Handler Tests: Unknown Timeline Pressure ---

func TestLoadCapability_Handler_Unknown_TimelinePressure(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	invoker := newTestInvoker(ctx)
	invoker.executeLoopResult = true
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	loop.Set("_load_cap_identifier", "some-unknown")
	loop.Set("_load_cap_resolved_type", string(aicommon.ResolvedAs_Unknown))

	op := reactloops.NewActionHandlerOperator(task)
	action := buildAction("some-unknown")
	loopAction_LoadCapability.ActionHandler(loop, action, op)

	timeline := invoker.getTimelineString()
	assert.Contains(t, timeline, "Do NOT call load_capability",
		"timeline should explicitly warn against retrying")
	assert.Contains(t, timeline, "some-unknown",
		"timeline should reference the identifier by name")
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "was NOT found",
		"feedback should clearly state the identifier was not found")
	assert.Contains(t, feedback, "Do NOT retry",
		"feedback should discourage retrying")
}
