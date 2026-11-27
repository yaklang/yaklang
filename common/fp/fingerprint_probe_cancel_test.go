package fp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanceledServiceScanKeepsFinishedProbe(t *testing.T) {
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe TCP Finished q|GET /finished HTTP/1.0\r\n\r\n|
rarity 1
match fixture m|inventory fingerprint ready| p/Inventory fixture/
Probe TCP Waiting q|GET /waiting HTTP/1.0\r\n\r\n|
rarity 2
match fixture m|inventory waiting fixture| p/Inventory fixture/
Probe TCP QueuedOne q|GET /queued-one HTTP/1.0\r\n\r\n|
rarity 3
match fixture m|inventory queued fixture| p/Inventory fixture/
Probe TCP QueuedTwo q|GET /queued-two HTTP/1.0\r\n\r\n|
rarity 4
match fixture m|inventory queued fixture| p/Inventory fixture/
`))
	require.NoError(t, err)
	require.Len(t, rules, 4)
	config := NewConfig(WithNmapRule(rules), WithActiveMode(true), WithProbesConcurrentMax(1), WithProbeTimeout(5*time.Second))
	matcher, err := NewDefaultFingerprintMatcher(config)
	require.NoError(t, err)

	waiting := make(chan struct{})
	workerReleased := make(chan struct{})
	var queuedRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/finished":
			_, _ = io.WriteString(w, "inventory fingerprint ready")
		case "/waiting":
			close(waiting)
			<-r.Context().Done()
			close(workerReleased)
		default:
			queuedRequests.Add(1)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addr := server.Listener.Addr().(*net.TCPAddr)
	completed := make(chan *MatchResult, 1)
	go func() {
		result, _ := matcher.matchWithContext(ctx, addr.IP, addr.Port, addr.IP.String(), config)
		completed <- result
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("the second probe never reached the fixture")
	}
	cancel()
	select {
	case result := <-completed:
		require.NotNil(t, result)
		require.True(t, result.IsOpen())
		require.Equal(t, "fixture", result.Fingerprint.ServiceName)
		require.Contains(t, result.GetBanner(), "inventory fingerprint ready")
	case <-time.After(time.Second):
		t.Fatal("probe workers did not finish after cancellation")
	}
	select {
	case <-workerReleased:
	case <-time.After(time.Second):
		t.Fatal("the probe connection was not released")
	}
	require.Zero(t, queuedRequests.Load())
}

func TestProbeCancellationInterruptsWrite(t *testing.T) {
	for _, udp := range []bool{false, true} {
		name := "tcp"
		if udp {
			name = "udp"
		}
		t.Run(name, func(t *testing.T) {
			client, peer := net.Pipe()
			defer peer.Close()
			started := make(chan struct{})
			conn := &protocolResponseConn{Conn: &probeWriteObserver{Conn: client, started: started}, udp: udp}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			config := NewConfig(WithProbeTimeout(5 * time.Second))
			done := make(chan struct{})
			go func() {
				defer close(done)
				exchangeProbe(ctx, conn, "fixture request", config, nil)
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("probe never attempted to write")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("probe stayed in the blocked write after cancellation")
			}
		})
	}
}

type probeWriteObserver struct {
	net.Conn
	started chan struct{}
}

func (c *probeWriteObserver) Write(b []byte) (int, error) {
	close(c.started)
	return c.Conn.Write(b)
}
