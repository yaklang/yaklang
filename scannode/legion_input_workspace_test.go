//go:build linux

package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"google.golang.org/protobuf/proto"
)

type managedInputTestSink struct {
	recordingServerFocusSink
	reports []aiFocusCodeAuditReport
}

func (*managedInputTestSink) bindFocusExecutionContract(*legionFocusExecutionContract) error {
	return nil
}
func (*managedInputTestSink) SubmitCodeFinding(context.Context, string, aiFocusCodeFinding) (aiFocusResultReceipt, error) {
	return aiFocusResultReceipt{}, nil
}
func (s *managedInputTestSink) SubmitCodeAuditReport(_ context.Context, _ string, report aiFocusCodeAuditReport) (aiFocusResultReceipt, error) {
	s.reports = append(s.reports, report)
	return aiFocusResultReceipt{ResultID: "report"}, nil
}

func managedInputBindFixture(t *testing.T, task, content string) *aiv1.BindAISessionCommand {
	t.Helper()
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.FocusMode = task
	command.ResultContext.FocusReleaseId = task + "@9.8.7+abcdef123456"
	command.Session.RunId = command.ResultContext.FocusRunId
	command.ResultContext.TargetUrl = "https://workspace.invalid/" + testLegionCodeWorkspaceID + "/"
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
	manifest := &aiv1.InputManifest{SchemaVersion: inputresolver.SchemaV1, OwnerUserId: command.OwnerUserId, ProductKey: "ssa", RunId: "task-run-a", SessionId: command.Session.SessionId, AttemptId: command.ResultContext.Job.AttemptId, WorkspaceId: testLegionCodeWorkspaceID, OutputPath: "outputs", AttemptCommandId: command.Metadata.CommandId,
		Resources: []*aiv1.InputResource{{ResourceId: "input-a", Kind: inputresolver.ManagedAttachment, InputField: "documents", RelativePath: "inputs/documents/001-same.txt", Filename: "same.txt", MediaType: "text/plain", SizeBytes: uint64(len(content)), Sha256: digest, Required: true, ReadOnly: true}}}
	if err := inputresolver.Seal(manifest); err != nil {
		t.Fatal(err)
	}
	command.InputManifest = manifest
	command.Attachments = []*aiv1.AISessionAttachmentRef{{AttachmentId: "input-a", Filename: "same.txt", ContentType: "text/plain", SizeBytes: uint64(len(content)), Sha256: digest, DownloadUrl: "https://forged.invalid/never-used"}}
	command.RuntimeOptionSnapshotJson = mustJSON(map[string]any{"ai_task_run_id": manifest.RunId, "ai_task_key": task, "ai_task_version": "9.8.7", "ai_task_definition_checksum": strings.Repeat("a", 64), "ai_task_session_role": "execution", "input_manifest_id": manifest.ManifestId, "focus_target_url": command.ResultContext.TargetUrl})
	return command
}

