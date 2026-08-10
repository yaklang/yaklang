package scannode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// noopEmitter is a no-op aiSessionRuntimeEmitter for stateless driver tests
// (buildYakAIEngineOptions wires emitter into OnEvent; nil would panic).
type noopEmitter struct{}

func (noopEmitter) Emit(string, []byte)           {}
func (noopEmitter) Done([]byte)                   {}
func (noopEmitter) Failed(string, string, []byte) {}

type recordingStatelessEmitter struct {
	failed chan string
}

func (recordingStatelessEmitter) Emit(string, []byte) {}
func (recordingStatelessEmitter) Done([]byte)         {}
func (e recordingStatelessEmitter) Failed(code string, _ string, _ []byte) {
	e.failed <- code
}

type singleRunCompletion struct {
	turnID string
	result []byte
}

type recordingSingleRunEmitter struct {
	done   chan singleRunCompletion
	failed chan singleRunCompletion
}

type blockingTurnFailureEmitter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingTurnFailureEmitter() *blockingTurnFailureEmitter {
	return &blockingTurnFailureEmitter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (*blockingTurnFailureEmitter) Emit(string, []byte)           {}
func (*blockingTurnFailureEmitter) Done([]byte)                   {}
func (*blockingTurnFailureEmitter) Failed(string, string, []byte) {}
func (*blockingTurnFailureEmitter) DoneTurn(string, []byte)       {}
func (e *blockingTurnFailureEmitter) FailTurn(string, string, string, []byte) {
	e.once.Do(func() { close(e.started) })
	<-e.release
}

func (recordingSingleRunEmitter) Emit(string, []byte) {}
func (recordingSingleRunEmitter) Done([]byte)         {}
func (recordingSingleRunEmitter) Failed(string, string, []byte) {
}
func (e recordingSingleRunEmitter) DoneTurn(turnID string, result []byte) {
	e.done <- singleRunCompletion{turnID: turnID, result: append([]byte(nil), result...)}
}
func (e recordingSingleRunEmitter) FailTurn(turnID, code, message string, detail []byte) {
	if e.failed == nil {
		return
	}
	e.failed <- singleRunCompletion{
		turnID: turnID,
		result: append([]byte(code+":"+message+":"), detail...),
	}
}

type fakeStatelessTurnEngine struct {
	ctx          context.Context
	config       *aiengine.AIEngineConfig
	started      chan struct{}
	release      chan struct{}
	closed       chan struct{}
	events       chan *ypb.AIInputEvent
	eventStarted chan struct{}
	eventRelease chan struct{}
	sendErr      error
	eventErr     error
	startOnce    sync.Once
	closeOnce    sync.Once
}

func newFakeStatelessTurnEngine() *fakeStatelessTurnEngine {
	return &fakeStatelessTurnEngine{
		ctx:     context.Background(),
		config:  aiengine.NewAIEngineConfig(),
		started: make(chan struct{}),
		release: make(chan struct{}),
		closed:  make(chan struct{}),
		events:  make(chan *ypb.AIInputEvent, 4),
	}
}

func (f *fakeStatelessTurnEngine) Config() *aiengine.AIEngineConfig { return f.config }

func (f *fakeStatelessTurnEngine) Context() context.Context { return f.ctx }

func (f *fakeStatelessTurnEngine) SendMsg(string, ...aiengine.AIEngineConfigOption) error {
	f.startOnce.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-f.closed:
	}
	return f.sendErr
}

func (f *fakeStatelessTurnEngine) SendInputEvent(event *ypb.AIInputEvent) error {
	if f.eventStarted != nil {
		f.startOnce.Do(func() { close(f.eventStarted) })
	}
	if f.eventRelease != nil {
		<-f.eventRelease
	}
	if f.eventErr != nil {
		return f.eventErr
	}
	f.events <- event
	return nil
}

func (f *fakeStatelessTurnEngine) Close() {
	f.closeOnce.Do(func() { close(f.closed) })
}

