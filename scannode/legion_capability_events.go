package scannode

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

var ErrNodeSessionNotReady = errors.New("node session is not ready")

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
	result = normalizeCapabilityApplyResult(result)
	eventID := ref.CommandID + ":status"
	if ref.CommandID == "" {
		eventID = capabilityStatusEventID(result)
	}
	return p.publish(ctx, legionEventCapabilityStatus, ref, eventID, &capabilityv1.CapabilityStatus{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: result.CapabilityKey,
			SpecVersion:   result.SpecVersion,
		},
		Status:     result.Status,
		Message:    result.Message,
		DetailJson: cloneBytes(result.StatusDetailJSON),
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

func (p *capabilityEventPublisher) PublishAlert(
	ctx context.Context,
	alert CapabilityRuntimeAlert,
) error {
	ref := capabilityCommandRef{
		NodeID:        p.node.CurrentNodeID(),
		CapabilityKey: alert.CapabilityKey,
		SpecVersion:   alert.SpecVersion,
	}
	return p.publish(ctx, legionEventCapabilityAlert, ref, capabilityAlertEventID(alert), &capabilityv1.CapabilityAlert{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: alert.CapabilityKey,
			SpecVersion:   alert.SpecVersion,
		},
		Severity:   alert.Severity,
		Title:      alert.Title,
		DetailJson: cloneBytes(alert.DetailJSON),
	})
}

func (p *capabilityEventPublisher) PublishObservation(
	ctx context.Context,
	observation CapabilityRuntimeObservation,
) error {
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	ref := capabilityCommandRef{
		NodeID:        p.node.CurrentNodeID(),
		CapabilityKey: observation.CapabilityKey,
		SpecVersion:   observation.SpecVersion,
	}
	return p.publish(ctx, legionEventHIDSObservation, ref, capabilityObservationEventID(observation, observedAt), &hidsv1.HIDSObservation{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: observation.CapabilityKey,
			SpecVersion:   observation.SpecVersion,
		},
		HidsEventType: observation.HIDSEventType,
		ObservedAt:    timestamppb.New(observedAt),
		EventJson:     cloneBytes(observation.EventJSON),
	})
}

func (p *capabilityEventPublisher) PublishResponseActionResult(
	ctx context.Context,
	input HIDSResponseActionResultInput,
) error {
	observedAt := input.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	ref := capabilityCommandRef{
		CommandID:     input.CommandID,
		NodeID:        p.node.CurrentNodeID(),
		CapabilityKey: stringOrDefault(input.CapabilityKey, "hids"),
		SpecVersion:   normalizeCapabilitySpecVersion(input.SpecVersion),
	}
	eventID := input.CommandID + ":response-action:" + strings.TrimSpace(input.Status)
	if strings.TrimSpace(input.CommandID) == "" {
		eventID = capabilityResponseActionEventID(input, observedAt)
	}
	return p.publish(ctx, legionEventHIDSResponseActionResult, ref, eventID, &hidsv1.HIDSResponseActionResult{
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: ref.CapabilityKey,
			SpecVersion:   ref.SpecVersion,
		},
		Action:       input.ActionType,
		Status:       input.Status,
		ErrorCode:    input.ErrorCode,
		ErrorMessage: input.ErrorMessage,
		ObservedAt:   timestamppb.New(observedAt),
		DetailJson:   cloneBytes(input.DetailJSON),
		Process: &hidsv1.HIDSProcessRef{
			Pid:             int32(input.Process.PID),
			BootId:          input.Process.BootID,
			StartTimeUnixMs: input.Process.StartTimeUnixMillis,
			ProcessName:     input.Process.ProcessName,
			ProcessImage:    input.Process.ProcessImage,
			ProcessCommand:  input.Process.ProcessCommand,
			Username:        input.Process.Username,
		},
	})
}

