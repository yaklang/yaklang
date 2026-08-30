package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const testAttachmentTaskContent = "INFO synthetic request completed\nWARN synthetic retry\n"
const testAttachmentTaskTarget = "https://attachments.invalid/aicw_0123456789abcdef0123456789abcdef/"
const testAttachmentTaskContractJSON = `{"schema_version":"legion.focus-execution/v1","stages":[{"key":"log_collect"},{"key":"normalize"},{"key":"anomaly_detection"},{"key":"event_correlation"},{"key":"report"}],"capabilities":["result.report.v1","task.stage"],"results":[{"key":"report","capability":"result.report.v1","kind":"ai_log_analysis_v1","required":true}]}`

func testAttachmentTaskFocusRelease() *aiv1.ContextFocusRelease {
	const focusName, version, entryFile = "log_analysis", "1.1.0", "attachment-contract.ai-focus.yak"
	const entryCode = `__VERBOSE_NAME__ = "Attachment Contract Fixture"`
	checksum := contextFocusReleaseChecksum(focusName, version, entryFile, entryCode, nil, testAttachmentTaskContractJSON)
	return &aiv1.ContextFocusRelease{
		ReleaseId: focusName + "@" + version + "+" + checksum[:12], FocusName: focusName,
		Version: version, RuntimeName: "legion_release_log_analysis_1_1_0_" + checksum[:12],
		EntryFile: entryFile, EntryCode: entryCode, Sha256: checksum,
		ExecutionContractJson: testAttachmentTaskContractJSON,
	}
}

func testAttachmentTaskResultContext() *aiv1.AIFocusResultContext {
	result := validCodeAuditResultContext()
	release := testAttachmentTaskFocusRelease()
	result.FocusMode = release.FocusName
	result.FocusReleaseId = release.ReleaseId
	result.TargetUrl = testAttachmentTaskTarget
	return result
}

func testAttachmentTaskBindCommand(t *testing.T) *aiv1.BindAISessionCommand {
	t.Helper()
	command := validAISessionBindCommand()
	command.ProjectId = ""
	command.ResultContext = testAttachmentTaskResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	command.Attachments = []*aiv1.AISessionAttachmentRef{{
		AttachmentId: "attachment-log-1", Filename: "application.log", ContentType: "text/plain",
		SizeBytes: uint64(len(testAttachmentTaskContent)), Sha256: fmt.Sprintf("%x", sha256.Sum256([]byte(testAttachmentTaskContent))),
		DownloadUrl: "https://download.invalid/attachment-log-1",
	}}
	release := testAttachmentTaskFocusRelease()
	setAttachmentTaskRuntimeOptions(t, command, yakRuntimeOptions{
		FocusModeLoop: release.FocusName, FocusReleaseID: release.ReleaseId,
		FocusReleaseSHA256: release.Sha256, FocusRuntimeName: release.RuntimeName,
		FocusTargetURL: testAttachmentTaskTarget,
	})
	return command
}

func setAttachmentTaskRuntimeOptions(t *testing.T, command *aiv1.BindAISessionCommand, options yakRuntimeOptions) {
	t.Helper()
	raw, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal attachment runtime options: %v", err)
	}
	command.RuntimeOptionSnapshotJson = raw
}