func TestStatelessDriverBindReturnsHandleWithoutEngineField(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-1", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, err := driver.Bind(context.Background(), binding, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if handle == nil {
		t.Fatal("bind returned nil handle")
	}
	// The handle must NOT hold a live *aiengine.AIEngine yet — engine is per-turn.
	// Assert via type assertion that it's a *statelessAIEngineRuntimeHandle
	// with no engine construction (newEngine not called yet).
	sh, ok := handle.(*statelessAIEngineRuntimeHandle)
	if !ok {
		t.Fatalf("handle is %T, want *statelessAIEngineRuntimeHandle", handle)
	}
	if sh.newEngine == nil {
		t.Fatal("handle.newEngine must be set (defaults to aiengine.NewAIEngine)")
	}
}

func TestStatelessDriverRejectsNewInputWhileTerminalFailurePublicationIsBlocked(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	engine.sendErr = errors.New("provider failed")
	emitter := newBlockingTurnFailureEmitter()
	t.Cleanup(func() { close(emitter.release) })
	handle := &statelessAIEngineRuntimeHandle{
		binding: aiSessionBinding{
			ExecutionMode: "conversation",
		},
		emitter: emitter,
		newEngine: func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
			return engine, nil
		},
	}

	if err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-failed"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"first"}`),
	}); err != nil {
		t.Fatalf("send failing input: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("failing turn did not start")
	}
	close(engine.release)
	select {
	case <-emitter.started:
	case <-time.After(time.Second):
		t.Fatal("terminal failure publication did not start")
	}

	err := handle.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-too-late"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"second"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "runtime is closed") {
		t.Fatalf("new input during terminal publication error = %v, want closed runtime", err)
	}
}

func TestStatelessDriverRejectsContextAfterTerminalFailureClosesHandle(t *testing.T) {
	handle := &statelessAIEngineRuntimeHandle{closed: true}
	err := handle.AppendContext(context.Background(), aiSessionContextUpdate{})
	if err == nil || !strings.Contains(err.Error(), "runtime is closed") {
		t.Fatalf("append context error = %v, want closed runtime", err)
	}
}

func TestDefaultStatelessDriverExecutesConfiguredForgeDirectly(t *testing.T) {
	t.Setenv("LEGION_AI_RUNTIME", "")
	original := executeYakAIForge
	t.Cleanup(func() { executeYakAIForge = original })
	invoked := make(chan []*ypb.ExecParamItem, 1)
	executeYakAIForge = func(_ string, input any, _ ...any) (any, error) {
		params, _ := input.([]*ypb.ExecParamItem)
		invoked <- params
		return map[string]string{"status": "done"}, nil
	}

	driver := selectAISessionRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-direct-forge", OwnerUserID: "u1"},
		RuntimeOptionSnapshotJSON: []byte(`{
			"forge_name":"yak-cve-analysis",
			"forge_params":[{"key":"target","value":"https://example.test"}]
		}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh, ok := handle.(*statelessAIEngineRuntimeHandle)
	if !ok {
		t.Fatalf("default driver handle is %T, want stateless", handle)
	}
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-forge-1"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"do not replace explicit forge params"}`),
	}); err != nil {
		t.Fatalf("send input: %v", err)
	}
	select {
	case params := <-invoked:
		if len(params) != 1 || params[0].GetKey() != "target" || params[0].GetValue() != "https://example.test" {
			t.Fatalf("unexpected direct forge params: %#v", params)
		}
	case <-time.After(time.Second):
		t.Fatal("default stateless driver did not execute configured forge")
	}
}

func TestDefaultStatelessDriverAppliesIdleHotpatchToNextTurn(t *testing.T) {
	t.Setenv("LEGION_AI_RUNTIME", "")
	driver := selectAISessionRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-idle-hotpatch", OwnerUserID: "u1"},
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	engineCreated := false
	sh.newEngine = func(options ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		engineCreated = true
		config := aiengine.NewAIEngineConfig(options...)
		runtimeConfig := aicommon.NewConfig(context.Background(), config.ExtOptions...)
		if !runtimeConfig.GetEnablePlanAndExec() {
			t.Fatal("idle hotpatch was not applied to the next turn config")
		}
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
	}); err != nil {
		t.Fatalf("idle hotpatch: %v", err)
	}
	if engineCreated {
		t.Fatal("idle hotpatch must not create a turn engine")
	}
	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start after hotpatch"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("next turn did not start")
	}
	close(engine.release)
}

