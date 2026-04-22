package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type perceptionMidtermSchedulerTestInvoker struct {
	*mockcfg.MockInvoker
	scheduledSummary  string
	scheduledTopics   []string
	scheduledKeywords []string
}

func (i *perceptionMidtermSchedulerTestInvoker) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	_ = ctx
	_ = actionName
	_ = prompt
	_ = outputs
	_ = opts
	return aicommon.ExtractAction(`{
		"@action": "perception",
		"summary": "focused summary from perception",
		"topics": ["http fuzzing"],
		"keywords": ["header", "malformed"],
		"changed": true,
		"confidence": 0.92
	}`, "perception")
}

func (i *perceptionMidtermSchedulerTestInvoker) ScheduleMidtermTimelineRecallFromPerception(summary string, topics []string, keywords []string) {
	i.scheduledSummary = summary
	i.scheduledTopics = append([]string{}, topics...)
	i.scheduledKeywords = append([]string{}, keywords...)
}

func TestTriggerPerception_SchedulesMidtermRecallSummary(t *testing.T) {
	invoker := &perceptionMidtermSchedulerTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}

	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.loopName = "perception-midterm-test"
	loop.perception = newPerceptionController(loop.periodicCheckpointInterval)
	loop.maxIterations = 100
	loop.actionHistory = make([]*ActionRecord, 0)
	loop.actionHistoryMutex = new(sync.Mutex)

	state := loop.TriggerPerception(PerceptionTriggerForced, true)
	require.NotNil(t, state)
	require.Equal(t, "focused summary from perception", state.OneLinerSummary)
	require.Equal(t, "focused summary from perception", invoker.scheduledSummary)
	require.Equal(t, []string{"http fuzzing"}, invoker.scheduledTopics)
	require.Equal(t, []string{"header", "malformed"}, invoker.scheduledKeywords)
}

func TestHashTopics_DeterministicAndOrderIndependent(t *testing.T) {
	h1 := hashTopics([]string{"SQL Injection", "WAF Bypass"})
	h2 := hashTopics([]string{"WAF Bypass", "SQL Injection"})
	if h1 != h2 {
		t.Fatalf("hashTopics should be order-independent, got %s vs %s", h1, h2)
	}
	h3 := hashTopics([]string{"SQL Injection", "XSS"})
	if h1 == h3 {
		t.Fatalf("different topic sets should produce different hashes")
	}
}

func TestPerceptionState_ShouldUpdate_ForcedAlwaysTrue(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     false,
		LastTrigger: PerceptionTriggerForced,
	}
	if !prev.ShouldUpdate(newState) {
		t.Fatal("forced trigger should always update")
	}

	newState.LastTrigger = PerceptionTriggerSpinDetected
	if !prev.ShouldUpdate(newState) {
		t.Fatal("spin_detected trigger should always update")
	}

	newState.LastTrigger = PerceptionTriggerLoopSwitch
	if !prev.ShouldUpdate(newState) {
		t.Fatal("loop_switch trigger should always update")
	}
}

func TestPerceptionState_ShouldUpdate_UnchangedSkips(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if prev.ShouldUpdate(newState) {
		t.Fatal("unchanged non-forced perception should not update")
	}
}

func TestPerceptionState_ShouldUpdate_ChangedWithNewTopics(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection", "WAF Bypass"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if !prev.ShouldUpdate(newState) {
		t.Fatal("changed perception with new topics should update")
	}
}

