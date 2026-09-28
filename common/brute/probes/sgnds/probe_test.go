package sgnds

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// mockBridge 是一个最小 TCP 模拟器，复现 Java 代理的文本协议。
type mockBridge struct {
	addr      string
	validUser string
	validPass string
	listener  net.Listener
	closed    chan struct{}
}

func newMockBridge(t *testing.T, user, pass string) *mockBridge {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &mockBridge{
		addr:      l.Addr().String(),
		validUser: user,
		validPass: pass,
		listener:  l,
		closed:    make(chan struct{}),
	}
	go m.serve()
	return m
}

func (m *mockBridge) serve() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			select {
			case <-m.closed:
				return
			default:
				continue
			}
		}
		go m.handle(conn)
	}
}

func (m *mockBridge) handle(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		line := strings.TrimSpace(string(buf[:n]))
		resp := m.reply(line)
		_, _ = conn.Write([]byte(resp + "\n"))
	}
}

func (m *mockBridge) reply(line string) string {
	if line == "action=ping" {
		return "ok=true&pong=true"
	}
	if strings.HasPrefix(line, "action=open") {
		if strings.Contains(line, "user="+m.validUser) && strings.Contains(line, "password="+m.validPass) {
			return "ok=true&connId=test-conn-123"
		}
		return "ok=false&error=invalid+username+or+password"
	}
	if strings.HasPrefix(line, "action=query") {
		return "ok=true&columns=DUMMY&rows=1"
	}
	return "ok=false&error=unknown+action"
}

func (m *mockBridge) stop() {
	close(m.closed)
	_ = m.listener.Close()
}

func TestSGNDSAuthSuccess(t *testing.T) {
	m := newMockBridge(t, "admin", "admin123")
	defer m.stop()

	prober := Prober{BridgeAddr: m.addr, AppName: "yakit_test"}
	target := core.Target{Host: "170.20.8.223", Port: 18600}
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "admin", Password: "admin123"},
		core.Options{Timeout: 3 * time.Second})
	if res.Outcome != core.OutcomeAuthSuccess {
		t.Fatalf("want auth success, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestSGNDSAuthFailed(t *testing.T) {
	m := newMockBridge(t, "admin", "admin123")
	defer m.stop()

	prober := Prober{BridgeAddr: m.addr, AppName: "yakit_test"}
	target := core.Target{Host: "170.20.8.223", Port: 18600}
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "admin", Password: "wrong"},
		core.Options{Timeout: 3 * time.Second})
	if res.Outcome != core.OutcomeAuthFailed {
		t.Fatalf("want auth failed, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestSGNDSBridgeUnreachable(t *testing.T) {
	// 找一个未监听端口
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	prober := Prober{BridgeAddr: addr, AppName: "yakit_test"}
	target := core.Target{Host: "170.20.8.223", Port: 18600}
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "admin", Password: "admin123"},
		core.Options{Timeout: 1 * time.Second})
	if res.Outcome != core.OutcomeTargetUnavailable {
		t.Fatalf("want target unavailable, got %v (%s)", res.Outcome, res.ErrDetail)
	}
}

func TestBridgeClientPing(t *testing.T) {
	m := newMockBridge(t, "u", "p")
	defer m.stop()

	c := NewBridgeClient(m.addr, 3*time.Second)
	ok, err := c.Ping(context.Background())
	if err != nil || !ok {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestBridgeClientQuery(t *testing.T) {
	m := newMockBridge(t, "u", "p")
	defer m.stop()

	c := NewBridgeClient(m.addr, 3*time.Second)
	res, err := c.Query(context.Background(), "test-conn-123", "SELECT 1 FROM DUAL", nil)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(res.Columns) != 1 || res.Columns[0] != "DUMMY" {
		t.Fatalf("unexpected columns: %v", res.Columns)
	}
	if len(res.Rows) != 1 || len(res.Rows[0]) != 1 || res.Rows[0][0] != "1" {
		t.Fatalf("unexpected rows: %v", res.Rows)
	}
}

func TestNoCredentialLeak(t *testing.T) {
	m := newMockBridge(t, "u", "p")
	defer m.stop()

	sentinel := "SENTINEL-PASSw0rd"
	prober := Prober{BridgeAddr: m.addr, AppName: "yakit_test"}
	target := core.Target{Host: "170.20.8.223", Port: 18600}
	res := prober.Probe(context.Background(), target,
		core.Credential{Username: "u", Password: sentinel},
		core.Options{Timeout: 3 * time.Second})
	if strings.Contains(res.String(), sentinel) {
		t.Fatalf("result leaks sentinel: %s", res.String())
	}
	if strings.Contains(fmt.Sprintf("%v", res.ErrDetail), sentinel) {
		t.Fatalf("err detail leaks sentinel: %s", res.ErrDetail)
	}
}
