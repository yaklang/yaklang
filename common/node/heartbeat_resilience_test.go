package node

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/atomicbool"
)

type resilienceHeartbeatTransport struct {
	mu             sync.Mutex
	failures       []error
	heartbeatCalls int
	bootstrapCalls int
}

func (s *resilienceHeartbeatTransport) Bootstrap(context.Context, BootstrapRequest) (SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstrapCalls++
	return SessionState{NodeID: "node-1", SessionID: "session-rebuilt", SessionToken: "token-rebuilt"}, nil
}

func (s *resilienceHeartbeatTransport) Heartbeat(context.Context, SessionState, HeartbeatRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeatCalls++
	if s.heartbeatCalls <= len(s.failures) {
		return s.failures[s.heartbeatCalls-1]
	}
	return nil
}

func (s *resilienceHeartbeatTransport) Shutdown(context.Context, SessionState, ShutdownRequest) error {
	return nil
}

func (s *resilienceHeartbeatTransport) snapshot() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.heartbeatCalls, s.bootstrapCalls
}

func runResilienceHeartbeatSequence(t *testing.T, failures []error, wantCalls, wantBootstraps int, wantSession string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &resilienceHeartbeatTransport{failures: failures}
	registered := atomicbool.NewBool(false)
	registered.Set()
	n := &NodeBase{
		rootCtx: ctx, cancel: cancel, NodeId: "node-1",
		requestTimeout: 100 * time.Millisecond, transport: transport,
		heartbeatInterval: 5 * time.Millisecond, tickerInterval: time.Hour,
		isRegistered: registered,
		session:      SessionState{NodeID: "node-1", SessionID: "session-original", SessionToken: "token-original"},
	}
	done := make(chan struct{})
	go func() { n.runDaemonLoop(); close(done) }()
	deadline := time.After(2 * time.Second)
	for {
		calls, bootstraps := transport.snapshot()
		if calls >= wantCalls && bootstraps >= wantBootstraps {
			cancel()
			<-done
			if bootstraps != wantBootstraps {
				t.Fatalf("bootstrap calls = %d, want %d", bootstraps, wantBootstraps)
			}
			session, ok := n.currentSession()
			if !ok || session.SessionID != wantSession {
				t.Fatalf("session = %+v (present=%v), want %s", session, ok, wantSession)
			}
			return
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("heartbeat sequence did not converge: calls=%d bootstraps=%d", calls, bootstraps)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestResilienceNodeKeepsSessionAfterSingleHeartbeatTimeout(t *testing.T) {
	runResilienceHeartbeatSequence(t, []error{context.DeadlineExceeded}, 2, 0, "session-original")
}

func TestResilienceNodeRebuildsAfterRepeatedHeartbeatTimeout(t *testing.T) {
	runResilienceHeartbeatSequence(t, []error{context.DeadlineExceeded, context.DeadlineExceeded}, 3, 1, "session-rebuilt")
}

func TestResilienceNodeRebuildsImmediatelyWhenSessionInactive(t *testing.T) {
	runResilienceHeartbeatSequence(t, []error{&HTTPStatusError{StatusCode: http.StatusConflict, Message: "node session is not active"}}, 2, 1, "session-rebuilt")
}
