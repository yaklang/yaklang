package synscanx

import (
	"context"
	"errors"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/netstackvm"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/time/rate"
)

func TestChooseRouteSamplePrefersNonLoopback(t *testing.T) {
	if got := chooseRouteSample(""); got != "" {
		t.Fatalf("empty target sample = %q", got)
	}
	if got := chooseRouteSample("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("only loopback sample = %q", got)
	}
	if got := chooseRouteSample("127.0.0.1,10.1.2.3,8.8.8.8"); got != "10.1.2.3" {
		t.Fatalf("sample = %q, want first non-loopback", got)
	}
}

func TestToUint16AcceptsNumericTypes(t *testing.T) {
	cases := []struct {
		in   interface{}
		want uint16
	}{
		{uint16(7), 7},
		{int(8), 8},
		{int64(9), 9},
		{float64(10), 10},
		{"nope", 0},
		{nil, 0},
	}
	for _, tc := range cases {
		if got := toUint16(tc.in); got != tc.want {
			t.Fatalf("toUint16(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSynScanResultString(t *testing.T) {
	var none *synscan.SynScanResult
	if none.String() != "" {
		t.Fatal("nil result string")
	}
	none.Show()
	got := (&synscan.SynScanResult{Host: "10.0.0.8", Port: 80}).String()
	if got != "OPEN: 10.0.0.8:80          from synscan" {
		t.Fatalf("result string = %q", got)
	}
}

func TestCallCallbackRecoversPanic(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.callback = func(result *synscan.SynScanResult) {
		panic("callback boom")
	}
	cfg.callCallback(&synscan.SynScanResult{Host: "10.0.0.1", Port: 1})
}

func TestGetNonExcludedPortsSkipsExcludedAndKeepsOrder(t *testing.T) {
	s := newPlanScanner(t)
	s.config.excludePorts = utils.NewPortsFilter("81,83")
	ports := s.GetNonExcludedPorts("80,81,82,83")
	want := []int{80, 82}
	if len(ports) != len(want) {
		t.Fatalf("ports = %v", ports)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports = %v, want %v", ports, want)
		}
	}
}

func TestGetNonExcludedHostsSkipsExcluded(t *testing.T) {
	s := newPlanScanner(t)
	WithExcludeHosts("10.0.0.2")(s.config)
	hosts := s.GetNonExcludedHosts("10.0.0.1,10.0.0.2,10.0.0.3")
	want := []string{"10.0.0.1", "10.0.0.3"}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %v", hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Fatalf("hosts = %v, want %v", hosts, want)
		}
	}
}

func TestSubmitTargetEmitsPlannedTCPTargets(t *testing.T) {
	s := newPlanScanner(t)
	ch, err := s.SubmitTarget("10.0.0.1,10.0.0.2", "80,443")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for target := range ch {
		if target.Mode != TCP {
			t.Fatalf("mode = %v", target.Mode)
		}
		got = append(got, target.Host+":"+itoa(target.Port))
	}
	want := []string{"10.0.0.1:80", "10.0.0.1:443", "10.0.0.2:80", "10.0.0.2:443"}
	if len(got) != len(want) {
		t.Fatalf("targets = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets = %v, want %v", got, want)
		}
	}
}

func TestSubmitTargetRejectsEmptyInput(t *testing.T) {
	s := newPlanScanner(t)
	if _, err := s.SubmitTarget("", "80"); err == nil {
		t.Fatal("expected error for empty targets")
	}
	s = newPlanScanner(t)
	if _, err := s.SubmitTarget("10.0.0.1", ""); err == nil {
		t.Fatal("expected error for empty ports")
	}
}

func TestSubmitTargetStopsWhenOpenPortCapAlreadyHit(t *testing.T) {
	s := newPlanScanner(t)
	s.config.maxOpenPorts = 1
	s.ipOpenPortMap.Store("10.0.0.1", uint16(1))
	ch, err := s.SubmitTarget("10.0.0.1,10.0.0.2", "80,81")
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
		t.Fatal("submit continued after the open-port cap was already reached")
	}
}

func TestGenerateHostPortStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := generateHostPort(ctx, []string{"10.0.0.1", "10.0.0.2"}, []int{1, 2, 3})
	first := <-out
	if first.Host != "10.0.0.1" || first.Port != 1 {
		t.Fatalf("first = %+v", first)
	}
	cancel()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("generateHostPort did not stop after cancel")
		}
	}
}

