package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestAISessionRuntimeManagerRebindReplacesHandleAndDuplicateDoesNot(t *testing.T) {
	bridge, _, driver := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	first.RuntimeOptionSnapshotJson = []byte(`{"enable_plan":false}`)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("duplicate bind: %v", err)
	}
	driver.mu.Lock()
	duplicateBindCount := len(driver.bindings)
	driver.mu.Unlock()
	if duplicateBindCount != 1 {
		t.Fatalf("duplicate bind created %d handles, want 1", duplicateBindCount)
	}

	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-2"
	rebind.BindEpoch = first.BindEpoch + 1
	rebind.RuntimeOptionSnapshotJson = []byte(`{"enable_plan":true}`)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("replacement bind: %v", err)
	}
	driver.mu.Lock()
	if len(driver.bindings) != 2 {
		driver.mu.Unlock()
		t.Fatalf("replacement bind created %d handles, want 2", len(driver.bindings))
	}
	if string(driver.bindings[1].RuntimeOptionSnapshotJSON) != `{"enable_plan":true}` {
		driver.mu.Unlock()
		t.Fatalf("replacement runtime options = %s", driver.bindings[1].RuntimeOptionSnapshotJSON)
	}
	driver.mu.Unlock()
	driver.assertClose(t, 0, "runtime rebind")
}

func TestAISessionRuntimeManagerFailedRebindKeepsPreviousRuntime(t *testing.T) {
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{
		recorder: recorder,
		failOn:   2,
	}
	manager := newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}

	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-fails"
	rebind.BindEpoch = first.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), rebind, nil, aiSessionRuntimeBindOptions{}); err == nil {
		t.Fatal("expected replacement bind failure")
	}

	input := validAISessionInputCommand()
	input.Metadata.CommandId = "cmd-input-after-failed-rebind"
	accepted, err := manager.AcceptInput(input)
	if err != nil {
		t.Fatalf("old runtime rejected input after failed rebind: %v", err)
	}
	if accepted.ref.BindEpoch != first.BindEpoch {
		t.Fatalf("accepted bind epoch = %d, want %d", accepted.ref.BindEpoch, first.BindEpoch)
	}
	if err := accepted.handle.SendInput(context.Background(), aiSessionInput{
		Ref:         accepted.ref,
		InputType:   accepted.inputType,
		PayloadJSON: accepted.payloadJSON,
	}); err != nil {
		t.Fatalf("old runtime input after failed rebind: %v", err)
	}
	manager.CompleteInput(accepted, true)
	recorder.assertInput(t, 0, "hello")
}

func TestAISessionRuntimeManagerFencesProcessedInputReplayAcrossRebind(t *testing.T) {
	recorder := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(recorder)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	input := validAISessionInputCommand()
	accepted, err := manager.AcceptInput(input)
	if err != nil {
		t.Fatalf("accept first input: %v", err)
	}
	if err := accepted.handle.SendInput(context.Background(), aiSessionInput{
		Ref:         accepted.ref,
		InputType:   accepted.inputType,
		PayloadJSON: accepted.payloadJSON,
	}); err != nil {
		t.Fatalf("execute first input: %v", err)
	}

	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-after-input"
	rebind.BindEpoch = first.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), rebind, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("replacement bind: %v", err)
	}
	// The old handle can finish after the swap. Completion must update the
	// Session-wide command ledger inherited by the new generation.
	manager.CompleteInput(accepted, true)
	if _, err := manager.AcceptInput(input); err == nil {
		t.Fatal("processed pre-rebind command bypassed the new bind epoch fence")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.inputs) != 1 {
		t.Fatalf("input executed %d times, want once", len(recorder.inputs))
	}
}

func TestAISessionRuntimeManagerRejectsMissingEpochOnModernRuntime(t *testing.T) {
	manager := newAISessionRuntimeManager(&recordingAISessionRuntimeDriver{})
	if _, err := manager.Bind(context.Background(), validAISessionBindCommand(), nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("bind runtime: %v", err)
	}
	input := validAISessionInputCommand()
	input.Session.BindEpoch = 0
	if _, err := manager.AcceptInput(input); err == nil {
		t.Fatal("modern runtime accepted an input without bind epoch")
	}
	contextCommand := validAISessionContextCommand()
	contextCommand.Session.BindEpoch = 0
	if _, err := manager.AcceptContextUpdate(contextCommand); err == nil {
		t.Fatal("modern runtime accepted context without bind epoch")
	}
	cancelCommand := validAISessionCancelCommand()
	cancelCommand.Session.BindEpoch = 0
	if _, err := manager.Cancel(cancelCommand); err == nil {
		t.Fatal("modern runtime accepted cancel without bind epoch")
	}
	closeCommand := validAISessionCloseCommand()
	closeCommand.Session.BindEpoch = 0
	if _, err := manager.Close(closeCommand); err == nil {
		t.Fatal("modern runtime accepted close without bind epoch")
	}
}

func TestAISessionRuntimeManagerHigherPendingBindEpochWins(t *testing.T) {
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{
		recorder: recorder,
		blockOn:  2,
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	manager := newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}

	second := proto.Clone(first).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "cmd-bind-2"
	second.BindEpoch = first.BindEpoch + 1
	secondDone := make(chan error, 1)
	go func() {
		_, err := manager.Bind(context.Background(), second, nil, aiSessionRuntimeBindOptions{})
		secondDone <- err
	}()
	select {
	case <-driver.started:
	case <-time.After(time.Second):
		t.Fatal("second bind did not reach the controlled barrier")
	}

	third := proto.Clone(first).(*aiv1.BindAISessionCommand)
	third.Metadata.CommandId = "cmd-bind-3"
	third.BindEpoch = second.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), third, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("higher pending bind: %v", err)
	}
	close(driver.release)
	if err := <-secondDone; !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("superseded pending bind error = %v, want fenced", err)
	}

	manager.mu.Lock()
	active := manager.sessions[first.GetSession().GetSessionId()]
	manager.mu.Unlock()
	if active == nil || active.bindEpoch != third.BindEpoch {
		t.Fatalf("active runtime = %#v, want bind epoch %d", active, third.BindEpoch)
	}
}

func TestAISessionRuntimeManagerDuplicateBindRemainsIdempotentAfterInput(t *testing.T) {
	bridge, _, driver := newTestAISessionBridge(t)
	bind := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, bind)); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("input: %v", err)
	}
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, bind)); err != nil {
		t.Fatalf("redelivered bind: %v", err)
	}
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if len(driver.bindings) != 1 {
		t.Fatalf("redelivered original bind created %d handles, want 1", len(driver.bindings))
	}
}

