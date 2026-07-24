package scannode

import (
	"context"
	"errors"
	"testing"
	"time"

	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
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
		{name: "artifact_ready", message: &jobv1.JobArtifactReady{}},
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

func TestAttemptProgressCheckpointPublishesFirstDuplicateAndSignificantJump(t *testing.T) {
	t.Parallel()

	var (
		checkpoint attemptProgressCheckpoint
		published  []float64
	)
	publish := func(process float64) error {
		published = append(published, process)
		return nil
	}
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	if err := checkpoint.report(0.10, base, publish); err != nil {
		t.Fatalf("publish first checkpoint: %v", err)
	}
	if err := checkpoint.report(0.10, base.Add(500*time.Millisecond), publish); err != nil {
		t.Fatalf("publish duplicate checkpoint: %v", err)
	}
	if err := checkpoint.report(0.16, base.Add(750*time.Millisecond), publish); err != nil {
		t.Fatalf("publish significant jump checkpoint: %v", err)
	}

	if got := len(published); got != 2 {
		t.Fatalf("unexpected publish count: got=%d want=%d", got, 2)
	}
	if published[0] != 0.10 || published[1] != 0.16 {
		t.Fatalf("unexpected published checkpoints: %#v", published)
	}
}

func TestAttemptProgressCheckpointSuppressesHighFrequencyMonotonicProgress(t *testing.T) {
	t.Parallel()

	var (
		checkpoint attemptProgressCheckpoint
		published  []float64
	)
	publish := func(process float64) error {
		published = append(published, process)
		return nil
	}
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	if err := checkpoint.report(0.10001, base, publish); err != nil {
		t.Fatalf("publish first checkpoint: %v", err)
	}
	if err := checkpoint.report(0.10004, base.Add(legionProgressCheckpointInterval/2), publish); err != nil {
		t.Fatalf("publish throttled duplicate checkpoint: %v", err)
	}
	if err := checkpoint.report(0.10490, base.Add(700*time.Millisecond), publish); err != nil {
		t.Fatalf("publish throttled monotonic checkpoint: %v", err)
	}

	if got := len(published); got != 1 {
		t.Fatalf("unexpected publish count: got=%d want=%d", got, 1)
	}
	if published[0] != 0.10001 {
		t.Fatalf("unexpected published checkpoints: %#v", published)
	}
}

func TestAttemptProgressCheckpointReemitsAfterIntervalForSmallMonotonicGrowth(t *testing.T) {
	t.Parallel()

	var (
		checkpoint attemptProgressCheckpoint
		published  []float64
	)
	publish := func(process float64) error {
		published = append(published, process)
		return nil
	}
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	if err := checkpoint.report(0.20, base, publish); err != nil {
		t.Fatalf("publish first checkpoint: %v", err)
	}
	if err := checkpoint.report(0.20490, base.Add(500*time.Millisecond), publish); err != nil {
		t.Fatalf("publish throttled monotonic checkpoint: %v", err)
	}
	if err := checkpoint.report(0.20490, base.Add(legionProgressCheckpointInterval+100*time.Millisecond), publish); err != nil {
		t.Fatalf("publish interval checkpoint: %v", err)
	}

	if got := len(published); got != 2 {
		t.Fatalf("unexpected publish count: got=%d want=%d", got, 2)
	}
	if published[0] != 0.20 || published[1] != 0.20490 {
		t.Fatalf("unexpected published checkpoints: %#v", published)
	}
}

func TestAttemptProgressCheckpointRepublishesTerminalOnce(t *testing.T) {
	t.Parallel()

	var (
		checkpoint attemptProgressCheckpoint
		published  []float64
	)
	publish := func(process float64) error {
		published = append(published, process)
		return nil
	}
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	if err := checkpoint.report(0.99996, base, publish); err != nil {
		t.Fatalf("publish near-terminal checkpoint: %v", err)
	}
	if err := checkpoint.report(1.0, base.Add(100*time.Millisecond), publish); err != nil {
		t.Fatalf("publish terminal checkpoint: %v", err)
	}
	if err := checkpoint.report(1.0, base.Add(legionProgressCheckpointInterval+200*time.Millisecond), publish); err != nil {
		t.Fatalf("publish duplicate terminal checkpoint: %v", err)
	}

	if got := len(published); got != 2 {
		t.Fatalf("unexpected publish count: got=%d want=%d", got, 2)
	}
	if published[0] != 0.99996 || published[1] != 1.0 {
		t.Fatalf("unexpected published checkpoints: %#v", published)
	}
}

func TestAttemptProgressCheckpointDoesNotAdvanceStateOnPublishError(t *testing.T) {
	t.Parallel()

	var checkpoint attemptProgressCheckpoint
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)
	wantErr := errors.New("publish failed")

	if err := checkpoint.report(0.20, base, func(float64) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("unexpected publish error: %v", err)
	}

	published := 0
	if err := checkpoint.report(0.20, base.Add(100*time.Millisecond), func(float64) error {
		published++
		return nil
	}); err != nil {
		t.Fatalf("publish after error: %v", err)
	}

	if published != 1 {
		t.Fatalf("expected retry to publish once, got=%d", published)
	}
}

func TestAttemptProgressCheckpointFlushesLatestObservedProgress(t *testing.T) {
	t.Parallel()

	var (
		checkpoint attemptProgressCheckpoint
		published  []float64
	)
	publish := func(process float64) error {
		published = append(published, process)
		return nil
	}
	base := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)

	if err := checkpoint.report(0.20, base, publish); err != nil {
		t.Fatalf("publish first checkpoint: %v", err)
	}
	if err := checkpoint.report(0.24, base.Add(500*time.Millisecond), publish); err != nil {
		t.Fatalf("observe throttled checkpoint: %v", err)
	}
	if err := checkpoint.flushLatest(base.Add(600*time.Millisecond), publish); err != nil {
		t.Fatalf("flush latest checkpoint: %v", err)
	}

	if got := len(published); got != 2 {
		t.Fatalf("unexpected publish count: got=%d want=%d", got, 2)
	}
	if published[0] != 0.20 || published[1] != 0.24 {
		t.Fatalf("unexpected published checkpoints: %#v", published)
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
	case *jobv1.JobArtifactReady:
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