func TestLoopbackScanSubmitsEveryRequestedPort(t *testing.T) {
	// This test exercises submission and loopback capture, not system routing.
	// In particular Go 1.22's Darwin route parser can fail checkptr under -race.
	ifaces, ifaceErr := net.Interfaces()
	if ifaceErr != nil {
		t.Skipf("interfaces unavailable: %v", ifaceErr)
	}
	var loop *net.Interface
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagLoopback != 0 {
			loop = &ifaces[i]
			break
		}
	}
	if loop == nil {
		t.Skip("no loopback interface")
	}
	restore := stubRouteLookup(t, func(time.Duration, string) (*net.Interface, net.IP, net.IP, error) {
		return loop, nil, net.IPv4(127, 0, 0, 1), nil
	})
	defer restore()

	var submitted []string
	ch, err := Scan(context.Background(), "127.0.0.1", "80,443,1",
		WithWaiting(1),
		WithShuffle(false),
		WithSubmitTaskCallback(func(addr string) {
			submitted = append(submitted, addr)
		}),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	for range ch {
	}
	want := []string{"127.0.0.1:1", "127.0.0.1:80", "127.0.0.1:443"}
	if len(submitted) != len(want) {
		t.Fatalf("submitted %v, want %v", submitted, want)
	}
	for i := range want {
		if submitted[i] != want[i] {
			t.Fatalf("submitted %v, want %v", submitted, want)
		}
	}
}

func TestConcurrentBelowTenStillHasBurst(t *testing.T) {
	cfg := NewDefaultConfig()
	WithConcurrent(4)(cfg)
	if cfg.rateLimitDelayGap < 1 {
		t.Fatalf("burst = %d, a zero burst limiter sends no packets", cfg.rateLimitDelayGap)
	}
	if cfg.tcpProbeConcurrency != 4 {
		t.Fatalf("concurrency = %d, want 4 outstanding probes", cfg.tcpProbeConcurrency)
	}
	limiter := rate.NewLimiter(rate.Every(time.Duration(cfg.rateLimitDelayMs*float64(time.Millisecond))), cfg.rateLimitDelayGap)
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	pps, inFlight := synPacketRate(cfg)
	if pps != 4 || inFlight != 4 {
		t.Fatalf("session budget = %d pps, %d in flight", pps, inFlight)
	}
}

func TestPortsIncludeUDP(t *testing.T) {
	if portsIncludeUDP("80,443") {
		t.Fatal("tcp ports were treated as udp")
	}
	if !portsIncludeUDP("80,U:53") {
		t.Fatal("udp port was missed")
	}
}

type recordingProber struct {
	mu       sync.Mutex
	got      []string
	contexts []context.Context
	probe    func(context.Context, string) (netstackvm.TCPSegment, error)
}

func (r *recordingProber) Probe(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
	r.mu.Lock()
	r.got = append(r.got, target)
	r.contexts = append(r.contexts, ctx)
	fn := r.probe
	r.mu.Unlock()
	if fn != nil {
		return fn(ctx, target)
	}
	return netstackvm.TCPSegment{}, nil
}
func (r *recordingProber) Close() error { return nil }

func TestSendPacketUsesIndependentSynchronousProbes(t *testing.T) {
	var calls, opened atomic.Int32
	rec := &recordingProber{}
	rec.probe = func(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
		calls.Add(1)
		if _, ok := ctx.Deadline(); !ok {
			t.Error("probe has no independent deadline")
		}
		switch target {
		case "10.0.0.8:82":
			return netstackvm.TCPSegment{}, netstackvm.ErrProbeSend
		case "10.0.0.8:81":
			<-ctx.Done()
			return netstackvm.TCPSegment{}, ctx.Err()
		case "10.0.0.8:83":
			return netstackvm.TCPSegment{}, netstackvm.ErrProbeRefused
		case "10.0.0.8:84":
			return netstackvm.TCPSegment{}, fmt.Errorf("%w after 3 attempts", netstackvm.ErrProbeNoResponse)
		default:
			return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
		}
	}
	scanner := &Scannerx{ctx: context.Background(), halfOpen: rec, config: &SynxConfig{tcpProbeTimeout: 30 * time.Millisecond, tcpProbeConcurrency: 2}, OpenPortHandlers: func(ip net.IP, port int) {
		if port != 80 || ip == nil || !ip.Equal(net.IPv4(10, 0, 0, 8)) {
			t.Errorf("unexpected open %v:%d", ip, port)
		}
		opened.Add(1)
	}}
	targetCh := make(chan *SynxTarget, 5)
	for _, port := range []int{81, 80, 82, 83, 84} {
		targetCh <- &SynxTarget{Host: "10.0.0.8", Port: port, Mode: TCP}
	}
	close(targetCh)
	scanner.sendPacket(targetCh)
	if calls.Load() != 5 || opened.Load() != 1 {
		t.Fatalf("calls=%d opened=%d", calls.Load(), opened.Load())
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.contexts) != 5 {
		t.Fatalf("contexts=%d", len(rec.contexts))
	}
	for i, ctx := range rec.contexts {
		if ctx.Err() == nil {
			t.Error("probe context leaked")
		}
		for j := 0; j < i; j++ {
			if ctx == rec.contexts[j] {
				t.Error("targets shared a lifetime")
			}
		}
	}
}