func TestAISessionRuntimeManagerRejectsLateOlderBindAfterRebind(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second := proto.Clone(first).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "cmd-bind-2"
	second.BindEpoch = first.BindEpoch + 1
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, second)); err != nil {
		t.Fatalf("replacement bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("stale bind should be a transport-successful no-op: %v", err)
	}
	driver.mu.Lock()
	if len(driver.bindings) != 2 {
		driver.mu.Unlock()
		t.Fatalf("late stale bind created %d handles, want 2 total", len(driver.bindings))
	}
	driver.mu.Unlock()
	driver.assertClose(t, 0, "runtime rebind")
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	if len(fakeJS.publish) != 0 {
		t.Fatalf("stale bind published %d events and could terminate the active generation", len(fakeJS.publish))
	}
}

func TestAISessionRuntimeManagerDropsLateEventsFromRetiredRuntime(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second := proto.Clone(first).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "cmd-bind-2"
	second.BindEpoch = first.BindEpoch + 1
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, second)); err != nil {
		t.Fatalf("replacement bind: %v", err)
	}
	driver.mu.Lock()
	oldEmitter := driver.emitters[0]
	driver.mu.Unlock()
	resetPublishedMessages(fakeJS)
	oldEmitter.Emit("ai.session.timeline", []byte(`{"late":true}`))
	oldEmitter.Failed("old_runtime_failed", "late failure", nil)
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	if len(fakeJS.publish) != 0 {
		t.Fatalf("retired runtime published %d late events", len(fakeJS.publish))
	}
}

func TestAISessionRuntimeRebindDrainDoesNotBlockOtherSessions(t *testing.T) {
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("bind first session: %v", err)
	}
	secondSession := proto.Clone(first).(*aiv1.BindAISessionCommand)
	secondSession.Metadata.CommandId = "cmd-bind-other"
	secondSession.Session.SessionId = "ai-session-other"
	if _, err := manager.Bind(context.Background(), secondSession, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("bind other session: %v", err)
	}

	manager.mu.Lock()
	oldRuntime := manager.sessions[first.GetSession().GetSessionId()]
	manager.mu.Unlock()
	if oldRuntime == nil || !oldRuntime.beginEmission() {
		t.Fatal("failed to hold an in-flight emission")
	}
	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-replacement"
	rebind.BindEpoch = first.BindEpoch + 1
	rebindDone := make(chan error, 1)
	go func() {
		_, err := manager.Bind(context.Background(), rebind, nil, aiSessionRuntimeBindOptions{})
		rebindDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		_, draining := manager.bindings[first.GetSession().GetSessionId()]
		manager.mu.Unlock()
		if draining {
			break
		}
		time.Sleep(time.Millisecond)
	}
	otherInput := validAISessionInputCommand()
	otherInput.Metadata.CommandId = "cmd-input-other"
	otherInput.Session.SessionId = "ai-session-other"
	if _, err := manager.AcceptInput(otherInput); err != nil {
		t.Fatalf("other session input was blocked by rebind drain: %v", err)
	}
	oldRuntime.endEmission()
	select {
	case err := <-rebindDone:
		if err != nil {
			t.Fatalf("rebind after drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("rebind did not finish after emission drained")
	}
}

func TestAISessionRootTerminalEventFencesLaterRebind(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	first.ResultContext = validAIFocusResultContext()
	first.ResultContext.ExecutionMode = "single_run"
	first.Session.RunId = first.ResultContext.FocusRunId
	if _, err := bridge.aiRuntime.Bind(
		context.Background(),
		first,
		bridge.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{},
	); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	resetPublishedMessages(fakeJS)
	emitter.Emit("ai.session.event", []byte(`{"type":"end_plan_and_execution","task_index":"","content_json":{}}`))
	if hasAISessionRuntime(bridge.aiRuntime, first.GetSession().GetSessionId()) {
		t.Fatal("root terminal event did not retire runtime")
	}
	resetPublishedMessages(fakeJS)
	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-after-terminal"
	rebind.BindEpoch = first.BindEpoch + 1
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("post-terminal rebind should be a transport no-op: %v", err)
	}
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	if len(fakeJS.publish) != 0 {
		t.Fatalf("post-terminal rebind published %d events", len(fakeJS.publish))
	}
}

func TestAISessionRootTerminalCompletesFocusResult(t *testing.T) {
	bridge, _, driver := newTestAISessionBridge(t)
	sink := &recordingLifecycleAIFocusResultSink{}
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.ExecutionMode = "single_run"
	command.Session.RunId = command.ResultContext.FocusRunId
	if _, err := bridge.aiRuntime.Bind(
		context.Background(),
		command,
		bridge.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{ResultSink: sink},
	); err != nil {
		t.Fatalf("bind runtime: %v", err)
	}
	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	payload := []byte(`{"type":"end_plan_and_execution","task_index":"","content_json":{}}`)
	emitter.Emit("ai.session.event", payload)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.succeeded) != 1 || string(sink.succeeded[0]) != string(payload) {
		t.Fatalf("focus success payloads = %q", sink.succeeded)
	}
}

func TestAISessionRootTerminalPublishTimeoutAllowsHigherEpochRebind(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	oldTimeout := aiSessionTerminalPublishTimeout
	aiSessionTerminalPublishTimeout = 20 * time.Millisecond
	defer func() { aiSessionTerminalPublishTimeout = oldTimeout }()

	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.ExecutionMode = "single_run"
	command.Session.RunId = command.ResultContext.FocusRunId
	if _, err := bridge.aiRuntime.Bind(
		context.Background(),
		command,
		bridge.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{},
	); err != nil {
		t.Fatalf("bind runtime: %v", err)
	}
	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	fakeJS.failNextPublishes(1000)
	emitter.Emit("ai.session.event", []byte(`{"type":"end_plan_and_execution","task_index":"","content_json":{}}`))

	rebind := proto.Clone(command).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-recover-terminal-publish"
	rebind.BindEpoch = command.BindEpoch + 1
	if _, err := bridge.aiRuntime.Bind(
		context.Background(),
		rebind,
		nil,
		aiSessionRuntimeBindOptions{},
	); err != nil {
		t.Fatalf("higher epoch rebind after terminal publish timeout: %v", err)
	}
}

