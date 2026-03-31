package scannode

import (
	"context"
	"testing"

	jobv1 "github.com/yaklang/yaklang/common/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/common/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestAttachEventMetadata(t *testing.T) {
	t.Parallel()

	metadata := &nodev1.EventMetadata{EventId: "event-1"}
	tests := []struct {
		name    string
		message proto.Message
	}{
		{name: "claimed", message: &jobv1.JobClaimed{}},
		{name: "started", message: &jobv1.JobStarted{}},
		{name: "progressed", message: &jobv1.JobProgressed{}},
		{name: "asset", message: &jobv1.JobAsset{}},
		{name: "risk", message: &jobv1.JobRisk{}},
		{name: "report", message: &jobv1.JobReport{}},
		{name: "succeeded", message: &jobv1.JobSucceeded{}},
		{name: "failed", message: &jobv1.JobFailed{}},
		{name: "cancelled", message: &jobv1.JobCancelled{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := attachEventMetadata(tt.message, metadata); err != nil {
				t.Fatalf("attach event metadata: %v", err)
			}
			if got := eventMetadataFromMessage(tt.message); got != metadata {
				t.Fatalf("metadata not attached for %T", tt.message)
			}
		})
	}
}

func TestAttachEventMetadataUnsupported(t *testing.T) {
	t.Parallel()

	err := attachEventMetadata(&emptypb.Empty{}, &nodev1.EventMetadata{EventId: "event-1"})
	if err == nil {
		t.Fatal("expected unsupported event error")
	}
}

func TestProgressUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		process float64
		want    uint32
	}{
		{name: "negative clamped to zero", process: -0.5, want: 0},
		{name: "fraction rounded", process: 0.3333, want: 3333},
		{name: "overflow clamped to max", process: 1.5, want: legionProgressTotalUnits},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := progressUnits(tt.process); got != tt.want {
				t.Fatalf("unexpected progress units: got=%d want=%d", got, tt.want)
			}
		})
	}
}

func TestLegionJobExecutionRefContext(t *testing.T) {
	t.Parallel()

	ref := jobExecutionRef{
		CommandID: "cmd-1",
		JobID:     "job-1",
		SubtaskID: "subtask-1",
		AttemptID: "attempt-1",
	}

	ctx := withLegionJobExecutionRef(context.Background(), ref)
	got := legionJobExecutionRefFromContext(ctx)
	if got == nil {
		t.Fatal("expected execution ref in context")
	}
	if *got != ref {
		t.Fatalf("unexpected execution ref: %#v", got)
	}
}

func TestJobEventSubject(t *testing.T) {
	t.Parallel()

	got := jobEventSubject("legion.node.event.", "."+legionEventStarted)
	if got != "legion.node.event.job.started" {
		t.Fatalf("unexpected event subject: %s", got)
	}
}

func eventMetadataFromMessage(message proto.Message) *nodev1.EventMetadata {
	switch value := message.(type) {
	case *jobv1.JobClaimed:
		return value.Metadata
	case *jobv1.JobStarted:
		return value.Metadata
	case *jobv1.JobProgressed:
		return value.Metadata
	case *jobv1.JobAsset:
		return value.Metadata
	case *jobv1.JobRisk:
		return value.Metadata
	case *jobv1.JobReport:
		return value.Metadata
	case *jobv1.JobSucceeded:
		return value.Metadata
	case *jobv1.JobFailed:
		return value.Metadata
	case *jobv1.JobCancelled:
		return value.Metadata
	default:
		return nil
	}
}
