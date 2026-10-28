//go:build hids && linux

package filewatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yaklang/yaklang/common/hids/model"
)

func waitForWatchCount(t *testing.T, c *Collector, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.watchedDirectoryCount() == count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("watch count=%d want=%d", c.watchedDirectoryCount(), count)
}

func TestFilewatchRemovedDirectoriesReleaseCapacityAcrossChurn(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	c.maxWatchedDirectories = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer c.Close()
	if err := c.Start(ctx, make(chan model.Event, 512)); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		path := filepath.Join(root, time.Unix(int64(i), 0).Format("150405"))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		waitForWatchCount(t, c, 2)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		waitForWatchCount(t, c, 1)
	}
	if snapshot := c.HealthSnapshot(); snapshot.Status == "degraded" {
		t.Fatalf("historical directories exhausted watch budget: %+v", snapshot)
	}
}

func TestFilewatchForgetsRenamedTreeWithoutRemovingSiblingPrefix(t *testing.T) {
	root := t.TempDir()
	moved := filepath.Join(root, "a")
	sibling := filepath.Join(root, "ab")
	for _, path := range []string{filepath.Join(moved, "child"), sibling} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	if err := c.addRecursive(context.Background(), watcher, root); err != nil {
		t.Fatal(err)
	}
	c.maxWatchedDirectories = 4
	// Re-visiting an already watched root at capacity is harmless.
	if err := c.addRecursive(context.Background(), watcher, root); err != nil {
		t.Fatal(err)
	}
	c.forgetDirectoryTree(watcher, moved)
	if c.watchedDirectoryCount() != 2 || c.state.hasDirectory(moved) || c.state.hasDirectory(filepath.Join(moved, "child")) || !c.state.hasDirectory(sibling) {
		t.Fatalf("rename cleanup affected the wrong paths: %+v", c.state.snapshot())
	}
	// A stale traversal can leave only a descendant after the root's removal
	// event. Its later rename/removal must still reclaim that watch.
	child := filepath.Join(moved, "child")
	if err := watcher.Add(child); err != nil {
		t.Fatal(err)
	}
	c.state.observeDirectory(child)
	c.forgetDirectoryTree(watcher, moved)
	if c.watchedDirectoryCount() != 2 || c.state.hasDirectoryTree(moved) {
		t.Fatal("descendant-only tree was not reclaimed")
	}
}

