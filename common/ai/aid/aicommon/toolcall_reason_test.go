package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestBuildToolCallReasonPrompt_ContainsContext(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"port_scan",
		aitool.WithDescription("Scan a target for open ports."),
		aitool.WithStringParam("target"),
	)

	task := NewStatefulTaskBase("task-1", "scan 10.0.0.1 for open ports", context.Background(), nil, true)
	task.name = "recon"

	prompt := buildToolCallReasonPrompt(tool, aitool.InvokeParams{"target": "10.0.0.1"}, task)

	require.Contains(t, prompt, "Tool: port_scan")
	require.Contains(t, prompt, "Tool description: Scan a target for open ports.")
	require.Contains(t, prompt, "User input: scan 10.0.0.1 for open ports")
	require.Contains(t, prompt, "Params:")
	require.Contains(t, prompt, "target")
	require.Contains(t, prompt, "Output only the reason in the `reason` field.")
	require.Contains(t, prompt, "Match the language of the user input")
	require.Contains(t, prompt, "~20 chars/words")
}

func TestBuildToolCallReasonPrompt_NilTaskAndEmptyParams(t *testing.T) {
	tool := aitool.NewWithoutCallback("noop_tool")

	prompt := buildToolCallReasonPrompt(tool, nil, nil)

	require.Contains(t, prompt, "Tool: noop_tool")
	require.NotContains(t, prompt, "User input:")
	require.NotContains(t, prompt, "Params:")
}

func TestGenerateReasonByLiteForge_NoRuntimeNoop(t *testing.T) {
	tc, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(NewTestConfig(context.Background())),
		WithToolCaller_AICaller(&ProxyAICaller{callFunc: func(request *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}}),
		WithToolCaller_Task(NewStatefulTaskBase("task-1", "abc", context.Background(), nil, true)),
	)
	require.NoError(t, err)
	require.Nil(t, tc.invokeRuntime)

	tool := aitool.NewWithoutCallback("sleep")
	require.Empty(t, tc.generateReasonByLiteForge(context.Background(), tool, nil))
}

func TestGenerateReasonByLiteForge_NilToolNoop(t *testing.T) {
	tc := newToolCallerWithReasonRuntime(t, &reasonTestRuntime{})
	require.Empty(t, tc.generateReasonByLiteForge(context.Background(), nil, nil))
}

func TestGenerateReasonByLiteForge_WithRuntimeReturnsReason(t *testing.T) {
	tc := newToolCallerWithReasonRuntime(t, &reasonTestRuntime{})
	tool := aitool.NewWithoutCallback("sleep", aitool.WithStringParam("seconds"))

	reason := tc.generateReasonByLiteForge(context.Background(), tool, aitool.InvokeParams{"seconds": "0.1"})
	require.Equal(t, "mocked tool-call reason", reason)
}

func TestGenerateReasonByLiteForge_WithRuntimeFailureReturnsEmpty(t *testing.T) {
	tc := newToolCallerWithReasonRuntime(t, &reasonTestRuntime{failSpeedForge: true})
	tool := aitool.NewWithoutCallback("sleep")
	require.Empty(t, tc.generateReasonByLiteForge(context.Background(), tool, nil))
}

func TestWithToolCaller_InvokeRuntime_SetsRuntime(t *testing.T) {
	rt := &reasonTestRuntime{}
	tc := newToolCallerWithReasonRuntime(t, rt)
	require.NotNil(t, tc.invokeRuntime)
	require.Same(t, rt, tc.invokeRuntime)
}

func newToolCallerWithReasonRuntime(t *testing.T, rt AIInvokeRuntime) *ToolCaller {
	t.Helper()
	task := NewStatefulTaskBase("task-1", "user input here", context.Background(), nil, true)
	task.name = "test-task"
	tc, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(NewTestConfig(context.Background())),
		WithToolCaller_AICaller(&ProxyAICaller{callFunc: func(request *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}}),
		WithToolCaller_Task(task),
		WithToolCaller_InvokeRuntime(rt),
	)
	require.NoError(t, err)
	return tc
}
