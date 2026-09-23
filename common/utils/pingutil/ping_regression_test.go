package pingutil

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestPingCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ip := range []string{"127.0.0.1", "::1", "192.0.2.1"} {
		result := PingAuto(ip, WithPingContext(ctx))
		if result.Ok || result.Reason != context.Canceled.Error() {
			t.Fatalf("unexpected result: %+v", result)
		}
		result = PingNativeBase(ip, ctx, time.Second)
		if result.Ok || result.Reason != context.Canceled.Error() {
			t.Fatalf("unexpected native result: %+v", result)
		}
	}
	if _, err := PcapxPing("192.0.2.1", &PingConfig{Ctx: ctx}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestPingLoopback(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "127.10.20.30", "::1"} {
		result := PingAuto(ip)
		if !result.Ok || result.IP != ip {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
}

func TestPingTCPListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	result := PingAuto("127.0.0.1", WithForceTcpPing(), WithDefaultTcpPort(port), WithTimeout(time.Second))
	if !result.Ok {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPingTCPDeadline(t *testing.T) {
	start := time.Now()
	result := PingAuto("192.0.2.1", WithForceTcpPing(), WithTimeout(20*time.Millisecond), WithTcpDialHandler(func(ctx context.Context, _ string, _ ...string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	if result.Ok || time.Since(start) > time.Second {
		t.Fatalf("unexpected result or timeout: %+v", result)
	}
}

func TestPingProxyAndCompatibility(t *testing.T) {
	var calls atomic.Int32
	config := NewPingConfig()
	WithProxies("socks5://127.0.0.1:1080")(config)
	WithPingNativeHandler(func(string, time.Duration) *PingResult { t.Error("proxy must bypass ICMP"); return nil })(config)
	WithTcpDialHandler(func(_ context.Context, _ string, proxies ...string) (net.Conn, error) {
		calls.Add(1)
		if len(proxies) != 1 || proxies[0] != config.proxies[0] {
			t.Error("proxy was not propagated")
		}
		return nil, errors.New("connection refused")
	})(config)
	if result := PingAuto2("127.0.0.1", config); !result.Ok {
		t.Fatalf("unexpected result: %+v", result)
	}
	if calls.Load() == 0 {
		t.Fatal("TCP was not used")
	}
	if !PingAuto2("::1", nil).Ok {
		t.Fatal("nil configuration should use defaults")
	}
}

func TestPingICMPUnavailableFallsBack(t *testing.T) {
	var calls atomic.Int32
	result := PingAuto("192.0.2.1", WithPingNativeHandler(func(string, time.Duration) *PingResult { return nil }), WithTcpDialHandler(func(context.Context, string, ...string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("connection refused")
	}))
	if !result.Ok || calls.Load() == 0 {
		t.Fatalf("missing TCP fallback: %+v", result)
	}
}

func TestPingNoTCPPorts(t *testing.T) {
	result := PingAuto("192.0.2.1", WithForceTcpPing(), WithDefaultTcpPort(""), WithTcpDialHandler(func(context.Context, string, ...string) (net.Conn, error) {
		t.Error("unexpected TCP probe")
		return nil, nil
	}))
	if result.Ok || result.Reason != "no TCP probe ports configured" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
