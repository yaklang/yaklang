package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// selectAISessionRuntimeDriver picks the AI runtime driver based on the
// LEGION_AI_RUNTIME env var. Stateless is the default; stateful remains as an
// explicit rollback mode that takes effect after the container is restarted.
func selectAISessionRuntimeDriver() aiSessionRuntimeDriver {
	rawMode := os.Getenv("LEGION_AI_RUNTIME")
	mode, invalid := normalizeAISessionRuntimeMode(rawMode)
	if invalid {
		log.Warnf(
			"unsupported LEGION_AI_RUNTIME=%q; defaulting to %s",
			rawMode,
			aiSessionRuntimeModeStateless,
		)
	} else {
		log.Infof("selected AI session runtime: mode=%s", mode)
	}
	if mode == aiSessionRuntimeModeStateful {
		log.Warn("legacy AI session runtime rollback enabled explicitly")
		return newYakAIEngineRuntimeDriver()
	}
	return newStatelessAIEngineRuntimeDriver()
}

const (
	aiSessionRuntimeModeStateless = "stateless"
	aiSessionRuntimeModeStateful  = "stateful"
)

func normalizeAISessionRuntimeMode(rawMode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(rawMode)) {
	case "", aiSessionRuntimeModeStateless:
		return aiSessionRuntimeModeStateless, false
	case aiSessionRuntimeModeStateful:
		return aiSessionRuntimeModeStateful, false
	default:
		return aiSessionRuntimeModeStateless, true
	}
}

const aiSessionRuntimeEventInput = "ai.session.input"
const aiSessionRuntimeEventContextUpdated = "ai.session.context_updated"
const aiSessionRuntimeEventTurnCompleted = "ai.session.turn.completed"
const aiSessionRuntimeEventTurnFailed = "ai.session.turn.failed"
const aiSessionRuntimeEventTurnCancelled = "ai.session.turn.cancelled"

var errAISessionInputInFlight = errors.New("ai session input command is already in flight")

type aiSessionRuntimeDriver interface {
	Bind(context.Context, aiSessionBinding, aiSessionRuntimeEmitter) (aiSessionRuntimeHandle, error)
}

type aiSessionRuntimeHandle interface {
	SendInput(context.Context, aiSessionInput) error
	AppendContext(context.Context, aiSessionContextUpdate) error
	Cancel(string)
	Close(string)
}

// aiSessionRuntimeTurnRefProvider exposes the stable command identity of the
// live logical turn. Control commands (review decisions, free interventions,
// sync events) have their own command IDs, but events emitted by the running
// engine must remain causally attached to the turn that owns that engine.
type aiSessionRuntimeTurnRefProvider interface {
	activeTurnID() string
}

type aiSessionRuntimeEmitter interface {
	Emit(string, []byte)
	Done([]byte)
	Failed(string, string, []byte)
}

// aiSessionRuntimeRefEmitter publishes a runtime event with the exact command
// reference that admitted it. Control responses can otherwise be attributed
// to a concurrent Close or replacement input after the manager advances its
// mutable current ref.
type aiSessionRuntimeRefEmitter interface {
	EmitForRef(aiSessionCommandRef, string, []byte) bool
}

type aiSessionRuntimeTurnCompleter interface {
	DoneTurn(string, []byte)
	FailTurn(string, string, string, []byte)
}

// aiSessionRuntimeTurnReporter closes a logical turn without closing the
// reusable multi-turn Session runtime. Session terminal completion continues
// to use aiSessionRuntimeTurnCompleter for single_run execution only.
type aiSessionRuntimeTurnReporter interface {
	TurnCompleted(string, []byte)
	TurnFailed(string, string, string, []byte)
}

// aiSessionRuntimeFocusTurnReporter completes the run-scoped result sink while
// keeping the reusable AI Chat Session alive. A Focus release is a Turn/Task
// policy, not a Session terminal mode.
type aiSessionRuntimeFocusTurnReporter interface {
	FocusTurnCompleted(string, []byte)
	FocusTurnFailed(string, string, string, []byte)
	FocusTurnCancelled(string, string)
}

type aiSessionBinding struct {
	Ref                        aiSessionCommandRef
	ProjectID                  string
	Title                      string
	ProviderPolicySnapshotJSON []byte
	RuntimeOptionSnapshotJSON  []byte
	Attachments                []aiSessionAttachmentRef
	CredentialRefs             []aiSessionCredentialRef
	PlatformBearerToken        string
	HTTPClient                 *http.Client
	LegionResultRuntime        aicommon.LegionResultRuntime
	ExecutionMode              string
	AuthorizedFocusReleaseID   string
	AuthorizedTargetURL        string
}

type aiSessionAttachmentRef struct {
	AttachmentID string
	ObjectKey    string
	Filename     string
	ContentType  string
	SizeBytes    uint64
	SHA256       string
	DownloadURL  string
}

type aiSessionCredentialRef struct {
	CredentialID   string
	CredentialType string
	Scope          string
}

type aiSessionRuntimeBindOptions struct {
	PlatformBearerToken string
	PlatformAPIBaseURL  string
	NodeSessionID       string
	HTTPClient          *http.Client
	ResultSink          aiFocusResultSink
}

func bindAIFocusCodeWorkspaceEvidence(
	sink aiFocusResultSink,
	workspace *legionCodeWorkspaceRuntime,
) error {
	if workspace == nil {
		return nil
	}
	binder, ok := sink.(aiFocusCodeWorkspaceEvidenceBinder)
	if !ok {
		return fmt.Errorf("ai code finding result sink does not accept bound source evidence")
	}
	if err := binder.bindCodeWorkspaceEvidence(workspace.lockedRevision, workspace.sha256); err != nil {
		return fmt.Errorf("bind ai code finding source evidence: %w", err)
	}
	return nil
}

type aiSessionInput struct {
	Ref            aiSessionCommandRef
	InputType      string
	PayloadJSON    []byte
	ContextPackage *aiv1.ContextPackage // S3c: per-turn server-assembled context (history/tools/user_input)
	ReviewID       string
	TurnID         string
}

type aiSessionContextUpdate struct {
	Ref            aiSessionCommandRef
	Reason         string
	AttachmentRefs []aiSessionAttachmentRef
	CredentialRefs []aiSessionCredentialRef
}

type acceptedAISessionInput struct {
	ref            aiSessionCommandRef
	seq            uint64
	inputType      string
	payloadJSON    []byte
	handle         aiSessionRuntimeHandle
	contextPackage *aiv1.ContextPackage // S3c: carried through to handle.SendInput
	reviewID       string
	turnID         string
	duplicate      bool
}

type acceptedAISessionContextUpdate struct {
	ref         aiSessionCommandRef
	seq         uint64
	reason      string
	payloadJSON []byte
	update      aiSessionContextUpdate
	handle      aiSessionRuntimeHandle
}

type cancelledAISessionRuntime struct {
	ref         aiSessionCommandRef
	reason      string
	handle      aiSessionRuntimeHandle
	resultSink  *aiSessionResultSinkProxy
	applyHandle bool
}

type closedAISessionRuntime struct {
	ref             aiSessionCommandRef
	reason          string
	handle          aiSessionRuntimeHandle
	resultSink      *aiSessionResultSinkProxy
	applyHandle     bool
	alreadyTerminal bool
	acknowledge     bool
}

type aiSessionRuntimeManager struct {
	mu                 sync.Mutex
	sessions           map[string]*aiSessionRuntime
	bindings           map[string]aiSessionBindReservation
	terminalTombstones map[string]aiSessionTerminalTombstone
	terminalOrder      []string
	driver             aiSessionRuntimeDriver
}

type aiSessionBindReservation struct {
	commandID string
	epoch     uint64
}

type aiSessionTerminalTombstone struct {
	commandID string
	kind      string
	epoch     uint64
}

const maxAISessionTerminalTombstones = 1024

type aiSessionRuntime struct {
	emissionWG             sync.WaitGroup
	mu                     sync.Mutex
	ref                    aiSessionCommandRef
	bindCommandID          string
	bindEpoch              uint64
	bindIssuedAt           time.Time
	bindIssuedAtValid      bool
	retired                bool
	projectID              string
	title                  string
	seq                    uint64
	cancel                 context.CancelFunc
	handle                 aiSessionRuntimeHandle
	resultSink             *aiSessionResultSinkProxy
	processedInputCommands map[string]processedAISessionInput
	processedInputOrder    []string
	inFlightInputCommands  map[string]struct{}
	terminalCommandID      string
	terminalKind           string
	terminalReason         string
	terminalPublishFailed  bool
	executionMode          string
	codeWorkspace          *legionCodeWorkspaceRuntime
}

