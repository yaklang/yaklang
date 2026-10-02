package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCoordinatorChannelsNeverConstructTheOtherRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	original := newCoordinatorContextForPlanExec
	t.Cleanup(func() { newCoordinatorContextForPlanExec = original })
	legacyCalls := 0
	legacyResult := errors.New("legacy route selected")
	newCoordinatorContextForPlanExec = func(ctx context.Context, _ string, opts ...aicommon.ConfigOption) (*coordinator_legacy.Coordinator, error) {
		legacyCalls++
		cfg := aicommon.NewConfig(ctx, opts...)
		require.Nil(t, cfg.LiteForgeExecutor, "legacy must not inherit native helper injection")
		return nil, legacyResult
	}
	var calls atomic.Int64
	r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithFocus(coordinator.Name), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeYOLO(), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		if wire.ToolCallCallback == nil || wire.FinishReasonCallback == nil {
			return nil, fmt.Errorf("native channel emitted a text request")
		}
		name, args := "finish", map[string]any{}
		switch calls.Add(1) {
		case 1:
			name = "create_plan"
			args = map[string]any{"plan": map[string]any{"name": "Plan", "goal": "Approve only", "tasks": []any{map[string]any{"name": "Check", "goal": "Execute later", "identifier": "check", "depends_on": []string{}}}}, "plan_document": "# Independent document"}
		case 2:
			name = "submit_plan"
			args = map[string]any{"plan_version": 1}
		}
		data, _ := json.Marshal(args)
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: "native-call", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
		wire.FinishReasonCallback("tool_calls", nil)
		response := c.NewAIResponse()
		response.Close()
		return response, nil
	}))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("route-test", "Prepare a plan", ctx, r.Emitter, true)
	done := make(chan struct{})
	require.NoError(t, r.invokePlanOnly(done, ctx, WithInvokePlanAndExecuteTask(task), WithInvokePlanAndExecutePlanPayload(task.GetUserInput())))
	select {
	case <-done:
	default:
		t.Fatal("native ready channel was not closed")
	}
	require.Equal(t, int64(3), calls.Load())
	require.Zero(t, legacyCalls)
	for _, focus := range []string{"", "plan", coordinator_legacy.Name, coordinator.Name} {
		r.config.Focus = focus
		channel, err := r.coordinatorChannel("")
		require.NoError(t, err)
		require.Equal(t, coordinator.Name, channel)
	}
	require.Zero(t, legacyCalls)
	require.Equal(t, int64(3), calls.Load())
	metadata, ok := reactloops.GetLoopMetadata(coordinator.Name)
	require.True(t, ok)
	require.False(t, metadata.IsHidden)
	_, ok = reactloops.GetLoopMetadata(coordinator_legacy.Name)
	require.False(t, ok, "retired focus must not be registered")
}

func TestCoordinatorRecoverySelectsStoredOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()))
	cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(cfg.Id, db)
	r := &ReAct{config: cfg}
	for _, test := range []struct{ id, progress, expected string }{{"old", `{"phase":"NotCompleted"}`, coordinator_legacy.Name}, {"old-named", `{"plan_engine":"coordinator_legacy"}`, coordinator_legacy.Name}, {"native", `{"plan_engine":"coordinator"}`, coordinator.Name}, {"native-snapshot", `{"coordinator_state":{"schema":1}}`, coordinator.Name}} {
		require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, &schema.AISessionPlanAndExec{SessionID: "session", CoordinatorID: test.id, TaskTree: "{}", TaskProgress: test.progress}))
		for _, current := range []string{coordinator.Name, coordinator_legacy.Name} {
			cfg.Focus = current
			channel, err := r.coordinatorChannel(test.id)
			if test.expected == coordinator_legacy.Name {
				require.ErrorContains(t, err, "legacy PLAN execution is disabled")
				continue
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, channel)
		}
	}
	for _, progress := range []string{`{"plan_engine":"unsupported"}`, `{"plan_engine":"unsupported","coordinator_state":{"schema":1}}`, `{"plan_engine":"coordinator_legacy","coordinator_state":{"schema":1}}`} {
		require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, &schema.AISessionPlanAndExec{SessionID: "session", CoordinatorID: "invalid", TaskTree: "{}", TaskProgress: progress}))
		_, err = r.coordinatorChannel("invalid")
		require.Error(t, err)
	}
}

