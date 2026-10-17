package scannode

import (
	"context"
	"testing"
	"time"
)

func TestResilienceCompanyFetchFailureFencesWorkBeforeDisconnectCallback(t *testing.T) {
	bridge, _, _ := newCompanyRenewalTestBridge(t)
	old := bridge.consumer
	old.conn = newReconnectingEventTestConn(t)
	execution := bridge.companyExecutionContext(context.Background())
	bridge.invalidateCommandConsumer(old)
	if execution.Err() == nil {
		t.Fatal("fetch failure detached consumer without canceling its work")
	}
	if bridge.consumer != nil {
		t.Fatal("failed consumer remains installed")
	}
	// A new authenticated transport installs a new execution generation.
	replacementCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	replacement := &commandConsumer{conn: newReconnectingEventTestConn(t), cancel: func() {}}
	bridge.consumer, bridge.companyExecutionCtx, bridge.companyExecutionCancel = replacement, replacementCtx, cancel
	bridge.invalidateCompanyTransport(old.conn)
	bridge.invalidateCommandConsumer(old)
	if replacementCtx.Err() != nil || bridge.consumer != replacement {
		t.Fatal("late disconnect or fetch failure retired the replacement")
	}
}

func TestResilienceCompanyDelayedDisconnectRechecksTransportAfterRotation(t *testing.T) {
	bridge, _, _ := newCompanyRenewalTestBridge(t)
	old := bridge.consumer
	old.conn = newReconnectingEventTestConn(t)
	execution := bridge.companyExecutionContext(context.Background())
	// Rotation and callback ownership checks share a lifecycle lock. Hold it to
	// deterministically deliver the old callback while rotation is in progress.
	bridge.companyCleanupMu.Lock()
	entered, done := make(chan struct{}), make(chan struct{})
	go func() { close(entered); bridge.invalidateCompanyTransport(old.conn); close(done) }()
	<-entered
	bridge.stopConsumer()
	replacement := &commandConsumer{conn: newReconnectingEventTestConn(t), cancel: func() {}}
	bridge.mu.Lock()
	bridge.consumer = replacement
	bridge.mu.Unlock()
	bridge.companyCleanupMu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect callback deadlocked with rotation")
	}
	if execution.Err() != nil || bridge.consumer != replacement {
		t.Fatal("old callback canceled work during valid credential rotation")
	}
	bridge.invalidateCompanyTransport(replacement.conn)
	if execution.Err() == nil {
		t.Fatal("current transport loss failed to cancel execution")
	}
}

func TestResilienceCompanyClosedTransportIsFencedBeforeReplacement(t *testing.T) {
	bridge, _, _ := newCompanyRenewalTestBridge(t)
	conn := newReconnectingEventTestConn(t)
	bridge.consumer.conn = conn
	execution := bridge.companyExecutionContext(context.Background())
	conn.Close()
	// syncConsumer may observe the close before the asynchronous NATS callback.
	// The fixture's invalid credentials deliberately make reconnect fail closed.
	bridge.syncConsumer(context.Background())
	if execution.Err() == nil {
		t.Fatal("closed transport work survived consumer replacement")
	}
}
