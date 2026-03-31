package scannode

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	capabilityv1 "github.com/yaklang/yaklang/common/legionpb/legion/capability/v1"
	nodev1 "github.com/yaklang/yaklang/common/legionpb/legion/node/v1"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
)

type capabilityEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newCapabilityEventPublisher(base *node.NodeBase) *capabilityEventPublisher {
	return &capabilityEventPublisher{node: base}
}

func (p *capabilityEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *capabilityEventPublisher) PublishStatus(
	ctx context.Context,
	ref capabilityCommandRef,
	result CapabilityApplyResult,
) error {
	eventID := ref.CommandID + ":status"
	if ref.CommandID == "" {
		eventID = result.CapabilityKey + ":status"
	}
	return p.publish(ctx, legionEventCapabilityStatus, ref, eventID, &capabilityv1.CapabilityStatus{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: result.CapabilityKey,
			SpecVersion:   result.SpecVersion,
		},
		Status:     result.Status,
		Message:    result.Message,
		ObservedAt: timestamppb.New(result.ObservedAt),
	})
}

func (p *capabilityEventPublisher) PublishFailed(
	ctx context.Context,
	ref capabilityCommandRef,
	errorCode string,
	errorMessage string,
) error {
	eventID := ref.CommandID + ":failed"
	if ref.CommandID == "" {
		eventID = ref.CapabilityKey + ":failed"
	}
	return p.publish(ctx, legionEventCapabilityFailed, ref, eventID, &capabilityv1.CapabilityFailed{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: ref.CapabilityKey,
			SpecVersion:   ref.SpecVersion,
		},
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	})
}

func (p *capabilityEventPublisher) publish(
	ctx context.Context,
	eventType string,
	ref capabilityCommandRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return fmt.Errorf("node session is not ready")
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: capabilityCorrelationID(ref.NodeID, ref.CapabilityKey),
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.NodeId,
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachCapabilityEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal capability event: %w", err)
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
		return fmt.Errorf("publish capability event %s: %w", eventType, err)
	}
	log.Infof("published legion capability event: type=%s capability=%s", eventType, ref.CapabilityKey)
	return nil
}

func (p *capabilityEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	p.closeLocked()

	conn, err := nats.Connect(natsURL, nats.Name("yak-node-capability-events-"+p.node.NodeId))
	if err != nil {
		return fmt.Errorf("connect capability event nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build capability event jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func (p *capabilityEventPublisher) closeLocked() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func attachCapabilityEventMetadata(
	message proto.Message,
	metadata *nodev1.EventMetadata,
) error {
	switch value := message.(type) {
	case *capabilityv1.CapabilityStatus:
		value.Metadata = metadata
	case *capabilityv1.CapabilityAlert:
		value.Metadata = metadata
	case *capabilityv1.CapabilityFailed:
		value.Metadata = metadata
	default:
		return fmt.Errorf("unsupported capability event message: %T", message)
	}
	return nil
}

func capabilityCorrelationID(nodeID string, capabilityKey string) string {
	if nodeID == "" {
		return capabilityKey
	}
	if capabilityKey == "" {
		return nodeID
	}
	return nodeID + ":" + capabilityKey
}