func TestDefaultStatelessDriverRoutesActiveHotpatchAndRejectsUnknownType(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-active-hotpatch", OwnerUserID: "u1"},
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}
	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	<-engine.started

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
	}); err != nil {
		t.Fatalf("active hotpatch: %v", err)
	}
	select {
	case event := <-engine.events:
		if !event.GetIsConfigHotpatch() || event.GetHotpatchType() != aicommon.HotPatchType_EnablePlan {
			t.Fatalf("unexpected active hotpatch event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("active engine did not receive hotpatch")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"UnknownPatch","params":{}}`),
	})
	if err == nil || !contains(err.Error(), "unsupported ai session hotpatch_type") {
		t.Fatalf("expected unsupported hotpatch error, got %v", err)
	}
	close(engine.release)
}

func TestDefaultStatelessDriverDoesNotCommitRejectedActiveHotpatch(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-rejected-hotpatch", OwnerUserID: "u1"},
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	engine.eventErr = errors.New("engine is closing")
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}
	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-rejected-hotpatch"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	<-engine.started

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`),
	})
	if err == nil || !contains(err.Error(), "engine is closing") {
		t.Fatalf("expected active hotpatch delivery failure, got %v", err)
	}
	sh.mu.Lock()
	committed := sh.runtime.EnablePlan
	sh.mu.Unlock()
	if committed != nil {
		t.Fatalf("failed hotpatch leaked into next-turn state: %v", *committed)
	}
	close(engine.release)
}

func TestStatelessDriverSendInputInvokesEngineFactoryPerTurn(t *testing.T) {
	var engineCreated int32
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-2", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, err := driver.Bind(context.Background(), binding, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	// Inject a factory that counts calls. It returns an error to short-circuit
	// the real SendMsg (we only want to verify the factory was invoked, not
	// run a real ReAct loop). The error is expected; we assert the count, not
	// the error path.
	sh.newEngine = func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		atomic.AddInt32(&engineCreated, 1)
		// Return nil + error so SendInput exits early after counting.
		return nil, errFakeEngineFactory
	}

	_ = sh.SendInput(context.Background(), aiSessionInput{
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-2",
			UserInput: "hello",
			Messages: []*aiv1.ContextMessage{
				{Role: "user", Content: "q1"},
			},
		},
	})

	if got := atomic.LoadInt32(&engineCreated); got != 1 {
		t.Fatalf("expected newEngine called once per SendInput, got %d", got)
	}
}

func TestStatelessDriverSendInputEmptyUserInputErrors(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-3", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, _ := driver.Bind(context.Background(), binding, noopEmitter{})
	sh := handle.(*statelessAIEngineRuntimeHandle)
	sh.newEngine = func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		t.Fatal("newEngine should not be called when user input is empty")
		return nil, nil
	}

	// ContextPackage with empty UserInput AND empty PayloadJSON → error.
	err := sh.SendInput(context.Background(), aiSessionInput{
		ContextPackage: &aiv1.ContextPackage{SessionId: "s-stateless-3"},
	})
	if err == nil {
		t.Fatal("expected error for empty user input")
	}
}

func TestStatelessDriverRequiresPinnedFocusReleaseInContextPackage(t *testing.T) {
	handle := &statelessAIEngineRuntimeHandle{pinnedFocusReleaseID: "http_fuzztest@1.0.0+abcdef123456"}
	err := handle.SendInput(context.Background(), aiSessionInput{
		ContextPackage: &aiv1.ContextPackage{UserInput: "run"},
	})
	if err == nil || !contains(err.Error(), "is missing from context package") {
		t.Fatalf("expected missing focus release error, got %v", err)
	}
}

func TestStatelessDriverRejectsMismatchedPinnedFocusRelease(t *testing.T) {
	handle := &statelessAIEngineRuntimeHandle{pinnedFocusReleaseID: "http_fuzztest@1.0.0+abcdef123456"}
	err := handle.SendInput(context.Background(), aiSessionInput{
		ContextPackage: &aiv1.ContextPackage{
			UserInput: "run",
			FocusRelease: &aiv1.ContextFocusRelease{
				ReleaseId: "infosec_recon@1.0.0+abcdef123456",
			},
		},
	})
	if err == nil || !contains(err.Error(), "focus release mismatch") {
		t.Fatalf("expected focus release mismatch, got %v", err)
	}
}

func TestStatelessDriverAppliesPinnedFocusReleaseToEngineConstruction(t *testing.T) {
	release := testContextFocusRelease(`__VERBOSE_NAME__ = "Pinned Runtime Focus"`)
	handle := &statelessAIEngineRuntimeHandle{
		pinnedFocusReleaseID: release.ReleaseId,
		newEngine: func(options ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
			config := aiengine.NewAIEngineConfig(options...)
			if config.Focus != release.RuntimeName {
				t.Fatalf("engine focus = %q, want pinned runtime %q", config.Focus, release.RuntimeName)
			}
			return nil, errFakeEngineFactory
		},
	}

	err := handle.SendInput(context.Background(), aiSessionInput{
		ContextPackage: &aiv1.ContextPackage{
			UserInput:    "run the pinned focus",
			FocusRelease: release,
		},
	})
	if err == nil || !contains(err.Error(), errFakeEngineFactory.Error()) {
		t.Fatalf("expected injected engine factory error after focus assertion, got %v", err)
	}
}

func TestStatelessDriverInteractiveResponseRequiresActiveTurnWithoutCreatingEngine(t *testing.T) {
	var engineCreated int32
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-review", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, err := driver.Bind(context.Background(), binding, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	sh.newEngine = func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		atomic.AddInt32(&engineCreated, 1)
		return nil, errFakeEngineFactory
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"id":"review-1","suggestion":"continue"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-review",
			UserInput: `{"id":"review-1","suggestion":"continue"}`,
		},
	})
	if err == nil || !contains(err.Error(), "no active turn") {
		t.Fatalf("expected no-active-turn error, got %v", err)
	}
	if got := atomic.LoadInt32(&engineCreated); got != 0 {
		t.Fatalf("interactive response must not create a new engine, got %d creations", got)
	}
}

func TestStatelessDriverFreeInputWithoutActiveTurnStartsNextTurn(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-idle-free-input", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"content":"continue with redirect checks"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-idle-free-input",
			UserInput: "continue with redirect checks",
		},
	})
	if err != nil {
		t.Fatalf("free input: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("free input did not start a new turn")
	}
	close(engine.release)
}

func TestStatelessDriverRunsTurnAsyncAndRoutesInteractiveResponseToActiveEngine(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-active-review", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	returned := make(chan error, 1)
	go func() {
		returned <- sh.SendInput(context.Background(), aiSessionInput{
			Ref:         aiSessionCommandRef{CommandID: "turn-command-1"},
			InputType:   "message",
			PayloadJSON: []byte(`{"content":"run a reviewed tool"}`),
			ContextPackage: &aiv1.ContextPackage{
				SessionId: "s-stateless-active-review",
				UserInput: "run a reviewed tool",
			},
		})
	}()

	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("SendInput returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendInput blocked the command consumer while the turn was running")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"id":"review-async-1","suggestion":"continue"}`),
		ReviewID:    "review-async-1",
		TurnID:      "turn-command-1",
	})
	if err != nil {
		t.Fatalf("interactive response: %v", err)
	}

	select {
	case event := <-engine.events:
		if !event.GetIsInteractiveMessage() {
			t.Fatal("expected interactive event")
		}
		if event.GetInteractiveId() != "review-async-1" {
			t.Fatalf("unexpected interactive id: %q", event.GetInteractiveId())
		}
	case <-time.After(time.Second):
		t.Fatal("active turn did not receive interactive response")
	}
	close(engine.release)
}

