package synscanx

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/synscan"
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
		if calls == 1 {
			return &net.Interface{Name: "eth0"}, net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 10), nil
		}
		return &net.Interface{Name: "eth1"}, net.IPv4(198, 51, 100, 1), net.IPv4(198, 51, 100, 10), nil
	})
	defer restore()
	previous := routeCacheTTL
	routeCacheTTL = time.Millisecond
	defer func() { routeCacheTTL = previous }()

	iface, gateway, src, err := getRoute("203.0.113.8")
	if err != nil || iface.Name != "eth0" || !gateway.Equal(net.IPv4(192, 0, 2, 1)) || !src.Equal(net.IPv4(192, 0, 2, 10)) {
		t.Fatalf("first route = %v %v %v %v", iface, gateway, src, err)
	}
	time.Sleep(30 * time.Millisecond)
	iface, gateway, src, err = getRoute("203.0.113.8")
	if err != nil || iface == nil || iface.Name != "eth1" || !gateway.Equal(net.IPv4(198, 51, 100, 1)) || !src.Equal(net.IPv4(198, 51, 100, 10)) {
		t.Fatalf("expired route = %v %v %v %v", iface, gateway, src, err)
	}
	if calls != 2 {
		t.Fatalf("route lookups = %d, want 2", calls)
	}
}

func TestRouteCacheReplacesExpiredError(t *testing.T) {
	var calls int
	restore := stubRouteLookup(t, func(time.Duration, string) (*net.Interface, net.IP, net.IP, error) {
		calls++
		if calls == 1 {
			return nil, nil, nil, errors.New("route down")
		}
		return &net.Interface{Name: "eth1"}, net.IPv4(198, 51, 100, 1), net.IPv4(198, 51, 100, 10), nil
	})
	defer restore()
	previous := routeCacheTTL
	routeCacheTTL = time.Hour
	defer func() { routeCacheTTL = previous }()

	if _, _, _, err := getRoute("203.0.113.9"); err == nil || err.Error() != "route down" {
		t.Fatalf("first error = %v", err)
	}
	if _, _, _, err := getRoute("203.0.113.9"); err == nil || calls != 1 {
		t.Fatalf("cached error was not reused: calls=%d err=%v", calls, err)
	}
	routeCacheTTL = time.Millisecond
	time.Sleep(30 * time.Millisecond)
	iface, gateway, src, err := getRoute("203.0.113.9")
	if err != nil || iface == nil || iface.Name != "eth1" || !gateway.Equal(net.IPv4(198, 51, 100, 1)) || !src.Equal(net.IPv4(198, 51, 100, 10)) {
		t.Fatalf("refreshed route = %v %v %v %v", iface, gateway, src, err)
	}
}

func TestTCPRetryPolicyStaysValidForShortBudgets(t *testing.T) {
	for _, budget := range []time.Duration{time.Millisecond, 20 * time.Millisecond, 49 * time.Millisecond, 50 * time.Millisecond} {
		policy := tcpRetryPolicy(budget)
		if policy.MaxAttempts < 1 || policy.MaxAttempts > 10 || policy.SendTimeout <= 0 || policy.ResponseTimeout <= 0 || policy.MaxResponseTimeout < policy.ResponseTimeout {
			t.Fatalf("budget %s policy %+v is invalid", budget, policy)
		}
		if budget < 50*time.Millisecond {
			if policy.MaxAttempts != 1 || policy.ResponseTimeout != budget || policy.MaxResponseTimeout != budget {
				t.Fatalf("budget %s policy %+v", budget, policy)
			}
		}
	}
}

func TestScanRouteGroupsReportsTotalStartupFailure(t *testing.T) {
	restore := stubStartRouteGroup(t, func(sample, _, _ string, _ *SynxConfig) (chan *synscan.SynScanResult, error) {
		return nil, errors.New("no such interface " + sample)
	})
	defer restore()
	cfg := NewDefaultConfig()
	ch, err := scanRouteGroups([]routeGroup{
		{sample: "10.0.0.1", hosts: []string{"10.0.0.1"}},
		{sample: "10.0.0.2", hosts: []string{"10.0.0.2"}},
	}, "80", cfg)
	if err == nil || ch != nil || !strings.Contains(err.Error(), "10.0.0.1") || !strings.Contains(err.Error(), "10.0.0.2") {
		t.Fatalf("ch=%v err=%v", ch, err)
	}
}

