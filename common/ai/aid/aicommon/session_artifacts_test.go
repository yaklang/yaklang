package aicommon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func requireDumpSizeNearQuarter(t *testing.T, before string, after string, stage string) {
	t.Helper()
	require.NotEmpty(t, before, "%s: before dump must not be empty", stage)
	require.NotEmpty(t, after, "%s: after dump must not be empty", stage)
	require.Less(t, len(after), len(before), "%s: after dump should be smaller than before dump", stage)

	ratio := float64(len(after)) / float64(len(before))
	// 目标是压缩后 timeline dump 接近原始 1/4；考虑头块/标签开销，容差放宽到 ±0.15。
	require.InDelta(t, 0.25, ratio, 0.15,
		"%s: dump size ratio should be close to 1/4 (before=%d after=%d ratio=%.4f)",
		stage, len(before), len(after), ratio,
	)
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

func TestPromptTimelineAfterCompression_KeepsQuarterAndSingleHead(t *testing.T) {
	const (
		totalItems     = 40
		oldMarker      = "old-marker-should-be-compressed"
		recentMarker   = "recent-marker-should-remain-open"
		summaryMarker1 = "compressed-summary-v1"
		dynamicSection = "timeline-compress-dynamic"
		nonce          = "ncmp"
	)
	var compressCount int64

	cfg := NewConfig(
		context.Background(),
		WithTimelineContentLimit(1<<30),
		WithAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			defer rsp.Close()
			prompt := req.GetPrompt()
			if strings.Contains(prompt, "批量精炼与浓缩") || strings.Contains(prompt, "batch compress") {
				seq := atomic.AddInt64(&compressCount, 1)
				rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(
					`{"@action":"timeline-reducer","reducer_memory":"compressed-summary-v%d"}`, seq,
				)))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{"@action":"timeline-shrink","persistent":"noop"}`))
			}
			return rsp, nil
		}),
	)
	timeline := cfg.GetTimeline()
	require.NotNil(t, timeline)

	for i := 1; i <= totalItems; i++ {
		name := "tool"
		payload := fmt.Sprintf("uniform-%03d-%s", i, strings.Repeat("payload-segment-", 48))
		switch i {
		case 1:
			name = "old-tool"
			payload = oldMarker + strings.Repeat("-A", 800)
		case totalItems:
			name = "recent-tool"
			payload = recentMarker + strings.Repeat("-B", 800)
		}
		timeline.PushToolResult(makeToolResult(int64(i), name, true, payload))
	}
	require.Equal(t, totalItems, len(timeline.getActiveTimelineItemIDs()))
	dumpBefore := timeline.Dump()
	require.Contains(t, dumpBefore, oldMarker)
	require.Contains(t, dumpBefore, recentMarker)

	beforeSize := timeline.calculateActualContentSize()
	keepTokens := beforeSize / 4
	if keepTokens < 1 {
		keepTokens = 1
	}
	expectedSplit := timeline.findCompressSplitByRecentKeepTokens(keepTokens)
	require.GreaterOrEqual(t, expectedSplit, 2)
	expectedActive := totalItems - expectedSplit

	timeline.SetTimelineContentLimit(beforeSize - 1)
	timeline.compressForSizeLimit()
	require.Eventually(t, func() bool {
		return timeline.compressedHead != nil &&
			strings.Contains(timeline.compressedHead.Text, summaryMarker1) &&
			timeline.compressedHead.Version == 1 &&
			len(timeline.getActiveTimelineItemIDs()) == expectedActive
	}, 8*time.Second, 50*time.Millisecond)
	require.Equal(t, int64(1), atomic.LoadInt64(&compressCount))
	require.Len(t, timeline.compressedHistory, 0)

	var dumpAfter string
	require.Eventually(t, func() bool {
		dumpAfter = timeline.Dump()
		return strings.Contains(dumpAfter, "[compressed/head]") &&
			strings.Contains(dumpAfter, summaryMarker1) &&
			strings.Contains(dumpAfter, recentMarker) &&
			!strings.Contains(dumpAfter, oldMarker)
	}, 8*time.Second, 50*time.Millisecond)
	requireDumpSizeNearQuarter(t, dumpBefore, dumpAfter, "first compression")

	frozenOpen := BuildPromptFrozenOpenMaterials(cfg, nonce)
	materials := &PromptMaterials{
		TaskInstruction: "instruction",
		Schema:          `{"type":"object"}`,
	}
	ApplyPromptFrozenOpenMaterials(materials, frozenOpen)

	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		dynamicSection,
		"dynamic",
		nil,
		nonce,
	)
	require.NoError(t, err)
	require.Contains(t, prompt, "[compressed/head]")
	require.Contains(t, prompt, summaryMarker1)
	require.Contains(t, prompt, recentMarker)
	require.NotContains(t, prompt, oldMarker)
	require.Contains(t, prompt, "<|PROMPT_SECTION_timeline-open|>")
}

func TestPromptTimelineAfterCompression_SecondCompressionRollsHeadAndHistory(t *testing.T) {
	const (
		firstWaveItems  = 40
		secondWaveItems = 40
		secondMarker    = "second-wave-recent-marker"
		nonce           = "ncmp2"
	)
	var compressCount int64

	cfg := NewConfig(
		context.Background(),
		WithTimelineContentLimit(1<<30),
		WithAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			defer rsp.Close()
			prompt := req.GetPrompt()
			if strings.Contains(prompt, "批量精炼与浓缩") || strings.Contains(prompt, "batch compress") {
				seq := atomic.AddInt64(&compressCount, 1)
				rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(
					`{"@action":"timeline-reducer","reducer_memory":"compressed-summary-v%d"}`, seq,
				)))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{"@action":"timeline-shrink","persistent":"noop"}`))
			}
			return rsp, nil
		}),
	)
	timeline := cfg.GetTimeline()
	require.NotNil(t, timeline)

	for i := 1; i <= firstWaveItems; i++ {
		payload := fmt.Sprintf("wave1-%03d-%s", i, strings.Repeat("payload-segment-", 48))
		timeline.PushToolResult(makeToolResult(int64(i), "tool", true, payload))
	}

	beforeSize1 := timeline.calculateActualContentSize()
	keepTokens1 := beforeSize1 / 4
	if keepTokens1 < 1 {
		keepTokens1 = 1
	}
	expectedSplit1 := timeline.findCompressSplitByRecentKeepTokens(keepTokens1)
	require.GreaterOrEqual(t, expectedSplit1, 2)
	expectedActiveAfterFirst := firstWaveItems - expectedSplit1

	timeline.SetTimelineContentLimit(beforeSize1 - 1)
	timeline.compressForSizeLimit()
	require.Eventually(t, func() bool {
		return timeline.compressedHead != nil &&
			timeline.compressedHead.Version == 1 &&
			strings.Contains(timeline.compressedHead.Text, "compressed-summary-v1") &&
			len(timeline.getActiveTimelineItemIDs()) == expectedActiveAfterFirst
	}, 8*time.Second, 50*time.Millisecond)
	require.Equal(t, int64(1), atomic.LoadInt64(&compressCount))
	timeline.SetTimelineContentLimit(1 << 30)

	for i := firstWaveItems + 1; i <= firstWaveItems+secondWaveItems; i++ {
		payload := fmt.Sprintf("wave2-%03d-%s", i, strings.Repeat("payload-segment-", 48))
		if i == firstWaveItems+secondWaveItems {
			payload = secondMarker + strings.Repeat("-C", 800)
		}
		timeline.PushToolResult(makeToolResult(int64(i), "tool", true, payload))
	}
	dumpBeforeSecond := timeline.Dump()
	require.Contains(t, dumpBeforeSecond, secondMarker)

	activeBeforeSecond := len(timeline.getActiveTimelineItemIDs())
	beforeSize2 := timeline.calculateActualContentSize()
	keepTokens2 := beforeSize2 / 4
	if keepTokens2 < 1 {
		keepTokens2 = 1
	}
	expectedSplit2 := timeline.findCompressSplitByRecentKeepTokens(keepTokens2)
	require.GreaterOrEqual(t, expectedSplit2, 2)
	expectedActiveAfterSecond := activeBeforeSecond - expectedSplit2

	timeline.SetTimelineContentLimit(beforeSize2 - 1)
	timeline.compressForSizeLimit()
	var dumpAfterSecond string
	require.Eventually(t, func() bool {
		dumpAfterSecond = timeline.Dump()
		return timeline.compressedHead != nil &&
			timeline.compressedHead.Version == 2 &&
			strings.Contains(timeline.compressedHead.Text, "compressed-summary-v2") &&
			len(timeline.compressedHistory) == 1 &&
			timeline.compressedHistory[0].Version == 1 &&
			strings.Contains(timeline.compressedHistory[0].Text, "compressed-summary-v1") &&
			len(timeline.getActiveTimelineItemIDs()) == expectedActiveAfterSecond &&
			strings.Contains(dumpAfterSecond, "compressed-summary-v2") &&
			strings.Contains(dumpAfterSecond, secondMarker)
	}, 8*time.Second, 50*time.Millisecond)
	require.Equal(t, int64(2), atomic.LoadInt64(&compressCount))
	requireDumpSizeNearQuarter(t, dumpBeforeSecond, dumpAfterSecond, "second compression")

	frozenOpen := BuildPromptFrozenOpenMaterials(cfg, nonce)
	materials := &PromptMaterials{
		TaskInstruction: "instruction",
		Schema:          `{"type":"object"}`,
	}
	ApplyPromptFrozenOpenMaterials(materials, frozenOpen)
	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		"timeline-compress-dynamic-second",
		"dynamic",
		nil,
		nonce,
	)
	require.NoError(t, err)
	require.Contains(t, prompt, "[compressed/head]")
	require.Contains(t, prompt, "compressed-summary-v2")
	require.NotContains(t, prompt, "compressed-summary-v1")
	require.Contains(t, prompt, secondMarker)
}
