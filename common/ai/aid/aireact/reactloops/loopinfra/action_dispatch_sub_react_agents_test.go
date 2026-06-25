package loopinfra

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type dispatchSubReactTestInvoker struct {
	*mock.MockInvoker
	mu              sync.Mutex
	timelineEntries []struct {
		entry   string
		content string
	}
}

func newDispatchSubReactTestInvoker(ctx context.Context) *dispatchSubReactTestInvoker {
	return &dispatchSubReactTestInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
}

func (t *dispatchSubReactTestInvoker) AddToTimeline(entry, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timelineEntries = append(t.timelineEntries, struct {
		entry   string
		content string
	}{entry: entry, content: content})
}

func (t *dispatchSubReactTestInvoker) timelineDump() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var b strings.Builder
	for _, item := range t.timelineEntries {
		b.WriteString(item.entry)
		b.WriteString(": ")
		b.WriteString(item.content)
		b.WriteString("\n")
	}
	return b.String()
}

func (t *dispatchSubReactTestInvoker) subReactTimelineRecords() []subReactAgentTimelineRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []subReactAgentTimelineRecord
	for _, item := range t.timelineEntries {
		if item.entry != schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT {
			continue
		}
		var record subReactAgentTimelineRecord
		if err := json.Unmarshal([]byte(item.content), &record); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out
}

func mustBuildDispatchSubReactAction(t *testing.T, params map[string]any) *aicommon.Action {
	t.Helper()
	invokeParams := make(aitool.InvokeParams)
	for k, v := range params {
		invokeParams[k] = v
	}
	return aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS, invokeParams)
}

func dispatchSubReactJobs(items ...map[string]any) []map[string]any {
	return items
}

func TestVerifyDispatchSubReactAgents_RejectsSubAgentDepth(t *testing.T) {
	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.Set(subAgentDepthLoopVar, 1)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(map[string]any{"goal": "analyze logs"}),
	})
	err := verifyDispatchSubReactAgents(loop, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top-level agent")
}

func TestVerifyDispatchSubReactAgents_RejectsUnknownLoop(t *testing.T) {
	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(map[string]any{
			"goal":      "analyze logs",
			"loop_name": "not-a-real-loop",
		}),
	})
	err := verifyDispatchSubReactAgents(loop, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestVerifyDispatchSubReactAgents_AcceptsValidPayload(t *testing.T) {
	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(
			map[string]any{"identifier": "scan_a", "goal": "scan service A"},
			map[string]any{"identifier": "scan_b", "goal": "scan service B"},
		),
		"concurrency": 2,
	})
	err := verifyDispatchSubReactAgents(loop, action)
	require.NoError(t, err)
	assert.Contains(t, loop.Get(dispatchSubReactJobsLoopKey), "scan_a")
	assert.Equal(t, 2, loop.GetInt(dispatchSubReactConcurrencyLoopKey))
}

type mockSubReactAgentRunner struct {
	mu    sync.Mutex
	calls []subReactDispatchJob
	delay time.Duration
}

func (m *mockSubReactAgentRunner) Run(
	_ aicommon.AIInvokeRuntime,
	_ *reactloops.ReActLoop,
	_ aicommon.AIStatefulTask,
	job subReactDispatchJob,
) (*subReactAgentJobResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, job)
	m.mu.Unlock()

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	return &subReactAgentJobResult{
		Order: job.Order,
		Job:   job,
		Record: subReactAgentTimelineRecord{
			SubAgentID: "mock-" + job.Identifier,
			Order:      job.Order,
			LoopName:   job.LoopName,
			Goal:       job.Goal,
			Status:     "completed",
			Result:     "done:" + job.Identifier,
			ProcessStats: subReactProcessStats{
				Iterations:    2,
				Actions:       3,
				ToolCalls:     1,
				TimelineItems: 4,
			},
		},
		Feedback: "ok:" + job.Identifier,
	}, nil
}

