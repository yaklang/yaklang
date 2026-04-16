package test

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// TestPerception_DefaultLoop_PerceptionTriggered verifies that when perception is
// enabled (via WithDisablePerception(false)), the perception AI callback is invoked
// during loop execution and the perception state is populated with topics/keywords/summary.
func TestPerception_DefaultLoop_PerceptionTriggered(t *testing.T) {
	var perceptionCalled int32
	var mu sync.Mutex
	callCount := 0

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithDisablePerception(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err := tryHandlePerceptionPrompt(i, prompt); rsp != nil {
				atomic.AddInt32(&perceptionCalled, 1)
				return rsp, err
			}

			if isVerifySatisfactionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done"}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "SELF_REFLECTION") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "self-reflection", "suggestions": []}`))
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			mu.Lock()
			callCount++
			n := callCount
			mu.Unlock()

			if n < 4 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "continue_action"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	loop, err := reactloops.NewReActLoop("default-perception-test", reactIns,
		reactloops.WithMaxIterations(6),
		reactloops.WithRegisterLoopAction(
			"continue_action", "Continue action for testing", nil, nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Continue()
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !loop.IsPerceptionEnabled() {
		t.Fatal("perception should be enabled when DisablePerception is false")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	execErr := loop.Execute("test-perception-default", ctx, "test perception in default loop")
	if execErr != nil && execErr != context.DeadlineExceeded {
		t.Logf("execution result: %v", execErr)
	}

	// perception goroutines are async, give them a moment to complete
	time.Sleep(500 * time.Millisecond)

	count := atomic.LoadInt32(&perceptionCalled)
	if count == 0 {
		t.Error("perception AI callback was never invoked during loop execution")
	} else {
		t.Logf("perception AI callback was invoked %d time(s)", count)
	}

	state := loop.GetPerceptionState()
	if state == nil {
		t.Error("expected perception state to be populated after loop execution")
	} else {
		t.Logf("perception summary: %s", state.OneLinerSummary)
		t.Logf("perception topics: %v", state.Topics)
		t.Logf("perception keywords: %v", state.Keywords)
		if state.OneLinerSummary == "" {
			t.Error("expected non-empty perception summary")
		}
		if len(state.Topics) == 0 {
			t.Error("expected non-empty perception topics")
		}
		if len(state.Keywords) == 0 {
			t.Error("expected non-empty perception keywords")
		}
	}
}

// TestPerception_PlanLoop_PerceptionEnabled verifies that the plan loop factory
// creates a ReActLoop with perception enabled when the config allows it.
// Since both loop_default and loop_plan go through NewReActLoop, perception
// auto-registers via config. This test uses CreateLoopByName to verify the
// plan factory path.
func TestPerception_PlanLoop_PerceptionEnabled(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithDisablePerception(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	var capturedLoop *reactloops.ReActLoop
	planLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_PLAN, reactIns,
		reactloops.WithOnLoopInstanceCreated(func(loop *reactloops.ReActLoop) {
			capturedLoop = loop
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if planLoop == nil {
		t.Fatal("plan loop should not be nil")
	}

	if !planLoop.IsPerceptionEnabled() {
		t.Error("plan loop should have perception enabled when DisablePerception is false")
	}

	// Also verify the default loop
	defaultLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT, reactIns,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !defaultLoop.IsPerceptionEnabled() {
		t.Error("default loop should have perception enabled when DisablePerception is false")
	}

	_ = capturedLoop
	t.Log("both plan and default loops have perception enabled")
}

// TestPerception_DisabledByDefault_InTestReAct verifies that NewTestReAct disables
// perception by default (via WithDisablePerception(true)), so no perception AI calls
// are made during test loop execution.
func TestPerception_DisabledByDefault_InTestReAct(t *testing.T) {
	var perceptionCalled int32

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if isPerceptionPrompt(prompt) {
				atomic.AddInt32(&perceptionCalled, 1)
			}

			if isVerifySatisfactionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done"}`))
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	loop, err := reactloops.NewReActLoop("default-no-perception", reactIns,
		reactloops.WithMaxIterations(5),
	)
	if err != nil {
		t.Fatal(err)
	}

	if loop.IsPerceptionEnabled() {
		t.Error("perception should be disabled by default in NewTestReAct")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = loop.Execute("test-no-perception", ctx, "test perception disabled by default")

	time.Sleep(200 * time.Millisecond)

	state := loop.GetPerceptionState()
	if state != nil {
		t.Error("perception state should be nil when perception is disabled")
	}

	count := atomic.LoadInt32(&perceptionCalled)
	if count > 0 {
		t.Errorf("perception AI callback should not be invoked when disabled, but was called %d time(s)", count)
	}

	t.Log("perception correctly disabled by default in NewTestReAct")
}

// TestPerception_IntentLoop_AlwaysDisabled verifies that the intent loop factory
// always creates a ReActLoop with perception disabled, even when the config does
// not disable it. Intent loops are lightweight, single-iteration sub-loops that
// should never run perception.
func TestPerception_IntentLoop_AlwaysDisabled(t *testing.T) {
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithDisablePerception(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "answer": "done"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	intentLoop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_INTENT, reactIns,
	)
	if err != nil {
		t.Fatal(err)
	}

	if intentLoop.IsPerceptionEnabled() {
		t.Error("intent loop should always have perception disabled regardless of config")
	}

	t.Log("intent loop correctly has perception disabled")
}