func TestStatelessControlInputsAreLinearizedWithTurnClose(t *testing.T) {
	tests := []struct {
		name  string
		input aiSessionInput
	}{
		{
			name: "review",
			input: aiSessionInput{
				InputType:   "user_intervention",
				PayloadJSON: []byte(`{"id":"review-linearized","suggestion":"continue"}`),
				ReviewID:    "review-linearized",
				TurnID:      "turn-linearized",
			},
		},
		{
			name: "sync",
			input: aiSessionInput{
				InputType:   "sync_event",
				PayloadJSON: []byte(`{"sync_type":"recovery_plan_and_exec","sync_json_input":{"coordinator_id":"coor-1"}}`),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := newFakeStatelessTurnEngine()
			engine.eventStarted = make(chan struct{})
			engine.eventRelease = make(chan struct{})
			handle := &statelessAIEngineRuntimeHandle{
				activeTurn: &statelessAITurn{
					engine: engine,
					turnID: "turn-linearized",
				},
			}

			inputDone := make(chan error, 1)
			go func() { inputDone <- handle.SendInput(context.Background(), test.input) }()
			select {
			case <-engine.eventStarted:
			case <-time.After(time.Second):
				t.Fatal("control input did not reach active engine")
			}
			closeDone := make(chan struct{})
			go func() {
				handle.closeRuntime()
				close(closeDone)
			}()
			select {
			case <-closeDone:
				t.Fatal("turn close crossed an in-flight control delivery")
			case <-time.After(20 * time.Millisecond):
			}
			close(engine.eventRelease)
			if err := <-inputDone; err != nil {
				t.Fatalf("linearized control input: %v", err)
			}
			select {
			case <-closeDone:
			case <-time.After(time.Second):
				t.Fatal("turn close did not finish after control delivery")
			}
		})
	}
}

