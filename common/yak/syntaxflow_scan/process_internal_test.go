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