type processedAISessionInput struct {
	seq         uint64
	inputType   string
	payloadJSON []byte
}

func newAISessionRuntimeManager(driver aiSessionRuntimeDriver) *aiSessionRuntimeManager {
	if driver == nil {
		driver = noopAISessionRuntimeDriver{}
	}
	return &aiSessionRuntimeManager{
		sessions:           make(map[string]*aiSessionRuntime),
		bindings:           make(map[string]aiSessionBindReservation),
		terminalTombstones: make(map[string]aiSessionTerminalTombstone),
		driver:             driver,
	}
}

var (
	errAISessionBindFenced = errors.New("ai session bind was fenced")
	errAISessionBindRetry  = errors.New("ai session bind must be retried")
)

func (m *aiSessionRuntimeManager) Bind(
	parent context.Context,
	command *aiv1.BindAISessionCommand,
	publisher *aiSessionEventPublisher,
	options aiSessionRuntimeBindOptions,
) (aiSessionCommandRef, error) {
	ref := aiSessionRefFromBindCommand(command)
	bindIssuedAt, bindIssuedAtValid := aiSessionBindIssuedAt(command)
	bindEpoch := command.GetBindEpoch()

	m.mu.Lock()
	var replaced *aiSessionRuntime
	if pending, ok := m.bindings[ref.SessionID]; ok {
		if bindEpoch == 0 || bindEpoch <= pending.epoch {
			m.mu.Unlock()
			return ref, fmt.Errorf(
				"%w: bind %s cannot replace pending bind %s (epoch %d)",
				errAISessionBindFenced,
				ref.CommandID,
				pending.commandID,
				pending.epoch,
			)
		}
	}
	if tombstone, terminal := m.terminalTombstones[ref.SessionID]; terminal {
		if tombstone.kind != "bind_failed" || bindEpoch == 0 || bindEpoch <= tombstone.epoch {
			m.mu.Unlock()
			return ref, fmt.Errorf("%w: session %s is already terminal", errAISessionBindFenced, ref.SessionID)
		}
	}
	if existing, ok := m.sessions[ref.SessionID]; ok {
		if existing.ref.OwnerUserID != ref.OwnerUserID {
			m.mu.Unlock()
			return ref, fmt.Errorf("ai session owner mismatch: %s", existing.ref.OwnerUserID)
		}
		existing.mu.Lock()
		terminalCommandID := existing.terminalCommandID
		terminalKind := existing.terminalKind
		terminalPublishFailed := existing.terminalPublishFailed
		existing.mu.Unlock()
		if existing.bindCommandID == ref.CommandID {
			if terminalCommandID != "" || terminalPublishFailed {
				m.mu.Unlock()
				return ref, fmt.Errorf(
					"%w: original bind %s belongs to an unusable terminal runtime",
					errAISessionBindRetry,
					ref.CommandID,
				)
			}
			if err := bindAIFocusCodeWorkspaceEvidence(options.ResultSink, existing.codeWorkspace); err != nil {
				m.mu.Unlock()
				return ref, err
			}
			existing.resultSink.Set(options.ResultSink)
			m.mu.Unlock()
			return ref, nil
		}
		if aiSessionBindIsFenced(existing, bindEpoch, bindIssuedAt, bindIssuedAtValid) {
			m.mu.Unlock()
			return ref, fmt.Errorf("%w: stale bind %s for session %s", errAISessionBindFenced, ref.CommandID, ref.SessionID)
		}
		if terminalCommandID != "" {
			m.mu.Unlock()
			if terminalKind == "auto" {
				return ref, fmt.Errorf(
					"%w: runtime is publishing terminal command %s",
					errAISessionBindRetry,
					terminalCommandID,
				)
			}
			return ref, fmt.Errorf(
				"%w: runtime is terminal after command %s",
				errAISessionBindFenced,
				terminalCommandID,
			)
		}
		replaced = existing
	}
	m.bindings[ref.SessionID] = aiSessionBindReservation{commandID: ref.CommandID, epoch: bindEpoch}
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	codeWorkspace, publicRuntimeOptions, err := prepareLegionCodeWorkspace(
		ctx,
		command.GetRuntimeOptionSnapshotJson(),
		legionCodeWorkspaceMaterializeOptions{
			HTTPClient:          options.HTTPClient,
			PlatformAPIBaseURL:  options.PlatformAPIBaseURL,
			NodeSessionID:       options.NodeSessionID,
			PlatformBearerToken: options.PlatformBearerToken,
		},
	)
	if err != nil {
		cancel()
		m.clearBindReservation(ref.SessionID, ref.CommandID)
		return ref, fmt.Errorf("prepare source workspace: %w", err)
	}
	if err := bindAIFocusCodeWorkspaceEvidence(options.ResultSink, codeWorkspace); err != nil {
		cancel()
		_ = codeWorkspace.Cleanup()
		m.clearBindReservation(ref.SessionID, ref.CommandID)
		return ref, err
	}
	resultSink := newAISessionResultSinkProxy(options.ResultSink)
	focusRuntime, err := newLegionServerFocusRuntime(
		ctx,
		strings.TrimSpace(command.GetResultContext().GetTargetUrl()),
		resultSink,
		codeWorkspace,
	)
	if err != nil {
		cancel()
		_ = codeWorkspace.Cleanup()
		m.clearBindReservation(ref.SessionID, ref.CommandID)
		return ref, err
	}
	if codeWorkspace != nil {
		if _, err := resultSink.SubmitAsset(
			ctx,
			codeWorkspace.lockedAsset(strings.TrimSpace(command.GetResultContext().GetTargetUrl())),
		); err != nil {
			cancel()
			_ = codeWorkspace.Cleanup()
			m.clearBindReservation(ref.SessionID, ref.CommandID)
			return ref, fmt.Errorf("publish source_locked asset: %w", err)
		}
	}
	runtime := &aiSessionRuntime{
		ref:                    ref,
		bindCommandID:          ref.CommandID,
		bindEpoch:              bindEpoch,
		bindIssuedAt:           bindIssuedAt,
		bindIssuedAtValid:      bindIssuedAtValid,
		retired:                true,
		projectID:              strings.TrimSpace(command.GetProjectId()),
		title:                  strings.TrimSpace(command.GetTitle()),
		cancel:                 cancel,
		resultSink:             resultSink,
		processedInputCommands: make(map[string]processedAISessionInput),
		inFlightInputCommands:  make(map[string]struct{}),
		executionMode:          strings.TrimSpace(command.GetResultContext().GetExecutionMode()),
		codeWorkspace:          codeWorkspace,
	}
	runtime.handle = noopAISessionRuntimeHandle{}
	managedEmitter := &managedAISessionRuntimeEmitter{
		ctx:       ctx,
		runtime:   runtime,
		publisher: publisher,
		manager:   m,
	}
	if serverRuntime, ok := focusRuntime.(*legionServerFocusRuntime); ok {
		serverRuntime.emitEvent = managedEmitter.Emit
		serverRuntime.authorizedFocusReleaseID = strings.TrimSpace(command.GetResultContext().GetFocusReleaseId())
	}
	handle, err := m.driver.Bind(ctx, aiSessionBinding{
		Ref:                        ref,
		ProjectID:                  runtime.projectID,
		Title:                      runtime.title,
		ProviderPolicySnapshotJSON: cloneBytes(command.GetProviderPolicySnapshotJson()),
		RuntimeOptionSnapshotJSON:  publicRuntimeOptions,
		Attachments:                cloneAISessionAttachmentRefs(command.GetAttachments()),
		CredentialRefs:             cloneAISessionCredentialRefs(command.GetCredentialRefs()),
		PlatformBearerToken:        strings.TrimSpace(options.PlatformBearerToken),
		HTTPClient:                 options.HTTPClient,
		LegionResultRuntime:        focusRuntime,
		ExecutionMode:              strings.TrimSpace(command.GetResultContext().GetExecutionMode()),
		AuthorizedFocusReleaseID:   strings.TrimSpace(command.GetResultContext().GetFocusReleaseId()),
		AuthorizedTargetURL:        strings.TrimSpace(command.GetResultContext().GetTargetUrl()),
	}, managedEmitter)
	if err != nil {
		cancel()
		if handle != nil {
			handle.Close("runtime bind failed")
		}
		_ = codeWorkspace.Cleanup()
		m.clearBindReservation(ref.SessionID, ref.CommandID)
		return ref, err
	}
	if handle != nil {
		if codeWorkspace != nil {
			runtime.handle = &legionCodeWorkspaceRuntimeHandle{handle: handle, workspace: codeWorkspace}
		} else {
			runtime.handle = handle
		}
	}
	m.mu.Lock()
	pending, reserved := m.bindings[ref.SessionID]
	if !reserved || pending.commandID != ref.CommandID {
		m.mu.Unlock()
		cancel()
		runtime.handle.Close("bind reservation superseded")
		return ref, fmt.Errorf("%w: bind reservation changed for %s", errAISessionBindFenced, ref.SessionID)
	}
	current := m.sessions[ref.SessionID]
	if current != replaced {
		delete(m.bindings, ref.SessionID)
		m.mu.Unlock()
		cancel()
		runtime.handle.Close("active runtime changed during bind")
		return ref, fmt.Errorf("%w: active runtime changed for %s", errAISessionBindFenced, ref.SessionID)
	}
	if replaced != nil {
		replaced.mu.Lock()
		terminalCommandID := replaced.terminalCommandID
		terminalKind := replaced.terminalKind
		if terminalCommandID == "" {
			runtime.seq = replaced.seq
			runtime.processedInputCommands = cloneProcessedAISessionInputs(replaced.processedInputCommands)
			runtime.processedInputOrder = append([]string(nil), replaced.processedInputOrder...)
			runtime.inFlightInputCommands = cloneAISessionCommandSet(replaced.inFlightInputCommands)
			replaced.retired = true
			if replaced.cancel != nil {
				replaced.cancel()
			}
		}
		replaced.mu.Unlock()
		if terminalCommandID != "" {
			delete(m.bindings, ref.SessionID)
			m.mu.Unlock()
			cancel()
			runtime.handle.Close("rebind rejected after terminal claim")
			if terminalKind == "auto" {
				return ref, fmt.Errorf(
					"%w: runtime began publishing terminal command %s during bind",
					errAISessionBindRetry,
					terminalCommandID,
				)
			}
			return ref, fmt.Errorf("%w: runtime is terminal after command %s", errAISessionBindFenced, terminalCommandID)
		}
	}
	runtime.mu.Lock()
	runtime.retired = false
	runtime.mu.Unlock()
	m.sessions[ref.SessionID] = runtime
	m.deleteTerminalTombstoneLocked(ref.SessionID)
	delete(m.bindings, ref.SessionID)
	m.mu.Unlock()
	if replaced != nil {
		go func() {
			if replaced.handle != nil {
				replaced.handle.Close("runtime rebind")
			}
			replaced.emissionWG.Wait()
		}()
	}
	if codeWorkspace != nil {
		managedEmitter.Emit("source.workspace.ready", mustJSON(map[string]any{
			"workspace_id":    codeWorkspace.spec.WorkspaceID,
			"kind":            codeWorkspace.spec.Kind,
			"locked_revision": codeWorkspace.lockedRevision,
			"sha256":          codeWorkspace.sha256,
			"files":           codeWorkspace.files,
			"bytes":           codeWorkspace.bytes,
			"subpath":         codeWorkspace.spec.Subpath,
		}))
	}
	return ref, nil
}