func inputBindOptionsFixture(t *testing.T, handler http.Handler) aiSessionRuntimeBindOptions {
	t.Helper()
	r, err := inputresolver.New(inputresolver.Config{Root: filepath.Join(t.TempDir(), "inputs"), DiskHeadroomBytes: 1, OutputBytes: 1 << 20, StallTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return aiSessionRuntimeBindOptions{InputResolver: r, PlatformAPIBaseURL: server.URL, NodeSessionID: "node-session", PlatformBearerToken: "node-secret", HTTPClient: server.Client(), ResultSink: &managedInputTestSink{}}
}

func inputExecutionContract(capabilities ...string) *legionFocusExecutionContract {
	c := &legionFocusExecutionContract{SchemaVersion: legionFocusExecutionContractSchemaV1, capabilitySet: map[string]struct{}{}, stageSet: map[string]struct{}{}}
	for _, capability := range capabilities {
		c.Capabilities = append(c.Capabilities, capability)
		c.capabilitySet[capability] = struct{}{}
	}
	return c
}

func TestManagedInputLogAndUnrelatedTaskShareResolverAndFileTools(t *testing.T) {
	for _, task := range []string{"log_analysis", "synthetic_document_inventory"} {
		t.Run(task, func(t *testing.T) {
			command := managedInputBindFixture(t, task, "real scoped file\nlate marker\n")
			if err := validateAISessionBindCommand("node-ai", command); err != nil {
				t.Fatal(err)
			}
			var downloads atomic.Int32
			options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				if r.URL.Path != "/v1/ai/attachments/input-a/download" {
					t.Error("used command URL")
				}
				fmt.Fprint(w, "real scoped file\nlate marker\n")
			}))
			driver := &recordingAISessionRuntimeDriver{}
			manager := newAISessionRuntimeManager(driver)
			ref, err := manager.Bind(context.Background(), command, nil, options)
			if err != nil {
				t.Fatal(err)
			}
			binding := driver.bindings[0]
			workspace := binding.InputWorkspace
			if workspace == nil || len(binding.Attachments) != 0 {
				t.Fatal("managed input was inlined or missing")
			}
			defer workspace.Cleanup()
			if strings.Contains(string(binding.RuntimeOptionSnapshotJSON), "download_url") {
				t.Fatal("download URL entered model options")
			}
			runtime := binding.LegionResultRuntime.(*legionServerFocusRuntime)
			if _, err := runtime.Execute("input.read", map[string]any{"path": "inputs/documents/001-same.txt"}); err == nil {
				t.Fatal("read before authorized Focus turn")
			}
			contract := inputExecutionContract("source.workspace.info", "source.list", "source.read", "source.search", "output.write")
			if err := runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, contract); err != nil {
				t.Fatal(err)
			}
			ext, err := managedInputTools(runtime)
			if err != nil {
				t.Fatal(err)
			}
			config := aicommon.NewConfig(context.Background(), ext...)
			tool, err := config.GetAiToolManager().GetToolByName("read_file")
			if err != nil {
				t.Fatal(err)
			}
			result, err := tool.Callback(context.Background(), aitool.InvokeParams{"path": "inputs/documents/001-same.txt", "offset": int64(17)}, nil, nil, nil)
			if err != nil || !strings.Contains(fmt.Sprint(result), "late marker") {
				t.Fatalf("scoped standard tool failed: %v %v", result, err)
			}
			for _, denied := range []string{"bash", "exec", "ls", "get_file_content", "read_file_lines", "mcp_remote_shell"} {
				if _, err := config.GetAiToolManager().GetToolByName(denied); err == nil {
					t.Fatalf("escaped finite tools: %s", denied)
				}
			}
			if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
				t.Fatal(err)
			}
			if downloads.Load() != 1 || len(driver.bindings) != 1 {
				t.Fatal("duplicate bind prepared or created engine twice")
			}
			cancel := &aiv1.CancelAISessionCommand{Metadata: &nodev1.CommandMetadata{CommandId: "cancel"}, Session: &aiv1.AISessionRef{SessionId: ref.SessionID, RunId: ref.RunID, BindEpoch: ref.BindEpoch}, OwnerUserId: ref.OwnerUserID}
			cancelled, err := manager.Cancel(cancel)
			if err != nil {
				t.Fatal(err)
			}
			if cancelled.handle != nil {
				cancelled.handle.Cancel("test")
			}
			if err := manager.CompleteTerminal(cancelled.ref, "cancel"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(workspace.RootForDiagnostics()); !os.IsNotExist(err) {
				t.Fatalf("cancel leaked workspace: %v", err)
			}
		})
	}
}

func TestManagedInputFailuresNeverReachRuntimeDriver(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*aiv1.BindAISessionCommand)
		status int
		body   string
	}{
		{"unsupported", func(c *aiv1.BindAISessionCommand) { c.InputManifest.SchemaVersion = "future" }, 200, "hello"},
		{"owner", func(c *aiv1.BindAISessionCommand) { c.OwnerUserId = "different" }, 200, "hello"},
		{"attempt", func(c *aiv1.BindAISessionCommand) { c.ResultContext.Job.AttemptId = "different" }, 200, "hello"},
		{"missing_manifest", func(c *aiv1.BindAISessionCommand) { c.InputManifest = nil }, 200, "hello"},
		{"missing", nil, 404, ""}, {"authorization", nil, 403, ""}, {"size", nil, 200, "shorter"}, {"hash", nil, 200, "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := managedInputBindFixture(t, "synthetic_inventory", "hello")
			if tc.mutate != nil {
				tc.mutate(command)
			}
			options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			driver := &recordingAISessionRuntimeDriver{}
			manager := newAISessionRuntimeManager(driver)
			if _, err := manager.Bind(context.Background(), command, nil, options); err == nil {
				t.Fatal("bad input accepted")
			}
			if len(driver.bindings) != 0 {
				t.Fatal("Agent started before input validation completed")
			}
		})
	}
}

