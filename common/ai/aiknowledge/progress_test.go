package aiknowledge

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildProgressHeartbeatAndStop(t *testing.T) {
	events := make(chan string, 20)
	p := newBuildProgress(context.Background(), "fixture", func(s string) { events <- s }, 20*time.Millisecond)
	p.event("waiting for Omni range=20s~40s")
	p.persisted.Store(1)
	<-events
	select {
	case event := <-events:
		require.Contains(t, event, "range=20s~40s")
		require.Contains(t, event, "persisted_entries=1")
	case <-time.After(time.Second):
		t.Fatal("missing heartbeat while no new result is available")
	}
	p.finish(fmt.Errorf("unexpected EOF"))
	found := false
	for len(events) > 0 {
		found = strings.Contains(<-events, "failed; partial writes may remain") || found
	}
	require.True(t, found)
	p.event("late producer callback")
	select {
	case event := <-events:
		t.Fatalf("progress emitted after completion: %s", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestKnowledgeBuildReportsPersistedEntries(t *testing.T) {
	_, opts, _, _ := fixture(t)
	create(t, opts, "progress")
	var events []string // Callbacks are serialized and finished before Manage returns.
	opts = append(opts, WithProgress(func(s string) { events = append(events, s) }))
	_, err := Build("progress", "ORBIT-731 recovery procedure", append(opts, WithBuildMode("chunks"))...)
	require.NoError(t, err)
	output := strings.Join(events, "\n")
	require.Contains(t, output, "embedding ready")
	require.Contains(t, output, "persisted_entries=1")
	require.Contains(t, output, "completed; all reported entries persisted")
}