func cloneProcessedAISessionInputs(
	input map[string]processedAISessionInput,
) map[string]processedAISessionInput {
	cloned := make(map[string]processedAISessionInput, len(input))
	for commandID, processed := range input {
		processed.payloadJSON = cloneBytes(processed.payloadJSON)
		cloned[commandID] = processed
	}
	return cloned
}

func cloneAISessionCommandSet(input map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(input))
	for commandID := range input {
		cloned[commandID] = struct{}{}
	}
	return cloned
}

func (m *aiSessionRuntimeManager) clearBindReservation(sessionID, commandID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pending, ok := m.bindings[sessionID]; ok && pending.commandID == commandID {
		delete(m.bindings, sessionID)
	}
}

func aiSessionBindIsFenced(existing *aiSessionRuntime, epoch uint64, issuedAt time.Time, issuedAtValid bool) bool {
	if existing == nil {
		return false
	}
	if existing.bindEpoch > 0 || epoch > 0 {
		return epoch == 0 || epoch <= existing.bindEpoch
	}
	if existing.bindIssuedAtValid {
		return !issuedAtValid || !issuedAt.After(existing.bindIssuedAt)
	}
	return false
}

func aiSessionBindIssuedAt(command *aiv1.BindAISessionCommand) (time.Time, bool) {
	if command == nil || command.GetMetadata() == nil || command.GetMetadata().GetIssuedAt() == nil {
		return time.Time{}, false
	}
	issuedAt := command.GetMetadata().GetIssuedAt()
	if err := issuedAt.CheckValid(); err != nil {
		return time.Time{}, false
	}
	return issuedAt.AsTime(), true
}

func (m *aiSessionRuntimeManager) AcceptInput(
	command *aiv1.PushAISessionInputCommand,
) (acceptedAISessionInput, error) {
	ref := aiSessionRefFromInputCommand(command)

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[ref.SessionID]
	if !ok {
		return acceptedAISessionInput{ref: ref}, fmt.Errorf("ai session runtime is not bound: %s", ref.SessionID)
	}
	if session.ref.OwnerUserID != ref.OwnerUserID {
		return acceptedAISessionInput{ref: ref}, fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	}

	payload, err := normalizeAISessionInputPayload(command.GetInputType(), command.GetInputJson())
	if err != nil {
		return acceptedAISessionInput{ref: ref}, err
	}

	session.mu.Lock()
	if ref.BindEpoch != session.ref.BindEpoch {
		currentEpoch := session.ref.BindEpoch
		session.mu.Unlock()
		return acceptedAISessionInput{ref: ref}, fmt.Errorf(
			"ai session bind epoch mismatch: command=%d runtime=%d session=%s",
			ref.BindEpoch,
			currentEpoch,
			ref.SessionID,
		)
	}
	if processed, ok := session.processedInputCommands[ref.CommandID]; ok {
		ref.RunID = session.ref.RunID
		ref.BindEpoch = session.ref.BindEpoch
		session.mu.Unlock()
		return acceptedAISessionInput{
			ref:         ref,
			seq:         processed.seq,
			inputType:   processed.inputType,
			payloadJSON: cloneBytes(processed.payloadJSON),
			duplicate:   true,
		}, nil
	}
	if _, ok := session.inFlightInputCommands[ref.CommandID]; ok {
		session.mu.Unlock()
		return acceptedAISessionInput{ref: ref}, errAISessionInputInFlight
	}
	if session.terminalCommandID != "" || session.terminalPublishFailed {
		terminalCommandID := session.terminalCommandID
		session.mu.Unlock()
		if terminalCommandID == "" {
			return acceptedAISessionInput{ref: ref}, fmt.Errorf(
				"ai session terminal publication failed; rebind is required: %s",
				ref.SessionID,
			)
		}
		return acceptedAISessionInput{ref: ref}, fmt.Errorf(
			"ai session runtime is terminal after command %s: %s",
			terminalCommandID,
			ref.SessionID,
		)
	}
	session.inFlightInputCommands[ref.CommandID] = struct{}{}
	session.seq++
	ref.RunID = session.ref.RunID
	ref.BindEpoch = session.ref.BindEpoch
	session.ref.CommandID = ref.CommandID
	seq := session.seq
	handle := session.handle
	session.mu.Unlock()

	inputType := strings.TrimSpace(command.GetInputType())
	if inputType == "" {
		inputType = "message"
	}
	return acceptedAISessionInput{
		ref:            ref,
		seq:            seq,
		inputType:      inputType,
		payloadJSON:    payload,
		handle:         handle,
		contextPackage: command.GetContextPackage(),
		reviewID:       strings.TrimSpace(command.GetReviewId()),
		turnID:         strings.TrimSpace(command.GetTurnId()),
	}, nil
}

