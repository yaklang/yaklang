package scannode

import (
	"context"
	"fmt"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLegionDiscoveryRejectsUnsafeTargets(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1", "169.254.169.254", "100.100.100.100", "metadata.google.internal", "https://example.com", "example.com:443", "*.example.com", "a..example.com"} {
		if _, err := normalizeLegionDiscoveryHost(host); err == nil {
			t.Errorf("accepted %q", host)
		}
	}
	lookup := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.1")}, nil }
	if _, err := newLegionForgeDiscoveryRuntime(context.Background(), "example.com", []int{80}, lookup); err == nil {
		t.Fatal("private DNS accepted")
	}
	if _, err := newLegionForgeDiscoveryRuntime(context.Background(), "10.0.0.1", []int{80}, lookup); err != nil {
		t.Fatal(err)
	}
	lookup = func(context.Context, string) ([]net.IP, error) { return make([]net.IP, 9), nil }
	if _, err := newLegionForgeDiscoveryRuntime(context.Background(), "example.com", []int{80}, lookup); err == nil {
		t.Fatal("address budget ignored")
	}
	for _, ports := range []string{"", "0", "65536", "80,80", "1-65535", strings.Repeat("80,", 32) + "81"} {
		if _, err := parseLegionDiscoveryPorts(ports); err == nil {
			t.Errorf("accepted ports %q", ports)
		}
	}
}

func TestLegionDiscoveryPinnedConnectAndNoPayload(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan error, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			received <- err
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 1)
		n, err := c.Read(b)
		if n != 0 || err != io.EOF {
			received <- fmt.Errorf("unexpected payload/read: %d %v", n, err)
		} else {
			received <- nil
		}
	}()
	lookups := 0
	r, err := newLegionForgeDiscoveryRuntime(context.Background(), "example.com", []int{443}, func(context.Context, string) ([]net.IP, error) {
		lookups++
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:443" || network != "tcp" {
			t.Errorf("unpinned dial: %s %s", network, address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
	}
	result, err := r.execute(context.Background(), "tcp_connect_scan", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result["observations"].([]map[string]any)[0]["connected"].(bool) {
		t.Fatal(result)
	}
	if err := <-received; err != nil {
		t.Fatal(err)
	}
	if lookups != 1 {
		t.Fatal("DNS not pinned")
	}
	if len(r.applicationHTTPMaterialReferences()) != 1 {
		t.Fatal("missing provenance")
	}
	for _, args := range []map[string]any{{"host": "evil.example"}, {"ports": "22"}, {"label": "other"}} {
		if _, err := r.execute(context.Background(), "tcp_connect_scan", args); err == nil {
			t.Fatal("scope override accepted")
		}
	}
}

func TestLegionDiscoveryDNSBoundsCancellationAndBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := []string{}
	r, err := newLegionForgeDiscoveryRuntime(ctx, "example.com", []int{80}, func(ctx context.Context, host string) ([]net.IP, error) {
		hosts = append(hosts, host)
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.execute(ctx, "dns_lookup", map[string]any{"label": "www"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hosts, []string{"example.com", "www.example.com"}) {
		t.Fatal(hosts)
	}
	for _, label := range []string{"evil.com", "../other", strings.Repeat("a", 64), "-x"} {
		if _, err := r.execute(ctx, "dns_lookup", map[string]any{"label": label}); err == nil {
			t.Fatal("bad label accepted")
		}
	}
	r.remaining = 0
	if _, err := r.execute(ctx, "dns_lookup", nil); err == nil {
		t.Fatal("budget ignored")
	}
	r.remaining = 100
	r.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if _, err := r.execute(context.Background(), "tcp_connect_scan", nil); err == nil {
		t.Fatal("parent cancellation ignored")
	}
}

func TestLegionForgeNewProfilesExactAndCompatible(t *testing.T) {
	for _, profile := range []string{legionForgeDiscoveryProfile, legionForgeEvidenceProfile} {
		r := testLegionContextForgeRelease(t)
		r.CapabilityProfile = profile
		if profile == legionForgeDiscoveryProfile {
			r.DeclaredToolNames = append([]string(nil), legionForgeDiscoveryTools...)
			r.Parameters = []*aiv1.ContextForgeParameter{{Key: "target-host", ValueKind: "string", Value: "example.com"}}
		} else {
			r.DeclaredToolNames = append([]string(nil), legionForgeEvidenceTools...)
			r.Parameters = []*aiv1.ContextForgeParameter{{Key: "evidence", ValueKind: "resource", Value: "inputs/test/file.pcap"}}
		}
		rehashLegionContextForgeRelease(t, r)
		if err := validateContextForgeRelease(r); err != nil {
			t.Fatal(err)
		}
		r.DeclaredToolNames = append(r.DeclaredToolNames, "web_search")
		rehashLegionContextForgeRelease(t, r)
		if err := validateContextForgeRelease(r); err == nil {
			t.Fatal("extra tool accepted")
		}
	}
	r := testLegionContextForgeRelease(t)
	r.CapabilityProfile = legionForgeDiscoveryProfile
	if _, _, err := legionForgeDiscoveryParameters(r); err == nil {
		t.Fatal("missing target accepted")
	}
	r.Parameters = []*aiv1.ContextForgeParameter{{Key: "target-host", ValueKind: "string", Value: "example.com"}}
	_, ports, err := legionForgeDiscoveryParameters(r)
	if err != nil || !reflect.DeepEqual(ports, []int{80, 443}) {
		t.Fatalf("default ports: %v %v", ports, err)
	}
	if !reflect.DeepEqual(legionForgeReportTools, []string{"parse_office_to_text", "query_file_meta", "read_file", "read_file_lines"}) {
		t.Fatal("old report profile changed")
	}
}

func TestLegionDiscoveryRootNXDOMAINAllowsScopedSubdomain(t *testing.T) {
	r, err := newLegionForgeDiscoveryRuntime(context.Background(), "example.com", []int{443}, func(_ context.Context, host string) ([]net.IP, error) {
		if host == "example.com" {
			return nil, &net.DNSError{Name: host, IsNotFound: true, Err: "no such host"}
		}
		if host != "www.example.com" {
			t.Fatalf("escaped DNS scope: %s", host)
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("NXDOMAIN must not dial")
		return nil, nil
	}
	root, err := r.execute(context.Background(), "dns_lookup", nil)
	if err != nil || root["status"] != "not_found" {
		t.Fatalf("root: %v %v", root, err)
	}
	tcp, err := r.execute(context.Background(), "tcp_connect_scan", nil)
	if err != nil || tcp["status"] != "no_resolved_addresses" {
		t.Fatalf("tcp: %v %v", tcp, err)
	}
	sub, err := r.execute(context.Background(), "dns_lookup", map[string]any{"label": "www"})
	if err != nil || sub["status"] != "resolved" {
		t.Fatalf("sub: %v %v", sub, err)
	}
	if len(r.applicationHTTPMaterialReferences()) != 3 {
		t.Fatal("missing actual negative/positive evidence")
	}
}
