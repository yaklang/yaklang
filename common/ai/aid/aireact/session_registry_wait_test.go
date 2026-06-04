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
