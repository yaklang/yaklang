package node

import (
	"context"
	"testing"
	"time"

	"github.com/tevino/abool"
)

func TestNodeBaseShutdownReportsSessionEnd(t *testing.T) {
	t.Parallel()

	rootCtx, cancel := context.WithCancel(context.Background())
	transport := &stubSessionTransport{}
	node := &NodeBase{
		rootCtx:        rootCtx,
		cancel:         cancel,
		NodeId:         "node-1",
		requestTimeout: time.Second,
		transport:      transport,
		isRegistered:   abool.NewBool(true),
		session: SessionState{
			SessionID:    "session-1",
			SessionToken: "token-1",
		},
	}

	node.Shutdown()

	select {
	case <-rootCtx.Done():
	default:
		t.Fatal("expected root context to be canceled")
	}

	if transport.shutdownCalls != 1 {
		t.Fatalf("unexpected shutdown call count: %d", transport.shutdownCalls)
	}
	if transport.shutdownSession.SessionID != "session-1" {
		t.Fatalf("unexpected shutdown session_id: %s", transport.shutdownSession.SessionID)
	}
	if transport.shutdownRequest.ObservedAt.IsZero() {
		t.Fatal("expected shutdown observed_at to be set")
	}
	if node.IsRegistered() {
		t.Fatal("expected node registration to be cleared")
	}
	if _, ok := node.GetSessionState(); ok {
		t.Fatal("expected session to be cleared")
	}
}

func TestNodeBaseShutdownWithoutSessionStillCancels(t *testing.T) {
	t.Parallel()

	rootCtx, cancel := context.WithCancel(context.Background())
	transport := &stubSessionTransport{}
	node := &NodeBase{
		rootCtx:        rootCtx,
		cancel:         cancel,
		requestTimeout: time.Second,
		transport:      transport,
		isRegistered:   abool.NewBool(false),
	}

	node.Shutdown()

	select {
	case <-rootCtx.Done():
	default:
		t.Fatal("expected root context to be canceled")
	}

	if transport.shutdownCalls != 0 {
		t.Fatalf("unexpected shutdown call count: %d", transport.shutdownCalls)
	}
}
