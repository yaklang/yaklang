package coordinator

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestExecutionResolvedCancellationMustBeReported(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	require.NoError(t, c.CancelTasks(nil, "用户明确取消剩余目标，不再需要继续访问来源。"))
	settled(t, c, "a")
	settled(t, c, "b")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.checkedMessages(through)
	require.True(t, c.ReportReady())
	_, err = c.CreateReport(context.Background(), "取消报告", "# 取消报告\n记录执行范围。")
	require.NoError(t, err)
	s := c.Snapshot()
	draft, err := c.SubmitReport(s.UserRevision, s.NextMessage)
	require.NoError(t, err)
	data, err := os.ReadFile(draft.Path)
	require.NoError(t, err)
	require.Contains(t, string(data), "系统记录的失败、重试与未完成范围")
	require.Contains(t, string(data), "c / attempt 0: cancelled")
	require.Contains(t, string(data), "d / attempt 0: cancelled")
	require.NoError(t, c.FinalizeReport())
	require.True(t, c.Snapshot().Finished)
}

func TestExecutionUnresolvedFailureBlocksReportActions(t *testing.T) {
	c, h := executionFixture(t, false)
	h.execute = func(context.Context, Attempt) (Result, error) { return Result{}, context.DeadlineExceeded }
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	settled(t, c, "a")
	settled(t, c, "b")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.checkedMessages(through)
	require.False(t, c.ReportReady())
	_, err = c.CreateReport(context.Background(), "fake success", "completed")
	require.Error(t, err)
	require.False(t, c.Snapshot().Finished)
}

type completionMessageHost struct {
	*executionHost
	c *Controller
}

func (h *completionMessageHost) Changed(s Snapshot) {
	if s.Finished {
		// Simulate user ingress while the compatible completion is being emitted.
		// Ingress saves state before waiting for the publication lock.
		h.c.mu.Lock()
		h.c.state.UserRevision++
		h.c.enqueueLocked("user_message", "", 0, "还需要核对附件", nil, true)
		h.c.mu.Unlock()
	}
}

func TestExecutionCompletionPublicationRechecksLateUserMessage(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	require.NoError(t, c.CancelTasks(nil, "明确停止其余目标"))
	settled(t, c, "a")
	settled(t, c, "b")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.checkedMessages(through)
	_, err = c.CreateReport(context.Background(), "真实范围", "# 已明确取消剩余目标")
	require.NoError(t, err)
	s := c.Snapshot()
	_, err = c.SubmitReport(s.UserRevision, s.NextMessage)
	require.NoError(t, err)
	c.host = &completionMessageHost{h, c}
	require.ErrorContains(t, c.FinalizeReport(), "new work")
	require.False(t, c.Snapshot().Finished)
	require.False(t, c.Snapshot().Report.Submitted)
	require.False(t, c.ReportReady())
}
