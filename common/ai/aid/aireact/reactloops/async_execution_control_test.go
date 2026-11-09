package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

type asyncReviewTimelineInvoker struct {
	*mock.MockInvoker
	timeline *aicommon.Timeline
}

func (i *asyncReviewTimelineInvoker) AddToTimeline(kind, content string) {
	i.timeline.PushText(i.GetConfig().AcquireId(), "["+kind+"]:\n"+content)
}

func TestAsyncDirectAnswerSignalWritesTimelineOnce(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithWorkdir(t.TempDir()),
	)
	invoker := &asyncReviewTimelineInvoker{
		MockInvoker: mock.NewMockInvoker(ctx),
		timeline:    cfg.Timeline,
	}
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	defer loop.Release()
	loop.SetCurrentTask(mock.NewMockStatefulTask(ctx, "owner", "review control"))

	loop.RequestAsyncDirectAnswer()
	loop.RequestAsyncDirectAnswer()
	_, err := loop.GetAsyncExecutionManager()
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed, "review stop must close async admission before manager creation")

	beforeItems := timelineFromConfig(cfg).GetIdToTimelineItem().Len()
	require.True(t, loop.handleAsyncDirectAnswerSignal())
	require.False(t, loop.handleAsyncDirectAnswerSignal(), "one review request must only be consumed once")

	timeline := timelineFromConfig(cfg)
	require.NotNil(t, timeline)
	require.Equal(t, beforeItems+1, timeline.GetIdToTimelineItem().Len(), "duplicate signals must not append duplicate Timeline entries")
	serialized, err := aicommon.MarshalTimeline(timeline)
	require.NoError(t, err)
	require.Contains(t, serialized, "用户在后台工具调用审批中要求直接回答")
	require.Contains(t, serialized, "下一步使用 directly_answer")

	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err, "acknowledging direct answer must reopen session admission")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		require.NoError(t, manager.Shutdown(cleanupCtx))
	})
}
