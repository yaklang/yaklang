package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func isLoadCapabilityDirectToolPrompt(prompt string) bool {
	return strings.Contains(prompt, "Call exactly one tool using directly_call_tool.")
}

// TestReActLoop_LoadCapability_ToolEquivalence verifies that using load_capability
// with a tool identifier executes the requested tool through a direct proposal. This is a near-exact copy of TestReActLoop_MultipleIterations
// with the only difference being the action type (load_capability vs require_tool).
func TestReActLoop_LoadCapability_ToolEquivalence(t *testing.T) {
	iterationCount := 0
	executionCount := 0

	toolName := "sleep"

	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			executionCount++
			sleepInt := params.GetFloat("seconds", 0.01)
			if sleepInt <= 0 {
				sleepInt = 0.01
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithTools(sleepTool),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if isLoadCapabilityDirectToolPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_call_tool","directly_call_tool_name":"` + toolName + `","directly_call_tool_params":{"seconds":0.01},"directly_call_reason":"run the requested probe"}`))
				rsp.Close()
				return rsp, nil
			}

			if aicommon.IsPrimaryDecisionPrompt(prompt) {
				iterationCount++

				if iterationCount > 3 {
					rsp := i.NewAIResponse()
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
					rsp.Close()
					return rsp, nil
				}

				// THE KEY DIFFERENCE: use load_capability instead of require_tool
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "load_capability", "capability_identifier": "` + toolName + `",
"human_readable_thought": "mocked thought for tool calling via load_capability"}
`))
				rsp.Close()
				return rsp, nil
			}

			if aicommon.IsToolCallReasonLiteForgePrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(aicommon.MockedToolCallReasonActionJSON))
				rsp.Close()
				return rsp, nil
			}

			if aicommon.IsVerifySatisfactionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "abc-mocked-reason"}`))
				rsp.Close()
				return rsp, nil
			}

			fmt.Println("Unexpected prompt:", prompt)
			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	loop, err := reactloops.NewReActLoop("load-cap-equiv-loop", reactIns)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = loop.Execute("load-cap-equiv-task", ctx, "test load_capability tool equivalence")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if iterationCount < 3 {
		t.Errorf("Expected at least 3 iterations, got %d", iterationCount)
	}

	if executionCount != 3 {
		t.Errorf("Expected exactly 3 tool executions, got %d", executionCount)
	}

	t.Logf("load_capability tool equivalence: completed %d iterations (same as require_tool)", iterationCount)
}

// TestReActLoop_LoadCapability_MaxIterationsLimit mirrors TestReActLoop_MaxIterationsLimit
// but uses load_capability instead of require_tool. Verifies that load_capability
// correctly hits the max-iteration limit just like require_tool does.
func TestReActLoop_LoadCapability_MaxIterationsLimit(t *testing.T) {
	callCount := 0

	toolName := "sleep"

	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			callCount++
			sleepInt := params.GetFloat("seconds", 0.01)
			if sleepInt <= 0 {
				sleepInt = 0.01
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithTools(sleepTool),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if isLoadCapabilityDirectToolPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_call_tool","directly_call_tool_name":"` + toolName + `","directly_call_tool_params":{"seconds":0.01},"directly_call_reason":"run the requested probe"}`))
				rsp.Close()
				return rsp, nil
			}
			if aicommon.IsPrimaryDecisionPrompt(prompt) {
				// THE KEY DIFFERENCE: use load_capability instead of require_tool
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "load_capability", "capability_identifier": "` + toolName + `",
"human_readable_thought": "mocked thought for tool calling via load_capability"}
`))
				rsp.Close()
				return rsp, nil
			}

			if aicommon.IsToolCallReasonLiteForgePrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(aicommon.MockedToolCallReasonActionJSON))
				rsp.Close()
				return rsp, nil
			}

			if aicommon.IsVerifySatisfactionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "abc-mocked-reason"}`))
				rsp.Close()
				return rsp, nil
			}

			fmt.Println("Unexpected prompt:", prompt)
			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct: %v", err)
	}

	maxIter := 5
	loop, err := reactloops.NewReActLoop("load-cap-max-iter-loop", reactIns,
		reactloops.WithMaxIterations(maxIter),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = loop.Execute("load-cap-max-iter-task", ctx, "test load_capability max iterations")

	if callCount != maxIter {
		t.Errorf("Expected exactly %d tool executions, got %d", maxIter, callCount)
	}

	t.Logf("load_capability max iterations: stopped after %d tool calls (max: %d)", callCount, maxIter)
}
