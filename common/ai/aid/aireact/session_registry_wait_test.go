package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestWaitRunningSession(t *testing.T) {
	sessionID := "wait-running-session-test"
	_, err := WaitRunningSession(sessionID, 50*time.Millisecond)
	require.Error(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	react, err := NewReAct(
		aicommon.WithContext(ctx),
		aicommon.WithPersistentSessionId(sessionID),
	)
	require.NoError(t, err)
	require.NotNil(t, react)

	done := make(chan struct{})
	go func() {
		defer close(done)
		got, waitErr := WaitRunningSession(sessionID, time.Second)
		require.NoError(t, waitErr)
		require.Equal(t, react, got)
	}()

	time.Sleep(300 * time.Millisecond)
	react.config.EventLoopStartHook()
	<-done
}

func TestIsSessionBusyDistinguishesIdleAndActiveReAct(t *testing.T) {
	const sessionID = "scheduled-busy-session-test"
	react, err := NewReAct(aicommon.WithPersistentSessionId(sessionID))
	require.NoError(t, err)
	registerRunningSession(sessionID, react)
	t.Cleanup(func() { unregisterRunningSession(sessionID) })

	require.False(t, IsSessionBusy(sessionID), "an idle registered chat stream is not busy")
	task := aicommon.NewStatefulTaskBase("active-task", "work", nil, nil, true)
	react.setCurrentTask(task)
	require.True(t, IsSessionBusy(sessionID))
	react.setCurrentTask(nil)
	require.False(t, IsSessionBusy(sessionID))
	react.SetCurrentPlanExecutionTask(task)
	require.True(t, IsSessionBusy(sessionID), "a detached plan execution also owns the chat")
	task.SetStatus(aicommon.AITaskState_Completed)
	require.False(t, IsSessionBusy(sessionID))
}

func TestSessionStartReservationIsBusyAndExclusive(t *testing.T) {
	const sessionID = "session-start-reservation"
	release, ok := TryBeginSessionStart(sessionID)
	require.True(t, ok)
	require.True(t, IsSessionStarting(sessionID))
	require.True(t, IsSessionBusy(sessionID))
	_, ok = TryBeginSessionStart(sessionID)
	require.False(t, ok)
	release()
	require.False(t, IsSessionStarting(sessionID))

	secondRelease, ok := TryBeginSessionStart(sessionID)
	require.True(t, ok)
	secondRelease()
}
