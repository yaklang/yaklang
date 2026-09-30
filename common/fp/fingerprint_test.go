package fp

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise the real probe transport and matcher against owned local services.
// Completion is determined by Match returning, rather than a seven-second sleep.
func TestNewFingerprintMatcher(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		for {
			n, addr, err := listener.ReadFrom(buf)
			if err != nil {
				return
			}
			if string(buf[:n]) == "probe" {
				_, _ = listener.WriteTo([]byte("fixture-banner"), addr)
			}
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	assertLocalProbeMatch(t, UDP, listener.LocalAddr().(*net.UDPAddr).Port)
}

func TestNewFingerprintMatcher1(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			buf := make([]byte, 5)
			_, err = io.ReadFull(conn, buf)
			if err == nil && string(buf) == "probe" {
				_, _ = conn.Write([]byte("fixture-banner"))
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	assertLocalProbeMatch(t, TCP, listener.Addr().(*net.TCPAddr).Port)
}

func assertLocalProbeMatch(t *testing.T, proto TransportProto, port int) {
	t.Helper()
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(fmt.Sprintf(`Probe %s LocalFixture q|probe|
rarity 1
ports %d
match local-fixture m|^fixture-banner$|
`, proto, port)))
	require.NoError(t, err)
	matcher, err := NewDefaultFingerprintMatcher(NewConfig(
		WithTransportProtos(proto), WithProbesMax(3), WithNmapRule(rules),
		WithProbeTimeout(300*time.Millisecond), WithActiveMode(true), WithDisableWebFingerprint(true),
	))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := matcher.MatchWithContext(ctx, "127.0.0.1", port)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OPEN, result.State)
	require.Contains(t, result.GetServiceName(), "local-fixture")
	require.Contains(t, result.Fingerprint.Banner, "fixture-banner")
}

func TestNewFingerprintMatcher_TCP_No_http(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })

	matcher, err := NewDefaultFingerprintMatcher(
		NewConfig(WithTransportProtos(TCP), WithProbesMax(3),
			WithProbeTimeout(150*time.Millisecond), WithActiveMode(true),
			WithDebugLog(true),
		))
	require.NoError(t, err)
	result, err := matcher.Match("127.0.0.1", port)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OPEN, result.State)
}
