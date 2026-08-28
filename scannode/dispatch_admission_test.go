package scannode

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type recordingDispatchReporter struct {
	mu            sync.Mutex
	events        []string
	failureCodes  map[string]string
	failureDetail map[string]map[string]string
	failStarted   bool
	failSucceeded bool
	failFailed    bool
	cancelBlock   <-chan struct{}
}

func (r *recordingDispatchReporter) record(kind string, ref jobExecutionRef) {
	r.mu.Lock()
	r.events = append(r.events, kind+":"+ref.AttemptID)
	r.mu.Unlock()
}

func (r *recordingDispatchReporter) PublishClaimed(_ context.Context, ref jobExecutionRef) error {
	r.record("claimed", ref)
	return nil
}

func (r *recordingDispatchReporter) PublishStarted(_ context.Context, ref jobExecutionRef) error {
	r.record("started", ref)
	if r.failStarted {
		return errors.New("started unavailable")
	}
	return nil
}

func (r *recordingDispatchReporter) PublishSucceeded(_ context.Context, ref jobExecutionRef, _ any) error {
	r.record("succeeded", ref)
	if r.failSucceeded {
		return errors.New("terminal unavailable")
	}
	return nil
}

func (r *recordingDispatchReporter) PublishFailed(
	_ context.Context,
	ref jobExecutionRef,
	code string,
	_ string,
	detail map[string]string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "failed:"+ref.AttemptID)
	if r.failureCodes == nil {
		r.failureCodes = make(map[string]string)
	}
	if r.failureDetail == nil {
		r.failureDetail = make(map[string]map[string]string)
	}
	r.failureCodes[ref.AttemptID] = code
	r.failureDetail[ref.AttemptID] = detail
	if r.failFailed {
		return errors.New("failed unavailable")
	}
	return nil
}

func (r *recordingDispatchReporter) PublishCancelled(
	_ context.Context,
	ref jobExecutionRef,
	_ string,
) error {
	r.record("cancelled", ref)
	if r.cancelBlock != nil {
		<-r.cancelBlock
	}
	return nil
}

func (r *recordingDispatchReporter) count(kind, attemptID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := kind + ":" + attemptID
	count := 0
	for _, event := range r.events {
		if event == want {
			count++
		}
	}
	return count
}

func (r *recordingDispatchReporter) failure(attemptID string) (string, map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failureCodes[attemptID], r.failureDetail[attemptID]
}

func newAdmissionTestBridge(
	maximum uint32,
	executor dispatchExecutor,
) (*legionJobBridge, *recordingDispatchReporter) {
	agent := &ScanNode{
		manager:           newTaskManager(),
		invokeLimiter:     newInvokeLimiter(maximum),
		maxRunningJobs:    maximum,
		heartbeatInterval: 100 * time.Millisecond,
	}
	reporter := &recordingDispatchReporter{}
	bridge := &legionJobBridge{
		agent:               agent,
		dispatchEvents:      reporter,
		dispatchExecutor:    executor,
		dispatchAdmissions:  newDispatchAdmissionRegistry(),
		nodeIDProvider:      func() string { return "node-a" },
		rootContextProvider: context.Background,
	}
	return bridge, reporter
}

func admissionTestCommand(commandID, jobID, subtaskID, attemptID string) *jobv1.DispatchJobCommand {
	command := validDispatchCommand()
	command.Metadata.CommandId = commandID
	command.Metadata.IssuedAt = timestamppb.Now()
	command.Job.JobId = jobID
	command.Job.SubtaskId = subtaskID
	command.Job.AttemptId = attemptID
	return command
}

