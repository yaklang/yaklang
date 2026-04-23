package diagnostics

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHeapProfileTargetPrefersOptionDumpDirOverEnv(t *testing.T) {
	t.Setenv(envHeapDumpDir, filepath.Join(t.TempDir(), "env"))

	optionDir := filepath.Join(t.TempDir(), "option")
	target := resolveHeapProfileTarget(
		newHeapDumpConfig(
			WithDumpDir(optionDir),
			WithName("node-hids"),
		),
		64*1024*1024,
		"before_gc",
	)

	if !strings.HasPrefix(target, optionDir+string(filepath.Separator)) {
		t.Fatalf("expected heap target under option dump dir, got %s", target)
	}
	if !strings.HasSuffix(target, ".pb.gz") {
		t.Fatalf("expected heap target to end with .pb.gz, got %s", target)
	}
	if !strings.Contains(target, "node-hids_before_gc") {
		t.Fatalf("expected heap target to include normalized name and phase, got %s", target)
	}
}

func TestResolveHeapProfileTargetUsesEnvDumpDirByDefault(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "env")
	t.Setenv(envHeapDumpDir, envDir)

	target := resolveHeapProfileTarget(
		newHeapDumpConfig(
			WithName("node-hids"),
		),
		64*1024*1024,
		"after_gc",
	)

	if !strings.HasPrefix(target, envDir+string(filepath.Separator)) {
		t.Fatalf("expected heap target under env dump dir, got %s", target)
	}
	if !strings.Contains(target, "node-hids_after_gc") {
		t.Fatalf("expected heap target to include normalized name and phase, got %s", target)
	}
}
