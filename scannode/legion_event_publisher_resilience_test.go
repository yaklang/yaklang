package scannode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	capabilityv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/capability/v1"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/proto"
)

// Keep deterministic client-state tests independent of sockets and brokers.
type unavailableEventTestDialer struct{}

func (unavailableEventTestDialer) Dial(string, string) (net.Conn, error) {
	return nil, errors.New("event test broker unavailable")
}

func newReconnectingEventTestConn(t *testing.T) *nats.Conn {
	t.Helper()
	conn, err := nats.Connect(nats.DefaultURL,
		nats.SetCustomDialer(unavailableEventTestDialer{}),
		nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Hour), nats.ReconnectJitter(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	if !conn.IsReconnecting() {
		t.Fatalf("expected RECONNECTING, got %s", conn.Status())
	}
	return conn
}

type recoveryEvent interface {
	proto.Message
	GetMetadata() *nodev1.EventMetadata
}

type eventPublisherRecoveryCase struct {
	name      string
	conn      **nats.Conn
	js        *nats.JetStreamContext
	url       *string
	ensure    func(string) error
	close     func()
	publish   func(string) error
	eventType string
	newEvent  func() recoveryEvent
}

func eventPublisherRecoveryCases(base *node.NodeBase) []eventPublisherRecoveryCase {
	capability := newCapabilityEventPublisher(base)
	job := newJobEventPublisher(base)
	ai := newAISessionEventPublisher(base)
	rules := newSSARuleSyncEventPublisher(base)
	return []eventPublisherRecoveryCase{
		{
			name: "capability", conn: &capability.conn, js: &capability.js, url: &capability.natsURL,
			ensure: capability.ensureJetStream, close: capability.Close,
			publish: func(id string) error {
				return capability.PublishStatus(context.Background(), capabilityCommandRef{
					CommandID: id, NodeID: base.CurrentNodeID(), CapabilityKey: "ai.runtime.host.v1", SpecVersion: "v1",
				}, CapabilityApplyResult{
					CapabilityKey: "ai.runtime.host.v1", SpecVersion: "v1", Status: capabilityStatusRunning, ObservedAt: time.Now().UTC(),
				})
			},
			eventType: legionEventCapabilityStatus, newEvent: func() recoveryEvent { return &capabilityv1.CapabilityStatus{} },
		},
		{
			name: "job", conn: &job.conn, js: &job.js, url: &job.natsURL,
			ensure: job.ensureJetStream, close: job.Close,
			publish: func(id string) error {
				return job.PublishClaimed(context.Background(), jobExecutionRef{
					CommandID: id, JobID: "job-recovery", SubtaskID: "subtask-recovery", AttemptID: id,
				})
			},
			eventType: legionEventClaimed, newEvent: func() recoveryEvent { return &jobv1.JobClaimed{} },
		},
		{
			name: "ai", conn: &ai.conn, js: &ai.js, url: &ai.natsURL,
			ensure: ai.ensureJetStream, close: ai.Close,
			publish: func(id string) error {
				return ai.PublishReady(context.Background(), aiSessionCommandRef{
					CommandID: id, SessionID: "ai-session-recovery", RunID: "run-recovery", BindEpoch: 1,
				})
			},
			eventType: legionEventAISessionReady, newEvent: func() recoveryEvent { return &aiv1.AISessionReady{} },
		},
		{
			name: "ssa_rules", conn: &rules.conn, js: &rules.js, url: &rules.natsURL,
			ensure: rules.ensureJetStream, close: rules.Close,
			publish: func(id string) error {
				return rules.PublishFailed(context.Background(), ssaRuleSyncCommandRef{
					CommandID: id, NodeID: base.CurrentNodeID(),
				}, "test_failure", "recovery regression")
			},
			eventType: legionEventSSARuleSyncFailed, newEvent: func() recoveryEvent { return &ssav1.RuleCatalogExportFailed{} },
		},
	}
}

func TestResilienceEventPublishersRejectClosedConnection(t *testing.T) {
	for _, tc := range eventPublisherRecoveryCases(&node.NodeBase{}) {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(tc.close)
			conn := newReconnectingEventTestConn(t)
			conn.Close()
			*tc.conn, *tc.js, *tc.url = conn, &fakeJetStreamContext{}, "nats://[invalid"
			// A cached context must not bypass a new connection attempt. The
			// invalid URL makes that attempt fail immediately without network I/O.
			for attempt := 0; attempt < 2; attempt++ {
				if err := tc.ensure("nats://[invalid"); err == nil {
					t.Fatal("closed connection was reused instead of attempting recovery")
				}
			}
		})
	}
}