func TestStatelessTaskScopedCapabilityHotpatchDoesNotPersist(t *testing.T) {
	engine := newFakeStatelessTurnEngine()
	handle := &statelessAIEngineRuntimeHandle{
		runtime: yakRuntimeOptions{EnabledCapabilities: []yakAICapability{{Name: "base", Type: "tool"}}},
		activeTurn: &statelessAITurn{
			engine: engine,
			turnID: "turn-a",
		},
	}
	input := aiSessionInput{
		InputType:   "hotpatch",
		PayloadJSON: []byte(`{"hotpatch_type":"EnabledCapabilities","task_id":"task-a","params":{"enabled_capabilities":[{"name":"temporary","type":"tool"}]}}`),
	}
	if err := handle.SendInput(context.Background(), input); err != nil {
		t.Fatalf("active task-scoped hotpatch: %v", err)
	}
	if got := handle.runtime.EnabledCapabilities; len(got) != 1 || got[0].Name != "base" {
		t.Fatalf("task-scoped capability leaked into next-turn snapshot: %#v", got)
	}
	select {
	case <-engine.events:
	case <-time.After(time.Second):
		t.Fatal("active task did not receive task-scoped hotpatch")
	}
	handle.activeTurn = nil
	if err := handle.SendInput(context.Background(), input); err == nil || !strings.Contains(err.Error(), "requires an active task") {
		t.Fatalf("idle task-scoped hotpatch error = %v, want explicit rejection", err)
	}
}

