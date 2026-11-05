package syntaxflow_scan

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestUpdateRuleErrorCreatesMissingEntry(t *testing.T) {
	pm := newProcessMonitor(context.Background(), time.Second, nil, nil, true)
	pm.UpdateRuleError("prog", "fast-fail", errors.New("boom"))

	info, ok := pm.Status.Get("fast-fail@prog")
	require.True(t, ok, "failed rules that never progressed must still enter Status")
	require.True(t, info.Finished)
	require.Equal(t, 1.0, info.Progress)
	require.ErrorContains(t, info.Error, "boom")
}

func TestUpdateRuleSkippedEntersStatus(t *testing.T) {
	pm := newProcessMonitor(context.Background(), time.Second, nil, nil, true)
	pm.UpdateRuleSkipped("prog", "php-rule", "skipped: language mismatch")

	info, ok := pm.Status.Get("php-rule@prog")
	require.True(t, ok, "skipped rules must enter Status so snapshots can name them")
	require.True(t, info.Finished)
	require.True(t, info.Skipped)
	require.Equal(t, 1.0, info.Progress)
	require.Equal(t, "skipped: language mismatch", info.Info)
}

func TestUpdateRuleStatusFinishMarksFinished(t *testing.T) {
	pm := newProcessMonitor(context.Background(), time.Second, nil, nil, true)
	pm.UpdateRuleStatus("prog", "fast-ok", 1, "")

	info, ok := pm.Status.Get("fast-ok@prog")
	require.True(t, ok)
	require.True(t, info.Finished)
	require.False(t, info.Skipped)
	require.Equal(t, 1.0, info.Progress)
}

func TestRuleProgressSnapshotsAreImmutableAndMonotone(t *testing.T) {
	pm := newProcessMonitor(context.Background(), time.Second, nil, nil, true)
	pm.UpdateRuleStatus("prog", "multi-unit", .93, "first unit")
	first := pm.snapshotInfoList(true)
	require.Len(t, first.Rules, 1)
	pm.UpdateRuleStatus("prog", "multi-unit", .75, "next unit")
	second := pm.snapshotInfoList(true)
	require.Equal(t, .93, second.Rules[0].Progress, "a new frame does not reset rule progress")
	pm.UpdateRuleStatus("prog", "multi-unit", 1, "done")
	require.Equal(t, .93, first.Rules[0].Progress, "published snapshots must not change")
	require.False(t, first.Rules[0].Finished)
	pm.UpdateRuleStatus("prog", "multi-unit", .5, "stale callback")
	final := pm.snapshotInfoList(true)
	require.Equal(t, 1.0, final.Rules[0].Progress)
	require.True(t, final.Rules[0].Finished)
}

func TestSharedScanCallbackOptionsForwardRuleDetailAndDebugDir(t *testing.T) {
	cfg, err := NewConfig(
		WithProcessRuleDetail(true),
		ssaconfig.WithDebugDir("/tmp/debug-scan"),
	)
	require.NoError(t, err)
	require.True(t, cfg.ProcessWithRule)
	require.Equal(t, "/tmp/debug-scan", cfg.GetDebugDir())

	rebuilt, err := NewConfig(sharedScanCallbackOptions(cfg)...)
	require.NoError(t, err)
	require.True(t, rebuilt.ProcessWithRule, "ScanProject must forward withProcessRuleDetail into StartScan")
	require.Equal(t, "/tmp/debug-scan", rebuilt.GetDebugDir(), "ScanProject must forward debug_dir into StartScan")
}

func TestProcessMonitorQueuedEventsKeepObservedProgress(t *testing.T) {
	for _, withRules := range []bool{false, true} {
		t.Run(fmt.Sprint(withRules), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			got := make(chan *RuleProcessInfoList, 8)
			var once sync.Once
			pm := newProcessMonitor(context.Background(), time.Hour, func(_ float64, info *RuleProcessInfoList) {
				got <- info
				once.Do(func() { close(entered); <-release })
			}, nil, withRules)
			pm.TotalQuery.Store(2)
			pm.StartMonitor()
			defer pm.Close()
			pm.EmitEvent()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("initial observer did not start")
			}
			pm.FinishedQuery.Store(1)
			pm.UpdateRuleStatus("prog", "one", .5, "first")
			pm.EmitEvent()
			pm.FinishedQuery.Store(2)
			pm.UpdateRuleStatus("prog", "one", 1, "done")
			pm.EmitEvent()
			close(release)
			var values []*RuleProcessInfoList
			for len(values) < 3 {
				select {
				case info := <-got:
					values = append(values, info)
				case <-time.After(3 * time.Second):
					t.Fatal("queued progress was lost")
				}
			}
			require.Equal(t, 0.0, values[0].Progress)
			require.Equal(t, .5, values[1].Progress)
			require.Equal(t, int64(1), values[1].FinishedQuery)
			require.Equal(t, 1.0, values[2].Progress)
			if withRules {
				require.Len(t, values[1].Rules, 1)
				require.Equal(t, .5, values[1].Rules[0].Progress)
				require.False(t, values[1].Rules[0].Finished)
			} else {
				require.Empty(t, values[1].Rules)
			}
		})
	}
}