func TestValidateAISessionBindAttachmentTargetContract(t *testing.T) {
	t.Parallel()

	attachmentTarget := "https://attachments.invalid/" + testLegionCodeWorkspaceID + "/"
	workspaceTarget := "https://workspace.invalid/" + testLegionCodeWorkspaceID + "/"
	otherWorkspaceTarget := "https://workspace.invalid/aicw_fedcba9876543210fedcba9876543210/"
	tests := []struct {
		name          string
		target        string
		withWorkspace bool
		wantErr       string
	}{
		{
			name:   "attachment_only_without_source_workspace",
			target: attachmentTarget,
		},
		{
			name:    "legacy_attachment_workspace_sentinel_without_source_rejected",
			target:  workspaceTarget,
			wantErr: "workspace.invalid target requires source_workspace",
		},
		{
			name:          "code_workspace_matching_target",
			target:        workspaceTarget,
			withWorkspace: true,
		},
		{
			name:          "code_workspace_mismatched_id_rejected",
			target:        otherWorkspaceTarget,
			withWorkspace: true,
			wantErr:       "source_workspace target_url must equal " + workspaceTarget,
		},
		{
			name:          "code_workspace_cannot_use_attachment_target",
			target:        attachmentTarget,
			withWorkspace: true,
			wantErr:       "source_workspace target_url must equal " + workspaceTarget,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			command := testAttachmentTaskBindCommand(t)
			command.ResultContext.TargetUrl = tt.target
			command.Session.RunId = command.ResultContext.FocusRunId
			options := yakRuntimeOptions{
				FocusModeLoop:  command.ResultContext.FocusMode,
				FocusReleaseID: command.ResultContext.FocusReleaseId,
				FocusTargetURL: tt.target,
			}
			if tt.withWorkspace {
				workspace := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
				options.SourceWorkspace = &workspace
				command.Attachments = nil
				command.ProjectId = "project-1"
			}
			rawOptions, err := json.Marshal(options)
			if err != nil {
				t.Fatalf("marshal bind runtime options: %v", err)
			}
			command.RuntimeOptionSnapshotJson = rawOptions
			decoded, err := decodeYakRuntimeOptions(command.GetRuntimeOptionSnapshotJson(), true)
			if err != nil {
				t.Fatalf("decode bind runtime options: %v", err)
			}
			// The producer carries the same server resource on both surfaces.
			if decoded.FocusTargetURL != tt.target || decoded.FocusTargetURL != command.ResultContext.GetTargetUrl() {
				t.Fatalf("runtime/result target mismatch: %q != %q", decoded.FocusTargetURL, command.ResultContext.GetTargetUrl())
			}
			if (decoded.SourceWorkspace != nil) != tt.withWorkspace {
				t.Fatalf("unexpected source workspace in bind runtime options")
			}
			if !tt.withWorkspace && strings.Contains(string(rawOptions), `"source_workspace"`) {
				t.Fatal("attachment-only runtime options must omit source_workspace, not fabricate one")
			}

			ref, err := validateLegionAIFocusResultContext(command.GetMetadata().GetCommandId(), command.GetResultContext())
			if err != nil {
				t.Fatalf("professional-task result context rejected before workspace pairing: %v", err)
			}
			if ref.CommandID != command.GetMetadata().GetCommandId() || ref.JobID != command.ResultContext.GetJob().GetJobId() {
				t.Fatalf("result context lost bind/job identity: %#v", ref)
			}
			err = validateAISessionBindCommand("node-ai", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("valid bind contract rejected: %v", err)
				}
			} else if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("expected bind rejection %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAISessionBindAttachmentBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aiv1.BindAISessionCommand, *yakRuntimeOptions)
	}{
		{"project", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.ProjectId = "project-1" }},
		{"missing_result_context", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.ResultContext = nil }},
		{"release_identity", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.ResultContext.FocusReleaseId = "another_focus@1.0.0+abcdef123456"
		}},
		{"conversation", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.ResultContext.FocusMode = legionAIConversationAuditResultMode
			c.ResultContext.FocusReleaseId = ""
			c.ResultContext.ExecutionMode = legionAIConversationExecutionMode
		}},
		{"runtime_target_mismatch", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			o.FocusTargetURL = "https://attachments.invalid/aicw_fedcba9876543210fedcba9876543210/"
		}},
		{"runtime_target_missing", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) { o.FocusTargetURL = "" }},
		{"runtime_focus_mismatch", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) { o.FocusModeLoop = "another_focus" }},
		{"runtime_release_mismatch", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) { o.FocusReleaseID += "0" }},
		{"runtime_sha_mismatch", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			o.FocusReleaseSHA256 = strings.Repeat("0", 64)
		}},
		{"runtime_name_mismatch", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			o.FocusRuntimeName = "legion_release_another_v1_000000000000"
		}},
		{"extra_mcp", func(_ *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			o.SessionMCPServers = []sessionMCPServer{{Name: "forbidden", URL: "https://mcp.invalid/sse"}}
		}},
		{"no_attachments", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.Attachments = nil }},
		{"nil_attachment", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.Attachments[0] = nil }},
		{"missing_id", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.Attachments[0].AttachmentId = "" }},
		{"missing_filename", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.Attachments[0].Filename = "" }},
		{"missing_digest", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.Attachments[0].Sha256 = "" }},
		{"nonhex_digest", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.Attachments[0].Sha256 = strings.Repeat("z", 64)
		}},
		{"oversized", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.Attachments[0].SizeBytes = maxAISessionAttachmentBytes + 1
		}},
		{"duplicate", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.Attachments = append(c.Attachments, c.Attachments[0])
		}},
		{"too_many", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			for i := 1; i < 6; i++ {
				a := *c.Attachments[0]
				a.AttachmentId = fmt.Sprintf("attachment-%d", i)
				c.Attachments = append(c.Attachments, &a)
			}
		}},
		{"total_size", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.Attachments[0].SizeBytes = maxAISessionAttachmentBytes
			for i := 1; i < 5; i++ {
				a := *c.Attachments[0]
				a.AttachmentId = fmt.Sprintf("attachment-%d", i)
				c.Attachments = append(c.Attachments, &a)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := testAttachmentTaskBindCommand(t)
			options, err := decodeYakRuntimeOptions(command.RuntimeOptionSnapshotJson, true)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(command, &options)
			setAttachmentTaskRuntimeOptions(t, command, options)
			if err := validateAISessionBindCommand("node-ai", command); err == nil {
				t.Fatal("unsafe attachment bind accepted")
			}
		})
	}
}