func TestSendPacketBoundsWorkersAndCancelsPendingProbes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 10)
	rec := &recordingProber{probe: func(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
		started <- struct{}{}
		<-ctx.Done()
		return netstackvm.TCPSegment{}, ctx.Err()
	}}
	scanner := &Scannerx{ctx: ctx, halfOpen: rec, config: &SynxConfig{tcpProbeConcurrency: 2}}
	targets := make(chan *SynxTarget, 10)
	for i := 0; i < 10; i++ {
		targets <- &SynxTarget{Host: "10.0.0.8", Port: 80 + i, Mode: TCP}
	}
	close(targets)
	done := make(chan struct{})
	go func() { scanner.sendPacket(targets); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("worker limit exceeded")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not drain workers")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.contexts) < 2 {
		t.Fatal("active probes were not started")
	}
	for _, probeCtx := range rec.contexts {
		if !errors.Is(probeCtx.Err(), context.Canceled) && !errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			t.Errorf("context remained active: %v", probeCtx.Err())
		}
	}
}

func TestSynPacketRateFollowsYakLimits(t *testing.T) {
	pps, inFlight := synPacketRate(nil)
	if pps != defaultSYNPacketsPerSecond || inFlight != defaultTCPProbeConcurrency {
		t.Fatalf("nil config = %d pps, %d in flight", pps, inFlight)
	}
	pps, inFlight = synPacketRate(NewDefaultConfig())
	if pps != 1000 || inFlight != 256 {
		t.Fatalf("default = %d pps, %d in flight", pps, inFlight)
	}

	slow := NewDefaultConfig()
	WithRateLimit(10000, 1)(slow)
	pps, inFlight = synPacketRate(slow)
	if pps != 1 || inFlight != 256 {
		t.Fatalf("slow rate = %d pps, %d in flight", pps, inFlight)
	}

	unlimited := NewDefaultConfig()
	WithRateLimit(0, 1)(unlimited)
	pps, _ = synPacketRate(unlimited)
	if pps != defaultSYNPacketsPerSecond {
		t.Fatalf("non-positive delay = %d pps, want the session default", pps)
	}

	fast := NewDefaultConfig()
	WithConcurrent(2000)(fast)
	pps, inFlight = synPacketRate(fast)
	if pps != 2000 || inFlight != 2000 {
		t.Fatalf("concurrent(2000) = %d pps, %d in flight", pps, inFlight)
	}

	capped := NewDefaultConfig()
	WithConcurrent(1000000)(capped)
	pps, inFlight = synPacketRate(capped)
	if pps != maxSYNPacketsPerSecond || inFlight != maxTCPProbeConcurrency {
		t.Fatalf("huge concurrent = %d pps, %d in flight", pps, inFlight)
	}
}

func TestTCPSubmitDoesNotBlockOnLegacyLimiter(t *testing.T) {
	s := newPlanScanner(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	s.ctx = ctx
	s.limiter = rate.NewLimiter(rate.Every(time.Hour), 1)
	start := time.Now()
	ch, err := s.SubmitTarget("10.0.0.8", "80,81,82")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for range ch {
		n++
	}
	if n != 3 {
		t.Fatalf("submitted %d TCP targets", n)
	}
	if time.Since(start) > 150*time.Millisecond {
		t.Fatalf("TCP submit waited on the legacy limiter for %s", time.Since(start))
	}
}

func TestSendPacketSkipsTCPWithoutSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:        ctx,
		config:     &SynxConfig{},
		PacketChan: make(chan []byte, 1),
		LoopPacket: make(chan []byte, 1),
	}
	ch := make(chan *SynxTarget, 2)
	ch <- nil
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	close(ch)
	done := make(chan struct{})
	go func() { scanner.sendPacket(ch); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TCP target blocked in the legacy packet queue")
	}
	select {
	case pkt := <-scanner.PacketChan:
		t.Fatalf("legacy TCP packet of %d bytes", len(pkt))
	default:
	}
}

