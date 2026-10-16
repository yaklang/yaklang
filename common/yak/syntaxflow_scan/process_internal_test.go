package syntaxflow_scan

import (
	"context"
	"errors"
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
