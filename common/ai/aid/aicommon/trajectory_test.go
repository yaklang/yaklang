package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func newTrajectoryConfig(t *testing.T) *Config {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}, &schema.AITrajectory{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "trajectory-runtime", PersistentSession: "persistent-trajectory"}).Error)
	cfg := &Config{Id: "trajectory-runtime", PersistentSessionId: "persistent-trajectory", Ctx: context.Background(),
		m: new(sync.Mutex), Emitter: NewDummyEmitter(), BaseCheckpointableStorage: NewCheckpointableStorageWithDB("trajectory-runtime", db)}
	cfg.Timeline = NewTimeline(nil, nil)
	cfg.Timeline.SoftBindConfig(cfg, nil)
	return cfg
}

type trajectoryEvaluationCapture struct {
	mu      sync.Mutex
	records []schema.AITrajectory
}

func (c *trajectoryEvaluationCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.records)
}
func waitForTrajectoryWrites(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return trajectoryWrites.pending.Load() == 0 }, 3*time.Second, time.Millisecond)
}
func captureTrajectoryEvaluations(t *testing.T) *trajectoryEvaluationCapture {
	t.Helper()
	waitForTrajectoryWrites(t)
	trajectoryEvaluatorRegistry.RLock()
	previous := trajectoryEvaluatorRegistry.evaluator
	trajectoryEvaluatorRegistry.RUnlock()
	capture := &trajectoryEvaluationCapture{}
	RegisterTrajectoryEvaluator(func(_ *gorm.DB, _ bool, record *schema.AITrajectory) {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		capture.records = append(capture.records, *record)
	})
	t.Cleanup(func() { waitForTrajectoryWrites(t); RegisterTrajectoryEvaluator(previous) })
	return capture
}

func readyTrajectoryResponse(response string) TrajectoryResponseReceipt {
	return func(receive func(string)) { receive(response) }
}

func TestTrajectoryShortSessionAndContinuation(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	evaluations := captureTrajectoryEvaluations(t)
	// Local capture must retain full content even with an extremely small
	// Timeline display budget, and even in single-model mode.
	cfg.singleAIModelMode, cfg.singleAIModelModeResolved = true, true
	content := "BEGIN_FULL_TRAJECTORY " + strings.Repeat("original history ", 5000) + " END_FULL_TRAJECTORY"
	cfg.Timeline.PushText(1, "%s", content)
	cfg.Timeline.SetTimelineContentLimit(1)
	task := NewStatefulTaskBase("user-task", "original question", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, content)
	require.True(t, cfg.CollectTerminalTrajectory(task))
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 1, evaluations.count(), "duplicate task completion must not reevaluate")
	require.Contains(t, rows[0].Prompt, "END_FULL_TRAJECTORY")
	require.Empty(t, rows[0].Response)
	require.Equal(t, "original question", rows[0].UserQuery)
	id := rows[0].ID
	RecordTaskTrajectoryPrompt(task, "updated actual prompt")
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	rows, err = yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, id, rows[0].ID)
	require.Equal(t, "updated actual prompt", rows[0].Prompt)
	require.Empty(t, rows[0].Response)
}

func TestTrajectoryCompressionPlusTerminal(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	evaluations := captureTrajectoryEvaluations(t)
	for i, prompt := range []string{"first exact main-loop prompt", "second exact main-loop prompt"} {
		cfg.Timeline.PushText(int64(i+1), "observation %d", i)
		result, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "committed summary"}, prompt,
			readyTrajectoryResponse(`{"content":"full compression response","summary":"committed summary","retain_timeline":""}`))
		require.NoError(t, err)
		require.NotNil(t, result)
	}
	task := NewStatefulTaskBase("user-task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, 3, evaluations.count())
	require.Equal(t, "first exact main-loop prompt", rows[0].Prompt)
	require.Equal(t, "second exact main-loop prompt", rows[1].Prompt)
	require.JSONEq(t, `{"content":"full compression response","summary":"committed summary","retain_timeline":""}`, rows[0].Response)
	require.NotEmpty(t, rows[0].CompressionJSON)
	require.Equal(t, schema.AITrajectoryTerminal, rows[2].Kind)
	// Failed compression attempts produce no additional trajectory or score.
	cfg.Timeline.PushText(10, "must keep this history")
	_, err = cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{}, "invalid compression")
	require.Error(t, err)
	waitForTrajectoryWrites(t)
	rows, err = yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