func TestResilienceEventPublishersPreserveReconnectingConnection(t *testing.T) {
	for _, tc := range eventPublisherRecoveryCases(&node.NodeBase{}) {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(tc.close)
			conn := newReconnectingEventTestConn(t)
			js := &fakeJetStreamContext{}
			*tc.conn, *tc.js, *tc.url = conn, js, "nats://[invalid"
			if err := tc.ensure("nats://[invalid"); err != nil {
				t.Fatalf("NATS-managed reconnect was interrupted: %v", err)
			}
			if *tc.conn != conn || *tc.js != js || !conn.IsReconnecting() {
				t.Fatal("reconnecting transport was replaced or closed")
			}
		})
	}
}

func TestResilienceEventPublishersRecoverJetStreamPublication(t *testing.T) {
	brokerURL := os.Getenv("LEGION_TEST_NATS_URL")
	if brokerURL == "" {
		t.Skip("set LEGION_TEST_NATS_URL to a dedicated JetStream test broker")
	}
	reader, err := nats.Connect(brokerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Close)
	js, err := reader.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	stream := "RECOVERY_" + uuid.NewString()
	prefix := "recovery." + uuid.NewString()
	if _, err := js.AddStream(&nats.StreamConfig{
		Name: stream, Subjects: []string{prefix + ".>"}, Storage: nats.FileStorage,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := js.DeleteStream(stream); err != nil {
			t.Errorf("delete recovery stream: %v", err)
		}
	})
	session := node.SessionState{
		NodeID: "node-recovery", SessionID: "node-session-recovery", SessionToken: "test-token",
		NATSURL: brokerURL, CommandSubject: "recovery.command", EventSubjectPrefix: prefix,
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID: session.NodeID, BaseDir: t.TempDir(), EnrollmentToken: "test-enrollment",
		PlatformAPIBaseURL: "http://platform.test", TransportClient: &bootstrapSessionTransport{session: session},
		HeartbeatInterval: time.Hour, TickerInterval: time.Hour, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	go base.Serve()
	t.Cleanup(func() { base.Shutdown() })
	waitForNodeSession(t, base)

	for _, tc := range eventPublisherRecoveryCases(base) {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(tc.close)
			var previousConn *nats.Conn
			var previousSequence uint64
			for phase := 0; phase < 3; phase++ {
				if phase == 1 {
					// Model the terminal state reached after reconnect attempts
					// are exhausted, without sleeping through the retry budget.
					previousConn.Close()
				}
				id := fmt.Sprintf("%s-%d", tc.name, phase)
				started := time.Now().UTC()
				if err := tc.publish(id); err != nil {
					t.Fatalf("publish phase %d: %v", phase, err)
				}
				if *tc.conn == nil || !(*tc.conn).IsConnected() {
					t.Fatal("publish did not leave a connected transport")
				}
				if phase == 1 && *tc.conn == previousConn {
					t.Fatal("terminal connection was not replaced")
				}
				if phase == 2 && *tc.conn != previousConn {
					t.Fatal("healthy recovered connection was unnecessarily replaced")
				}
				previousConn = *tc.conn
				raw, err := js.GetLastMsg(stream, prefix+"."+tc.eventType)
				if err != nil {
					t.Fatal(err)
				}
				if raw.Sequence <= previousSequence {
					t.Fatal("no new durable event was stored")
				}
				previousSequence = raw.Sequence
				event := tc.newEvent()
				if err := proto.Unmarshal(raw.Data, event); err != nil {
					t.Fatal(err)
				}
				metadata := event.GetMetadata()
				if metadata.GetCausationId() != id || metadata.GetEventType() != tc.eventType ||
					metadata.GetNode().GetNodeId() != session.NodeID ||
					metadata.GetNode().GetNodeSessionId() != session.SessionID {
					t.Fatalf("unexpected recovered event metadata: %v", metadata)
				}
				if metadata.GetEventId() == "" || raw.Header.Get(nats.MsgIdHdr) != metadata.GetEventId() {
					t.Fatal("durable event lost its deduplication identity")
				}
				if status, ok := event.(*capabilityv1.CapabilityStatus); ok {
					if status.GetStatus() != capabilityStatusRunning || status.GetCapability().GetCapabilityKey() != "ai.runtime.host.v1" ||
						status.GetObservedAt().AsTime().Before(started) {
						t.Fatalf("capability observation was not refreshed: %v", status)
					}
				}
			}
		})
	}
}