func TestAISessionRootTerminalPublicationNAKsRecoveryBindUntilClaimSettles(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	oldTimeout := aiSessionTerminalPublishTimeout
	aiSessionTerminalPublishTimeout = 80 * time.Millisecond
	defer func() { aiSessionTerminalPublishTimeout = oldTimeout }()

	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.ExecutionMode = "single_run"
	command.Session.RunId = command.ResultContext.FocusRunId
	if _, err := bridge.aiRuntime.Bind(
		context.Background(),
		command,
		bridge.ensureAIPublisher(),
		aiSessionRuntimeBindOptions{},
	); err != nil {
		t.Fatalf("bind runtime: %v", err)
	}
	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	fakeJS.failNextPublishes(1000)
	emitDone := make(chan struct{})
	go func() {
		emitter.Emit("ai.session.event", []byte(`{"type":"end_plan_and_execution","task_index":"","content_json":{}}`))
		close(emitDone)
	}()

	claimed := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		bridge.aiRuntime.mu.Lock()
		runtime := bridge.aiRuntime.sessions[command.GetSession().GetSessionId()]
		bridge.aiRuntime.mu.Unlock()
		if runtime != nil {
			runtime.mu.Lock()
			claiming := runtime.terminalCommandID != ""
			runtime.mu.Unlock()
			if claiming {
				claimed = true
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if !claimed {
		t.Fatal("terminal publication did not claim the runtime")
	}

	rebind := proto.Clone(command).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-during-terminal-publish"
	rebind.BindEpoch = command.BindEpoch + 1
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); !errors.Is(err, errAISessionBindRetry) {
		t.Fatalf("recovery bind during terminal publish = %v, want retry", err)
	}

	select {
	case <-emitDone:
	case <-time.After(time.Second):
		t.Fatal("terminal publication did not time out")
	}
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); !errors.Is(err, errAISessionBindRetry) {
		t.Fatalf("original bind for failed terminal runtime = %v, want retry", err)
	}
	fakeJS.failNextPublishes(0)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("higher epoch recovery bind after timeout: %v", err)
	}
}

func TestAISessionBindFailureRetirementDefersToHigherPendingEpoch(t *testing.T) {
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{
		recorder: recorder,
		failOn:   2,
		blockOn:  3,
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	manager := newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second := proto.Clone(first).(*aiv1.BindAISessionCommand)
	second.Metadata.CommandId = "cmd-bind-failed-epoch-2"
	second.BindEpoch = first.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), second, nil, aiSessionRuntimeBindOptions{}); err == nil {
		t.Fatal("expected epoch 2 bind failure")
	}
	third := proto.Clone(first).(*aiv1.BindAISessionCommand)
	third.Metadata.CommandId = "cmd-bind-pending-epoch-3"
	third.BindEpoch = second.BindEpoch + 1
	thirdDone := make(chan error, 1)
	go func() {
		_, err := manager.Bind(context.Background(), third, nil, aiSessionRuntimeBindOptions{})
		thirdDone <- err
	}()
	select {
	case <-driver.started:
	case <-time.After(time.Second):
		t.Fatal("higher epoch bind did not reserve the session")
	}
	manager.RetireAfterBindFailure(aiSessionRefFromBindCommand(second))
	if !hasAISessionRuntime(manager, first.GetSession().GetSessionId()) {
		t.Fatal("stale bind failure retired the runtime while a higher epoch was pending")
	}
	close(driver.release)
	if err := <-thirdDone; err != nil {
		t.Fatalf("higher epoch bind: %v", err)
	}
	manager.mu.Lock()
	active := manager.sessions[first.GetSession().GetSessionId()]
	manager.mu.Unlock()
	if active == nil || active.bindEpoch != third.BindEpoch {
		t.Fatalf("active runtime = %#v, want bind epoch %d", active, third.BindEpoch)
	}
}

func TestAISessionBindFailureTombstoneAllowsStrictlyHigherEpochRecovery(t *testing.T) {
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{recorder: recorder, failOn: 2}
	manager := newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if _, err := manager.Bind(context.Background(), first, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	failed := proto.Clone(first).(*aiv1.BindAISessionCommand)
	failed.Metadata.CommandId = "cmd-bind-failed-epoch-2"
	failed.BindEpoch = first.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), failed, nil, aiSessionRuntimeBindOptions{}); err == nil {
		t.Fatal("expected replacement bind failure")
	}
	manager.RetireAfterBindFailure(aiSessionRefFromBindCommand(failed))
	if hasAISessionRuntime(manager, first.GetSession().GetSessionId()) {
		t.Fatal("failed replacement did not retire the previous runtime")
	}
	if _, err := manager.Bind(context.Background(), failed, nil, aiSessionRuntimeBindOptions{}); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("same failed epoch = %v, want fenced", err)
	}
	recovery := proto.Clone(first).(*aiv1.BindAISessionCommand)
	recovery.Metadata.CommandId = "cmd-bind-recovery-epoch-3"
	recovery.BindEpoch = failed.BindEpoch + 1
	if _, err := manager.Bind(context.Background(), recovery, nil, aiSessionRuntimeBindOptions{}); err != nil {
		t.Fatalf("strictly higher recovery bind: %v", err)
	}
}

func TestAISessionFailedReplacementBindRetiresPreviousRuntimeAfterTerminalEvent(t *testing.T) {
	bridge, fakeJS, _ := newTestAISessionBridge(t)
	recorder := &recordingAISessionRuntimeDriver{}
	driver := &controlledAISessionRuntimeDriver{recorder: recorder, failOn: 2}
	bridge.aiRuntime = newAISessionRuntimeManager(driver)
	first := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	rebind := proto.Clone(first).(*aiv1.BindAISessionCommand)
	rebind.Metadata.CommandId = "cmd-bind-terminal-failure"
	rebind.BindEpoch = first.BindEpoch + 1
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("replacement bind failure publication: %v", err)
	}
	if hasAISessionRuntime(bridge.aiRuntime, first.GetSession().GetSessionId()) {
		t.Fatal("old runtime survived a terminal replacement bind failure")
	}
	recorder.assertClose(t, 0, "replacement runtime bind failed")
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.failed", 1)
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

