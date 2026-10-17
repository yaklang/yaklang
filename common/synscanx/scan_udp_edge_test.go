package synscanx

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

func TestUDPUnreachableAndSilenceDoNotOpen(t *testing.T) {
	rec := &recordingProber{udp: func(_ context.Context, target string, _ []byte) (netstackvm.UDPSegment, error) {
		switch target {
		case "10.0.0.8:53":
			return netstackvm.UDPSegment{}, netstackvm.ErrUDPUnreachable
		case "10.0.0.8:123":
			return netstackvm.UDPSegment{}, netstackvm.ErrUDPNoResponse
		default:
			t.Errorf("unexpected udp target %s", target)
			return netstackvm.UDPSegment{}, netstackvm.ErrUDPSend
		}
	}}
	opened := 0
	scanner := &Scannerx{
		ctx:      context.Background(),
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 2, waiting: 50 * time.Millisecond},
		OpenPortHandlers: func(net.IP, int) {
			opened++
		},
	}
	ch := make(chan *SynxTarget, 8)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 53, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 123, Mode: UDP}
	ch <- &SynxTarget{Host: "", Port: 53, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 0, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 70000, Mode: UDP}
	close(ch)
	scanner.sendPacket(ch)
	if opened != 0 {
		t.Fatalf("opened %d ports from inconclusive udp probes", opened)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	// Unreachable is final. Silence is sent a second time.
	if len(rec.udpGot) != 3 {
		t.Fatalf("udp probes = %v", rec.udpGot)
	}
}

func TestUDPProbesHaveIndependentDeadlines(t *testing.T) {
	rec := &recordingProber{}
	rec.udp = func(ctx context.Context, target string, _ []byte) (netstackvm.UDPSegment, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("udp probe has no independent deadline")
		}
		if target == "10.0.0.8:7" {
			<-ctx.Done()
			return netstackvm.UDPSegment{}, ctx.Err()
		}
		return netstackvm.UDPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 9}, nil
	}
	var opened atomic.Int32
	scanner := &Scannerx{
		ctx:      context.Background(),
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 2, waiting: 40 * time.Millisecond},
		OpenPortHandlers: func(_ net.IP, port int) {
			if port != 9 {
				t.Errorf("opened %d", port)
			}
			opened.Add(1)
		},
	}
	ch := make(chan *SynxTarget, 2)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 7, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 9, Mode: UDP}
	close(ch)
	start := time.Now()
	scanner.sendPacket(ch)
	if opened.Load() != 1 {
		t.Fatalf("opened = %d", opened.Load())
	}
	if time.Since(start) > time.Second {
		t.Fatalf("a fast udp probe waited for its sibling: %s", time.Since(start))
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.contexts) != 2 || rec.contexts[0] == rec.contexts[1] {
		t.Fatal("udp probes shared a lifetime")
	}
}

func TestUDPWorkersStayInsideConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var current, peak atomic.Int32
	started := make(chan struct{}, 8)
	rec := &recordingProber{udp: func(ctx context.Context, _ string, _ []byte) (netstackvm.UDPSegment, error) {
		n := current.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-ctx.Done()
		current.Add(-1)
		return netstackvm.UDPSegment{}, ctx.Err()
	}}
	scanner := &Scannerx{ctx: ctx, halfOpen: rec, config: &SynxConfig{tcpProbeConcurrency: 1, waiting: time.Second}}
	ch := make(chan *SynxTarget, 6)
	for i := 0; i < 6; i++ {
		ch <- &SynxTarget{Host: "10.0.0.8", Port: 1000 + i, Mode: UDP}
	}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("udp worker did not start")
	}
	select {
	case <-started:
		t.Fatal("udp worker limit exceeded")
	case <-time.After(30 * time.Millisecond):
	}
	if peak.Load() != 1 {
		t.Fatalf("peak udp probes = %d", peak.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not drain udp workers")
	}
}

func TestUDPCancelDoesNotReportALateReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	rec := &recordingProber{udp: func(ctx context.Context, _ string, _ []byte) (netstackvm.UDPSegment, error) {
		close(started)
		<-ctx.Done()
		return netstackvm.UDPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 53}, nil
	}}
	opened := 0
	scanner := &Scannerx{
		ctx:      ctx,
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 1, waiting: time.Second},
		OpenPortHandlers: func(net.IP, int) {
			opened++
		},
	}
	ch := make(chan *SynxTarget, 1)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 53, Mode: UDP}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send did not return")
	}
	if opened != 0 {
		t.Fatal("a canceled udp probe reported an open port")
	}
}

