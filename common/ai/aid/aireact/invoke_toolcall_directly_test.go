package aireact

import (
	"bytes"
	"io"
	"sync/atomic"
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

func mockedDirectlyCallTool(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if isPrimaryDecisionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_call_tool", "directly_call_tool_name": "` + toolName + `", "directly_call_identifier": "sleep_briefly", "directly_call_expectations": "~0.1s, instant", "directly_call_tool_params": {"seconds": 0.1} },
"human_readable_thought": "directly calling cached tool", "cumulative_summary": "..directly-call-summary.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "directly-call-satisfied", "human_readable_result": "done via directly_call_tool"}`))
		rsp.Close()
		return rsp, nil
	}

	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "fallback"}`))
	rsp.Close()
	return rsp, nil
}

func mockedDirectlyCallToolLegacyWrapped(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	if isPrimaryDecisionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_call_tool", "directly_call_tool_name": "` + toolName + `", "directly_call_identifier": "sleep_briefly", "directly_call_expectations": "~0.1s, instant", "directly_call_tool_params": {"@action": "call-tool", "tool": "` + toolName + `", "params": {"seconds": 0.1}} },
"human_readable_thought": "directly calling cached tool with wrapped params", "cumulative_summary": "..directly-call-summary.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "directly-call-satisfied", "human_readable_result": "done via directly_call_tool"}`))
		rsp.Close()
		return rsp, nil
	}

	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "fallback"}`))
	rsp.Close()
	return rsp, nil
}

// TestReAct_DirectlyCallTool_Basic verifies that directly_call_tool
// executes the tool without an extra param-generation AI call.
func TestReAct_DirectlyCallTool_Basic(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var toolCallCount int32
	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			atomic.AddInt32(&toolCallCount, 1)
			return "done", nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectlyCallTool(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
	)
	require.NoError(t, err)

	// pre-populate the cache so directly_call_tool is available
	react.config.GetAiToolManager().AddRecentlyUsedTool(sleepTool)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test directly call tool",
		}
	}()

	timeout := time.After(10 * time.Second)
	taskCompleted := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					break LOOP
				}
			}
		case <-timeout:
			break LOOP
		}
	}

	require.True(t, taskCompleted, "task should complete")
	require.Equal(t, int32(1), atomic.LoadInt32(&toolCallCount), "tool should be called exactly once")
}

func TestReAct_DirectlyCallTool_LegacyWrappedParams(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)

	var toolCallCount int32
	var sawParamProgress bool
	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			atomic.AddInt32(&toolCallCount, 1)
			return "done", nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectlyCallToolLegacyWrapped(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
	)
	require.NoError(t, err)

	react.config.GetAiToolManager().AddRecentlyUsedTool(sleepTool)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test directly call tool with legacy wrapped params",
		}
	}()

	timeout := time.After(10 * time.Second)
	taskCompleted := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.NodeId == "directly_call_tool_params" {
				sawParamProgress = true
			}
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					break LOOP
				}
			}
		case <-timeout:
			break LOOP
		}
	}

	require.True(t, taskCompleted, "task should complete")
	require.Equal(t, int32(1), atomic.LoadInt32(&toolCallCount), "tool should be called exactly once")
	require.True(t, sawParamProgress, "should emit directly_call_tool params progress event")
}

// TestReAct_DirectlyCallTool_RequireThenDirect uses require_tool first, then directly_call_tool.
func TestReAct_DirectlyCallTool_RequireThenDirect(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 400)

	var toolCallCount int32
	var aiCallCount int32

	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			atomic.AddInt32(&toolCallCount, 1)
			return "done", nil
		}),
	)
	require.NoError(t, err)

	var verifyCount int32

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			atomic.AddInt32(&aiCallCount, 1)
			prompt := r.GetPrompt()

			if isPrimaryDecisionPrompt(prompt) {
				rsp := i.NewAIResponse()
				if atomic.LoadInt32(&toolCallCount) == 0 {
					rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "sleep_test" },
"human_readable_thought": "first call via require", "cumulative_summary": "..phase1.."}
`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_call_tool", "directly_call_tool_name": "sleep_test", "directly_call_identifier": "sleep_again", "directly_call_expectations": "~0.1s", "directly_call_tool_params": {"seconds": 0.1} },
"human_readable_thought": "second call via directly", "cumulative_summary": "..phase2.."}
`))
				}
				rsp.Close()
				return rsp, nil
			}

			if isToolParamGenerationPrompt(prompt, "sleep_test") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "sleep_test", "params": { "seconds" : 0.1 }}`))
				rsp.Close()
				return rsp, nil
			}

			if isVerifySatisfactionPrompt(prompt) {
				count := atomic.AddInt32(&verifyCount, 1)
				rsp := i.NewAIResponse()
				if count <= 1 {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "need one more call", "human_readable_result": "not done"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "all done", "human_readable_result": "complete"}`))
				}
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "fallback"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test require then direct",
		}
	}()

	timeout := time.After(15 * time.Second)
	taskCompleted := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					break LOOP
				}
			}
		case <-timeout:
			break LOOP
		}
	}

	require.True(t, taskCompleted, "task should complete")
	require.Equal(t, int32(2), atomic.LoadInt32(&toolCallCount), "tool should be called twice (require + direct)")
}

// TestReAct_DirectlyCallTool_PersistentSession verifies that the recently-used tool cache
// is persisted across conversations sharing the same PersistentSessionId.
// Conversation 1: require_tool -> tool is cached and persisted to DB.
// Conversation 2: new ReAct instance restores the cache, then uses directly_call_tool.
func TestReAct_DirectlyCallTool_PersistentSession(t *testing.T) {
	pid := "test-persistent-session-" + utils.RandStringBytes(8)

	var toolCallCount int32
	sleepTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			atomic.AddInt32(&toolCallCount, 1)
			return "done", nil
		}),
	)
	require.NoError(t, err)

	// === Conversation 1: require_tool ===
	in1 := make(chan *ypb.AIInputEvent, 10)
	out1 := make(chan *ypb.AIOutputEvent, 400)

	react1, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in1),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out1 <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithPersistentSessionId(pid),
	)
	require.NoError(t, err)
	require.NotNil(t, react1)

	go func() {
		in1 <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test require tool for persistence",
		}
	}()

	timeout1 := time.After(15 * time.Second)
	task1Completed := false
LOOP1:
	for {
		select {
		case e := <-out1:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in1 <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					task1Completed = true
					break LOOP1
				}
			}
		case <-timeout1:
			break LOOP1
		}
	}
	require.True(t, task1Completed, "conversation 1 task should complete")
	require.True(t, atomic.LoadInt32(&toolCallCount) >= 1, "tool should have been called in conversation 1")

	// Verify tool is in cache after conversation 1
	require.True(t, react1.config.GetAiToolManager().HasRecentlyUsedTools(),
		"conversation 1 should have cached the tool")
	require.True(t, react1.config.GetAiToolManager().IsRecentlyUsedTool("sleep_test"),
		"sleep_test should be in the cache")

	// Wait for timeline save throttle to flush
	time.Sleep(3500 * time.Millisecond)
	close(in1)

	// === Conversation 2: directly_call_tool via restored cache ===
	in2 := make(chan *ypb.AIInputEvent, 10)
	out2 := make(chan *ypb.AIOutputEvent, 400)

	var conv2ToolCallCount int32
	react2, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedDirectlyCallTool(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in2),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case out2 <- e.ToGRPC():
			default:
			}
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithPersistentSessionId(pid),
	)
	require.NoError(t, err)
	require.NotNil(t, react2)

	// Key assertion: the cache should be restored from the persistent session
	require.True(t, react2.config.GetAiToolManager().HasRecentlyUsedTools(),
		"conversation 2 should have restored the tool cache from persistent session")
	require.True(t, react2.config.GetAiToolManager().IsRecentlyUsedTool("sleep_test"),
		"sleep_test should be available in conversation 2's cache")

	go func() {
		in2 <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test directly call tool from restored cache",
		}
	}()

	timeout2 := time.After(10 * time.Second)
	task2Completed := false
LOOP2:
	for {
		select {
		case e := <-out2:
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in2 <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(string(e.Content), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					task2Completed = true
					break LOOP2
				}
			}
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				atomic.AddInt32(&conv2ToolCallCount, 1)
			}
		case <-timeout2:
			break LOOP2
		}
	}
	close(in2)

	require.True(t, task2Completed, "conversation 2 task should complete")
	require.True(t, atomic.LoadInt32(&conv2ToolCallCount) >= 1,
		"tool should have been called via directly_call_tool in conversation 2")
}
