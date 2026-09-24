package netx

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSInspect_DialFailureSkipsSecondHandshake(t *testing.T) {
	var dials atomic.Int32
	orig := inspectDialTCP
	inspectDialTCP = func(timeout time.Duration, addr string, proxies ...string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("connection refused")
	}
	t.Cleanup(func() { inspectDialTCP = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results, err := TLSInspectContext(ctx, "127.0.0.1:1")
	require.Error(t, err)
	require.Empty(t, results)
	require.Equal(t, int32(1), dials.Load())
}

func TestTLSInspect_HandshakeFailureStillTriesNextMode(t *testing.T) {
	var dials atomic.Int32
	orig := inspectDialTCP
	inspectDialTCP = func(timeout time.Duration, addr string, proxies ...string) (net.Conn, error) {
		dials.Add(1)
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			buf := make([]byte, 8)
			_, _ = server.Read(buf)
		}()
		return client, nil
	}
	t.Cleanup(func() { inspectDialTCP = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := TLSInspectContext(ctx, "127.0.0.1:443")
	require.NoError(t, err)
	require.Empty(t, results)
	require.Equal(t, int32(2), dials.Load())
}
