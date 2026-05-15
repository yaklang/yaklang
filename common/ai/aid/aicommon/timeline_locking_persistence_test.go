package aicommon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineDump_NoNestedRLockDeadlockWithWriterPending(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for i := 1; i <= 50; i++ {
		tl.PushText(int64(i), "seed-%d", i)
	}

	differ := NewTimelineDiffer(tl)
	differ.SetBaseline()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = tl.Dump()
				time.Sleep(time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = differ.Diff()
				time.Sleep(time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		var id int64 = 1000
		for {
			select {
			case <-stop:
				return
			default:
				tl.PushText(id, "writer-%d", id)
				id++
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(1200 * time.Millisecond)
	close(stop)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeline dump/diff/push goroutines did not finish in time, possible lock re-entrance deadlock")
	}
}

func TestTimelineBranch_MarkerNotSerialized(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.markBranchTimeline(true)

	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)

	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.False(t, restored.IsBranchTimeline())
}

func TestTimelineBranch_SaveSkipped(t *testing.T) {
	mainTL := NewTimeline(nil, nil)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	fork, err := mainTL.ForkForTask("1-1", "branch", cfg, cfg)
	require.NoError(t, err)
	require.NotNil(t, fork)
	require.True(t, fork.Branch.IsBranchTimeline())

	// If Save() still attempts marshal/update DB on branch timelines, this malformed map setup would panic.
	fork.Branch.idToTimelineItem = nil
	require.NotPanics(t, func() {
		fork.Branch.Save(nil, "persistent-session-id")
	})
}