func TestTrajectoryCompressionStorageFailureDoesNotAffectCompression(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	captureTrajectoryEvaluations(t)
	cfg.Timeline.PushText(1, "original must not disappear")
	require.NoError(t, cfg.GetDB().Exec("CREATE TRIGGER reject_trajectory BEFORE INSERT ON ai_trajectories BEGIN SELECT RAISE(ABORT, 'trajectory storage unavailable'); END").Error)
	_, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "summary"}, "exact prompt", readyTrajectoryResponse("full compression response"))
	require.NoError(t, err, "telemetry storage must not reject compression")
	require.NotContains(t, cfg.Timeline.Dump(), "original must not disappear")
	require.Contains(t, cfg.Timeline.Dump(), "summary")
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestTrajectoryCompressionKeepsSessionOwnerWhenNestedConfigBinds(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	evaluations := captureTrajectoryEvaluations(t)
	cfg.Timeline.PushText(1, "session observation")
	nested := &Config{Id: "nested-helper", Ctx: context.Background(), DisableCreateDBRuntime: true}
	cfg.Timeline.SoftBindConfig(nested, nil)
	_, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "nested-loop summary"}, "actual shared-timeline prompt", readyTrajectoryResponse("full compression response"))
	require.NoError(t, err)
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, cfg.GetRuntimeId(), rows[0].RuntimeID)
	require.Equal(t, cfg.PersistentSessionId, rows[0].PersistentSessionID)
	require.Equal(t, 1, evaluations.count())
}

func TestTrajectoryCompressionResponseFailureDoesNotAffectCommit(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	evaluations := captureTrajectoryEvaluations(t)
	for i, response := range []TrajectoryResponseReceipt{
		nil,
		readyTrajectoryResponse(""),
		func(func(string)) { panic("broken response receipt") },
	} {
		cfg.Timeline.PushText(int64(i+1), "source observation")
		_, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "committed summary"}, "exact prompt", response)
		require.NoError(t, err)
		require.Contains(t, cfg.Timeline.Dump(), "committed summary")
	}
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Empty(t, rows, "missing or broken response receipts must not fabricate trajectory data")
	require.Zero(t, evaluations.count())
}

func TestTrajectoryPendingCompressionResponseDoesNotOccupyWriters(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	evaluations := captureTrajectoryEvaluations(t)
	var receive func(string)
	source := strings.Repeat("complete original compression input ", 16000)
	cfg.Timeline.PushText(1, "%s", source)
	requireTrajectoryCollectionReturns(t, func() {
		_, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "summary"}, source,
			func(callback func(string)) { receive = callback })
		require.NoError(t, err)
	})
	require.NotNil(t, receive)
	require.Zero(t, trajectoryWrites.pending.Load(), "an incomplete provider must not occupy the database queue or its workers")
	task := NewStatefulTaskBase("user-task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "full terminal prompt")
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1, "terminal persistence is independent of a pending compression receipt")
	fullResponse := strings.Repeat("complete response reasoning and content ", 12000)
	receive(fullResponse)
	receive(fullResponse)
	waitForTrajectoryWrites(t)
	rows, err = yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 2, "the completed receipt is admitted only once")
	require.Equal(t, source, rows[0].Prompt)
	require.Equal(t, fullResponse, rows[0].Response)
	require.Equal(t, 2, evaluations.count())
}

func TestTrajectoryIgnoresInternalTasksAndDisabledPersistence(t *testing.T) {
	for _, mode := range []string{"subagent", "branch", "disabled", "unfinished", "no_prompt"} {
		t.Run(mode, func(t *testing.T) {
			cfg := newTrajectoryConfig(t)
			evaluations := captureTrajectoryEvaluations(t)
			task := NewStatefulTaskBase("task", "input", context.Background(), nil)
			if mode != "unfinished" {
				task.SetStatus(AITaskState_Completed)
				RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
			}
			switch mode {
			case "subagent":
				task.SetSubAgent(true)
			case "branch":
				cfg.Timeline.markBranchTimeline(true)
			case "no_prompt":
				RecordTaskTrajectoryPrompt(task, "")
			case "disabled":
				cfg.DisableCreateDBRuntime = true
			}
			require.False(t, cfg.CollectTerminalTrajectory(task))
			waitForTrajectoryWrites(t)
			rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
			require.NoError(t, err)
			require.Empty(t, rows)
			require.Zero(t, evaluations.count())
		})
	}
}

