package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
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
	runtimeOptions, err := mergedYakRuntimeOptions(binding)
	if err != nil {
		return nil, fmt.Errorf("stateless bind: decode runtime options: %w", err)
	}
	return &statelessAIEngineRuntimeHandle{
		binding:              binding,
		emitter:              emitter,
		cachedOptions:        cachedOptions,
		runtime:              runtimeOptions,
		pinnedFocusReleaseID: pinnedFocusReleaseID(binding.RuntimeOptionSnapshotJSON),
		newEngine: func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
			return aiengine.NewAIEngine(opts...)
		},
	}, nil
}

type statelessTurnEngine interface {
	SendMsg(string, ...aiengine.AIEngineConfigOption) error
	WaitTaskFinish() error
	SendInputEvent(*ypb.AIInputEvent) error
	Config() *aiengine.AIEngineConfig
	Context() context.Context
	Close()
}

type statelessAITurn struct {
	engine      statelessTurnEngine
	turnID      string
	directForge bool
	runtime     yakRuntimeOptions
	binding     aiSessionBinding
	forgeInput  *chanx.UnlimitedChan[*ypb.AIInputEvent]
	closeOnce   sync.Once
}

func (t *statelessAITurn) close() {
	if t == nil || t.engine == nil {
		return
	}
	t.closeOnce.Do(t.engine.Close)
}

type statelessAIEngineRuntimeHandle struct {
	binding              aiSessionBinding
	emitter              aiSessionRuntimeEmitter
	cachedOptions        []aiengine.AIEngineConfigOption
	pinnedFocusReleaseID string
	runtime              yakRuntimeOptions

	mu           sync.Mutex
	activeTurn   *statelessAITurn
	forgeStarted bool
	closed       bool
	idleEmits    int
	idleEmitDone chan struct{}

	// newEngine is overridable in tests so lifecycle and control routing can
	// be verified without a real model provider.
	newEngine func(opts ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error)
}

func (h *statelessAIEngineRuntimeHandle) activeTurnID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.activeTurn == nil {
		return ""
	}
	return h.activeTurn.turnID
}