func TestValidateAISessionContextCommandRequiresRefs(t *testing.T) {
	t.Parallel()

	command := validAISessionContextCommand()
	command.Attachments = nil
	command.CredentialRefs = nil

	err := validateAISessionContextCommand(command)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if err.Error() != "ai session context attachments or credential_refs are required" {
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

func TestHandleAISessionHotpatchPayloadIsAcceptedByRuntimeDecoder(t *testing.T) {
	t.Parallel()

	bridge, _, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	command := validAISessionInputCommand()
	command.InputType = "hotpatch"
	command.InputJson = []byte(`{"hotpatch_type":"EnablePlan","params":{"enable_plan":true}}`)
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai hotpatch: %v", err)
	}

	driver.mu.Lock()
	if len(driver.inputs) != 1 {
		driver.mu.Unlock()
		t.Fatalf("expected one runtime input, got %d", len(driver.inputs))
	}
	input := driver.inputs[0]
	driver.mu.Unlock()
	event, err := buildYakAIHotpatchEvent(input)
	if err != nil {
		t.Fatalf("bridge-normalized hotpatch was rejected by runtime decoder: %v; payload=%s", err, input.PayloadJSON)
	}
	if !event.GetIsConfigHotpatch() || event.GetHotpatchType() != "EnablePlan" || !event.GetParams().GetEnablePlan() {
		t.Fatalf("unexpected decoded hotpatch: %#v", event)
	}
}

func TestHandleAISessionInputDeduplicatesRedeliveredCommand(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	raw := mustMarshalProto(t, validAISessionInputCommand())
	if err := bridge.handleAISessionInput(context.Background(), raw); err != nil {
		t.Fatalf("handle first input: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), raw); err != nil {
		t.Fatalf("handle duplicate input: %v", err)
	}

	driver.mu.Lock()
	inputCount := len(driver.inputs)
	driver.mu.Unlock()
	if inputCount != 1 {
		t.Fatalf("duplicate command reached runtime %d times", inputCount)
	}
	fakeJS.mu.Lock()
	published := append([]*nats.Msg(nil), fakeJS.publish...)
	fakeJS.mu.Unlock()
	eventCount := len(published)
	if eventCount != 2 {
		t.Fatalf("duplicate command must republish the idempotent acknowledgement, got %d events", eventCount)
	}
	firstPublished := published[0]
	secondPublished := published[1]
	var firstEvent, secondEvent aiv1.AISessionEvent
	if err := proto.Unmarshal(firstPublished.Data, &firstEvent); err != nil {
		t.Fatalf("unmarshal first input acknowledgement: %v", err)
	}
	if err := proto.Unmarshal(secondPublished.Data, &secondEvent); err != nil {
		t.Fatalf("unmarshal duplicate input acknowledgement: %v", err)
	}
	if firstEvent.GetMetadata().GetEventId() != secondEvent.GetMetadata().GetEventId() ||
		firstEvent.GetSeq() != secondEvent.GetSeq() {
		t.Fatalf("duplicate acknowledgement changed identity: first=%#v second=%#v", &firstEvent, &secondEvent)
	}
}

func TestHandleAISessionInputRepublishesAckWithoutReexecutingAfterPublishFailure(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	fakeJS.failNextPublishes(1)
	raw := mustMarshalProto(t, validAISessionInputCommand())
	if err := bridge.handleAISessionInput(context.Background(), raw); err == nil {
		t.Fatal("expected the first acknowledgement publish to fail")
	}
	if err := bridge.handleAISessionInput(context.Background(), raw); err != nil {
		t.Fatalf("republish input acknowledgement: %v", err)
	}

	driver.mu.Lock()
	inputCount := len(driver.inputs)
	driver.mu.Unlock()
	if inputCount != 1 {
		t.Fatalf("publish retry reexecuted runtime input %d times", inputCount)
	}
	fakeJS.mu.Lock()
	eventCount := len(fakeJS.publish)
	fakeJS.mu.Unlock()
	if eventCount != 1 {
		t.Fatalf("expected one successful acknowledgement after retry, got %d", eventCount)
	}
}

func TestHandleAISessionReviewRejectsStaleNodeSession(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	command := validAISessionInputCommand()
	command.InputType = "user_intervention"
	command.InputJson = []byte(`{"id":"review-fenced","suggestion":"continue"}`)
	command.ReviewId = "review-fenced"
	command.TurnId = "turn-command"
	command.ExpectedNodeSessionId = "stale-node-session"

	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("fenced review should publish a terminal command failure: %v", err)
	}
	driver.mu.Lock()
	inputCount := len(driver.inputs)
	driver.mu.Unlock()
	if inputCount != 0 {
		t.Fatalf("fenced review reached runtime %d times", inputCount)
	}
	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.failed" {
		t.Fatalf("unexpected fenced review event subject: %s", msg.Subject)
	}
	var failed aiv1.AISessionFailed
	if err := proto.Unmarshal(msg.Data, &failed); err != nil {
		t.Fatalf("unmarshal fenced failure: %v", err)
	}
	if failed.GetErrorCode() != "ai_session_review_fenced" {
		t.Fatalf("unexpected fenced failure: %#v", &failed)
	}
}

