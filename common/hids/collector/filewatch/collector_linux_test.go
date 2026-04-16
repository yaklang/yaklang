//go:build hids && linux

package filewatch

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestToEventIncludesFileIdentity(t *testing.T) {
	t.Parallel()

	file, err := os.CreateTemp(t.TempDir(), "hids-filewatch-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("resolve current user: %v", err)
	}

	collector := &Collector{}
	event := collector.toEvent(fsnotify.Event{
		Name: file.Name(),
		Op:   fsnotify.Write,
	})
	if event.File == nil {
		t.Fatal("expected file payload")
	}
	if event.File.Path != file.Name() {
		t.Fatalf("unexpected file path: %s", event.File.Path)
	}
	if event.File.Mode == "" {
		t.Fatal("expected file mode")
	}
	if event.File.UID != currentUser.Uid {
		t.Fatalf("unexpected uid: %s", event.File.UID)
	}
	if event.File.GID != currentUser.Gid {
		t.Fatalf("unexpected gid: %s", event.File.GID)
	}
	if currentUser.Username != "" && event.File.Owner != currentUser.Username {
		t.Fatalf("unexpected owner: %s", event.File.Owner)
	}
	group, err := user.LookupGroupId(currentUser.Gid)
	if err == nil && group.Name != "" && event.File.Group != group.Name {
		t.Fatalf("unexpected group: %s", event.File.Group)
	}
}

func TestCollectorHealthSnapshotTracksWatchRootsAndEvents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	collector := New(model.FileCollectorSpec{
		WatchPaths: []string{root},
	}).(*Collector)
	sink := make(chan model.Event, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer collector.Close()

	if err := collector.Start(ctx, sink); err != nil {
		t.Fatalf("start collector: %v", err)
	}

	target := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	select {
	case <-sink:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for filewatch event")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := collector.HealthSnapshot()
		detail, ok := snapshot.Detail["stats"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected stats detail: %#v", snapshot.Detail["stats"])
		}
		if detail["emitted"] == uint64(0) {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if snapshot.Name != "file" {
			t.Fatalf("unexpected collector name: %s", snapshot.Name)
		}
		if snapshot.Backend != "filewatch" {
			t.Fatalf("unexpected backend: %s", snapshot.Backend)
		}
		watch, ok := snapshot.Detail["watch"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected watch detail: %#v", snapshot.Detail["watch"])
		}
		if directories, ok := watch["directories"].(int); ok && directories <= 0 {
			t.Fatalf("expected watched directories > 0, got %d", directories)
		}
		return
	}

	t.Fatal("timed out waiting for filewatch health snapshot update")
}