func (p *capabilityEventPublisher) PublishDesiredSpecDryRunResult(
	ctx context.Context,
	ref capabilityCommandRef,
	result CapabilityDryRunResult,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return ErrNodeSessionNotReady
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	observedAt := result.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	eventID := ref.CommandID + ":desired-spec-dry-run"
	if strings.TrimSpace(ref.CommandID) == "" {
		eventID = capabilityDryRunEventID(result, observedAt)
	}

	message := &hidsv1.HIDSDesiredSpecDryRunResult{
		Metadata: &nodev1.EventMetadata{
			EventId:       eventID,
			EventType:     legionEventHIDSDesiredSpecDryRunResult,
			CausationId:   ref.CommandID,
			CorrelationId: capabilityCorrelationID(ref.NodeID, ref.CapabilityKey),
			EmittedAt:     timestamppb.New(time.Now().UTC()),
			Node: &nodev1.NodeRef{
				NodeId:        p.node.CurrentNodeID(),
				NodeSessionId: session.SessionID,
			},
		},
		Capability: &capabilityv1.CapabilityRef{
			CapabilityKey: result.CapabilityKey,
			SpecVersion:   result.SpecVersion,
		},
		Status:       result.Status,
		Message:      result.Message,
		ErrorCode:    result.ErrorCode,
		ErrorMessage: result.ErrorMessage,
		ObservedAt:   timestamppb.New(observedAt),
		DetailJson:   cloneBytes(result.DetailJSON),
	}
	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal hids desired spec dry-run result: %w", err)
	}

	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("capability event nats connection is not ready")
	}
	subject := hidsDesiredSpecDryRunResultSubject(ref.CommandID)
	if err := conn.Publish(subject, raw); err != nil {
		return fmt.Errorf("publish hids desired spec dry-run result: %w", err)
	}
	flushCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := conn.FlushWithContext(flushCtx); err != nil {
		return fmt.Errorf("flush hids desired spec dry-run result: %w", err)
	}
	log.Infof("published hids desired spec dry-run result: capability=%s status=%s", ref.CapabilityKey, result.Status)
	return nil
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
		return ErrNodeSessionNotReady
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
			NodeId:        p.node.CurrentNodeID(),
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
	msg := nats.NewMsg(capabilityEventSubject(session.EventSubjectPrefix, eventType))
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
	if eventType == "hids.observation" {
		log.Debugf("published legion capability event: type=%s capability=%s", eventType, ref.CapabilityKey)
		return nil
	}
	if eventType == legionEventCapabilityStatus && ref.CommandID == "" {
		log.Debugf("published legion capability event: type=%s capability=%s", eventType, ref.CapabilityKey)
		return nil
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

	conn, err := nats.Connect(natsURL, nats.Name("yak-node-capability-events-"+p.node.CurrentNodeID()))
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
	case *hidsv1.HIDSObservation:
		value.Metadata = metadata
	case *hidsv1.HIDSResponseActionResult:
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

func capabilityAlertEventID(alert CapabilityRuntimeAlert) string {
	observedAt := alert.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	payload := append(cloneBytes(alert.DetailJSON), []byte("\x00"+alert.RuleID+"\x00"+alert.Title)...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf("%s:alert:%d:%x", alert.CapabilityKey, observedAt.UnixNano(), sum[:6])
}

func capabilityObservationEventID(observation CapabilityRuntimeObservation, observedAt time.Time) string {
	payload := append(cloneBytes(observation.EventJSON), []byte("\x00"+observation.HIDSEventType)...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf(
		"%s:observation:%s:%d:%x",
		observation.CapabilityKey,
		observation.HIDSEventType,
		observedAt.UnixNano(),
		sum[:6],
	)
}

func capabilityStatusEventID(result CapabilityApplyResult) string {
	observedAt := result.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	payload := append(cloneBytes(result.StatusDetailJSON), []byte("\x00"+result.Status+"\x00"+result.Message)...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf("%s:status:%d:%x", result.CapabilityKey, observedAt.UnixNano(), sum[:6])
}

func capabilityDryRunEventID(result CapabilityDryRunResult, observedAt time.Time) string {
	payload := append(cloneBytes(result.DetailJSON), []byte("\x00"+result.Status+"\x00"+result.ErrorCode+"\x00"+result.ErrorMessage)...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf(
		"%s:desired-spec-dry-run:%d:%x",
		result.CapabilityKey,
		observedAt.UnixNano(),
		sum[:6],
	)
}

func capabilityResponseActionEventID(input HIDSResponseActionResultInput, observedAt time.Time) string {
	payload := append(cloneBytes(input.DetailJSON), []byte("\x00"+input.ActionType+"\x00"+input.Status)...)
	sum := sha1.Sum(payload)
	return fmt.Sprintf(
		"%s:response-action:%d:%x",
		stringOrDefault(input.CapabilityKey, "hids"),
		observedAt.UnixNano(),
		sum[:6],
	)
}