func TestValidateLegionAIFocusAttachmentResultContextBoundaries(t *testing.T) {
	for _, target := range []string{
		"http://attachments.invalid/" + testLegionCodeWorkspaceID + "/",
		"https://attachments.invalid/",
		"https://attachments.invalid/not-a-resource/",
		"https://attachments.invalid/" + testLegionCodeWorkspaceID + "/child",
		testAttachmentTaskTarget + "?query=1",
		testAttachmentTaskTarget + "#fragment",
		"https://attachments.invalid:444/" + testLegionCodeWorkspaceID + "/",
		"https://attachments.invalid/%61icw_0123456789abcdef0123456789abcdef/",
		"https://attachments.invalid/aicw_0123456789ABCDEF0123456789ABCDEF/",
	} {
		t.Run(target, func(t *testing.T) {
			result := testAttachmentTaskResultContext()
			result.TargetUrl = target
			if _, err := validateLegionAIFocusResultContext("bind-attachment", result); err == nil {
				t.Fatal("malformed attachment resource target accepted")
			}
		})
	}
}

func TestAttachmentTaskRuntimeManagerKeepsInputsImmutable(t *testing.T) {
	for _, attachmentTask := range []bool{true, false} {
		for _, inputKind := range []string{"hotpatch", "context", "message", "sync_event", "cancel", "close"} {
			t.Run(fmt.Sprintf("attachment_%t/%s", attachmentTask, inputKind), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				manager := newAISessionRuntimeManager(noopAISessionRuntimeDriver{})
				command := validAISessionBindCommand()
				var bindOptions aiSessionRuntimeBindOptions
				if attachmentTask {
					command = testAttachmentTaskBindCommand(t)
					sink, err := newLegionAIFocusResultSink(&recordingAIFocusRiskPublisher{}, command.Metadata.CommandId, command.ResultContext)
					if err != nil {
						t.Fatal(err)
					}
					bindOptions.ResultSink = sink
				}
				if _, err := manager.Bind(ctx, command, nil, bindOptions); err != nil {
					t.Fatal(err)
				}
				var err error
				switch inputKind {
				case "context":
					update := validAISessionContextCommand()
					update.Attachments = testAttachmentTaskBindCommand(t).Attachments
					_, err = manager.AcceptContextUpdate(update)
				case "cancel":
					_, err = manager.Cancel(validAISessionCancelCommand())
				case "close":
					_, err = manager.Close(validAISessionCloseCommand())
				default:
					input := validAISessionInputCommand()
					input.InputType = inputKind
					if inputKind == "hotpatch" {
						input.InputJson = []byte(`{"hotpatch_type":"EnabledCapabilities","params":{"enabled_capabilities":[{"name":"forbidden","type":"mcp"}]}}`)
					}
					if inputKind == "sync_event" {
						input.InputJson = []byte(`{"sync_type":"queue_info","sync_json_input":{}}`)
					}
					if inputKind == "message" && attachmentTask {
						input.ContextPackage = &aiv1.ContextPackage{UserInput: "Analyze the pinned logs", FocusRelease: testAttachmentTaskFocusRelease()}
					}
					_, err = manager.AcceptInput(input)
				}
				mustReject := attachmentTask && (inputKind == "hotpatch" || inputKind == "context")
				if mustReject {
					if err == nil || !strings.Contains(err.Error(), "attachment") {
						t.Fatalf("attachment input expansion accepted: %v", err)
					}
				} else if err != nil {
					t.Fatalf("existing %s behavior rejected: %v", inputKind, err)
				}
			})
		}
	}
}