func TestHandleDispatchSubReactAgents_WritesOneTimelineRecordPerAgent(t *testing.T) {
	origRunner := subReactAgentRunner
	defer func() { subReactAgentRunner = origRunner }()

	mockRunner := &mockSubReactAgentRunner{}
	subReactAgentRunner = mockRunner

	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("parent-task", "parent input", context.Background(), invoker.GetConfig().GetEmitter(), true)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(
			map[string]any{"identifier": "agent_a", "goal": "task A"},
			map[string]any{"identifier": "agent_b", "goal": "task B"},
		),
	})
	require.NoError(t, verifyDispatchSubReactAgents(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	handleDispatchSubReactAgents(loop, action, op)

	require.True(t, op.IsContinued())
	records := invoker.subReactTimelineRecords()
	require.Len(t, records, 2)
	assert.Equal(t, 1, records[0].Order)
	assert.Equal(t, 2, records[1].Order)
	assert.Equal(t, "completed", records[0].Status)
	assert.Equal(t, 2, records[0].ProcessStats.Iterations)
	assert.Equal(t, 3, records[0].ProcessStats.Actions)
	assert.Equal(t, 1, records[0].ProcessStats.ToolCalls)
	assert.Contains(t, op.GetFeedback().String(), "agent_a")
	assert.Contains(t, op.GetFeedback().String(), "agent_b")
}

func TestRunDispatchSubReactJobsConcurrently_PreservesInputOrderInResults(t *testing.T) {
	origRunner := subReactAgentRunner
	defer func() { subReactAgentRunner = origRunner }()

	mockRunner := &mockSubReactAgentRunner{delay: 20 * time.Millisecond}
	subReactAgentRunner = mockRunner

	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("parent-task", "parent input", context.Background(), invoker.GetConfig().GetEmitter(), true)

	jobs := []subReactDispatchJob{
		{Order: 1, Identifier: "slow_a", Goal: "A", LoopName: schema.AI_REACT_LOOP_NAME_DEFAULT},
		{Order: 2, Identifier: "slow_b", Goal: "B", LoopName: schema.AI_REACT_LOOP_NAME_DEFAULT},
	}

	results := runDispatchSubReactJobsConcurrently(invoker, loop, task, jobs, 2)
	require.Len(t, results, 2)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
	orders := make([]int, 0, len(results))
	for _, result := range results {
		orders = append(orders, result.Order)
	}
	assert.Equal(t, []int{1, 2}, orders)
}

func TestNewReActLoop_InjectsDispatchSubReactAgents(t *testing.T) {
	// Dispatch sub react agents is gated by EnableDispatchSubReactAgents on the
	// real *aicommon.Config; NewReActLoop only injects the action when the flag is on.
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableDispatchSubReactAgent(true),
		aicommon.WithDisableAutoSkills(true),
	)
	invoker := &configBackedDispatchInvoker{
		dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(context.Background()),
		cfg:                         cfg,
	}
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, invoker)
	require.NoError(t, err)

	action, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS)
	require.NoError(t, err)
	require.NotNil(t, action)
}

func TestNewReActLoop_OmitsDispatchSubReactAgentsWhenDisabled(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
	)
	require.False(t, cfg.EnableDispatchSubReactAgents)
	invoker := &configBackedDispatchInvoker{
		dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(context.Background()),
		cfg:                         cfg,
	}
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, invoker)
	require.NoError(t, err)

	_, err = loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS)
	require.Error(t, err)
}

func TestBuildSubReactLoopOptions_FiltersDispatchAction(t *testing.T) {
	opts := buildSubReactLoopOptions(subReactDispatchJob{MaxIterations: 3})
	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		newDispatchSubReactTestInvoker(context.Background()),
		opts...,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loop.GetInt(subAgentDepthLoopVar))

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(map[string]any{"goal": "nested dispatch"}),
	})
	err = verifyDispatchSubReactAgents(loop, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top-level agent")
}

type partialFailSubReactRunner struct {
	failIdentifiers map[string]struct{}
}