func TestHandleAISessionInputPublishesSyncEventPayload(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	command := validAISessionInputCommand()
	command.InputType = "sync_event"
	command.InputJson = []byte(`{"sync_type":"recovery_plan_and_exec","sync_json_input":{"coordinator_id":"coor-1","start_task_index":"1-2"}}`)

	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, command)); err != nil {
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
	if event.GetEventType() != aiSessionRuntimeEventInput {
		t.Fatalf("unexpected runtime event type: %s", event.GetEventType())
	}

	var payload map[string]any
	if err := json.Unmarshal(event.GetPayloadJson(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["sync_type"] != "recovery_plan_and_exec" {
		t.Fatalf("unexpected sync type: %#v", payload["sync_type"])
	}
	if _, exists := payload["input_type"]; exists {
		t.Fatalf("sync_event payload should not be rewritten with input_type: %#v", payload)
	}
	driver.assertSyncEvent(t, 0, "recovery_plan_and_exec", `{"coordinator_id":"coor-1","start_task_index":"1-2"}`)
}

func TestAISessionPublisherUsesDistinctEventIDPerSequence(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)

	ref := aiSessionCommandRef{
		CommandID:   "cmd-input-1",
		SessionID:   "ai-session-1",
		RunID:       "run-1",
		OwnerUserID: "user-1",
	}

	if err := bridge.aiPublisher.PublishEvent(
		context.Background(),
		ref,
		1,
		aiSessionRuntimeEventMessage,
		[]byte(`{"content":"first"}`),
	); err != nil {
		t.Fatalf("publish first ai session event: %v", err)
	}
	if err := bridge.aiPublisher.PublishEvent(
		context.Background(),
		ref,
		2,
		aiSessionRuntimeEventMessage,
		[]byte(`{"content":"second"}`),
	); err != nil {
		t.Fatalf("publish second ai session event: %v", err)
	}

	firstMsg := waitForPublishedMessage(t, fakeJS, 0)
	secondMsg := waitForPublishedMessage(t, fakeJS, 1)

	var firstEvent aiv1.AISessionEvent
	if err := proto.Unmarshal(firstMsg.Data, &firstEvent); err != nil {
		t.Fatalf("unmarshal first ai session event: %v", err)
	}
	var secondEvent aiv1.AISessionEvent
	if err := proto.Unmarshal(secondMsg.Data, &secondEvent); err != nil {
		t.Fatalf("unmarshal second ai session event: %v", err)
	}

	firstEventID := firstEvent.GetMetadata().GetEventId()
	secondEventID := secondEvent.GetMetadata().GetEventId()
	if firstEventID == "" || secondEventID == "" {
		t.Fatalf("expected non-empty event ids, got first=%q second=%q", firstEventID, secondEventID)
	}
	if firstEventID == secondEventID {
		t.Fatalf("expected distinct event ids, got duplicated id %q", firstEventID)
	}
	if firstEvent.GetSeq() != 1 || secondEvent.GetSeq() != 2 {
		t.Fatalf("unexpected seq values: first=%d second=%d", firstEvent.GetSeq(), secondEvent.GetSeq())
	}
}

func TestAISessionRuntimeEventsKeepStableStatelessTurnCausationAcrossControlInputs(t *testing.T) {
	t.Parallel()

	handle := &statelessAIEngineRuntimeHandle{
		activeTurn: &statelessAITurn{turnID: "turn-root"},
	}
	runtime := &aiSessionRuntime{
		ref: aiSessionCommandRef{
			CommandID:   "review-command-1",
			SessionID:   "ai-session-1",
			RunID:       "run-1",
			OwnerUserID: "user-1",
		},
		handle: handle,
	}

	firstRef, firstSeq := runtime.nextEventRefAndSeq()
	if firstRef.CommandID != "turn-root" {
		t.Fatalf("first review changed turn causation: got %q", firstRef.CommandID)
	}
	if terminalRef := runtime.currentRef(); terminalRef.CommandID != "turn-root" {
		t.Fatalf("terminal event changed turn causation: got %q", terminalRef.CommandID)
	}

	runtime.mu.Lock()
	runtime.ref.CommandID = "review-command-2"
	runtime.mu.Unlock()
	secondRef, secondSeq := runtime.nextEventRefAndSeq()
	if secondRef.CommandID != "turn-root" {
		t.Fatalf("second review changed turn causation: got %q", secondRef.CommandID)
	}
	if firstSeq != 1 || secondSeq != 2 {
		t.Fatalf("unexpected event sequence: first=%d second=%d", firstSeq, secondSeq)
	}

	handle.mu.Lock()
	handle.activeTurn = nil
	handle.mu.Unlock()
	runtime.mu.Lock()
	runtime.ref.CommandID = "next-turn-command"
	runtime.mu.Unlock()
	nextRef, _ := runtime.nextEventRefAndSeq()
	if nextRef.CommandID != "next-turn-command" {
		t.Fatalf("idle runtime did not fall back to latest command: got %q", nextRef.CommandID)
	}
	if idleTerminalRef := runtime.currentRef(); idleTerminalRef.CommandID != "next-turn-command" {
		t.Fatalf("idle terminal event did not fall back to latest command: got %q", idleTerminalRef.CommandID)
	}
}

func TestHandleAISessionAppendContextPublishesRuntimeEvent(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	if err := bridge.handleAISessionAppendContext(context.Background(), mustMarshalProto(t, validAISessionContextCommand())); err != nil {
		t.Fatalf("handle ai append context: %v", err)
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
	if event.GetEventType() != aiSessionRuntimeEventContextUpdated {
		t.Fatalf("unexpected runtime event type: %s", event.GetEventType())
	}
	driver.assertContextReason(t, 0, "append from project context")
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

func TestHandleAISessionClosePublishesCloseAcknowledgement(t *testing.T) {
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
	if msg.Subject != "legion.event.ai.session.close" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}

	var event aiv1.AISessionDone
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session done: %v", err)
	}
	if event.GetSession().GetSessionId() != "ai-session-1" {
		t.Fatalf("unexpected session id: %s", event.GetSession().GetSessionId())
	}
	if event.GetMetadata().GetEventType() != legionEventAISessionClose {
		t.Fatalf("unexpected event type: %s", event.GetMetadata().GetEventType())
	}
	if !strings.Contains(string(event.GetResultJson()), "\"closed_by\":\"platform\"") {
		t.Fatalf("unexpected done payload: %s", string(event.GetResultJson()))
	}
	driver.assertClose(t, 0, "platform done")
}

func TestHandleAISessionCloseAcknowledgesMissingRuntimeAfterRestart(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)
	if err := bridge.handleAISessionClose(context.Background(), mustMarshalProto(t, validAISessionCloseCommand())); err != nil {
		t.Fatalf("handle ai close without local runtime: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.close" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}
	var event aiv1.AISessionDone
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal ai session close: %v", err)
	}
	if !strings.Contains(string(event.GetResultJson()), `"already_terminal":"true"`) {
		t.Fatalf("missing restart acknowledgement marker: %s", string(event.GetResultJson()))
	}
}

func TestHandleAISessionCloseRetainsRuntimeUntilTerminalEventsPublish(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	fakeJS.failNextPublishes(1)

	closeRaw := mustMarshalProto(t, validAISessionCloseCommand())
	if err := bridge.handleAISessionClose(context.Background(), closeRaw); err == nil {
		t.Fatal("expected terminal focus-result publication failure")
	}
	driver.assertClose(t, 0, "platform done")
	if !hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("runtime was removed before terminal events could be retried")
	}

	if err := bridge.handleAISessionClose(context.Background(), closeRaw); err != nil {
		t.Fatalf("retry ai close: %v", err)
	}
	driver.mu.Lock()
	closeCount := len(driver.closes)
	driver.mu.Unlock()
	if closeCount != 1 {
		t.Fatalf("expected runtime close to be applied once, got %d", closeCount)
	}
	if hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("runtime was not removed after both terminal events published")
	}
	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.succeeded", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.close", 1)
}

func TestHandleAISessionCancelRetainsRuntimeUntilTerminalEventsPublish(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	fakeJS.failNextPublishes(1)

	cancelRaw := mustMarshalProto(t, validAISessionCancelCommand())
	if err := bridge.handleAISessionCancel(context.Background(), cancelRaw); err == nil {
		t.Fatal("expected terminal focus-result publication failure")
	}
	driver.assertCancel(t, 0, "user requested")
	if !hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("runtime was removed before terminal events could be retried")
	}

	if err := bridge.handleAISessionCancel(context.Background(), cancelRaw); err != nil {
		t.Fatalf("retry ai cancel: %v", err)
	}
	driver.mu.Lock()
	cancelCount := len(driver.cancels)
	driver.mu.Unlock()
	if cancelCount != 1 {
		t.Fatalf("expected runtime cancel to be applied once, got %d", cancelCount)
	}
	if hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("runtime was not removed after both terminal events published")
	}
	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.cancelled", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.cancelled", 1)
}

func TestRuntimeEmitterRetriesTerminalPublication(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	fakeJS.failNextPublishes(1)

	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	emitter.Done([]byte(`{"summary":"finished"}`))

	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.succeeded", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.done", 1)
}