func (h *statelessAIEngineRuntimeHandle) SendInput(ctx context.Context, input aiSessionInput) error {
	if strings.EqualFold(strings.TrimSpace(input.InputType), "hotpatch") {
		return h.sendHotpatchInput(ctx, input)
	}
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

	var contextFocusRelease *aiv1.ContextFocusRelease
	if input.ContextPackage != nil {
		contextFocusRelease = input.ContextPackage.GetFocusRelease()
	}
	if h.pinnedFocusReleaseID != "" {
		if contextFocusRelease == nil {
			return fmt.Errorf("stateless sendinput: pinned focus release %q is missing from context package", h.pinnedFocusReleaseID)
		}
		if strings.TrimSpace(contextFocusRelease.GetReleaseId()) != h.pinnedFocusReleaseID {
			return fmt.Errorf("stateless sendinput: focus release mismatch: pinned %q, received %q", h.pinnedFocusReleaseID, contextFocusRelease.GetReleaseId())
		}
	}
	runtimeFocusName, err := registerContextFocusRelease(contextFocusRelease)
	if err != nil {
		return fmt.Errorf("stateless sendinput: %w", err)
	}
	if runtimeFocusName != "" {
		// Focus is consumed while NewAIEngine builds the ReAct operator. SendMsg
		// only applies per-message attached resources, so setting it there would
		// merely advertise the release as an available capability while the task
		// still ran through the default loop.
		options = append(options, aiengine.WithFocus(runtimeFocusName))
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
	directForge := !h.forgeStarted && strings.TrimSpace(h.runtime.ForgeName) != ""
	if directForge {
		h.forgeStarted = true
	}
	turn := &statelessAITurn{
		engine:      engine,
		turnID:      strings.TrimSpace(input.Ref.CommandID),
		directForge: directForge,
		runtime:     h.runtime,
		binding:     h.binding,
	}
	if directForge {
		turn.forgeInput = chanx.NewUnlimitedChan[*ypb.AIInputEvent](engine.Context(), 10)
	}
	h.activeTurn = turn
	h.mu.Unlock()

	go h.runTurn(ctx, turn, userInput, messageOptions...)
	return nil
}

func (h *statelessAIEngineRuntimeHandle) sendHotpatchInput(ctx context.Context, input aiSessionInput) error {
	event, err := buildYakAIHotpatchEvent(input)
	if err != nil {
		return fmt.Errorf("stateless sendinput: %w", err)
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: runtime is closed")
	}
	turn := h.activeTurn
	if isTaskScopedCapabilityHotpatchEvent(event) {
		if turn == nil {
			h.mu.Unlock()
			return fmt.Errorf("stateless sendinput: task-scoped capability hotpatch requires an active task")
		}
		if turn.directForge && turn.forgeInput != nil {
			if !turn.forgeInput.SafeFeedWithResult(event) {
				h.mu.Unlock()
				return fmt.Errorf("stateless sendinput: direct forge input channel is closed")
			}
		} else if err := turn.engine.SendInputEvent(event); err != nil {
			h.mu.Unlock()
			return fmt.Errorf("stateless sendinput: send task-scoped hotpatch: %w", err)
		}
		if h.closed || h.activeTurn != turn || turn.engine.Context().Err() != nil {
			h.mu.Unlock()
			return fmt.Errorf("stateless sendinput: active task closed while sending task-scoped hotpatch")
		}
		h.mu.Unlock()
		return nil
	}
	nextRuntime, err := applyYakAIHotpatchRuntime(h.runtime, event)
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: %w", err)
	}
	nextBinding := h.binding
	nextBinding.RuntimeOptionSnapshotJSON, err = json.Marshal(nextRuntime)
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: encode hotpatch runtime options: %w", err)
	}
	nextOptions, err := buildYakAIEngineOptions(ctx, nextBinding, h.emitter)
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: apply hotpatch runtime options: %w", err)
	}
	turn = h.activeTurn
	if turn != nil {
		if turn.directForge && turn.forgeInput != nil {
			if !turn.forgeInput.SafeFeedWithResult(event) {
				h.mu.Unlock()
				return fmt.Errorf("stateless sendinput: direct forge input channel is closed")
			}
		} else if err := turn.engine.SendInputEvent(event); err != nil {
			h.mu.Unlock()
			return fmt.Errorf("stateless sendinput: send config hotpatch: %w", err)
		}
	}
	// Commit the next-turn snapshot only after the live turn accepted the
	// hotpatch. Holding the lifecycle lock also prevents runTurn from closing
	// the engine between validation and delivery.
	h.runtime = nextRuntime
	h.binding = nextBinding
	h.cachedOptions = nextOptions
	h.mu.Unlock()
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
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return true, fmt.Errorf("stateless sendinput: runtime is closed")
	}
	turn := h.activeTurn
	if turn == nil {
		if event.GetIsFreeInput() {
			return false, nil
		}
		return true, fmt.Errorf("stateless sendinput: no active turn for user intervention")
	}
	if input.ReviewID != "" && event.GetInteractiveId() != input.ReviewID {
		return true, fmt.Errorf(
			"stateless sendinput: review id mismatch: expected %s, got %s",
			input.ReviewID,
			event.GetInteractiveId(),
		)
	}
	if input.TurnID != "" && turn.turnID != input.TurnID {
		return true, fmt.Errorf(
			"stateless sendinput: turn id mismatch: expected %s, active %s",
			input.TurnID,
			turn.turnID,
		)
	}
	if turn.directForge && turn.forgeInput != nil {
		if !turn.forgeInput.SafeFeedWithResult(event) {
			return true, fmt.Errorf("stateless sendinput: direct forge intervention channel is closed")
		}
	} else if err := turn.engine.SendInputEvent(event); err != nil {
		return true, fmt.Errorf("stateless sendinput: send user intervention: %w", err)
	}
	if h.closed || h.activeTurn != turn || turn.engine.Context().Err() != nil {
		return true, fmt.Errorf("stateless sendinput: active turn closed while sending user intervention")
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
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: runtime is closed")
	}
	turn := h.activeTurn
	if turn == nil {
		if strings.EqualFold(strings.TrimSpace(syncEvent.SyncType), "queue_info") {
			if h.idleEmits == 0 {
				h.idleEmitDone = make(chan struct{})
			}
			h.idleEmits++
			ref := input.Ref
			h.mu.Unlock()
			err := h.emitIdleQueueInfo(ref, syncEvent)
			h.finishIdleEmit()
			return err
		}
		h.mu.Unlock()
		return fmt.Errorf("stateless sendinput: no active turn for sync event")
	}
	defer h.mu.Unlock()
	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncEvent.SyncType,
		SyncJsonInput: syncEvent.SyncJSONInput,
		SyncID:        syncEvent.SyncID,
	}
	if turn.directForge && turn.forgeInput != nil {
		if !turn.forgeInput.SafeFeedWithResult(event) {
			return fmt.Errorf("stateless sendinput: direct forge sync channel is closed")
		}
	} else if err := turn.engine.SendInputEvent(event); err != nil {
		return fmt.Errorf("stateless sendinput: send sync event: %w", err)
	}
	if h.closed || h.activeTurn != turn || turn.engine.Context().Err() != nil {
		return fmt.Errorf("stateless sendinput: active turn closed while sending sync event")
	}
	return nil
}

func (h *statelessAIEngineRuntimeHandle) finishIdleEmit() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idleEmits == 0 {
		return
	}
	h.idleEmits--
	if h.idleEmits == 0 && h.idleEmitDone != nil {
		close(h.idleEmitDone)
		h.idleEmitDone = nil
	}
}

