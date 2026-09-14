//go:build linux

package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"google.golang.org/protobuf/proto"
)

type managedInputJetStreamHarness struct {
	bridge         *legionJobBridge
	manager        *aiSessionRuntimeManager
	driver         *recordingAISessionRuntimeDriver
	resolver       *inputresolver.Resolver
	resolverDir    string
	commandJS      nats.JetStreamContext
	commandConn    *nats.Conn
	consumer       *commandConsumer
	durable        string
	stream         string
	nodeID         string
	sessionID      string
	commandSubject string
	base           *node.NodeBase
	platform       *httptest.Server
}

func newManagedInputJetStreamHarness(t *testing.T, suffix string, ackWait time.Duration, handler http.Handler) *managedInputJetStreamHarness {
	t.Helper()
	brokerURL := os.Getenv("LEGION_TEST_NATS_URL")
	if brokerURL == "" {
		t.Skip("set LEGION_TEST_NATS_URL to a dedicated JetStream test broker")
	}
	platform := httptest.NewServer(handler)
	unique := suffix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	nodeID := "node-ai-" + unique
	sessionID := "node-session-" + suffix
	commandSubject := "legion.command.node." + nodeID
	session := node.SessionState{
		NodeID:             nodeID,
		SessionID:          sessionID,
		SessionToken:       "node-session-token",
		NATSURL:            brokerURL,
		CommandSubject:     commandSubject,
		EventSubjectPrefix: "legion.event",
	}
	base, err := node.NewNodeBase(node.BaseConfig{
		NodeID:             "node-ai-bootstrap",
		BaseDir:            t.TempDir(),
		EnrollmentToken:    "enroll-ai",
		PlatformAPIBaseURL: platform.URL,
		TransportClient:    &aiBootstrapSessionTransport{session: session},
		HeartbeatInterval:  time.Hour,
		TickerInterval:     time.Hour,
		RequestTimeout:     time.Second,
	})
	if err != nil {
		platform.Close()
		t.Fatalf("new node base: %v", err)
	}
	go base.Serve()
	waitForAINodeSession(t, base)

	resolverDir := filepath.Join(t.TempDir(), "inputs")
	resolver, err := inputresolver.New(inputresolver.Config{
		Root:                resolverDir,
		DiskHeadroomBytes:   1,
		OutputBytes:         1 << 20,
		MaxResourceBytes:    1 << 20,
		MaxWorkspaceBytes:   2 << 20,
		StallTimeout:        2 * time.Second,
		TotalTimeout:        10 * time.Second,
		DownloadConcurrency: 2,
	})
	if err != nil {
		base.Shutdown()
		platform.Close()
		t.Fatalf("new input resolver: %v", err)
	}
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	manager.inputResolver = resolver
	manager.inputResolverError = nil
	agent := &ScanNode{
		node:       base,
		httpClient: platform.Client(),
		ruleSyncClient: NewRuleSyncClient(&RuleSyncConfig{
			ServerURL: platform.URL,
			CacheDir:  filepath.Join(t.TempDir(), "rules"),
			Client:    platform.Client(),
		}),
	}
	bridge := newLegionJobBridge(agent)
	bridge.aiRuntime = manager

	commandConn, err := nats.Connect(brokerURL, nats.Name("managed-input-test-publisher"))
	if err != nil {
		base.Shutdown()
		platform.Close()
		t.Fatalf("connect task broker: %v", err)
	}
	commandJS, err := commandConn.JetStream()
	if err != nil {
		commandConn.Close()
		base.Shutdown()
		platform.Close()
		t.Fatalf("create task JetStream context: %v", err)
	}
	ensureManagedInputStream(t, commandJS, legionCommandStream, []string{"legion.command.>"})
	ensureManagedInputStream(t, commandJS, "LEGION_EVENTS", []string{"legion.event.>"})
	durable := consumerNameForNode(session.NodeID)
	if err := commandJS.DeleteConsumer(legionCommandStream, durable); err != nil && err != nats.ErrConsumerNotFound {
		t.Fatalf("clear task consumer: %v", err)
	}
	if _, err := commandJS.AddConsumer(legionCommandStream, &nats.ConsumerConfig{
		Durable:       durable,
		FilterSubject: commandSubjectWildcard(session.CommandSubject),
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxAckPending: 64,
	}); err != nil {
		t.Fatalf("create task consumer ack_wait=%s: %v", ackWait, err)
	}
	consumer, err := bridge.startConsumer(context.Background(), brokerURL, session.SessionID, session.CommandSubject)
	if err != nil {
		_ = commandJS.DeleteConsumer(legionCommandStream, durable)
		commandConn.Close()
		base.Shutdown()
		platform.Close()
		t.Fatalf("start task consumer: %v", err)
	}
	bridge.mu.Lock()
	bridge.consumer = consumer
	bridge.mu.Unlock()
	h := &managedInputJetStreamHarness{
		bridge: bridge, manager: manager, driver: driver, resolver: resolver,
		resolverDir: resolverDir, commandJS: commandJS, commandConn: commandConn,
		consumer: consumer, durable: durable, stream: legionCommandStream,
		nodeID: nodeID, sessionID: sessionID, commandSubject: commandSubject,
		base: base, platform: platform,
	}
	t.Cleanup(func() {
		bridge.stopConsumer()
		bridge.publisher.Close()
		bridge.aiPublisher.Close()
		_ = commandJS.DeleteConsumer(legionCommandStream, durable)
		commandConn.Close()
		base.Shutdown()
		platform.Close()
	})
	return h
}

