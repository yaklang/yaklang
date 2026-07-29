package scannode

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

type fakeStatelessTurnEngine struct {
	started   chan struct{}
	release   chan struct{}
	closed    chan struct{}
	events    chan *ypb.AIInputEvent
	sendErr   error
	startOnce sync.Once
	closeOnce sync.Once
}

func newFakeStatelessTurnEngine() *fakeStatelessTurnEngine {
	return &fakeStatelessTurnEngine{
		started: make(chan struct{}),
		release: make(chan struct{}),
		closed:  make(chan struct{}),
		events:  make(chan *ypb.AIInputEvent, 4),
	}
}

func (f *fakeStatelessTurnEngine) SendMsg(string, ...aiengine.AIEngineConfigOption) error {
	f.startOnce.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-f.closed:
	}
	return f.sendErr
}

func (f *fakeStatelessTurnEngine) SendInputEvent(event *ypb.AIInputEvent) error {
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
