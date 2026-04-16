package scannode

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/node"
	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	hidsv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/hids/v1"
)

type bootstrapSessionTransport struct {
	session node.SessionState
}

func (s *bootstrapSessionTransport) Bootstrap(context.Context, node.BootstrapRequest) (node.SessionState, error) {
	return s.session, nil
}

func (s *bootstrapSessionTransport) Heartbeat(context.Context, node.SessionState, node.HeartbeatRequest) error {
	return nil
}

func (s *bootstrapSessionTransport) Shutdown(context.Context, node.SessionState, node.ShutdownRequest) error {
	return nil
}

type fakeJetStreamContext struct {
	nats.JetStreamContext

	mu      sync.Mutex
	publish []*nats.Msg
}

func (f *fakeJetStreamContext) PublishMsg(msg *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	cloned := nats.NewMsg(msg.Subject)
	cloned.Header = msg.Header
	cloned.Reply = msg.Reply
	cloned.Data = cloneBytes(msg.Data)
	f.publish = append(f.publish, cloned)

	return &nats.PubAck{
		Stream:   "LEGION_EVENTS",
		Sequence: uint64(len(f.publish)),
	}, nil
}

func (f *fakeJetStreamContext) waitForMessage(t *testing.T) *nats.Msg {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.publish) > 0 {
			msg := f.publish[0]
			f.mu.Unlock()
			return msg
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for published capability event")
	return nil
}