func (m *partialFailSubReactRunner) Run(
	_ aicommon.AIInvokeRuntime,
	_ *reactloops.ReActLoop,
	_ aicommon.AIStatefulTask,
	job subReactDispatchJob,
) (*subReactAgentJobResult, error) {
	if _, ok := m.failIdentifiers[job.Identifier]; ok {
		return &subReactAgentJobResult{
			Order: job.Order,
			Job:   job,
			Record: subReactAgentTimelineRecord{
				SubAgentID: "mock-" + job.Identifier,
				Order:      job.Order,
				LoopName:   job.LoopName,
				Goal:       job.Goal,
				Status:     "failed",
				Error:      "simulated failure",
			},
			Feedback: "fail:" + job.Identifier,
		}, nil
	}
	return &subReactAgentJobResult{
		Order: job.Order,
		Job:   job,
		Record: subReactAgentTimelineRecord{
			SubAgentID: "mock-" + job.Identifier,
			Order:      job.Order,
			LoopName:   job.LoopName,
			Goal:       job.Goal,
			Status:     "completed",
			Result:     "done:" + job.Identifier,
		},
		Feedback: "ok:" + job.Identifier,
	}, nil
}

func TestHandleDispatchSubReactAgents_PartialFailureContinues(t *testing.T) {
	origRunner := subReactAgentRunner
	defer func() { subReactAgentRunner = origRunner }()

	subReactAgentRunner = &partialFailSubReactRunner{
		failIdentifiers: map[string]struct{}{"agent_b": {}},
	}

	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("parent-task", "parent input", context.Background(), invoker.GetConfig().GetEmitter(), true)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(
			map[string]any{"identifier": "agent_a", "goal": "task A"},
			map[string]any{"identifier": "agent_b", "goal": "task B"},
		),
	})
	require.NoError(t, verifyDispatchSubReactAgents(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	handleDispatchSubReactAgents(loop, action, op)

	require.True(t, op.IsContinued())
	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "1 succeeded, 1 failed")
	assert.Contains(t, feedback, "agent_a")
	assert.Contains(t, feedback, "agent_b")

	records := invoker.subReactTimelineRecords()
	require.Len(t, records, 2)
	assert.Equal(t, "completed", records[0].Status)
	assert.Equal(t, "failed", records[1].Status)
}

