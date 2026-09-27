package loopinfra

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestNativeToolSchemaLoadTimelineLifecycle(t *testing.T) {
	ctx := context.Background()
	manager, _, _ := newToolBatchTestManager(t)
	cfg := aicommon.NewConfig(ctx)
	cfg.AiToolManager = manager
	cfg.Timeline.SetTimelineBucketByteSize(-1)
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	load := func(payload string) string {
		action := parseToolBatchPromptExample(t, payload, "require_tool")
		require.NoError(t, nativeToolSchemaLoadAction.ActionVerifier(loop, action))
		op := reactloops.NewActionHandlerOperator(newTestTask(ctx))
		nativeToolSchemaLoadAction.ActionHandler(loop, action, op)
		require.True(t, op.IsContinued())
		require.False(t, invoker.toolCallCalled, "loading must not execute or generate parameters")
		return op.GetFeedback().String()
	}
	feedback := load(`{"@action":"require_tool","tool_require_calls":[{"tool_name":"read_file"},{"tool_name":"read_file"},{"tool_name":"grep"},{"tool_name":"missing-probe"}]}`)
	require.Contains(t, feedback, "schema_loaded")
	require.Contains(t, feedback, "Tool unavailable")
	before := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Empty(t, before.PromotedSemiDynamic1)
	require.Equal(t, 2, strings.Count(before.Open, "Direct Params Schema"))
	require.Equal(t, 1, strings.Count(before.Open, "[UPSERT] read_file"))
	load(`{"@action":"require_tool","tool_require_payload":"read_file"}`)
	after := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, after.Open, "[REUSE] read_file")
	require.Equal(t, 2, strings.Count(after.Open, "Direct Params Schema"))
	cfg.Timeline.FreezeAll()
	frozen := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Empty(t, frozen.Open)
	require.Equal(t, 2, strings.Count(frozen.PromotedSemiDynamic1, "Direct Params Schema"))
	require.NotContains(t, frozen.Frozen, "Direct Params Schema")
}

func TestNativeToolSchemaLoadRejectsMalformedRequests(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	for _, payload := range []string{
		`{}`, `{"tool_require_payload":42}`, `{"tool_require_payload":" "}`,
		`{"tool_require_calls":[]}`, `{"tool_require_calls":[null]}`,
		`{"tool_require_payload":"read_file","tool_require_calls":[{"tool_name":"grep"}]}`,
		`{"tool_require_calls":[{"tool_name":"read_file","params":{"file":"a"}}]}`,
	} {
		t.Run(payload, func(t *testing.T) {
			raw := `{"@action":"require_tool",` + payload[1:]
			if payload == "{}" {
				raw = `{"@action":"require_tool"}`
			}
			action := parseToolBatchPromptExample(t, raw, "require_tool")
			require.Error(t, nativeToolSchemaLoadAction.ActionVerifier(loop, action))
		})
	}
}

func TestNativeDirectToolMetadataAndNoParameterFallback(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	batch := parseToolBatchPromptExample(t, `{"@action":"directly_call_tool","directly_call_reason":"inspect files","directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"a"}},{"tool_name":"read_file","params":{"file":"b"},"reason":"specific"}]}`, "directly_call_tool")
	require.NoError(t, nativeDirectToolAction.ActionVerifier(loop, batch))
	request, ok := loop.GetActionExecutionValue(batch, actionStateDirectToolBatch).(*aicommon.ToolBatchRequest)
	require.True(t, ok)
	require.Equal(t, "inspect files", request.Calls[0].Reason)
	require.Equal(t, "specific", request.Calls[1].Reason)
	// The old text path retains its historical validation behavior.
	require.Error(t, loopAction_directlyCallTool.ActionVerifier(loop, batch))
	invalid := parseToolBatchPromptExample(t, `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{}}`, "directly_call_tool")
	err := nativeDirectToolAction.ActionVerifier(loop, invalid)
	require.ErrorContains(t, err, "never generates parameters")
	require.Nil(t, loop.GetActionExecutionValue(invalid, actionStateNativeDirectParams))
}
