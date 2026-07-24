package scannode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

func TestValidateDispatchCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*jobv1.DispatchJobCommand)
		wantErr string
	}{
		{
			name: "valid command",
		},
		{
			name: "missing metadata",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.Metadata = nil
			},
			wantErr: "dispatch metadata is required",
		},
		{
			name: "missing command id",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.Metadata.CommandId = ""
			},
			wantErr: "dispatch command_id is required",
		},
		{
			name: "missing job",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.Job = nil
			},
			wantErr: "dispatch job reference is required",
		},
		{
			name: "missing script version id",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.Script.Version.ReleaseId = ""
			},
			wantErr: "dispatch script release_id is required",
		},
		{
			name: "missing script content",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.Script.Content = ""
			},
			wantErr: "dispatch script content is required",
		},
		{
			name: "target mismatch",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "dispatch target_node_id mismatch: node-b",
		},
		{
			name: "unsupported execution kind",
			mutate: func(command *jobv1.DispatchJobCommand) {
				command.ExecutionKind = "binary_payload"
			},
			wantErr: "unsupported execution_kind: binary_payload",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			command := validDispatchCommand()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateDispatchCommand("node-a", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate dispatch command: %v", err)
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

func TestHandleCancel(t *testing.T) {
	t.Parallel()

	manager := newTaskManager()
	taskID := taskIDForSubtask("subtask-1")
	taskCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := &Task{
		TaskType: "script-task",
		TaskId:   taskID,
		Ctx:      taskCtx,
		Cancel:   cancel,
	}
	manager.Add(taskID, task)

	bridge := &legionJobBridge{agent: &ScanNode{manager: manager}}
	raw, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job:    &jobv1.JobRef{SubtaskId: "subtask-1"},
		Reason: "platform stop requested",
	})
	if err != nil {
		t.Fatalf("marshal cancel command: %v", err)
	}

	if err := bridge.handleCancel(raw); err != nil {
		t.Fatalf("handle cancel: %v", err)
	}
	if got := task.CancelReason(); got != "platform stop requested" {
		t.Fatalf("unexpected cancel reason: %s", got)
	}

	select {
	case <-taskCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("task context was not cancelled")
	}
}

func TestHandleCancelMissingTaskIsNoop(t *testing.T) {
	t.Parallel()

	bridge := &legionJobBridge{agent: &ScanNode{manager: newTaskManager()}}
	raw, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job:    &jobv1.JobRef{SubtaskId: "missing-task"},
		Reason: "platform stop requested",
	})
	if err != nil {
		t.Fatalf("marshal cancel command: %v", err)
	}

	if err := bridge.handleCancel(raw); err != nil {
		t.Fatalf("handle cancel: %v", err)
	}
}

func TestConsumerNameForNode(t *testing.T) {
	t.Parallel()

	got := consumerNameForNode("Node A/1")
	if !strings.HasPrefix(got, "legion-node-") {
		t.Fatalf("unexpected consumer prefix: %s", got)
	}
	if strings.ContainsAny(got, " /") {
		t.Fatalf("consumer name still contains invalid characters: %s", got)
	}
}

func TestIsCommandConsumerResetError(t *testing.T) {
	t.Parallel()

	if !isCommandConsumerResetError(nats.ErrConsumerDeleted) {
		t.Fatal("expected ErrConsumerDeleted to reset consumer")
	}
	if !isCommandConsumerResetError(nats.ErrNoResponders) {
		t.Fatal("expected ErrNoResponders to reset consumer")
	}
	if isCommandConsumerResetError(nats.ErrTimeout) {
		t.Fatal("did not expect ErrTimeout to reset consumer")
	}
}

func validDispatchCommand() *jobv1.DispatchJobCommand {
	return &jobv1.DispatchJobCommand{
		Metadata: &nodev1.CommandMetadata{
			CommandId: "cmd-1",
		},
		TargetNodeId: "node-a",
		Job: &jobv1.JobRef{
			JobId:     "job-1",
			SubtaskId: "subtask-1",
			AttemptId: "attempt-1",
		},
		Script: &jobv1.InlineScript{
			Version: &jobv1.ScriptVersionRef{
				ReleaseId: "release-a",
			},
			Content: `println("test")`,
		},
		ExecutionKind: "yak_script",
	}
}
