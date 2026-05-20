package aicommon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeArtifactFile(t *testing.T, root string, rel string, body string, mod time.Time) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))
	require.NoError(t, os.Chtimes(path, mod, mod))
}

func TestRenderSessionArtifactsFrozenOpenEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg, 0)
	require.Empty(t, blocks.Frozen)
	require.Empty(t, blocks.Open)
}

func TestRenderSessionArtifactsFrozenOpenSplitsByFrozenTime(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)
	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(time.Minute))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg, baseTime.Add(30*time.Second).Unix())
	require.Contains(t, blocks.Frozen, "task_1-1_scan")
	require.NotContains(t, blocks.Frozen, "task_1-2_verify")
	require.Contains(t, blocks.Open, "task_1-2_verify")
	require.NotContains(t, blocks.Open, "task_1-1_scan")
	require.NotContains(t, blocks.Frozen, "total_files")
	require.Contains(t, blocks.Frozen, "frozen_time:")
}

func TestRenderSessionArtifactsFrozenOpenEqualFrozenTimeStaysOpen(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg, baseTime.Unix())
	require.Empty(t, blocks.Frozen)
	require.Contains(t, blocks.Open, "task_1-1_scan")
}

func TestRenderSessionArtifactsFrozenOpenStableFrozenWhenFrozenGroupChanges(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)
	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(time.Minute))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	frozenTime := baseTime.Add(30 * time.Second).Unix()
	before := RenderSessionArtifactsFrozenOpen(cfg, frozenTime)
	writeArtifactFile(t, dir, "task_1-1_scan/details.txt", "details", baseTime.Add(2*time.Minute))
	after := RenderSessionArtifactsFrozenOpen(cfg, frozenTime)

	require.Equal(t, before.Frozen, after.Frozen)
	require.NotEqual(t, before.Open, after.Open)
	require.Contains(t, after.Open, "details.txt")
	require.Contains(t, after.Open, "task_1-1_scan (updates after frozen snapshot)")
}

func TestRenderSessionArtifactsFrozenOpenFrozenTimeAdvanceSealsEligibleGroup(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	before := RenderSessionArtifactsFrozenOpen(cfg, baseTime.Unix())
	require.Empty(t, before.Frozen)
	require.Contains(t, before.Open, "task_1-1_scan")

	after := RenderSessionArtifactsFrozenOpen(cfg, baseTime.Add(time.Second).Unix())
	require.Contains(t, after.Frozen, "task_1-1_scan")
	require.NotContains(t, after.Open, "task_1-1_scan")
}

func TestRenderSessionArtifactsFrozenOpenRootFilesAlwaysOpen(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "session_summary.txt", "root", baseTime)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg, baseTime.Add(time.Hour).Unix())
	require.Contains(t, blocks.Frozen, "task_1-1_scan")
	require.NotContains(t, blocks.Frozen, "session_summary.txt")
	require.Contains(t, blocks.Open, "[root files]")
	require.Contains(t, blocks.Open, "session_summary.txt")
}

func TestBuildPromptFrozenOpenMaterialsCoordinatesTimelineAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime.Add(30*time.Second))
	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(4*time.Minute))

	timeline := NewTimeline(nil, nil)
	injectTimelineItem(timeline, 1, baseTime.Add(30*time.Second), makeToolResult(1, "scan", true, "scan-ok"))
	injectTimelineItem(timeline, 2, baseTime.Add(4*time.Minute), makeToolResult(2, "verify", true, "verify-ok"))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir
	cfg.Timeline = timeline
	partition, ok := NewFrozenBlockPartition("plan_facts", "Plan Facts", "stable facts", 100)
	require.True(t, ok)
	cfg.GetOrCreateFrozenBlockPartitionProducer().AppendPartition(partition)

	materials := BuildPromptFrozenOpenMaterials(cfg)
	require.Equal(t, baseTime.Add(3*time.Minute).Unix(), materials.TimelineFrozenTimeUnix)
	require.Contains(t, materials.TimelineFrozen, "scan-ok")
	require.Contains(t, materials.TimelineOpen, "verify-ok")
	require.Len(t, materials.FrozenPartitions, 1)
	require.Equal(t, "plan_facts", materials.FrozenPartitions[0].ID)
	require.Contains(t, materials.SessionArtifactsFrozen, "task_1-1_scan")
	require.Contains(t, materials.SessionArtifactsOpen, "task_1-2_verify")
}

func TestSessionArtifactsTemplatesPlacement(t *testing.T) {
	materials := &PromptMaterials{
		SessionArtifactsFrozen: "artifacts_dir: /tmp/session\ntotal_files: 1\n\n### task_1-1_done\n- result.txt (1B, 00:00:00)\n",
		SessionArtifactsOpen:   "artifacts_dir: /tmp/session\ntotal_files: 1\n\n### task_1-2_open\n- result.txt (1B, 00:00:00)\n",
		Workspace:              true,
		OSArch:                 "darwin/arm64",
		WorkingDir:             "/tmp/session",
	}

	frozen, err := RenderPromptTemplate("test-frozen-artifacts", SharedFrozenBlockTemplate, materials.FrozenBlockData())
	require.NoError(t, err)
	open, err := RenderPromptTemplate("test-open-artifacts", SharedTimelineOpenTemplate, materials.TimelineOpenData())
	require.NoError(t, err)

	require.Contains(t, frozen, "# Session Artifacts (Frozen)")
	require.Contains(t, frozen, "task_1-1_done")
	require.Contains(t, open, "# Session Artifacts (Open)")
	require.Contains(t, open, "task_1-2_open")
	require.NotContains(t, open, "## Session Artifacts")

	workspaceIdx := strings.Index(open, "# Workspace Context")
	artifactsIdx := strings.Index(open, "# Session Artifacts (Open)")
	require.Greater(t, artifactsIdx, workspaceIdx)
}
