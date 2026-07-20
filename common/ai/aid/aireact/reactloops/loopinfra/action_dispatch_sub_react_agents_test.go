package loopinfra

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
		"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
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

func (t *dispatchSubReactTestInvoker) subReactTimelineRecords() []reactloops.TimelineRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []reactloops.TimelineRecord
	for _, item := range t.timelineEntries {
		if item.entry != schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT {
			continue
		}
		var record reactloops.TimelineRecord
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
	loop.Set(reactloops.SubAgentDepthLoopVar, 1)

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
	opts := reactloops.DefaultSubAgentLoopOptions()
	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		newDispatchSubReactTestInvoker(context.Background()),
		opts...,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, loop.GetInt(reactloops.SubAgentDepthLoopVar))

	action := mustBuildDispatchSubReactAction(t, map[string]any{
		"dispatches": dispatchSubReactJobs(map[string]any{"goal": "nested dispatch"}),
	})
	err = verifyDispatchSubReactAgents(loop, action)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top-level agent")
}

type configBackedDispatchInvoker struct {
	*dispatchSubReactTestInvoker
	cfg *aicommon.Config
}

func (c *configBackedDispatchInvoker) GetConfig() aicommon.AICallerConfigIf {
	return c.cfg
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

// TestBuildForwardingEmitter_ForwardsEventsToParentAndStampsTaskId verifies the
// core dispatch contract: a sub-agent emitter forwards its events to the parent emitter
// (so they reach the frontend) and stamps every event's TaskId with the sub-task id, which
// is the marker the frontend uses to aggregate sub-agent messages.
func TestBuildForwardingEmitter_ForwardsEventsToParentAndStampsTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	const subTaskId = "sub-agent-xyz"
	subEmitter := reactloops.BuildForwardingEmitter(parentEmitter, subTaskId)
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

// TestBuildForwardingEmitter_DistinguishesSubAgentsByTaskId verifies that
// multiple sub agents sharing one parent emitter stay distinguishable: each event is
// tagged with the id of the sub agent that produced it.
func TestBuildForwardingEmitter_DistinguishesSubAgentsByTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	subA := reactloops.BuildForwardingEmitter(parentEmitter, "sub-A")
	subB := reactloops.BuildForwardingEmitter(parentEmitter, "sub-B")
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

// TestBuildForwardingEmitter_OverridesPreExistingTaskId verifies the sub-task id
// stamp is authoritative: even events that already carry a stale (e.g. parent) TaskId end
// up tagged with the sub-task id, so the frontend never mis-aggregates them under the parent.
func TestBuildForwardingEmitter_OverridesPreExistingTaskId(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	sub := reactloops.BuildForwardingEmitter(parentEmitter, "sub-fresh")
	require.NotNil(t, sub)

	_, err := sub.Emit(&schema.AiOutputEvent{NodeId: "raw", TaskId: "stale-parent-id"})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "sub-fresh", events[0].TaskId, "sub-task id must override any pre-existing TaskId")
}

// TestBuildForwardingEmitter_RunsParentProcessorsOnForwardedEvents verifies the
// forwarded emitter threads events through the parent emitter's processor chain (not just
// the sink), so parent-level metadata (i18n, AI info, process association, ...) still applies
// to sub-agent events.
func TestBuildForwardingEmitter_RunsParentProcessorsOnForwardedEvents(t *testing.T) {
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

	sub := reactloops.BuildForwardingEmitter(parentEmitter, "sub-X")
	_, err := sub.EmitSchema("node", map[string]any{"k": 1})
	require.NoError(t, err)

	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "sub-X", events[0].TaskId, "sub-task id stamp must still be applied")
	assert.Equal(t, "parent-meta", events[0].AIService, "forwarded events must still pass through the parent emitter's processors")
}