func marshalAdmissionCommand(t *testing.T, command *jobv1.DispatchJobCommand) []byte {
	t.Helper()
	raw, err := proto.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func waitForAdmission(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestDispatchStartsOnlyAfterAckSyncCallback(t *testing.T) {
	executed := make(chan struct{}, 1)
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executed <- struct{}{}
		return &ScriptExecutionResult{}, nil
	})
	command := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, command))
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageAckSync || disposition.afterAck == nil {
		t.Fatalf("disposition = %+v, want AckSync with after-ack callback", disposition)
	}
	if reporter.count("claimed", "attempt-1") != 1 || reporter.count("started", "attempt-1") != 0 {
		t.Fatalf("events before AckSync callback = %+v", reporter.events)
	}
	select {
	case <-executed:
		t.Fatal("executor ran before AckSync callback")
	default:
	}
	disposition.afterAck()
	waitForAdmission(t, func() bool { return reporter.count("started", "attempt-1") == 1 }, "started was not published")
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("executor did not run after started")
	}
	waitForAdmission(t, func() bool {
		return bridge.agent.invokeLimiter.activeCount() == 0 && bridge.agent.manager.Count() == 0
	}, "successful dispatch did not release its slot and active attempt")
}

func TestDispatchAckSyncErrorRollsBackSlotAndRetriesConfirmation(t *testing.T) {
	executed := make(chan struct{}, 1)
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executed <- struct{}{}
		return &ScriptExecutionResult{}, nil
	})
	command := admissionTestCommand("cmd-ack", "job-ack", "subtask-ack", "attempt-ack")
	raw := marshalAdmissionCommand(t, command)
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if bridge.agent.invokeLimiter.activeCount() != 1 || bridge.agent.manager.Count() != 1 {
		t.Fatal("dispatch was not registered before AckSync")
	}
	if err := applyMessageDisposition(&nats.Msg{}, disposition); err == nil {
		t.Fatal("unbound message AckSync unexpectedly succeeded")
	}
	if bridge.agent.invokeLimiter.activeCount() != 0 || bridge.agent.manager.Count() != 0 {
		t.Fatal("AckSync error leaked slot or active attempt")
	}
	if reporter.count("claimed", "attempt-ack") != 1 || reporter.count("started", "attempt-ack") != 0 {
		t.Fatalf("events after AckSync error = %+v", reporter.events)
	}

	retry, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if retry.kind != messageAckSync || retry.afterAck == nil {
		t.Fatalf("retry disposition = %+v, want AckSync", retry)
	}
	if reporter.count("claimed", "attempt-ack") != 1 {
		t.Fatal("AckSync retry republished claimed")
	}
	retry.afterAck()
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("dispatch did not execute after retry confirmation")
	}
}

func TestMaxOneTwoDispatchesPublishAtMostOneStarted(t *testing.T) {
	releaseExecution := make(chan struct{})
	var executions atomic.Int32
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executions.Add(1)
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{}
		}
	})
	first := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	firstDisposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, first))
	if err != nil {
		t.Fatal(err)
	}
	firstDisposition.afterAck()
	waitForAdmission(t, func() bool { return reporter.count("started", "attempt-1") == 1 }, "first dispatch did not start")

	second := admissionTestCommand("cmd-2", "job-2", "subtask-2", "attempt-2")
	secondDisposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, second))
	if err != nil {
		t.Fatal(err)
	}
	if secondDisposition.kind != messageNakDelayed || secondDisposition.delay != time.Second {
		t.Fatalf("second disposition = %+v, want delayed NAK", secondDisposition)
	}
	if reporter.count("claimed", "attempt-2") != 0 || reporter.count("started", "attempt-2") != 0 {
		t.Fatalf("full dispatch emitted lifecycle events: %+v", reporter.events)
	}
	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want 1", executions.Load())
	}
	close(releaseExecution)
}