type attachmentBindReplayFixture struct {
	ctx       context.Context
	command   *aiv1.BindAISessionCommand
	manager   *aiSessionRuntimeManager
	runtime   *legionServerFocusRuntime
	proxy     *aiSessionResultSinkProxy
	publisher *recordingAIFocusRiskPublisher
	driver    *recordingAISessionRuntimeDriver
}

func newAttachmentBindReplayFixture(t *testing.T) *attachmentBindReplayFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	command := testAttachmentTaskBindCommand(t)
	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(publisher, command.Metadata.CommandId, command.ResultContext)
	if err != nil {
		t.Fatal(err)
	}
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	if _, err := manager.Bind(ctx, command, nil, aiSessionRuntimeBindOptions{ResultSink: sink}); err != nil {
		t.Fatal(err)
	}
	runtime := driver.bindings[0].LegionResultRuntime.(*legionServerFocusRuntime)
	if err := runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, testAttachmentTaskExecutionContract(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.deactivateFocusTurn(command.ResultContext.FocusReleaseId) })
	return &attachmentBindReplayFixture{ctx: ctx, command: command, manager: manager, runtime: runtime,
		proxy: runtime.sink.(*aiSessionResultSinkProxy), publisher: publisher, driver: driver}
}

func (f *attachmentBindReplayFixture) report(t *testing.T) {
	t.Helper()
	if _, err := f.runtime.Execute(serverFocusCapabilitySubmitReportV1, map[string]any{
		"markdown": "# Synthetic replay report", "structured_summary": map[string]any{"retry_count": 1},
	}); err != nil {
		t.Fatalf("publish attachment report: %v", err)
	}
}

func (f *attachmentBindReplayFixture) bind(t *testing.T, command *aiv1.BindAISessionCommand) (*recordingAIFocusRiskPublisher, error) {
	t.Helper()
	if err := validateAISessionBindCommand("node-ai", command); err != nil {
		t.Fatalf("replay fixture must be independently valid: %v", err)
	}
	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(publisher, command.Metadata.CommandId, command.ResultContext)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.manager.Bind(f.ctx, command, nil, aiSessionRuntimeBindOptions{ResultSink: sink})
	return publisher, err
}

func TestAttachmentTaskBindReplayPreservesResultState(t *testing.T) {
	for _, afterReport := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_report_%t", afterReport), func(t *testing.T) {
			fixture := newAttachmentBindReplayFixture(t)
			if afterReport {
				fixture.report(t)
			}
			replay := proto.Clone(fixture.command).(*aiv1.BindAISessionCommand)
			replayedPublisher, err := fixture.bind(t, replay)
			if err != nil {
				t.Fatalf("exact attachment Bind replay rejected: %v", err)
			}
			if !afterReport {
				if err := fixture.proxy.Succeed(fixture.ctx, nil); err == nil || !strings.Contains(err.Error(), "required results") {
					t.Errorf("Bind replay lost the required report contract: %v", err)
				}
				fixture.report(t)
			}
			if err := fixture.proxy.Succeed(fixture.ctx, nil); err != nil {
				t.Fatalf("Bind replay lost published result state: %v", err)
			}
			if len(fixture.publisher.reports) != 2 || fixture.publisher.reportKinds[0] != "ai_log_analysis_v1" || fixture.publisher.succeeded != 1 {
				t.Fatalf("original result publisher lost report/completion state: kinds=%v succeeded=%d", fixture.publisher.reportKinds, fixture.publisher.succeeded)
			}
			if len(replayedPublisher.reports) != 0 || replayedPublisher.succeeded != 0 {
				t.Fatal("exact replay replaced the active result sink")
			}
			fixture.driver.mu.Lock()
			bindCount := len(fixture.driver.bindings)
			fixture.driver.mu.Unlock()
			if bindCount != 1 {
				t.Fatalf("exact replay created another runtime: %d", bindCount)
			}
		})
	}
}

