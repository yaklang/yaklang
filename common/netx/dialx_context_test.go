package netx

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDialContextCancelledBeforeConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DialContextWithoutProxy(ctx, "127.0.0.1:1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestDialContextDNSBudget(t *testing.T) {
	for _, respond := range []bool{false, true} {
		t.Run(map[bool]string{false: "stalled", true: "resolved"}[respond], func(t *testing.T) {
			dnsServer, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer dnsServer.Close()
			_ = dnsServer.SetReadDeadline(time.Now().Add(2 * time.Second))
			observed := make(chan error, 1)
			go func() {
				buffer := make([]byte, 512)
				n, peer, err := dnsServer.ReadFrom(buffer)
				if err == nil && respond {
					var request dns.Msg
					err = request.Unpack(buffer[:n])
					if err == nil {
						reply := new(dns.Msg).SetReply(&request)
						reply.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: net.IPv4(127, 0, 0, 1)}}
						var raw []byte
						raw, err = reply.Pack()
						if err == nil {
							_, err = dnsServer.WriteTo(raw, peer)
						}
					}
				}
				observed <- err
			}()
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			_, port, _ := net.SplitHostPort(target.Addr().String())
			budget := 80 * time.Millisecond
			if respond {
				budget = time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			finished := make(chan struct{})
			start := time.Now()
			conn, err := DialX(net.JoinHostPort("oracle-probe-budget.invalid", port), DialX_WithContext(ctx), DialX_WithDisableProxy(true), DialX_WithDNSOptions(
				WithDNSServers(dnsServer.LocalAddr().String()), WithDNSDisableSystemResolver(true), WithDNSNoCache(true), WithDNSPreferDoH(false), WithDNSFallbackDoH(false), WithDNSFallbackTCP(false), WithDNSRetryTimes(2), WithDNSOnFinished(func() { close(finished) }),
			))
			if respond {
				if err != nil {
					t.Fatal(err)
				}
				conn.Close()
			} else if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
				t.Fatalf("DNS ignored budget: elapsed=%v err=%v", time.Since(start), err)
			}
			if err := <-observed; err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("DNS worker retained its socket/retry after cancellation")
			}
		})
	}
}

func TestDialContextCancelsRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := DialX("127.0.0.1:12345", DialX_WithContext(ctx), DialX_WithDisableProxy(true), DialX_WithTimeoutRetryWait(time.Second), DialX_WithDialer(func(time.Duration, string) (net.Conn, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}))
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("retry wait ignored cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("dial continued after cancel")
	}
}

func TestDialContextExpiredBudget(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := DialContextWithoutProxy(ctx, "127.0.0.1:1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestDialContextTCPDNSBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closed := make(chan error, 1)
	probeReturned := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		// The resolver half-closes its write side after sending the question;
		// write periodically to observe closure of the read side on cancellation.
		_, err = io.Copy(io.Discard, conn)
		<-probeReturned
		if err == nil {
			for err == nil {
				_, err = conn.Write([]byte{0})
				time.Sleep(10 * time.Millisecond)
			}
		}
		closed <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	finished := make(chan struct{})
	ip := lookupFirstWithContext(ctx, "oracle-tcp-budget.invalid", WithDNSServers(listener.Addr().String()), WithDNSDisableSystemResolver(true), WithDNSNoCache(true), WithDNSPreferTCP(true), WithDNSFallbackDoH(false), WithDNSPreferDoH(false), WithDNSRetryTimes(1), WithDNSOnFinished(func() { close(finished) }))
	close(probeReturned)
	if ip != "" || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("unexpected DNS result: ip=%q err=%v", ip, ctx.Err())
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("TCP DNS worker ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("TCP DNS socket remained open")
	}
}
