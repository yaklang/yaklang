package synscanx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

func collectSynResults(t *testing.T, ch <-chan *synscan.SynScanResult, limit time.Duration) []*synscan.SynScanResult {
	t.Helper()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	var out []*synscan.SynScanResult
	for {
		select {
		case result, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, result)
		case <-timer.C:
			t.Fatalf("result stream still open after %s (%d results)", limit, len(out))
		}
	}
}

func listenLocal(t *testing.T) *net.TCPListener {
	t.Helper()
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func acceptNone(t *testing.T, ln *net.TCPListener) {
	t.Helper()
	if err := ln.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	conn, err := ln.Accept()
	if conn != nil {
		conn.Close()
		t.Fatal("scan completed the handshake")
	}
	if err == nil {
		t.Fatal("expected listener timeout")
	}
}

func reserveClosedPorts(t *testing.T, n int) []int {
	t.Helper()
	held := make([]net.Listener, 0, n)
	ports := make([]int, 0, n)
	for len(ports) < n {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve closed ports: %v", err)
		}
		held = append(held, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	for _, ln := range held {
		ln.Close()
	}
	return ports
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = fmt.Sprintf("%d", port)
	}
	return strings.Join(parts, ",")
}

func TestSubmitTargetMixesHostsPortsAndProtocols(t *testing.T) {
	s := newPlanScanner(t)
	ch, err := s.SubmitTarget("10.0.0.8/30,::1", "443,U:53,80")
	if err != nil {
		t.Fatal(err)
	}
	type gotTarget struct {
		host string
		port int
		mode ProtocolType
	}
	var got []gotTarget
	for target := range ch {
		got = append(got, gotTarget{target.Host, target.Port, target.Mode})
	}
	hosts := utils.ParseStringToHosts("10.0.0.8/30,::1")
	if len(hosts) < 5 {
		t.Fatalf("host expansion = %v", hosts)
	}
	wantTCP := map[int]bool{80: true, 443: true}
	seenHosts := map[string]bool{}
	udpHosts := 0
	for _, target := range got {
		seenHosts[target.host] = true
		switch target.mode {
		case TCP:
			if !wantTCP[target.port] {
				t.Fatalf("unexpected tcp target %+v", target)
			}
		case UDP:
			if target.port != 53 {
				t.Fatalf("unexpected udp target %+v", target)
			}
			udpHosts++
		default:
			t.Fatalf("mode %+v", target)
		}
	}
	if udpHosts != len(hosts) {
		t.Fatalf("udp targets = %d, hosts = %d", udpHosts, len(hosts))
	}
	for _, host := range hosts {
		if !seenHosts[host] {
			t.Fatalf("missing host %s in %v", host, seenHosts)
		}
	}
}

func TestSendPacketMixesTCPProbeAndUDPQueue(t *testing.T) {
	rec := &recordingProber{probe: func(_ context.Context, target string) (netstackvm.TCPSegment, error) {
		if target == "10.0.0.8:80" {
			return netstackvm.TCPSegment{RemoteIP: net.IPv4(10, 0, 0, 8), RemotePort: 80}, nil
		}
		return netstackvm.TCPSegment{}, netstackvm.ErrProbeNoResponse
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := &Scannerx{
		ctx:        ctx,
		config:     &SynxConfig{Iface: &net.Interface{Name: "lo", Flags: net.FlagLoopback}, SourceIP: net.IPv4(127, 0, 0, 1), tcpProbeConcurrency: 2, tcpProbeTimeout: time.Second},
		halfOpen:   rec,
		PacketChan: make(chan []byte, 4),
		LoopPacket: make(chan []byte, 4),
		OpenPortHandlers: func(ip net.IP, port int) {
			if port != 80 || !ip.Equal(net.IPv4(10, 0, 0, 8)) {
				t.Errorf("open %v:%d", ip, port)
			}
		},
	}
	ch := make(chan *SynxTarget, 4)
	ch <- &SynxTarget{Host: "10.0.0.8", Port: 80, Mode: TCP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: 53, Mode: UDP}
	ch <- &SynxTarget{Host: "10.0.0.9", Port: 81, Mode: TCP}
	ch <- &SynxTarget{Host: "127.0.0.1", Port: 123, Mode: UDP}
	close(ch)
	scanner.sendPacket(ch)
	rec.mu.Lock()
	probed := append([]string(nil), rec.got...)
	rec.mu.Unlock()
	if len(probed) != 2 || probed[0] == probed[1] {
		t.Fatalf("tcp probes = %v", probed)
	}
	udp := 0
	for i := 0; i < 2; i++ {
		select {
		case pkt := <-scanner.LoopPacket:
			if len(pkt) == 0 {
				t.Fatal("empty udp packet")
			}
			udp++
		case pkt := <-scanner.PacketChan:
			t.Fatalf("udp went to the non-loopback queue (%d bytes)", len(pkt))
		default:
			t.Fatal("udp packet was not queued")
		}
	}
	if udp != 2 {
		t.Fatalf("queued udp = %d", udp)
	}
}

func TestLoopbackMixedTargetTypesAndUDP(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	openLn := listenLocal(t)
	excludedLn := listenLocal(t)
	openPort := openLn.Addr().(*net.TCPAddr).Port
	excludedPort := excludedLn.Addr().(*net.TCPAddr).Port
	closed := reserveClosedPorts(t, 1)[0]

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started := time.Now()
	results, err := Scan(ctx, "localhost,127.0.0.1,127.0.0.1/32",
		fmt.Sprintf("%d,%d,%d,U:53", openPort, excludedPort, closed),
		WithShuffle(true),
		WithTCPProbeConcurrency(8),
		WithTCPProbeTimeout(4*time.Second),
		WithWaiting(0.3),
		WithExcludePorts(fmt.Sprintf("%d", excludedPort)),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := collectSynResults(t, results, 15*time.Second)
	if time.Since(started) > 12*time.Second {
		t.Fatalf("mixed loopback scan took %s", time.Since(started))
	}
	foundOpen := 0
	for _, result := range got {
		if result.Port == excludedPort || result.Port == closed || result.Port == 53 {
			t.Fatalf("unexpected result %+v", result)
		}
		if result.Port == openPort {
			foundOpen++
		}
	}
	if foundOpen != 1 {
		t.Fatalf("open results = %d, all = %v", foundOpen, got)
	}
	acceptNone(t, openLn)
	acceptNone(t, excludedLn)
}

func TestLoopbackStressFindsOpenPortsWithoutHandshake(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	const opens = 16
	const closedN = 480
	listeners := make([]*net.TCPListener, 0, opens)
	openPorts := map[int]bool{}
	for i := 0; i < opens; i++ {
		ln := listenLocal(t)
		listeners = append(listeners, ln)
		openPorts[ln.Addr().(*net.TCPAddr).Port] = true
	}
	ports := reserveClosedPorts(t, closedN)
	for port := range openPorts {
		ports = append(ports, port)
	}

	run := func(pass int) time.Duration {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		started := time.Now()
		results, err := Scan(ctx, "127.0.0.1", joinPorts(ports),
			WithShuffle(pass == 2),
			WithTCPProbeConcurrency(128),
			WithTCPProbeTimeout(3*time.Second),
		)
		if err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		got := collectSynResults(t, results, 40*time.Second)
		elapsed := time.Since(started)
		if ctx.Err() != nil {
			t.Fatalf("pass %d timed out after %s", pass, elapsed)
		}
		found := map[int]int{}
		for _, result := range got {
			if !openPorts[result.Port] {
				t.Fatalf("pass %d reported closed port %+v", pass, result)
			}
			found[result.Port]++
		}
		if len(found) != opens {
			t.Fatalf("pass %d found %d/%d open ports", pass, len(found), opens)
		}
		for _, ln := range listeners {
			acceptNone(t, ln)
		}
		t.Logf("pass %d: %d targets in %s (%.0f ports/s)", pass, len(ports), elapsed, float64(len(ports))/elapsed.Seconds())
		if elapsed > 25*time.Second {
			t.Fatalf("pass %d too slow: %s", pass, elapsed)
		}
		return elapsed
	}

	before := runtime.NumGoroutine()
	run(1)
	run(2)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+50 {
		t.Fatalf("goroutines %d -> %d after two scans", before, after)
	}
}

func physicalIPv4Interface() *net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for i := range ifaces {
		iface := &ifaces[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		if len(iface.HardwareAddr) != 6 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			return iface
		}
	}
	return nil
}

func TestSelectedInterfaceGatewayStaysOnLink(t *testing.T) {
	iface := physicalIPv4Interface()
	if iface == nil {
		t.Skip("no physical ipv4 interface")
	}
	var src net.IP
	addrs, err := iface.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			src = ipNet.IP
			break
		}
	}
	gateway, err := gatewayForSelectedInterface(iface, src, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if !gatewayOnInterface(iface, gateway) {
		t.Fatalf("gateway %v is not on %s", gateway, iface.Name)
	}
	if gateway != nil && gateway.IsLoopback() {
		t.Fatalf("public route gateway is loopback: %v", gateway)
	}
	t.Logf("%s src %s gateway %v", iface.Name, src, gateway)
}

func TestPublicProbeClassifiesOpenSilenceAndBlackhole(t *testing.T) {
	iface := physicalIPv4Interface()
	if iface == nil {
		t.Skip("no physical ipv4 interface")
	}
	var src net.IP
	addrs, err := iface.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			src = ipNet.IP
			break
		}
	}
	gateway, err := gatewayForSelectedInterface(iface, src, "1.1.1.1")
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := netstackvm.OpenHalfOpenSYN(ctx, netstackvm.HalfOpenSYNConfig{
		Iface:            iface,
		SourceIP:         src,
		Gateway:          gateway,
		MaxInFlight:      2,
		PacketsPerSecond: 10,
	})
	if err != nil {
		t.Skipf("physical capture unavailable: %v", err)
	}
	defer session.Close()

	probe := func(target string) error {
		probeCtx, probeCancel := context.WithTimeout(ctx, 6*time.Second)
		defer probeCancel()
		segment, err := session.ProbeSYN(probeCtx, target)
		t.Logf("%s via %s gw %v -> %v err=%v", target, iface.Name, gateway, segment, err)
		if errors.Is(err, netstackvm.ErrProbeSend) {
			t.Fatalf("%s could not be sent: %v", target, err)
		}
		return err
	}
	if err := probe("192.0.2.1:80"); err == nil {
		t.Fatal("TEST-NET 192.0.2.1:80 was reported open")
	}
	_ = probe("1.1.1.1:443")
}

