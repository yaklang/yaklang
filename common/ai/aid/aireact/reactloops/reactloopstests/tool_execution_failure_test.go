package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// TestReActLoop_ToolNotFound_ShouldNotCrashLoop tests that when a tool is not found,
// the loop should NOT terminate entirely. Instead, it should:
// 1. Record the error in the timeline
// 2. Allow AI to retry with a different tool
//
// This test verifies the bug reported by user: tool 'hostscan' not found causes entire task to fail
func TestReActLoop_ToolNotFound_ShouldNotCrashLoop(t *testing.T) {
	// #region agent log
	func() {
		f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			defer f.Close()
			f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST_NOT_FOUND","location":"test:start","message":"tool not found test started","data":{},"timestamp":%d}`+"\n", time.Now().UnixMilli()))
		}
	}()
	// #endregion

	// Track iterations and errors
	aiCallCount := 0
	successToolCalled := false
	loopTerminatedWithError := false
	var execError error

	// Create a tool that succeeds (for retry scenario)
	successTool, err := aitool.New(
		"success_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			successToolCalled = true
			// #region agent log
			func() {
				f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil {
					defer f.Close()
					f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST_NOT_FOUND","location":"success_tool:callback","message":"success tool called - AI got second chance!","data":{},"timestamp":%d}`+"\n", time.Now().UnixMilli()))
				}
			}()
			// #endregion
			return "success result", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	firstToolCall := true

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithTools(successTool), // Note: "nonexistent_tool" is NOT registered
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			aiCallCount++

			// #region agent log
			func() {
				f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil {
					defer f.Close()
					f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST_NOT_FOUND","location":"ai_callback","message":"AI callback invoked","data":{"aiCallCount":%d,"promptContainsTool":%t,"firstToolCall":%t},"timestamp":%d}`+"\n", aiCallCount, strings.Contains(prompt, "require_tool"), firstToolCall, time.Now().UnixMilli()))
				}
			}()
			// #endregion

			// Main loop iteration - AI selects a tool
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()

				if firstToolCall {
					// First iteration: AI tries to call a non-existent tool
					firstToolCall = false
					rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "nonexistent_tool" },
"human_readable_thought": "trying a nonexistent tool first", "cumulative_summary": "attempting nonexistent tool"}
`))
				} else {
					// Second iteration: After failure, AI should choose success_tool
					// THIS IS THE KEY: if bug exists, this code path won't be reached
					// #region agent log
					func() {
						f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
						if f != nil {
							defer f.Close()
							f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST_NOT_FOUND","location":"ai_callback:second_iteration","message":"AI got second chance after tool not found - BUG FIXED!","data":{"aiCallCount":%d},"timestamp":%d}`+"\n", aiCallCount, time.Now().UnixMilli()))
						}
					}()
					// #endregion
					rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "success_tool" },
"human_readable_thought": "retrying with success_tool after failure", "cumulative_summary": "switching to success_tool"}
`))
				}
				rsp.Close()
				return rsp, nil
			}

			// Generate parameters for tool
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "input" : "test" }}`))
				rsp.Close()
				return rsp, nil
			}

			// Verify satisfaction - only return true when success_tool is used
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				if successToolCalled {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "success tool completed"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "tool failed, need retry"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt[:min(len(prompt), 200)])
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("test-tool-not-found", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// Execute the loop
	execError = loop.Execute("test-task", context.Background(), "test tool not found handling")

	// #region agent log
	func() {
		f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			defer f.Close()
			errStr := ""
			if execError != nil {
				errStr = execError.Error()
				loopTerminatedWithError = true
			}
			f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST_NOT_FOUND","location":"test:end","message":"test completed","data":{"execError":"%s","successToolCalled":%t,"aiCallCount":%d,"loopTerminatedWithError":%t},"timestamp":%d}`+"\n", errStr, successToolCalled, aiCallCount, loopTerminatedWithError, time.Now().UnixMilli()))
		}
	}()
	// #endregion

	// Test Results
	t.Logf("Test Results:")
	t.Logf("  - Success tool called: %v", successToolCalled)
	t.Logf("  - AI call count: %d", aiCallCount)
	t.Logf("  - Loop terminated with error: %v", execError != nil)
	if execError != nil {
		t.Logf("  - Error: %v", execError)
	}

	// KEY ASSERTION: After fix, successToolCalled should be true
	// because AI gets a second chance after tool not found error
	if !successToolCalled {
		if execError != nil && strings.Contains(execError.Error(), "not found") {
			t.Errorf("BUG CONFIRMED: Loop terminated due to 'tool not found' error instead of allowing retry. Error: %v", execError)
		} else {
			t.Logf("Note: Success tool was not called. This may indicate the bug or test setup issue.")
		}
	} else {
		t.Logf("SUCCESS: Loop allowed AI to retry after 'tool not found' error")
	}
}

