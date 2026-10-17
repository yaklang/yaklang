package synscanx

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

func TestOpenPortCapSkipsQueuedProbesAndKeepsOtherHosts(t *testing.T) {
	for _, mode := range []ProtocolType{TCP, UDP} {
		t.Run(itoa(int(mode)), func(t *testing.T) {
			rec := &recordingProber{}
			rec.probe = func(_ context.Context, target string) (netstackvm.TCPSegment, error) {
				host, port, err := utils.ParseStringToHostPort(target)
				return netstackvm.TCPSegment{RemoteIP: net.ParseIP(host), RemotePort: uint16(port)}, err
			}
			rec.udp = func(ctx context.Context, target string, _ []byte) (netstackvm.UDPSegment, error) {
				seg, err := rec.probe(ctx, target)
				return netstackvm.UDPSegment{RemoteIP: seg.RemoteIP, RemotePort: seg.RemotePort}, err
			}
			s := &Scannerx{
				ctx: context.Background(), halfOpen: rec,
				config:        &SynxConfig{maxOpenPorts: 1, tcpProbeConcurrency: 1},
				ipOpenPortMap: new(sync.Map),
			}
			s.OpenPortHandlers = func(ip net.IP, _ int) { s.ipOpenPortMap.Store(ip.String(), 1) }
			in := make(chan *SynxTarget, 33)
			for port := 1; port <= 32; port++ {
				in <- &SynxTarget{Host: "10.0.0.1", Port: port, Mode: mode}
			}
			in <- &SynxTarget{Host: "10.0.0.2", Port: 80, Mode: mode}
			close(in)
			s.sendPacket(in)
			got := rec.got
			if mode == UDP {
				got = rec.udpGot
			}
			if len(got) != 2 || got[0] != "10.0.0.1:1" || got[1] != "10.0.0.2:80" {
				t.Fatalf("capped host still consumed queued probes, or the other host was skipped: %v", got)
			}
		})
	}
}

func TestLiveMaxOpenPortsCapsConcurrentResultsPerHost(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	primary := net.IPv4(127, 0, 0, 1)
	first := listenTCP(t, primary)
	port := first.Addr().(*net.TCPAddr).Port
	secondary, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 2), Port: port})
	if err != nil {
		t.Skipf("second loopback host is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = secondary.Close() })
	listenUDP(t, primary, port, true)
	ports := []int{port}
	for i := 0; i < 7; i++ {
		ports = append(ports, listenTCP(t, primary).Addr().(*net.TCPAddr).Port)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cfg := NewDefaultConfig()
	cfg.Ctx = ctx
	WithMaxOpenPorts(1)(cfg)
	WithTCPProbeConcurrency(4)(cfg)
	WithTCPProbeTimeout(500 * time.Millisecond)(cfg)
	WithWaiting(0.3)(cfg)
	WithShuffle(false)(cfg)
	cfg.outputFile = filepath.Join(t.TempDir(), "ports.txt")
	var mu sync.Mutex
	callbacks := make(map[string]int)
	cfg.callback = func(result *synscan.SynScanResult) {
		mu.Lock()
		callbacks[result.Host]++
		mu.Unlock()
	}
	ch, err := ScanWithConfig("127.0.0.1,127.0.0.2", joinPorts(ports)+",U:"+itoa(port), cfg)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, hit := range collectLive(t, ctx, ch) {
		counts[hit.host]++
	}
	for _, host := range []string{"127.0.0.1", "127.0.0.2"} {
		if counts[host] != 1 {
			t.Errorf("maxOpenPorts(1) returned %d results for %s; all counts=%v", counts[host], host, counts)
		}
		if callbacks[host] != 1 {
			t.Errorf("maxOpenPorts(1) called the callback %d times for %s", callbacks[host], host)
		}
	}
	raw, err := os.ReadFile(cfg.outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(raw)), "\n"); len(lines) != 2 {
		t.Errorf("maxOpenPorts(1) wrote %d lines, want one per host: %s", len(lines), raw)
	}
}