func TestProcessMonitorCloseDrainsQueuedProgress(t *testing.T) {
	entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var lock sync.Mutex
	var values []float64
	pm := newProcessMonitor(context.Background(), time.Hour, func(progress float64, _ *RuleProcessInfoList) {
		lock.Lock()
		values = append(values, progress)
		lock.Unlock()
		once.Do(func() { close(entered); <-release })
	}, nil, false)
	pm.TotalQuery.Store(2)
	pm.StartMonitor()
	pm.EmitEvent()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial observer did not start")
	}
	pm.FinishedQuery.Store(1)
	pm.EmitEvent()
	pm.FinishedQuery.Store(2)
	pm.EmitEvent()
	go func() { pm.Close(); close(closed) }()
	require.Eventually(t, func() bool { return pm.closed.Load() }, 3*time.Second, time.Millisecond)
	close(release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("progress close did not return")
	}
	lock.Lock()
	defer lock.Unlock()
	require.Contains(t, values, .5, "a closed result channel must not discard earlier progress events")
	require.Equal(t, 1.0, values[len(values)-1])
}

func TestProcessMonitorLiveSnapshotFollowsQueuedObservations(t *testing.T) {
	var got []float64
	pm := newProcessMonitor(context.Background(), time.Hour, func(progress float64, _ *RuleProcessInfoList) {
		got = append(got, progress)
	}, nil, false)
	pm.TotalQuery.Store(2)
	pm.EmitEvent()
	pm.FinishedQuery.Store(1)
	pm.EmitEvent()
	pm.FinishedQuery.Store(2)
	pm.reportProcess(true)
	require.Equal(t, []float64{0, .5, 1}, got, "a timer cannot overtake already queued progress")
	require.Empty(t, pm.eventCh)
}

func TestProcessMonitorFullQueueDoesNotAcknowledgeUndeliveredRules(t *testing.T) {
	pm := newProcessMonitor(context.Background(), time.Hour, nil, nil, true)
	for i := 0; i < cap(pm.eventCh); i++ {
		pm.EmitEvent()
	}
	pm.UpdateRuleStatus("prog", "finished-while-full", 1, "done")
	pm.EmitEvent()
	require.Len(t, pm.eventCh, cap(pm.eventCh))
	pending := pm.snapshotInfoList(true)
	require.Len(t, pending.Rules, 1, "a dropped event must leave its finished rule eligible for the next snapshot")
	require.Equal(t, "finished-while-full", pending.Rules[0].RuleName)
	require.True(t, pending.Rules[0].Finished)
}

func TestProcessMonitorConcurrentProgressAndTimerRemainMonotone(t *testing.T) {
	for _, withRules := range []bool{false, true} {
		t.Run(fmt.Sprint(withRules), func(t *testing.T) {
			var mu sync.Mutex
			var got []float64
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pm := newProcessMonitor(ctx, time.Millisecond, func(progress float64, _ *RuleProcessInfoList) {
				mu.Lock()
				got = append(got, progress)
				mu.Unlock()
			}, nil, withRules)
			pm.TotalQuery.Store(2048)
			pm.StartMonitor()
			var workers sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				workers.Add(1)
				go func(worker int) {
					defer workers.Done()
					for i := 0; i < 256; i++ {
						pm.UpdateRuleStatus("prog", fmt.Sprint(worker), float64(i+1)/256, "")
						pm.FinishedQuery.Add(1)
						pm.EmitEvent()
					}
				}(worker)
			}
			workers.Wait()
			pm.Close()
			mu.Lock()
			defer mu.Unlock()
			require.NotEmpty(t, got)
			for i := 1; i < len(got); i++ {
				require.GreaterOrEqual(t, got[i], got[i-1], "callback %d regressed", i)
			}
			require.Equal(t, 1.0, got[len(got)-1])
		})
	}
}
