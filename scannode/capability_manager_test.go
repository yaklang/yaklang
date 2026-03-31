package scannode

import (
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
		CapabilityKey:   "hids.file_monitor",
		SpecVersion:     "2026-03-28",
		DesiredSpecJSON: []byte(`{"paths":["/etc/passwd"]}`),
	})
	if err != nil {
		t.Fatalf("apply capability: %v", err)
	}
	if result.Status != capabilityStatusStored {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	raw, err := os.ReadFile(filepath.Join(baseDir, "legion", "capabilities", "hids.file_monitor.json"))
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
