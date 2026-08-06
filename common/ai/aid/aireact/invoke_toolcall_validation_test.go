package aireact

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestExecuteToolCallInternal_ValidationFailureIsVisibleInEventAndTimeline(t *testing.T) {
	events := make(chan *schema.AiOutputEvent, 64)
	callbackCalled := false
	tool, err := aitool.New(
		"validation_failure_visibility",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam("value", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			callbackCalled = true
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithContext(context.Background()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		aicommon.WithTools(tool),
	)
	require.NoError(t, err)

	result, directlyAnswer, err := react.executeToolCallInternal(
		context.Background(),
		tool.Name,
		aitool.InvokeParams{"value": []any{"invalid"}},
		true,
		aicommon.WithToolCaller_Reason("verify invalid JSON input visibility"),
	)
	require.NoError(t, err, "tool failures should be returned as inspectable ToolResult values")
	require.False(t, directlyAnswer)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.False(t, callbackCalled)
	require.Contains(t, result.Error, "参数验证失败")
	require.Contains(t, result.Error, "修复建议")
	require.Contains(t, result.Error, "请求尚未发送")

	timeline := react.config.Timeline.String()
	require.Contains(t, timeline, "validation_failure_visibility")
	require.Contains(t, timeline, "参数验证失败")
	require.Contains(t, timeline, "修复建议")
	require.Contains(t, timeline, "请求尚未发送")

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event != nil && event.Type == schema.EventType(schema.EVENT_TOOL_CALL_ERROR) {
				content := string(event.Content)
				require.Contains(t, content, "参数验证失败")
				require.Contains(t, content, "修复建议")
				require.Contains(t, content, "请求尚未发送")
				return
			}
		case <-deadline:
			t.Fatal("expected actionable tool_call_error event")
		}
	}
}
