//go:build linux

package hids

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	libaudit "github.com/elastic/go-libaudit/v2"
	"github.com/elastic/go-libaudit/v2/auparse"
)

type auditTestReceiver struct {
	messages []*libaudit.RawAuditMessage
	err      error
	closed   chan struct{}
	received chan struct{}
	once     sync.Once
}

func (r *auditTestReceiver) Receive(nonblocking bool) (*libaudit.RawAuditMessage, error) {
	if !nonblocking {
		panic("blocking audit receive prevents cancellation")
	}
	if r.received != nil {
		r.once.Do(func() { close(r.received) })
	}
	if len(r.messages) == 0 {
		return nil, r.err
	}
	msg := r.messages[0]
	r.messages = r.messages[1:]
	return msg, nil
}
func (r *auditTestReceiver) Close() error { close(r.closed); return nil }
func auditRecord(typ auparse.AuditMessageType, raw string) *libaudit.RawAuditMessage {
	return &libaudit.RawAuditMessage{Type: typ, Data: []byte(raw)}
}
func TestAuditV2Reassembly(t *testing.T) {
	commands := make(chan *CommandEvent, 1)
	logins := make(chan *LoginEvent, 1)
	m, _ := NewAuditMonitor(WithOnCommandEvent(func(e *CommandEvent) { commands <- e }), WithOnLoginEvent(func(e *LoginEvent) { logins <- e }))
	client := &auditTestReceiver{err: io.EOF, closed: make(chan struct{}), messages: []*libaudit.RawAuditMessage{
		auditRecord(auparse.AUDIT_SYSCALL, `audit(1600000000.123:10): arch=c000003e syscall=59 success=yes exit=0 pid=123 ppid=12 uid=4294967295 auid=4294967295 comm="echo" exe="/usr/bin/echo"`),
		auditRecord(auparse.AUDIT_EXECVE, `audit(1600000000.123:10): argc=2 a0="echo" a1="hello"`),
		auditRecord(auparse.AUDIT_CWD, `audit(1600000000.123:10): cwd="/tmp"`),
		auditRecord(auparse.AUDIT_EOE, `audit(1600000000.123:10):`),
		auditRecord(auparse.AUDIT_USER_LOGIN, `audit(1600000001.123:11): pid=5 uid=4294967295 auid=4294967295 msg='op=login acct="tester" exe="/usr/sbin/sshd" hostname=? addr=192.0.2.1 terminal=ssh res=success'`),
	}}
	runAuditReceiveLoop(client, make(chan struct{}), &auditStream{monitor: m})
	select {
	case <-client.closed:
	default:
		t.Fatal("client not closed on EOF")
	}
	select {
	case e := <-commands:
		if e.PID != 123 || e.PPID != 12 || e.WorkingDir != "/tmp" || e.CommandLine != "echo hello" || !reflect.DeepEqual(e.Arguments, []string{"echo", "hello"}) {
			t.Fatalf("command: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no reconstructed command")
	}
	select {
	case e := <-logins:
		if e.Username != "tester" || e.RemoteIP != "192.0.2.1" || e.LoginMethod != "ssh" || e.Result != "success" {
			t.Fatalf("login: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no login")
	}
	if parseCommandEvent(nil) != nil || parseLoginEvent(nil) != nil {
		t.Fatal("empty records must not produce events")
	}
	msg, err := auparse.Parse(auparse.AUDIT_EXECVE, `audit(1600000000.123:12): argc=1 a0="echo"`)
	if err != nil {
		t.Fatal(err)
	}
	if parseCommandEvent([]*auparse.AuditMessage{msg}) != nil {
		t.Fatal("partial command accepted")
	}
}
func TestAuditV2Cancellation(t *testing.T) {
	m, _ := NewAuditMonitor()
	m.running = true
	client := &auditTestReceiver{err: errors.New("EAGAIN"), closed: make(chan struct{}), received: make(chan struct{})}
	done := make(chan struct{})
	stop := m.stopCh
	go func() { runAuditReceiveLoop(client, stop, &auditStream{monitor: m}); close(done) }()
	<-client.received
	m.Stop()
	m.Stop()
	if m.IsRunning() {
		t.Fatal("monitor still running")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked without records")
	}
	select {
	case <-client.closed:
	default:
		t.Fatal("client not closed")
	}
}

type auditTestStream struct {
	completed int
	lost      int
}

func (s *auditTestStream) ReassemblyComplete(msgs []*auparse.AuditMessage) { s.completed++ }
func (s *auditTestStream) EventsLost(n int)                                { s.lost += n }
func TestAuditV2LostAndFlush(t *testing.T) {
	s := &auditTestStream{}
	r, err := libaudit.NewReassembler(100, time.Second, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`audit(1600000000.123:20): argc=1 a0="a"`, `audit(1600000001.123:23): argc=1 a0="b"`} {
		if err := r.Push(auparse.AUDIT_EXECVE, []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if s.completed != 2 || s.lost != 2 {
		t.Fatalf("completed=%d lost=%d", s.completed, s.lost)
	}
}
