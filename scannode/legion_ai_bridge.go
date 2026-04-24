package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const aiSessionRuntimeEventInput = "ai.session.input"

type aiSessionRuntimeDriver interface {
	Bind(context.Context, aiSessionBinding, aiSessionRuntimeEmitter) (aiSessionRuntimeHandle, error)
}

type aiSessionRuntimeHandle interface {
	SendInput(context.Context, aiSessionInput) error
	Cancel(string)
	Close(string)
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
}

type aiSessionInput struct {
	Ref         aiSessionCommandRef
	InputType   string
	PayloadJSON []byte
}

type acceptedAISessionInput struct {
	ref         aiSessionCommandRef
	seq         uint64
	inputType   string
	payloadJSON []byte
	handle      aiSessionRuntimeHandle
}

type cancelledAISessionRuntime struct {
	ref    aiSessionCommandRef
	reason string
	handle aiSessionRuntimeHandle
}

type closedAISessionRuntime struct {
	ref    aiSessionCommandRef
	reason string
	handle aiSessionRuntimeHandle
}

type aiSessionRuntimeManager struct {
	mu       sync.Mutex
	sessions map[string]*aiSessionRuntime
	driver   aiSessionRuntimeDriver
}

type aiSessionRuntime struct {
	mu        sync.Mutex
	ref       aiSessionCommandRef
	projectID string
	title     string
	seq       uint64
	cancel    context.CancelFunc
	handle    aiSessionRuntimeHandle
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
		return ref, nil
	}

	ctx, cancel := context.WithCancel(parent)
	runtime := &aiSessionRuntime{
		ref:       ref,
		projectID: strings.TrimSpace(command.GetProjectId()),
		title:     strings.TrimSpace(command.GetTitle()),
		cancel:    cancel,
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
		ref:         ref,
		seq:         seq,
		inputType:   inputType,
		payloadJSON: payload,
		handle:      handle,
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
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()
	delete(m.sessions, ref.SessionID)
	return cancelledAISessionRuntime{ref: ref, reason: reason, handle: handle}, nil
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
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()
	delete(m.sessions, ref.SessionID)
	return closedAISessionRuntime{ref: ref, reason: reason, handle: handle}, nil
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
	ref, err := b.ensureAIRuntime().Bind(
		b.agent.node.GetRootContext(),
		&command,
		b.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{
			PlatformBearerToken: session.SessionToken,
			HTTPClient:          b.agent.httpClient,
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

	accepted, err := b.ensureAIRuntime().AcceptInput(&command)
	if err != nil {
		return b.publishAISessionCommandFailure(ctx, ref, "ai_session_input_failed", err)
	}
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
		return nil
	}
	if err := accepted.handle.SendInput(ctx, aiSessionInput{
		Ref:         accepted.ref,
		InputType:   accepted.inputType,
		PayloadJSON: accepted.payloadJSON,
	}); err != nil {
		return b.publishAISessionCommandFailure(ctx, accepted.ref, "ai_session_runtime_input_failed", err)
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
	if cancelled.handle != nil {
		cancelled.handle.Cancel(cancelled.reason)
	}
	return b.ensureAIPublisher().PublishCancelled(ctx, cancelled.ref, cancelled.reason)
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
	if closed.handle != nil {
		closed.handle.Close(closed.reason)
	}
	return b.ensureAIPublisher().PublishDone(ctx, closed.ref, mustJSON(map[string]string{
		"reason":    closed.reason,
		"closed_by": "platform",
	}))
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
		b.aiRuntime = newAISessionRuntimeManager(newYakAIEngineRuntimeDriver())
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
	default:
		return nil
	}
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
	default:
		_, err := normalizeAISessionInputPayload(command.GetInputType(), command.GetInputJson())
		return err
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
		return json.Marshal(map[string]string{
			"input_type": normalizedInputType,
			"role":       "user",
		})
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("ai session input_json must be valid json: %w", err)
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
	if err := e.publisher.PublishDone(e.ctx, ref, resultJSON); err != nil {
		logAISessionRuntimePublishError("done", ref.SessionID, err)
	}
}

func (e *managedAISessionRuntimeEmitter) Failed(code string, message string, detailJSON []byte) {
	if e == nil || e.runtime == nil || e.publisher == nil {
		return
	}
	ref := e.runtime.currentRef()
	if err := e.publisher.PublishFailed(e.ctx, ref, code, message, detailJSON); err != nil {
		logAISessionRuntimePublishError("failed", ref.SessionID, err)
	}
}

func (r *aiSessionRuntime) nextEventRefAndSeq() (aiSessionCommandRef, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	return r.ref, r.seq
}

func (r *aiSessionRuntime) currentRef() aiSessionCommandRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ref
}