func (m *aiSessionRuntimeManager) CompleteInput(accepted acceptedAISessionInput, succeeded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[strings.TrimSpace(accepted.ref.SessionID)]
	if !ok {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	commandID := strings.TrimSpace(accepted.ref.CommandID)
	delete(session.inFlightInputCommands, commandID)
	if !succeeded || commandID == "" {
		return
	}
	if _, exists := session.processedInputCommands[commandID]; exists {
		return
	}
	session.processedInputCommands[commandID] = processedAISessionInput{
		seq:         accepted.seq,
		inputType:   accepted.inputType,
		payloadJSON: cloneBytes(accepted.payloadJSON),
	}
	session.processedInputOrder = append(session.processedInputOrder, commandID)
	const processedInputLimit = 1024
	if len(session.processedInputOrder) > processedInputLimit {
		oldest := session.processedInputOrder[0]
		session.processedInputOrder = session.processedInputOrder[1:]
		delete(session.processedInputCommands, oldest)
	}
}

func (m *aiSessionRuntimeManager) AcceptContextUpdate(
	command *aiv1.AppendAISessionContextCommand,
) (acceptedAISessionContextUpdate, error) {
	ref := aiSessionRefFromContextCommand(command)

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[ref.SessionID]
	if !ok {
		return acceptedAISessionContextUpdate{ref: ref}, fmt.Errorf("ai session runtime is not bound: %s", ref.SessionID)
	}
	if session.ref.OwnerUserID != ref.OwnerUserID {
		return acceptedAISessionContextUpdate{ref: ref}, fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	}

	attachments := cloneAISessionAttachmentRefs(command.GetAttachments())
	credentials := cloneAISessionCredentialRefs(command.GetCredentialRefs())
	reason := strings.TrimSpace(command.GetReason())

	session.mu.Lock()
	if ref.BindEpoch != session.ref.BindEpoch {
		currentEpoch := session.ref.BindEpoch
		session.mu.Unlock()
		return acceptedAISessionContextUpdate{ref: ref}, fmt.Errorf(
			"ai session bind epoch mismatch: command=%d runtime=%d session=%s",
			ref.BindEpoch,
			currentEpoch,
			ref.SessionID,
		)
	}
	if session.terminalCommandID != "" || session.terminalPublishFailed {
		terminalCommandID := session.terminalCommandID
		session.mu.Unlock()
		if terminalCommandID == "" {
			return acceptedAISessionContextUpdate{ref: ref}, fmt.Errorf(
				"ai session terminal publication failed; rebind is required: %s",
				ref.SessionID,
			)
		}
		return acceptedAISessionContextUpdate{ref: ref}, fmt.Errorf(
			"ai session runtime is terminal after command %s: %s",
			terminalCommandID,
			ref.SessionID,
		)
	}
	session.seq++
	ref.RunID = session.ref.RunID
	ref.BindEpoch = session.ref.BindEpoch
	session.ref.CommandID = ref.CommandID
	seq := session.seq
	handle := session.handle
	session.mu.Unlock()

	payloadJSON, err := json.Marshal(map[string]any{
		"reason":                 reason,
		"added_attachment_count": len(attachments),
		"added_credential_count": len(credentials),
	})
	if err != nil {
		return acceptedAISessionContextUpdate{ref: ref}, err
	}

	return acceptedAISessionContextUpdate{
		ref:         ref,
		seq:         seq,
		reason:      reason,
		payloadJSON: payloadJSON,
		update: aiSessionContextUpdate{
			Ref:            ref,
			Reason:         reason,
			AttachmentRefs: attachments,
			CredentialRefs: credentials,
		},
		handle: handle,
	}, nil
}

func (m *aiSessionRuntimeManager) Cancel(
	command *aiv1.CancelAISessionCommand,
) (cancelledAISessionRuntime, error) {
	ref := aiSessionRefFromCancelCommand(command)
	reason := strings.TrimSpace(command.GetReason())
	if reason == "" {
		reason = "platform cancel requested"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[ref.SessionID]
	if !ok {
		return cancelledAISessionRuntime{ref: ref, reason: reason}, nil
	}
	if session.ref.OwnerUserID != ref.OwnerUserID {
		return cancelledAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	}
	session.mu.Lock()
	if ref.BindEpoch != session.ref.BindEpoch {
		currentEpoch := session.ref.BindEpoch
		session.mu.Unlock()
		return cancelledAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf(
			"ai session bind epoch mismatch: command=%d runtime=%d session=%s",
			ref.BindEpoch,
			currentEpoch,
			ref.SessionID,
		)
	}
	ref.RunID = session.ref.RunID
	ref.BindEpoch = session.ref.BindEpoch
	handle := session.handle
	applyHandle := false
	if session.terminalCommandID == "" {
		session.terminalCommandID = ref.CommandID
		session.terminalKind = "cancel"
		session.terminalReason = reason
		applyHandle = true
		if session.cancel != nil {
			session.cancel()
		}
	} else {
		if session.terminalCommandID != ref.CommandID || session.terminalKind != "cancel" {
			session.mu.Unlock()
			return cancelledAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf(
				"ai session terminal command conflicts with pending %s command %s",
				session.terminalKind,
				session.terminalCommandID,
			)
		}
		reason = session.terminalReason
	}
	session.mu.Unlock()
	return cancelledAISessionRuntime{
		ref:         ref,
		reason:      reason,
		handle:      handle,
		resultSink:  session.resultSink,
		applyHandle: applyHandle,
	}, nil
}

func (m *aiSessionRuntimeManager) Close(
	command *aiv1.CloseAISessionCommand,
) (closedAISessionRuntime, error) {
	ref := aiSessionRefFromCloseCommand(command)
	reason := strings.TrimSpace(command.GetReason())
	if reason == "" {
		reason = "platform close requested"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[ref.SessionID]
	if !ok {
		tombstone, known := m.terminalTombstones[ref.SessionID]
		acknowledge := !known || (tombstone.kind == "close" && tombstone.commandID == ref.CommandID)
		return closedAISessionRuntime{
			ref:             ref,
			reason:          reason,
			alreadyTerminal: true,
			acknowledge:     acknowledge,
		}, nil
	}
	if session.ref.OwnerUserID != ref.OwnerUserID {
		return closedAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	}
	session.mu.Lock()
	if ref.BindEpoch != session.ref.BindEpoch {
		currentEpoch := session.ref.BindEpoch
		session.mu.Unlock()
		return closedAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf(
			"ai session bind epoch mismatch: command=%d runtime=%d session=%s",
			ref.BindEpoch,
			currentEpoch,
			ref.SessionID,
		)
	}
	ref.RunID = session.ref.RunID
	ref.BindEpoch = session.ref.BindEpoch
	handle := session.handle
	applyHandle := false
	if session.terminalCommandID == "" {
		session.terminalCommandID = ref.CommandID
		session.terminalKind = "close"
		session.terminalReason = reason
		applyHandle = true
		if session.cancel != nil {
			session.cancel()
		}
	} else {
		if session.terminalKind == "auto" {
			session.mu.Unlock()
			return closedAISessionRuntime{ref: ref, reason: reason, alreadyTerminal: true}, nil
		}
		if session.terminalCommandID != ref.CommandID || session.terminalKind != "close" {
			session.mu.Unlock()
			return closedAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf(
				"ai session terminal command conflicts with pending %s command %s",
				session.terminalKind,
				session.terminalCommandID,
			)
		}
		reason = session.terminalReason
	}
	session.mu.Unlock()
	return closedAISessionRuntime{
		ref:         ref,
		reason:      reason,
		handle:      handle,
		resultSink:  session.resultSink,
		applyHandle: applyHandle,
	}, nil
}

func (m *aiSessionRuntimeManager) CompleteTerminal(
	ref aiSessionCommandRef,
	kind string,
) error {
	m.mu.Lock()
	session, ok := m.sessions[ref.SessionID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	session.mu.Lock()
	switch {
	case session.ref.OwnerUserID != ref.OwnerUserID:
		session.mu.Unlock()
		m.mu.Unlock()
		return fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	case ref.BindEpoch > 0 && session.bindEpoch != ref.BindEpoch:
		currentEpoch := session.bindEpoch
		session.mu.Unlock()
		m.mu.Unlock()
		return fmt.Errorf(
			"ai session bind epoch mismatch: got %d want %d",
			ref.BindEpoch,
			currentEpoch,
		)
	case session.terminalCommandID != ref.CommandID:
		currentCommandID := session.terminalCommandID
		session.mu.Unlock()
		m.mu.Unlock()
		return fmt.Errorf(
			"ai session terminal command mismatch: got %s want %s",
			ref.CommandID,
			currentCommandID,
		)
	case session.terminalKind != kind:
		currentKind := session.terminalKind
		session.mu.Unlock()
		m.mu.Unlock()
		return fmt.Errorf(
			"ai session terminal kind mismatch: got %s want %s",
			kind,
			currentKind,
		)
	}
	delete(m.sessions, ref.SessionID)
	m.recordTerminalTombstoneLocked(ref.SessionID, aiSessionTerminalTombstone{
		commandID: ref.CommandID,
		kind:      kind,
		epoch:     session.bindEpoch,
	})
	if session.cancel != nil {
		session.cancel()
	}
	workspace := session.codeWorkspace
	session.mu.Unlock()
	m.mu.Unlock()
	return workspace.Cleanup()
}

func (m *aiSessionRuntimeManager) RetireAfterBindFailure(ref aiSessionCommandRef) {
	m.mu.Lock()
	if pending, ok := m.bindings[ref.SessionID]; ok && pending.epoch > ref.BindEpoch {
		m.mu.Unlock()
		return
	}
	session, ok := m.sessions[ref.SessionID]
	if !ok {
		if ref.BindEpoch > 0 {
			if tombstone, exists := m.terminalTombstones[ref.SessionID]; !exists || (tombstone.kind == "bind_failed" && ref.BindEpoch > tombstone.epoch) {
				m.recordTerminalTombstoneLocked(ref.SessionID, aiSessionTerminalTombstone{
					commandID: ref.CommandID,
					kind:      "bind_failed",
					epoch:     ref.BindEpoch,
				})
			}
		}
		m.mu.Unlock()
		return
	}
	if session.ref.OwnerUserID != ref.OwnerUserID || ref.BindEpoch == 0 || ref.BindEpoch <= session.bindEpoch {
		m.mu.Unlock()
		return
	}
	session.mu.Lock()
	session.retired = true
	session.terminalCommandID = ref.CommandID
	session.terminalKind = "bind_failed"
	session.terminalReason = "replacement runtime bind failed"
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()
	delete(m.sessions, ref.SessionID)
	m.recordTerminalTombstoneLocked(ref.SessionID, aiSessionTerminalTombstone{
		commandID: ref.CommandID,
		kind:      "bind_failed",
		epoch:     ref.BindEpoch,
	})
	m.mu.Unlock()
	go func() {
		if session.handle != nil {
			session.handle.Close("replacement runtime bind failed")
		}
		session.emissionWG.Wait()
	}()
}

func (m *aiSessionRuntimeManager) recordTerminalTombstoneLocked(
	sessionID string,
	tombstone aiSessionTerminalTombstone,
) {
	if sessionID == "" {
		return
	}
	if _, exists := m.terminalTombstones[sessionID]; !exists {
		m.terminalOrder = append(m.terminalOrder, sessionID)
	}
	m.terminalTombstones[sessionID] = tombstone
	for len(m.terminalOrder) > maxAISessionTerminalTombstones {
		oldest := m.terminalOrder[0]
		m.terminalOrder = m.terminalOrder[1:]
		delete(m.terminalTombstones, oldest)
	}
}

func (m *aiSessionRuntimeManager) deleteTerminalTombstoneLocked(sessionID string) {
	if _, exists := m.terminalTombstones[sessionID]; !exists {
		return
	}
	delete(m.terminalTombstones, sessionID)
	for index, candidate := range m.terminalOrder {
		if candidate != sessionID {
			continue
		}
		m.terminalOrder = append(m.terminalOrder[:index], m.terminalOrder[index+1:]...)
		return
	}
}

func (b *legionJobBridge) handleAISessionBind(ctx context.Context, raw []byte) error {
	var command aiv1.BindAISessionCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session bind command: %w", err)
	}

	ref := aiSessionRefFromBindCommand(&command)
	if err := validateAISessionBindCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_session_bind_command", err)
	}

	session, _ := b.agent.node.GetSessionState()
	resultSink, err := newLegionAIFocusResultSink(
		b.publisher,
		command.GetMetadata().GetCommandId(),
		command.GetResultContext(),
	)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_focus_result_context", err)
	}
	ref, err = b.ensureAIRuntime().Bind(
		b.agent.node.GetRootContext(),
		&command,
		b.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{
			PlatformBearerToken: session.SessionToken,
			PlatformAPIBaseURL:  b.agent.resolvePlatformAPIBaseURL(),
			NodeSessionID:       session.SessionID,
			HTTPClient:          b.agent.httpClient,
			ResultSink:          resultSink,
		},
	)
	if err != nil {
		if errors.Is(err, errAISessionBindRetry) {
			// A terminal publication is still settling, or the original bind
			// belongs to a runtime whose terminal publication failed. Returning
			// the error makes JetStream NAK the command so a recoverable bind is
			// not silently consumed.
			return err
		}
		if errors.Is(err, errAISessionBindFenced) {
			// Delayed/duplicate generations are transport-successful no-ops. A
			// terminal bind-failed event would incorrectly fail the newer runtime.
			return nil
		}
		failureErr := err
		if strings.Contains(err.Error(), "prepare source workspace") ||
			strings.Contains(err.Error(), "source_workspace") {
			if publishErr := b.ensureAIPublisher().PublishEvent(
				ctx,
				ref,
				1,
				"source.workspace.failed",
				mustJSON(map[string]string{
					"code":    "source_workspace_materialize_failed",
					"message": "source workspace could not be materialized",
				}),
			); publishErr != nil {
				return publishErr
			}
			failureErr = fmt.Errorf("source workspace materialization failed")
		}
		if publishErr := b.publishAISessionCommandFailure(ctx, ref, "ai_session_bind_failed", failureErr); publishErr != nil {
			return publishErr
		}
		b.ensureAIRuntime().RetireAfterBindFailure(ref)
		return nil
	}
	return b.ensureAIPublisher().PublishReady(ctx, ref)
}

func (b *legionJobBridge) handleAISessionInput(ctx context.Context, raw []byte) error {
	var command aiv1.PushAISessionInputCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session input command: %w", err)
	}

	ref := aiSessionRefFromInputCommand(&command)
	if err := validateAISessionInputCommand(&command); err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_session_input_command", err)
	}
	if expected := strings.TrimSpace(command.GetExpectedNodeSessionId()); expected != "" {
		nodeSession, ok := b.agent.node.GetSessionState()
		if !ok || strings.TrimSpace(nodeSession.SessionID) != expected {
			return b.publishAISessionCommandFailure(
				ctx,
				ref,
				"ai_session_review_fenced",
				fmt.Errorf("ai session review expected node session %s", expected),
			)
		}
	}

	runtime := b.ensureAIRuntime()
	accepted, err := runtime.AcceptInput(&command)
	if err != nil {
		if errors.Is(err, errAISessionInputInFlight) {
			return err
		}
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_input_failed", err)
	}
	if accepted.duplicate {
		return b.ensureAIPublisher().PublishEvent(
			ctx,
			accepted.ref,
			accepted.seq,
			aiSessionRuntimeEventInput,
			accepted.payloadJSON,
		)
	}
	succeeded := false
	defer func() {
		runtime.CompleteInput(accepted, succeeded)
	}()
	if accepted.handle != nil {
		if err := accepted.handle.SendInput(ctx, aiSessionInput{
			Ref:            accepted.ref,
			InputType:      accepted.inputType,
			PayloadJSON:    accepted.payloadJSON,
			ContextPackage: accepted.contextPackage,
			ReviewID:       accepted.reviewID,
			TurnID:         accepted.turnID,
		}); err != nil {
			return b.publishAISessionCommandFailure(ctx, accepted.ref, "ai_session_runtime_input_failed", err)
		}
	}
	succeeded = true
	return b.ensureAIPublisher().PublishEvent(
		ctx,
		accepted.ref,
		accepted.seq,
		aiSessionRuntimeEventInput,
		accepted.payloadJSON,
	)
}

