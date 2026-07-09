package loopinfra

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/subagent"
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
	loop.Set(subagent.DepthLoopVar, 1)

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
	opts := buildSubReactLoopOptions()
	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		newDispatchSubReactTestInvoker(context.Background()),
		opts...,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loop.GetInt(subagent.DepthLoopVar))

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

// subReactEmitterCapture wraps a capturing baseEmitter so tests can observe every
// AiOutputEvent that reaches the parent sink, which is exactly what the frontend sees.
type subReactEmitterCapture struct {
	mu     sync.Mutex
	events []*schema.AiOutputEvent
}

func newCapturingSubReactEmitter(id string) (*aicommon.Emitter, *subReactEmitterCapture) {
	c := &subReactEmitterCapture{}
	emitter := aicommon.NewEmitter(id, func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		c.mu.Lock()
		c.events = append(c.events, e)
		c.mu.Unlock()
		return e, nil
	})
	return emitter, c
}

func (c *subReactEmitterCapture) snapshot() []*schema.AiOutputEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*schema.AiOutputEvent, len(c.events))
	copy(out, c.events)
	return out
}

// TestBuildSubReactForwardingEmitter_ForwardsEventsToParentAndStampsTaskId verifies the
// core dispatch contract: a sub-agent emitter forwards its events to the parent emitter
// (so they reach the frontend) and stamps every event's TaskId with the sub-task id, which
// is the marker the frontend uses to aggregate sub-agent messages.
func TestBuildSubReactForwardingEmitter_ForwardsEventsToParentAndStampsTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	const subTaskId = "sub-agent-xyz"
	subEmitter := BuildSubReactForwardingEmitter(parentEmitter, subTaskId)
	require.NotNil(t, subEmitter)
	require.NotSame(t, parentEmitter, subEmitter, "sub-agent must get its own derived emitter")

	_, err := subEmitter.EmitStatus("fuzz-status", "running")
	require.NoError(t, err)
	_, err = subEmitter.EmitSchema("sub-react-node", map[string]any{"k": "v"})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 2, "sub-agent events must be forwarded to the parent emitter")
	for _, e := range events {
		assert.Equal(t, subTaskId, e.TaskId, "every forwarded event must carry the sub-task id as aggregation marker")
	}
}

// TestBuildSubReactForwardingEmitter_DistinguishesSubAgentsByTaskId verifies that
// multiple sub agents sharing one parent emitter stay distinguishable: each event is
// tagged with the id of the sub agent that produced it.
func TestBuildSubReactForwardingEmitter_DistinguishesSubAgentsByTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	subA := BuildSubReactForwardingEmitter(parentEmitter, "sub-A")
	subB := BuildSubReactForwardingEmitter(parentEmitter, "sub-B")
	require.NotNil(t, subA)
	require.NotNil(t, subB)

	_, _ = subA.EmitSchema("agent-a-node", map[string]any{"i": 1})
	_, _ = subB.EmitSchema("agent-b-node", map[string]any{"i": 2})
	_, _ = subA.EmitSchema("agent-a-node", map[string]any{"i": 3})

	events := capture.snapshot()
	require.Len(t, events, 3)
	var aCount, bCount int
	for _, e := range events {
		switch e.TaskId {
		case "sub-A":
			aCount++
		case "sub-B":
			bCount++
		default:
			t.Fatalf("unexpected TaskId %q: sub-agent events must be tagged with their own sub-task id", e.TaskId)
		}
	}
	assert.Equal(t, 2, aCount)
	assert.Equal(t, 1, bCount)
}

// TestBuildSubReactForwardingEmitter_OverridesPreExistingTaskId verifies the sub-task id
// stamp is authoritative: even events that already carry a stale (e.g. parent) TaskId end
// up tagged with the sub-task id, so the frontend never mis-aggregates them under the parent.
func TestBuildSubReactForwardingEmitter_OverridesPreExistingTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	sub := BuildSubReactForwardingEmitter(parentEmitter, "sub-fresh")
	require.NotNil(t, sub)

	_, err := sub.Emit(&schema.AiOutputEvent{NodeId: "raw", TaskId: "stale-parent-id"})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "sub-fresh", events[0].TaskId, "sub-task id must override any pre-existing TaskId")
}

