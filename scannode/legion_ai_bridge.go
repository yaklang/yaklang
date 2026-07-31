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
	ResultSink                 aicommon.ResultSink
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
	HTTPClient          *http.Client
	ResultSink          aicommon.ResultSink
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
	ref         aiSessionCommandRef
	reason      string
	handle      aiSessionRuntimeHandle
	resultSink  *aiSessionResultSinkProxy
	applyHandle bool
}

type aiSessionRuntimeManager struct {
	mu       sync.Mutex
	sessions map[string]*aiSessionRuntime
	driver   aiSessionRuntimeDriver
}

type aiSessionRuntime struct {
	mu                     sync.Mutex
	ref                    aiSessionCommandRef
	projectID              string
	title                  string
	seq                    uint64
	cancel                 context.CancelFunc
	handle                 aiSessionRuntimeHandle
	resultSink             *aiSessionResultSinkProxy
	processedInputCommands map[string]struct{}
	processedInputOrder    []string
	inFlightInputCommands  map[string]struct{}
	terminalCommandID      string
	terminalKind           string
	terminalReason         string
}

func newAISessionRuntimeManager(driver aiSessionRuntimeDriver) *aiSessionRuntimeManager {
	if driver == nil {
		driver = noopAISessionRuntimeDriver{}
	}
	return &aiSessionRuntimeManager{
		sessions: make(map[string]*aiSessionRuntime),
		driver:   driver,
	}
}

