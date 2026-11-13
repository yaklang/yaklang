package aicommon

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolCallerCancelledReviewReleasesCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	var requested, released string
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithAgreePolicy(AgreePolicyManual),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			switch event.Type {
			case schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE:
				requested = event.GetInteractiveId()
				stopWorker()
			case schema.EVENT_TYPE_REVIEW_RELEASE:
				released = event.GetInteractiveId()
				require.Equal(t, "", event.GetContentJSONPath("$.params.suggestion"))
			}
		}))
	invoked := false
	tool, err := aitool.New("cancelled_review", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		invoked = true
		return "unexpected", nil
	}))
	require.NoError(t, err)
	caller, err := NewToolCaller(workerCtx, WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_Task(cfg.DefaultTask), WithToolCaller_Emitter(cfg.GetEmitter()))
	require.NoError(t, err)
	_, _, err = caller.CallToolWithExistedParams(tool, aitool.InvokeParams{})
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, ctx.Err(), "the enclosing session must remain alive")
	require.NotEmpty(t, requested)
	require.Equal(t, requested, released)
	require.False(t, invoked)
}
