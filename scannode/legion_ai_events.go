package scannode

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

type aiSessionCommandRef struct {
	CommandID   string
	SessionID   string
	RunID       string
	OwnerUserID string
}

type aiProviderCommandRef struct {
	CommandID   string
	OwnerUserID string
}

type aiSessionEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newAISessionEventPublisher(base *node.NodeBase) *aiSessionEventPublisher {
	return &aiSessionEventPublisher{node: base}
}

func (p *aiSessionEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *aiSessionEventPublisher) PublishReady(ctx context.Context, ref aiSessionCommandRef) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionReady, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "ready"), &aiv1.AISessionReady{
		Session:        aiSessionProtoRef(ref),
		RuntimeName:    "yak-ai-runtime",
		RuntimeVersion: "nats-bridge-v1",
		ReadyAt:        timestamppb.New(now),
	})
}

func (p *aiSessionEventPublisher) PublishEvent(
	ctx context.Context,
	ref aiSessionCommandRef,
	seq uint64,
	eventType string,
	payloadJSON []byte,
) error {
	if eventType == "" {
		eventType = legionEventAISessionEvent
	}
	suffix := "event"
	if seq > 0 {
		suffix = fmt.Sprintf("event-%d", seq)
	}
	return p.publish(ctx, legionEventAISessionEvent, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, suffix), &aiv1.AISessionEvent{
		Session:     aiSessionProtoRef(ref),
		Seq:         seq,
		EventType:   eventType,
		PayloadJson: cloneBytes(payloadJSON),
	})
}

func (p *aiSessionEventPublisher) PublishDone(
	ctx context.Context,
	ref aiSessionCommandRef,
	resultJSON []byte,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionDone, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "done"), &aiv1.AISessionDone{
		Session:    aiSessionProtoRef(ref),
		FinishedAt: timestamppb.New(now),
		ResultJson: cloneBytes(resultJSON),
	})
}

func (p *aiSessionEventPublisher) PublishFailed(
	ctx context.Context,
	ref aiSessionCommandRef,
	errorCode string,
	errorMessage string,
	errorDetailJSON []byte,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionFailed, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "failed"), &aiv1.AISessionFailed{
		Session:         aiSessionProtoRef(ref),
		FinishedAt:      timestamppb.New(now),
		ErrorCode:       errorCode,
		ErrorMessage:    errorMessage,
		ErrorDetailJson: cloneBytes(errorDetailJSON),
	})
}

func (p *aiSessionEventPublisher) PublishCancelled(
	ctx context.Context,
	ref aiSessionCommandRef,
	reason string,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventAISessionCancelled, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "cancelled"), &aiv1.AISessionCancelled{
		Session:    aiSessionProtoRef(ref),
		FinishedAt: timestamppb.New(now),
		Reason:     reason,
	})
}

func (p *aiSessionEventPublisher) publish(
	ctx context.Context,
	eventType string,
	ref aiSessionCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.SessionID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai session event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai session event %s: %w", eventType, err)
	}
	logPublishedAISessionEvent(eventType, ref.SessionID, message)
	return nil
}

func (p *aiSessionEventPublisher) PublishProviderModelsListed(
	ctx context.Context,
	ref aiProviderCommandRef,
	items []*aiv1.AIProviderPreviewModel,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderModelsListed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "models-listed"),
		&aiv1.AIProviderModelsListed{
			OwnerUserId: strings.TrimSpace(ref.OwnerUserID),
			Items:       items,
		},
	)
}

func (p *aiSessionEventPublisher) PublishProviderModelsFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderModelsFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "models-failed"),
		&aiv1.AIProviderModelsFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) PublishProviderHealthCheckCompleted(
	ctx context.Context,
	ref aiProviderCommandRef,
	result *aiv1.AIProviderHealthCheckCompleted,
) error {
	if result == nil {
		result = &aiv1.AIProviderHealthCheckCompleted{}
	}
	result.OwnerUserId = strings.TrimSpace(ref.OwnerUserID)
	return p.publishProvider(
		ctx,
		legionEventAIProviderHealthCheckCompleted,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "health-check-completed"),
		result,
	)
}