func TestAttachmentTaskBindReplayRejectsChangedIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aiv1.BindAISessionCommand, *yakRuntimeOptions)
	}{
		{"resource", func(c *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			c.ResultContext.TargetUrl = "https://attachments.invalid/aicw_fedcba9876543210fedcba9876543210/"
			o.FocusTargetURL = c.ResultContext.TargetUrl
		}},
		{"release", func(c *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			c.ResultContext.FocusReleaseId = "log_analysis@1.1.1+abcdef123456"
			o.FocusReleaseID, o.FocusReleaseSHA256, o.FocusRuntimeName = c.ResultContext.FocusReleaseId, "", ""
		}},
		{"job", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.ResultContext.Job.JobId = "other-job" }},
		{"subtask", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.ResultContext.Job.SubtaskId = "other-subtask"
		}},
		{"attempt", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.ResultContext.Job.AttemptId = "other-attempt"
		}},
		{"focus_run", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.ResultContext.FocusRunId, c.Session.RunId = "other-focus-run", "other-focus-run"
		}},
		{"attachment_pin", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) {
			c.Attachments[0].Sha256 = strings.Repeat("0", 64)
		}},
		{"owner", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.OwnerUserId = "other-owner" }},
		{"bind_epoch", func(c *aiv1.BindAISessionCommand, _ *yakRuntimeOptions) { c.BindEpoch++ }},
		{"ordinary_transition", func(c *aiv1.BindAISessionCommand, o *yakRuntimeOptions) {
			c.ResultContext = validAIFocusResultContext()
			c.Session.RunId = c.ResultContext.FocusRunId
			*o = yakRuntimeOptions{}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAttachmentBindReplayFixture(t)
			replay := proto.Clone(fixture.command).(*aiv1.BindAISessionCommand)
			options, err := decodeYakRuntimeOptions(replay.RuntimeOptionSnapshotJson, true)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(replay, &options)
			setAttachmentTaskRuntimeOptions(t, replay, options)
			replayedPublisher, err := fixture.bind(t, replay)
			if !errors.Is(err, errAISessionBindFenced) {
				t.Errorf("changed attachment Bind must be fenced without retiring the original runtime: %v", err)
			}
			fixture.report(t)
			if err := fixture.proxy.Succeed(fixture.ctx, nil); err != nil {
				t.Fatalf("rejected replay damaged the original binding: %v", err)
			}
			if fixture.publisher.succeeded != 1 || len(replayedPublisher.reports) != 0 || replayedPublisher.succeeded != 0 {
				t.Fatal("rejected replay changed the authorized result sink")
			}
		})
	}
	t.Run("ordinary_to_attachment", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		command := validAISessionBindCommand()
		command.ResultContext = validAIFocusResultContext()
		command.Session.RunId = command.ResultContext.FocusRunId
		publisher := &recordingAIFocusRiskPublisher{}
		sink, err := newLegionAIFocusResultSink(publisher, command.Metadata.CommandId, command.ResultContext)
		if err != nil {
			t.Fatal(err)
		}
		driver := &recordingAISessionRuntimeDriver{}
		manager := newAISessionRuntimeManager(driver)
		if _, err := manager.Bind(ctx, command, nil, aiSessionRuntimeBindOptions{ResultSink: sink}); err != nil {
			t.Fatal(err)
		}
		replay := testAttachmentTaskBindCommand(t)
		replaySink, err := newLegionAIFocusResultSink(&recordingAIFocusRiskPublisher{}, replay.Metadata.CommandId, replay.ResultContext)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Bind(ctx, replay, nil, aiSessionRuntimeBindOptions{ResultSink: replaySink}); !errors.Is(err, errAISessionBindFenced) {
			t.Errorf("ordinary-to-attachment replay must be fenced without retiring the original runtime: %v", err)
		}
		runtime := driver.bindings[0].LegionResultRuntime.(*legionServerFocusRuntime)
		if _, err := runtime.Execute(serverFocusCapabilitySubmitAsset, map[string]any{
			"kind": "http_endpoint", "title": "Synthetic ordinary result", "target": command.ResultContext.TargetUrl,
			"identity_key": "synthetic:ordinary", "payload": map[string]any{"status": "observed"},
		}); err != nil {
			t.Fatalf("rejected upgrade damaged ordinary binding behavior: %v", err)
		}
		if len(publisher.assets) != 1 {
			t.Fatal("ordinary result sink was replaced")
		}
	})
}

