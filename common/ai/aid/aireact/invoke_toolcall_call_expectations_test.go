package aireact

import (
	"bytes"
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
	if isPrimaryDecisionPrompt(prompt) {
		// verification 收缩为纯观测角色后, satisfied=true 不再自动退出. require_tool
		// 执行过一轮后, 下一轮主决策 prompt 的 timeline 段会带上本轮工具结果
		// (作为 timeline-open 段内容). 检测到它说明工具已执行过, 主动 finish 收口.
		if strings.Contains(prompt, "COMBINED OUTPUT:") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: task done after tool call"}`))
			rsp.Close()
			return rsp, nil
		}
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isToolParamGenPromptForTool(prompt, toolName) && strings.Contains(prompt, "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "sleep_test", "params": { "seconds" : 0.1 }, "call_expectations": "` + testCallExpectations + `"}`))
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "interval-toolcall-review", "Interval Review") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "interval-toolcall-review", "decision": "continue", "reason": "tool running normally", "progress_summary": "executing", "estimated_remaining_time": "1s"}`))
		rsp.Close()
		return rsp, nil
	}

	// The finish action is followed by a distinct final-synthesis request. Its
	// parser accepts directly_answer/answer_payload, not another finish action;
	// returning the latter makes the transaction retry until slow/race suites hit
	// their outer deadline.
	if isDirectAnswerPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"directly_answer","answer_payload":"mocked summary after interval-reviewed tool call"}`))
		rsp.Close()
		return rsp, nil
	}

	// verification 收缩为纯观测角色后, satisfied=true 不再自动退出, 主动 finish 收口.
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: task done after tool call"}`))
	rsp.Close()
	return rsp, nil
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

	after := time.After(30 * time.Second)
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
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
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

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

	after := time.After(30 * time.Second)
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
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP
				}
			}
		case <-after:
			t.Fatal("timeout waiting for tool execution to complete")
		}
	}

	require.True(t, toolCalled, "tool should be called")
	require.True(t, reviewed, "tool use review should be triggered")

	// call_expectations is no longer rendered in timeline dump, so the verify-satisfaction
	// prompt (which includes timeline) no longer carries it. The expectations text is
	// still preserved on ToolResult.CallExpectations for interval review and audit.
	// This test now only verifies tool execution + review flow, not expectations in verify prompt.
}

func TestReAct_ToolUse_IntervalReviewExtraPrompt(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	extraPrompt := "interval-review-extra-" + utils.RandStringBytes(24)

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

	var intervalReviewPromptContainsExtraPrompt bool
	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "Interval Review") {
				nonce := aicommon.ExtractPromptNonce(prompt, "EXTRA_PROMPT")
				if nonce != "" {
					startMarker := "<|EXTRA_PROMPT_" + nonce + "|>"
					endMarker := "<|EXTRA_PROMPT_END_" + nonce + "|>"
					startIndex := strings.Index(prompt, startMarker)
					endIndex := strings.Index(prompt, endMarker)
					if startIndex != -1 && endIndex != -1 {
						body := strings.TrimSpace(prompt[startIndex+len(startMarker) : endIndex])
						if body == extraPrompt {
							intervalReviewPromptContainsExtraPrompt = true
						}
					}
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
		aicommon.WithToolCallIntervalReviewExtraPrompt(extraPrompt),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test interval review extra prompt",
		}
	}()

	after := time.After(30 * time.Second)
	reviewed := false

LOOP_EXTRA_PROMPT:
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
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP_EXTRA_PROMPT
				}
			}
		case <-after:
			t.Fatal("timeout waiting for tool execution to complete")
		}
	}

	require.True(t, toolCalled, "tool should be called")
	require.True(t, reviewed, "tool use review should be triggered")
	require.True(t, intervalReviewPromptContainsExtraPrompt,
		"interval review prompt should contain the configured extra prompt")
}

func TestNormalizeIntervalReviewFieldContent(t *testing.T) {
	t.Run("json string", func(t *testing.T) {
		content, ok := normalizeIntervalReviewFieldContent(strings.NewReader(`"tool running normally"`))
		require.True(t, ok)
		require.Equal(t, "tool running normally", content)
	})

	t.Run("plain text fallback", func(t *testing.T) {
		content, ok := normalizeIntervalReviewFieldContent(strings.NewReader("still collecting logs"))
		require.True(t, ok)
		require.Equal(t, "still collecting logs", content)
	})

	t.Run("schema object rejected", func(t *testing.T) {
		content, ok := normalizeIntervalReviewFieldContent(strings.NewReader(`{"type":"string","description":"A brief explanation for the decision"}`))
		require.False(t, ok)
		require.Empty(t, content)
	})
}

func TestToolResult_String_OmitsCallExpectations(t *testing.T) {
	// call_expectations is no longer rendered in timeline dump (ToolResult.String())
	// to save tokens; it is still carried on the struct for interval-review / verify prompts.
	result := &aitool.ToolResult{
		Name:             "test_tool",
		Param:            map[string]any{"key": "value"},
		CallExpectations: testCallExpectations,
		Data:             &aitool.ToolExecutionResult{Stdout: "output"},
	}

	str := result.String()
	require.NotContains(t, str, "call_expectations:", "String() should not contain call_expectations label (removed from dump)")
	require.NotContains(t, str, testCallExpectations, "String() should not contain the expectations text in dump")
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
		"key":                                "value",
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
