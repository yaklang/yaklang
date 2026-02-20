package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// TestReActLoop_LoadCapability_ToolEquivalence verifies that using load_capability
// with a tool identifier produces the same end-to-end tool execution flow as
// require_tool. This is a near-exact copy of TestReActLoop_MultipleIterations
// with the only difference being the action type (load_capability vs require_tool).
func TestReActLoop_LoadCapability_ToolEquivalence(t *testing.T) {
	iterationCount := 0

	toolName := "sleep"

	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
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
		aicommon.WithTools(sleepTool),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if utils.MatchAllOfSubString(prompt, "directly_answer", "load_capability") {
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
{"@action": "load_capability", "identifier": "` + toolName + `",
"human_readable_thought": "mocked thought for tool calling via load_capability"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.01 }}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
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

	err = loop.Execute("load-cap-equiv-task", context.Background(), "test load_capability tool equivalence")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if iterationCount < 3 {
		t.Errorf("Expected at least 3 iterations, got %d", iterationCount)
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
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "directly_answer", "load_capability") {
				// THE KEY DIFFERENCE: use load_capability instead of require_tool
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "load_capability", "identifier": "` + toolName + `",
"human_readable_thought": "mocked thought for tool calling via load_capability"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				callCount++
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.01 }}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
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

	err = loop.Execute("load-cap-max-iter-task", context.Background(), "test load_capability max iterations")

	if callCount != maxIter {
		t.Errorf("Expected exactly %d tool calls (same as require_tool), got %d", maxIter, callCount)
	}

	t.Logf("load_capability max iterations: stopped after %d tool calls (max: %d)", callCount, maxIter)
}