func TestStatelessDriverFencesReviewByTurnAndReviewID(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-fenced-review", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}
	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-live"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"id":"review-live","suggestion":"continue"}`),
		ReviewID:    "review-other",
		TurnID:      "turn-live",
	})
	if err == nil || !contains(err.Error(), "review id mismatch") {
		t.Fatalf("expected review id fence, got %v", err)
	}
	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"id":"review-live","suggestion":"continue"}`),
		ReviewID:    "review-live",
		TurnID:      "turn-stale",
	})
	if err == nil || !contains(err.Error(), "turn id mismatch") {
		t.Fatalf("expected turn id fence, got %v", err)
	}
	select {
	case event := <-engine.events:
		t.Fatalf("fenced review reached active engine: %#v", event)
	default:
	}
	close(engine.release)
}

func TestStatelessDriverRoutesFreeInputToActiveEngine(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-free-input", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-free-input",
			UserInput: "start",
		},
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "user_intervention",
		PayloadJSON: []byte(`{"content":"also inspect redirects"}`),
	})
	if err != nil {
		t.Fatalf("free input: %v", err)
	}
	select {
	case event := <-engine.events:
		if !event.GetIsFreeInput() || event.GetFreeInput() != "also inspect redirects" {
			t.Fatalf("unexpected free input event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("active turn did not receive free input")
	}
	close(engine.release)
}

func TestStatelessDriverStartsFreshEngineAfterTurnCompletes(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-next-turn", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engines := []*fakeStatelessTurnEngine{
		newFakeStatelessTurnEngine(),
		newFakeStatelessTurnEngine(),
	}
	var engineIndex int
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		engine := engines[engineIndex]
		engineIndex++
		return engine, nil
	}

	send := func(content string) error {
		return sh.SendInput(context.Background(), aiSessionInput{
			InputType:   "message",
			PayloadJSON: []byte(fmt.Sprintf(`{"content":%q}`, content)),
			ContextPackage: &aiv1.ContextPackage{
				SessionId: "s-stateless-next-turn",
				UserInput: content,
			},
		})
	}
	if err := send("first"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	select {
	case <-engines[0].started:
	case <-time.After(time.Second):
		t.Fatal("first turn did not start")
	}
	close(engines[0].release)
	select {
	case <-engines[0].closed:
	case <-time.After(time.Second):
		t.Fatal("first turn engine was not closed")
	}

	if err := send("second"); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	select {
	case <-engines[1].started:
	case <-time.After(time.Second):
		t.Fatal("second turn did not start with a fresh engine")
	}
	close(engines[1].release)
}

func TestStatelessDriverSingleRunCompletesAndRejectsSecondTurn(t *testing.T) {
	emitter := recordingSingleRunEmitter{done: make(chan singleRunCompletion, 1)}
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:           aiSessionCommandRef{SessionID: "s-stateless-single-run", OwnerUserID: "u1"},
		ExecutionMode: "single_run",
	}, emitter)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-single-1"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	<-engine.started
	close(engine.release)

	select {
	case done := <-emitter.done:
		if done.turnID != "turn-single-1" {
			t.Fatalf("unexpected terminal turn id: %q", done.turnID)
		}
	case <-time.After(time.Second):
		t.Fatal("single run did not complete automatically")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-single-2"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"again"}`),
	})
	if err == nil || !contains(err.Error(), "runtime is closed") {
		t.Fatalf("expected closed runtime after single run, got %v", err)
	}
}

func TestStatelessDriverSingleRunFailureTerminatesTheOwningTurn(t *testing.T) {
	emitter := recordingSingleRunEmitter{
		done:   make(chan singleRunCompletion, 1),
		failed: make(chan singleRunCompletion, 1),
	}
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:           aiSessionCommandRef{SessionID: "s-stateless-single-fail", OwnerUserID: "u1"},
		ExecutionMode: "single_run",
	}, emitter)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	engine.sendErr = fmt.Errorf("bounded probe failed")
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-fail-1"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	<-engine.started
	close(engine.release)

	select {
	case failed := <-emitter.failed:
		if failed.turnID != "turn-fail-1" || !contains(string(failed.result), "bounded probe failed") {
			t.Fatalf("unexpected terminal failure: %#v", failed)
		}
	case <-time.After(time.Second):
		t.Fatal("single-run failure did not terminate automatically")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-fail-2"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"again"}`),
	})
	if err == nil || !contains(err.Error(), "runtime is closed") {
		t.Fatalf("expected closed runtime after single-run failure, got %v", err)
	}
}

