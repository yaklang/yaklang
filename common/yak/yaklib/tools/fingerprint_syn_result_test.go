package tools

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSynScanProbeTargetKeepsProtocolAndDecodedPort(t *testing.T) {
	encoded := utils.EncodeUDPPort(53)
	cases := []struct {
		in       synscan.SynScanResult
		proto    string
		port     int
		explicit bool
	}{
		{synscan.SynScanResult{Port: 53, Protocol: "udp"}, "udp", 53, true},
		{synscan.SynScanResult{Port: 53, Protocol: "UDP"}, "udp", 53, true},
		{synscan.SynScanResult{Port: encoded}, "udp", 53, true},
		{synscan.SynScanResult{Port: encoded, Protocol: "udp"}, "udp", 53, true},
		{synscan.SynScanResult{Port: 80}, "", 80, false},
		{synscan.SynScanResult{Port: 80, Protocol: "tcp"}, "tcp", 80, true},
	}
	for _, tc := range cases {
		proto, port, explicit := synScanProbeTarget(&tc.in)
		if proto != tc.proto || port != tc.port || explicit != tc.explicit {
			t.Fatalf("%+v -> %s %d explicit=%v", tc.in, proto, port, explicit)
		}
	}
}

func TestSynResultFingerprintKeepsTCPAndUDP(t *testing.T) {
	port := listenTCPAndUDPBanner(t)
	rules := fmt.Sprintf(`Probe TCP TcpBanner q|probe|
rarity 1
ports %d
match tcp-fixture m|^tcp-banner$|

Probe UDP UdpBanner q|probe|
rarity 1
ports %d
match udp-fixture m|^udp-banner$|
`, port, port)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	in := make(chan *synscan.SynScanResult, 3)
	in <- &synscan.SynScanResult{Host: "127.0.0.1", Port: port, Protocol: "udp"}
	in <- &synscan.SynScanResult{Host: "127.0.0.1", Port: utils.EncodeUDPPort(port)}
	in <- &synscan.SynScanResult{Host: "127.0.0.1", Port: port, Protocol: "tcp"}
	close(in)

	out, err := _scanFromTargetStream(in,
		fp.WithCtx(ctx),
		fp.WithTransportProtos(fp.TCP),
		fp.WithNmapRule(rules),
		fp.WithActiveMode(true),
		fp.WithDisableWebFingerprint(true),
		fp.WithProbeTimeout(time.Second),
		fp.WithProbesMax(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	var udp, tcp int
	for result := range out {
		if result == nil || result.Fingerprint == nil {
			t.Fatalf("empty result: %#v", result)
		}
		banner := result.Fingerprint.Banner
		switch {
		case strings.Contains(banner, "udp-banner"):
			udp++
			if result.Fingerprint.Proto != fp.UDP || result.Port != port {
				t.Fatalf("udp result proto=%s port=%d", result.Fingerprint.Proto, result.Port)
			}
		case strings.Contains(banner, "tcp-banner"):
			tcp++
			if result.Fingerprint.Proto != fp.TCP || result.Port != port {
				t.Fatalf("tcp result proto=%s port=%d", result.Fingerprint.Proto, result.Port)
			}
		default:
			t.Fatalf("unexpected fingerprint: state=%s proto=%s banner=%q reason=%s", result.State, result.Fingerprint.Proto, banner, result.Reason)
		}
	}
	if udp != 2 || tcp != 1 {
		t.Fatalf("udp results=%d tcp results=%d", udp, tcp)
	}
}

func TestBareAddressKeepsCallerTransport(t *testing.T) {
	port := listenTCPAndUDPBanner(t)
	rules := bannerRules(port)
	target := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	if banner := scanAddressBanners(t, rules, []string{target}, fp.UDP); !strings.Contains(banner, "udp-banner") {
		t.Fatalf("udp option scanned %q", banner)
	}
	if banner := scanAddressBanners(t, rules, []string{target}, fp.TCP); !strings.Contains(banner, "tcp-banner") {
		t.Fatalf("tcp option scanned %q", banner)
	}
	udpPort := listenUDPBanner(t)
	udpRules := bannerRules(udpPort)
	udpTarget := net.JoinHostPort("127.0.0.1", fmt.Sprint(udpPort))
	if banner := scanAddressBanners(t, udpRules, []string{udpTarget}, fp.TCP, fp.UDP); !strings.Contains(banner, "udp-banner") {
		t.Fatalf("tcp+udp option scanned %q", banner)
	}
}

func TestSynResultCacheDoesNotCrossProtocols(t *testing.T) {
	port := listenTCPAndUDPBanner(t)
	rules := bannerRules(port)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	tcp := scanSynBanner(t, ctx, rules, &synscan.SynScanResult{Host: "127.0.0.1", Port: port, Protocol: "tcp"}, true, fp.TCP)
	udp := scanSynBanner(t, ctx, rules, &synscan.SynScanResult{Host: "127.0.0.1", Port: port, Protocol: "udp"}, true, fp.TCP)
	again := scanSynBanner(t, ctx, rules, &synscan.SynScanResult{Host: "127.0.0.1", Port: port, Protocol: "tcp"}, true, fp.TCP)
	if !strings.Contains(tcp, "tcp-banner") || !strings.Contains(udp, "udp-banner") || !strings.Contains(again, "tcp-banner") {
		t.Fatalf("tcp=%q udp=%q again=%q", tcp, udp, again)
	}

	other := listenTCPAndUDPBanner(t)
	otherRules := bannerRules(other)
	first := scanSynBanner(t, ctx, otherRules, &synscan.SynScanResult{Host: "127.0.0.1", Port: other, Protocol: "udp"}, true, fp.UDP)
	second := scanSynBanner(t, ctx, otherRules, &synscan.SynScanResult{Host: "127.0.0.1", Port: other, Protocol: "tcp"}, true, fp.UDP)
	if !strings.Contains(first, "udp-banner") || !strings.Contains(second, "tcp-banner") {
		t.Fatalf("udp=%q tcp=%q", first, second)
	}
}

func bannerRules(port int) string {
	return fmt.Sprintf(`Probe TCP TcpBanner q|probe|
rarity 1
ports %d
match tcp-fixture m|^tcp-banner$|

Probe UDP UdpBanner q|probe|
rarity 1
ports %d
match udp-fixture m|^udp-banner$|
`, port, port)
}

func scanAddressBanners(t *testing.T, rules string, targets []string, protos ...fp.TransportProto) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := _scanFromTargetStream(targets,
		fp.WithCtx(ctx),
		fp.WithTransportProtos(protos...),
		fp.WithNmapRule(rules),
		fp.WithActiveMode(true),
		fp.WithDisableWebFingerprint(true),
		fp.WithProbeTimeout(time.Second),
		fp.WithProbesMax(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	var banners []string
	for result := range out {
		if result != nil && result.Fingerprint != nil {
			banners = append(banners, result.Fingerprint.Banner)
		}
	}
	return strings.Join(banners, ",")
}

func scanSynBanner(t *testing.T, ctx context.Context, rules string, res *synscan.SynScanResult, cache bool, protos ...fp.TransportProto) string {
	t.Helper()
	in := make(chan *synscan.SynScanResult, 1)
	in <- res
	close(in)
	opts := []fp.ConfigOption{
		fp.WithCtx(ctx),
		fp.WithTransportProtos(protos...),
		fp.WithNmapRule(rules),
		fp.WithActiveMode(true),
		fp.WithDisableWebFingerprint(true),
		fp.WithProbeTimeout(time.Second),
		fp.WithProbesMax(2),
		fp.WithCache(cache),
	}
	out, err := _scanFromTargetStream(in, opts...)
	if err != nil {
		t.Fatal(err)
	}
	var banners []string
	for result := range out {
		if result != nil && result.Fingerprint != nil {
			banners = append(banners, result.Fingerprint.Banner)
		}
	}
	return strings.Join(banners, ",")
}

func listenUDPBanner(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 32)
		for {
			_, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP([]byte("udp-banner"), remote)
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func listenTCPAndUDPBanner(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		_ = ln.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
		_ = udp.Close()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 32)
				_, _ = conn.Read(buf)
				_, _ = conn.Write([]byte("tcp-banner"))
			}(conn)
		}
	}()
	go func() {
		buf := make([]byte, 32)
		for {
			_, remote, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = udp.WriteToUDP([]byte("udp-banner"), remote)
		}
	}()
	return port
}
