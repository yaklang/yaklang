package utils

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// TestTriggerWriterImmediateFiresOnFirstWrite verifies that
// NewTriggerWriterImmediate invokes the trigger handler on the first Write
// (without waiting for a size or time threshold), only fires once across
// multiple writes, passes the ResponseTooSlow trigger event, and that the
// data written after the trigger is fully readable from the pipe reader
// handed to the handler.
func TestTriggerWriterImmediateFiresOnFirstWrite(t *testing.T) {
	var (
		mu       sync.Mutex
		fired    int
		gotEvent string
		gotBuf   io.ReadCloser
	)

	tw := NewTriggerWriterImmediate(func(buffer io.ReadCloser, triggerEvent string) {
		mu.Lock()
		fired++
		gotEvent = triggerEvent
		gotBuf = buffer
		mu.Unlock()
	})

	require.Zero(t, tw.GetCount(), "no bytes counted before any write")

	// First write must fire the handler immediately.
	n, err := tw.Write([]byte("data: ready\n\n"))
	require.NoError(t, err)
	require.Equal(t, len("data: ready\n\n"), n)
	require.Equal(t, int64(n), tw.GetCount())

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return fired == 1
	}, time.Second, 5*time.Millisecond, "handler must fire on first write")

	mu.Lock()
	require.Equal(t, httpctx.REQUEST_CONTEXT_KEY_ResponseTooSlow, gotEvent, "immediate trigger must use ResponseTooSlow event")
	require.NotNil(t, gotBuf, "handler must receive the pipe reader")
	mu.Unlock()

	// Subsequent writes must NOT re-trigger the handler (once is idempotent).
	for i := 0; i < 5; i++ {
		_, werr := tw.Write([]byte("data: more\n\n"))
		require.NoError(t, werr)
	}
	mu.Lock()
	require.Equal(t, 1, fired, "handler must fire exactly once")
	mu.Unlock()

	// All written bytes must be fully readable from the pipe reader end that
	// was passed to the handler — this is the invariant the SSE forwarding
	// path relies on (io.TeeReader -> TriggerWriter -> pipe -> IOCopy).
	require.NoError(t, tw.Close())
	mu.Lock()
	buf := gotBuf
	mu.Unlock()
	out, err := io.ReadAll(buf)
	require.NoError(t, err)
	expected := append([]byte("data: ready\n\n"), bytes.Repeat([]byte("data: more\n\n"), 5)...)
	require.Equal(t, expected, out, "pipe reader must deliver all bytes written via TeeReader path")
}
