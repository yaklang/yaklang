package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_FocusModeLoop_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	gotPrompt := make(chan string, 1)
	gotDequeueFocusMode := make(chan string, 1)

	_, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e == nil || e.NodeId != "react_task_dequeue" || len(e.Content) == 0 {
				return
			}
			var payload map[string]any
			if json.Unmarshal(e.Content, &payload) != nil {
				return
			}
			focusMode := payload["focus_mode"]
			if focusMode == nil {
				return
			}
			focusModeStr, ok := focusMode.(string)
			if !ok || focusModeStr == "" {
				return
			}
			select {
			case gotDequeueFocusMode <- focusModeStr:
			default:
			}
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			select {
			case gotPrompt <- req.GetPrompt():
			default:
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{
"@action": "object",
"next_action": {"type": "directly_answer"},
"answer_payload": "ok",
"human_readable_thought": "done"
}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput:   true,
			FreeInput:     "@__FOCUS__java_decompiler please generate a Python PoC",
			FocusModeLoop: schema.AI_REACT_LOOP_NAME_PYTHON_POC,
		}
		close(in)
	}()

	var prompt string
	var focusMode string
	for prompt == "" || focusMode == "" {
		select {
		case prompt = <-gotPrompt:
		case focusMode = <-gotDequeueFocusMode:
		case <-ctx.Done():
			t.Fatal("timeout waiting for end-to-end focus mode chain")
		}
	}

	require.Contains(t, prompt, "## Python 环境状态")
	require.Equal(t, schema.AI_REACT_LOOP_NAME_PYTHON_POC, focusMode)
}

func TestReAct_SelectLoopForTask_FocusModeOverridesDirective(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reactIns, err := NewTestReAct(
		aicommon.WithContext(parentCtx),
		aicommon.WithFocus(""),
	)
	require.NoError(t, err)
	cancel()

	task := aicommon.NewStatefulTaskBase("t1", "@__FOCUS__java_decompiler hello", context.Background(), reactIns.Emitter)
	task.SetFocusMode(schema.AI_REACT_LOOP_NAME_DEFAULT)

	parsedQuery, focus, _ := reactIns.selectLoopForTask(task)
	require.Equal(t, "hello", parsedQuery)
	require.Equal(t, schema.AI_REACT_LOOP_NAME_DEFAULT, focus)
}