// TestBuildSubReactForwardingEmitter_RunsParentProcessorsOnForwardedEvents verifies the
// forwarded emitter threads events through the parent emitter's processor chain (not just
// the sink), so parent-level metadata (i18n, AI info, process association, ...) still applies
// to sub-agent events.
func TestBuildSubReactForwardingEmitter_RunsParentProcessorsOnForwardedEvents(t *testing.T) {
	capture := &subReactEmitterCapture{}
	parentEmitter := aicommon.NewEmitter("parent", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.mu.Lock()
		capture.events = append(capture.events, e)
		capture.mu.Unlock()
		return e, nil
	})
	// parent processor that tags metadata, like the coordinator's stamp / AIInfo provider do.
	parentEmitter = parentEmitter.PushEventProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
		if e != nil {
			e.AIService = "parent-meta"
		}
		return e
	})

	sub := BuildSubReactForwardingEmitter(parentEmitter, "sub-X")
	_, err := sub.EmitSchema("node", map[string]any{"k": 1})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "sub-X", events[0].TaskId, "sub-task id stamp must still be applied")
	assert.Equal(t, "parent-meta", events[0].AIService, "forwarded events must still pass through the parent emitter's processors")
}

// TestBuildSubReactForwardingEmitter_DoesNotStampParentOwnEvents verifies PushEventProcesser
// returns a copy and does not mutate the parent: the parent's own emits keep their original
// TaskId, so the sub-agent marker never leaks onto parent traffic.
func TestBuildSubReactForwardingEmitter_DoesNotStampParentOwnEvents(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	sub := BuildSubReactForwardingEmitter(parentEmitter, "sub-Z")
	require.NotNil(t, sub)

	_, err := parentEmitter.EmitSchema("parent-node", map[string]any{"k": 1})
	require.NoError(t, err)
	_, err = sub.EmitSchema("sub-node", map[string]any{"k": 2})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 2)
	assert.Equal(t, "", events[0].TaskId, "parent's own emits must keep their original TaskId, not the sub-agent marker")
	assert.Equal(t, "sub-Z", events[1].TaskId, "sub-agent emits must be tagged with the sub-task id")
}

// TestBuildSubReactForwardingEmitter_NilParentIsSafe verifies a nil parent emitter (e.g.
// some test configs) degrades gracefully instead of panicking.
func TestBuildSubReactForwardingEmitter_NilParentIsSafe(t *testing.T) {
	sub := BuildSubReactForwardingEmitter(nil, "sub-nil")
	require.NotNil(t, sub)
	require.NotPanics(t, func() {
		_, err := sub.EmitStatus("status", "x")
		require.NoError(t, err)
		_, err = sub.Emit(&schema.AiOutputEvent{NodeId: "raw"})
		require.NoError(t, err)
	})
}

// TestBuildForkedSubReactInvoker_ChildEmitterForwardsAndStampsTaskId verifies the dispatch
// wiring (buildForkedSubReactInvoker) actually hands the child invoker an emitter that
// forwards to the parent and stamps the sub-task id — i.e. dispatch correctly "下发 emitter".
// AIRuntimeInvokerGetter is swapped so we can build a real child config from the options
// without a live AI runtime.
func TestBuildForkedSubReactInvoker_ChildEmitterForwardsAndStampsTaskId(t *testing.T) {
	capture := &subReactEmitterCapture{}
	captureEmitter := aicommon.NewEmitter("parent", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.mu.Lock()
		capture.events = append(capture.events, e)
		capture.mu.Unlock()
		return e, nil
	})
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEmitter(captureEmitter),
	)
	require.True(t, parentCfg.GetEmitter() == captureEmitter, "test setup: parent config must carry the capturing emitter")

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return &configBackedDispatchInvoker{
			dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(ctx),
			cfg:                         cfg,
		}, nil
	}

	const subTaskId = "sub-agent-42"
	taskEmitter := subagent.BuildForwardingEmitter(captureEmitter, subTaskId)
	childInvoker, err := subagent.BuildForkReactInvoker(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childEmitter := childInvoker.GetConfig().GetEmitter()
	require.NotNil(t, childEmitter)
	require.NotSame(t, captureEmitter, childEmitter, "child must receive its own derived emitter, not the parent's")

	_, err = childEmitter.EmitStatus("dispatch-status", "running")
	require.NoError(t, err)
	_, err = childEmitter.EmitSchema("sub-node", map[string]any{"k": "v"})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 2, "child invoker emitter must forward events to the parent emitter (dispatch下发emitter)")
	for _, e := range events {
		assert.Equal(t, subTaskId, e.TaskId, "dispatched child events must be tagged with the sub-agent task id")
	}
}