func requireTrajectoryCollectionReturns(t *testing.T, collect func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); collect() }()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("trajectory collection blocked the foreground")
	}
}

func TestTrajectoryCollectionNeverWaitsForDatabase(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	captureTrajectoryEvaluations(t)
	task := NewStatefulTaskBase("task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
	// Occupy the only database connection. Any foreground DB call would wait
	// forever until this test releases it, including a seemingly harmless query.
	tx := cfg.GetDB().Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	requireTrajectoryCollectionReturns(t, func() { cfg.CollectTerminalTrajectory(task) })
	requireTrajectoryCollectionReturns(t, func() { cfg.ResumeTrajectoryEvaluations() })
	require.NoError(t, tx.Rollback().Error)
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestTrajectoryTerminalSurvivesBusySourceLocks(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	captureTrajectoryEvaluations(t)
	task := NewStatefulTaskBase("completed-task", "long task", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	prompt := strings.Repeat("complete terminal context ", 20000) + "TERMINAL_CONTEXT_END"
	RecordTaskTrajectoryPrompt(task, prompt)
	admitted := false
	func() {
		task.statusMu.Lock()
		defer task.statusMu.Unlock()
		cfg.Timeline.mu.Lock()
		defer cfg.Timeline.mu.Unlock()
		requireTrajectoryCollectionReturns(t, func() { admitted = cfg.CollectTerminalTrajectory(task) })
	}()
	require.True(t, admitted, "ordinary lifecycle/timeline contention must not discard the final prompt")
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, prompt, rows[0].Prompt)
}

func TestTrajectoryStatusSnapshotTracksDeferredAndRestoredLifecycle(t *testing.T) {
	task := NewStatefulTaskBase("lifecycle", "goal", context.Background(), nil)
	check := func(expected AITaskState) {
		task.statusMu.Lock()
		defer task.statusMu.Unlock()
		status, ok := task.TryGetStatus()
		require.True(t, ok)
		require.Equal(t, expected, status)
	}
	check(AITaskState_Created)
	task.SetStatus(AITaskState_Processing)
	release := task.DeferCompletion()
	task.SetStatus(AITaskState_Completed)
	check(AITaskState_Processing)
	release()
	check(AITaskState_Completed)
	task.ForceSetStatus(AITaskState_Processing)
	check(AITaskState_Processing)
	task.RestoreStatus(AITaskState_Skipped)
	check(AITaskState_Skipped)
}

func TestTrajectoryCompressionOnPrivateTaskTimelinesIsRetained(t *testing.T) {
	for _, private := range []bool{false, true} {
		t.Run(map[bool]string{false: "fork", true: "private"}[private], func(t *testing.T) {
			cfg := newTrajectoryConfig(t)
			evaluations := captureTrajectoryEvaluations(t)
			worker := &Config{Id: "worker-without-own-runtime", Ctx: context.Background(), DisableCreateDBRuntime: true, Timeline: cfg.Timeline}
			var branch *Timeline
			if private {
				branch = NewPrivateTimeline(worker, nil)
				branch.SoftBindConfig(worker, worker)
			} else {
				fork, err := cfg.Timeline.ForkForTask("worker", "long worker", worker, worker)
				require.NoError(t, err)
				branch = fork.Branch
			}
			source := strings.Repeat("original private-task observation ", 16000) + "PRIVATE_SOURCE_END"
			response := strings.Repeat("complete compression response ", 2500) + "PRIVATE_RESPONSE_END"
			branch.PushText(1, "%s", source)
			_, err := branch.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "worker summary"}, source, readyTrajectoryResponse(response))
			require.NoError(t, err)
			waitForTrajectoryWrites(t)
			rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
			require.NoError(t, err)
			require.Len(t, rows, 1, "a worker's successful compression belongs to its persistent session")
			require.Equal(t, cfg.Id, rows[0].RuntimeID)
			require.Equal(t, source, rows[0].Prompt)
			require.Equal(t, response, rows[0].Response)
			require.Equal(t, 1, evaluations.count())
			// A worker may settle after its parent session has been deleted.
			// Its inherited owner must not resurrect either runtime or payloads.
			require.NoError(t, cfg.GetDB().Unscoped().Where("uuid = ?", cfg.Id).Delete(&schema.AIAgentRuntime{}).Error)
			require.NoError(t, yakit.DeleteAITrajectories(cfg.GetDB(), cfg.PersistentSessionId))
			branch.PushText(2, "late worker observation")
			_, err = branch.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "late summary"}, "late worker prompt", readyTrajectoryResponse("late response"))
			require.NoError(t, err)
			waitForTrajectoryWrites(t)
			rows, err = yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
			require.NoError(t, err)
			require.Empty(t, rows)
			require.Equal(t, 1, evaluations.count())
		})
	}
}

