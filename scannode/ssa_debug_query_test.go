package scannode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebugDirRegistryRegisterResolveUnregister(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "debug", "job-a_attempt-b")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir debug dir: %v", err)
	}

	registry := &debugDirRegistry{dirs: make(map[string]string)}
	registry.register(base, "job-a", "attempt-b", dir)

	if got := registry.resolve("job-a", "attempt-b"); got != dir {
		t.Fatalf("resolve registered dir: got %q want %q", got, dir)
	}

	registry.unregister("job-a", "attempt-b")
	// After unregister the convention-path fallback must still find the dir.
	if got := registry.resolve("job-a", "attempt-b"); got != dir {
		t.Fatalf("resolve fallback dir: got %q want %q", got, dir)
	}

	if got := registry.resolve("job-a", "missing-attempt"); got != "" {
		t.Fatalf("resolve unknown attempt: got %q want empty", got)
	}
}

func TestDebugDirRegistryIgnoresEmptyRegistration(t *testing.T) {
	t.Parallel()

	registry := &debugDirRegistry{dirs: make(map[string]string)}
	registry.register("", "job-a", "attempt-b", "")
	if got := registry.resolve("job-a", "attempt-b"); got != "" {
		t.Fatalf("resolve after empty registration: got %q want empty", got)
	}
}

func TestDebugDirKeySanitizes(t *testing.T) {
	t.Parallel()

	// sanitizeLogName replaces path separators, backslashes and NUL bytes
	// only — spaces are valid in directory names and are preserved.
	if got := debugDirKey("job/a", "attempt\\b"); got != "job_a_attempt_b" {
		t.Fatalf("unexpected debug dir key: %q", got)
	}
}