func TestDispatchSameIdentityRedeliveryExecutesOnceAndConflictTerms(t *testing.T) {
	releaseExecution := make(chan struct{})
	var executions atomic.Int32
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executions.Add(1)
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{}
		}
	})
	command := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	raw := marshalAdmissionCommand(t, command)
	first, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.kind != messageAckSync || duplicate.afterAck == nil {
		t.Fatalf("duplicate disposition = %+v", duplicate)
	}
	first.afterAck()
	duplicate.afterAck()
	waitForAdmission(t, func() bool { return reporter.count("started", "attempt-1") == 1 }, "same-ID dispatch did not start")
	if executions.Load() != 1 || reporter.count("claimed", "attempt-1") != 1 {
		t.Fatalf("same-ID counts: executions=%d events=%+v", executions.Load(), reporter.events)
	}

	conflictingAttempt := proto.Clone(command).(*jobv1.DispatchJobCommand)
	conflictingAttempt.Metadata.CommandId = "cmd-other"
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, conflictingAttempt)); err != nil || disposition.kind != messageTerm {
		t.Fatalf("attempt conflict disposition=%+v err=%v", disposition, err)
	}
	conflictingCommand := proto.Clone(command).(*jobv1.DispatchJobCommand)
	conflictingCommand.Job.AttemptId = "attempt-other"
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, conflictingCommand)); err != nil || disposition.kind != messageTerm {
		t.Fatalf("command conflict disposition=%+v err=%v", disposition, err)
	}
	close(releaseExecution)
}

func TestFullDispatchNAKExpiresAtIssuedHeartbeatDeadline(t *testing.T) {
	releaseExecution := make(chan struct{})
	bridge, _ := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{}
		}
	})
	bridge.agent.heartbeatInterval = 40 * time.Millisecond
	first := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	firstDisposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, first))
	if err != nil {
		t.Fatal(err)
	}
	firstDisposition.afterAck()
	waitForAdmission(t, func() bool { return bridge.agent.invokeLimiter.activeCount() == 1 }, "first slot was not occupied")
	second := admissionTestCommand("cmd-2", "job-2", "subtask-2", "attempt-2")
	raw := marshalAdmissionCommand(t, second)
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageNakDelayed {
		t.Fatalf("full disposition=%+v err=%v", disposition, err)
	}
	time.Sleep(60 * time.Millisecond)
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageTerm {
		t.Fatalf("expired disposition=%+v err=%v", disposition, err)
	}
	close(releaseExecution)
}

// TestFullDispatchNAKExpiryPublishesCapacityFailure asserts the expired
// full-capacity redelivery does not terminate silently: the node must report
// node_capacity_exceeded so the platform records the real cause instead of
// reclaiming the attempt later as attempt_missing_from_heartbeat.
func TestFullDispatchNAKExpiryPublishesCapacityFailure(t *testing.T) {
	releaseExecution := make(chan struct{})
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{}
		}
	})
	bridge.agent.heartbeatInterval = 40 * time.Millisecond
	first := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	firstDisposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, first))
	if err != nil {
		t.Fatal(err)
	}
	firstDisposition.afterAck()
	waitForAdmission(t, func() bool { return bridge.agent.invokeLimiter.activeCount() == 1 }, "first slot was not occupied")
	second := admissionTestCommand("cmd-2", "job-2", "subtask-2", "attempt-2")
	raw := marshalAdmissionCommand(t, second)
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageNakDelayed {
		t.Fatalf("full disposition=%+v err=%v", disposition, err)
	}
	if code, _ := reporter.failure("attempt-2"); code != "" {
		t.Fatalf("failure published before deadline expiry: %s", code)
	}
	time.Sleep(60 * time.Millisecond)
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageTerm {
		t.Fatalf("expired disposition=%+v err=%v", disposition, err)
	}
	code, detail := reporter.failure("attempt-2")
	if code != dispatchCapacityFailureCode {
		t.Fatalf("expired capacity drop code = %q, want %q", code, dispatchCapacityFailureCode)
	}
	if detail["script_release_id"] != "release-a" {
		t.Fatalf("failure detail missing dispatch context: %#v", detail)
	}
	close(releaseExecution)
}