func TestTrajectoryCollectionDropsInsteadOfWaitingForFullQueue(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	waitForTrajectoryWrites(t)
	task := NewStatefulTaskBase("task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
	originalPool := trajectoryWrites
	fullPool := &trajectoryWritePool{queue: make(chan trajectoryWrite, 1)}
	fullPool.queue <- trajectoryWrite{}
	trajectoryWrites = fullPool
	defer func() { trajectoryWrites = originalPool }()
	requireTrajectoryCollectionReturns(t, func() { require.False(t, cfg.CollectTerminalTrajectory(task)) })
}

type panickingTrajectoryTask struct{ *AIStatefulTaskBase }

func (panickingTrajectoryTask) GetOriginUserInput() string { panic("broken telemetry source") }

func TestTrajectoryCollectionAndWorkerPanicsAreIsolated(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	captureTrajectoryEvaluations(t)
	task := NewStatefulTaskBase("task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
	require.NotPanics(t, func() {
		require.False(t, cfg.CollectTerminalTrajectory(panickingTrajectoryTask{task}))
	})
	RegisterTrajectoryEvaluator(func(*gorm.DB, bool, *schema.AITrajectory) { panic("broken evaluation observer") })
	// A malformed/closed database implementation must also be contained by
	// the fixed worker boundary, not terminate the process.
	trajectoryWrites.enqueue(trajectoryWrite{db: &gorm.DB{}, record: schema.AITrajectory{
		PersistentSessionID: "panic-db-session", TaskID: "task", Kind: schema.AITrajectoryTerminal}})
	waitForTrajectoryWrites(t)
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	RecordTaskTrajectoryPrompt(task, "updated exact decision prompt")
	require.True(t, cfg.CollectTerminalTrajectory(task))
	waitForTrajectoryWrites(t)
	rows, err := yakit.QueryAITrajectories(cfg.GetDB(), cfg.PersistentSessionId)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "updated exact decision prompt", rows[0].Prompt, "a panicking observer must not kill the write workers")
}

func TestTrajectoryCollectionNeverWaitsForSlowEvaluator(t *testing.T) {
	cfg := newTrajectoryConfig(t)
	captureTrajectoryEvaluations(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer func() { releaseOnce.Do(func() { close(release) }); waitForTrajectoryWrites(t) }()
	entered := make(chan struct{}, 2)
	RegisterTrajectoryEvaluator(func(*gorm.DB, bool, *schema.AITrajectory) { entered <- struct{}{}; <-release })
	for range 2 {
		cfg.compressionTrajectoryNotification("source", &TimelineCompressionResult{Summary: "summary"}, readyTrajectoryResponse("full compression response"))()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("write workers did not reach evaluator")
		}
	}
	task := NewStatefulTaskBase("task", "goal", context.Background(), nil)
	task.SetStatus(AITaskState_Completed)
	RecordTaskTrajectoryPrompt(task, "last exact decision prompt")
	requireTrajectoryCollectionReturns(t, func() {
		for range 100 {
			cfg.CollectTerminalTrajectory(task)
		}
	})
	// Subsequent callbacks need not report their entry once the gate opens.
	RegisterTrajectoryEvaluator(func(*gorm.DB, bool, *schema.AITrajectory) {})
	releaseOnce.Do(func() { close(release) })
	waitForTrajectoryWrites(t)
}
