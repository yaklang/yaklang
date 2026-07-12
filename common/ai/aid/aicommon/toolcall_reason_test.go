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
	require.Contains(t, prompt, "15-40 words")
	require.Contains(t, prompt, "AT THIS POINT")
	require.Contains(t, prompt, "Avoid generic descriptions")
}

func TestBuildToolCallReasonPrompt_NilTaskAndEmptyParams(t *testing.T) {
	tool := aitool.NewWithoutCallback("noop_tool")

	prompt := buildToolCallReasonPrompt(tool, nil, nil)

	require.Contains(t, prompt, "Tool: noop_tool")
	require.NotContains(t, prompt, "User input:")
	require.NotContains(t, prompt, "Params:")
	require.NotContains(t, prompt, "Recent steps")
}

func TestBuildToolCallReasonPrompt_IncludesRecentSteps(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"do_http_request",
		aitool.WithDescription("Send an HTTP request."),
	)

	task := NewStatefulTaskBase("task-1", "test SQLi on login page", context.Background(), nil, true)
	task.name = "pentest"

	task.PushToolCallResult(&aitool.ToolResult{ID: 1, Name: "bash", Success: true})
	task.PushToolCallResult(&aitool.ToolResult{ID: 2, Name: "do_http_request", Success: true})
	task.PushToolCallResult(&aitool.ToolResult{ID: 3, Name: "grep", Success: false, Error: "no matches found"})

	prompt := buildToolCallReasonPrompt(tool, nil, task)

	require.Contains(t, prompt, "Recent steps")
	require.Contains(t, prompt, "- bash: success")
	require.Contains(t, prompt, "- do_http_request: success")
	require.Contains(t, prompt, "- grep: failed (no matches found)")
}

func TestBuildRecentToolCallSummary_MaxItems(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "test", context.Background(), nil, true)
	for i := 0; i < 8; i++ {
		task.PushToolCallResult(&aitool.ToolResult{ID: int64(i + 1), Name: "tool_" + string(rune('a'+i)), Success: true})
	}

	summary := buildRecentToolCallSummary(task, 3)
	require.NotContains(t, summary, "tool_a")
	require.NotContains(t, summary, "tool_e")
	require.Contains(t, summary, "tool_f")
	require.Contains(t, summary, "tool_g")
	require.Contains(t, summary, "tool_h")
}

func TestBuildRecentToolCallSummary_NilTask(t *testing.T) {
	require.Empty(t, buildRecentToolCallSummary(nil, 5))
}

func TestBuildRecentToolCallSummary_NoResults(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "test", context.Background(), nil, true)
	require.Empty(t, buildRecentToolCallSummary(task, 5))
}

func TestWithToolCaller_Reason_SetsReasonFinalized(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "test", context.Background(), nil, true)
	tc, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(NewTestConfig(context.Background())),
		WithToolCaller_AICaller(&ProxyAICaller{callFunc: func(request *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}}),
		WithToolCaller_Task(task),
		WithToolCaller_Reason("specific reason for this call"),
	)
	require.NoError(t, err)
	require.True(t, tc.reasonFinalized)
	require.Equal(t, "specific reason for this call", tc.reason)
}

func TestWithToolCaller_Reason_EmptyDoesNotFinalize(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "test", context.Background(), nil, true)
	tc, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(NewTestConfig(context.Background())),
		WithToolCaller_AICaller(&ProxyAICaller{callFunc: func(request *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}}),
		WithToolCaller_Task(task),
		WithToolCaller_Reason(""),
	)
	require.NoError(t, err)
	require.False(t, tc.reasonFinalized)
}

func TestBuildRecentToolCallSummary_TruncatesLongError(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "test", context.Background(), nil, true)
	longErr := ""
	for i := 0; i < 100; i++ {
		longErr += "error_text"
	}
	task.PushToolCallResult(&aitool.ToolResult{ID: 1, Name: "fail_tool", Success: false, Error: longErr})

	summary := buildRecentToolCallSummary(task, 5)
	require.Contains(t, summary, "fail_tool: failed")
	require.Contains(t, summary, "...")
	require.Less(t, len(summary), 200)
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
