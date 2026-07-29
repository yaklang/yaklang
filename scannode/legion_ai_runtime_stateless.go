package scannode

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// statelessAIEngineRuntimeDriver is a parallel runtime driver (S3c) that runs
// a fresh stateless aiengine.AIEngine per turn. Unlike yakAIEngineRuntimeDriver,
// Bind does NOT create an engine. A normal input creates one engine for that
// turn, while review/sync inputs are routed into the same live engine. The
// engine is destroyed when the turn returns, so no state crosses turns.
// History/tools/user_input come from the ContextPackage carried on aiSessionInput.
type statelessAIEngineRuntimeDriver struct{}

func newStatelessAIEngineRuntimeDriver() aiSessionRuntimeDriver {
	return statelessAIEngineRuntimeDriver{}
}

func (statelessAIEngineRuntimeDriver) Bind(
	ctx context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	// Pre-compute the option slice that every turn will replay (attachment
	// content, credential projection, AI callback). This reuses buildYakAIEngineOptions
	// WITHOUT calling NewAIEngine — buildYakAIEngineOptions returns the options,
	// and Bind normally calls NewAIEngine(options...) itself. We skip that call
	// and cache the options for per-turn replay.
	cachedOptions, err := buildYakAIEngineOptions(ctx, binding, emitter)
	if err != nil {
		return nil, fmt.Errorf("stateless bind: build options: %w", err)
	}
	return &statelessAIEngineRuntimeHandle{
		binding:       binding,
		emitter:       emitter,
		cachedOptions: cachedOptions,
		newEngine: func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
			return aiengine.NewAIEngine(opts...)
		},
	}, nil
}

type statelessTurnEngine interface {
	SendMsg(string, ...aiengine.AIEngineConfigOption) error
	SendInputEvent(*ypb.AIInputEvent) error
	Close()
}

type statelessAITurn struct {
	engine    statelessTurnEngine
	closeOnce sync.Once
}

func (t *statelessAITurn) close() {
	if t == nil || t.engine == nil {
		return
	}
	t.closeOnce.Do(t.engine.Close)
}

type statelessAIEngineRuntimeHandle struct {
	binding       aiSessionBinding
	emitter       aiSessionRuntimeEmitter
	cachedOptions []aiengine.AIEngineConfigOption

	mu         sync.Mutex
	activeTurn *statelessAITurn
	closed     bool

	// newEngine is overridable in tests so lifecycle and control routing can
	// be verified without a real model provider.
	newEngine func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error)
}

func (h *statelessAIEngineRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	if isInteractiveAISessionInput(input.InputType) {
		handled, err := h.sendInterventionInput(input)
		if err != nil || handled {
			return err
		}
		// Session status can remain running until S3e's idle timer fires even
		// though the prior turn engine has already closed. A free input sent in
		// that window starts the next turn; a review response never does.
		input.InputType = "message"
	}
	if isSyncAISessionInput(input.InputType) {
		return h.sendSyncInput(input)
	}

	// Build a fresh engine per turn with WithStateless(true) + cached options +
	// ContextPackage-derived history injection.
	options := append([]aiengine.AIEngineConfigOption{}, h.cachedOptions...)
	options = append(options, aiengine.WithStateless(true))

	// Inject ContextPackage history if present (MVP: format as attached file content).
	if input.ContextPackage != nil {
		historyBlock := buildContextPackageHistoryBlock(input.ContextPackage)
		if historyBlock != "" {
			options = append(options, aiengine.WithAttachedFileContent(historyBlock))
		}
	}

	// Determine the user input text BEFORE building the engine — avoids
	// constructing an engine only to discover there's no input to send.
	// Prefer ContextPackage.user_input; fall back to the legacy PayloadJSON
	// content (for compatibility if ContextPackage is nil).
	userInput := ""
	var messageOptions []aiengine.AIEngineConfigOption
	if len(input.PayloadJSON) > 0 {
		content, interactive, syncEvent, decodedOptions, perr := yakAIInputContent(input)
		if perr != nil {
			return fmt.Errorf("stateless sendinput: decode input: %w", perr)
		}
		if interactive || syncEvent != nil {
			return fmt.Errorf("stateless sendinput: unsupported control input type %q", input.InputType)
		}
		userInput = content
		messageOptions = decodedOptions
	}
	if input.ContextPackage != nil && input.ContextPackage.UserInput != "" {
		userInput = input.ContextPackage.UserInput
	}
	if userInput == "" {
		return fmt.Errorf("stateless sendinput: empty user input")
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: runtime is closed")
	}
	if h.activeTurn != nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: turn already active")
	}
	engine, err := h.newEngine(options...)
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: new engine: %w", err)
	}
	if engine == nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: new engine returned nil")
	}
	turn := &statelessAITurn{engine: engine}
	h.activeTurn = turn
	h.mu.Unlock()

	go h.runTurn(ctx, turn, userInput, messageOptions...)
	return nil
}

