package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

func TestScanFromPingUsesHostFromHostPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
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

	rules, err := fp.ParseNmapServiceProbeToRuleMap([]byte(fmt.Sprintf(`Probe TCP LocalFixture q|probe|
rarity 1
ports %d
match local-fixture m|^fixture-banner$|
`, port)))
	if err != nil {
		t.Fatal(err)
	}

	pingCh := make(chan *pingutil.PingResult, 1)
	pingCh <- &pingutil.PingResult{IP: net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), Ok: true}
	close(pingCh)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := _scanFromPingUtils(
		pingCh,
		fmt.Sprintf("%d", port),
		fp.WithCtx(ctx),
		fp.WithNmapRule(rules),
		fp.WithTransportProtos(fp.TCP),
		fp.WithActiveMode(true),
		fp.WithDisableWebFingerprint(true),
		fp.WithProbeTimeout(500*time.Millisecond),
		fp.WithProbesMax(1),
	)
	if err != nil {
		t.Fatal(err)
	}

	sawOpen := false
	for result := range out {
		if result == nil {
			continue
		}
		if strings.Contains(result.Target, ":") {
			t.Fatalf("ping target kept the port in the host: %s", result.Target)
		}
		if result.Port == port && result.IsOpen() && strings.Contains(result.GetServiceName(), "local-fixture") {
			sawOpen = true
		}
	}
	if !sawOpen {
		t.Fatal("expected the embedded host:port ping result to be scanned as an open service")
	}
}

func TestScanFromPingCancelWhileIdle(t *testing.T) {
	rules, err := fp.ParseNmapServiceProbeToRuleMap([]byte(`Probe TCP LocalFixture q|probe|
rarity 1
ports 80
match local-fixture m|^fixture-banner$|
`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pingCh := make(chan *pingutil.PingResult)
	out, err := _scanFromPingUtils(
		pingCh,
		"80",
		fp.WithCtx(ctx),
		fp.WithNmapRule(rules),
		fp.WithTransportProtos(fp.TCP),
		fp.WithActiveMode(false),
		fp.WithDisableWebFingerprint(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range out {
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ScanFromPing stayed blocked on an idle ping channel after cancel")
	}
}
