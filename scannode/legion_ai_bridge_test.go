package scannode

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

func TestValidateAISessionBindCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*aiv1.BindAISessionCommand)
		wantErr string
	}{
		{
			name: "valid command",
		},
		{
			name: "missing metadata",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.Metadata = nil
			},
			wantErr: "ai session bind metadata is required",
		},
		{
			name: "missing command id",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.Metadata.CommandId = ""
			},
			wantErr: "ai session bind command_id is required",
		},
		{
			name: "missing target node id",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.TargetNodeId = ""
			},
			wantErr: "ai session bind target_node_id is required",
		},
		{
			name: "target mismatch",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "ai session bind target_node_id mismatch: node-b",
		},
		{
			name: "missing session",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.Session = nil
			},
			wantErr: "ai session bind session reference is required",
		},
		{
			name: "missing session id",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.Session.SessionId = ""
			},
			wantErr: "ai session bind session_id is required",
		},
		{
			name: "missing owner user id",
			mutate: func(command *aiv1.BindAISessionCommand) {
				command.OwnerUserId = ""
			},
			wantErr: "ai session bind owner_user_id is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			command := validAISessionBindCommand()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAISessionBindCommand("node-ai", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate ai bind command: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAISessionInputCommandRequiresValidJSON(t *testing.T) {
	t.Parallel()

	command := validAISessionInputCommand()
	command.InputJson = []byte(`{"content":`)

	err := validateAISessionInputCommand(command)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "ai session input_json must be valid json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleAISessionBindPublishesReady(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)
	raw := mustMarshalProto(t, validAISessionBindCommand())

	if err := bridge.handleAISessionBind(context.Background(), raw); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.ready" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event aiv1.AISessionReady
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session ready: %v", err)
	}
	if event.GetSession().GetSessionId() != "ai-session-1" {
		t.Fatalf("unexpected session id: %s", event.GetSession().GetSessionId())
	}
	if event.GetRuntimeName() != "yak-ai-runtime" {
		t.Fatalf("unexpected runtime name: %s", event.GetRuntimeName())
	}
	if event.GetMetadata().GetEventType() != legionEventAISessionReady {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	if event.GetMetadata().GetCausationId() != "cmd-bind-1" {
		t.Fatalf("unexpected causation id: %s", event.GetMetadata().GetCausationId())
	}
	if event.GetMetadata().GetCorrelationId() != "ai-session-1" {
		t.Fatalf("unexpected correlation id: %s", event.GetMetadata().GetCorrelationId())
	}
	if event.GetMetadata().GetNode().GetNodeId() != "node-ai" {
		t.Fatalf("unexpected node id: %s", event.GetMetadata().GetNode().GetNodeId())
	}
}

func TestHandleAISessionBindPassesAttachmentAndCredentialRefsToRuntime(t *testing.T) {
	t.Parallel()

	bridge, _, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.Attachments = []*aiv1.AISessionAttachmentRef{
		{
			AttachmentId: "inputf_123",
			Filename:     "targets.txt",
			DownloadUrl:  "http://platform.test/v1/ai/attachments/inputf_123/download?node_session_id=node-session-ai",
		},
	}
	command.CredentialRefs = []*aiv1.AISessionCredentialRef{
		{
			CredentialId:   "sourcecred-1",
			CredentialType: "ssa_source",
			Scope:          "ssa.source",
		},
	}

	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}

	driver.mu.Lock()
	defer driver.mu.Unlock()
	if len(driver.bindings) != 1 {
		t.Fatalf("unexpected bind count: %d", len(driver.bindings))
	}
	binding := driver.bindings[0]
	if len(binding.Attachments) != 1 || binding.Attachments[0].AttachmentID != "inputf_123" {
		t.Fatalf("unexpected binding attachments: %#v", binding.Attachments)
	}
	if len(binding.CredentialRefs) != 1 || binding.CredentialRefs[0].CredentialID != "sourcecred-1" {
		t.Fatalf("unexpected binding credential refs: %#v", binding.CredentialRefs)
	}
	if binding.PlatformBearerToken != "node-session-token" {
		t.Fatalf("unexpected platform bearer token: %q", binding.PlatformBearerToken)
	}
}

