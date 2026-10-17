package synscanx

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/synscan"
)

func listenTCP(t *testing.T, ip net.IP) *net.TCPListener {
	t.Helper()
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: ip})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func listenUDP(t *testing.T, ip net.IP, port int, answer bool) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			_, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if answer {
				_, _ = conn.WriteToUDP([]byte("pong"), peer)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func tryListenUDP(ip net.IP, port int, answer bool) (int, func(), bool) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		return 0, nil, false
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			_, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if answer {
				_, _ = conn.WriteToUDP([]byte("pong"), peer)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port, func() { _ = conn.Close() }, true
}

type liveHit struct {
	host  string
	port  int
	proto string
}

func collectLive(t *testing.T, ctx context.Context, results <-chan *synscan.SynScanResult) []liveHit {
	t.Helper()
	var got []liveHit
	for {
		select {
		case result, ok := <-results:
			if !ok {
				return got
			}
			if result == nil {
				t.Fatal("nil scan result")
			}
			got = append(got, liveHit{result.Host, result.Port, result.Protocol})
		case <-ctx.Done():
			t.Fatal("scan did not finish before the deadline")
		}
	}
}

func hasLive(got []liveHit, host string, port int, proto string) bool {
	for _, hit := range got {
		if hit.host == host && hit.port == port && hit.proto == proto {
			return true
		}
	}
	return false
}

func TestLiveLoopbackMatrix(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)

	primary := net.IPv4(127, 0, 0, 1)
	secondary := net.IPv4(127, 0, 0, 2)
	tcpPrimary := listenTCP(t, primary)
	udpEcho := listenUDP(t, primary, 0, true)
	udpSilent := listenUDP(t, primary, 0, false)
	udpClosed := closedUDPPort(t)
	closedTCP := reserveClosedPorts(t, 8)

	targets := []string{"127.0.0.1"}
	var tcpSecondary *net.TCPListener
	if ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: secondary}); err != nil {
		t.Logf("127.0.0.2 is not bindable on this host: %v", err)
	} else {
		tcpSecondary = ln
		t.Cleanup(func() { _ = ln.Close() })
		targets = append(targets, "127.0.0.2")
	}

	ports := []string{
		strconv.Itoa(tcpPrimary.Addr().(*net.TCPAddr).Port),
		strconv.Itoa(udpClosed),
		"U:1",
		"U:" + strconv.Itoa(udpEcho),
		"U:" + strconv.Itoa(udpSilent),
		"U:" + strconv.Itoa(udpClosed),
	}
	for _, port := range closedTCP {
		ports = append(ports, strconv.Itoa(port))
	}
	var lowUDP int
	if port, closeLow, ok := tryListenUDP(primary, 1, true); ok {
		lowUDP = port
		t.Cleanup(closeLow)
	}
	if tcpSecondary != nil {
		ports = append(ports, strconv.Itoa(tcpSecondary.Addr().(*net.TCPAddr).Port))
	}

	outPath := filepath.Join(t.TempDir(), "open.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	results, err := Scan(ctx, strings.Join(targets, ","), strings.Join(ports, ","),
		WithShuffle(false),
		WithConcurrent(8),
		WithWaiting(0.8),
		WithTCPProbeTimeout(2*time.Second),
		WithOutputFile(outPath),
		WithOutputFilePrefix("hit "),
		WithCallback(func(result *synscan.SynScanResult) {
			if result != nil && result.Port == tcpPrimary.Addr().(*net.TCPAddr).Port && result.Protocol == "tcp" {
				panic("callback boom")
			}
		}),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := collectLive(t, ctx, results)
	t.Logf("live results: %+v", got)

	openTCP := tcpPrimary.Addr().(*net.TCPAddr).Port
	if !hasLive(got, "127.0.0.1", openTCP, "tcp") {
		t.Fatalf("missing tcp %d in %+v", openTCP, got)
	}
	if !hasLive(got, "127.0.0.1", udpEcho, "udp") {
		t.Fatalf("missing udp echo %d in %+v", udpEcho, got)
	}
	if hasLive(got, "127.0.0.1", udpSilent, "udp") || hasLive(got, "127.0.0.1", udpClosed, "udp") {
		t.Fatalf("silent or closed udp was reported: %+v", got)
	}
	for _, port := range closedTCP {
		if hasLive(got, "127.0.0.1", port, "tcp") {
			t.Fatalf("closed tcp %d was reported", port)
		}
	}
	for _, hit := range got {
		if hit.port == 1 && hit.proto != "udp" {
			t.Fatalf("port 1 reported as %s", hit.proto)
		}
		if hit.port == openTCP && hit.proto != "tcp" {
			t.Fatalf("open tcp reported as %s", hit.proto)
		}
	}
	if lowUDP == 1 && !hasLive(got, "127.0.0.1", 1, "udp") {
		t.Fatalf("answered udp/1 missing from %+v", got)
	}
	if tcpSecondary != nil {
		port := tcpSecondary.Addr().(*net.TCPAddr).Port
		if !hasLive(got, "127.0.0.2", port, "tcp") {
			t.Fatalf("second loopback tcp missing from %+v", got)
		}
		if hasLive(got, "127.0.0.1", port, "tcp") && port != openTCP {
			t.Fatalf("second loopback port was rewritten onto 127.0.0.1")
		}
	}

	acceptNone(t, tcpPrimary)
	if tcpSecondary != nil {
		acceptNone(t, tcpSecondary)
	}

	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, fmt.Sprintf("hit 127.0.0.1:%d\n", openTCP)) {
		t.Fatalf("output missing tcp line: %q", text)
	}
	if !strings.Contains(text, fmt.Sprintf("hit udp://127.0.0.1:%d\n", udpEcho)) {
		t.Fatalf("output missing udp line: %q", text)
	}
	if strings.Contains(text, fmt.Sprintf(":%d\n", udpSilent)) || strings.Contains(text, fmt.Sprintf(":%d\n", udpClosed)) {
		t.Fatalf("output contains a closed or silent port: %q", text)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("output file is group/world accessible: %s", info.Mode().Perm())
	}
}

