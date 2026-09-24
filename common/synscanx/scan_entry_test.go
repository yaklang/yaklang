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
	limiter := rate.NewLimiter(rate.Every(time.Duration(cfg.rateLimitDelayMs*float64(time.Millisecond))), cfg.rateLimitDelayGap)
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatal(err)
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
	start    func(context.Context, string) (tcpSYNProbe, error)
}

func (r *recordingProber) startTCPProbe(ctx context.Context, target string) (tcpSYNProbe, error) {
	r.mu.Lock()
	r.got = append(r.got, target)
	r.contexts = append(r.contexts, ctx)
	r.mu.Unlock()
	if r.start != nil {
		return r.start(ctx, target)
	}
	return &stubSYNProbe{}, nil
}
func (r *recordingProber) Close() error { return nil }

type stubSYNProbe struct {
	send    func(context.Context) error
	receive func(context.Context) (netstackvm.TCPSegment, error)
	close   func()
}

func (p *stubSYNProbe) ProbeSYNContext(ctx context.Context) (netstackvm.TCPSegment, error) {
	if p.send != nil {
		return netstackvm.TCPSegment{}, p.send(ctx)
	}
	return netstackvm.TCPSegment{}, nil
}
func (p *stubSYNProbe) ReceiveSYNACKContext(ctx context.Context) (netstackvm.TCPSegment, error) {
	if p.receive != nil {
		return p.receive(ctx)
	}
	return netstackvm.TCPSegment{}, nil
}
func (p *stubSYNProbe) Close() error {
	if p.close != nil {
		p.close()
	}
	return nil
}

func TestSendPacketUsesIndependentSynchronousProbes(t *testing.T) {
	var closed, received, opened atomic.Int32
	rec := &recordingProber{}
	rec.start = func(lifetime context.Context, target string) (tcpSYNProbe, error) {
		if _, ok := lifetime.Deadline(); !ok {
			t.Error("probe has no independent deadline")
		}
		return &stubSYNProbe{
			send: func(ctx context.Context) error {
				if ctx != lifetime {
					t.Error("send uses a different lifetime")
				}
				if target == "10.0.0.8:82" {
					return netstackvm.ErrProbeSend
				}
				return nil
			},
			receive: func(ctx context.Context) (netstackvm.TCPSegment, error) {
				received.Add(1)
				if ctx != lifetime {
					t.Error("receive uses a different lifetime")
				}
				if target == "10.0.0.8:81" {
					<-ctx.Done()
					return netstackvm.TCPSegment{}, ctx.Err()
				}
				return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
			}, close: func() { closed.Add(1) },
		}, nil
	}
	scanner := &Scannerx{ctx: context.Background(), halfOpen: rec, config: &SynxConfig{tcpProbeTimeout: 30 * time.Millisecond, tcpProbeConcurrency: 2}, OpenPortHandlers: func(ip net.IP, port int) {
		if port != 80 {
			t.Errorf("unexpected open port %d", port)
		}
		opened.Add(1)
	}}
	targetCh := make(chan *SynxTarget, 3)
	for _, port := range []int{81, 80, 82} {
		targetCh <- &SynxTarget{Host: "10.0.0.8", Port: port, Mode: TCP}
	}
	close(targetCh)
	scanner.sendPacket(targetCh)
	if closed.Load() != 3 || received.Load() != 2 || opened.Load() != 1 {
		t.Fatalf("closed=%d received=%d opened=%d", closed.Load(), received.Load(), opened.Load())
	}
	if len(rec.contexts) != 3 {
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
	var closed atomic.Int32
	rec := &recordingProber{start: func(context.Context, string) (tcpSYNProbe, error) {
		started <- struct{}{}
		return &stubSYNProbe{receive: func(ctx context.Context) (netstackvm.TCPSegment, error) {
			<-ctx.Done()
			return netstackvm.TCPSegment{}, ctx.Err()
		}, close: func() { closed.Add(1) }}, nil
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
	if closed.Load() < 2 {
		t.Fatal("active probes not closed")
	}
	for _, ctx := range rec.contexts {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("context remained active: %v", ctx.Err())
		}
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