func TestResilienceManagedInputPendingCancelAndAttemptReplacement(t *testing.T) {
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	started := make(chan struct{})
	var calls atomic.Int32
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, "hello")
	}))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	firstDone := make(chan error, 1)
	go func() { _, err := manager.Bind(context.Background(), command, nil, options); firstDone <- err }()
	<-started
	if _, err := manager.Bind(context.Background(), command, nil, options); !errors.Is(err, errAISessionBindRetry) {
		t.Fatalf("pending duplicate=%v", err)
	}
	second := proto.Clone(command).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "bind-new"
	second.BindEpoch = 2
	// Transport rebind retains the immutable attempt command ID and manifest.
	if _, err := manager.Bind(context.Background(), second, nil, options); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstDone:
		if !errors.Is(err, errAISessionBindFenced) {
			t.Fatalf("superseded prep=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("old download survived replacement")
	}
	if len(driver.bindings) != 1 {
		t.Fatal("superseded preparation created runtime")
	}
	w := driver.bindings[0].InputWorkspace
	defer w.Cleanup()
	oldCancel := &aiv1.CancelAISessionCommand{Metadata: &nodev1.CommandMetadata{CommandId: "old-cancel"}, Session: &aiv1.AISessionRef{SessionId: command.Session.SessionId, RunId: command.Session.RunId, BindEpoch: 1}, OwnerUserId: command.OwnerUserId}
	if _, err := manager.Cancel(oldCancel); err == nil {
		t.Fatal("old epoch cancellation accepted")
	}
	if _, err := w.Read(context.Background(), command.InputManifest.Resources[0].RelativePath, 0, 10); err != nil {
		t.Fatalf("old cancel damaged replacement: %v", err)
	}
}

func TestManagedInputTurnCannotReplacePinsOrLoadHostFiles(t *testing.T) {
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") }))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
		t.Fatal(err)
	}
	binding := driver.bindings[0]
	defer binding.InputWorkspace.Cleanup()
	valid := aiSessionInput{InputType: "message", PayloadJSON: []byte(`{"content":"analyze"}`), ContextPackage: &aiv1.ContextPackage{RuntimeOptionSnapshotJson: mustJSON(map[string]string{"input_manifest_id": command.InputManifest.ManifestId, "focus_target_url": command.ResultContext.TargetUrl})}}
	if err := validateInputWorkspaceTurn(binding, valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*aiSessionInput){
		func(i *aiSessionInput) { i.ContextPackage = nil }, func(i *aiSessionInput) { i.InputType = "hotpatch" },
		func(i *aiSessionInput) {
			i.PayloadJSON = []byte(`{"content":"x","attached_resource_info":[{"type":"file","value":"/etc/passwd"}]}`)
		},
		func(i *aiSessionInput) {
			i.ContextPackage = &aiv1.ContextPackage{RuntimeOptionSnapshotJson: []byte(`{"input_manifest_id":"other"}`)}
		},
	} {
		copy := valid
		change(&copy)
		if err := validateInputWorkspaceTurn(binding, copy); err == nil {
			t.Fatal("mutable/host context accepted")
		}
	}
}

func TestResilienceManagedInputCancelDuringPreparation(t *testing.T) {
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	started := make(chan struct{})
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	done := make(chan error, 1)
	go func() { _, err := manager.Bind(context.Background(), command, nil, options); done <- err }()
	<-started
	cancel := &aiv1.CancelAISessionCommand{Metadata: &nodev1.CommandMetadata{CommandId: "cancel-pending"}, Session: &aiv1.AISessionRef{SessionId: command.Session.SessionId, RunId: command.Session.RunId, BindEpoch: command.BindEpoch}, OwnerUserId: command.OwnerUserId}
	if _, err := manager.Cancel(cancel); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errAISessionBindFenced) {
			t.Fatalf("cancelled preparation=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending cancellation did not stop download")
	}
	if len(driver.bindings) != 0 {
		t.Fatal("cancelled preparation started Agent")
	}
	if _, err := manager.Bind(context.Background(), command, nil, options); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("cancelled bind replay was not fenced: %v", err)
	}
}

