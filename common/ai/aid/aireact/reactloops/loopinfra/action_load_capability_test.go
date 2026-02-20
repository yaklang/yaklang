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
	assert.Contains(t, op.GetFeedback().String(), "completed successfully")
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
	assert.Contains(t, op.GetFeedback().String(), "execution failed")
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
	assert.Contains(t, op.GetFeedback().String(), "returned unsuccessful")
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
	assert.Contains(t, feedback, "was not found")
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
	assert.Contains(t, op.GetFeedback().String(), "intent recognition failed")
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
	assert.Contains(t, op.GetFeedback().String(), "produced no results")
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