// TestFirstDispatchCapacityExpiryPublishesCapacityFailure covers the
// first-delivery variant: the admission deadline is already expired when the
// command arrives and the only execution slot is occupied, so the node must
// publish the capacity failure instead of terminating quietly.
func TestFirstDispatchCapacityExpiryPublishesCapacityFailure(t *testing.T) {
	bridge, reporter := newAdmissionTestBridge(1, nil)
	release, acquired := bridge.agent.invokeLimiter.TryAcquire()
	if !acquired {
		t.Fatal("pre-acquiring the only execution slot must succeed")
	}
	defer release()

	command := admissionTestCommand("cmd-cap", "job-cap", "subtask-cap", "attempt-cap")
	command.Metadata.IssuedAt = timestamppb.New(time.Now().Add(-time.Minute))
	disposition, err := bridge.handleDispatch(
		context.Background(),
		"session-a",
		marshalAdmissionCommand(t, command),
	)
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageTerm {
		t.Fatalf("disposition=%+v, want term after capacity expiry", disposition)
	}
	code, _ := reporter.failure("attempt-cap")
	if code != dispatchCapacityFailureCode {
		t.Fatalf("failure code = %q, want %q", code, dispatchCapacityFailureCode)
	}
	if count := reporter.count("failed", "attempt-cap"); count != 1 {
		t.Fatalf("expected exactly one failure event, got %d", count)
	}
}

// TestCapacityFailurePublishErrorNAKsForRetry keeps the report deliverable:
// when publishing the capacity failure fails, the message must be NAKed and
// the reservation must stay pending so the next redelivery retries the report
// instead of acking it away as a terminal reservation.
func TestCapacityFailurePublishErrorNAKsForRetry(t *testing.T) {
	bridge, reporter := newAdmissionTestBridge(1, nil)
	reporter.failFailed = true
	release, acquired := bridge.agent.invokeLimiter.TryAcquire()
	if !acquired {
		t.Fatal("pre-acquiring the only execution slot must succeed")
	}
	defer release()

	command := admissionTestCommand("cmd-cap", "job-cap", "subtask-cap", "attempt-cap")
	command.Metadata.IssuedAt = timestamppb.New(time.Now().Add(-time.Minute))
	disposition, err := bridge.handleDispatch(
		context.Background(),
		"session-a",
		marshalAdmissionCommand(t, command),
	)
	if err == nil {
		t.Fatal("expected the publish error to surface")
	}
	if disposition.kind != messageNak {
		t.Fatalf("disposition=%+v, want NAK so redelivery retries the report", disposition)
	}
	state, _, _, _ := bridge.admissions().Snapshot(
		bridge.admissions().byAttempt["attempt-cap"],
	)
	if state != dispatchAdmissionPending {
		t.Fatalf("reservation state = %v, want pending so the report can be retried", state)
	}

	// The next redelivery must retry the report and terminate on success.
	reporter.failFailed = false
	disposition, err = bridge.handleDispatch(
		context.Background(),
		"session-a",
		marshalAdmissionCommand(t, command),
	)
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageTerm {
		t.Fatalf("redelivery disposition=%+v, want term after report succeeds", disposition)
	}
	if code, _ := reporter.failure("attempt-cap"); code != dispatchCapacityFailureCode {
		t.Fatalf("redelivery failure code = %q, want %q", code, dispatchCapacityFailureCode)
	}
}

func TestLateFirstDispatchUsesFreeSlotDespiteAdmissionDeadline(t *testing.T) {
	executed := make(chan struct{}, 1)
	bridge, _ := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executed <- struct{}{}
		return &ScriptExecutionResult{}, nil
	})
	bridge.agent.heartbeatInterval = 10 * time.Millisecond
	command := admissionTestCommand("cmd-late", "job-late", "subtask-late", "attempt-late")
	command.Metadata.IssuedAt = timestamppb.New(time.Now().Add(-time.Minute))

	disposition, err := bridge.handleDispatch(
		context.Background(),
		"session-a",
		marshalAdmissionCommand(t, command),
	)
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageAckSync || disposition.afterAck == nil {
		t.Fatalf("late first delivery disposition = %+v, want AckSync", disposition)
	}
	disposition.afterAck()
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("late first delivery did not execute with a free slot")
	}
}

func TestDispatchFromReplacedSessionTerms(t *testing.T) {
	bridge, _ := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		return &ScriptExecutionResult{}, nil
	})
	command := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	raw := marshalAdmissionCommand(t, command)
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageAckSync {
		t.Fatalf("initial disposition=%+v err=%v", disposition, err)
	}
	bridge.switchDispatchSession("session-b")
	if disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw); err != nil || disposition.kind != messageTerm {
		t.Fatalf("stale session disposition=%+v err=%v", disposition, err)
	}
}