func TestSendPacketKeepsOneTCPProbeWhenConcurrencyIsOne(t *testing.T) {
	var current, peak atomic.Int32
	rec := &recordingProber{probe: func(context.Context, string) (netstackvm.TCPSegment, error) {
		n := current.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		current.Add(-1)
		return netstackvm.TCPSegment{}, netstackvm.ErrProbeNoResponse
	}}
	scanner := &Scannerx{ctx: context.Background(), halfOpen: rec, config: &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: time.Second}}
	ch := make(chan *SynxTarget, 4)
	for i := 0; i < 4; i++ {
		ch <- &SynxTarget{Host: "10.0.0.8", Port: 80 + i, Mode: TCP}
	}
	close(ch)
	scanner.sendPacket(ch)
	if peak.Load() != 1 {
		t.Fatalf("peak in-flight probes = %d, want 1", peak.Load())
	}
}

func TestOpenResultDroppedWhenScanCanceledDuringProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rec := &recordingProber{probe: func(context.Context, string) (netstackvm.TCPSegment, error) {
		cancel()
		return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
	}}
	var opened atomic.Int32
	scanner := &Scannerx{ctx: ctx, halfOpen: rec, config: &SynxConfig{tcpProbeConcurrency: 1}, OpenPortHandlers: func(net.IP, int) {
		opened.Add(1)
	}}
	ch := make(chan *SynxTarget, 1)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	close(ch)
	scanner.sendPacket(ch)
	if opened.Load() != 0 {
		t.Fatal("canceled scan reported an open port")
	}
}

func loopbackScannerIface(t *testing.T) *net.Interface {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("interfaces unavailable: %v", err)
	}
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagUp != 0 && ifaces[i].Flags&net.FlagLoopback != 0 {
			return &ifaces[i]
		}
	}
	t.Skip("no loopback interface")
	return nil
}

func stubLoopbackRoute(t *testing.T, loop *net.Interface) {
	t.Helper()
	restore := stubRouteLookup(t, func(time.Duration, string) (*net.Interface, net.IP, net.IP, error) {
		return loop, nil, net.IPv4(127, 0, 0, 1), nil
	})
	t.Cleanup(restore)
}

func TestLoopbackScanReportsOpenAndIgnoresClosed(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)

	closedLn, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closedLn.Addr().(*net.TCPAddr).Port
	if err := closedLn.Close(); err != nil {
		t.Fatal(err)
	}
	openLn, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer openLn.Close()
	openPort := openLn.Addr().(*net.TCPAddr).Port
	excludedLn, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer excludedLn.Close()
	excludedPort := excludedLn.Addr().(*net.TCPAddr).Port

	var callbacks []int
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results, err := Scan(ctx, "127.0.0.1", fmt.Sprintf("%d,%d,%d", closedPort, openPort, excludedPort),
		WithShuffle(false),
		WithConcurrent(4),
		WithTCPProbeTimeout(3*time.Second),
		WithExcludePorts(fmt.Sprintf("%d", excludedPort)),
		WithCallback(func(result *synscan.SynScanResult) {
			callbacks = append(callbacks, result.Port)
		}),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := map[int]int{}
	for result := range results {
		got[result.Port]++
	}
	if ctx.Err() != nil {
		t.Fatal("scan did not finish with its probes")
	}
	if got[openPort] != 1 || got[closedPort] != 0 || got[excludedPort] != 0 {
		t.Fatalf("results = %v, open %d closed %d excluded %d", got, openPort, closedPort, excludedPort)
	}
	if len(callbacks) != 1 || callbacks[0] != openPort {
		t.Fatalf("callbacks = %v, want [%d]", callbacks, openPort)
	}
	if err := openLn.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	conn, err := openLn.Accept()
	if conn != nil {
		conn.Close()
		t.Fatal("scan completed the handshake")
	}
	if err == nil {
		t.Fatal("expected listener timeout")
	}
}