func ensureManagedInputStream(t *testing.T, js nats.JetStreamContext, name string, subjects []string) {
	t.Helper()
	if _, err := js.StreamInfo(name); err == nil {
		return
	}
	if _, err := js.AddStream(&nats.StreamConfig{Name: name, Subjects: subjects, Storage: nats.FileStorage}); err != nil {
		t.Fatalf("create task stream %s: %v", name, err)
	}
}

func (h *managedInputJetStreamHarness) publish(t *testing.T, subject string, message proto.Message) {
	t.Helper()
	if _, err := h.commandJS.Publish(subject, mustMarshalProto(t, message)); err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
}

func (h *managedInputJetStreamHarness) publishBind(t *testing.T, command *aiv1.BindAISessionCommand) {
	command.TargetNodeId = h.nodeID
	h.publish(t, h.commandSubject+"."+legionCommandAISessionBind, command)
}

func (h *managedInputJetStreamHarness) publishCancel(t *testing.T, command *aiv1.BindAISessionCommand, reason string) {
	h.publish(t, h.commandSubject+"."+legionCommandAISessionCancel, &aiv1.CancelAISessionCommand{
		Metadata:    &nodev1.CommandMetadata{CommandId: "cancel-" + h.nodeID},
		Session:     &aiv1.AISessionRef{SessionId: command.Session.SessionId, RunId: command.Session.RunId, BindEpoch: command.BindEpoch},
		OwnerUserId: command.OwnerUserId,
		Reason:      reason,
	})
}

func (h *managedInputJetStreamHarness) publishClose(t *testing.T, command *aiv1.BindAISessionCommand, reason string) {
	h.publish(t, h.commandSubject+"."+legionCommandAISessionClose, &aiv1.CloseAISessionCommand{
		Metadata:    &nodev1.CommandMetadata{CommandId: "close-" + h.nodeID},
		Session:     &aiv1.AISessionRef{SessionId: command.Session.SessionId, RunId: command.Session.RunId, BindEpoch: command.BindEpoch},
		OwnerUserId: command.OwnerUserId,
		Reason:      reason,
	})
}