func TestHandleDispatchSubReactAgents_PreservesInputOrderInTimeline(t *testing.T) {
	origRunner := subReactAgentRunner
	defer func() { subReactAgentRunner = origRunner }()

	subReactAgentRunner = &mockSubReactAgentRunner{delay: 30 * time.Millisecond}

	invoker := newDispatchSubReactTestInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("parent-task", "parent input", context.Background(), invoker.GetConfig().GetEmitter(), true)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(
			map[string]any{"identifier": "slow_b", "goal": "task B"},
			map[string]any{"identifier": "slow_a", "goal": "task A"},
		),
		"concurrency": 2,
	})
	require.NoError(t, verifyDispatchSubReactAgents(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	handleDispatchSubReactAgents(loop, action, op)
	require.True(t, op.IsContinued())

	records := invoker.subReactTimelineRecords()
	require.Len(t, records, 2)
	assert.Equal(t, 1, records[0].Order)
	assert.Equal(t, "task B", records[0].Goal)
	assert.Equal(t, 2, records[1].Order)
	assert.Equal(t, "task A", records[1].Goal)
}

type configBackedDispatchInvoker struct {
	*dispatchSubReactTestInvoker
	cfg *aicommon.Config
}

func (c *configBackedDispatchInvoker) GetConfig() aicommon.AICallerConfigIf {
	return c.cfg
}

type forkIsolationSubReactRunner struct {
	branchSecrets []string
}

func (r *forkIsolationSubReactRunner) Run(
	parentInvoker aicommon.AIInvokeRuntime,
	_ *reactloops.ReActLoop,
	_ aicommon.AIStatefulTask,
	job subReactDispatchJob,
) (*subReactAgentJobResult, error) {
	parentCfg, ok := parentInvoker.GetConfig().(*aicommon.Config)
	if !ok || parentCfg == nil || parentCfg.GetTimeline() == nil {
		return nil, utils.Error("timeline isolation test requires *aicommon.Config with timeline")
	}

	fork, err := parentCfg.GetTimeline().ForkForTask(job.Identifier, job.Identifier, parentCfg, parentCfg)
	if err != nil {
		return nil, err
	}
	secret := "branch-only-secret-" + job.Identifier
	fork.Branch.PushText(parentCfg.AcquireId(), secret)
	r.branchSecrets = append(r.branchSecrets, secret)

	return &subReactAgentJobResult{
		Order: job.Order,
		Job:   job,
		Record: subReactAgentTimelineRecord{
			SubAgentID: "fork-" + job.Identifier,
			Order:      job.Order,
			LoopName:   job.LoopName,
			Goal:       job.Goal,
			Status:     "completed",
			Result:     "isolated:" + job.Identifier,
			ProcessStats: subReactProcessStats{
				TimelineItems: 1,
			},
		},
		Feedback: "ok:" + job.Identifier,
	}, nil
}

func TestHandleDispatchSubReactAgents_BranchWritesDoNotPolluteParentTimeline(t *testing.T) {
	origRunner := subReactAgentRunner
	defer func() { subReactAgentRunner = origRunner }()

	forkRunner := &forkIsolationSubReactRunner{}
	subReactAgentRunner = forkRunner

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithTimeline(parentTimeline),
		aicommon.WithDisableAutoSkills(true),
	)

	baseInvoker := newDispatchSubReactTestInvoker(context.Background())
	invoker := &configBackedDispatchInvoker{
		dispatchSubReactTestInvoker: baseInvoker,
		cfg:                         cfg,
	}

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("parent-task", "parent input", context.Background(), cfg.GetEmitter(), true)

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(map[string]any{
			"identifier": "agent_a",
			"goal":       "task A",
		}),
	})
	require.NoError(t, verifyDispatchSubReactAgents(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	handleDispatchSubReactAgents(loop, action, op)
	require.True(t, op.IsContinued())

	parentDump := parentTimeline.Dump()
	for _, secret := range forkRunner.branchSecrets {
		assert.NotContains(t, parentDump, secret)
	}
	assert.Contains(t, parentDump, "parent-seed")

	records := invoker.subReactTimelineRecords()
	require.Len(t, records, 1)
	assert.Equal(t, "completed", records[0].Status)
}

func TestWriteDispatchSubReactDispatchesDisplayStream_FormatsJobs(t *testing.T) {
	input := `[{"identifier":"scan_a","goal":"scan service A"},{"identifier":"scan_b","goal":"scan service B","loop_name":"default"}]`
	var out strings.Builder
	require.NoError(t, writeDispatchSubReactDispatchesDisplayStream(strings.NewReader(input), &out))
	assert.Equal(t, "- [scan_a] scan service A\n- [scan_b] scan service B", out.String())
}

func TestDispatchSubReactAgents_StreamFieldsUseI18nNodeIDs(t *testing.T) {
	require.NotNil(t, loopAction_DispatchSubReactAgents)
	require.Len(t, loopAction_DispatchSubReactAgents.StreamFields, 2)
	assert.Equal(t, loopInfraNodeDispatchSubReact, loopAction_DispatchSubReactAgents.StreamFields[0].AINodeId)
	assert.Equal(t, loopInfraNodeDispatchConcurrency, loopAction_DispatchSubReactAgents.StreamFields[1].AINodeId)

	zh := schema.NodeIdToI18n(loopInfraNodeDispatchSubReact, true)
	require.NotNil(t, zh)
	assert.Equal(t, "下发子 Agent", zh.Zh)
	assert.Equal(t, "Dispatch Sub Agents", zh.En)
}
