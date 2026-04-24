package scannode

import (
	"context"
	"fmt"
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
	return p.publish(ctx, legionEventAISessionEvent, ref, eventIDWithSuffix(ref.CommandID, ref.SessionID, "event"), &aiv1.AISessionEvent{
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
	if err := attachAISessionEventMetadata(message, metadata); err != nil {
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
	log.Infof("published legion ai session event: type=%s session_id=%s", eventType, ref.SessionID)
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

func attachAISessionEventMetadata(message proto.Message, metadata *nodev1.EventMetadata) error {
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
	default:
		return fmt.Errorf("unsupported ai session event message: %T", message)
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