func TestHandleAISessionInputPublishesRuntimeEvent(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("handle ai input: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.event" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event aiv1.AISessionEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session event: %v", err)
	}
	if event.GetSession().GetSessionId() != "ai-session-1" {
		t.Fatalf("unexpected session id: %s", event.GetSession().GetSessionId())
	}
	if event.GetSeq() != 1 {
		t.Fatalf("unexpected seq: %d", event.GetSeq())
	}
	if event.GetEventType() != aiSessionRuntimeEventInput {
		t.Fatalf("unexpected runtime event type: %s", event.GetEventType())
	}

	var payload map[string]any
	if err := json.Unmarshal(event.GetPayloadJson(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["content"] != "hello" {
		t.Fatalf("unexpected payload content: %#v", payload["content"])
	}
	if payload["role"] != "user" {
		t.Fatalf("unexpected payload role: %#v", payload["role"])
	}
	if payload["input_type"] != "message" {
		t.Fatalf("unexpected input type: %#v", payload["input_type"])
	}
	driver.assertInput(t, 0, "hello")
}

func TestHandleAISessionCancelPublishesCancelled(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	if err := bridge.handleAISessionCancel(context.Background(), mustMarshalProto(t, validAISessionCancelCommand())); err != nil {
		t.Fatalf("handle ai cancel: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.cancelled" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event aiv1.AISessionCancelled
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session cancelled: %v", err)
	}
	if event.GetSession().GetSessionId() != "ai-session-1" {
		t.Fatalf("unexpected session id: %s", event.GetSession().GetSessionId())
	}
	if event.GetReason() != "user requested" {
		t.Fatalf("unexpected reason: %s", event.GetReason())
	}
	if event.GetMetadata().GetEventType() != legionEventAISessionCancelled {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	driver.assertCancel(t, 0, "user requested")
}

func TestHandleAISessionClosePublishesDone(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	if err := bridge.handleAISessionClose(context.Background(), mustMarshalProto(t, validAISessionCloseCommand())); err != nil {
		t.Fatalf("handle ai close: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.done" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event aiv1.AISessionDone
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session done: %v", err)
	}
	if event.GetSession().GetSessionId() != "ai-session-1" {
		t.Fatalf("unexpected session id: %s", event.GetSession().GetSessionId())
	}
	if event.GetMetadata().GetEventType() != legionEventAISessionDone {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	if !strings.Contains(string(event.GetResultJson()), "\"closed_by\":\"platform\"") {
		t.Fatalf("unexpected done payload: %s", string(event.GetResultJson()))
	}
	driver.assertClose(t, 0, "platform done")
}

func TestCommandConsumerRoutesAISessionBindCommand(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)
	message := nats.NewMsg("legion.command.node.node-ai.ai.session.bind")
	message.Data = mustMarshalProto(t, validAISessionBindCommand())

	if err := bridge.handleMessage(context.Background(), message); err != nil {
		t.Fatalf("handle routed ai bind: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.ready" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}
}

func TestCommandConsumerRoutesAISessionCloseCommand(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	message := nats.NewMsg("legion.command.node.node-ai.ai.session.close")
	message.Data = mustMarshalProto(t, validAISessionCloseCommand())

	if err := bridge.handleMessage(context.Background(), message); err != nil {
		t.Fatalf("handle routed ai close: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.done" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}
}

type aiBootstrapSessionTransport struct {
	session node.SessionState
}

func (s *aiBootstrapSessionTransport) Bootstrap(context.Context, node.BootstrapRequest) (node.SessionState, error) {
	return s.session, nil
}

func (s *aiBootstrapSessionTransport) Heartbeat(context.Context, node.SessionState, node.HeartbeatRequest) error {
	return nil
}

func (s *aiBootstrapSessionTransport) Shutdown(context.Context, node.SessionState, node.ShutdownRequest) error {
	return nil
}

type aiFakeJetStreamContext struct {
	nats.JetStreamContext

	mu      sync.Mutex
	publish []*nats.Msg
}

func (f *aiFakeJetStreamContext) PublishMsg(msg *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
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

func newTestAISessionBridge(
	t *testing.T,
) (*legionJobBridge, *aiFakeJetStreamContext, *recordingAISessionRuntimeDriver) {
	t.Helper()

	session := node.SessionState{
		NodeID:             "node-ai",
		SessionID:          "node-session-ai",
		SessionToken:       "node-session-token",
		NATSURL:            "nats://node-ai.test",
		CommandSubject:     "legion.command.node.node-ai",
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-ai-bootstrap",
		BaseDir:            t.TempDir(),
		EnrollmentToken:    "enroll-ai",
		PlatformAPIBaseURL: "http://platform.test",
		TransportClient:    &aiBootstrapSessionTransport{session: session},
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
	waitForAINodeSession(t, base)

	bridge := newLegionJobBridge(&ScanNode{
		node:       base,
		httpClient: &http.Client{Timeout: time.Second},
	})
	driver := &recordingAISessionRuntimeDriver{}
	bridge.aiRuntime = newAISessionRuntimeManager(driver)
	fakeJS := &aiFakeJetStreamContext{}
	bridge.aiPublisher.js = fakeJS
	bridge.aiPublisher.natsURL = session.NATSURL
	return bridge, fakeJS, driver
}

func validAISessionBindCommand() *aiv1.BindAISessionCommand {
	return &aiv1.BindAISessionCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-bind-1",
		},
		TargetNodeId: "node-ai",
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
		},
		OwnerUserId: "user-1",
		ProjectId:   "project-1",
		Title:       "AI session",
	}
}

func validAISessionInputCommand() *aiv1.PushAISessionInputCommand {
	return &aiv1.PushAISessionInputCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-input-1",
		},
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
		},
		OwnerUserId: "user-1",
		InputType:   "message",
		InputJson:   []byte(`{"content":"hello"}`),
	}
}

func validAISessionCancelCommand() *aiv1.CancelAISessionCommand {
	return &aiv1.CancelAISessionCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-cancel-1",
		},
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
		},
		OwnerUserId: "user-1",
		Reason:      "user requested",
	}
}