func TestStatelessDriverCloseWinsAgainstSingleRunCompletion(t *testing.T) {
	emitter := recordingSingleRunEmitter{done: make(chan singleRunCompletion, 1)}
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:           aiSessionCommandRef{SessionID: "s-stateless-single-close", OwnerUserID: "u1"},
		ExecutionMode: "single_run",
	}, emitter)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}
	if err := sh.SendInput(context.Background(), aiSessionInput{
		Ref:         aiSessionCommandRef{CommandID: "turn-close-1"},
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"start"}`),
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	<-engine.started
	sh.Close("platform close")

	select {
	case done := <-emitter.done:
		t.Fatalf("close must suppress automatic success: %#v", done)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStatelessDriverRejectsSecondMessageWhileTurnIsActive(t *testing.T) {
	var engineCreated int32
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-overlap", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		atomic.AddInt32(&engineCreated, 1)
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"first"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-overlap",
			UserInput: "first",
		},
	}); err != nil {
		t.Fatalf("first input: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("first turn did not start")
	}

	err = sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"second"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-overlap",
			UserInput: "second",
		},
	})
	if err == nil || !contains(err.Error(), "turn already active") {
		t.Fatalf("expected active-turn error, got %v", err)
	}
	if got := atomic.LoadInt32(&engineCreated); got != 1 {
		t.Fatalf("overlapping input created %d engines, want 1", got)
	}
	close(engine.release)
}

func TestStatelessDriverCancelClosesActiveTurn(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-cancel", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, noopEmitter{})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"long turn"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-cancel",
			UserInput: "long turn",
		},
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}

	sh.Cancel("user stopped")
	select {
	case <-engine.closed:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close active engine")
	}
}

func TestStatelessDriverCancelDoesNotReportTurnFailure(t *testing.T) {
	emitter := recordingStatelessEmitter{failed: make(chan string, 1)}
	driver := newStatelessAIEngineRuntimeDriver()
	handle, err := driver.Bind(context.Background(), aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-cancel-error", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}, emitter)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	sh := handle.(*statelessAIEngineRuntimeHandle)
	engine := newFakeStatelessTurnEngine()
	engine.sendErr = fmt.Errorf("turn interrupted by close")
	sh.newEngine = func(...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
		return engine, nil
	}

	if err := sh.SendInput(context.Background(), aiSessionInput{
		InputType:   "message",
		PayloadJSON: []byte(`{"content":"long turn"}`),
		ContextPackage: &aiv1.ContextPackage{
			SessionId: "s-stateless-cancel-error",
			UserInput: "long turn",
		},
	}); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	select {
	case <-engine.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}

	sh.Cancel("user stopped")
	select {
	case code := <-emitter.failed:
		t.Fatalf("cancelled turn emitted failure %q", code)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBuildContextPackageHistoryBlockFormatsMessages(t *testing.T) {
	block := buildContextPackageHistoryBlock(&aiv1.ContextPackage{
		Messages: []*aiv1.ContextMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	})
	if block == "" {
		t.Fatal("expected non-empty history block")
	}
	if !contains(block, "user: hello") || !contains(block, "assistant: hi there") {
		t.Fatalf("history block missing messages: %q", block)
	}
}

func TestBuildContextPackageHistoryBlockEmptyReturnsEmpty(t *testing.T) {
	if s := buildContextPackageHistoryBlock(nil); s != "" {
		t.Fatalf("nil package should yield empty, got %q", s)
	}
	if s := buildContextPackageHistoryBlock(&aiv1.ContextPackage{}); s != "" {
		t.Fatalf("no messages should yield empty, got %q", s)
	}
}

func TestStatelessDriverCancelAndCloseAreIdempotentWhenIdle(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref:                        aiSessionCommandRef{SessionID: "s-stateless-4", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, _ := driver.Bind(context.Background(), binding, noopEmitter{})
	// Repeated shutdown calls with no active turn must not panic.
	handle.Cancel("test-reason")
	handle.Cancel("test-reason")
	handle.Close("test-reason")
	sh := handle.(*statelessAIEngineRuntimeHandle)
	if !sh.closed {
		t.Fatal("Close should set closed=true")
	}
}

// errFakeEngineFactory is a sentinel error used by the fake engine factory to
// short-circuit SendMsg without a real AI provider.
var errFakeEngineFactory = fmt.Errorf("fake engine factory: short-circuit for test")

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
