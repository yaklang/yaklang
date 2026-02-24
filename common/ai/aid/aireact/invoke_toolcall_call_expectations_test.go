package aireact

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const testCallExpectations = "estimated ~2s execution, if timeout force stop and retry. success: returns valid data. failure: adjust params."

func mockedToolCallingWithCallExpectations(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "sleep_test", "params": { "seconds" : 0.1 }, "call_expectations": "` + testCallExpectations + `"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason", "human_readable_result": "mocked thought for verification"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "interval-toolcall-review", "Interval Review") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "interval-toolcall-review", "decision": "continue", "reason": "tool running normally", "progress_summary": "executing", "estimated_remaining_time": "1s"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_ToolUse_CallExpectations_InIntervalReview(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			time.Sleep(200 * time.Millisecond)
			return "done", nil
		}),
	)
	require.NoError(t, err)

	var intervalReviewPromptContainsExpectations bool
	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "Interval Review") {
				if strings.Contains(prompt, testCallExpectations) {
					intervalReviewPromptContainsExpectations = true
				}
			}
			return mockedToolCallingWithCallExpectations(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithToolCallerIntervalReviewDuration(50*time.Millisecond),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test call expectations in interval review",
		}
	}()

	after := time.After(15 * time.Second)
	reviewed := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "timeline_item" {
				content := string(e.Content)
				if strings.Contains(content, "ReAct Iteration Done") {
					break LOOP
				}
			}
		case <-after:
			t.Fatal("timeout waiting for tool execution to complete")
		}
	}

	require.True(t, toolCalled, "tool should be called")
	require.True(t, reviewed, "tool use review should be triggered")
	require.True(t, intervalReviewPromptContainsExpectations,
		"interval review prompt should contain call_expectations content")
}

func TestReAct_ToolUse_CallExpectations_InTimelineVerify(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			return "done", nil
		}),
	)
	require.NoError(t, err)

	var verifyPromptContainsExpectations bool
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
				if strings.Contains(prompt, testCallExpectations) {
					verifyPromptContainsExpectations = true
				}
			}
			return mockedToolCallingWithCallExpectations(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test call expectations in timeline verify",
		}
	}()

	after := time.After(15 * time.Second)
	reviewed := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "timeline_item" {
				content := string(e.Content)
				if strings.Contains(content, "ReAct Iteration Done") {
					break LOOP
				}
			}
		case <-after:
			t.Fatal("timeout waiting for tool execution to complete")
		}
	}

	require.True(t, toolCalled, "tool should be called")
	require.True(t, reviewed, "tool use review should be triggered")

	tl := ins.DumpTimeline()
	require.Contains(t, tl, testCallExpectations,
		"timeline dump should contain call_expectations from ToolResult")

	require.True(t, verifyPromptContainsExpectations,
		"verify-satisfaction prompt should contain call_expectations via timeline")
}

func TestToolResult_String_ContainsCallExpectations(t *testing.T) {
	result := &aitool.ToolResult{
		Name:             "test_tool",
		Param:            map[string]any{"key": "value"},
		CallExpectations: testCallExpectations,
		Data:             &aitool.ToolExecutionResult{Stdout: "output"},
	}

	str := result.String()
	require.Contains(t, str, "call_expectations:", "String() output should contain call_expectations label")
	require.Contains(t, str, testCallExpectations, "String() output should contain the actual expectations text")
}

func TestToolResult_String_OmitsEmptyCallExpectations(t *testing.T) {
	result := &aitool.ToolResult{
		Name:  "test_tool",
		Param: map[string]any{"key": "value"},
		Data:  &aitool.ToolExecutionResult{Stdout: "output"},
	}

	str := result.String()
	require.NotContains(t, str, "call_expectations:", "String() should not contain call_expectations when empty")
}

func TestCallExpectations_InToolCallerPresetParams(t *testing.T) {
	params := aitool.InvokeParams{
		"key":                               "value",
		aicommon.ReservedKeyCallExpectations: testCallExpectations,
	}

	tc := &aicommon.ToolCaller{}
	aicommon.WithToolCaller_CallExpectations("should-be-overridden")(tc)
	require.Equal(t, "should-be-overridden", tc.GetCallExpectations())

	aicommon.WithToolCaller_CallExpectations(testCallExpectations)(tc)
	require.Equal(t, testCallExpectations, tc.GetCallExpectations())

	require.Contains(t, params, aicommon.ReservedKeyCallExpectations,
		"reserved key should exist in params before extraction")
	delete(params, aicommon.ReservedKeyCallExpectations)
	require.NotContains(t, params, aicommon.ReservedKeyCallExpectations,
		"reserved key should be removed from params after extraction")
	require.Contains(t, params, "key", "original params should remain")
}