func TestFilewatchMovedInTreeDiscoveryDoesNotBlockEventDelivery(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); _ = c.Close() }()
	started, allowDiscovery, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	sink := make(chan model.Event, 64)
	if err := c.start(ctx, sink, func(ctx context.Context, watcher *fsnotify.Watcher, path string) {
		close(started)
		select {
		case <-allowDiscovery:
			c.tryWatchNewDirectory(ctx, watcher, path)
		case <-ctx.Done():
		}
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staging, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(root, "incoming")
	if err := os.Rename(staging, incoming); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchSignal(t, started, "discovery worker did not receive moved-in directory")
	other := filepath.Join(root, "unrelated.txt")
	if err := os.WriteFile(other, []byte("still delivered"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchEvent(t, sink, other, "")
	close(allowDiscovery)
	waitForFilewatchSignal(t, finished, "discovery did not finish after release")
	waitForWatchCount(t, c, 5)
	leaf := filepath.Join(incoming, "a", "b", "c", "tracked.txt")
	if err := os.WriteFile(leaf, []byte("new subtree is watched"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchEvent(t, sink, leaf, "")
}

func TestFilewatchCloseJoinsDiscoveryBeforePublishingStopped(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	ctx, cancel := context.WithCancel(context.Background())
	started, cancelled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() { unblock(); cancel(); _ = c.Close() }()
	if err := c.start(ctx, make(chan model.Event, 16), func(ctx context.Context, _ *fsnotify.Watcher, _ string) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		c.state.observeError(errors.New("late discovery cancellation"))
		close(exited)
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "pending"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchSignal(t, started, "discovery worker did not start")
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	waitForFilewatchSignal(t, cancelled, "Close did not cancel discovery")
	select {
	case <-closed:
		t.Fatal("Close returned before discovery worker exited")
	default:
	}
	unblock()
	waitForFilewatchSignal(t, closed, "Close did not join discovery worker")
	select {
	case <-exited:
	default:
		t.Fatal("worker remained live after Close")
	}
	if c.HealthSnapshot().Status != "stopped" || c.watchedDirectoryCount() != 0 {
		t.Fatalf("late worker changed stopped state: %+v", c.HealthSnapshot())
	}
	c.state.observeError(errors.New("stale callback after stop"))
	if c.HealthSnapshot().Status != "stopped" {
		t.Fatal("late error overwrote stopped lifecycle")
	}
}

func TestFilewatchRenameBeforeDiscoveryDoesNotRetainOldPaths(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); _ = c.Close() }()
	started, allowDiscovery, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	sink := make(chan model.Event, 64)
	if err := c.start(ctx, sink, func(ctx context.Context, watcher *fsnotify.Watcher, path string) {
		close(started)
		select {
		case <-allowDiscovery:
			c.tryWatchNewDirectory(ctx, watcher, path)
		case <-ctx.Done():
		}
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staging, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(root, "incoming")
	if err := os.Rename(staging, incoming); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchSignal(t, started, "discovery worker did not start")
	if err := os.Rename(incoming, filepath.Join(t.TempDir(), "moved-out")); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchEvent(t, sink, incoming, "RENAME")
	close(allowDiscovery)
	waitForFilewatchSignal(t, finished, "stale discovery did not finish")
	waitForWatchCount(t, c, 1)
	if c.state.hasDirectoryTree(incoming) || c.HealthSnapshot().Status != "running" {
		t.Fatalf("removed discovery retained stale state: %+v", c.HealthSnapshot())
	}
}

func TestFilewatchConcurrentCloseAndRestart(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	defer c.Close()
	for iteration := 0; iteration < 2; iteration++ {
		if err := c.Start(context.Background(), make(chan model.Event, 16)); err != nil {
			t.Fatal(err)
		}
		if err := c.Start(context.Background(), make(chan model.Event, 16)); err == nil {
			t.Fatal("duplicate Start replaced a live watcher")
		}
		waitForWatchCount(t, c, 1)
		var closing sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			closing.Add(1)
			go func() { defer closing.Done(); _ = c.Close() }()
		}
		done := make(chan struct{})
		go func() { closing.Wait(); close(done) }()
		waitForFilewatchSignal(t, done, "concurrent Close calls did not finish")
		if c.HealthSnapshot().Status != "stopped" || c.watchedDirectoryCount() != 0 {
			t.Fatal("closed watcher retained resources")
		}
	}
}

func TestFilewatchDiscoveryBurstReconcilesOverflowedDirectories(t *testing.T) {
	root := t.TempDir()
	c := New(model.FileCollectorSpec{WatchPaths: []string{root}}).(*Collector)
	c.maxWatchedDirectories = 256
	ctx, cancel := context.WithCancel(context.Background())
	started, allowDiscovery := make(chan struct{}), make(chan struct{})
	var startOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(allowDiscovery) }) }
	defer func() { unblock(); cancel(); _ = c.Close() }()
	if err := c.start(ctx, make(chan model.Event, 1024), func(ctx context.Context, watcher *fsnotify.Watcher, path string) {
		startOnce.Do(func() { close(started) })
		select {
		case <-allowDiscovery:
			c.tryWatchNewDirectory(ctx, watcher, path)
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "first"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitForFilewatchSignal(t, started, "discovery worker did not start")
	const burst = directoryDiscoveryQueueSize + 8
	for index := 0; index < burst; index++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("burst-%d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if snapshot := c.HealthSnapshot(); strings.Contains(snapshot.Message, "queue is full") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test did not exercise directory queue overflow")
		}
		time.Sleep(5 * time.Millisecond)
	}
	unblock()
	waitForWatchCount(t, c, 2+burst)
}

func waitForFilewatchSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}

func waitForFilewatchEvent(t *testing.T, sink <-chan model.Event, path, operation string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-sink:
			if event.File != nil && event.File.Path == path && strings.Contains(event.File.Operation, operation) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s event for %s", operation, path)
		}
	}
}