func (b *legionJobBridge) handleAISessionAppendContext(ctx context.Context, raw []byte) error {
	var command aiv1.AppendAISessionContextCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session append context command: %w", err)
	}

	ref := aiSessionRefFromContextCommand(&command)
	if err := validateAISessionContextCommand(&command); err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_session_context_command", err)
	}

	accepted, err := b.ensureAIRuntime().AcceptContextUpdate(&command)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_context_failed", err)
	}
	if err := b.ensureAIPublisher().PublishEvent(
		ctx,
		accepted.ref,
		accepted.seq,
		aiSessionRuntimeEventContextUpdated,
		accepted.payloadJSON,
	); err != nil {
		return err
	}
	if accepted.handle == nil {
		return nil
	}
	if err := accepted.handle.AppendContext(ctx, accepted.update); err != nil {
		return b.publishAISessionCommandFailure(ctx, accepted.ref, "ai_session_runtime_context_failed", err)
	}
	return nil
}

func (b *legionJobBridge) handleAISessionCancel(ctx context.Context, raw []byte) error {
	var command aiv1.CancelAISessionCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session cancel command: %w", err)
	}

	ref := aiSessionRefFromCancelCommand(&command)
	if err := validateAISessionCancelCommand(&command); err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_session_cancel_command", err)
	}

	cancelled, err := b.ensureAIRuntime().Cancel(&command)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_cancel_failed", err)
	}
	if cancelled.applyHandle && cancelled.handle != nil {
		cancelled.handle.Cancel(cancelled.reason)
	}
	if err := cancelled.resultSink.Cancel(ctx, cancelled.reason); err != nil {
		return fmt.Errorf("publish focus result cancelled: %w", err)
	}
	if err := b.ensureAIPublisher().PublishCancelled(ctx, cancelled.ref, cancelled.reason); err != nil {
		return err
	}
	return b.ensureAIRuntime().CompleteTerminal(cancelled.ref, "cancel")
}