func TestLiveTCPOpenPortSurvivesRateQueue(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	closed := reserveClosedPorts(t, 1)[0]
	ln := listenTCP(t, net.IPv4(127, 0, 0, 1))
	open := ln.Addr().(*net.TCPAddr).Port
	if open < closed {
		t.Skipf("open port %d is below closed port %d", open, closed)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// One packet per second, and a TCP budget shorter than that gap. The open
	// port is second, so it has to wait for a token before its response clock starts.
	results, err := Scan(ctx, "127.0.0.1", fmt.Sprintf("%d,%d", closed, open),
		WithShuffle(false),
		WithConcurrent(1),
		WithTCPProbeTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := collectLive(t, ctx, results)
	if !hasLive(got, "127.0.0.1", open, "tcp") {
		t.Fatalf("open port waited out of its budget: %+v", got)
	}
	if hasLive(got, "127.0.0.1", closed, "tcp") {
		t.Fatalf("closed port reported: %+v", got)
	}
	acceptNone(t, ln)
}

func TestLiveCancelUnblocksSilentUDP(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	var ports []string
	for i := 0; i < 4; i++ {
		ports = append(ports, "U:"+strconv.Itoa(listenUDP(t, net.IPv4(127, 0, 0, 1), 0, false)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	results, err := Scan(ctx, "127.0.0.1", strings.Join(ports, ","),
		WithShuffle(false),
		WithConcurrent(1),
		WithWaiting(4),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	time.AfterFunc(300*time.Millisecond, cancel)
	done := make(chan struct{})
	go func() {
		for result := range results {
			if result != nil {
				t.Errorf("canceled scan reported %s/%d %s", result.Host, result.Port, result.Protocol)
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled scan did not finish")
	}
}

func TestScanRejectsEmptyOrIPv6OnlyTargets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Scan(ctx, "::1", "80"); err == nil {
		t.Fatal("ipv6-only target was accepted")
	}
	if _, err := Scan(ctx, "", "80"); err == nil {
		t.Fatal("empty target was accepted")
	}
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	if _, err := Scan(ctx, "127.0.0.1", ""); err == nil {
		t.Fatal("empty ports were accepted")
	}
}

func TestLiveOutputFileStaysClosedAfterScan(t *testing.T) {
	loop := loopbackScannerIface(t)
	stubLoopbackRoute(t, loop)
	ln := listenTCP(t, net.IPv4(127, 0, 0, 1))
	port := ln.Addr().(*net.TCPAddr).Port
	outPath := filepath.Join(t.TempDir(), "ports.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results, err := Scan(ctx, "127.0.0.1", strconv.Itoa(port),
		WithConcurrent(2),
		WithTCPProbeTimeout(2*time.Second),
		WithOutputFile(outPath),
	)
	if err != nil {
		t.Skipf("packet capture is not available: %v", err)
	}
	got := collectLive(t, ctx, results)
	if !hasLive(got, "127.0.0.1", port, "tcp") {
		t.Fatalf("results = %+v", got)
	}
	file, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != fmt.Sprintf("127.0.0.1:%d", port) {
		t.Fatalf("lines = %#v", lines)
	}
}
