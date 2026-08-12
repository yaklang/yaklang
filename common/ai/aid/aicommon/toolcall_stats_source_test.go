package aicommon

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

type toolHitRecord struct {
	name   string
	source string
}

type toolHitRecorder struct {
	mu   sync.Mutex
	hits []toolHitRecord
}

func (r *toolHitRecorder) RecordToolHit(_ *Config, name, source string) {
	r.mu.Lock()
	r.hits = append(r.hits, toolHitRecord{name: name, source: source})
	r.mu.Unlock()
}

func (*toolHitRecorder) RecordSkillHit(*Config, string, string) {}
func (*toolHitRecorder) RecordAction(*Config, string)           {}
func (*toolHitRecorder) RecordAICall(*Config, string, *aispec.ChatUsage) {
}

func (r *toolHitRecorder) snapshot() []toolHitRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]toolHitRecord(nil), r.hits...)
}

func installToolHitRecorder(t *testing.T) *toolHitRecorder {
	t.Helper()
	previous := getStatsRecorder()
	recorder := &toolHitRecorder{}
	RegisterStatsRecorder(recorder)
	t.Cleanup(func() { RegisterStatsRecorder(previous) })
	return recorder
}

func TestNotifySessionSnapshotToolCall_SourceAndCallIDDeduplication(t *testing.T) {
	recorder := installToolHitRecorder(t)
	cfg := NewTestConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.ResetSessionSnapshotExecution("stats-source", "processing", testSessionSnapshotStartTime())

	requested := &aitool.ToolResult{Name: "requested-tool", ToolCallID: "call-requested", Success: true}
	NotifySessionSnapshotToolCall(cfg, requested, StatsSourceToolRequested)
	NotifySessionSnapshotToolCall(cfg, requested, StatsSourceToolRequested)
	NotifySessionSnapshotToolCall(cfg, &aitool.ToolResult{Name: "direct-tool", ToolCallID: "call-direct", Success: false}, StatsSourceToolDirect)
	// Preserve the historical API/default for callers that do not yet provide
	// explicit metadata: an omitted or unknown source is classified as direct.
	NotifySessionSnapshotToolCall(cfg, &aitool.ToolResult{Name: "legacy-tool", ToolCallID: "call-legacy", Success: true})

	require.Equal(t, []toolHitRecord{
		{name: "requested-tool", source: StatsSourceToolRequested},
		{name: "direct-tool", source: StatsSourceToolDirect},
		{name: "legacy-tool", source: StatsSourceToolDirect},
	}, recorder.snapshot())

	execution := cfg.BuildSessionSnapshotExecution(nil)
	require.Equal(t, 3, execution.ToolCallTotal)
	require.Equal(t, 2, execution.ToolCallSuccess)
	require.Equal(t, 1, execution.ToolCallFailed)
}

func TestToolCaller_CheckpointReplayDoesNotDoubleCountSessionOrStats(t *testing.T) {
	setupToolCallStatsProjectDB(t)
	recorder := installToolHitRecorder(t)

	var callbackCount atomic.Int32
	tool, err := aitool.New(
		"checkpoint-stats-tool",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			callbackCount.Add(1)
			return map[string]any{"ok": true}, nil
		}),
	)
	require.NoError(t, err)

	runtimeID := "checkpoint-stats-runtime"
	callToolID := "checkpoint-stats-call"
	workdir := t.TempDir()
	const checkpointSeq int64 = 991_001
	newConfigAndCaller := func() (*Config, *ToolCaller) {
		cfg := NewTestConfig(
			context.Background(),
			WithID(runtimeID),
			WithWorkdir(workdir),
			WithDisableAutoSkills(true),
		)
		caller, callerErr := NewToolCaller(
			context.Background(),
			WithToolCaller_AICallerConfig(cfg),
			WithToolCaller_AICaller(cfg),
			WithToolCaller_Task(cfg.DefaultTask),
			WithToolCaller_Emitter(cfg.GetEmitter()),
			WithToolCaller_CallToolID(callToolID),
			WithToolCaller_RuntimeId(callToolID),
			WithToolCaller_CheckpointSeq(checkpointSeq),
			WithToolCaller_StatsSource(StatsSourceToolRequested),
		)
		require.NoError(t, callerErr)
		return cfg, caller
	}

	firstConfig, firstCaller := newConfigAndCaller()
	firstResult, directlyAnswer, err := firstCaller.CallToolWithExistedParams(tool, true, aitool.InvokeParams{})
	require.NoError(t, err)
	require.False(t, directlyAnswer)
	require.NotNil(t, firstResult)
	require.True(t, firstResult.Success)
	require.Equal(t, int32(1), callbackCount.Load())
	require.Equal(t, 1, firstConfig.BuildSessionSnapshotExecution(nil).ToolCallTotal)

	// A fresh Config models process/runtime reconstruction. The finished
	// checkpoint returns the stored result but must not emit a second session or
	// persistent hit-stat event, because no plugin callback executes this time.
	replayConfig, replayCaller := newConfigAndCaller()
	replayResult, directlyAnswer, err := replayCaller.CallToolWithExistedParams(tool, true, aitool.InvokeParams{})
	require.NoError(t, err)
	require.False(t, directlyAnswer)
	require.NotNil(t, replayResult)
	require.True(t, replayResult.Success)
	require.Equal(t, int32(1), callbackCount.Load())
	require.Equal(t, 0, replayConfig.BuildSessionSnapshotExecution(nil).ToolCallTotal)
	require.Equal(t, []toolHitRecord{{name: tool.Name, source: StatsSourceToolRequested}}, recorder.snapshot())
}

func setupToolCallStatsProjectDB(t *testing.T) {
	t.Helper()
	originalPath := consts.GetCurrentProjectDatabasePath()
	require.NoError(t, consts.SetGormProjectDatabase(filepath.Join(t.TempDir(), "toolcall-stats.db")))
	t.Cleanup(func() { require.NoError(t, consts.SetGormProjectDatabase(originalPath)) })
	require.NoError(t, consts.GetGormProjectDatabase().AutoMigrate(
		&schema.AiOutputEvent{},
		&schema.AiCheckpoint{},
	).Error)
}

func testSessionSnapshotStartTime() time.Time {
	return time.Unix(1_700_000_000, 0)
}