func isInteractiveAISessionInput(inputType string) bool {
	switch strings.ToLower(strings.TrimSpace(inputType)) {
	case "interactive", "interactive_response", "review_response", "user_intervention":
		return true
	default:
		return false
	}
}

func isSyncAISessionInput(inputType string) bool {
	return strings.EqualFold(strings.TrimSpace(inputType), "sync_event")
}

func (h *statelessAIEngineRuntimeHandle) sendInterventionInput(input aiSessionInput) (bool, error) {
	event, err := buildYakAIInterventionEvent(input)
	if err != nil {
		return true, fmt.Errorf("stateless sendinput: %w", err)
	}
	turn, err := h.currentTurn("user intervention")
	if err != nil {
		if event.GetIsFreeInput() && h.runtimeOpenWithoutActiveTurn() {
			return false, nil
		}
		return true, err
	}
	if err := turn.engine.SendInputEvent(event); err != nil {
		return true, fmt.Errorf("stateless sendinput: send user intervention: %w", err)
	}
	return true, nil
}

func (h *statelessAIEngineRuntimeHandle) sendSyncInput(input aiSessionInput) error {
	_, _, syncEvent, _, err := yakAIInputContent(input)
	if err != nil {
		return fmt.Errorf("stateless sendinput: decode sync event: %w", err)
	}
	if syncEvent == nil {
		return fmt.Errorf("stateless sendinput: sync event is required")
	}
	turn, err := h.currentTurn("sync event")
	if err != nil {
		return err
	}
	if err := turn.engine.SendInputEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncEvent.SyncType,
		SyncJsonInput: syncEvent.SyncJSONInput,
	}); err != nil {
		return fmt.Errorf("stateless sendinput: send sync event: %w", err)
	}
	return nil
}

func (h *statelessAIEngineRuntimeHandle) currentTurn(inputKind string) (*statelessAITurn, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("stateless sendinput: runtime is closed")
	}
	if h.activeTurn == nil {
		return nil, fmt.Errorf("stateless sendinput: no active turn for %s", inputKind)
	}
	return h.activeTurn, nil
}

func (h *statelessAIEngineRuntimeHandle) runtimeOpenWithoutActiveTurn() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.closed && h.activeTurn == nil
}

func (h *statelessAIEngineRuntimeHandle) runTurn(
	ctx context.Context,
	turn *statelessAITurn,
	userInput string,
	options ...aiengine.AIEngineConfigOption,
) {
	err := turn.engine.SendMsg(userInput, options...)

	h.mu.Lock()
	closed := h.closed
	if h.activeTurn == turn {
		h.activeTurn = nil
	}
	h.mu.Unlock()
	turn.close()

	if err != nil && ctx.Err() == nil && !closed {
		h.emitter.Failed(yakAISendFailureCode(err), err.Error(), mustJSON(map[string]string{
			"runtime": "stateless_yak_ai_engine",
		}))
	}
}

func (h *statelessAIEngineRuntimeHandle) AppendContext(_ context.Context, _ aiSessionContextUpdate) error {
	// Stateless engine has no cross-turn state; AppendContext is a no-op.
	// (If needed in the future, the next turn's ContextPackage will carry it.)
	return nil
}

func (h *statelessAIEngineRuntimeHandle) Cancel(_ string) {
	h.closeRuntime()
}

func (h *statelessAIEngineRuntimeHandle) Close(_ string) {
	h.closeRuntime()
}

func (h *statelessAIEngineRuntimeHandle) closeRuntime() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	turn := h.activeTurn
	h.activeTurn = nil
	h.mu.Unlock()
	turn.close()
}

// buildContextPackageHistoryBlock formats the replayed conversation messages
// into a text block that aiengine injects as an "attached file" so the LLM
// sees prior turns. This is the MVP history-injection mechanism (S3 spec §11
// open question — resolved: use WithAttachedFileContent since no direct
// WithHistory option exists in aiengine).
func buildContextPackageHistoryBlock(pkg *aiv1.ContextPackage) string {
	if pkg == nil || len(pkg.Messages) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[Conversation history replayed by server (S3 stateless engine)]\n\n")
	for _, m := range pkg.Messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		fmt.Fprintf(&sb, "%s: %s\n", role, m.Content)
	}
	return sb.String()
}