func TestCancelBeforeStartCreatesSessionTombstoneAndDoesNotExecute(t *testing.T) {
	releaseExecution := make(chan struct{})
	var executions atomic.Int32
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, input ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executions.Add(1)
		if input.RuntimeID == "attempt-1" {
			select {
			case <-releaseExecution:
				return &ScriptExecutionResult{}, nil
			case <-task.Ctx.Done():
				return nil, &TaskCancelledError{}
			}
		}
		return nil, fmt.Errorf("cancelled attempt unexpectedly executed")
	})
	first := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	firstDisposition, _ := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, first))
	firstDisposition.afterAck()
	waitForAdmission(t, func() bool { return bridge.agent.invokeLimiter.activeCount() == 1 }, "first slot was not occupied")
	second := admissionTestCommand("cmd-2", "job-2", "subtask-2", "attempt-2")
	raw := marshalAdmissionCommand(t, second)
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil || disposition.kind != messageNakDelayed {
		t.Fatalf("pending disposition=%+v err=%v", disposition, err)
	}
	cancelRaw, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job:    second.Job,
		Reason: "cancel before capacity",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.handleCancel(cancelRaw); err != nil {
		t.Fatal(err)
	}
	waitForAdmission(t, func() bool { return reporter.count("cancelled", "attempt-2") == 1 }, "pending cancellation was not published")
	redelivery, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil || redelivery.kind != messageAckSync || redelivery.afterAck != nil {
		t.Fatalf("cancel tombstone disposition=%+v err=%v", redelivery, err)
	}
	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want only the first dispatch", executions.Load())
	}
	close(releaseExecution)
}

func TestCancelBeforeFirstDispatchTombstonesAttempt(t *testing.T) {
	var executions atomic.Int32
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executions.Add(1)
		return &ScriptExecutionResult{}, nil
	})
	command := admissionTestCommand("cmd-late", "job-late", "subtask-late", "attempt-late")
	cancelRaw, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job:    command.Job,
		Reason: "cancel before dispatch delivery",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.handleCancelForSession("session-a", cancelRaw); err != nil {
		t.Fatal(err)
	}

	raw := marshalAdmissionCommand(t, command)
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageAckSync || disposition.afterAck != nil {
		t.Fatalf("late cancelled dispatch disposition = %+v", disposition)
	}
	if executions.Load() != 0 || reporter.count("claimed", "attempt-late") != 0 ||
		reporter.count("started", "attempt-late") != 0 || reporter.count("cancelled", "attempt-late") != 1 {
		t.Fatalf("late cancelled dispatch executed or emitted wrong events: executions=%d events=%+v", executions.Load(), reporter.events)
	}
	redelivery, err := bridge.handleDispatch(context.Background(), "session-a", raw)
	if err != nil || redelivery.kind != messageAckSync || reporter.count("cancelled", "attempt-late") != 1 {
		t.Fatalf("cancelled redelivery disposition=%+v err=%v events=%+v", redelivery, err, reporter.events)
	}
}

