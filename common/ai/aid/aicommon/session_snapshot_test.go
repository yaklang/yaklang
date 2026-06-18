package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSessionSnapshot_BuildAndRevision(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.ResetSessionSnapshotExecution("demo-task", "processing", time.Unix(1_700_000_000, 0))
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{
		Name:       "grep",
		Success:    true,
		ToolCallID: "call-1",
	})
	cfg.RecordSessionSnapshotFileWrite("/tmp/demo.txt")
	cfg.RecordSessionSnapshotFileWrite("/tmp/other.txt")

	snapshot := &SessionSnapshot{
		Revision:     cfg.NextSessionSnapshotRevision(),
		UpdatedAt:    time.Now().Unix(),
		Execution:    cfg.BuildSessionSnapshotExecution(nil),
		Capabilities: BuildCapabilityInventoryItems(cfg, ConfigPromptCapabilityLoopContext{}),
	}
	require.Equal(t, int64(1), snapshot.Revision)
	require.NotNil(t, snapshot.Execution)
	require.Equal(t, 1, snapshot.Execution.ToolCallSuccess)
	require.Equal(t, 2, snapshot.Execution.ModifiedFileCount)
	require.Equal(t, "demo-task", snapshot.Execution.TaskName)
}

func TestNotifySessionSnapshotEmit_Immediate(t *testing.T) {
	emitted := false
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.SetSessionSnapshotEmitHandler(func() {
		emitted = true
	})
	cfg.NotifySessionSnapshotEmit(true)
	require.True(t, emitted)
}

func TestNotifySessionSnapshotEmit_Debounced(t *testing.T) {
	emitted := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.SetSessionSnapshotEmitHandler(func() {
		emitted++
	})
	cfg.NotifySessionSnapshotEmit()
	require.Equal(t, 0, emitted)
	time.Sleep(1100 * time.Millisecond)
	require.Equal(t, 1, emitted)
}

func TestNormalizeSessionSnapshot_FullPayload(t *testing.T) {
	snapshot := &SessionSnapshot{
		Revision:  1,
		UpdatedAt: time.Now().Unix(),
	}
	NormalizeSessionSnapshot(snapshot)
	require.NotNil(t, snapshot.Execution)
	require.NotNil(t, snapshot.Perception)
	require.NotNil(t, snapshot.Capabilities)
	require.Equal(t, "processing", snapshot.Execution.Status)
}

func TestBuildSessionSnapshotExecution_NilTaskReturnsNonNil(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	exec := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, exec)
	require.Equal(t, "processing", exec.Status)
}
