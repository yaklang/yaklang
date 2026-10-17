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
		in    synscan.SynScanResult
		proto string
		port  int
	}{
		{synscan.SynScanResult{Port: 53, Protocol: "udp"}, "udp", 53},
		{synscan.SynScanResult{Port: 53, Protocol: "UDP"}, "udp", 53},
		{synscan.SynScanResult{Port: encoded}, "udp", 53},
		{synscan.SynScanResult{Port: encoded, Protocol: "udp"}, "udp", 53},
		{synscan.SynScanResult{Port: 80}, "tcp", 80},
		{synscan.SynScanResult{Port: 80, Protocol: "tcp"}, "tcp", 80},
	}
	for _, tc := range cases {
		proto, port := synScanProbeTarget(&tc.in)
		if proto != tc.proto || port != tc.port {
			t.Fatalf("%+v -> %s %d", tc.in, proto, port)
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