func TestCoordinatorNativeReviewAndDetachedAdapters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	original := newCoordinatorContextForPlanExec
	t.Cleanup(func() { newCoordinatorContextForPlanExec = original })
	newCoordinatorContextForPlanExec = func(context.Context, string, ...aicommon.ConfigOption) (*coordinator_legacy.Coordinator, error) {
		return nil, errors.New("native adapter called legacy constructor")
	}
	in := make(chan *ypb.AIInputEvent, 4)
	var mu sync.Mutex
	var panel map[string]any
	cfg := aicommon.NewConfig(ctx, aicommon.WithFocus(coordinator.Name), aicommon.WithPersistentSessionId(utils.RandStringBytes(30)), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableAutoSkills(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeYOLO(), aicommon.WithEventInputChan(in), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, errors.New("review/publish adapter must not request a model")
	}), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			var payload map[string]any
			if json.Unmarshal(e.Content, &payload) == nil {
				in <- &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: fmt.Sprint(payload["id"]), InteractiveJSONInput: `{"suggestion":"continue","plans":{"document":"Edited document","root_task":{"name":"Edited plan","goal":"Approved scope","subtasks":[{"name":"Check","goal":"Edited task brief","identifier":"check"}]}}}`}
			}
		}
		if e.Type == schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE {
			mu.Lock()
			defer mu.Unlock()
			_ = json.Unmarshal(e.Content, &panel)
		}
	}))
	configureCoordinatorChannel(cfg)
	cfg.StartEventLoop(ctx)
	require.NoError(t, cfg.GetDB().AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	r := &ReAct{config: cfg, Emitter: cfg.GetEmitter(), taskQueue: NewTaskQueue(MainTaskQueueName)}
	input := &aicommon.ExecutePlanInput{PlanPayload: "User request", PlanData: `{"name":"Plan","goal":"Original scope","tasks":[{"name":"Check","goal":"Original task brief","identifier":"check"}]}`, PlanDocument: "Original document"}
	review, err := r.BeginPlanCoordinatorSession(ctx, input, true)
	require.NoError(t, err)
	t.Cleanup(review.Close)
	require.NoError(t, review.ReviewPlan(ctx))
	require.Equal(t, "Edited document", review.ApprovedPlanInput().PlanDocument)
	require.Contains(t, review.ApprovedPlanInput().PlanData, "Edited task brief")
	channel, err := r.coordinatorChannel(review.CoordinatorID())
	require.NoError(t, err)
	require.Equal(t, coordinator.Name, channel)
	review.Close()

	id, err := r.PublishDetachedPlan(ctx, input, "explicit-parent-task")
	require.NoError(t, err)
	mu.Lock()
	require.Equal(t, "explicit-parent-task", panel["re-act_task"])
	require.Equal(t, input.PlanPayload, panel["plan_payload"])
	mu.Unlock()
	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(cfg.GetDB(), id)
	require.NoError(t, err)
	require.Contains(t, record.TaskProgress, `"plan_document":"Original document"`)
	require.Contains(t, record.TaskProgress, `"react_task_id":"explicit-parent-task"`)
	// The queue has no consumer: inspect the actual recovery input before any
	// worker can start. Clients need only return the coordinator ID.
	require.NoError(t, r.HandleSyncTypeExecuteDetachedPlanEvent(&ypb.AIInputEvent{SyncJsonInput: fmt.Sprintf(`{"coordinator_id":%q}`, id), SyncID: "native-approval"}))
	queued := r.taskQueue.GetQueueingTasks()
	require.Len(t, queued, 1)
	recovery := queued[0].GetRecoveryData()
	require.Equal(t, id, recovery.CoordinatorID)
	require.Equal(t, input.PlanDocument, recovery.ExecutePlanInput.PlanDocument)
	require.Contains(t, recovery.ExecutePlanInput.PlanData, "Original task brief")
}
