package coreplugin_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/grpc"
)

// This fixture implements the actual reverse-service protocol. Target HTTP
// callbacks require sockets; the Yak gRPC transport itself stays in memory.
// Only observed callbacks produce events, and no query falls back to the public
// reverse service. All listeners and token state belong to this suite.
type localDNSLog struct {
	tpb.UnimplementedDNSLogServer
	mu        sync.Mutex
	events    map[string]*tpb.DNSLogEvent
	domains   map[string]string
	callbacks []*httptest.Server
	grpc      *grpc.Server
	listener  net.Listener
	previous  string
	closeOnce sync.Once
	done      chan struct{}
}

func startLocalDNSLog(options ...grpc.ServerOption) *localDNSLog {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	d := &localDNSLog{events: make(map[string]*tpb.DNSLogEvent), domains: make(map[string]string), grpc: grpc.NewServer(append(options, grpc.WaitForHandlers(true))...), done: make(chan struct{}), listener: lis, previous: consts.GetDefaultPublicReverseServer()}
	tpb.RegisterDNSLogServer(d.grpc, d)
	go func() { defer close(d.done); _ = d.grpc.Serve(lis) }()
	consts.SetDefaultPublicReverseServer(lis.Addr().String())
	return d
}
func (d *localDNSLog) RequireDomain(ctx context.Context, _ *tpb.RequireDomainParams) (*tpb.RequireDomainResponse, error) {
	token := utils.RandStringBytes(24)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.events[token] = &tpb.DNSLogEvent{Token: token, Domain: r.Host, RemoteAddr: r.RemoteAddr, Type: "A"}
		d.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	d.mu.Lock()
	d.callbacks = append(d.callbacks, callback)
	d.domains[strings.TrimPrefix(callback.URL, "http://")] = token
	d.mu.Unlock()
	return &tpb.RequireDomainResponse{Domain: strings.TrimPrefix(callback.URL, "http://"), Token: token}, nil
}
func (d *localDNSLog) QueryExistedDNSLog(ctx context.Context, req *tpb.QueryExistedDNSLogParams) (*tpb.QueryExistedDNSLogResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	resp := &tpb.QueryExistedDNSLogResponse{}
	if event := d.events[req.Token]; event != nil {
		resp.Events = []*tpb.DNSLogEvent{event}
	}
	return resp, nil
}

// ObserveDomain is the vulnerable target's DNS resolver. It records only an
// allocated domain that the Fastjson parser actually attempted to resolve.
func (d *localDNSLog) ObserveDomain(domain string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if token, ok := d.domains[domain]; ok {
		d.events[token] = &tpb.DNSLogEvent{Token: token, Domain: domain, RemoteAddr: "127.0.0.1", Type: "A"}
	}
}

func (d *localDNSLog) Close() {
	d.closeOnce.Do(func() {
		// RequireDomain may allocate callbacks after its client disconnects.
		// Join those handlers before traversing and closing their listeners.
		d.grpc.Stop()
		_ = d.listener.Close()
		<-d.done
		for _, callback := range d.callbacks {
			callback.Close()
		}
		consts.SetDefaultPublicReverseServer(d.previous)
	})
}

func TestLocalDNSLog(t *testing.T) {
	t.Run("close waits for domain allocation", testDNSLogCloseWaitsForHandlers)
	addr := consts.GetDefaultPublicReverseServer()
	domain, token, _, err := cybertunnel.RequireDNSLogDomainByRemote(addr, "")
	require.NoError(t, err)
	events, err := cybertunnel.QueryExistedDNSLogEventsEx(addr, token, "", 1)
	require.NoError(t, err)
	require.Empty(t, events)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://" + domain)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	events, err = cybertunnel.QueryExistedDNSLogEventsEx(addr, token, "", 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, token, events[0].Token)
	events, err = cybertunnel.QueryExistedDNSLogEventsEx(addr, "unknown-token", "", 1)
	require.NoError(t, err)
	require.Empty(t, events)
	_, other, _, err := cybertunnel.RequireDNSLogDomainByRemote(addr, "")
	require.NoError(t, err)
	events, err = cybertunnel.QueryExistedDNSLogEventsEx(addr, other, "", 1)
	require.NoError(t, err)
	require.Empty(t, events)
}

func testDNSLogCloseWaitsForHandlers(t *testing.T) {
	const requests = 8
	entered := make(chan struct{}, requests)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	d := startLocalDNSLog(grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		entered <- struct{}{}
		<-release
		return handler(ctx, req)
	}))
	t.Cleanup(d.Close)
	t.Cleanup(unblock)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, d.listener.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
	require.NoError(t, err)
	defer conn.Close()
	client := tpb.NewDNSLogClient(conn)
	replies := make(chan error, requests)
	for i := 0; i < requests; i++ {
		go func() { _, err := client.RequireDomain(ctx, &tpb.RequireDomainParams{}); replies <- err }()
	}
	for i := 0; i < requests; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("RPC did not enter handler")
		}
	}
	closed := make(chan struct{})
	go func() { d.Close(); close(closed) }()
	// Stop cancels the active RPCs while their handlers are still behind the gate.
	for i := 0; i < requests; i++ {
		select {
		case err := <-replies:
			require.Error(t, err)
		case <-ctx.Done():
			t.Fatal("Stop did not cancel RPCs")
		}
	}
	select {
	case <-closed:
		t.Error("Close returned before callback allocation finished")
	case <-time.After(100 * time.Millisecond): // Bound the assertion; channels establish the window.
	}
	unblock()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("Close did not join handlers")
	}
	require.Len(t, d.callbacks, requests)
	for _, callback := range d.callbacks {
		conn, err := net.DialTimeout("tcp", callback.Listener.Addr().String(), time.Second)
		if conn != nil {
			conn.Close()
		}
		require.Error(t, err, "callback listener leaked: %s", callback.URL)
	}
}