// TestRunForkedSubReactAgentJob_SubTaskEmitterForwardsAndStampsTaskId replicates the exact
// emitter wiring runForkedSubReactAgentJob applies to the sub-task (NewSubTaskBase +
// SetEmitter(BuildSubReactForwardingEmitter(...))) and verifies the sub-task's emitter —
// which is the effective emitter the sub loop runs through — forwards to the parent and
// stamps the sub-task id. This avoids executing the sub loop's AI while still covering the
// sub-task emitter wiring path.
func TestRunForkedSubReactAgentJob_SubTaskEmitterForwardsAndStampsTaskId(t *testing.T) {
	capture := &subReactEmitterCapture{}
	captureEmitter := aicommon.NewEmitter("parent", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		capture.mu.Lock()
		capture.events = append(capture.events, e)
		capture.mu.Unlock()
		return e, nil
	})
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEmitter(captureEmitter),
	)

	parentTask := aicommon.NewStatefulTaskBase(
		"parent-task", "parent input", context.Background(), parentCfg.GetEmitter(), true,
	)
	require.NotNil(t, parentTask.GetEmitter())

	const subTaskId = "sub-agent-77"
	// Mirror runForkedSubReactAgentJob lines: create sub-task, then override its emitter
	// with the forwarding emitter derived from the parent config emitter.
	subTask := aicommon.NewSubTaskBase(parentTask, subTaskId, "sub input", true)
	subTask.SetEmitter(BuildSubReactForwardingEmitter(parentCfg.GetEmitter(), subTaskId))
	require.NotEqual(t, captureEmitter, subTask.GetEmitter(), "sub-task must use the derived forwarding emitter, not the inherited parent emitter")

	subEmitter := subTask.GetEmitter()
	require.NotNil(t, subEmitter)
	_, err := subEmitter.EmitStatus("sub-status", "running")
	require.NoError(t, err)
	_, err = subEmitter.EmitSchema("sub-node", map[string]any{"k": "v"})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 2, "sub-task emitter must forward events to the parent emitter")
	for _, e := range events {
		assert.Equal(t, subTaskId, e.TaskId, "sub-task events must be tagged with the sub-task id aggregation marker")
	}
}

// newSubReactAICallbackProbe returns a distinct AI callback closure that increments its own
// hit counter each time it is invoked. Using closures (rather than comparing function pointers,
// which Go forbids for non-nil funcs) lets the test verify behaviorally that a given child slot
// actually runs the callback the parent put there — invoke the slot and check which probe fired.
func newSubReactAICallbackProbe() (aicommon.AICallbackType, *int64) {
	var hits int64
	cb := func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		atomic.AddInt64(&hits, 1)
		return &aicommon.AIResponse{}, nil
	}
	return cb, &hits
}

func assertProbeHits(t *testing.T, label string, cb aicommon.AICallbackType, hits *int64) {
	t.Helper()
	require.NotNil(t, cb, "%s callback must be present on the child invoker", label)
	before := atomic.LoadInt64(hits)
	_, err := cb(nil, nil)
	require.NoError(t, err, "%s callback must run without error", label)
	assert.Equal(t, before+1, atomic.LoadInt64(hits), "%s callback must be the one wired into the child slot", label)
}

// TestBuildForkedSubReactInvoker_PassesAICallbacksToChild verifies the core dispatch wiring at
// buildForkedSubReactInvoker: the child invoker inherits the parent's AI callbacks
// (Original / QualityPriorityRaw / SpeedPriorityRaw) via WithAICallbacks(parentCfg.GetRawAICallbacks()),
// so every forked sub agent actually calls the same AI the parent is configured with. Each probe
// lands in its own slot — this catches both "callbacks lost entirely" and "callbacks shuffled
// into the wrong tier" regressions. AIRuntimeInvokerGetter is swapped so we can build a real
// child config from the options without a live AI runtime.
func TestBuildForkedSubReactInvoker_PassesAICallbacksToChild(t *testing.T) {
	origCb, origHits := newSubReactAICallbackProbe()
	qualityCb, qualityHits := newSubReactAICallbackProbe()
	speedCb, speedHits := newSubReactAICallbackProbe()

	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithAICallbacks(&aicommon.AICallbacks{
			Original:           origCb,
			QualityPriorityRaw: qualityCb,
			SpeedPriorityRaw:   speedCb,
		}),
		aicommon.WithDisableAutoSkills(true),
	)

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return &configBackedDispatchInvoker{
			dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(ctx),
			cfg:                         cfg,
		}, nil
	}

	const subTaskId = "sub-agent-cb"
	taskEmitter := subagent.BuildForwardingEmitter(parentCfg.GetEmitter(), subTaskId)
	childInvoker, err := subagent.BuildForkReactInvoker(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childCfg, ok := childInvoker.GetConfig().(*aicommon.Config)
	require.True(t, ok, "child invoker config must be *aicommon.Config to expose AI callbacks")
	require.NotNil(t, childCfg)

	childRaw := childCfg.GetRawAICallbacks()
	require.NotNil(t, childRaw)

	// No probe should have fired yet — the callbacks are only wired, not invoked, during build.
	assert.Equal(t, int64(0), atomic.LoadInt64(origHits), "test setup: Original probe must start cold")
	assert.Equal(t, int64(0), atomic.LoadInt64(qualityHits), "test setup: QualityPriorityRaw probe must start cold")
	assert.Equal(t, int64(0), atomic.LoadInt64(speedHits), "test setup: SpeedPriorityRaw probe must start cold")

	// Invoking each child slot must fire exactly its own probe — proving the parent's callback
	// for that tier is the one wired into the child, not a fabricated default or a shuffled slot.
	assertProbeHits(t, "Original", childRaw.Original, origHits)
	assertProbeHits(t, "QualityPriorityRaw", childRaw.QualityPriorityRaw, qualityHits)
	assertProbeHits(t, "SpeedPriorityRaw", childRaw.SpeedPriorityRaw, speedHits)

	assert.Equal(t, int64(1), atomic.LoadInt64(origHits), "only the Original slot may have fired the Original probe")
	assert.Equal(t, int64(1), atomic.LoadInt64(qualityHits), "only the QualityPriorityRaw slot may have fired the QualityPriorityRaw probe")
	assert.Equal(t, int64(1), atomic.LoadInt64(speedHits), "only the SpeedPriorityRaw slot may have fired the SpeedPriorityRaw probe")
}

