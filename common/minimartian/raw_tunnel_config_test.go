package minimartian

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type rawTunnelTimeoutError struct{}

func (rawTunnelTimeoutError) Error() string   { return "test timeout" }
func (rawTunnelTimeoutError) Timeout() bool   { return true }
func (rawTunnelTimeoutError) Temporary() bool { return true }

// Unknown tunnels are still MITM upstream connections. Although their payload
// stays opaque, their TCP dial must inherit the same connect timeout, DNS/hosts
// mapping, custom dialer, retry, downstream proxy and system-proxy policy as a
// normal HTTP request made by the same Proxy.
func TestProxy_DialRawTunnel_InheritsLowhttpTransportConfig(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	const targetHost = "raw-tunnel-config.invalid"
	const connectTimeout = 137 * time.Millisecond
	_, targetPort, err := utils.ParseStringToHostPort(listener.Addr().String())
	require.NoError(t, err)

	type dialCall struct {
		timeout time.Duration
		target  string
	}
	var (
		mu    sync.Mutex
		calls []dialCall
	)
	capturedDialer := func(timeout time.Duration, target string) (net.Conn, error) {
		mu.Lock()
		calls = append(calls, dialCall{timeout: timeout, target: target})
		mu.Unlock()
		return net.DialTimeout("tcp", target, timeout)
	}

	p := NewProxy()
	p.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithConnectTimeout(connectTimeout),
		lowhttp.WithETCHosts(map[string]string{targetHost: "127.0.0.1"}),
		// Proxy.SetDialer has the same later precedence in the ordinary HTTP path.
		lowhttp.WithDialer(func(time.Duration, string) (net.Conn, error) {
			return nil, errors.New("lowhttp dialer should have been overridden")
		}),
	})
	p.SetDialer(capturedDialer)

	conn, err := p.dialRawTunnel(
		context.Background(),
		utils.HostPort(targetHost, targetPort),
		targetHost,
		"",
	)
	require.NoError(t, err)
	defer conn.Close()

	select {
	case serverConn := <-accepted:
		defer serverConn.Close()
	case <-time.After(time.Second):
		t.Fatal("raw tunnel did not reach the host-mapped upstream")
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []dialCall{{
		timeout: connectTimeout,
		target:  utils.HostPort("127.0.0.1", targetPort),
	}}, calls)
}

func TestProxy_DialRawTunnel_HonorsContextCancellation(t *testing.T) {
	releaseDialer := make(chan struct{})
	p := NewProxy()
	p.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithETCHosts(map[string]string{"cancel.invalid": "127.0.0.1"}),
	})
	p.SetDialer(func(time.Duration, string) (net.Conn, error) {
		<-releaseDialer
		return nil, errors.New("dialer released")
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := p.dialRawTunnel(ctx, "cancel.invalid:1", "cancel.invalid", "")
		result <- err
	}()

	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("raw tunnel dial did not stop when its MITM context was canceled")
	}
	close(releaseDialer)
}

func TestProxy_DialRawTunnel_InheritsRetryPolicy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	var calls int
	p := NewProxy()
	p.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithRetryTimes(1),
		lowhttp.WithRetryWaitTime(time.Millisecond),
		lowhttp.WithRetryMaxWaitTime(time.Millisecond),
	})
	p.SetDialer(func(timeout time.Duration, target string) (net.Conn, error) {
		calls++
		if calls == 1 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: rawTunnelTimeoutError{}}
		}
		return net.DialTimeout("tcp", target, timeout)
	})

	conn, err := p.dialRawTunnel(context.Background(), listener.Addr().String(), "127.0.0.1", "")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Equal(t, 2, calls, "raw tunnel should use the retry policy shared with ordinary HTTP dials")
}

func TestProxy_UpstreamTransportConfig_SystemProxyOverride(t *testing.T) {
	defaultConfig := NewProxy().upstreamTransportConfig()
	require.False(t, defaultConfig.OverrideEnableSystemProxyFromEnv,
		"MITM should preserve netx's process-wide system-proxy setting by default")

	p := NewProxy()
	p.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithEnableSystemProxyFromEnv(true),
	})

	config := p.upstreamTransportConfig()
	require.True(t, config.OverrideEnableSystemProxyFromEnv)
	require.True(t, config.EnableSystemProxyFromEnv)

	p.SetDisableSystemProxy(true)
	config = p.upstreamTransportConfig()
	require.True(t, config.OverrideEnableSystemProxyFromEnv)
	require.False(t, config.EnableSystemProxyFromEnv)
}
