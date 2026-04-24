package scannode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityManagerApplyPersistsSpec(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})

	result, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "yak.execute",
		SpecVersion:     "2026-03-28",
		DesiredSpecJSON: []byte(`{"paths":["/etc/passwd"]}`),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}
	if result.Status != capabilityStatusStored {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	raw, err := os.ReadFile(filepath.Join(baseDir, "legion", "capabilities", "yak.execute.json"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected persisted capability file")
	}
}

func TestCapabilityManagerApplyRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})

	_, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids/file_monitor",
		DesiredSpecJSON: []byte(`{}`),
	})
	if !errors.Is(err, ErrInvalidCapabilityKey) {
		t.Fatalf("unexpected error: %v", err)
	}
}

type sessionReadyHooksStub struct {
	called bool
}

func (s *sessionReadyHooksStub) Apply(*CapabilityManager, capabilityHIDSApplyInput) (CapabilityApplyResult, error) {
	return CapabilityApplyResult{}, nil
}

func (s *sessionReadyHooksStub) DryRun(*CapabilityManager, capabilityHIDSApplyInput) (CapabilityDryRunResult, error) {
	return CapabilityDryRunResult{}, nil
}

func (s *sessionReadyHooksStub) Alerts() <-chan CapabilityRuntimeAlert { return nil }

func (s *sessionReadyHooksStub) Observations() <-chan CapabilityRuntimeObservation { return nil }

func (s *sessionReadyHooksStub) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	return CapabilityRuntimeStatus{}, false
}

func (s *sessionReadyHooksStub) OnSessionReady(context.Context) error {
	s.called = true
	return nil
}

func (s *sessionReadyHooksStub) Close() error { return nil }

func TestCapabilityManagerOnSessionReadyDelegatesToHooks(t *testing.T) {
	t.Parallel()

	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})
	hooks := &sessionReadyHooksStub{}
	manager.hidsHooks = hooks

	if err := manager.OnSessionReady(context.Background()); err != nil {
		t.Fatalf("on session ready: %v", err)
	}
	if !hooks.called {
		t.Fatal("expected OnSessionReady to delegate to hids hooks")
	}
}