func TestPublicBlackholeAndLoopbackMixedProbe(t *testing.T) {
	openLn := listenLocal(t)
	openPort := openLn.Addr().(*net.TCPAddr).Port
	resetRouteCache()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	targets := "127.0.0.1,1.1.1.1,192.0.2.1"
	ports := fmt.Sprintf("%d,443", openPort)
	opts := []SynxConfigOption{
		WithShuffle(false),
		WithConcurrent(4),
		WithTCPProbeTimeout(6 * time.Second),
	}
	started := time.Now()
	results, err := Scan(ctx, targets, ports, opts...)
	if err != nil && errors.Is(err, netstackvm.ErrUnverifiedSYNTransport) {
		iface := physicalIPv4Interface()
		if iface == nil {
			t.Skipf("no physical interface after %v", err)
		}
		t.Logf("default route is not a verified SYN transport, retrying on %s", iface.Name)
		resetRouteCache()
		results, err = Scan(ctx, targets, ports, append(opts, WithIface(iface.Name))...)
	}
	if err != nil {
		t.Skipf("public probe unavailable: %v", err)
	}
	got := collectSynResults(t, results, 30*time.Second)
	elapsed := time.Since(started)
	t.Logf("mixed public scan: %d results in %s", len(got), elapsed)
	foundLocal := false
	for _, result := range got {
		if result.Host == "192.0.2.1" {
			t.Fatalf("blackhole 192.0.2.1 reported open: %+v", result)
		}
		if result.Port != openPort && result.Port != 443 {
			t.Fatalf("unexpected port %+v", result)
		}
		if result.Port == 443 && result.Host != "1.1.1.1" {
			t.Fatalf("443 was not from the public target: %+v", result)
		}
		if result.Port == openPort {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Fatalf("local listener %d missing from %+v", openPort, got)
	}
	if elapsed > 28*time.Second {
		t.Fatalf("mixed scan took %s", elapsed)
	}
	acceptNone(t, openLn)
}