func (b *legionJobBridge) handleAISessionClose(ctx context.Context, raw []byte) error {
	var command aiv1.CloseAISessionCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai session close command: %w", err)
	}

	ref := aiSessionRefFromCloseCommand(&command)
	if err := validateAISessionCloseCommand(&command); err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "invalid_ai_session_close_command", err)
	}

	closed, err := b.ensureAIRuntime().Close(&command)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_close_failed", err)
	}
	if closed.alreadyTerminal && !closed.acknowledge {
		return nil
	}
	if closed.alreadyTerminal {
		// Close is an idempotent command. The runtime can already be absent when
		// Legion retries after losing the first acknowledgement (for example after
		// a node restart). Re-publish the deterministic close event so the server
		// can converge its closing state instead of timing out in close_failed.
		resultJSON := mustJSON(map[string]string{
			"reason":           closed.reason,
			"closed_by":        "platform",
			"already_terminal": "true",
		})
		return b.ensureAIPublisher().PublishClose(ctx, closed.ref, resultJSON)
	}
	if closed.applyHandle && closed.handle != nil {
		closed.handle.Close(closed.reason)
	}
	resultJSON := mustJSON(map[string]string{
		"reason":    closed.reason,
		"closed_by": "platform",
	})
	if err := closed.resultSink.Succeed(ctx, resultJSON); err != nil {
		return fmt.Errorf("publish focus result succeeded: %w", err)
	}
	if err := b.ensureAIPublisher().PublishClose(ctx, closed.ref, resultJSON); err != nil {
		return err
	}
	return b.ensureAIRuntime().CompleteTerminal(closed.ref, "close")
}

func (b *legionJobBridge) publishAISessionCommandFailure(
	ctx context.Context,
	ref aiSessionCommandRef,
	code string,
	err error,
) error {
	if strings.TrimSpace(ref.SessionID) == "" {
		return err
	}
	detail, marshalErr := json.Marshal(map[string]string{
		"owner_user_id": ref.OwnerUserID,
	})
	if marshalErr != nil {
		detail = nil
	}
	return b.ensureAIPublisher().PublishFailed(ctx, ref, code, err.Error(), detail)
}

func (b *legionJobBridge) ensureAIRuntime() *aiSessionRuntimeManager {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.aiRuntime == nil {
		b.aiRuntime = newAISessionRuntimeManager(selectAISessionRuntimeDriver())
	}
	return b.aiRuntime
}

func cloneAISessionAttachmentRefs(items []*aiv1.AISessionAttachmentRef) []aiSessionAttachmentRef {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]aiSessionAttachmentRef, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cloned = append(cloned, aiSessionAttachmentRef{
			AttachmentID: strings.TrimSpace(item.GetAttachmentId()),
			ObjectKey:    strings.TrimSpace(item.GetObjectKey()),
			Filename:     strings.TrimSpace(item.GetFilename()),
			ContentType:  strings.TrimSpace(item.GetContentType()),
			SizeBytes:    item.GetSizeBytes(),
			SHA256:       strings.TrimSpace(item.GetSha256()),
			DownloadURL:  strings.TrimSpace(item.GetDownloadUrl()),
		})
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func cloneAISessionCredentialRefs(items []*aiv1.AISessionCredentialRef) []aiSessionCredentialRef {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]aiSessionCredentialRef, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cloned = append(cloned, aiSessionCredentialRef{
			CredentialID:   strings.TrimSpace(item.GetCredentialId()),
			CredentialType: strings.TrimSpace(item.GetCredentialType()),
			Scope:          strings.TrimSpace(item.GetScope()),
		})
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func (b *legionJobBridge) ensureAIPublisher() *aiSessionEventPublisher {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.aiPublisher == nil {
		b.aiPublisher = newAISessionEventPublisher(b.agent.node)
	}
	return b.aiPublisher
}

func validateAISessionBindCommand(nodeID string, command *aiv1.BindAISessionCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session bind metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session bind command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai session bind target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai session bind target_node_id mismatch: %s", command.GetTargetNodeId())
	case command.GetSession() == nil:
		return fmt.Errorf("ai session bind session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session bind session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session bind owner_user_id is required")
	}
	if resultContext := command.GetResultContext(); resultContext != nil {
		if _, err := validateLegionAIFocusResultContext(
			command.GetMetadata().GetCommandId(),
			resultContext,
		); err != nil {
			return err
		}
		if strings.TrimSpace(command.GetSession().GetRunId()) !=
			strings.TrimSpace(resultContext.GetFocusRunId()) {
			return fmt.Errorf("ai session run_id must match focus result focus_run_id")
		}
	}
	runtimeOptions, err := decodeYakRuntimeOptions(command.GetRuntimeOptionSnapshotJson(), true)
	if err != nil {
		return fmt.Errorf("invalid ai session runtime options: %w", err)
	}
	if err := validateYakRiskJudgementScopePin(
		runtimeOptions.RiskJudgementScope,
		command.GetResultContext(),
	); err != nil {
		return fmt.Errorf("invalid ai session risk judgement scope: %w", err)
	}
	if runtimeOptions.SourceWorkspace != nil {
		spec := *runtimeOptions.SourceWorkspace
		if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
			return err
		}
		if command.GetResultContext() == nil {
			return fmt.Errorf("source_workspace requires an ai focus result context")
		}
		expectedTarget, err := legionCodeWorkspaceSentinel(spec.WorkspaceID)
		if err != nil {
			return err
		}
		target, err := normalizeServerFocusURL(command.GetResultContext().GetTargetUrl())
		if err != nil || target.String() != expectedTarget {
			return fmt.Errorf("source_workspace target_url must equal %s", expectedTarget)
		}
	} else if command.GetResultContext() != nil {
		target, targetErr := normalizeServerFocusURL(command.GetResultContext().GetTargetUrl())
		if targetErr == nil && strings.EqualFold(target.Hostname(), "workspace.invalid") {
			return fmt.Errorf("workspace.invalid target requires source_workspace")
		}
	}
	return nil
}

func validateAISessionInputCommand(command *aiv1.PushAISessionInputCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session input metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session input command_id is required")
	case command.GetSession() == nil:
		return fmt.Errorf("ai session input session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session input session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session input owner_user_id is required")
	case strings.TrimSpace(command.GetReviewId()) != "" &&
		strings.TrimSpace(command.GetTurnId()) == "":
		return fmt.Errorf("ai session review turn_id is required")
	case strings.TrimSpace(command.GetReviewId()) != "" &&
		strings.TrimSpace(command.GetExpectedNodeSessionId()) == "":
		return fmt.Errorf("ai session review expected_node_session_id is required")
	default:
		_, err := normalizeAISessionInputPayload(command.GetInputType(), command.GetInputJson())
		return err
	}
}

func validateAISessionContextCommand(command *aiv1.AppendAISessionContextCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session context metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session context command_id is required")
	case command.GetSession() == nil:
		return fmt.Errorf("ai session context session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session context session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session context owner_user_id is required")
	case len(command.GetAttachments()) == 0 && len(command.GetCredentialRefs()) == 0:
		return fmt.Errorf("ai session context attachments or credential_refs are required")
	default:
		return nil
	}
}

func validateAISessionCancelCommand(command *aiv1.CancelAISessionCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session cancel metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session cancel command_id is required")
	case command.GetSession() == nil:
		return fmt.Errorf("ai session cancel session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session cancel session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session cancel owner_user_id is required")
	default:
		return nil
	}
}

func validateAISessionCloseCommand(command *aiv1.CloseAISessionCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai session close metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai session close command_id is required")
	case command.GetSession() == nil:
		return fmt.Errorf("ai session close session reference is required")
	case strings.TrimSpace(command.GetSession().GetSessionId()) == "":
		return fmt.Errorf("ai session close session_id is required")
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai session close owner_user_id is required")
	default:
		return nil
	}
}