func TestLoopbackScanContextCancelClosesResults(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	results, err := Scan(ctx, "127.0.0.1", "1-300",
		WithShuffle(false),
		WithConcurrent(4),
		WithTCPProbeTimeout(2*time.Second),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	for range results {
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("canceled scan stayed open for %s", time.Since(start))
	}
}

func TestProbeTCPRejectsBadTargetsWithoutCallingSession(t *testing.T) {
	var calls atomic.Int32
	rec := &recordingProber{probe: func(context.Context, string) (netstackvm.TCPSegment, error) {
		calls.Add(1)
		return netstackvm.TCPSegment{}, nil
	}}
	scanner := &Scannerx{ctx: context.Background(), halfOpen: rec, config: &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: time.Second}}
	ch := make(chan *SynxTarget, 3)
	ch <- &SynxTarget{Host: "", Port: 80, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 0, Mode: TCP}
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 70000, Mode: TCP}
	close(ch)
	scanner.sendPacket(ch)
	if calls.Load() != 0 {
		t.Fatalf("session probe calls = %d", calls.Load())
	}
}

func TestScanRejectsEmptyTargetBeforeCapture(t *testing.T) {
	if _, err := Scan(context.Background(), "", "80"); err == nil {
		t.Fatal("expected empty target error")
	}
	if _, err := ScanWithConfig("   ", "80", nil); err == nil {
		t.Fatal("expected blank target error")
	}
}

func newPlanScanner(t *testing.T) *Scannerx {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Scannerx{
		ctx:           ctx,
		cancel:        cancel,
		config:        &SynxConfig{shuffle: false, Ctx: ctx},
		ports:         utils.NewPortsFilter(),
		ipOpenPortMap: &sync.Map{},
		limiter:       rate.NewLimiter(rate.Limit(1e6), 1<<20),
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestUDPCaptureCannotBypassTCPProbeValidation(t *testing.T) {
	reported := 0
	s := &Scannerx{halfOpen: &recordingProber{}, OpenPortHandlers: func(net.IP, int) { reported++ }}
	buf := gopacket.NewSerializeBuffer()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 20), DstIP: net.IPv4(192, 0, 2, 10)}
	tcp := &layers.TCP{SrcPort: 80, DstPort: 40000, SYN: true, ACK: true, Ack: 123}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, tcp); err != nil {
		t.Fatal(err)
	}
	s.handlePacket(gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default))
	if reported != 0 {
		t.Fatal("uncorrelated TCP escaped via the UDP capture")
	}
	ip.Protocol = layers.IPProtocolUDP
	udp := &layers.UDP{SrcPort: 53, DstPort: 40000}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, udp); err != nil {
		t.Fatal(err)
	}
	s.handlePacket(gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default))
	if reported != 1 {
		t.Fatal("UDP result path was lost")
	}
}

func TestLoopbackScanDrainsSynchronousProbeResults(t *testing.T) {
	device := os.Getenv("NETSTACKVM_PCAP_DEVICE")
	if device == "" {
		t.Skip("set NETSTACKVM_PCAP_DEVICE for real loopback scan")
	}
	iface, err := net.InterfaceByName(device)
	if err != nil {
		t.Fatal(err)
	}
	if iface.Flags&net.FlagLoopback == 0 {
		t.Fatal("test requires a loopback interface")
	}
	restore := stubRouteLookup(t, func(time.Duration, string) (*net.Interface, net.IP, net.IP, error) {
		return iface, nil, net.IPv4(127, 0, 0, 1), nil
	})
	defer restore()
	var listeners []*net.TCPListener
	want := make(map[int]bool)
	var ports string
	for i := 0; i < 2; i++ {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		listeners = append(listeners, listener)
		port := listener.Addr().(*net.TCPAddr).Port
		want[port] = true
		if ports != "" {
			ports += ","
		}
		ports += fmt.Sprint(port)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, err := Scan(ctx, "127.0.0.1", ports, WithWaiting(30), WithShuffle(false), WithTCPProbeConcurrency(2), WithTCPProbeTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for result := range results {
		if !want[result.Port] {
			t.Fatalf("unexpected or duplicate result: %+v", result)
		}
		delete(want, result.Port)
	}
	if ctx.Err() != nil {
		t.Fatal("TCP-only scan waited for the legacy packet window instead of finishing with its probes")
	}
	if len(want) != 0 {
		t.Fatalf("result stream closed before probes completed: missing %v", want)
	}
	for _, listener := range listeners {
		_ = listener.SetDeadline(time.Now().Add(50 * time.Millisecond))
		conn, err := listener.Accept()
		if conn != nil {
			conn.Close()
			t.Fatal("scan completed the handshake")
		}
		if err == nil {
			t.Fatal("expected listener timeout")
		}
	}
}