func (p *aiSessionEventPublisher) PublishProviderHealthCheckFailed(
	ctx context.Context,
	ref aiProviderCommandRef,
	errorCode string,
	errorMessage string,
) error {
	return p.publishProvider(
		ctx,
		legionEventAIProviderHealthCheckFailed,
		ref,
		providerEventIDWithSuffix(ref.CommandID, ref.OwnerUserID, "health-check-failed"),
		&aiv1.AIProviderHealthCheckFailed{
			OwnerUserId:  strings.TrimSpace(ref.OwnerUserID),
			ErrorCode:    strings.TrimSpace(errorCode),
			ErrorMessage: strings.TrimSpace(errorMessage),
		},
	)
}

func (p *aiSessionEventPublisher) publishProvider(
	ctx context.Context,
	eventType string,
	ref aiProviderCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.OwnerUserID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachAIEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal ai provider event: %w", err)
	}
	msg := nats.NewMsg(jobEventSubject(session.EventSubjectPrefix, eventType))
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish ai provider event %s: %w", eventType, err)
	}
	return nil
}

func (p *aiSessionEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	p.closeLocked()

	conn, err := nats.Connect(natsURL, nats.Name("yak-node-ai-events-"+p.node.CurrentNodeID()))
	if err != nil {
		return fmt.Errorf("connect ai event nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build ai event jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func (p *aiSessionEventPublisher) closeLocked() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func attachAIEventMetadata(message proto.Message, metadata *nodev1.EventMetadata) error {
	switch value := message.(type) {
	case *aiv1.AISessionReady:
		value.Metadata = metadata
	case *aiv1.AISessionEvent:
		value.Metadata = metadata
	case *aiv1.AISessionDone:
		value.Metadata = metadata
	case *aiv1.AISessionFailed:
		value.Metadata = metadata
	case *aiv1.AISessionCancelled:
		value.Metadata = metadata
	case *aiv1.AIProviderModelsListed:
		value.Metadata = metadata
	case *aiv1.AIProviderModelsFailed:
		value.Metadata = metadata
	case *aiv1.AIProviderHealthCheckCompleted:
		value.Metadata = metadata
	case *aiv1.AIProviderHealthCheckFailed:
		value.Metadata = metadata
	default:
		return fmt.Errorf("unsupported ai event message: %T", message)
	}
	return nil
}

func aiSessionProtoRef(ref aiSessionCommandRef) *aiv1.AISessionRef {
	return &aiv1.AISessionRef{
		SessionId: ref.SessionID,
		RunId:     ref.RunID,
	}
}

func eventIDWithSuffix(commandID string, sessionID string, suffix string) string {
	if commandID != "" {
		return commandID + ":" + suffix
	}
	if sessionID != "" {
		return sessionID + ":" + suffix + ":" + uuid.NewString()
	}
	return uuid.NewString()
}

func providerEventIDWithSuffix(commandID string, ownerUserID string, suffix string) string {
	base := strings.TrimSpace(ownerUserID)
	if base == "" {
		base = "provider"
	}
	return eventIDWithSuffix(commandID, base, suffix)
}

func logPublishedAISessionEvent(eventType string, sessionID string, message proto.Message) {
	runtimeType, seq := extractAISessionRuntimeLogFields(message)
	if shouldDebugAISessionRuntimeEvent(runtimeType) {
		log.Debugf(
			"published legion ai session event: type=%s runtime_type=%s seq=%d session_id=%s",
			eventType,
			runtimeType,
			seq,
			sessionID,
		)
		return
	}
	if runtimeType != "" {
		log.Infof(
			"published legion ai session event: type=%s runtime_type=%s seq=%d session_id=%s",
			eventType,
			runtimeType,
			seq,
			sessionID,
		)
		return
	}
	log.Infof("published legion ai session event: type=%s session_id=%s", eventType, sessionID)
}

func extractAISessionRuntimeLogFields(message proto.Message) (string, uint64) {
	event, ok := message.(*aiv1.AISessionEvent)
	if !ok {
		return "", 0
	}
	return strings.TrimSpace(event.GetEventType()), event.GetSeq()
}

func shouldDebugAISessionRuntimeEvent(runtimeType string) bool {
	switch strings.TrimSpace(runtimeType) {
	case aiSessionRuntimeEventDelta,
		aiSessionRuntimeEventThought,
		aiSessionRuntimeEventMessage,
		aiSessionRuntimeEventToolCall,
		aiSessionRuntimeEventToolResult,
		"consumption":
		return true
	default:
		return false
	}
}
