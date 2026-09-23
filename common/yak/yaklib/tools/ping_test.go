package tools

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

func collectPingResults(t *testing.T, results chan *pingutil.PingResult) []*pingutil.PingResult {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var collected []*pingutil.PingResult
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return collected
			}
			collected = append(collected, r)
		case <-timer.C:
			t.Fatal("ping scan did not close")
			return nil
		}
	}
}

func TestPingScanNonPositiveConcurrency(t *testing.T) {
	for _, concurrent := range []int{-1, 0, 1, 10} {
		results := collectPingResults(t, _pingScan("192.0.2.1,192.0.2.2", _pingConfigOpt_skipped(true), _pingConfigOpt_concurrent(concurrent), WithPingCtx(nil)))
		if len(results) != 2 {
			t.Fatalf("concurrency %d: got %d results", concurrent, len(results))
		}
		for _, r := range results {
			if !r.Ok || r.Reason != "skipped" {
				t.Fatalf("unexpected result: %+v", r)
			}
		}
	}
}

func TestPingScanCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := collectPingResults(t, _pingScan("192.0.2.0/24", WithPingCtx(ctx), _pingConfigOpt_skipped(true)))
	if len(results) != 0 {
		t.Fatalf("canceled scan returned %d results", len(results))
	}
}

func TestPingScanCancelBlockedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	results := _pingScan("127.0.0.1,127.0.0.2", WithPingCtx(ctx), _pingConfigOpt_concurrent(1), _pingConfigOpt_onResult(func(*pingutil.PingResult) { started <- struct{}{} }))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("probe did not finish")
	}
	cancel()
	collectPingResults(t, results)
}

func TestPingSingleSkipAndExclude(t *testing.T) {
	for _, opt := range []PingConfigOpt{_pingConfigOpt_skipped(true), _pingConfigOpt_excludeHosts("192.0.2.1")} {
		result := _ping("192.0.2.1", opt)
		if !result.Ok || result.Reason != "skipped" {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
}

func TestPingScanLoopback(t *testing.T) {
	results := collectPingResults(t, _pingScan("127.0.0.1,127.0.0.2"))
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	for _, result := range results {
		if !result.Ok {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
}

func TestPingCanceledDNS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := _ping("pingscan.invalid", WithPingCtx(ctx))
	if result.Ok || result.Reason != context.Canceled.Error() {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPingCustomDNS(t *testing.T) {
	old := netx.GetDefaultOptions()
	defer netx.SetDefaultDNSOptions(old...)
	netx.SetDefaultDNSOptions(netx.WithDNSDisableSystemResolver(true), netx.WithDNSFallbackDoH(false), netx.WithDNSPreferDoH(false), netx.WithDNSNoCache(true))
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	server := &dns.Server{PacketConn: conn, NotifyStartedFunc: func() { close(started) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(r)
		for _, q := range r.Question {
			if q.Qtype == dns.TypeA {
				reply.Answer = append(reply.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP("127.0.0.1")})
			}
		}
		_ = w.WriteMsg(reply)
	})}
	go server.ActivateAndServe()
	<-started
	defer server.Shutdown()
	result := _ping("pingscan-dns.invalid", _pingConfigOpt_dnsServers(conn.LocalAddr().String()), _pingConfigOpt_withDNSTimeout(1))
	if !result.Ok || result.IP != "pingscan-dns.invalid" || result.Reason != "loopback" {
		t.Fatalf("resolved address was not used: %+v", result)
	}
}

func TestPingScanCallbackIdentity(t *testing.T) {
	callbacks := make(chan string, 1)
	results := collectPingResults(t, _pingScan("http://127.0.0.1:8080", _pingConfigOpt_skipped(true), _pingConfigOpt_onResult(func(r *pingutil.PingResult) { callbacks <- r.IP })))
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	select {
	case target := <-callbacks:
		if target != results[0].IP {
			t.Fatalf("callback %q differs from output %q", target, results[0].IP)
		}
	default:
		t.Fatal("missing skipped result callback")
	}
}