func normalizeAISessionInputPayload(inputType string, raw []byte) ([]byte, error) {
	normalizedInputType := strings.TrimSpace(inputType)
	if normalizedInputType == "" {
		normalizedInputType = "message"
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		if normalizedInputType == "sync_event" {
			return nil, fmt.Errorf("ai session sync_event input_json must not be empty")
		}
		return json.Marshal(map[string]string{
			"input_type": normalizedInputType,
			"role":       "user",
		})
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("ai session input_json must be valid json: %w", err)
	}
	if normalizedInputType == "sync_event" {
		return cloneBytes([]byte(trimmed)), nil
	}
	if object, ok := decoded.(map[string]any); ok {
		object["input_type"] = normalizedInputType
		if _, exists := object["role"]; !exists {
			object["role"] = "user"
		}
		return json.Marshal(object)
	}
	return json.Marshal(map[string]any{
		"input_type": normalizedInputType,
		"role":       "user",
		"value":      decoded,
	})
}

func aiSessionRefFromBindCommand(command *aiv1.BindAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		BindEpoch:   command.GetBindEpoch(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromInputCommand(command *aiv1.PushAISessionInputCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		BindEpoch:   command.GetSession().GetBindEpoch(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromContextCommand(command *aiv1.AppendAISessionContextCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		BindEpoch:   command.GetSession().GetBindEpoch(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromCancelCommand(command *aiv1.CancelAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		BindEpoch:   command.GetSession().GetBindEpoch(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromCloseCommand(command *aiv1.CloseAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		BindEpoch:   command.GetSession().GetBindEpoch(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

type noopAISessionRuntimeDriver struct{}

func (noopAISessionRuntimeDriver) Bind(
	context.Context,
	aiSessionBinding,
	aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	return noopAISessionRuntimeHandle{}, nil
}

type noopAISessionRuntimeHandle struct{}

func (noopAISessionRuntimeHandle) SendInput(context.Context, aiSessionInput) error {
	return nil
}

func (noopAISessionRuntimeHandle) AppendContext(context.Context, aiSessionContextUpdate) error {
	return nil
}

func (noopAISessionRuntimeHandle) Cancel(string) {}

func (noopAISessionRuntimeHandle) Close(string) {}

type managedAISessionRuntimeEmitter struct {
	ctx       context.Context
	runtime   *aiSessionRuntime
	publisher *aiSessionEventPublisher
	manager   *aiSessionRuntimeManager
}

func (e *managedAISessionRuntimeEmitter) Emit(eventType string, payloadJSON []byte) {
	e.emitForRef(nil, eventType, payloadJSON)
}

func (e *managedAISessionRuntimeEmitter) EmitForRef(
	ref aiSessionCommandRef,
	eventType string,
	payloadJSON []byte,
) bool {
	return e.emitForRef(&ref, eventType, payloadJSON)
}

func (e *managedAISessionRuntimeEmitter) emitForRef(
	frozenRef *aiSessionCommandRef,
	eventType string,
	payloadJSON []byte,
) bool {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return false
	}
	if !e.runtime.beginEmission() {
		return false
	}
	var ref aiSessionCommandRef
	var seq uint64
	if frozenRef == nil {
		ref, seq = e.runtime.nextEventRefAndSeq()
	} else {
		ref, seq = e.runtime.nextEventRefAndSeqFor(*frozenRef)
	}
	rootTerminal := e.runtime.singleRunExecution() && isYakAIRootPlanExecutionCompleted(payloadJSON)
	claimed := false
	if rootTerminal {
		ref, claimed = e.runtime.claimRootPlanTerminal(ref.CommandID)
	}
	publish := func(ctx context.Context) error {
		if claimed {
			if err := e.runtime.resultSink.Succeed(ctx, payloadJSON); err != nil {
				return err
			}
		}
		return e.publisher.PublishEvent(ctx, ref, seq, eventType, payloadJSON)
	}
	var err error
	if claimed {
		err = retryAISessionTerminalPublish(e.ctx, publish)
	} else {
		err = publish(e.ctx)
	}
	if err != nil {
		logAISessionRuntimePublishError("event", ref.SessionID, err)
		if claimed {
			e.runtime.releaseFailedAutomaticTerminal(ref)
		}
		e.runtime.endEmission()
		return false
	}
	e.runtime.endEmission()
	if claimed && e.manager != nil {
		if err := e.manager.CompleteTerminal(ref, "auto"); err != nil {
			logAISessionRuntimePublishError("complete", ref.SessionID, err)
		}
	}
	return true
}

func (e *managedAISessionRuntimeEmitter) Done(resultJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	if !e.runtime.beginEmission() {
		return
	}
	defer e.runtime.endEmission()
	ref := e.runtime.currentRef()
	if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
		if err := e.runtime.resultSink.Succeed(ctx, resultJSON); err != nil {
			return err
		}
		return e.publisher.PublishDone(ctx, ref, resultJSON)
	}); err != nil {
		logAISessionRuntimePublishError("done", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) DoneTurn(turnID string, resultJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil || e.manager == nil {
		return
	}
	if !e.runtime.beginEmission() {
		return
	}
	ref, claimed := e.runtime.claimAutomaticTerminal(turnID)
	if !claimed {
		e.runtime.endEmission()
		return
	}
	if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
		if err := e.runtime.resultSink.Succeed(ctx, resultJSON); err != nil {
			return err
		}
		return e.publisher.PublishDone(ctx, ref, resultJSON)
	}); err != nil {
		logAISessionRuntimePublishError("done", ref.SessionID, err)
		e.runtime.releaseFailedAutomaticTerminal(ref)
		e.runtime.endEmission()
		return
	}
	e.runtime.endEmission()
	if err := e.manager.CompleteTerminal(ref, "auto"); err != nil {
		logAISessionRuntimePublishError("complete", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) FailTurn(
	turnID string,
	code string,
	message string,
	detailJSON []byte,
) {
	if e == nil || e.runtime == nil || e.publisher == nil || e.manager == nil {
		return
	}
	if !e.runtime.beginEmission() {
		return
	}
	ref, claimed := e.runtime.claimTerminalFailure(turnID)
	if !claimed {
		e.runtime.endEmission()
		return
	}
	if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
		if err := e.runtime.resultSink.Fail(ctx, code, message, detailJSON); err != nil {
			return err
		}
		return e.publisher.PublishFailed(ctx, ref, code, message, detailJSON)
	}); err != nil {
		logAISessionRuntimePublishError("failed", ref.SessionID, err)
		e.runtime.releaseFailedAutomaticTerminal(ref)
		e.runtime.endEmission()
		return
	}
	e.runtime.endEmission()
	if err := e.manager.CompleteTerminal(ref, "auto"); err != nil {
		logAISessionRuntimePublishError("complete", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) TurnCompleted(turnID string, resultJSON []byte) {
	e.publishTurnState(
		turnID,
		aiSessionRuntimeEventTurnCompleted,
		mustJSON(map[string]any{
			"turn_id":     strings.TrimSpace(turnID),
			"status":      "completed",
			"finished_at": time.Now().UTC().Format(time.RFC3339Nano),
			"result":      json.RawMessage(cloneBytes(resultJSON)),
		}),
	)
}

func (e *managedAISessionRuntimeEmitter) TurnFailed(
	turnID string,
	code string,
	message string,
	detailJSON []byte,
) {
	e.publishTurnState(
		turnID,
		aiSessionRuntimeEventTurnFailed,
		mustJSON(map[string]any{
			"turn_id":     strings.TrimSpace(turnID),
			"status":      "failed",
			"finished_at": time.Now().UTC().Format(time.RFC3339Nano),
			"error": map[string]any{
				"code":    strings.TrimSpace(code),
				"message": strings.TrimSpace(message),
				"detail":  json.RawMessage(cloneBytes(detailJSON)),
			},
		}),
	)
}

func (e *managedAISessionRuntimeEmitter) FocusTurnCompleted(turnID string, resultJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	if !e.runtime.beginEmission() {
		return
	}
	err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
		return e.runtime.resultSink.Succeed(ctx, resultJSON)
	})
	e.runtime.endEmission()
	if err != nil {
		detail := mustJSON(map[string]string{"cause": err.Error()})
		if e.runtime.beginEmission() {
			_ = retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
				return e.runtime.resultSink.Fail(ctx, "focus_result_finalize_failed", err.Error(), detail)
			})
			e.runtime.endEmission()
		}
		e.TurnFailed(turnID, "focus_result_finalize_failed", err.Error(), detail)
		return
	}
	e.TurnCompleted(turnID, resultJSON)
}

func (e *managedAISessionRuntimeEmitter) FocusTurnFailed(
	turnID string,
	code string,
	message string,
	detailJSON []byte,
) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	if e.runtime.beginEmission() {
		if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
			return e.runtime.resultSink.Fail(ctx, code, message, detailJSON)
		}); err != nil {
			logAISessionRuntimePublishError("focus-turn-failed", e.runtime.currentRef().SessionID, err)
		}
		e.runtime.endEmission()
	}
	e.TurnFailed(turnID, code, message, detailJSON)
}

func (e *managedAISessionRuntimeEmitter) FocusTurnCancelled(turnID string, reason string) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "user requested cancellation"
	}
	if e.runtime.beginEmission() {
		if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
			return e.runtime.resultSink.Cancel(ctx, reason)
		}); err != nil {
			logAISessionRuntimePublishError("focus-turn-cancelled", e.runtime.currentRef().SessionID, err)
		}
		e.runtime.endEmission()
	}
	e.publishTurnState(
		turnID,
		aiSessionRuntimeEventTurnCancelled,
		mustJSON(map[string]any{
			"turn_id":     strings.TrimSpace(turnID),
			"status":      "cancelled",
			"reason":      reason,
			"finished_at": time.Now().UTC().Format(time.RFC3339Nano),
		}),
	)
}

