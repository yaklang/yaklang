package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecisionBoundaryDoesNotSwallowUserInputOrUnreviewedResult(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	a := nextStart(t, h)
	nextStart(t, h)
	c.Wake()
	c.taskDiscovered(a.Task.ID, a.ID, "discovery")
	close(h.gates[a.Task.ID])
	a = settled(t, c, a.Task.ID)
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.completeDecision(through, &decisionBoundary{considered: true, yield: true})
	s := c.Snapshot()
	require.Zero(t, s.CheckedThrough, "unresolved first user message retains the prefix gap")
	require.Len(t, s.Inbox, 2, "wait only considers the ordinary discovery")
	require.Equal(t, AwaitingReview, s.Attempts[a.Task.ID].State)
	require.False(t, c.ReportReady())
	require.NoError(t, c.ReviewTask(a.Task.ID, a.ID, "accept", "discovery checked"))
	c.Wake() // arrives during this decision, never included in its acknowledgement
	c.completeDecision(through, &decisionBoundary{considered: true, handledUser: true})
	s = c.Snapshot()
	require.Equal(t, through, s.CheckedThrough)
	require.Len(t, s.Inbox, 2, "own acceptance plus new user input await their next prompt")
	require.Equal(t, "user_message", s.Inbox[1].Type)
}

func TestDecisionBoundaryRejectPreservesPendingMessages(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	a := nextStart(t, h)
	nextStart(t, h)
	c.taskDiscovered(a.Task.ID, a.ID, "discovery")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.completeDecision(through, &decisionBoundary{considered: true, rejected: true})
	require.Len(t, c.Snapshot().Inbox, 1)
	require.Zero(t, c.Snapshot().CheckedThrough)
}

func TestOrdinaryNotificationsHaveFixedDeadlineAndUrgentBypass(t *testing.T) {
	for _, urgent := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed_deadline", true: "user_bypass"}[urgent], func(t *testing.T) {
			c, h := executionFixture(t, false)
			c.messageBatchDelay = 120 * time.Millisecond
			require.NoError(t, c.SubmitPlan(context.Background()))
			a := nextStart(t, h)
			nextStart(t, h)
			c.taskDiscovered(a.Task.ID, a.ID, "first")
			c.mu.Lock()
			deadline := c.messageBatchDue
			c.mu.Unlock()
			done := make(chan error, 1)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			go func() { done <- c.WaitMessages(ctx, time.Second, nil) }()
			select {
			case <-done:
				t.Fatal("ordinary discovery did not enter aggregation wait")
			case <-time.After(20 * time.Millisecond):
			}
			c.taskDiscovered(a.Task.ID, a.ID, "second")
			c.mu.Lock()
			secondDeadline := c.messageBatchDue
			c.mu.Unlock()
			require.Equal(t, deadline, secondDeadline, "new arrivals cannot move the deadline")
			if urgent {
				c.Wake()
			}
			require.NoError(t, <-done)
			if !urgent {
				require.False(t, time.Now().Before(deadline), "ordinary batch released early")
			}
			_, err := c.deliverMessages()
			require.NoError(t, err)
			require.Len(t, c.Snapshot().Inbox, map[bool]int{false: 2, true: 3}[urgent])
		})
	}
}

func TestExplicitWaitIgnoresAlreadyDeliveredInformationalBatch(t *testing.T) {
	c, h := executionFixture(t, true)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	c.mu.Lock()
	c.enqueueLocked("task_reviewed", "a", 1, "user accepted", nil, false)
	c.mu.Unlock()
	c.publish()
	_, err := c.deliverMessages()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.WaitMessages(ctx, time.Millisecond, nil), context.DeadlineExceeded)
}