func TestPerceptionState_ShouldUpdate_ChangedButSameTopicsHash(t *testing.T) {
	prev := &PerceptionState{
		Topics:         []string{"SQL Injection"},
		PrevTopicsHash: hashTopics([]string{"SQL Injection"}),
	}

	newState := &PerceptionState{
		Topics:      []string{"SQL Injection"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	if prev.ShouldUpdate(newState) {
		t.Fatal("changed=true but same topic hash should not update")
	}
}

func TestPerceptionState_ShouldUpdate_NilNewState(t *testing.T) {
	prev := &PerceptionState{Topics: []string{"test"}}
	if prev.ShouldUpdate(nil) {
		t.Fatal("nil new state should not trigger update")
	}
}

func TestPerceptionController_IntervalThrottling(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)

	state1 := &PerceptionState{
		Topics:      []string{"Topic A"},
		Changed:     true,
		LastTrigger: PerceptionTriggerPostAction,
	}
	pc.applyResult(state1)

	if pc.shouldSkipDueToInterval() {
		// just applied, interval has not elapsed; it should skip
	}
	if !pc.shouldSkipDueToInterval() {
		t.Fatal("immediately after apply, should skip due to interval")
	}
}

func TestPerceptionController_ExponentialBackoff(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.currentInterval = 10 * time.Millisecond
	pc.minInterval = 10 * time.Millisecond
	pc.maxInterval = 100 * time.Millisecond

	unchanged := &PerceptionState{
		Topics:      []string{"Same"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}

	// First call initializes current (consecutiveUnchanged stays 0).
	// Second and third calls are "unchanged" increments.
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	if pc.consecutiveUnchanged < 2 {
		t.Fatalf("expected at least 2 consecutive unchanged, got %d", pc.consecutiveUnchanged)
	}
	if pc.currentInterval <= 10*time.Millisecond {
		t.Fatalf("interval should have doubled, got %v", pc.currentInterval)
	}
}

func TestPerceptionController_IntervalResetOnChange(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.currentInterval = 10 * time.Millisecond
	pc.minInterval = 10 * time.Millisecond

	unchanged := &PerceptionState{
		Topics:      []string{"Same"},
		Changed:     false,
		LastTrigger: PerceptionTriggerPostAction,
	}
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)
	pc.applyResult(unchanged)

	changed := &PerceptionState{
		Topics:      []string{"New Topic"},
		Changed:     true,
		LastTrigger: PerceptionTriggerForced,
	}
	pc.applyResult(changed)

	if pc.consecutiveUnchanged != 0 {
		t.Fatalf("change should reset consecutiveUnchanged, got %d", pc.consecutiveUnchanged)
	}
	if pc.currentInterval != pc.minInterval {
		t.Fatalf("change should reset interval to min, got %v", pc.currentInterval)
	}
}

func TestPerceptionController_ShouldTriggerOnIteration(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	pc.iterationTriggerInterval = 2

	if pc.shouldTriggerOnIteration(0) {
		t.Fatal("iteration 0 should not trigger")
	}
	if pc.shouldTriggerOnIteration(1) {
		t.Fatal("iteration 1 should not trigger with interval=2")
	}
	if !pc.shouldTriggerOnIteration(2) {
		t.Fatal("iteration 2 should trigger with interval=2")
	}
	if pc.shouldTriggerOnIteration(3) {
		t.Fatal("iteration 3 should not trigger with interval=2")
	}
	if !pc.shouldTriggerOnIteration(4) {
		t.Fatal("iteration 4 should trigger with interval=2")
	}
}

func TestPerceptionController_EpochIncrements(t *testing.T) {
	pc := newPerceptionController(perceptionDefaultIterationInterval)
	s1 := &PerceptionState{Topics: []string{"A"}, Changed: true, LastTrigger: PerceptionTriggerForced}
	pc.applyResult(s1)
	if pc.getCurrent().Epoch != 1 {
		t.Fatalf("expected epoch 1, got %d", pc.getCurrent().Epoch)
	}

	s2 := &PerceptionState{Topics: []string{"B"}, Changed: true, LastTrigger: PerceptionTriggerForced}
	pc.applyResult(s2)
	if pc.getCurrent().Epoch != 2 {
		t.Fatalf("expected epoch 2, got %d", pc.getCurrent().Epoch)
	}
}

func TestPerceptionState_FormatForContext(t *testing.T) {
	state := &PerceptionState{
		Topics:          []string{"SQL Injection", "WAF Bypass"},
		Keywords:        []string{"sqlmap", "union select"},
		OneLinerSummary: "Attempting SQL injection via UNION SELECT",
		Epoch:           3,
		LastUpdateAt:    time.Now().Add(-30 * time.Second),
	}
	output := state.FormatForContext()
	if !strings.Contains(output, "Current Perception") {
		t.Fatal("expected header in context output")
	}
	if !strings.Contains(output, "SQL Injection") {
		t.Fatal("expected topics in context output")
	}
	if !strings.Contains(output, "sqlmap") {
		t.Fatal("expected keywords in context output")
	}
	if !strings.Contains(output, "Attempting SQL injection") {
		t.Fatal("expected summary in context output")
	}
}

func TestPerceptionState_FormatForContext_Nil(t *testing.T) {
	var state *PerceptionState
	if state.FormatForContext() != "" {
		t.Fatal("nil state should produce empty string")
	}
}

func TestPerceptionState_FormatForContext_TokenLimit(t *testing.T) {
	topics := make([]string, 100)
	keywords := make([]string, 100)
	for i := range topics {
		topics[i] = strings.Repeat("topic_", 20)
		keywords[i] = strings.Repeat("keyword_", 20)
	}
	state := &PerceptionState{
		Topics:          topics,
		Keywords:        keywords,
		OneLinerSummary: strings.Repeat("very long summary ", 50),
		Epoch:           1,
		LastUpdateAt:    time.Now(),
	}
	output := state.FormatForContext()
	if output == "" {
		t.Fatal("expected non-empty output even when truncated")
	}
}