func validAISessionCloseCommand() *aiv1.CloseAISessionCommand {
	return &aiv1.CloseAISessionCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-close-1",
		},
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
		},
		OwnerUserId: "user-1",
		Reason:      "platform done",
	}
}

func mustMarshalProto(t *testing.T, message proto.Message) []byte {
	t.Helper()

	raw, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}
	return raw
}

func waitForAINodeSession(t *testing.T, base *node.NodeBase) node.SessionState {
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

func waitForPublishedMessage(t *testing.T, fakeJS *aiFakeJetStreamContext, index int) *nats.Msg {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fakeJS.mu.Lock()
		if len(fakeJS.publish) > index {
			msg := fakeJS.publish[index]
			fakeJS.mu.Unlock()
			return msg
		}
		fakeJS.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for published message at index %d", index)
	return nil
}

func resetPublishedMessages(fakeJS *aiFakeJetStreamContext) {
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	fakeJS.publish = nil
}

type recordingAISessionRuntimeDriver struct {
	mu       sync.Mutex
	bindings []aiSessionBinding
	inputs   []aiSessionInput
	cancels  []string
	closes   []string
}

func (d *recordingAISessionRuntimeDriver) Bind(
	_ context.Context,
	binding aiSessionBinding,
	_ aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bindings = append(d.bindings, binding)
	return &recordingAISessionRuntimeHandle{driver: d}, nil
}

func (d *recordingAISessionRuntimeDriver) assertInput(t *testing.T, index int, content string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if len(d.inputs) > index {
			input := d.inputs[index]
			d.mu.Unlock()

			var payload map[string]any
			if err := json.Unmarshal(input.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal recorded input payload: %v", err)
			}
			if payload["content"] != content {
				t.Fatalf("unexpected recorded input content: %#v", payload["content"])
			}
			return
		}
		d.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for recorded input at index %d", index)
}

func (d *recordingAISessionRuntimeDriver) assertCancel(t *testing.T, index int, reason string) {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.cancels) <= index {
		t.Fatalf("missing recorded cancel at index %d", index)
	}
	if d.cancels[index] != reason {
		t.Fatalf("unexpected cancel reason: %s", d.cancels[index])
	}
}

func (d *recordingAISessionRuntimeDriver) assertClose(t *testing.T, index int, reason string) {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.closes) <= index {
		t.Fatalf("missing recorded close at index %d", index)
	}
	if d.closes[index] != reason {
		t.Fatalf("unexpected close reason: %s", d.closes[index])
	}
}

type recordingAISessionRuntimeHandle struct {
	driver *recordingAISessionRuntimeDriver
}

func (h *recordingAISessionRuntimeHandle) SendInput(_ context.Context, input aiSessionInput) error {
	h.driver.mu.Lock()
	defer h.driver.mu.Unlock()
	h.driver.inputs = append(h.driver.inputs, input)
	return nil
}

func (h *recordingAISessionRuntimeHandle) Cancel(reason string) {
	h.driver.mu.Lock()
	defer h.driver.mu.Unlock()
	h.driver.cancels = append(h.driver.cancels, reason)
}

func (h *recordingAISessionRuntimeHandle) Close(reason string) {
	h.driver.mu.Lock()
	defer h.driver.mu.Unlock()
	h.driver.closes = append(h.driver.closes, reason)
}