func TestAttachmentTaskBindReplayConcurrent(t *testing.T) {
	fixture := newAttachmentBindReplayFixture(t)
	fixture.report(t)
	const replayCount = 8
	var work sync.WaitGroup
	start := make(chan struct{})
	errors := make(chan error, replayCount)
	publishers := make([]*recordingAIFocusRiskPublisher, replayCount)
	for index := 0; index < replayCount; index++ {
		command := proto.Clone(fixture.command).(*aiv1.BindAISessionCommand)
		publisher := &recordingAIFocusRiskPublisher{}
		publishers[index] = publisher
		sink, err := newLegionAIFocusResultSink(publisher, command.Metadata.CommandId, command.ResultContext)
		if err != nil {
			t.Fatal(err)
		}
		work.Add(1)
		go func() {
			defer work.Done()
			<-start
			_, err := fixture.manager.Bind(fixture.ctx, command, nil, aiSessionRuntimeBindOptions{ResultSink: sink})
			errors <- err
		}()
	}
	close(start)
	work.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Errorf("concurrent exact replay rejected: %v", err)
		}
	}
	if err := fixture.proxy.Succeed(fixture.ctx, nil); err != nil {
		t.Fatalf("concurrent replay lost completed report: %v", err)
	}
	if fixture.publisher.succeeded != 1 || len(fixture.publisher.reports) != 2 {
		t.Fatal("concurrent replay replaced original report state")
	}
	for _, publisher := range publishers {
		if publisher.succeeded != 0 || len(publisher.reports) != 0 {
			t.Fatal("concurrent replay published through a new sink")
		}
	}
	fixture.driver.mu.Lock()
	bindCount := len(fixture.driver.bindings)
	fixture.driver.mu.Unlock()
	if bindCount != 1 {
		t.Fatalf("concurrent replay constructed %d runtimes", bindCount)
	}
}

func TestAttachmentTaskBindReplayRetainsFencing(t *testing.T) {
	for _, boundary := range []string{"cancel", "close", "same_epoch", "new_epoch"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newAttachmentBindReplayFixture(t)
			replay := proto.Clone(fixture.command).(*aiv1.BindAISessionCommand)
			switch boundary {
			case "cancel":
				if _, err := fixture.manager.Cancel(validAISessionCancelCommand()); err != nil {
					t.Fatal(err)
				}
			case "close":
				if _, err := fixture.manager.Close(validAISessionCloseCommand()); err != nil {
					t.Fatal(err)
				}
			case "same_epoch":
				replay.Metadata.CommandId = "new-bind"
			case "new_epoch":
				replay.Metadata.CommandId, replay.BindEpoch = "new-bind", replay.BindEpoch+1
			}
			_, err := fixture.bind(t, replay)
			if boundary == "new_epoch" {
				if err != nil {
					t.Fatalf("fresh higher-epoch binding rejected: %v", err)
				}
				fixture.manager.mu.Lock()
				active := fixture.manager.sessions[replay.Session.SessionId]
				fixture.manager.mu.Unlock()
				if active == nil || active.resultSink == fixture.proxy {
					t.Fatal("fresh bind reused a retired sink")
				}
			} else if err == nil {
				t.Fatalf("%s fencing was bypassed by Bind replay", boundary)
			}
		})
	}
}