// TestBuildForwardingEmitter_DoesNotStampParentOwnEvents verifies PushEventProcesser
// returns a copy and does not mutate the parent: the parent's own emits keep their original
// TaskId, so the sub-agent marker never leaks onto parent traffic.
func TestBuildForwardingEmitter_DoesNotStampParentOwnEvents(t *testing.T) {
	parentEmitter, capture := newCapturingSubReactEmitter("parent")
	sub := reactloops.BuildForwardingEmitter(parentEmitter, "sub-Z")
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

// TestBuildForwardingEmitter_NilParentIsSafe verifies a nil parent emitter (e.g.
// some test configs) degrades gracefully instead of panicking.
func TestBuildForwardingEmitter_NilParentIsSafe(t *testing.T) {
	sub := reactloops.BuildForwardingEmitter(nil, "sub-nil")
	require.NotNil(t, sub)
	require.NotPanics(t, func() {
		_, err := sub.EmitStatus("status", "x")
		require.NoError(t, err)
		_, err = sub.Emit(&schema.AiOutputEvent{NodeId: "raw"})
		require.NoError(t, err)
	})
}


// TestRunForkedSubReactAgentJob_SubTaskEmitterForwardsAndStampsTaskId replicates the exact
// emitter wiring DispatchSubAgents applies to the sub-task (NewSubTaskBase +
// SetEmitter(BuildForwardingEmitter(...))) and verifies the sub-task's emitter —
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
	// Mirror DispatchSubAgents lines: create sub-task, then override its emitter
	// with the forwarding emitter derived from the parent config emitter.
	subTask := aicommon.NewSubTaskBase(parentTask, subTaskId, "sub input", true)
	subTask.SetEmitter(reactloops.BuildForwardingEmitter(parentCfg.GetEmitter(), subTaskId))
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



// ---------------------------------------------------------------------------
// Tests for rebase fix changes: early SetStatus(Processing), HotPatchOptionChan
// removal, and buildSubAgentStrategyOptions.
// ---------------------------------------------------------------------------

// elaborationObservingInvoker wraps the dispatch test invoker to observe the
// sub-task status at the moment InvokeQualityPriorityLiteForge is called (i.e.
// during goal elaboration) and returns a valid elaborated goal action.
type elaborationObservingInvoker struct {
	*dispatchSubReactTestInvoker
	cfg           *aicommon.Config
	observeStatus func(aicommon.AITaskState)
}

func (e *elaborationObservingInvoker) GetConfig() aicommon.AICallerConfigIf {
	return e.cfg
}

func (e *elaborationObservingInvoker) InvokeQualityPriorityLiteForge(
	ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption,
) (*aicommon.Action, error) {
	if e.observeStatus != nil {
		task := e.GetCurrentTask()
		if task != nil {
			e.observeStatus(task.GetStatus())
		}
	}
	// Return a valid elaborated goal action so elaborateSubReactAgentGoal succeeds.
	return aicommon.NewSimpleAction("sub_react_agent_goal_elaboration", aitool.InvokeParams{
		"goal":            "Elaborated self-contained goal for sub agent",
		"result_contract": "Return a summary of findings",
	}), nil
}

// TestRunForkedSubReactAgentJob_SetsProcessingBeforeElaboration verifies the
// rebase fix that moves subTask.SetStatus(Processing) to BEFORE the
// elaborateSubReactAgentGoal call. This ensures the sub-agent task card shows
// "processing" in the UI while the goal-elaboration AI call is in flight,
// rather than only flipping to processing after elaboration completes.
//
// We intercept the child invoker's InvokeQualityPriorityLiteForge (called during
// goal elaboration) and check that the sub-task is already in the Processing
// state at that point — not still in Created.
func TestRunForkedSubReactAgentJob_SetsProcessingBeforeElaboration(t *testing.T) {
	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithTimeline(parentTimeline),
		aicommon.WithDisableAutoSkills(true),
		// Provide a dummy AI callback so the sub-loop does not panic when the
		// mock invoker cannot reach a real AI backend.
		aicommon.WithAICallbacks(&aicommon.AICallbacks{
			Original: func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				return &aicommon.AIResponse{}, nil
			},
		}),
	)

	parentInvoker := &configBackedDispatchInvoker{
		dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(context.Background()),
		cfg:                         parentCfg,
	}

	elaborationStatusCh := make(chan aicommon.AITaskState, 1)
	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return &elaborationObservingInvoker{
			dispatchSubReactTestInvoker: newDispatchSubReactTestInvoker(ctx),
			cfg:                         cfg,
			observeStatus: func(s aicommon.AITaskState) {
				select {
				case elaborationStatusCh <- s:
				default:
				}
			},
		}, nil
	}

	parentTask := aicommon.NewStatefulTaskBase(
		"parent-task", "parent input", context.Background(), parentCfg.GetEmitter(), true,
	)

	job := reactloops.SubAgentJob{
		Order:      1,
		Identifier: "agent_a",
		Goal:       "task A",
		LoopName:   schema.AI_REACT_LOOP_NAME_DEFAULT,
	}

	results := reactloops.DispatchSubAgents(parentInvoker, parentTask, []reactloops.SubAgentJob{job}, reactloops.SubAgentOptions{
		ParentLoop:     nil,
		TimelineMode:   reactloops.SubAgentTimelineFork,
		ElaborateGoals: true,
	})
	require.Len(t, results, 1)
	result := results[0]
	require.NotNil(t, result)

	select {
	case s := <-elaborationStatusCh:
		assert.Equal(t, aicommon.AITaskState_Processing, s,
			"sub-task must be in Processing state when goal elaboration runs, not Created")
	default:
		t.Fatal("goal elaboration was not invoked, so status-at-elaboration could not be observed")
	}
}


