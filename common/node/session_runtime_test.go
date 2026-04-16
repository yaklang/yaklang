package node

import (
	"context"
	"testing"
	"time"
)

type stubBootstrapTransport struct {
	request BootstrapRequest
	session SessionState
	calls   int
}

func (s *stubBootstrapTransport) Bootstrap(
	_ context.Context,
	request BootstrapRequest,
) (SessionState, error) {
	s.calls++
	s.request = request
	return s.session, nil
}

func (s *stubBootstrapTransport) Heartbeat(context.Context, SessionState, HeartbeatRequest) error {
	return nil
}

func (s *stubBootstrapTransport) Shutdown(context.Context, SessionState, ShutdownRequest) error {
	return nil
}

type staticHostInfoProvider struct {
	info HostInfo
}

func (s staticHostInfoProvider) Snapshot() HostInfo {
	return s.info
}

func TestNodeBaseBootstrapSessionIncludesHostInfo(t *testing.T) {
	t.Parallel()

	transport := &stubBootstrapTransport{
		session: SessionState{
			SessionID:    "session-1",
			SessionToken: "token-1",
		},
	}
	node := &NodeBase{
		rootCtx:           context.Background(),
		NodeId:            "node-1",
		NodeType:          "scanner-agent",
		enrollmentToken:   "enroll-1",
		version:           "dev",
		labels:            map[string]string{"zone": "cn"},
		capabilityKeys:    []string{"yak.execute", "hids"},
		requestTimeout:    time.Second,
		transport:         transport,
		heartbeatInterval: 30 * time.Second,
		hostInfoProvider: staticHostInfoProvider{info: HostInfo{
			Hostname:        "host-a",
			PrimaryIP:       "10.0.0.5",
			IPAddresses:     []string{"10.0.0.5", "192.168.1.7"},
			OperatingSystem: "linux",
			Architecture:    "amd64",
		}},
	}

	if err := node.bootstrapSession(); err != nil {
		t.Fatalf("bootstrapSession: %v", err)
	}

	if transport.calls != 1 {
		t.Fatalf("unexpected bootstrap call count: %d", transport.calls)
	}
	if transport.request.Hostname != "host-a" {
		t.Fatalf("unexpected hostname: %q", transport.request.Hostname)
	}
	if transport.request.PrimaryIP != "10.0.0.5" {
		t.Fatalf("unexpected primary ip: %q", transport.request.PrimaryIP)
	}
	if len(transport.request.IPAddresses) != 2 {
		t.Fatalf("unexpected ip_addresses: %#v", transport.request.IPAddresses)
	}
	if transport.request.OperatingSystem != "linux" {
		t.Fatalf("unexpected operating system: %q", transport.request.OperatingSystem)
	}
	if transport.request.Architecture != "amd64" {
		t.Fatalf("unexpected architecture: %q", transport.request.Architecture)
	}
}
