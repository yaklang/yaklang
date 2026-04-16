//go:build !hids

package scannode

import (
	"errors"
	"testing"
)

func TestCapabilityManagerApplyRejectsHIDSCapabilityWhenNotCompiled(t *testing.T) {
	t.Parallel()

	manager := newCapabilityManager(CapabilityManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})

	_, err := manager.Apply(CapabilityApplyInput{
		CapabilityKey:   "hids",
		SpecVersion:     "2026-03-28",
		DesiredSpecJSON: []byte(`{"collectors":{"file":{"enabled":true,"backend":"filewatch","watch_paths":["/etc"]}}}`),
	})
	if !errors.Is(err, ErrHIDSCapabilityNotCompiled) {
		t.Fatalf("unexpected error: %v", err)
	}
}