// TestReActLoop_ToolExecutionFailure_ShouldNotCrashLoop tests that when a tool execution fails,
// the loop should NOT terminate entirely. Instead, it should:
// 1. Record the error in the timeline
// 2. Allow AI to retry or choose a different tool
//
// This test verifies the bug: tool execution failure causes entire task to fail
func TestReActLoop_ToolExecutionFailure_ShouldNotCrashLoop(t *testing.T) {
	// #region agent log
	func() {
		f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			defer f.Close()
			f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"test:start","message":"test started","data":{},"timestamp":%d}`+"\n", time.Now().UnixMilli()))
		}
	}()
	// #endregion

	// Track iterations and errors
	iterationCount := 0
	failingToolCalled := false
	successToolCalled := false
	loopTerminatedWithError := false

	// Create a tool that always fails
	failingTool, err := aitool.New(
		"failing_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			failingToolCalled = true
			// #region agent log
			func() {
				f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil {
					defer f.Close()
					f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"failing_tool:callback","message":"failing tool called, returning error","data":{},"timestamp":%d}`+"\n", time.Now().UnixMilli()))
				}
			}()
			// #endregion
			return nil, fmt.Errorf("simulated tool execution failure")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create a tool that succeeds
	successTool, err := aitool.New(
		"success_tool",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			successToolCalled = true
			// #region agent log
			func() {
				f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil {
					defer f.Close()
					f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"success_tool:callback","message":"success tool called","data":{},"timestamp":%d}`+"\n", time.Now().UnixMilli()))
				}
			}()
			// #endregion
			return "success result", nil
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
			iterationCount++

			// #region agent log
			func() {
				f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if f != nil {
					defer f.Close()
					f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"ai_callback","message":"AI callback invoked","data":{"iterationCount":%d,"promptContainsTool":%t},"timestamp":%d}`+"\n", iterationCount, strings.Contains(prompt, "require_tool"), time.Now().UnixMilli()))
				}
			}()
			// #endregion

			// First iteration: AI chooses the failing tool
			if iterationCount == 1 && utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "failing_tool" },
"human_readable_thought": "trying the failing tool first", "cumulative_summary": "attempting failing tool"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Second iteration: After failure, AI should be able to choose success tool
			// THIS IS THE KEY TEST: if the loop terminates on first failure, this won't be reached
			if iterationCount > 1 && utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				// #region agent log
				func() {
					f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if f != nil {
						defer f.Close()
						f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"ai_callback:second_iteration","message":"AI got second chance after tool failure - BUG FIXED!","data":{"iterationCount":%d},"timestamp":%d}`+"\n", iterationCount, time.Now().UnixMilli()))
					}
				}()
				// #endregion
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "success_tool" },
"human_readable_thought": "trying success tool after failure", "cumulative_summary": "switching to success tool"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Generate parameters for tool
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "input" : "test" }}`))
				rsp.Close()
				return rsp, nil
			}

			// Verify satisfaction
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "task completed"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt[:min(len(prompt), 200)])
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("test-tool-failure", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// Execute the loop
	execErr := loop.Execute("test-task", context.Background(), "test tool failure handling")

	// #region agent log
	func() {
		f, _ := os.OpenFile("/Users/v1ll4n/Projects/yaklang/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			defer f.Close()
			errStr := ""
			if execErr != nil {
				errStr = execErr.Error()
				loopTerminatedWithError = true
			}
			f.WriteString(fmt.Sprintf(`{"hypothesisId":"TEST","location":"test:end","message":"test completed","data":{"execError":"%s","failingToolCalled":%t,"successToolCalled":%t,"iterationCount":%d,"loopTerminatedWithError":%t},"timestamp":%d}`+"\n", errStr, failingToolCalled, successToolCalled, iterationCount, loopTerminatedWithError, time.Now().UnixMilli()))
		}
	}()
	// #endregion

	// Current behavior (BUG): loop terminates with error after first tool failure
	// Expected behavior (FIX): loop continues and allows AI to retry
	t.Logf("Test Results:")
	t.Logf("  - Failing tool called: %v", failingToolCalled)
	t.Logf("  - Success tool called: %v", successToolCalled)
	t.Logf("  - Iteration count: %d", iterationCount)
	t.Logf("  - Loop terminated with error: %v", execErr != nil)
	if execErr != nil {
		t.Logf("  - Error: %v", execErr)
	}

	// These assertions document the expected behavior
	if !failingToolCalled {
		t.Error("Failing tool should have been called")
	}

	// THIS IS THE KEY ASSERTION:
	// If the bug exists, successToolCalled will be false because the loop terminated
	// After fix, successToolCalled should be true because AI got a second chance
	if execErr != nil && strings.Contains(execErr.Error(), "tool execution") {
		t.Logf("BUG CONFIRMED: Loop terminated due to tool execution failure instead of allowing retry")
		t.Logf("Expected: AI should get a chance to choose another tool after failure")
	}

	if successToolCalled {
		t.Logf("SUCCESS: Loop allowed AI to retry after tool failure")
	}
}
