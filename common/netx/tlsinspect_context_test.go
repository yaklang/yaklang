package netx

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSInspectContextPassesCancellationToDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	previous := inspectDialTCP
	t.Cleanup(func() { inspectDialTCP = previous })
	attempts := 0
	inspectDialTCP = func(dialCtx context.Context, _ time.Duration, _ string) (net.Conn, error) {
		attempts++
		cancel()
		if err := dialCtx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("dial context did not inherit cancellation")
	}
	_, err := TLSInspectContext(ctx, "tls-inspect-cancel.invalid:443")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestTLSInspectContextStopsAfterCanceledHandshake(t *testing.T) {
	client, peer := net.Pipe()
	defer client.Close()
	defer peer.Close()
	previous := inspectDialTCP
	attempts := 0
	inspectDialTCP = func(_ context.Context, _ time.Duration, _ string) (net.Conn, error) {
		attempts++
		if attempts > 1 {
			return nil, errors.New("unexpected second dial after cancellation")
		}
		return client, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var results []*TLSInspectResult
	var inspectErr error
	go func() {
		defer close(done)
		results, inspectErr = TLSInspectContext(ctx, "tls-inspect-cancel.invalid:443")
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("TLS fixture did not finish")
		}
		inspectDialTCP = previous
	})
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := peer.Read(make([]byte, 1))
	require.NoError(t, err, "fixture should reach the TLS handshake")
	start := time.Now()
	cancel()
	select {
	case <-done:
		require.NoError(t, inspectErr)
		require.NotNil(t, results)
		require.Empty(t, results)
	case <-time.After(time.Second):
		t.Fatal("TLS inspection kept waiting after cancellation")
	}
	require.Equal(t, 1, attempts)
	t.Logf("TLS inspection returned %s after cancellation", time.Since(start))
}