func managedInputVariantBind(t *testing.T, base *aiv1.BindAISessionCommand, index int) *aiv1.BindAISessionCommand {
	t.Helper()
	command := proto.Clone(base).(*aiv1.BindAISessionCommand)
	runID := fmt.Sprintf("run-saturation-%d", index)
	sessionID := fmt.Sprintf("ai-session-saturation-%d", index)
	workspaceID := base.InputManifest.WorkspaceId[:len(base.InputManifest.WorkspaceId)-1] + fmt.Sprintf("%x", index)
	command.Metadata.CommandId = fmt.Sprintf("bind-saturation-%d", index)
	command.Session.SessionId = sessionID
	command.Session.RunId = runID
	command.ResultContext.FocusRunId = runID
	command.ResultContext.TargetUrl = "https://workspace.invalid/" + workspaceID + "/"
	command.InputManifest.SessionId = sessionID
	command.InputManifest.WorkspaceId = workspaceID
	command.InputManifest.AttemptCommandId = command.Metadata.CommandId
	if err := inputresolver.Seal(command.InputManifest); err != nil {
		t.Fatalf("seal saturation input manifest: %v", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(command.RuntimeOptionSnapshotJson, &snapshot); err != nil {
		t.Fatalf("decode saturation runtime snapshot: %v", err)
	}
	snapshot["focus_target_url"] = command.ResultContext.TargetUrl
	snapshot["input_manifest_id"] = command.InputManifest.ManifestId
	var err error
	command.RuntimeOptionSnapshotJson, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode saturation runtime snapshot: %v", err)
	}
	return command
}

func managedInputHasTombstone(manager *aiSessionRuntimeManager, sessionID string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	_, ok := manager.terminalTombstones[sessionID]
	return ok
}

func (h *managedInputJetStreamHarness) stopConsumerOnly() {
	h.bridge.stopConsumer()
}

func waitManagedInput(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not reached within %s", timeout)
}

func managedInputBindingCount(driver *recordingAISessionRuntimeDriver) int {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return len(driver.bindings)
}

func managedInputBindingEpoch(driver *recordingAISessionRuntimeDriver) uint64 {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if len(driver.bindings) == 0 {
		return 0
	}
	return driver.bindings[len(driver.bindings)-1].Ref.BindEpoch
}

func managedInputNoPendingBinding(manager *aiSessionRuntimeManager) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.bindings) == 0
}

func managedInputPendingBindingCount(manager *aiSessionRuntimeManager) int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.bindings)
}

func managedInputBindWorkersIdle(consumer *commandConsumer) bool {
	return consumer == nil || len(consumer.bindSlots) == 0
}

func managedInputRuntimeClean(h *managedInputJetStreamHarness, sessionID string) bool {
	return managedInputNoPendingBinding(h.manager) && !hasAISessionRuntime(h.manager, sessionID)
}

func managedInputNoWorkspaceDirs(t *testing.T, root string) {
	t.Helper()
	for _, pattern := range []string{filepath.Join(root, "workspace-*"), filepath.Join(root, ".staging", "workspace-*")} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("scan resolver root: %v", err)
		}
		if len(matches) != 0 {
			t.Fatalf("workspace residue: %s", matches[0])
		}
	}
}

func TestResilienceManagedInputJetStreamCancelDuringPreparation(t *testing.T) {
	var started, cancelled atomic.Bool
	h := newManagedInputJetStreamHarness(t, "cancel", 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Store(true)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		cancelled.Store(true)
	}))
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	h.publishBind(t, command)
	waitManagedInput(t, time.Second, started.Load)
	h.publishCancel(t, command, "cancel pending preparation")
	waitManagedInput(t, time.Second, cancelled.Load)
	waitManagedInput(t, time.Second, func() bool { return managedInputRuntimeClean(h, command.Session.SessionId) })
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}

func TestResilienceManagedInputJetStreamHigherEpochRebind(t *testing.T) {
	var requestCount atomic.Int32
	firstStarted := make(chan struct{})
	firstCancelled := make(chan struct{})
	h := newManagedInputJetStreamHarness(t, "rebind", 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			close(firstStarted)
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
			close(firstCancelled)
			return
		}
		fmt.Fprint(w, "hello")
	}))
	first := managedInputBindFixture(t, "synthetic_inventory", "hello")
	h.publishBind(t, first)
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first bind did not reach download")
	}
	second := proto.Clone(first).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "bind-followup-epoch-2"
	second.BindEpoch = 2
	second.Session.BindEpoch = 2
	h.publishBind(t, second)
	waitManagedInput(t, 2*time.Second, func() bool { return managedInputBindingCount(h.driver) == 1 && managedInputBindingEpoch(h.driver) == 2 })
	select {
	case <-firstCancelled:
	case <-time.After(time.Second):
		t.Fatal("old preparation was not cancelled by higher epoch")
	}
	h.publishCancel(t, second, "followup cleanup")
	waitManagedInput(t, time.Second, func() bool { return managedInputRuntimeClean(h, second.Session.SessionId) })
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}