func TestCancelTargetsAttemptWithoutCancellingSameSubtaskRetry(t *testing.T) {
	releaseNew := make(chan struct{})
	bridge, reporter := newAdmissionTestBridge(2, func(task *Task, input ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		if input.RuntimeID == "attempt-new" {
			select {
			case <-releaseNew:
				return &ScriptExecutionResult{}, nil
			case <-task.Ctx.Done():
				return nil, &TaskCancelledError{Reason: task.CancelReason()}
			}
		}
		<-task.Ctx.Done()
		return nil, &TaskCancelledError{Reason: task.CancelReason()}
	})
	oldCommand := admissionTestCommand("cmd-old", "job-shared", "subtask-shared", "attempt-old")
	newCommand := admissionTestCommand("cmd-new", "job-shared", "subtask-shared", "attempt-new")
	startAdmissionDispatch(t, bridge, oldCommand)
	startAdmissionDispatch(t, bridge, newCommand)
	waitForAdmission(t, func() bool { return bridge.agent.invokeLimiter.activeCount() == 2 }, "both attempts did not start")

	cancelRaw, err := proto.Marshal(&jobv1.CancelJobCommand{Job: oldCommand.Job, Reason: "cancel old only"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.handleCancelForSession("session-a", cancelRaw); err != nil {
		t.Fatal(err)
	}
	waitForAdmission(t, func() bool { return reporter.count("cancelled", "attempt-old") == 1 }, "old attempt was not cancelled")
	if _, err := bridge.agent.manager.GetTaskByAttemptID("attempt-new"); err != nil {
		t.Fatal("cancel for old attempt removed the new retry")
	}
	if reporter.count("cancelled", "attempt-new") != 0 || bridge.agent.invokeLimiter.activeCount() != 1 {
		t.Fatalf("new retry was affected: events=%+v active=%d", reporter.events, bridge.agent.invokeLimiter.activeCount())
	}
	close(releaseNew)
}

func TestStaleSessionCancelDoesNotAffectCurrentAttempt(t *testing.T) {
	releaseExecution := make(chan struct{})
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{Reason: task.CancelReason()}
		}
	})
	bridge.switchDispatchSession("session-a")
	bridge.switchDispatchSession("session-b")
	current := admissionTestCommand("cmd-current", "job-current", "subtask-shared", "attempt-current")
	disposition, err := bridge.handleDispatch(context.Background(), "session-b", marshalAdmissionCommand(t, current))
	if err != nil {
		t.Fatal(err)
	}
	disposition.afterAck()
	waitForAdmission(t, func() bool { return reporter.count("started", "attempt-current") == 1 }, "current attempt did not start")

	staleCancel, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job: &jobv1.JobRef{
			JobId:     "job-old",
			SubtaskId: "subtask-shared",
			AttemptId: "attempt-old",
		},
		Reason: "stale session cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.handleCancelForSession("session-a", staleCancel); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.agent.manager.GetTaskByAttemptID("attempt-current"); err != nil {
		t.Fatal("stale-session cancel removed current attempt")
	}
	if reporter.count("cancelled", "attempt-current") != 0 || bridge.agent.invokeLimiter.activeCount() != 1 {
		t.Fatalf("stale-session cancel affected current attempt: events=%+v active=%d", reporter.events, bridge.agent.invokeLimiter.activeCount())
	}
	close(releaseExecution)
}

func TestSessionScopedCancelNeverFallsBackToUnregisteredTask(t *testing.T) {
	bridge, _ := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		return &ScriptExecutionResult{}, nil
	})
	bridge.switchDispatchSession("session-a")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := newScriptTask(ctx, cancel, "legacy-task", "job-a", "subtask-a", "attempt-a")
	if _, loaded, accepted := bridge.agent.manager.LoadOrStoreAttempt(task); !accepted || loaded {
		t.Fatal("unregistered TaskManager fixture was not added")
	}
	cancelRaw, err := proto.Marshal(&jobv1.CancelJobCommand{
		Job: &jobv1.JobRef{
			JobId:     "job-a",
			SubtaskId: "subtask-a",
			AttemptId: "attempt-a",
		},
		Reason: "session-scoped cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.handleCancelForSession("session-a", cancelRaw); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal("session-scoped cancel used the unfenced TaskManager fallback")
	default:
	}
	bridge.agent.manager.RemoveAttempt(task.AttemptID)
}