func TestUDPAndTCPResultsStaySeparated(t *testing.T) {
	rec := &recordingProber{
		probe: func(context.Context, string) (netstackvm.TCPSegment, error) {
			return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
		},
		udp: func(context.Context, string, []byte) (netstackvm.UDPSegment, error) {
			return netstackvm.UDPSegment{RemoteIP: net.IPv4(10, 0, 0, 9), RemotePort: 53}, nil
		},
	}
	var mu sync.Mutex
	var opened []string
	scanner := &Scannerx{
		ctx:      context.Background(),
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 2, tcpProbeTimeout: time.Second, waiting: time.Second},
		OpenPortHandlers: func(ip net.IP, port int) {
			mu.Lock()
			opened = append(opened, utils.HostPort(ip.String(), port))
			mu.Unlock()
		},
	}
	ch := make(chan *SynxTarget, 2)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.9", Port: 53, Mode: UDP}
	close(ch)
	scanner.sendPacket(ch)
	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 2 {
		t.Fatalf("opened = %v", opened)
	}
	seen := map[string]bool{}
	for _, addr := range opened {
		seen[addr] = true
	}
	if !seen["10.0.0.8:80"] || !seen["10.0.0.9:53"] {
		t.Fatalf("opened = %v", opened)
	}
}

func TestUDPSubmitDoesNotWaitOnASlowClock(t *testing.T) {
	s := newPlanScanner(t)
	start := time.Now()
	ch, err := s.SubmitTarget("10.0.0.8", "U:1-30")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for range ch {
		n++
	}
	if n != 30 {
		t.Fatalf("submitted %d udp targets", n)
	}
	if time.Since(start) > 150*time.Millisecond {
		t.Fatalf("udp submit took %s", time.Since(start))
	}
}

func TestUDPExcludeMatchesServicePort(t *testing.T) {
	s := newPlanScanner(t)
	s.config.excludePorts = utils.NewPortsFilter("U:53,80")
	ports := s.GetNonExcludedPorts("80,U:53,U:123,443")
	got := map[string]bool{}
	for _, port := range ports {
		proto, service := utils.ParsePortToProtoPort(port)
		got[proto+":"+strconv.Itoa(service)] = true
	}
	if got["tcp:80"] || got["udp:53"] || !got["udp:123"] || !got["tcp:443"] {
		t.Fatalf("remaining targets = %v", got)
	}
}

// listenUDPLocal binds 127.0.0.1:0. respond may return a nil reply to stay silent.
// delay is applied before the reply so a scan deadline can expire first.
func listenUDPLocal(t *testing.T, respond func([]byte) ([]byte, time.Duration)) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if respond == nil {
				continue
			}
			reply, delay := respond(append([]byte(nil), buf[:n]...))
			if delay > 0 {
				timer := time.NewTimer(delay)
				<-timer.C
			}
			if reply == nil {
				continue
			}
			_, _ = conn.WriteToUDP(reply, peer)
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func closedUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func countSynPorts(results []*synscan.SynScanResult) map[int]int {
	got := map[int]int{}
	for _, result := range results {
		got[result.Port]++
	}
	return got
}

