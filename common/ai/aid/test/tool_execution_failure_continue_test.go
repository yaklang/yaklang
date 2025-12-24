package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// TestReActLoop_ToolNotFound_ShouldContinue tests that when a tool is not found,
// the loop should NOT terminate entirely. Instead, it should:
// 1. Record the error in the timeline
// 2. Allow AI to retry with a different tool
func TestReActLoop_ToolNotFound_ShouldContinue(t *testing.T) {
	successToolCalled := false
	firstToolCall := true

	// Create a tool that succeeds
	successTool, err := aitool.New(
		"success_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			successToolCalled = true
			return "success result", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithTools(successTool),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			rsp := i.NewAIResponse()

			// Main loop - select tool
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				if firstToolCall {
					firstToolCall = false
					// First: try nonexistent tool
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "nonexistent_tool"}, "human_readable_thought": "trying nonexistent", "cumulative_summary": "test"}`))
				} else {
					// Second: use success_tool
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "success_tool"}, "human_readable_thought": "using success_tool", "cumulative_summary": "retry"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Generate params
			if utils.MatchAllOfSubString(prompt, "generate parameters", "call-tool") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": {"input": "test"}}`))
				rsp.Close()
				return rsp, nil
			}

			// Verify satisfaction - exit when success
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction") {
				if successToolCalled {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done", "human_readable_result": "ok"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "retry", "human_readable_result": "failed"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Self-reflection - skip quickly
			if utils.MatchAllOfSubString(prompt, "SELF_REFLECTION") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "self-reflection", "suggestions": []}`))
				rsp.Close()
				return rsp, nil
			}

			// Default: finish
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	loop, err := reactloops.NewReActLoop("test-not-found", reactIns, reactloops.WithMaxIterations(5))
	if err != nil {
		t.Fatal(err)
	}

	execErr := loop.Execute("test", context.Background(), "test tool not found")

	// Assertions
	if !successToolCalled {
		t.Errorf("FAIL: success_tool was not called - AI did not get retry chance after tool not found")
	}
	if execErr != nil && strings.Contains(execErr.Error(), "not found") {
		t.Errorf("FAIL: Loop terminated due to 'tool not found': %v", execErr)
	}
	t.Logf("SUCCESS: successToolCalled=%v, error=%v", successToolCalled, execErr)
}

// TestReActLoop_ToolExecutionError_ShouldContinue tests that when tool execution fails,
// the loop continues and allows AI to retry
func TestReActLoop_ToolExecutionError_ShouldContinue(t *testing.T) {
	failingToolCalled := false
	successToolCalled := false
	firstToolCall := true

	// Failing tool
	failingTool, err := aitool.New(
		"failing_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			failingToolCalled = true
			return nil, fmt.Errorf("simulated failure")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Success tool
	successTool, err := aitool.New(
		"success_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			successToolCalled = true
			return "success", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithTools(failingTool, successTool),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			rsp := i.NewAIResponse()

			// Main loop - select tool
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				if firstToolCall {
					firstToolCall = false
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "failing_tool"}, "human_readable_thought": "trying failing", "cumulative_summary": "test"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "require_tool", "tool_require_payload": "success_tool"}, "human_readable_thought": "using success", "cumulative_summary": "retry"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Generate params
			if utils.MatchAllOfSubString(prompt, "generate parameters", "call-tool") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": {"input": "test"}}`))
				rsp.Close()
				return rsp, nil
			}

			// Verify satisfaction
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction") {
				if successToolCalled {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done", "human_readable_result": "ok"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "retry", "human_readable_result": "failed"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Self-reflection - skip
			if utils.MatchAllOfSubString(prompt, "SELF_REFLECTION") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "self-reflection", "suggestions": []}`))
				rsp.Close()
				return rsp, nil
			}

			// Default
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	loop, err := reactloops.NewReActLoop("test-exec-fail", reactIns, reactloops.WithMaxIterations(5))
	if err != nil {
		t.Fatal(err)
	}

	execErr := loop.Execute("test", context.Background(), "test tool execution failure")

	// Assertions
	if !failingToolCalled {
		t.Error("FAIL: failing_tool should have been called")
	}
	if !successToolCalled {
		t.Errorf("FAIL: success_tool was not called - AI did not get retry chance after tool failure")
	}
	if execErr != nil && strings.Contains(execErr.Error(), "execution") {
		t.Errorf("FAIL: Loop terminated due to tool execution error: %v", execErr)
	}
	t.Logf("SUCCESS: failingToolCalled=%v, successToolCalled=%v, error=%v", failingToolCalled, successToolCalled, execErr)
}
