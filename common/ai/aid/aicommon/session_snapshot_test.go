package aicommon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	var emitted atomic.Int32
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.SetSessionSnapshotEmitHandler(func() {
		emitted.Add(1)
	})
	cfg.NotifySessionSnapshotEmit()
	require.Equal(t, int32(0), emitted.Load())
	time.Sleep(1100 * time.Millisecond)
	require.Equal(t, int32(1), emitted.Load())
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
	require.NotNil(t, snapshot.BackgroundProcesses)
	require.Equal(t, "processing", snapshot.Execution.Status)
}

func TestSessionSnapshot_BackgroundProcesses(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.AddSessionSnapshotBackgroundProcess(SessionSnapshotProcessTypeBrowser, "scanner", "Scanner Browser")
	cfg.AddSessionSnapshotBackgroundProcess(SessionSnapshotProcessTypeBrowser, "crawler", "Crawler Browser")

	procs := cfg.BuildSessionSnapshotBackgroundProcesses()
	require.Len(t, procs, 2)

	cfg.RemoveSessionSnapshotBackgroundProcess("scanner")
	procs = cfg.BuildSessionSnapshotBackgroundProcesses()
	require.Len(t, procs, 1)
	require.Equal(t, "crawler", procs[0].ProcessID)
	require.Equal(t, "Crawler Browser", procs[0].ProcessName)
	require.Equal(t, SessionSnapshotProcessTypeBrowser, procs[0].Type)
	require.Equal(t, SessionSnapshotProcessStatusRunning, procs[0].Status)
}

func TestBuildSessionSnapshotExecution_NilTaskReturnsNonNil(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	exec := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, exec)
	require.Equal(t, "processing", exec.Status)
}

func TestBuildSessionSnapshotExecution_EndedAtOnEmit(t *testing.T) {
	startedAt := time.Unix(1_700_000_000, 0)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.ResetSessionSnapshotExecution("demo-task", "processing", startedAt)

	first := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, first)
	require.Equal(t, startedAt.Unix(), first.StartedAt)
	require.Equal(t, first.StartedAt, first.EndedAt)

	time.Sleep(10 * time.Millisecond)
	second := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, second)
	require.GreaterOrEqual(t, second.EndedAt, first.EndedAt)
	require.Greater(t, second.EndedAt, second.StartedAt)

	cfg.FinalizeSessionSnapshotExecution("completed", time.Unix(1_700_000_100, 0))
	final := cfg.BuildSessionSnapshotExecution(nil)
	require.Equal(t, "completed", final.Status)
	require.Equal(t, int64(1_700_000_100), final.EndedAt)

	time.Sleep(10 * time.Millisecond)
	afterFinal := cfg.BuildSessionSnapshotExecution(nil)
	require.Equal(t, final.EndedAt, afterFinal.EndedAt)
}

func TestSessionSnapshot_RiskLevelCount(t *testing.T) {
	originProjectDBPath := consts.GetCurrentProjectDatabasePath()
	require.NoError(t, consts.SetGormProjectDatabase(filepath.Join(t.TempDir(), "snapshot-risk.db")))
	t.Cleanup(func() {
		require.NoError(t, consts.SetGormProjectDatabase(originProjectDBPath))
	})

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)

	createRisk := func(runtimeId, severity, url string) {
		risk := &schema.Risk{
			RuntimeId: runtimeId, RiskType: "sqli", Severity: severity,
			Url: url, Title: "t-" + utils.RandStringBytes(5),
		}
		require.NoError(t, yakit.CreateOrUpdateRisk(db, utils.RandStringBytes(16), risk))
	}

	// 会话内两个 runtime 的风险必须合并统计，middle 计入 warning
	createRisk("snapshot-rt-a", "critical", "http://a/1")
	createRisk("snapshot-rt-a", "middle", "http://a/2")
	createRisk("snapshot-rt-b", "high", "http://b/1")
	// 不属于本会话的 runtime，不应进入统计
	createRisk("snapshot-rt-c", "critical", "http://c/1")

	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.ResetSessionSnapshotExecution("demo-task", "processing", time.Unix(1_700_000_000, 0))
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{Name: "grep", Success: true, ToolCallID: "snapshot-rt-a"})
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{Name: "fuzz", Success: true, ToolCallID: "snapshot-rt-b"})

	exec := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, exec)
	require.EqualValues(t, 1, exec.RiskLevelCount.Critical)
	require.EqualValues(t, 1, exec.RiskLevelCount.High)
	require.EqualValues(t, 1, exec.RiskLevelCount.Warning)
	require.EqualValues(t, 0, exec.RiskLevelCount.Low)
	require.EqualValues(t, 3, exec.RiskLevelCount.Total)
	require.Equal(t, int(exec.RiskLevelCount.Total), exec.RiskCount)

	raw, err := json.Marshal(exec)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"risk_level_count"`)

	// 重置后不得残留上一轮执行的等级计数
	cfg.ResetSessionSnapshotExecution("demo-task", "processing", time.Unix(1_700_000_000, 0))
	empty := cfg.BuildSessionSnapshotExecution(nil)
	require.NotNil(t, empty)
	require.EqualValues(t, 0, empty.RiskLevelCount.Total)
	require.Equal(t, 0, empty.RiskCount)
}
