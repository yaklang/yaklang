//go:build hids && linux

package auditd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestSensitiveFileStateCacheEnrichesPreviousAndCurrentMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "etc", "passwd")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("root:x:0:0:root:/root:/bin/bash\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	cache := newSensitiveFileStateCache()
	cache.seedPath(target)

	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatalf("chmod target file: %v", err)
	}

	event := cache.Enrich(model.Event{
		Type: model.EventTypeAudit,
		File: &model.File{
			Path:      target,
			Operation: "chmod",
		},
		Audit: &model.Audit{
			Family: "file",
			Action: "chmod",
		},
	})

	if event.Audit == nil {
		t.Fatal("expected audit payload")
	}
	if event.File == nil {
		t.Fatal("expected file payload")
	}
	if event.Audit.PreviousFileMode == "" {
		t.Fatal("expected previous file mode")
	}
	if event.Audit.FileMode == "" {
		t.Fatal("expected current file mode")
	}
	if event.Audit.PreviousFileMode == event.Audit.FileMode {
		t.Fatalf(
			"expected previous/current file mode to differ after chmod: %q",
			event.Audit.FileMode,
		)
	}
	if event.File.Mode != event.Audit.FileMode {
		t.Fatalf("expected file mode to mirror audit mode: file=%q audit=%q", event.File.Mode, event.Audit.FileMode)
	}
}

func TestSensitiveFileStateCacheDropsRemovedPathAfterEnrichment(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "etc", "shadow")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("root:*:19793:0:99999:7:::\n"), 0o600); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	cache := newSensitiveFileStateCache()
	cache.seedPath(target)

	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target file: %v", err)
	}

	event := cache.Enrich(model.Event{
		Type: model.EventTypeAudit,
		File: &model.File{
			Path:      target,
			Operation: "remove",
		},
		Audit: &model.Audit{
			Family: "file",
			Action: "remove",
		},
	})

	if event.Audit == nil {
		t.Fatal("expected audit payload")
	}
	if event.Audit.PreviousFileMode == "" {
		t.Fatal("expected previous metadata on removed path")
	}
	if _, ok := cache.load(target); ok {
		t.Fatal("expected removed path to be evicted from cache")
	}
}
