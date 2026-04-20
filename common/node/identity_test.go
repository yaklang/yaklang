package node

import "testing"

func TestResolveAgentInstallationIDPersistsGeneratedValue(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	first, err := resolveAgentInstallationID(baseDir, "")
	if err != nil {
		t.Fatalf("resolve first agent installation id: %v", err)
	}
	if first == "" {
		t.Fatal("expected generated agent installation id")
	}

	second, err := resolveAgentInstallationID(baseDir, "")
	if err != nil {
		t.Fatalf("resolve second agent installation id: %v", err)
	}
	if second != first {
		t.Fatalf("expected persisted agent installation id, got %q want %q", second, first)
	}
}

func TestResolveAgentInstallationIDPrefersExplicitValue(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	got, err := resolveAgentInstallationID(baseDir, "  INSTALL-1  ")
	if err != nil {
		t.Fatalf("resolve explicit agent installation id: %v", err)
	}
	if got != "install-1" {
		t.Fatalf("unexpected explicit agent installation id: %q", got)
	}
}
