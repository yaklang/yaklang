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

	blocks := RenderSessionArtifactsFrozenOpen(cfg)
	require.Empty(t, blocks.Frozen)
	require.Empty(t, blocks.Open)
}

func TestRenderSessionArtifactsFrozenOpenSplitsLastTaskGroupOpen(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)
	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(time.Minute))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg)
	require.Contains(t, blocks.Frozen, "task_1-1_scan")
	require.NotContains(t, blocks.Frozen, "task_1-2_verify")
	require.Contains(t, blocks.Open, "task_1-2_verify")
	require.NotContains(t, blocks.Open, "task_1-1_scan")
}

func TestRenderSessionArtifactsFrozenOpenSingleTaskGroupOpenOnly(t *testing.T) {
	dir := t.TempDir()
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", time.Unix(1700000000, 0))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	blocks := RenderSessionArtifactsFrozenOpen(cfg)
	require.Empty(t, blocks.Frozen)
	require.Contains(t, blocks.Open, "task_1-1_scan")
}

func TestRenderSessionArtifactsFrozenOpenStableFrozenWhenOpenGroupChanges(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)
	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(time.Minute))

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	before := RenderSessionArtifactsFrozenOpen(cfg)
	writeArtifactFile(t, dir, "task_1-2_verify/details.txt", "details", baseTime.Add(2*time.Minute))
	after := RenderSessionArtifactsFrozenOpen(cfg)

	require.Equal(t, before.Frozen, after.Frozen)
	require.NotEqual(t, before.Open, after.Open)
	require.Contains(t, after.Open, "details.txt")
}

func TestRenderSessionArtifactsFrozenOpenNewTaskFreezesOldOpenGroup(t *testing.T) {
	dir := t.TempDir()
	baseTime := time.Unix(1700000000, 0)
	writeArtifactFile(t, dir, "task_1-1_scan/result.txt", "scan", baseTime)

	cfg := NewConfig(context.Background())
	cfg.Workdir = dir

	before := RenderSessionArtifactsFrozenOpen(cfg)
	require.Empty(t, before.Frozen)
	require.Contains(t, before.Open, "task_1-1_scan")

	writeArtifactFile(t, dir, "task_1-2_verify/result.txt", "verify", baseTime.Add(time.Minute))
	after := RenderSessionArtifactsFrozenOpen(cfg)
	require.Contains(t, after.Frozen, "task_1-1_scan")
	require.Contains(t, after.Open, "task_1-2_verify")
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