func TestScanRouteGroupsKeepsPartialResults(t *testing.T) {
	var appended []bool
	restore := stubStartRouteGroup(t, func(sample, _, _ string, cfg *SynxConfig) (chan *synscan.SynScanResult, error) {
		appended = append(appended, cfg.appendOutput)
		if sample == "10.0.0.1" {
			return nil, errors.New("no such interface")
		}
		ch := make(chan *synscan.SynScanResult, 1)
		ch <- &synscan.SynScanResult{Host: sample, Port: 80, Protocol: "tcp"}
		close(ch)
		return ch, nil
	})
	defer restore()
	cfg := NewDefaultConfig()
	cfg.outputFile = filepath.Join(t.TempDir(), "ports.txt")
	ch, err := scanRouteGroups([]routeGroup{
		{sample: "10.0.0.1", hosts: []string{"10.0.0.1"}},
		{sample: "10.0.0.2", hosts: []string{"10.0.0.2"}},
		{sample: "10.0.0.3", hosts: []string{"10.0.0.3"}},
	}, "80", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for result := range ch {
		hosts = append(hosts, result.Host)
	}
	if strings.Join(hosts, ",") != "10.0.0.2,10.0.0.3" {
		t.Fatalf("hosts = %v", hosts)
	}
	if len(appended) != 3 || appended[0] || appended[1] || !appended[2] {
		t.Fatalf("append flags = %v", appended)
	}
	if errs := cfg.RouteGroupErrors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "10.0.0.1") {
		t.Fatalf("route errors = %v", errs)
	}
}

func TestScanRouteGroupsCancelBeforeStart(t *testing.T) {
	called := false
	restore := stubStartRouteGroup(t, func(string, string, string, *SynxConfig) (chan *synscan.SynScanResult, error) {
		called = true
		return nil, nil
	})
	defer restore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := NewDefaultConfig()
	cfg.Ctx = ctx
	ch, err := scanRouteGroups([]routeGroup{{sample: "10.0.0.1", hosts: []string{"10.0.0.1"}}}, "80", cfg)
	if err == nil || ch != nil || called {
		t.Fatalf("ch=%v err=%v called=%v", ch, err, called)
	}
}

func TestScanRouteGroupsCancelUnblocksOpenGroup(t *testing.T) {
	restore := stubStartRouteGroup(t, func(_, _, _ string, cfg *SynxConfig) (chan *synscan.SynScanResult, error) {
		ch := make(chan *synscan.SynScanResult)
		go func() {
			<-cfg.Ctx.Done()
			close(ch)
		}()
		return ch, nil
	})
	defer restore()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := NewDefaultConfig()
	cfg.Ctx = ctx
	ch, err := scanRouteGroups([]routeGroup{{sample: "10.0.0.1", hosts: []string{"10.0.0.1"}}}, "80", cfg)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("canceled scan delivered a result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled scan did not close the result channel")
	}
}

func stubStartRouteGroup(t *testing.T, fn func(sample, targets, ports string, config *SynxConfig) (chan *synscan.SynScanResult, error)) func() {
	t.Helper()
	previous := startRouteGroup
	startRouteGroup = fn
	return func() { startRouteGroup = previous }
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

func TestHostFilterResolvesHyphenatedDomain(t *testing.T) {
	if !looksLikeIPv4Range("10.0.0.1-10.0.0.20") || !looksLikeIPv4Range("1.1.1.1-3") {
		t.Fatal("ip range was not recognized")
	}
	if looksLikeIPv4Range("my-host.example.com") || looksLikeIPv4Range("a-b-c.example.com") {
		t.Fatal("hostname was treated as an ip range")
	}
	previous := lookupHostIPv4
	lookupHostIPv4 = func(host string) []string {
		if host != "my-host.example.com" {
			t.Errorf("resolved %s", host)
		}
		return []string{"203.0.113.9"}
	}
	defer func() { lookupHostIPv4 = previous }()

	filter := hostResultFilter("my-host.example.com,10.0.0.0/30")
	if filter == nil || !filter.Contains("203.0.113.9") || !filter.Contains("10.0.0.1") {
		t.Fatal("hyphenated domain or cidr was not accepted")
	}
	if filter.Contains("203.0.113.10") {
		t.Fatal("unrelated address matched the domain filter")
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