func TestRuntimeEmitterSingleRunCompletesWithTurnCausationAndRemovesRuntime(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("handle ai input: %v", err)
	}
	resetPublishedMessages(fakeJS)

	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	completer, ok := emitter.(aiSessionRuntimeTurnCompleter)
	if !ok {
		t.Fatal("managed runtime emitter does not support turn completion")
	}
	completer.DoneTurn("cmd-input-1", []byte(`{"status":"done"}`))

	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.report", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.succeeded", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.done", 1)
	if hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("single-run runtime was not removed after terminal publication")
	}

	before := publishedMessageCount(fakeJS)
	if err := bridge.handleAISessionClose(context.Background(), mustMarshalProto(t, validAISessionCloseCommand())); err != nil {
		t.Fatalf("late close after automatic completion: %v", err)
	}
	if after := publishedMessageCount(fakeJS); after != before {
		t.Fatalf("late close duplicated terminal events: before=%d after=%d", before, after)
	}
}

func TestRuntimeEmitterConversationResultKeepsRuntimeAfterTurn(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.FocusMode = legionAIConversationAuditResultMode
	command.ResultContext.FocusReleaseId = ""
	command.ResultContext.ExecutionMode = legionAIConversationExecutionMode
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai conversation bind: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("handle ai conversation input: %v", err)
	}
	resetPublishedMessages(fakeJS)

	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	reporter, ok := emitter.(aiSessionRuntimeTurnReporter)
	if !ok {
		t.Fatal("managed runtime emitter does not support conversation turn completion")
	}
	reporter.TurnCompleted("cmd-input-1", []byte(`{"status":"done"}`))

	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.succeeded", 0)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.done", 0)
	msg := waitForPublishedMessage(t, fakeJS, 0)
	var event aiv1.AISessionEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal conversation turn completion: %v", err)
	}
	if event.GetEventType() != aiSessionRuntimeEventTurnCompleted || event.GetSession().GetBindEpoch() != 1 {
		t.Fatalf("turn completion event = %#v", &event)
	}
	if !hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("multi-turn conversation runtime was removed after one turn")
	}
}

func TestRuntimeEmitterConversationTurnPublicationOutlivesTerminalTimeout(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.FocusMode = legionAIConversationAuditResultMode
	command.ResultContext.FocusReleaseId = ""
	command.ResultContext.ExecutionMode = legionAIConversationExecutionMode
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle conversation bind: %v", err)
	}
	resetPublishedMessages(fakeJS)
	previousTimeout := aiSessionTerminalPublishTimeout
	aiSessionTerminalPublishTimeout = time.Millisecond
	defer func() { aiSessionTerminalPublishTimeout = previousTimeout }()
	fakeJS.failNextPublishes(3)

	driver.mu.Lock()
	reporter := driver.emitters[0].(aiSessionRuntimeTurnReporter)
	driver.mu.Unlock()
	reporter.TurnCompleted("cmd-input-1", []byte(`{"status":"done"}`))

	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.event", 1)
}

func TestRuntimeEmitterConversationFailureKeepsRuntime(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.FocusMode = legionAIConversationAuditResultMode
	command.ResultContext.FocusReleaseId = ""
	command.ResultContext.ExecutionMode = legionAIConversationExecutionMode
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai conversation bind: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("handle ai conversation input: %v", err)
	}
	resetPublishedMessages(fakeJS)

	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	reporter, ok := emitter.(aiSessionRuntimeTurnReporter)
	if !ok {
		t.Fatal("managed runtime emitter does not support conversation turn failure")
	}
	reporter.TurnFailed(
		"cmd-input-1",
		"yak_ai_forge_failed",
		"forge failed",
		[]byte(`{"runtime":"stateless_yak_ai_engine"}`),
	)

	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.failed", 0)
	msg := waitForPublishedMessage(t, fakeJS, 0)
	var event aiv1.AISessionEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("unmarshal conversation turn failure: %v", err)
	}
	if event.GetEventType() != aiSessionRuntimeEventTurnFailed {
		t.Fatalf("turn failure event type = %q", event.GetEventType())
	}
	if !hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("failed conversation turn removed reusable runtime")
	}
}

func TestAISessionConversationRootPlanMarkerDoesNotTerminalizeRuntime(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.ExecutionMode = legionAIConversationExecutionMode
	command.ResultContext.FocusMode = legionAIConversationAuditResultMode
	command.ResultContext.FocusReleaseId = ""
	command.Session.RunId = command.ResultContext.FocusRunId
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("bind conversation runtime: %v", err)
	}
	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	resetPublishedMessages(fakeJS)
	emitter.Emit("ai.session.event", []byte(`{"type":"end_plan_and_execution","task_index":"","content_json":{}}`))
	if !hasAISessionRuntime(bridge.aiRuntime, command.GetSession().GetSessionId()) {
		t.Fatal("multi-turn root plan marker terminalized reusable runtime")
	}
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.event", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.done", 0)
}

func TestRuntimeManagerRejectsNewCommandsAfterTerminalFailureClaim(t *testing.T) {
	bridge, _, _ := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.ResultContext.FocusMode = legionAIConversationAuditResultMode
	command.ResultContext.FocusReleaseId = ""
	command.ResultContext.ExecutionMode = legionAIConversationExecutionMode
	command.Session.RunId = command.ResultContext.FocusRunId
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai conversation bind: %v", err)
	}
	first := validAISessionInputCommand()
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("handle first input: %v", err)
	}

	manager := bridge.aiRuntime
	manager.mu.Lock()
	runtime := manager.sessions[first.GetSession().GetSessionId()]
	manager.mu.Unlock()
	if runtime == nil {
		t.Fatal("bound runtime is missing")
	}
	if _, claimed := runtime.claimTerminalFailure(first.GetMetadata().GetCommandId()); !claimed {
		t.Fatal("terminal failure was not claimed")
	}

	duplicate, err := manager.AcceptInput(first)
	if err != nil || !duplicate.duplicate {
		t.Fatalf("processed duplicate after terminal claim = (%+v, %v), want durable replay", duplicate, err)
	}
	late := proto.Clone(first).(*aiv1.PushAISessionInputCommand)
	late.Metadata.CommandId = "cmd-input-too-late"
	if _, err := manager.AcceptInput(late); err == nil || !strings.Contains(err.Error(), "runtime is terminal") {
		t.Fatalf("late input error = %v, want terminal rejection", err)
	}
	contextCommand := validAISessionContextCommand()
	contextCommand.Metadata.CommandId = "cmd-context-too-late"
	if _, err := manager.AcceptContextUpdate(contextCommand); err == nil || !strings.Contains(err.Error(), "runtime is terminal") {
		t.Fatalf("late context error = %v, want terminal rejection", err)
	}
}