func (h *statelessAIEngineRuntimeHandle) emitIdleQueueInfo(
	ref aiSessionCommandRef,
	syncEvent *yakAISyncEvent,
) error {
	if h == nil || h.emitter == nil || syncEvent == nil {
		return fmt.Errorf("stateless sendinput: idle queue response emitter is unavailable")
	}
	event := &schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "queue_info",
		IsJson: true,
		IsSync: true,
		Content: mustJSON(map[string]any{
			"queue_name":    aireact.MainTaskQueueName,
			"total_tasks":   0,
			"is_processing": false,
			"tasks":         []any{},
			"queue_empty":   true,
		}),
		Timestamp: time.Now().Unix(),
		SyncID:    syncEvent.SyncID,
	}
	eventType := classifyYakAIEvent(event)
	payloadJSON := marshalYakAIOutputEvent(event)
	if emitter, ok := h.emitter.(aiSessionRuntimeRefEmitter); ok {
		if !emitter.EmitForRef(ref, eventType, payloadJSON) {
			return fmt.Errorf("stateless sendinput: idle queue response was not published")
		}
		return nil
	}
	h.emitter.Emit(eventType, payloadJSON)
	return nil
}

func (h *statelessAIEngineRuntimeHandle) runTurn(
	ctx context.Context,
	turn *statelessAITurn,
	userInput string,
	options ...aiengine.AIEngineConfigOption,
) {
	var err error
	if turn.directForge {
		err = runYakAIForgeDirect(
			turn.engine.Context(),
			turn.engine.Config(),
			turn.runtime,
			turn.binding,
			turn.forgeInput,
			h.emitter,
			userInput,
		)
	} else {
		err = turn.engine.SendMsg(userInput, options...)
		if err == nil {
			// SendMsg 只等待根任务。活跃 turn 期间接收的
			// user_intervention 可能已进入同一 ReAct 队列；必须等它们
			// 排空后才能 close stateless engine 并发布 turn.completed。
			err = turn.engine.WaitTaskFinish()
		}
	}

	h.mu.Lock()
	closed := h.closed
	singleRun := strings.EqualFold(strings.TrimSpace(h.binding.ExecutionMode), "single_run")
	singleRunTerminal := ctx.Err() == nil && !closed && singleRun
	turnFailure := err != nil && ctx.Err() == nil && !closed
	autoComplete := err == nil && singleRunTerminal
	if singleRunTerminal || (turnFailure && singleRun) {
		h.closed = true
	}
	h.mu.Unlock()
	turn.close()
	defer func() {
		h.mu.Lock()
		if h.activeTurn == turn {
			h.activeTurn = nil
		}
		h.mu.Unlock()
	}()

	if turnFailure {
		code := yakAISendFailureCode(err)
		if turn.directForge {
			code = "yak_ai_forge_failed"
		}
		detailJSON := mustJSON(map[string]string{
			"runtime": "stateless_yak_ai_engine",
		})
		if singleRun {
			if completer, ok := h.emitter.(aiSessionRuntimeTurnCompleter); ok {
				completer.FailTurn(turn.turnID, code, err.Error(), detailJSON)
				return
			}
			h.emitter.Failed(code, err.Error(), detailJSON)
			return
		}
		if reporter, ok := h.emitter.(aiSessionRuntimeTurnReporter); ok {
			reporter.TurnFailed(turn.turnID, code, err.Error(), detailJSON)
			return
		}
		if completer, ok := h.emitter.(aiSessionRuntimeTurnCompleter); ok {
			completer.FailTurn(turn.turnID, code, err.Error(), detailJSON)
			return
		}
		h.emitter.Failed(code, err.Error(), detailJSON)
		return
	}
	if err == nil && !singleRunTerminal && ctx.Err() == nil && !closed {
		resultJSON := mustJSON(map[string]string{
			"execution_mode": "multi_turn",
			"turn_id":        turn.turnID,
		})
		if reporter, ok := h.emitter.(aiSessionRuntimeTurnReporter); ok {
			reporter.TurnCompleted(turn.turnID, resultJSON)
			return
		}
		h.emitter.Emit(aiSessionRuntimeEventTurnCompleted, resultJSON)
		return
	}
	if autoComplete {
		resultJSON := mustJSON(map[string]string{
			"execution_mode": "single_run",
			"target_url":     strings.TrimSpace(h.binding.AuthorizedTargetURL),
			"turn_id":        turn.turnID,
		})
		if completer, ok := h.emitter.(aiSessionRuntimeTurnCompleter); ok {
			completer.DoneTurn(turn.turnID, resultJSON)
			return
		}
		h.emitter.Done(resultJSON)
	}
}

func (h *statelessAIEngineRuntimeHandle) AppendContext(_ context.Context, _ aiSessionContextUpdate) error {
	// Stateless engine has no cross-turn state; AppendContext is a no-op.
	// (If needed in the future, the next turn's ContextPackage will carry it.)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return fmt.Errorf("stateless append context: runtime is closed")
	}
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
	idleEmitDone := h.idleEmitDone
	h.mu.Unlock()
	if idleEmitDone != nil {
		<-idleEmitDone
	}
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