func (m *aiSessionRuntimeManager) Bind(
	parent context.Context,
	command *aiv1.BindAISessionCommand,
	publisher *aiSessionEventPublisher,
	options aiSessionRuntimeBindOptions,
) (aiSessionCommandRef, error) {
	ref := aiSessionRefFromBindCommand(command)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[ref.SessionID]; ok {
		if existing.ref.OwnerUserID != ref.OwnerUserID {
			return ref, fmt.Errorf("ai session owner mismatch: %s", existing.ref.OwnerUserID)
		}
		existing.ref = ref
		existing.projectID = strings.TrimSpace(command.GetProjectId())
		existing.title = strings.TrimSpace(command.GetTitle())
		existing.resultSink.Set(options.ResultSink)
		return ref, nil
	}

	ctx, cancel := context.WithCancel(parent)
	resultSink := newAISessionResultSinkProxy(options.ResultSink)
	runtime := &aiSessionRuntime{
		ref:                    ref,
		projectID:              strings.TrimSpace(command.GetProjectId()),
		title:                  strings.TrimSpace(command.GetTitle()),
		cancel:                 cancel,
		resultSink:             resultSink,
		processedInputCommands: make(map[string]struct{}),
		inFlightInputCommands:  make(map[string]struct{}),
	}
	runtime.handle = noopAISessionRuntimeHandle{}
	handle, err := m.driver.Bind(ctx, aiSessionBinding{
		Ref:                        ref,
		ProjectID:                  runtime.projectID,
		Title:                      runtime.title,
		ProviderPolicySnapshotJSON: cloneBytes(command.GetProviderPolicySnapshotJson()),
		RuntimeOptionSnapshotJSON:  cloneBytes(command.GetRuntimeOptionSnapshotJson()),
		Attachments:                cloneAISessionAttachmentRefs(command.GetAttachments()),
		CredentialRefs:             cloneAISessionCredentialRefs(command.GetCredentialRefs()),
		PlatformBearerToken:        strings.TrimSpace(options.PlatformBearerToken),
		HTTPClient:                 options.HTTPClient,
		ResultSink:                 resultSink,
	}, &managedAISessionRuntimeEmitter{
		ctx:       parent,
		runtime:   runtime,
		publisher: publisher,
	})
	if err != nil {
		cancel()
		return ref, err
	}
	if handle != nil {
		runtime.handle = handle
	}
	m.sessions[ref.SessionID] = runtime
	return ref, nil
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
	if _, ok := session.processedInputCommands[ref.CommandID]; ok {
		ref.RunID = session.ref.RunID
		session.mu.Unlock()
		return acceptedAISessionInput{ref: ref, duplicate: true}, nil
	}
	if _, ok := session.inFlightInputCommands[ref.CommandID]; ok {
		session.mu.Unlock()
		return acceptedAISessionInput{ref: ref}, errAISessionInputInFlight
	}
	session.inFlightInputCommands[ref.CommandID] = struct{}{}
	session.seq++
	ref.RunID = session.ref.RunID
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

func (m *aiSessionRuntimeManager) CompleteInput(sessionID, commandID string, succeeded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	commandID = strings.TrimSpace(commandID)
	delete(session.inFlightInputCommands, commandID)
	if !succeeded || commandID == "" {
		return
	}
	if _, exists := session.processedInputCommands[commandID]; exists {
		return
	}
	session.processedInputCommands[commandID] = struct{}{}
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
	session.seq++
	ref.RunID = session.ref.RunID
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
	ref.RunID = session.ref.RunID
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
		return closedAISessionRuntime{ref: ref, reason: reason}, nil
	}
	if session.ref.OwnerUserID != ref.OwnerUserID {
		return closedAISessionRuntime{ref: ref, reason: reason}, fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	}
	session.mu.Lock()
	ref.RunID = session.ref.RunID
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
	defer m.mu.Unlock()

	session, ok := m.sessions[ref.SessionID]
	if !ok {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	switch {
	case session.ref.OwnerUserID != ref.OwnerUserID:
		return fmt.Errorf("ai session owner mismatch: %s", session.ref.OwnerUserID)
	case session.terminalCommandID != ref.CommandID:
		return fmt.Errorf(
			"ai session terminal command mismatch: got %s want %s",
			ref.CommandID,
			session.terminalCommandID,
		)
	case session.terminalKind != kind:
		return fmt.Errorf(
			"ai session terminal kind mismatch: got %s want %s",
			kind,
			session.terminalKind,
		)
	}
	delete(m.sessions, ref.SessionID)
	return nil
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
			HTTPClient:          b.agent.httpClient,
			ResultSink:          resultSink,
		},
	)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_bind_failed", err)
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
		return nil
	}
	succeeded := false
	defer func() {
		runtime.CompleteInput(accepted.ref.SessionID, accepted.ref.CommandID, succeeded)
	}()
	if err := b.ensureAIPublisher().PublishEvent(
		ctx,
		accepted.ref,
		accepted.seq,
		aiSessionRuntimeEventInput,
		accepted.payloadJSON,
	); err != nil {
		return err
	}
	if accepted.handle == nil {
		succeeded = true
		return nil
	}
	if err := accepted.handle.SendInput(ctx, aiSessionInput{
		Ref:            accepted.ref,
		InputType:      accepted.inputType,
		PayloadJSON:    accepted.payloadJSON,
		ContextPackage: accepted.contextPackage,
		ReviewID:       accepted.reviewID,
		TurnID:         accepted.turnID,
	}); err != nil {
		failureErr := b.publishAISessionCommandFailure(ctx, accepted.ref, "ai_session_runtime_input_failed", err)
		succeeded = failureErr == nil
		return failureErr
	}
	succeeded = true
	return nil
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
	if err := b.ensureAIPublisher().PublishDone(ctx, closed.ref, resultJSON); err != nil {
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
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromInputCommand(command *aiv1.PushAISessionInputCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromContextCommand(command *aiv1.AppendAISessionContextCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromCancelCommand(command *aiv1.CancelAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiSessionRefFromCloseCommand(command *aiv1.CloseAISessionCommand) aiSessionCommandRef {
	return aiSessionCommandRef{
		CommandID:   command.GetMetadata().GetCommandId(),
		SessionID:   command.GetSession().GetSessionId(),
		RunID:       command.GetSession().GetRunId(),
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
}

func (e *managedAISessionRuntimeEmitter) Emit(eventType string, payloadJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	ref, seq := e.runtime.nextEventRefAndSeq()
	if err := e.publisher.PublishEvent(e.ctx, ref, seq, eventType, payloadJSON); err != nil {
		logAISessionRuntimePublishError("event", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) Done(resultJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
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

func (e *managedAISessionRuntimeEmitter) Failed(code string, message string, detailJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
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

func retryAISessionTerminalPublish(
	ctx context.Context,
	publish func(context.Context) error,
) error {
	const initialDelay = 100 * time.Millisecond
	const maxDelay = 5 * time.Second

	if ctx == nil {
		ctx = context.Background()
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

func (r *aiSessionRuntime) nextEventRefAndSeq() (aiSessionCommandRef, uint64) {
	r.mu.Lock()
	r.seq++
	ref := r.ref
	seq := r.seq
	handle := r.handle
	r.mu.Unlock()

	return stableAISessionRuntimeEventRef(ref, handle), seq
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