func TestLoopbackUDPCombatConditions(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	answered := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte("pong"), 0
	})
	empty := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte{}, 0
	})
	silent := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return nil, 0
	})
	late := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte("late"), 800 * time.Millisecond
	})
	excluded := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte("hidden"), 0
	})
	closed := closedUDPPort(t)
	tcpOpen := listenLocal(t)
	tcpPort := tcpOpen.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	started := time.Now()
	ports := fmt.Sprintf("%d,U:%d,U:%d,U:%d,U:%d,U:%d,U:%d", tcpPort, answered, empty, silent, late, excluded, closed)
	results, err := Scan(ctx, "127.0.0.1", ports,
		WithShuffle(true),
		WithTCPProbeConcurrency(8),
		WithTCPProbeTimeout(3*time.Second),
		WithWaiting(0.25),
		WithExcludePorts(fmt.Sprintf("U:%d", excluded)),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := countSynPorts(collectSynResults(t, results, 12*time.Second))
	elapsed := time.Since(started)
	t.Logf("combat udp scan %s results %v", elapsed, got)
	if elapsed > 8*time.Second {
		t.Fatalf("mixed udp scan took %s", elapsed)
	}
	if got[answered] != 1 || got[empty] != 1 || got[tcpPort] != 1 {
		t.Fatalf("missing open services, results = %v", got)
	}
	for _, port := range []int{silent, late, excluded, closed} {
		if got[port] != 0 {
			t.Fatalf("port %d must stay closed, results = %v", port, got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("unexpected extra results %v", got)
	}
	acceptNone(t, tcpOpen)
}

func TestLoopbackUDPBurstKeepsOpenSet(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	const opens = 6
	const silentN = 6
	openPorts := map[int]bool{}
	var spec []string
	for i := 0; i < opens; i++ {
		port := listenUDPLocal(t, func(payload []byte) ([]byte, time.Duration) {
			return append([]byte{0x10}, payload...), 0
		})
		openPorts[port] = true
		spec = append(spec, fmt.Sprintf("U:%d", port))
	}
	for i := 0; i < silentN; i++ {
		port := listenUDPLocal(t, nil)
		spec = append(spec, fmt.Sprintf("U:%d", port))
	}
	for i := 0; i < 4; i++ {
		spec = append(spec, fmt.Sprintf("U:%d", closedUDPPort(t)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	started := time.Now()
	results, err := Scan(ctx, "127.0.0.1", strings.Join(spec, ","),
		WithShuffle(true),
		WithTCPProbeConcurrency(16),
		WithTCPProbeTimeout(2*time.Second),
		WithWaiting(0.3),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := countSynPorts(collectSynResults(t, results, 12*time.Second))
	elapsed := time.Since(started)
	t.Logf("burst udp scan %s results %v", elapsed, got)
	if ctx.Err() != nil || elapsed > 8*time.Second {
		t.Fatalf("burst scan took %s", elapsed)
	}
	if len(got) != opens {
		t.Fatalf("found %d/%d open udp ports: %v", len(got), opens, got)
	}
	for port, n := range got {
		if !openPorts[port] || n != 1 {
			t.Fatalf("port %d count %d is not a single open service", port, n)
		}
	}
}

func TestLoopbackUDPCancelStopsTheWait(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	first := listenUDPLocal(t, nil)
	second := listenUDPLocal(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	results, err := Scan(ctx, "127.0.0.1", fmt.Sprintf("U:%d,U:%d", first, second),
		WithTCPProbeConcurrency(2),
		WithWaiting(2),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	time.AfterFunc(200*time.Millisecond, cancel)
	got := collectSynResults(t, results, 3*time.Second)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancel left the udp wait running for %s", elapsed)
	}
	if len(got) != 0 {
		t.Fatalf("canceled scan reported %v", got)
	}
}

func TestLoopbackUDPPayloadSizesReachTheSocket(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	port := listenUDPLocal(t, func(payload []byte) ([]byte, time.Duration) {
		return append([]byte{byte(len(payload))}, payload...), 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	session, err := netstackvm.OpenHalfOpenSYN(ctx, netstackvm.HalfOpenSYNConfig{
		Iface:    loop,
		SourceIP: net.IPv4(127, 0, 0, 1),
	})
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	for _, n := range []int{0, 1, 2, 3, 4, 30} {
		payload := bytes.Repeat([]byte{0x61}, n)
		probeCtx, stop := context.WithTimeout(ctx, time.Second)
		got, err := session.ProbeUDP(probeCtx, fmt.Sprintf("127.0.0.1:%d", port), payload)
		stop()
		if err != nil {
			t.Errorf("payload %d: %v", n, err)
			continue
		}
		if len(got.Payload) != n+1 || int(got.Payload[0]) != n {
			t.Errorf("payload %d reply = %x", n, got.Payload)
		}
	}
}

func TestLoopbackUDPServiceProbesOpen(t *testing.T) {
	// Windows loopback rejects injected DNS (53) and NetBIOS (137). TFTP, RPC
	// and SNMP still have to carry their nmap payloads and come back open.
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	var spec []string
	want := map[int]bool{}
	for _, port := range []int{69, 111, 161} {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			t.Logf("udp/%d unavailable: %v", port, err)
			continue
		}
		if len(udpProbePayload(port)) == 0 {
			conn.Close()
			t.Fatalf("udp/%d has no service probe", port)
		}
		t.Cleanup(func() { conn.Close() })
		go func(c *net.UDPConn) {
			buf := make([]byte, 2048)
			for {
				n, peer, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				if n == 0 {
					continue
				}
				_, _ = c.WriteToUDP([]byte("pong"), peer)
			}
		}(conn)
		want[port] = true
		spec = append(spec, fmt.Sprintf("U:%d", port))
	}
	if len(spec) == 0 {
		t.Skip("service udp ports are not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results, err := Scan(ctx, "127.0.0.1", strings.Join(spec, ","),
		WithTCPProbeConcurrency(4),
		WithWaiting(0.8),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := countSynPorts(collectSynResults(t, results, 8*time.Second))
	if len(got) != len(want) {
		t.Fatalf("service probe results = %v, want %v", got, want)
	}
	for port := range want {
		if got[port] != 1 {
			t.Fatalf("udp/%d results = %v", port, got)
		}
	}
}

func TestLoopbackUDPHostAliasesCollapse(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	port := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte("pong"), 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results, err := Scan(ctx, "localhost,127.0.0.1,127.0.0.1/32", fmt.Sprintf("U:%d", port),
		WithShuffle(true),
		WithTCPProbeConcurrency(4),
		WithWaiting(0.4),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := countSynPorts(collectSynResults(t, results, 8*time.Second))
	if got[port] != 1 || len(got) != 1 {
		t.Fatalf("alias results = %v", got)
	}
}

func TestQueuedTCPIsProbedAfterUDPReleasesTheSlot(t *testing.T) {
	hold := make(chan struct{})
	var tcpCalls atomic.Int32
	rec := &recordingProber{}
	rec.udp = func(ctx context.Context, _ string, _ []byte) (netstackvm.UDPSegment, error) {
		select {
		case <-hold:
		case <-ctx.Done():
			return netstackvm.UDPSegment{}, ctx.Err()
		}
		return netstackvm.UDPSegment{}, netstackvm.ErrUDPNoResponse
	}
	rec.probe = func(ctx context.Context, _ string) (netstackvm.TCPSegment, error) {
		tcpCalls.Add(1)
		return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
	}
	var opened atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:      ctx,
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: 80 * time.Millisecond, waiting: time.Second},
		OpenPortHandlers: func(net.IP, int) {
			opened.Add(1)
		},
	}
	ch := make(chan *SynxTarget, 2)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 9, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	time.Sleep(200 * time.Millisecond)
	if tcpCalls.Load() != 1 || opened.Load() != 1 {
		t.Fatalf("tcp did not run while udp was held: calls %d opened %d", tcpCalls.Load(), opened.Load())
	}
	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mixed scan did not finish")
	}
	if tcpCalls.Load() != 1 || opened.Load() != 1 {
		t.Fatalf("tcp calls %d opened %d", tcpCalls.Load(), opened.Load())
	}
}

func TestQueuedUDPIsProbedAfterTCPReleasesTheSlot(t *testing.T) {
	hold := make(chan struct{})
	var udpCalls atomic.Int32
	rec := &recordingProber{}
	rec.probe = func(ctx context.Context, _ string) (netstackvm.TCPSegment, error) {
		select {
		case <-hold:
		case <-ctx.Done():
			return netstackvm.TCPSegment{}, ctx.Err()
		}
		return netstackvm.TCPSegment{}, netstackvm.ErrProbeNoResponse
	}
	rec.udp = func(ctx context.Context, _ string, _ []byte) (netstackvm.UDPSegment, error) {
		udpCalls.Add(1)
		return netstackvm.UDPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 9}, nil
	}
	var opened atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:      ctx,
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: time.Second, waiting: 80 * time.Millisecond},
		OpenPortHandlers: func(net.IP, int) {
			opened.Add(1)
		},
	}
	ch := make(chan *SynxTarget, 2)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 9, Mode: UDP}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	time.Sleep(200 * time.Millisecond)
	if udpCalls.Load() != 1 || opened.Load() != 1 {
		t.Fatalf("udp did not run while tcp was held: calls %d opened %d", udpCalls.Load(), opened.Load())
	}
	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mixed scan did not finish")
	}
	if udpCalls.Load() != 1 || opened.Load() != 1 {
		t.Fatalf("udp calls %d opened %d", udpCalls.Load(), opened.Load())
	}
}

func TestLoopbackQueuedTCPSurvivesSilentUDP(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	silent := listenUDPLocal(t, nil)
	ln := listenLocal(t)
	tcpPort := ln.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:    ctx,
		cancel: cancel,
		config: &SynxConfig{
			Ctx:                 ctx,
			Iface:               loop,
			SourceIP:            net.IPv4(127, 0, 0, 1),
			tcpProbeConcurrency: 1,
			tcpProbeTimeout:     200 * time.Millisecond,
			waiting:             700 * time.Millisecond,
		},
	}
	if err := scanner.startHalfOpen(ctx); err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	t.Cleanup(func() {
		if scanner.halfOpen != nil {
			_ = scanner.halfOpen.Close()
		}
	})
	var opened atomic.Int32
	scanner.OpenPortHandlers = func(_ net.IP, port int) {
		if port == tcpPort {
			opened.Add(1)
			return
		}
		t.Errorf("unexpected open port %d", port)
	}
	ch := make(chan *SynxTarget, 2)
	ch <- &SynxTarget{Host: "127.0.0.1", Port: silent, Mode: UDP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: tcpPort, Mode: TCP}
	close(ch)
	scanner.sendPacket(ch)
	if opened.Load() != 1 {
		t.Fatalf("queued tcp open count = %d", opened.Load())
	}
	acceptNone(t, ln)
}

func TestQueuedMixedBurstKeepsLaterDeadlines(t *testing.T) {
	hold := make(chan struct{})
	var tcpCalls, udpCalls atomic.Int32
	rec := &recordingProber{}
	rec.udp = func(ctx context.Context, target string, _ []byte) (netstackvm.UDPSegment, error) {
		n := udpCalls.Add(1)
		if n == 1 {
			select {
			case <-hold:
			case <-ctx.Done():
				return netstackvm.UDPSegment{}, ctx.Err()
			}
		}
		if target == "10.0.0.8:9" {
			return netstackvm.UDPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 9}, nil
		}
		return netstackvm.UDPSegment{}, netstackvm.ErrUDPNoResponse
	}
	rec.probe = func(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
		tcpCalls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("queued tcp probe has no deadline")
		} else if remain := time.Until(deadline); remain < 50*time.Millisecond {
			t.Errorf("queued tcp deadline already consumed: %s", remain)
		}
		_, portText, _ := strings.Cut(target, ":")
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Errorf("bad tcp target %s", target)
		}
		return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: uint16(port)}, nil
	}
	var opened atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:      ctx,
		halfOpen: rec,
		config:   &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: 80 * time.Millisecond, waiting: time.Second},
		OpenPortHandlers: func(net.IP, int) {
			opened.Add(1)
		},
	}
	ch := make(chan *SynxTarget, 6)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 7, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 9, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 81, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 10, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 82, Mode: TCP}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	time.Sleep(200 * time.Millisecond)
	if tcpCalls.Load() != 3 || udpCalls.Load() != 1 {
		t.Fatalf("during hold tcp=%d udp=%d", tcpCalls.Load(), udpCalls.Load())
	}
	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queued burst did not finish")
	}
	// Silent UDP targets are retried once. The open UDP target is not.
	if tcpCalls.Load() != 3 || udpCalls.Load() != 5 || opened.Load() != 4 {
		t.Fatalf("tcp %d udp %d opened %d", tcpCalls.Load(), udpCalls.Load(), opened.Load())
	}
}

