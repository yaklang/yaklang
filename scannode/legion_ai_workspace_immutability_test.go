package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

func TestAttachmentWorkspaceBindKeepsInputsAndReplayImmutable(t *testing.T) {
	command := testAttachmentTaskBindCommand(t)
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindAttachments)
	target, err := legionCodeWorkspaceSentinel(spec.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	options, err := decodeYakRuntimeOptions(command.RuntimeOptionSnapshotJson, true)
	if err != nil {
		t.Fatal(err)
	}
	options.SourceWorkspace = &spec
	options.FocusTargetURL = target
	options.AITaskKey = "log_analysis"
	options.AITaskRunID = "aitr_" + strings.TrimPrefix(spec.WorkspaceID, "aicw_")
	options.AITaskVersion = "1.0.0"
	options.AITaskDefinitionChecksum = strings.Repeat("a", 64)
	options.AITaskSessionRole = legionAITaskExecutionRole
	options.FocusReleaseID = "log_analysis@1.0.0+" + options.FocusReleaseSHA256[:12]
	options.FocusRuntimeName = "legion_release_log_analysis_1_0_0_" + options.FocusReleaseSHA256[:12]
	command.ResultContext.TargetUrl = target
	command.ResultContext.FocusReleaseId = options.FocusReleaseID
	command.RuntimeOptionSnapshotJson, _ = json.Marshal(options)
	if err := validateAISessionBindCommand(command.TargetNodeId, command); err != nil {
		t.Fatal(err)
	}
	sink, err := newLegionAIFocusResultSink(&recordingAIFocusRiskPublisher{}, command.Metadata.CommandId, command.ResultContext)
	if err != nil {
		t.Fatal(err)
	}
	binding := testAttachmentTaskBinding(t, testAttachmentTaskContent)
	bindOptions := aiSessionRuntimeBindOptions{ResultSink: sink, HTTPClient: binding.HTTPClient,
		PlatformAPIBaseURL: binding.PlatformAPIBaseURL, NodeSessionID: binding.NodeSessionID, PlatformBearerToken: binding.PlatformBearerToken}
	manager := newAISessionRuntimeManager(noopAISessionRuntimeDriver{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := manager.Bind(ctx, command, nil, bindOptions); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = manager.Close(validAISessionCloseCommand()) })
	runtime := manager.sessions[command.Session.SessionId]
	if !runtime.attachmentTask {
		t.Fatal("workspace escaped immutable attachment classification")
	}
	if _, err := manager.AcceptContextUpdate(validAISessionContextCommand()); err == nil {
		t.Fatal("workspace accepted mutable context")
	}
	input := validAISessionInputCommand()
	input.InputType = "hotpatch"
	if _, err := manager.AcceptInput(input); err == nil {
		t.Fatal("workspace accepted a hotpatch")
	}
	originalSink := runtime.resultSink
	if _, err := manager.Bind(ctx, command, nil, bindOptions); err != nil {
		t.Fatalf("identical replay: %v", err)
	}
	if runtime.resultSink != originalSink {
		t.Fatal("replay replaced result accounting")
	}
	changed := proto.Clone(command).(*aiv1.BindAISessionCommand)
	changed.Attachments[0].Sha256 = strings.Repeat("0", 64)
	if _, err := manager.Bind(ctx, changed, nil, bindOptions); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("changed replay: %v", err)
	}
}

func TestPendingBindRedeliveryRequiresRetry(t *testing.T) {
	command := validAISessionBindCommand()
	ref := aiSessionRefFromBindCommand(command)
	manager := newAISessionRuntimeManager(noopAISessionRuntimeDriver{})
	manager.bindings[ref.SessionID] = aiSessionBindReservation{ref: ref, cancel: func() {}}
	if _, err := manager.Bind(context.Background(), command, nil, aiSessionRuntimeBindOptions{}); !errors.Is(err, errAISessionBindRetry) {
		t.Fatalf("same pending command must retry, not acknowledge: %v", err)
	}
	changed := proto.Clone(command).(*aiv1.BindAISessionCommand)
	changed.Metadata.CommandId = "different-command"
	if _, err := manager.Bind(context.Background(), changed, nil, aiSessionRuntimeBindOptions{}); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("different stale command must remain fenced: %v", err)
	}
}
