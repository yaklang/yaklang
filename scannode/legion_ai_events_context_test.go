package scannode

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
)

func aiEventContextTestListener(t *testing.T, handshake bool) (string, <-chan net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 8)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
			accepted <- conn
			if handshake {
				go func() {
					_, _ = conn.Write([]byte("INFO {\"server_id\":\"context-test\",\"version\":\"2.10.0\",\"proto\":1,\"max_payload\":1048576}\r\n"))
					scanner := bufio.NewScanner(conn)
					for scanner.Scan() {
						if strings.TrimSpace(scanner.Text()) == "PING" {
							_, _ = conn.Write([]byte("PONG\r\n"))
						}
					}
				}()
			}
		}
	}()
	return "nats://" + listener.Addr().String(), accepted
}

func aiEventContextTestPublisher(t *testing.T, url string) *aiSessionEventPublisher {
	t.Helper()
	session := node.SessionState{NodeID: "node-context", SessionID: "session-context", SessionToken: "secret", NATSURL: url, CommandSubject: "context.command", EventSubjectPrefix: "context.event"}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID: session.NodeID, BaseDir: t.TempDir(), EnrollmentToken: "context-enroll",
		PlatformAPIBaseURL: "http://platform.test", TransportClient: &aiBootstrapSessionTransport{session: session},
		HeartbeatInterval: time.Hour, TickerInterval: time.Hour, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	go base.Serve()
	t.Cleanup(base.Shutdown)
	waitForAINodeSession(t, base)
	publisher := newAISessionEventPublisher(base)
	t.Cleanup(publisher.Close)
	return publisher
}

func TestResilienceAISessionClosedPublisherConnectHonorsDeadline(t *testing.T) {
	url, accepted := aiEventContextTestListener(t, false)
	publisher := aiEventContextTestPublisher(t, url)
	publisher.conn = newReconnectingEventTestConn(t)
	publisher.conn.Close()
	publisher.js = &aiFakeJetStreamContext{}
	publisher.natsURL = url
	t.Cleanup(publisher.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- publisher.PublishEvent(ctx, aiSessionCommandRef{SessionID: "analysis", BindEpoch: 1}, 1, "ai_traffic_batch", []byte(`{}`))
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("terminally closed client was not replaced by a real connection attempt")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("initial NATS INFO wait did not return its context deadline: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("closed-client reconnect exceeded the caller's deadline")
	}
}

func TestResilienceAISessionConcurrentReconnectHonorsWaiterDeadline(t *testing.T) {
	url, accepted := aiEventContextTestListener(t, false)
	publisher := newAISessionEventPublisher(&node.NodeBase{})
	t.Cleanup(publisher.Close)
	connecting, cancelConnect := context.WithTimeout(context.Background(), time.Second)
	defer cancelConnect()
	first := make(chan error, 1)
	go func() {
		_, err := publisher.jetStreamForContext(connecting, node.SessionState{NATSURL: url})
		first <- err
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("first caller did not start a real connection attempt")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() {
		_, err := publisher.jetStreamForContext(ctx, node.SessionState{NATSURL: url})
		second <- err
	}()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lock waiter ignored its deadline: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("concurrent reconnect held the waiter past its deadline")
	}
	cancelConnect()
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("in-progress connection ignored cancellation: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("canceled initial handshake remained blocked")
	}
}

func TestResilienceAISessionInitialDeadlineDoesNotCancelAutoReconnect(t *testing.T) {
	url, accepted := aiEventContextTestListener(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dialer := &aiEventInitialDialer{ctx: ctx}
	reconnected := make(chan struct{}, 1)
	conn, err := nats.Connect(url, nats.SetCustomDialer(dialer), nats.SkipHostLookup(),
		nats.ReconnectWait(time.Millisecond), nats.ReconnectJitter(0, 0),
		nats.ReconnectHandler(func(*nats.Conn) { reconnected <- struct{}{} }))
	dialer.detach()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	initial := <-accepted
	cancel()
	if err := conn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("canceling initial context closed the healthy transport: %v", err)
	}
	_ = initial.Close()
	select {
	case <-reconnected:
	case <-time.After(time.Second):
		t.Fatal("autoreconnect retained the canceled initial context")
	}
	if err := conn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("recovered transport cannot exchange PING/PONG: %v", err)
	}
}
