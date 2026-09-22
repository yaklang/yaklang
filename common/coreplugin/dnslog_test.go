package coreplugin_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
}

func startLocalDNSLog() *localDNSLog {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	d := &localDNSLog{events: make(map[string]*tpb.DNSLogEvent), domains: make(map[string]string), grpc: grpc.NewServer(), listener: lis, previous: consts.GetDefaultPublicReverseServer()}
	tpb.RegisterDNSLogServer(d.grpc, d)
	go d.grpc.Serve(lis)
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
	d.grpc.Stop()
	_ = d.listener.Close()
	for _, callback := range d.callbacks {
		callback.Close()
	}
	consts.SetDefaultPublicReverseServer(d.previous)
}

func TestLocalDNSLog(t *testing.T) {
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