// TestBuildForkedSubReactInvoker_ChildHasNoAICallbacksWhenParentHasNone is the negative control:
// a parent with no AI callbacks set must produce a child that also has no AI callbacks, rather
// than silently fabricating or inheriting unrelated defaults through the dispatch wiring.
func TestBuildForkedSubReactInvoker_ChildHasNoAICallbacksWhenParentHasNone(t *testing.T) {
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
	)
	require.Nil(t, parentCfg.GetRawAICallbacks().Original)
	require.Nil(t, parentCfg.GetRawAICallbacks().QualityPriorityRaw)
	require.Nil(t, parentCfg.GetRawAICallbacks().SpeedPriorityRaw)

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return &configBackedDispatchInvoker{
			dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(ctx),
			cfg:                         cfg,
		}, nil
	}

	taskEmitter := subagent.BuildForwardingEmitter(parentCfg.GetEmitter(), "sub-agent-noop")
	childInvoker, err := subagent.BuildForkReactInvoker(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childCfg, ok := childInvoker.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	childRaw := childCfg.GetRawAICallbacks()
	assert.Nil(t, childRaw.Original, "child must not fabricate an Original callback when the parent has none")
	assert.Nil(t, childRaw.QualityPriorityRaw, "child must not fabricate a QualityPriorityRaw callback when the parent has none")
	assert.Nil(t, childRaw.SpeedPriorityRaw, "child must not fabricate a SpeedPriorityRaw callback when the parent has none")
}

// TestBuildForkedSubReactInvoker_StripsTopLevelStrategies verifies that a
// forked sub agent never inherits top-level execution strategies: plan,
// goal mode and the multi-agent dispatch preference must all be disabled on
// the child config, even when the parent has them enabled.
func TestBuildForkedSubReactInvoker_StripsTopLevelStrategies(t *testing.T) {
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEnableMultiAgentMode(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
		aicommon.WithEnablePlanAndExec(true),
	)
	require.True(t, parentCfg.GetEnableGoalMode())
	require.True(t, parentCfg.GetPreferDispatchSubReactAgents())
	require.True(t, parentCfg.GetEnablePlanAndExec())

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub-strategy", "sub", parentCfg, parentCfg)
	require.NoError(t, err)

	var capturedCfg *aicommon.Config
	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		capturedCfg = cfg
		return &configBackedDispatchInvoker{
			dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(ctx),
			cfg:                         cfg,
		}, nil
	}

	_, err = buildForkedSubReactInvoker(parentCfg, fork, context.Background(), "sub-strategy-1")
	require.NoError(t, err)
	require.NotNil(t, capturedCfg)

	assert.False(t, capturedCfg.GetEnableGoalMode(),
		"sub agent must not inherit goal mode (no minimum-iteration finish gate)")
	assert.False(t, capturedCfg.GetPreferDispatchSubReactAgents(),
		"sub agent must not inherit the multi-agent dispatch preference")
	assert.False(t, capturedCfg.GetEnablePlanAndExec(),
		"sub agent must not open plans")
	assert.False(t, capturedCfg.EnableDispatchSubReactAgents,
		"sub agent must not be able to dispatch further sub agents")
}
