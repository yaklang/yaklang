package scannode

import (
	"context"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/node"
)

type sessionReadyReporterStub struct{}

func (s *sessionReadyReporterStub) Close() {}

func (s *sessionReadyReporterStub) PublishStatus(
	_ context.Context,
	_ capabilityCommandRef,
	_ CapabilityApplyResult,
) error {
	return nil
}

func (s *sessionReadyReporterStub) PublishFailed(
	context.Context,
	capabilityCommandRef,
	string,
	string,
) error {
	return nil
}

func (s *sessionReadyReporterStub) PublishAlert(context.Context, CapabilityRuntimeAlert) error {
	return nil
}

func (s *sessionReadyReporterStub) PublishObservation(
	context.Context,
	CapabilityRuntimeObservation,
) error {
	return nil
}

type sessionReadyHookStub struct {
	called int
}

func (s *sessionReadyHookStub) Apply(*CapabilityManager, capabilityHIDSApplyInput) (CapabilityApplyResult, error) {
	return CapabilityApplyResult{}, nil
}
func (s *sessionReadyHookStub) Alerts() <-chan CapabilityRuntimeAlert             { return nil }
func (s *sessionReadyHookStub) Observations() <-chan CapabilityRuntimeObservation { return nil }
func (s *sessionReadyHookStub) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	return CapabilityRuntimeStatus{}, false
}
func (s *sessionReadyHookStub) OnSessionReady(context.Context) error {
	s.called++
	return nil
}
func (s *sessionReadyHookStub) Close() error { return nil }

func TestLegionJobBridgeSyncCapabilityStatusesTriggersSessionReadyHooks(t *testing.T) {
	t.Parallel()

	session := node.SessionState{
		NodeID:             "node-restore-canonical",
		SessionID:          "session-restore",
		SessionToken:       "token-restore",
		NATSURL:            "nats://session-restore.test",
		CommandSubject:     "legion.command.node.node-restore",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-restore",
		BaseDir:            t.TempDir(),
		EnrollmentToken:    "enroll-restore",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &bootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	t.Cleanup(func() {
		base.Shutdown()
	})
	waitForNodeSession(t, base)

	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-restore",
		BaseDir: t.TempDir(),
	})
	hooks := &sessionReadyHookStub{}
	manager.hidsHooks = hooks

	bridge := newLegionJobBridge(&ScanNode{
		node:              base,
		capabilityManager: manager,
	})
	bridge.capabilityPublisher = &sessionReadyReporterStub{}

	bridge.syncCapabilityStatuses(context.Background())

	if hooks.called != 1 {
		t.Fatalf("expected session ready hook once, got %d", hooks.called)
	}
}
