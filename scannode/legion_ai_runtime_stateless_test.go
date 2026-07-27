package scannode

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// noopEmitter is a no-op aiSessionRuntimeEmitter for stateless driver tests
// (buildYakAIEngineOptions wires emitter into OnEvent; nil would panic).
type noopEmitter struct{}

func (noopEmitter) Emit(string, []byte)        {}
func (noopEmitter) Done([]byte)               {}
func (noopEmitter) Failed(string, string, []byte) {}

// fakeEngineFactory records how many engines were created and returns a
// minimal stub *aiengine.AIEngine. We cannot easily construct a real AIEngine
// without an AI provider, so the test injects this factory to assert the
// per-turn lifecycle (creation count, Close calls) without a real LLM call.
//
// NOTE: aiengine.AIEngine is a struct, not an interface, so we cannot substitute
// it directly. Instead, the test asserts via the factory's call counter that
// SendInput invoked newEngine once per turn. The real engine construction +
// SendMsg + Close path is exercised in integration (S3d T2). For the unit test,
// we inject a factory that returns a real AIEngine constructed with a no-op
// operator setup — but since that still needs an AI callback, the simplest
// verifiable assertion is the factory call count.

func TestStatelessDriverBindReturnsHandleWithoutEngineField(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-1", OwnerUserID: "u1"},
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
		Ref: aiSessionCommandRef{SessionID: "s-stateless-2", OwnerUserID: "u1"},
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
	sh.newEngine = func(opts ...aiengine.AIEngineConfigOption) (*aiengine.AIEngine, error) {
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
		Ref: aiSessionCommandRef{SessionID: "s-stateless-3", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, _ := driver.Bind(context.Background(), binding, noopEmitter{})
	sh := handle.(*statelessAIEngineRuntimeHandle)
	sh.newEngine = func(opts ...aiengine.AIEngineConfigOption) (*aiengine.AIEngine, error) {
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

func TestStatelessDriverCancelAndCloseAreNoop(t *testing.T) {
	driver := newStatelessAIEngineRuntimeDriver()
	binding := aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "s-stateless-4", OwnerUserID: "u1"},
		ProviderPolicySnapshotJSON: []byte(`{}`),
		RuntimeOptionSnapshotJSON:  []byte(`{}`),
	}
	handle, _ := driver.Bind(context.Background(), binding, noopEmitter{})
	// These must not panic.
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