func TestDispatchFailurePanicCancelAndShutdownReleaseSlot(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
			return nil, errors.New("execution failed")
		})
		startAdmissionDispatch(t, bridge, admissionTestCommand("cmd-f", "job-f", "sub-f", "attempt-f"))
		waitForAdmission(t, func() bool { return reporter.count("failed", "attempt-f") == 1 && bridge.agent.manager.Count() == 0 }, "failure did not drain")
		if bridge.agent.invokeLimiter.activeCount() != 0 {
			t.Fatal("failure leaked slot")
		}
	})

	t.Run("rule snapshot preparation failure", func(t *testing.T) {
		bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
			return nil, &ruleSnapshotPreparationError{
				Expectation: RuleSnapshotExpectation{
					SnapshotID:    "snapshot-a",
					ContentSHA256: "sha256-a",
				},
				Err: errors.New("snapshot unavailable"),
			}
		})
		startAdmissionDispatch(t, bridge, admissionTestCommand("cmd-rs", "job-rs", "sub-rs", "attempt-rs"))
		waitForAdmission(t, func() bool {
			return reporter.count("failed", "attempt-rs") == 1 && bridge.agent.manager.Count() == 0
		}, "rule snapshot preparation failure did not drain")
		if bridge.agent.invokeLimiter.activeCount() != 0 {
			t.Fatal("rule snapshot preparation failure leaked slot")
		}
		code, detail := reporter.failure("attempt-rs")
		if code != "rule_snapshot_prepare_failed" {
			t.Fatalf("failure code = %q, want rule_snapshot_prepare_failed", code)
		}
		if detail["rule_snapshot_id"] != "snapshot-a" || detail["rule_snapshot_content_sha256"] != "sha256-a" {
			t.Fatalf("failure detail = %#v, want snapshot identity", detail)
		}
	})

	t.Run("panic", func(t *testing.T) {
		bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
			panic("boom")
		})
		startAdmissionDispatch(t, bridge, admissionTestCommand("cmd-p", "job-p", "sub-p", "attempt-p"))
		waitForAdmission(t, func() bool { return reporter.count("failed", "attempt-p") == 1 && bridge.agent.manager.Count() == 0 }, "panic did not drain")
		if bridge.agent.invokeLimiter.activeCount() != 0 {
			t.Fatal("panic leaked slot")
		}
	})

	for _, mode := range []string{"cancel", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
				<-task.Ctx.Done()
				return nil, &TaskCancelledError{Reason: task.CancelReason()}
			})
			command := admissionTestCommand("cmd-"+mode, "job-"+mode, "sub-"+mode, "attempt-"+mode)
			startAdmissionDispatch(t, bridge, command)
			waitForAdmission(t, func() bool { return reporter.count("started", command.Job.AttemptId) == 1 }, "dispatch did not start")
			if mode == "cancel" {
				raw, _ := proto.Marshal(&jobv1.CancelJobCommand{Job: command.Job, Reason: "stop"})
				if err := bridge.handleCancel(raw); err != nil {
					t.Fatal(err)
				}
			} else {
				bridge.BeginShutdown()
			}
			waitForAdmission(t, func() bool {
				return reporter.count("cancelled", command.Job.AttemptId) == 1 && bridge.agent.manager.Count() == 0
			}, mode+" did not drain")
			if bridge.agent.invokeLimiter.activeCount() != 0 {
				t.Fatalf("%s leaked slot", mode)
			}
		})
	}
}

func TestStartedPublishFailureIsBoundedAndDoesNotExecute(t *testing.T) {
	var executions atomic.Int32
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		executions.Add(1)
		return &ScriptExecutionResult{}, nil
	})
	reporter.failStarted = true
	startAdmissionDispatch(t, bridge, admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1"))
	waitForAdmission(t, func() bool { return reporter.count("failed", "attempt-1") == 1 && bridge.agent.manager.Count() == 0 }, "started failure did not terminate")
	if got := reporter.count("started", "attempt-1"); got != dispatchEventRetryAttempts {
		t.Fatalf("started attempts = %d, want %d", got, dispatchEventRetryAttempts)
	}
	if executions.Load() != 0 || bridge.agent.invokeLimiter.activeCount() != 0 {
		t.Fatalf("started failure executed=%d active=%d", executions.Load(), bridge.agent.invokeLimiter.activeCount())
	}
}