func TestResilienceManagedInputJetStreamAckWaitDuringPreparation(t *testing.T) {
	var downloads atomic.Int32
	h := newManagedInputJetStreamHarness(t, "ackwait", 100*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		time.Sleep(350 * time.Millisecond)
		fmt.Fprint(w, "hello")
	}))
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	h.publishBind(t, command)
	waitManagedInput(t, 3*time.Second, func() bool { return managedInputBindingCount(h.driver) == 1 })
	waitManagedInput(t, 2*time.Second, func() bool {
		info, err := h.commandJS.ConsumerInfo(h.stream, h.durable)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
	if got := downloads.Load(); got != 1 {
		t.Fatalf("long preparation downloaded %d times", got)
	}
	info, err := h.commandJS.ConsumerInfo(h.stream, h.durable)
	if err != nil {
		t.Fatalf("consumer info: %v", err)
	}
	if info.Delivered.Consumer != 1 {
		t.Fatalf("long preparation delivered %d times", info.Delivered.Consumer)
	}
	h.publishCancel(t, command, "followup cleanup")
	waitManagedInput(t, time.Second, func() bool { return managedInputRuntimeClean(h, command.Session.SessionId) })
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}

func TestResilienceManagedInputJetStreamStopDuringPreparation(t *testing.T) {
	var started, cancelled atomic.Bool
	h := newManagedInputJetStreamHarness(t, "stop", 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Store(true)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		cancelled.Store(true)
	}))
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	h.publishBind(t, command)
	waitManagedInput(t, time.Second, started.Load)
	h.stopConsumerOnly()
	waitManagedInput(t, time.Second, cancelled.Load)
	waitManagedInput(t, time.Second, func() bool { return managedInputRuntimeClean(h, command.Session.SessionId) })
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}

func TestResilienceManagedInputJetStreamCloseDuringPreparation(t *testing.T) {
	var started, cancelled atomic.Bool
	h := newManagedInputJetStreamHarness(t, "close", 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Store(true)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		cancelled.Store(true)
	}))
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	h.publishBind(t, command)
	waitManagedInput(t, time.Second, started.Load)
	h.publishClose(t, command, "close during preparation")
	waitManagedInput(t, time.Second, cancelled.Load)
	waitManagedInput(t, time.Second, func() bool { return managedInputRuntimeClean(h, command.Session.SessionId) })
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}

func TestResilienceManagedInputJetStreamBindPoolSaturation(t *testing.T) {
	var started, cancelled atomic.Int32
	h := newManagedInputJetStreamHarness(t, "saturation", 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Add(1)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		cancelled.Add(1)
	}))
	base := managedInputBindFixture(t, "synthetic_inventory", "hello")
	commands := make([]*aiv1.BindAISessionCommand, 5)
	for i := range commands {
		commands[i] = managedInputVariantBind(t, base, i+1)
		h.publishBind(t, commands[i])
	}
	// Resolver download concurrency is two; the other two preparation workers
	// remain occupied waiting for that semaphore, so all four bind slots are in use.
	waitManagedInput(t, 2*time.Second, func() bool { return started.Load() == 2 && managedInputPendingBindingCount(h.manager) == 4 })
	h.publishCancel(t, commands[4], "cancel queued saturation bind")
	waitManagedInput(t, time.Second, func() bool { return managedInputHasTombstone(h.manager, commands[4].Session.SessionId) })
	if got := started.Load(); got != 2 {
		t.Fatalf("queued bind entered download despite four occupied preparation slots: %d", got)
	}
	for i := 0; i < 4; i++ {
		h.publishCancel(t, commands[i], "cancel saturated preparation")
	}
	// Cancellation can admit another waiting download before its Cancel is
	// handled. Wait for all workers and the deferred fifth Bind to settle.
	waitManagedInput(t, 3*time.Second, func() bool {
		gotStarted, gotCancelled := started.Load(), cancelled.Load()
		info, err := h.commandJS.ConsumerInfo(h.stream, h.durable)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0 &&
			gotCancelled == gotStarted && managedInputNoPendingBinding(h.manager) && managedInputBindWorkersIdle(h.consumer)
	})
	if started.Load() > 4 || managedInputBindingCount(h.driver) != 0 {
		t.Fatalf("cancelled binds escaped preparation: downloads=%d engines=%d", started.Load(), managedInputBindingCount(h.driver))
	}
	managedInputNoWorkspaceDirs(t, h.resolverDir)
}