func TestLegionJobBridgePublishesCapabilityAlertEvent(t *testing.T) {
	t.Parallel()

	session := node.SessionState{
		SessionID:          "session-2",
		SessionToken:       "token-2",
		NATSURL:            "nats://session-2.test",
		CommandSubject:     "legion.command.node.node-2",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-2",
		EnrollmentToken:    "enroll-2",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &bootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	t.Cleanup(func() {
		base.Shutdown()
	})
	waitForNodeSession(t, base)

	manager := &CapabilityManager{
		alerts: make(chan CapabilityRuntimeAlert, 1),
	}
	bridge := newLegionJobBridge(&ScanNode{
		node:              base,
		capabilityManager: manager,
	})
	fakeJS := &fakeJetStreamContext{}
	publisher, ok := bridge.capabilityPublisher.(*capabilityEventPublisher)
	if !ok {
		t.Fatalf("unexpected capability publisher type: %T", bridge.capabilityPublisher)
	}
	publisher.js = fakeJS
	publisher.natsURL = session.NATSURL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bridge.forwardCapabilityAlerts(ctx)

	manager.alerts <- CapabilityRuntimeAlert{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		RuleID:        "tmp-observed-file",
		Severity:      "high",
		Title:         "hids rule matched: tmp-observed-file",
		DetailJSON:    []byte(`{"rule_id":"tmp-observed-file","match_event_type":"file.change"}`),
		ObservedAt:    time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
	}

	msg := fakeJS.waitForMessage(t)
	if msg.Subject != "legion.event.capability.alert" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event capabilityv1.CapabilityAlert
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal capability alert: %v", err)
	}
	if event.GetCapability().GetCapabilityKey() != "hids" {
		t.Fatalf("unexpected capability key: %s", event.GetCapability().GetCapabilityKey())
	}
	if event.GetCapability().GetSpecVersion() != "2026-03-28" {
		t.Fatalf("unexpected spec version: %s", event.GetCapability().GetSpecVersion())
	}
	if event.GetSeverity() != "high" {
		t.Fatalf("unexpected severity: %s", event.GetSeverity())
	}
	if event.GetTitle() != "hids rule matched: tmp-observed-file" {
		t.Fatalf("unexpected title: %s", event.GetTitle())
	}
	if string(event.GetDetailJson()) != `{"rule_id":"tmp-observed-file","match_event_type":"file.change"}` {
		t.Fatalf("unexpected detail json: %s", string(event.GetDetailJson()))
	}
	if event.GetMetadata() == nil {
		t.Fatal("expected capability alert metadata")
	}
	if event.GetMetadata().GetEventType() != legionEventCapabilityAlert {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	if event.GetMetadata().GetNode().GetNodeId() != "node-2" {
		t.Fatalf("unexpected node id: %s", event.GetMetadata().GetNode().GetNodeId())
	}
	if event.GetMetadata().GetNode().GetNodeSessionId() != "session-2" {
		t.Fatalf("unexpected node session id: %s", event.GetMetadata().GetNode().GetNodeSessionId())
	}
	if event.GetMetadata().GetCorrelationId() != "node-2:hids" {
		t.Fatalf("unexpected correlation id: %s", event.GetMetadata().GetCorrelationId())
	}
	if event.GetMetadata().GetEventId() == "" {
		t.Fatal("expected non-empty event id")
	}
}

func TestLegionJobBridgePublishesHIDSObservationEvent(t *testing.T) {
	t.Parallel()

	session := node.SessionState{
		SessionID:          "session-1",
		SessionToken:       "token-1",
		NATSURL:            "nats://session-1.test",
		CommandSubject:     "legion.command.node.node-1",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-1",
		EnrollmentToken:    "enroll-1",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &bootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	t.Cleanup(func() {
		base.Shutdown()
	})
	waitForNodeSession(t, base)

	manager := &CapabilityManager{
		observations: make(chan CapabilityRuntimeObservation, 1),
	}
	bridge := newLegionJobBridge(&ScanNode{
		node:              base,
		capabilityManager: manager,
	})
	fakeJS := &fakeJetStreamContext{}
	publisher, ok := bridge.capabilityPublisher.(*capabilityEventPublisher)
	if !ok {
		t.Fatalf("unexpected capability publisher type: %T", bridge.capabilityPublisher)
	}
	publisher.js = fakeJS
	publisher.natsURL = session.NATSURL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bridge.forwardCapabilityObservations(ctx)

	manager.observations <- CapabilityRuntimeObservation{
		CapabilityKey: "hids",
		SpecVersion:   "2026-03-28",
		EventType:     "process.exec",
		EventJSON:     []byte(`{"type":"process.exec","source":"ebpf","process":{"pid":42,"image":"/bin/bash"}}`),
		ObservedAt:    time.Date(2026, 4, 9, 12, 1, 0, 0, time.UTC),
	}

	msg := fakeJS.waitForMessage(t)
	if msg.Subject != "legion.hids.observation" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event hidsv1.HIDSObservation
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal hids observation: %v", err)
	}
	if event.GetCapability().GetCapabilityKey() != "hids" {
		t.Fatalf("unexpected capability key: %s", event.GetCapability().GetCapabilityKey())
	}
	if event.GetHidsEventType() != "process.exec" {
		t.Fatalf("unexpected hids event type: %s", event.GetHidsEventType())
	}
	if string(event.GetEventJson()) != `{"type":"process.exec","source":"ebpf","process":{"pid":42,"image":"/bin/bash"}}` {
		t.Fatalf("unexpected event json: %s", string(event.GetEventJson()))
	}
	if event.GetMetadata() == nil {
		t.Fatal("expected observation metadata")
	}
	if event.GetMetadata().GetEventType() != legionEventHIDSObservation {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	if event.GetMetadata().GetCorrelationId() != "node-1:hids" {
		t.Fatalf("unexpected correlation id: %s", event.GetMetadata().GetCorrelationId())
	}
}

func TestCapabilityEventPublisherPublishesCapabilityStatusDetail(t *testing.T) {
	t.Parallel()

	session := node.SessionState{
		SessionID:          "session-3",
		SessionToken:       "token-3",
		NATSURL:            "nats://session-3.test",
		CommandSubject:     "legion.command.node.node-3",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-3",
		EnrollmentToken:    "enroll-3",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &bootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	t.Cleanup(func() {
		base.Shutdown()
	})
	waitForNodeSession(t, base)

	publisher := newCapabilityEventPublisher(base)
	fakeJS := &fakeJetStreamContext{}
	publisher.js = fakeJS
	publisher.natsURL = session.NATSURL

	err = publisher.PublishStatus(context.Background(), capabilityCommandRef{
		NodeID:        "node-3",
		CapabilityKey: "hids",
		SpecVersion:   "2026-04-10",
	}, CapabilityApplyResult{
		CapabilityKey:    "hids",
		SpecVersion:      "2026-04-10",
		Status:           capabilityStatusRunning,
		Message:          "hids runtime applied with collectors: auditd",
		StatusDetailJSON: []byte(`{"collectors":{"audit":{"status":"running","backend":"auditd"}}}`),
		ObservedAt:       time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("publish status: %v", err)
	}

	msg := fakeJS.waitForMessage(t)
	if msg.Subject != "legion.event.capability.status" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event capabilityv1.CapabilityStatus
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal capability status: %v", err)
	}
	if event.GetStatus() != capabilityStatusRunning {
		t.Fatalf("unexpected status: %s", event.GetStatus())
	}
	if string(event.GetDetailJson()) != `{"collectors":{"audit":{"status":"running","backend":"auditd"}}}` {
		t.Fatalf("unexpected detail json: %s", string(event.GetDetailJson()))
	}
	if event.GetMetadata() == nil || event.GetMetadata().GetEventId() == "" {
		t.Fatal("expected status metadata with event id")
	}
}

func TestCapabilityEventPublisherNormalizesStoppedCapabilityStatus(t *testing.T) {
	t.Parallel()

	session := node.SessionState{
		SessionID:          "session-4",
		SessionToken:       "token-4",
		NATSURL:            "nats://session-4.test",
		CommandSubject:     "legion.command.node.node-4",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-4",
		EnrollmentToken:    "enroll-4",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &bootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	t.Cleanup(func() {
		base.Shutdown()
	})
	waitForNodeSession(t, base)

	publisher := newCapabilityEventPublisher(base)
	fakeJS := &fakeJetStreamContext{}
	publisher.js = fakeJS
	publisher.natsURL = session.NATSURL

	err = publisher.PublishStatus(context.Background(), capabilityCommandRef{
		NodeID:        "node-4",
		CapabilityKey: "hids",
		SpecVersion:   "2026-04-10",
	}, CapabilityApplyResult{
		CapabilityKey:    "hids",
		SpecVersion:      "2026-04-10",
		Status:           "stopped",
		Message:          "hids runtime is stopped",
		StatusDetailJSON: []byte(`{"collectors":{"process":{"status":"stopped"}}}`),
		ObservedAt:       time.Date(2026, 4, 10, 23, 55, 49, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("publish status: %v", err)
	}

	msg := fakeJS.waitForMessage(t)
	var event capabilityv1.CapabilityStatus
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal capability status: %v", err)
	}
	if event.GetStatus() != capabilityStatusStored {
		t.Fatalf("expected stopped to normalize to stored, got %s", event.GetStatus())
	}
	if !json.Valid(event.GetDetailJson()) {
		t.Fatalf("expected normalized detail json to stay valid: %s", string(event.GetDetailJson()))
	}
	if !strings.Contains(string(event.GetDetailJson()), `"reported":"stopped"`) {
		t.Fatalf("expected normalized detail to preserve reported status: %s", string(event.GetDetailJson()))
	}
}

func waitForNodeSession(t *testing.T, base *node.NodeBase) node.SessionState {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, ok := base.GetSessionState()
		if ok {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for node session")
	return node.SessionState{}
}