func TestTerminalPublishFailureRetriesBoundedlyThenRemovesActiveAttempt(t *testing.T) {
	bridge, reporter := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		return &ScriptExecutionResult{}, nil
	})
	reporter.failSucceeded = true
	startAdmissionDispatch(t, bridge, admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1"))
	waitForAdmission(t, func() bool { return bridge.agent.manager.Count() == 0 }, "terminal retry did not remove active attempt")
	if got := reporter.count("succeeded", "attempt-1"); got != dispatchEventRetryAttempts {
		t.Fatalf("terminal attempts = %d, want %d", got, dispatchEventRetryAttempts)
	}
	if bridge.agent.invokeLimiter.activeCount() != 0 {
		t.Fatal("terminal publish failure leaked slot")
	}
}

func TestTerminalReservationCompactsAndPrunes(t *testing.T) {
	bridge, _ := newAdmissionTestBridge(1, func(*Task, ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		return &ScriptExecutionResult{}, nil
	})
	command := admissionTestCommand("cmd-old", "job-old", "subtask-old", "attempt-old")
	startAdmissionDispatch(t, bridge, command)
	waitForAdmission(t, func() bool { return bridge.agent.manager.Count() == 0 }, "dispatch did not finish")

	registry := bridge.admissions()
	registry.mu.Lock()
	old := registry.byAttempt["attempt-old"]
	if old == nil || old.task != nil || old.release != nil {
		registry.mu.Unlock()
		t.Fatal("terminal reservation retained execution objects")
	}
	old.finishedAt = time.Now().Add(-dispatchReservationRetention - time.Minute)
	registry.mu.Unlock()

	newCommand := admissionTestCommand("cmd-new", "job-new", "subtask-new", "attempt-new")
	if _, result := registry.Reserve(
		"session-a",
		jobExecutionRefFromCommand(newCommand),
		"identity-new",
		time.Now().Add(time.Minute),
	); result != dispatchReserved {
		t.Fatalf("new reservation result = %v", result)
	}
	registry.mu.Lock()
	_, retained := registry.byAttempt["attempt-old"]
	registry.mu.Unlock()
	if retained {
		t.Fatal("expired terminal tombstone was not pruned")
	}
}

func TestFullAdmissionAndPendingCancelDoNotBlockConsumer(t *testing.T) {
	releaseExecution := make(chan struct{})
	cancelPublisher := make(chan struct{})
	bridge, reporter := newAdmissionTestBridge(1, func(task *Task, _ ScriptExecutionRequest) (*ScriptExecutionResult, error) {
		select {
		case <-releaseExecution:
			return &ScriptExecutionResult{}, nil
		case <-task.Ctx.Done():
			return nil, &TaskCancelledError{}
		}
	})
	reporter.cancelBlock = cancelPublisher
	first := admissionTestCommand("cmd-1", "job-1", "subtask-1", "attempt-1")
	startAdmissionDispatch(t, bridge, first)
	waitForAdmission(t, func() bool { return bridge.agent.invokeLimiter.activeCount() == 1 }, "first slot was not occupied")
	second := admissionTestCommand("cmd-2", "job-2", "subtask-2", "attempt-2")
	startedAt := time.Now()
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, second))
	if err != nil || disposition.kind != messageNakDelayed || time.Since(startedAt) > 100*time.Millisecond {
		t.Fatalf("full admission blocked: disposition=%+v err=%v elapsed=%s", disposition, err, time.Since(startedAt))
	}
	cancelRaw, _ := proto.Marshal(&jobv1.CancelJobCommand{Job: second.Job, Reason: "stop pending"})
	startedAt = time.Now()
	if err := bridge.handleCancel(cancelRaw); err != nil || time.Since(startedAt) > 100*time.Millisecond {
		t.Fatalf("pending cancel blocked consumer: err=%v elapsed=%s", err, time.Since(startedAt))
	}
	close(cancelPublisher)
	close(releaseExecution)
}

func startAdmissionDispatch(t *testing.T, bridge *legionJobBridge, command *jobv1.DispatchJobCommand) {
	t.Helper()
	disposition, err := bridge.handleDispatch(context.Background(), "session-a", marshalAdmissionCommand(t, command))
	if err != nil {
		t.Fatal(err)
	}
	if disposition.kind != messageAckSync || disposition.afterAck == nil {
		t.Fatalf("start disposition = %+v", disposition)
	}
	disposition.afterAck()
}
