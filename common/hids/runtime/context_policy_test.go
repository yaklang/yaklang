//go:build hids && linux

package runtime

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func TestShortTermContextBoundsProcessTracker(t *testing.T) {
	t.Parallel()

	tracker := newProcessTrackerWithConfig(shortTermContextConfig{
		window:       5 * time.Minute,
		maxProcesses: 2,
	})
	start := time.Date(2026, 4, 22, 13, 0, 0, 0, time.UTC)
	for pid := 1; pid <= 3; pid++ {
		tracker.Apply(model.Event{
			Type:      model.EventTypeProcessExec,
			Timestamp: start.Add(time.Duration(pid) * time.Second),
			Process: &model.Process{
				PID:   pid,
				Image: "/bin/test",
			},
		})
	}
	if got := len(tracker.byPID); got != 2 {
		t.Fatalf("expected bounded process tracker, got %d", got)
	}
	if _, exists := tracker.byPID[1]; exists {
		t.Fatal("expected oldest process context to be evicted")
	}
	if _, exists := tracker.byPID[2]; !exists {
		t.Fatal("expected newer process context to remain")
	}
	if _, exists := tracker.byPID[3]; !exists {
		t.Fatal("expected newest process context to remain")
	}
}

func TestShortTermContextCompactsProcessTrackerOrder(t *testing.T) {
	t.Parallel()

	tracker := newProcessTrackerWithConfig(shortTermContextConfig{
		window:       5 * time.Minute,
		maxProcesses: 2,
	})
	start := time.Date(2026, 4, 22, 13, 10, 0, 0, time.UTC)
	for index := 0; index < 1500; index++ {
		tracker.Apply(model.Event{
			Type:      model.EventTypeProcessExec,
			Timestamp: start.Add(time.Duration(index) * time.Millisecond),
			Process: &model.Process{
				PID:     42,
				Image:   "/bin/test",
				Command: "/bin/test",
			},
		})
	}
	if got := len(tracker.byPID); got != 1 {
		t.Fatalf("expected one active process context, got %d", got)
	}
	threshold := processTrackerOrderCompactThreshold(len(tracker.byPID), tracker.maxEntries)
	if len(tracker.order) > threshold {
		t.Fatalf("expected stale process order entries compacted below %d, got %d", threshold, len(tracker.order))
	}
}

func TestShortTermContextExpiresProcessTrackerEntries(t *testing.T) {
	t.Parallel()

	tracker := newProcessTrackerWithConfig(shortTermContextConfig{
		window:       time.Minute,
		maxProcesses: 16,
	})
	start := time.Date(2026, 4, 22, 13, 30, 0, 0, time.UTC)
	tracker.Apply(model.Event{
		Type:      model.EventTypeProcessExec,
		Timestamp: start,
		Process:   &model.Process{PID: 10, Image: "/bin/old"},
	})
	tracker.Apply(model.Event{
		Type:      model.EventTypeProcessExec,
		Timestamp: start.Add(2 * time.Minute),
		Process:   &model.Process{PID: 11, Image: "/bin/new"},
	})
	if _, exists := tracker.byPID[10]; exists {
		t.Fatal("expected expired process context to be evicted")
	}
	if _, exists := tracker.byPID[11]; !exists {
		t.Fatal("expected fresh process context to remain")
	}
}

func TestShortTermContextBoundsNetworkTracker(t *testing.T) {
	t.Parallel()

	tracker := newNetworkTrackerWithConfig(shortTermContextConfig{
		window:      5 * time.Minute,
		maxNetworks: 1,
	})
	start := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	for fd := 10; fd <= 11; fd++ {
		tracker.Apply(model.Event{
			Type:      model.EventTypeNetworkConnect,
			Timestamp: start.Add(time.Duration(fd) * time.Second),
			Process:   &model.Process{PID: 100},
			Network: &model.Network{
				Protocol:      "tcp",
				SourceAddress: "10.0.0.2",
				SourcePort:    50000 + fd,
				DestAddress:   "203.0.113.10",
				DestPort:      443,
			},
			Data: map[string]any{"fd": fd},
		})
	}
	if got := tracker.uniqueEntryCount(); got != 1 {
		t.Fatalf("expected bounded network tracker, got %d", got)
	}
}

func TestShortTermContextBoundsFileTracker(t *testing.T) {
	t.Parallel()

	tracker := newFileTrackerWithConfig(shortTermContextConfig{
		window:   5 * time.Minute,
		maxFiles: 1,
	})
	start := time.Date(2026, 4, 22, 15, 0, 0, 0, time.UTC)
	for _, path := range []string{"/tmp/a", "/tmp/b"} {
		tracker.Apply(model.Event{
			Type:      model.EventTypeFileChange,
			Timestamp: start,
			File: &model.File{
				Path:      path,
				Operation: "WRITE",
				Mode:      "0644",
			},
		})
	}
	if got := len(tracker.byPath); got != 1 {
		t.Fatalf("expected bounded file tracker, got %d", got)
	}
}
