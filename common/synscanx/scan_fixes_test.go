package synscanx

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestSubmitLowUDPPortsStayUDP(t *testing.T) {
	s := newPlanScanner(t)
	var submitted []string
	s.config.submitTaskCallback = func(addr string) {
		submitted = append(submitted, addr)
	}
	ch, err := s.SubmitTarget("10.0.0.8", "U:1,U:15,U:16,U:53,4096")
	if err != nil {
		t.Fatal(err)
	}
	var modes []string
	for target := range ch {
		modes = append(modes, targetMode(target))
	}
	want := []string{"tcp:4096", "udp:16", "udp:53", "udp:1", "udp:15"}
	if strings.Join(modes, ",") != strings.Join(want, ",") {
		t.Fatalf("modes = %v, want %v", modes, want)
	}
	if strings.Join(submitted, ",") != "10.0.0.8:4096,10.0.0.8:16,10.0.0.8:53,10.0.0.8:1,10.0.0.8:15" {
		t.Fatalf("callback ports = %v", submitted)
	}
}

func TestExcludeLowUDPPortUsesServicePort(t *testing.T) {
	s := newPlanScanner(t)
	s.config.excludePorts = utils.NewPortsFilter("U:1,U:53")
	ports := s.GetNonExcludedPorts("U:1,U:15,U:53,80")
	got := map[string]bool{}
	for _, port := range ports {
		proto, service := utils.ParsePortToProtoPort(port)
		got[proto+":"+strconv.Itoa(service)] = true
	}
	if got["udp:1"] || got["udp:53"] || !got["udp:15"] || !got["tcp:80"] {
		t.Fatalf("remaining = %#v", got)
	}
}

func TestRateLimitHonorsPacketCount(t *testing.T) {
	cfg := NewDefaultConfig()
	WithRateLimit(1, 2000)(cfg)
	pps, inFlight := synPacketRate(cfg)
	if pps != maxSYNPacketsPerSecond {
		t.Fatalf("pps = %d, want the cap %d", pps, maxSYNPacketsPerSecond)
	}
	if inFlight != maxTCPProbeConcurrency {
		t.Fatalf("in flight = %d, want %d", inFlight, maxTCPProbeConcurrency)
	}

	slow := NewDefaultConfig()
	WithRateLimit(1, 50)(slow)
	pps, _ = synPacketRate(slow)
	if pps != 50000 {
		t.Fatalf("50 packets per 1ms = %d pps, want 50000", pps)
	}
}

func TestRouteCacheExpires(t *testing.T) {
	var calls int
	restore := stubRouteLookup(t, func(time.Duration, string) (*net.Interface, net.IP, net.IP, error) {
		calls++
		return &net.Interface{Name: "eth0"}, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 10), nil
	})
	defer restore()
	previous := routeCacheTTL
	routeCacheTTL = time.Millisecond
	defer func() { routeCacheTTL = previous }()

	if _, _, _, err := getRoute("203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, _, _, err := getRoute("203.0.113.8"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("route lookups = %d, want 2", calls)
	}
}

func TestRuntimeIDAndMaxPorts(t *testing.T) {
	cfg := NewDefaultConfig()
	WithRuntimeId("runtime-1")(cfg)
	if cfg.RuntimeId != "runtime-1" || cfg.outputFilePrefix != "" {
		t.Fatalf("runtime id = %q prefix = %q", cfg.RuntimeId, cfg.outputFilePrefix)
	}
	WithMaxOpenPorts(70000)(cfg)
	if cfg.maxOpenPorts != 70000 {
		t.Fatalf("max ports = %d, wrapped", cfg.maxOpenPorts)
	}
	WithMaxOpenPorts(0)(cfg)
	if cfg.maxOpenPorts != 65535 {
		t.Fatalf("non-positive max = %d", cfg.maxOpenPorts)
	}
	WithWaiting(8)(cfg)
	if cfg.waiting != 8*time.Second || cfg.tcpProbeTimeout != 8*time.Second {
		t.Fatalf("wait = %s tcp = %s", cfg.waiting, cfg.tcpProbeTimeout)
	}
}

func TestPartitionGroupsDistinctRoutesAndKeepsLoopback(t *testing.T) {
	restore := stubRouteLookup(t, func(_ time.Duration, sample string) (*net.Interface, net.IP, net.IP, error) {
		if sample == "10.0.0.8" {
			return &net.Interface{Name: "lan"}, net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2), nil
		}
		return &net.Interface{Name: "wan"}, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 10), nil
	})
	defer restore()
	groups := partitionScanTargets("10.0.0.8,203.0.113.9,127.0.0.1,2001:db8::1")
	if len(groups) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[0].sample != "10.0.0.8" || !containsAll(groups[0].hosts, "10.0.0.8", "127.0.0.1") {
		t.Fatalf("first group = %#v", groups[0])
	}
	if groups[1].sample != "203.0.113.9" || strings.Join(groups[1].hosts, ",") != "203.0.113.9" {
		t.Fatalf("second group = %#v", groups[1])
	}
}

func TestOutputFileTruncatesOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ports.txt")
	write := func(appendOutput bool, line string) {
		t.Helper()
		f, err := os.OpenFile(path, outputFileFlags(appendOutput), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write(false, "old\n")
	write(false, "new\n")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("file = %q", got)
	}
	write(true, "more\n")
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\nmore\n" {
		t.Fatalf("appended file = %q", got)
	}
}

func targetMode(target *SynxTarget) string {
	name := "tcp"
	if target.Mode == UDP {
		name = "udp"
	}
	return name + ":" + strconv.Itoa(target.Port)
}

func containsAll(hosts []string, want ...string) bool {
	seen := map[string]bool{}
	for _, host := range hosts {
		seen[host] = true
	}
	for _, host := range want {
		if !seen[host] {
			return false
		}
	}
	return true
}