func TestManagedInputJobEventsUseTransportBindNamespace(t *testing.T) {
	bridge, events, _ := newTestAISessionBridge(t)
	session, _ := bridge.agent.node.GetSessionState()
	bridge.publisher.js = events
	bridge.publisher.natsURL = session.NATSURL
	command := managedInputBindFixture(t, "synthetic_inventory", "hello")
	publish := func(command *aiv1.BindAISessionCommand) ([]string, string) {
		t.Helper()
		resetPublishedMessages(events)
		rawSink, err := newLegionAIFocusResultSinkForBind(bridge.publisher, command)
		if err != nil {
			t.Fatal(err)
		}
		sink := rawSink.(*legionAIFocusResultSink)
		receipt, err := sink.SubmitCodeAuditReport(context.Background(), "inventory_report_v1", aiFocusCodeAuditReport{
			WorkspaceID: command.InputManifest.WorkspaceId, Title: "Inventory", Markdown: "Read the scoped input.", StructuredSummary: []byte(`{"files":1}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, err := range []error{
			sink.Succeed(context.Background(), nil),
			sink.Fail(context.Background(), "test", "test", nil),
			sink.Cancel(context.Background(), "test"),
			bridge.publisher.PublishClaimed(context.Background(), sink.ref),
			bridge.publisher.PublishStarted(context.Background(), sink.ref),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
		prototypes := []proto.Message{&jobv1.JobReport{}, &jobv1.JobReport{}, &jobv1.JobSucceeded{}, &jobv1.JobFailed{}, &jobv1.JobCancelled{}, &jobv1.JobClaimed{}, &jobv1.JobStarted{}}
		if len(events.publish) != len(prototypes) {
			t.Fatalf("unexpected event count: %d", len(events.publish))
		}
		ids := make([]string, len(prototypes))
		for i, event := range prototypes {
			if err := proto.Unmarshal(events.publish[i].Data, event); err != nil {
				t.Fatal(err)
			}
			reflected := event.ProtoReflect()
			metadata := reflected.Get(reflected.Descriptor().Fields().ByName("metadata")).Message().Interface().(*nodev1.EventMetadata)
			if metadata.CausationId != command.Metadata.CommandId || metadata.CorrelationId != command.InputManifest.AttemptId {
				t.Fatalf("event lost transport/attempt identity: %v", metadata)
			}
			ids[i] = metadata.EventId
		}
		if receipt.ResultID != ids[0] {
			t.Fatalf("receipt does not identify published event: %s != %s", receipt.ResultID, ids[0])
		}
		return ids, sink.ref.EventNamespace
	}
	first, firstNamespace := publish(command)
	retry, _ := publish(command)
	for i := range first {
		if first[i] != retry[i] {
			t.Fatalf("same Bind event %d lost idempotency", i)
		}
	}
	rebound := proto.Clone(command).(*aiv1.BindAISessionCommand)
	rebound.Metadata.CommandId = "transport-bind-2"
	rebound.BindEpoch++
	second, secondNamespace := publish(rebound)
	if firstNamespace == secondNamespace || rebound.InputManifest.AttemptCommandId != command.Metadata.CommandId {
		t.Fatal("transport namespace must change independently of the immutable attempt command")
	}
	for i := range first {
		if first[i] == second[i] {
			t.Fatalf("new Bind event %d would be lost to NATS deduplication", i)
		}
	}
	ordinary := proto.Clone(command).(*aiv1.BindAISessionCommand)
	ordinary.InputManifest = nil
	rawSink, err := newLegionAIFocusResultSinkForBind(bridge.publisher, ordinary)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryRef := rawSink.(*legionAIFocusResultSink).ref
	if ordinaryRef.EventNamespace != "" || ordinaryRef.scopedEventID("ordinary-event") != "ordinary-event" {
		t.Fatal("ordinary session event identity changed")
	}
}