func TestRuntimeEmitterSingleRunFailureRemovesRuntimeWithoutLateSuccess(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validAIFocusResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	bridge.publisher.js = fakeJS
	bridge.publisher.natsURL = "nats://node-ai.test"
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, validAISessionInputCommand())); err != nil {
		t.Fatalf("handle ai input: %v", err)
	}
	resetPublishedMessages(fakeJS)

	driver.mu.Lock()
	emitter := driver.emitters[0]
	driver.mu.Unlock()
	completer, ok := emitter.(aiSessionRuntimeTurnCompleter)
	if !ok {
		t.Fatal("managed runtime emitter does not support turn completion")
	}
	completer.FailTurn(
		"cmd-input-1",
		"bounded_probe_failed",
		"target did not return a verified response",
		[]byte(`{"runtime":"stateless_yak_ai_engine"}`),
	)

	assertPublishedSubjectCount(t, fakeJS, "legion.event.job.failed", 1)
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.failed", 1)
	if hasAISessionRuntime(bridge.aiRuntime, "ai-session-1") {
		t.Fatal("failed single-run runtime was not removed after terminal publication")
	}

	before := publishedMessageCount(fakeJS)
	if err := bridge.handleAISessionClose(context.Background(), mustMarshalProto(t, validAISessionCloseCommand())); err != nil {
		t.Fatalf("late close after automatic failure: %v", err)
	}
	if after := publishedMessageCount(fakeJS); after != before {
		t.Fatalf("late close converted failure into success: before=%d after=%d", before, after)
	}
}

func publishedMessageCount(fakeJS *aiFakeJetStreamContext) int {
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	return len(fakeJS.publish)
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
	if msg.Subject != "legion.event.ai.session.close" {
		t.Fatalf("unexpected subject: %s", msg.Subject)
	}
}

func TestCommandConsumerRoutesAISessionAppendContextCommand(t *testing.T) {
	t.Parallel()

	bridge, fakeJS, _ := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}
	resetPublishedMessages(fakeJS)

	message := nats.NewMsg("legion.command.node.node-ai.ai.session.context.append")
	message.Data = mustMarshalProto(t, validAISessionContextCommand())

	if err := bridge.handleMessage(context.Background(), message); err != nil {
		t.Fatalf("handle routed ai append context: %v", err)
	}

	msg := waitForPublishedMessage(t, fakeJS, 0)
	if msg.Subject != "legion.event.ai.session.event" {
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

	mu                sync.Mutex
	publish           []*nats.Msg
	remainingFailures int
}

func (f *aiFakeJetStreamContext) PublishMsg(msg *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.remainingFailures > 0 {
		f.remainingFailures--
		return nil, errors.New("injected JetStream publish failure")
	}
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

func (f *aiFakeJetStreamContext) failNextPublishes(count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remainingFailures = count
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
			IssuedAt:  timestamppb.New(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)),
		},
		TargetNodeId: "node-ai",
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
		},
		OwnerUserId: "user-1",
		ProjectId:   "project-1",
		Title:       "AI session",
		BindEpoch:   1,
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
			BindEpoch: 1,
		},
		OwnerUserId: "user-1",
		InputType:   "message",
		InputJson:   []byte(`{"content":"hello"}`),
	}
}

func TestRuntimeManagerRejectsDelayedInputFromOldBindEpoch(t *testing.T) {
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("bind epoch one: %v", err)
	}
	rebind := validAISessionBindCommand()
	rebind.Metadata.CommandId = "cmd-bind-2"
	rebind.BindEpoch = 2
	rebind.Session.BindEpoch = 2
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("bind epoch two: %v", err)
	}

	delayed := validAISessionInputCommand()
	delayed.Metadata.CommandId = "cmd-input-delayed"
	delayed.TurnId = delayed.Metadata.CommandId
	delayed.Session.BindEpoch = 1
	resetPublishedMessages(fakeJS)
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, delayed)); err != nil {
		t.Fatalf("publish delayed old-epoch input failure: %v", err)
	}
	assertPublishedSubjectCount(t, fakeJS, "legion.event.ai.session.failed", 1)
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if got := len(driver.inputs); got != 0 {
		t.Fatalf("old-epoch input reached current runtime: inputs=%d", got)
	}
}

func TestRuntimeManagerRejectsDelayedControlCommandsFromOldBindEpoch(t *testing.T) {
	bridge, _, _ := newTestAISessionBridge(t)
	first := validAISessionBindCommand()
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, first)); err != nil {
		t.Fatalf("bind epoch one: %v", err)
	}
	rebind := validAISessionBindCommand()
	rebind.Metadata.CommandId = "cmd-bind-control-2"
	rebind.BindEpoch = 2
	rebind.Session.BindEpoch = 2
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, rebind)); err != nil {
		t.Fatalf("bind epoch two: %v", err)
	}

	contextCommand := validAISessionContextCommand()
	contextCommand.Metadata.CommandId = "cmd-context-delayed"
	contextCommand.Session.BindEpoch = 1
	if _, err := bridge.aiRuntime.AcceptContextUpdate(contextCommand); err == nil {
		t.Fatal("old-epoch context command was accepted")
	}
	cancelCommand := validAISessionCancelCommand()
	cancelCommand.Metadata.CommandId = "cmd-cancel-delayed"
	cancelCommand.Session.BindEpoch = 1
	if _, err := bridge.aiRuntime.Cancel(cancelCommand); err == nil {
		t.Fatal("old-epoch cancel command was accepted")
	}
	closeCommand := validAISessionCloseCommand()
	closeCommand.Metadata.CommandId = "cmd-close-delayed"
	closeCommand.Session.BindEpoch = 1
	if _, err := bridge.aiRuntime.Close(closeCommand); err == nil {
		t.Fatal("old-epoch close command was accepted")
	}
	if !hasAISessionRuntime(bridge.aiRuntime, first.GetSession().GetSessionId()) {
		t.Fatal("old-epoch control command retired the current runtime")
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
			BindEpoch: 1,
		},
		OwnerUserId: "user-1",
		Reason:      "user requested",
	}
}

func validAISessionContextCommand() *aiv1.AppendAISessionContextCommand {
	return &aiv1.AppendAISessionContextCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-context-1",
		},
		Session: &aiv1.AISessionRef{
			SessionId: "ai-session-1",
			RunId:     "run-1",
			BindEpoch: 1,
		},
		OwnerUserId: "user-1",
		Attachments: []*aiv1.AISessionAttachmentRef{
			{
				AttachmentId: "aiatt_123",
				Filename:     "targets.txt",
				DownloadUrl:  "http://platform.test/v1/ai/attachments/aiatt_123/download?node_session_id=node-session-ai",
			},
		},
		Reason: "append from project context",
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
			BindEpoch: 1,
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

