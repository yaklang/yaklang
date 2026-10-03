package coordinator

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutionInboxBatchAndPersistentDeliveryCursor(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	a := nextStart(t, h)
	nextStart(t, h)
	for i := 0; i < 200; i++ {
		c.taskDiscovered(a.Task.ID, a.ID, "distinct-evidence")
	}
	c.Wake()
	close(h.gates[a.Task.ID])
	settled(t, c, a.Task.ID)
	s := c.Snapshot()
	require.GreaterOrEqual(t, len(s.Inbox), 202, "critical discoveries and terminal/user messages are not evicted")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	require.Equal(t, through, c.Snapshot().DeliveredThrough)
	require.Zero(t, c.Snapshot().CheckedThrough, "delivery is not resolution")
	require.Equal(t, AwaitingReview, c.Snapshot().Attempts[a.Task.ID].State)
	before := c.resultConfig.GetSessionEvidenceRendered()
	again, err := c.deliverMessages()
	require.NoError(t, err)
	require.Equal(t, through, again)
	require.Equal(t, before, c.resultConfig.GetSessionEvidenceRendered())
	c.Wake() // arrives while the decision for 'through' is in progress
	c.checkedMessages(through)
	require.Len(t, c.Snapshot().Inbox, 1, "next boundary retains messages arriving during the action")
	require.False(t, c.ReportReady())
}

func TestExecutionReviewDueCoalescesWithoutAutoAcceptance(t *testing.T) {
	c, h := executionFixture(t, false)
	c.mu.Lock()
	c.reviewInterval = time.Hour
	c.mu.Unlock()
	require.NoError(t, c.SubmitPlan(context.Background()))
	a := nextStart(t, h)
	nextStart(t, h)
	c.mu.Lock()
	c.reviewDueLocked(time.Now().Add(2 * time.Hour))
	c.reviewDueLocked(time.Now().Add(4 * time.Hour))
	count := 0
	for _, m := range c.state.Inbox {
		if m.Type == "review_due" {
			count++
		}
	}
	c.mu.Unlock()
	require.Equal(t, 2, count, "one unhandled reminder per running attempt")
	require.Equal(t, Running, c.Snapshot().Attempts[a.Task.ID].State)
	c.taskDiscovered(a.Task.ID, a.ID+999, "stale")
	require.Equal(t, 2, len(c.Snapshot().Inbox), "stale attempt callbacks cannot enqueue")
}

func TestExecutionRestorePreservesDeliveredAndPendingResult(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	a := nextStart(t, h)
	nextStart(t, h)
	close(h.gates[a.Task.ID])
	settled(t, c, a.Task.ID)
	through, err := c.deliverMessages()
	require.NoError(t, err)
	s := c.Snapshot()
	c.Close()
	c.owned.Wait()
	r := New(context.Background(), h, 2)
	defer r.Close()
	require.NoError(t, r.Restore(s))
	require.Equal(t, through, r.Snapshot().DeliveredThrough)
	require.Equal(t, AwaitingReview, r.Snapshot().Attempts[a.Task.ID].State)
	other := "a"
	if a.Task.ID == "a" {
		other = "b"
	}
	require.Equal(t, Failed, r.Snapshot().Attempts[other].State, "lost process ownership requires explicit retry")
	r.EnableExecution(false, 0)
	select {
	case <-h.started:
		t.Fatal("restoring settled/interrupted tasks must not dispatch them again")
	default:
	}
}

func TestExecutionRestoreRejectsCorruptInboxCursors(t *testing.T) {
	c, _ := executionFixture(t, false)
	base := c.Snapshot()
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.DeliveredThrough = 1 },
		func(s *Snapshot) { s.CheckedThrough = 1 },
		func(s *Snapshot) { s.NextMessage = 1; s.Inbox = []Message{{ID: "pending", Sequence: 2}} },
		func(s *Snapshot) {
			s.NextMessage = 1
			s.Inbox = []Message{{ID: "one", Sequence: 1}, {ID: "two", Sequence: 1}}
		},
	} {
		s := clone(base)
		mutate(&s)
		r := New(context.Background(), nil, 1)
		require.Error(t, r.Restore(s))
		r.Close()
	}
}