func (e *managedAISessionRuntimeEmitter) publishTurnState(
	turnID string,
	eventType string,
	payloadJSON []byte,
) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" || !e.runtime.beginEmission() {
		return
	}
	ref := e.runtime.currentRef()
	ref.CommandID = turnID
	ref, seq := e.runtime.nextEventRefAndSeqFor(ref)
	err := retryAISessionTurnPublish(e.ctx, func(ctx context.Context) error {
		return e.publisher.PublishEvent(ctx, ref, seq, eventType, payloadJSON)
	})
	e.runtime.endEmission()
	if err != nil {
		logAISessionRuntimePublishError("turn", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) Failed(code string, message string, detailJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	if !e.runtime.beginEmission() {
		return
	}
	defer e.runtime.endEmission()
	ref := e.runtime.currentRef()
	if err := retryAISessionTerminalPublish(e.ctx, func(ctx context.Context) error {
		if err := e.runtime.resultSink.Fail(ctx, code, message, detailJSON); err != nil {
			return err
		}
		return e.publisher.PublishFailed(ctx, ref, code, message, detailJSON)
	}); err != nil {
		logAISessionRuntimePublishError("failed", ref.SessionID, err)
	}
}

func (r *aiSessionRuntime) beginEmission() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	retired := r.retired
	if !retired {
		r.emissionWG.Add(1)
	}
	r.mu.Unlock()
	if retired {
		return false
	}
	return true
}

func (r *aiSessionRuntime) endEmission() {
	if r != nil {
		r.emissionWG.Done()
	}
}

var aiSessionTerminalPublishTimeout = 30 * time.Second

func retryAISessionTerminalPublish(
	ctx context.Context,
	publish func(context.Context) error,
) error {
	const initialDelay = 100 * time.Millisecond
	const maxDelay = 5 * time.Second

	if ctx == nil {
		ctx = context.Background()
	}
	if aiSessionTerminalPublishTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, aiSessionTerminalPublishTimeout)
		defer cancel()
	}
	var err error
	delay := initialDelay
	for {
		err = publish(ctx)
		if err == nil {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// Turn completion is the durable boundary that makes a reusable Session
// accept the next free input. Unlike a Session terminal command, it must not
// be abandoned after a local timeout: doing so leaves the platform with an
// active Turn while the runtime silently advances. Retry until the runtime is
// explicitly cancelled or retired; the handle keeps the Turn active while
// this call is blocked.
func retryAISessionTurnPublish(
	ctx context.Context,
	publish func(context.Context) error,
) error {
	const initialDelay = 100 * time.Millisecond
	const maxDelay = 5 * time.Second
	if ctx == nil {
		ctx = context.Background()
	}
	delay := initialDelay
	for {
		err := publish(ctx)
		if err == nil {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (r *aiSessionRuntime) releaseFailedAutomaticTerminal(ref aiSessionCommandRef) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalKind != "auto" || r.terminalCommandID != ref.CommandID {
		return
	}
	r.terminalCommandID = ""
	r.terminalKind = ""
	r.terminalReason = ""
	r.terminalPublishFailed = true
}

func (r *aiSessionRuntime) nextEventRefAndSeq() (aiSessionCommandRef, uint64) {
	r.mu.Lock()
	r.seq++
	ref := r.ref
	seq := r.seq
	handle := r.handle
	r.mu.Unlock()

	return stableAISessionRuntimeEventRef(ref, handle), seq
}

func (r *aiSessionRuntime) nextEventRefAndSeqFor(ref aiSessionCommandRef) (aiSessionCommandRef, uint64) {
	r.mu.Lock()
	r.seq++
	current := r.ref
	seq := r.seq
	r.mu.Unlock()

	if strings.TrimSpace(ref.SessionID) == "" {
		ref.SessionID = current.SessionID
	}
	if strings.TrimSpace(ref.RunID) == "" {
		ref.RunID = current.RunID
	}
	if ref.BindEpoch == 0 {
		ref.BindEpoch = current.BindEpoch
	}
	if strings.TrimSpace(ref.OwnerUserID) == "" {
		ref.OwnerUserID = current.OwnerUserID
	}
	return ref, seq
}

func stableAISessionRuntimeEventRef(
	ref aiSessionCommandRef,
	handle aiSessionRuntimeHandle,
) aiSessionCommandRef {
	if provider, ok := handle.(aiSessionRuntimeTurnRefProvider); ok {
		if turnID := strings.TrimSpace(provider.activeTurnID()); turnID != "" {
			ref.CommandID = turnID
		}
	}
	return ref
}

func (r *aiSessionRuntime) currentRef() aiSessionCommandRef {
	r.mu.Lock()
	ref := r.ref
	handle := r.handle
	r.mu.Unlock()
	return stableAISessionRuntimeEventRef(ref, handle)
}

func (r *aiSessionRuntime) claimAutomaticTerminal(turnID string) (aiSessionCommandRef, bool) {
	if r == nil {
		return aiSessionCommandRef{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !strings.EqualFold(strings.TrimSpace(r.executionMode), "single_run") || r.terminalCommandID != "" {
		return aiSessionCommandRef{}, false
	}
	ref := r.ref
	if turnID = strings.TrimSpace(turnID); turnID != "" {
		ref.CommandID = turnID
	}
	if strings.TrimSpace(ref.CommandID) == "" {
		return aiSessionCommandRef{}, false
	}
	r.terminalCommandID = ref.CommandID
	r.terminalKind = "auto"
	r.terminalReason = "single run completed"
	return ref, true
}

func (r *aiSessionRuntime) singleRunExecution() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.EqualFold(strings.TrimSpace(r.executionMode), "single_run")
}

func (r *aiSessionRuntime) claimTerminalFailure(turnID string) (aiSessionCommandRef, bool) {
	if r == nil {
		return aiSessionCommandRef{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalCommandID != "" {
		return aiSessionCommandRef{}, false
	}
	ref := r.ref
	if turnID = strings.TrimSpace(turnID); turnID != "" {
		ref.CommandID = turnID
	}
	if strings.TrimSpace(ref.CommandID) == "" {
		return aiSessionCommandRef{}, false
	}
	r.terminalCommandID = ref.CommandID
	r.terminalKind = "auto"
	r.terminalReason = "runtime failed"
	return ref, true
}

func (r *aiSessionRuntime) claimRootPlanTerminal(turnID string) (aiSessionCommandRef, bool) {
	if r == nil {
		return aiSessionCommandRef{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalCommandID != "" {
		return r.ref, false
	}
	ref := r.ref
	if turnID = strings.TrimSpace(turnID); turnID != "" {
		ref.CommandID = turnID
	}
	if strings.TrimSpace(ref.CommandID) == "" {
		return aiSessionCommandRef{}, false
	}
	r.terminalCommandID = ref.CommandID
	r.terminalKind = "auto"
	r.terminalReason = "root plan execution completed"
	return ref, true
}

func isYakAIRootPlanExecutionCompleted(payload []byte) bool {
	var envelope struct {
		Type        string `json:"type"`
		TaskIndex   string `json:"task_index"`
		ContentJSON struct {
			StartTaskIndex string `json:"start_task_index"`
		} `json:"content_json"`
	}
	if json.Unmarshal(payload, &envelope) != nil {
		return false
	}
	return envelope.Type == "end_plan_and_execution" &&
		strings.TrimSpace(envelope.TaskIndex) == "" &&
		strings.TrimSpace(envelope.ContentJSON.StartTaskIndex) == ""
}