func assertPublishedSubjectCount(
	t *testing.T,
	fakeJS *aiFakeJetStreamContext,
	subject string,
	want int,
) {
	t.Helper()
	fakeJS.mu.Lock()
	defer fakeJS.mu.Unlock()
	got := 0
	for _, msg := range fakeJS.publish {
		if msg.Subject == subject {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected %d %s events, got %d", want, subject, got)
	}
}

func hasAISessionRuntime(manager *aiSessionRuntimeManager, sessionID string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	_, ok := manager.sessions[sessionID]
	return ok
}

type recordingAISessionRuntimeDriver struct {
	mu       sync.Mutex
	bindings []aiSessionBinding
	emitters []aiSessionRuntimeEmitter
	inputs   []aiSessionInput
	contexts []aiSessionContextUpdate
	cancels  []string
	closes   []string
}

type controlledAISessionRuntimeDriver struct {
	mu       sync.Mutex
	recorder *recordingAISessionRuntimeDriver
	binds    int
	failOn   int
	blockOn  int
	started  chan struct{}
	release  chan struct{}
}

func (d *controlledAISessionRuntimeDriver) Bind(
	ctx context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	d.mu.Lock()
	d.binds++
	call := d.binds
	d.mu.Unlock()
	if call == d.blockOn {
		close(d.started)
		select {
		case <-d.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if call == d.failOn {
		return nil, errors.New("injected bind failure")
	}
	return d.recorder.Bind(ctx, binding, emitter)
}

type recordingLifecycleAIFocusResultSink struct {
	mu        sync.Mutex
	succeeded [][]byte
}

func (s *recordingLifecycleAIFocusResultSink) SubmitRisk(
	context.Context,
	*schema.Risk,
) (aiFocusResultReceipt, error) {
	return aiFocusResultReceipt{}, nil
}

func (s *recordingLifecycleAIFocusResultSink) Succeed(_ context.Context, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.succeeded = append(s.succeeded, cloneBytes(raw))
	return nil
}

func (*recordingLifecycleAIFocusResultSink) Fail(context.Context, string, string, []byte) error {
	return nil
}

func (*recordingLifecycleAIFocusResultSink) Cancel(context.Context, string) error {
	return nil
}

func (d *recordingAISessionRuntimeDriver) Bind(
	_ context.Context,
	binding aiSessionBinding,
	emitter aiSessionRuntimeEmitter,
) (aiSessionRuntimeHandle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bindings = append(d.bindings, binding)
	d.emitters = append(d.emitters, emitter)
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

func (d *recordingAISessionRuntimeDriver) assertContextReason(t *testing.T, index int, reason string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if len(d.contexts) > index {
			update := d.contexts[index]
			d.mu.Unlock()
			if update.Reason != reason {
				t.Fatalf("unexpected context reason: %s", update.Reason)
			}
			if len(update.AttachmentRefs) != 1 || update.AttachmentRefs[0].AttachmentID != "aiatt_123" {
				t.Fatalf("unexpected context attachments: %#v", update.AttachmentRefs)
			}
			return
		}
		d.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for recorded context update at index %d", index)
}

func (d *recordingAISessionRuntimeDriver) assertSyncEvent(t *testing.T, index int, syncType string, syncJSONInput string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if len(d.inputs) > index {
			input := d.inputs[index]
			d.mu.Unlock()
			if input.InputType != "sync_event" {
				t.Fatalf("unexpected recorded input type: %s", input.InputType)
			}
			var payload map[string]any
			if err := json.Unmarshal(input.PayloadJSON, &payload); err != nil {
				t.Fatalf("unmarshal recorded sync payload: %v", err)
			}
			if payload["sync_type"] != syncType {
				t.Fatalf("unexpected recorded sync type: %#v", payload["sync_type"])
			}
			got, ok := payload["sync_json_input"]
			if !ok {
				t.Fatalf("missing sync_json_input: %#v", payload)
			}
			gotRaw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal recorded sync_json_input: %v", err)
			}
			assertJSONEqual(t, gotRaw, syncJSONInput)
			return
		}
		d.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for recorded sync input at index %d", index)
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

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if len(d.closes) > index {
			got := d.closes[index]
			d.mu.Unlock()
			if got != reason {
				t.Fatalf("unexpected close reason: %s", got)
			}
			return
		}
		d.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing recorded close at index %d", index)
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

func (h *recordingAISessionRuntimeHandle) AppendContext(_ context.Context, update aiSessionContextUpdate) error {
	h.driver.mu.Lock()
	defer h.driver.mu.Unlock()
	h.driver.contexts = append(h.driver.contexts, update)
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

func TestHandleAISessionInputCarriesContextPackage(t *testing.T) {
	t.Parallel()

	bridge, _, driver := newTestAISessionBridge(t)
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
		t.Fatalf("handle ai bind: %v", err)
	}

	cmd := validAISessionInputCommand()
	cmd.ContextPackage = &aiv1.ContextPackage{
		SessionId: "ai-session-1",
		UserInput: "hello",
		Messages: []*aiv1.ContextMessage{
			{Role: "user", Content: "prior question"},
			{Role: "assistant", Content: "prior answer"},
		},
	}
	if err := bridge.handleAISessionInput(context.Background(), mustMarshalProto(t, cmd)); err != nil {
		t.Fatalf("handle ai input: %v", err)
	}

	driver.mu.Lock()
	inputs := len(driver.inputs)
	var gotCP *aiv1.ContextPackage
	if inputs > 0 {
		gotCP = driver.inputs[0].ContextPackage
	}
	driver.mu.Unlock()

	if inputs != 1 {
		t.Fatalf("expected 1 recorded input, got %d", inputs)
	}
	if gotCP == nil {
		t.Fatal("ContextPackage not carried through to handle.SendInput")
	}
	if gotCP.SessionId != "ai-session-1" || gotCP.UserInput != "hello" {
		t.Fatalf("context package session/user_input wrong: %#v", gotCP)
	}
	if len(gotCP.Messages) != 2 {
		t.Fatalf("expected 2 context messages, got %d", len(gotCP.Messages))
	}
	if gotCP.Messages[0].Role != "user" || gotCP.Messages[0].Content != "prior question" {
		t.Fatalf("first context message wrong: %#v", gotCP.Messages[0])
	}
	if gotCP.Messages[1].Role != "assistant" || gotCP.Messages[1].Content != "prior answer" {
		t.Fatalf("second context message wrong: %#v", gotCP.Messages[1])
	}
}