func TestLoopbackSilentQueueThenOpenPorts(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	silentA := listenUDPLocal(t, nil)
	silentB := listenUDPLocal(t, nil)
	echo := listenUDPLocal(t, func([]byte) ([]byte, time.Duration) {
		return []byte("pong"), 0
	})
	ln := listenLocal(t)
	tcpPort := ln.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:    ctx,
		cancel: cancel,
		config: &SynxConfig{
			Ctx:                 ctx,
			Iface:               loop,
			SourceIP:            net.IPv4(127, 0, 0, 1),
			tcpProbeConcurrency: 1,
			tcpProbeTimeout:     150 * time.Millisecond,
			waiting:             300 * time.Millisecond,
		},
	}
	if err := scanner.startHalfOpen(ctx); err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	t.Cleanup(func() {
		if scanner.halfOpen != nil {
			_ = scanner.halfOpen.Close()
		}
	})
	var opened atomic.Int32
	scanner.OpenPortHandlers = func(_ net.IP, port int) {
		if port == tcpPort || port == echo {
			opened.Add(1)
			return
		}
		t.Errorf("unexpected open port %d", port)
	}
	ch := make(chan *SynxTarget, 4)
	ch <- &SynxTarget{Host: "127.0.0.1", Port: silentA, Mode: UDP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: silentB, Mode: UDP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: echo, Mode: UDP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: tcpPort, Mode: TCP}
	close(ch)
	scanner.sendPacket(ch)
	if opened.Load() != 2 {
		t.Fatalf("queued open count = %d", opened.Load())
	}
	acceptNone(t, ln)
}
