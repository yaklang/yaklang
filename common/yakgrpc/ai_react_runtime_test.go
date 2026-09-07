package yakgrpc

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReActEventDeliveryKeepsNormalOutputBestEffortAndReportsSyncFailure(t *testing.T) {
	deliveryErr := errors.New("subscriber delivery failed")
	normalErrors := make(chan error, 1)
	delivery := newReActEventDelivery(context.Background(), func(*schema.AiOutputEvent) error {
		return deliveryErr
	}, func(err error) {
		normalErrors <- err
	})
	t.Cleanup(delivery.cancel)

	event := &schema.AiOutputEvent{}
	delivery.deliver(event)
	require.ErrorIs(t, <-normalErrors, deliveryErr)
	require.Positive(t, event.Timestamp)

	err := delivery.deliverWithResult(&schema.AiOutputEvent{IsSync: true})
	require.ErrorIs(t, err, deliveryErr)
	select {
	case err := <-normalErrors:
		t.Fatalf("synchronous delivery error was also reported as a normal output error: %v", err)
	default:
	}
}

func TestAttachedReActConnectionCloseWaitsForTaskCleanup(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task", "input", context.Background(), nil)
	task.SetUserInputUUID("scheduled-input")
	task.SetStatus(aicommon.AITaskState_Completed)
	react := &aireact.ReAct{RuntimeTasks: []aicommon.AIStatefulTask{task}}
	connection := &reActConnection{
		react:       react,
		reservation: &sessionReservation{},
		delivery:    newReActEventDelivery(context.Background(), nil, nil),
		done:        make(chan struct{}),
		inputUUIDs:  []string{"scheduled-input"},
	}

	closed := make(chan error, 1)
	go func() { closed <- connection.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("connection closed before its task cleanup finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	react.UpdateRuntimeTaskMutex.Lock()
	react.RuntimeTasks = nil
	react.UpdateRuntimeTaskMutex.Unlock()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("connection did not close after its task left the runtime")
	}
}

func TestReActSessionRuntimeHonorsProcessWideStartReservation(t *testing.T) {
	const sessionID = "runtime-global-start-reservation"
	release, ok := aireact.TryBeginSessionStart(sessionID)
	require.True(t, ok)
	defer release()

	runtime := newReActSessionRuntime(nil)
	var createCount atomic.Int32
	runtime.newReAct = func(...aicommon.ConfigOption) (*aireact.ReAct, error) {
		createCount.Add(1)
		return nil, errors.New("must not create while the process-wide start reservation is held")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := runtime.Connect(ctx, ConnectRequest{StartParams: &ypb.AIStartParams{TimelineSessionID: sessionID}}, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, createCount.Load())
}

func TestReActSessionRuntimeCoordinationBusyReasons(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*reActSessionRuntime, string)
		want  reActSessionBusyReason
	}{
		{
			name: "available",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{}
			},
			want: reActSessionAvailable,
		},
		{
			name: "non_task_input_admission_is_available_to_scheduler",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{admitting: 1}
			},
			want: reActSessionAvailable,
		},
		{
			name: "runtime_quiescing",
			setup: func(runtime *reActSessionRuntime, _ string) {
				runtime.allQuiescing = true
			},
			want: reActSessionBusyRuntimeQuiescing,
		},
		{
			name: "session_quiescing",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{quiescing: true}
			},
			want: reActSessionBusySessionQuiescing,
		},
		{
			name: "reserved",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{reservation: &sessionReservation{}}
			},
			want: reActSessionBusyReserved,
		},
		{
			name: "starting",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{starting: &reActSessionStart{}}
			},
			want: reActSessionBusyStarting,
		},
		{
			name: "task_admitting",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{admitting: 1, taskAdmitting: 1}
			},
			want: reActSessionBusyTaskAdmitting,
		},
		{
			name: "owned_runtime_transition",
			setup: func(runtime *reActSessionRuntime, sessionID string) {
				runtime.entries[sessionID] = &reActSessionState{
					runtime: &ownedReActRuntime{react: &aireact.ReAct{}},
				}
			},
			want: reActSessionBusyRuntimeTransition,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newReActSessionRuntime(nil)
			sessionID := "coordination-busy-reason-" + test.name
			test.setup(runtime, sessionID)

			require.Equal(t, test.want, runtime.sessionCoordinationBusyReason(sessionID))
			require.Equal(t, test.want != reActSessionAvailable, runtime.IsSessionBusy(sessionID))
		})
	}
}

func TestReActSessionRuntimeWillNotQuiesceAnotherRuntimeStart(t *testing.T) {
	const sessionID = "runtime-external-start-reservation"
	release, ok := aireact.TryBeginSessionStart(sessionID)
	require.True(t, ok)

	runtime := newReActSessionRuntime(nil)
	_, err := runtime.QuiesceSessions(context.Background(), []string{sessionID})
	require.ErrorContains(t, err, "starting outside this runtime")
	require.True(t, aireact.IsSessionStarting(sessionID))
	release()

	const allSessionID = "runtime-external-start-reservation-all"
	releaseAll, ok := aireact.TryBeginSessionStart(allSessionID)
	require.True(t, ok)
	defer releaseAll()
	_, err = runtime.QuiesceAll(context.Background())
	require.ErrorContains(t, err, "starting outside this runtime")
	require.True(t, aireact.IsSessionStarting(allSessionID))
}

func TestReActSessionRuntimeReservationAndQuiescence(t *testing.T) {
	runtime := newReActSessionRuntime(nil)
	reservation, err := runtime.ReserveSession(context.Background(), "lease-session", "execution-1")
	require.NoError(t, err)
	require.True(t, runtime.IsSessionBusy("lease-session"))
	_, err = runtime.ReserveSession(context.Background(), "lease-session", "execution-2")
	require.Error(t, err)

	result := make(chan struct {
		guard SessionQuiescence
		err   error
	}, 1)
	go func() {
		guard, quiesceErr := runtime.QuiesceSessions(context.Background(), []string{"lease-session"})
		result <- struct {
			guard SessionQuiescence
			err   error
		}{guard: guard, err: quiesceErr}
	}()

	select {
	case <-reservation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("quiescence did not cancel the reservation owner")
	}
	select {
	case <-result:
		t.Fatal("quiescence returned before the execution lease was released")
	default:
	}

	reservation.Release()
	var quiesced struct {
		guard SessionQuiescence
		err   error
	}
	select {
	case quiesced = <-result:
	case <-time.After(time.Second):
		t.Fatal("quiescence did not finish after lease release")
	}
	require.NoError(t, quiesced.err)
	require.NotNil(t, quiesced.guard)
	_, err = runtime.ReserveSession(context.Background(), "lease-session", "execution-3")
	require.Error(t, err, "the deletion guard must reject restarts")

	quiesced.guard.Release()
	next, err := runtime.ReserveSession(context.Background(), "lease-session", "execution-4")
	require.NoError(t, err)
	next.Release()
	require.False(t, runtime.IsSessionBusy("lease-session"))
}

func TestReActSessionRuntimeOnlyTaskAdmissionBlocksReservation(t *testing.T) {
	runtime := newReActSessionRuntime(nil)
	runtime.entries["input-admission"] = &reActSessionState{admitting: 1}
	reservation, err := runtime.ReserveSession(context.Background(), "input-admission", "execution")
	require.NoError(t, err, "non-task sync/hot-patch admission must not make a schedule skip")
	reservation.Release()

	runtime.entries["task-admission"] = &reActSessionState{admitting: 1, taskAdmitting: 1}
	_, err = runtime.ReserveSession(context.Background(), "task-admission", "execution")
	require.Error(t, err, "free-input admission must remain atomic with schedule reservation")
}

func TestServerRetireReActSessionRuntime(t *testing.T) {
	server := newScheduleTestServer(t)
	oldRuntime := server.getReActSessionRuntime().(*reActSessionRuntime)
	reservation, err := oldRuntime.ReserveSession(context.Background(), "old-project-session", "old-execution")
	require.NoError(t, err)

	retired := make(chan error, 1)
	go func() { retired <- server.retireReActSessionRuntime(context.Background()) }()
	select {
	case <-reservation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("retiring the project runtime did not cancel its reservation")
	}
	reservation.Release()
	select {
	case err := <-retired:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("project runtime retirement did not finish")
	}

	_, err = oldRuntime.ReserveSession(context.Background(), "stale-session", "stale-execution")
	require.Error(t, err, "a retired runtime must stay permanently quiesced")
	require.Same(t, oldRuntime, server.getReActSessionRuntime(), "the retired runtime must cover the project-switch window")
	server.resetReActSessionRuntimeAfterProjectSwitch()
	newRuntime := server.getReActSessionRuntime().(*reActSessionRuntime)
	require.NotSame(t, oldRuntime, newRuntime)
	newReservation, err := newRuntime.ReserveSession(context.Background(), "new-project-session", "new-execution")
	require.NoError(t, err)
	newReservation.Release()
}

func TestReActSessionRuntimeCreatesOnceAttachesAndGatesReservedInput(t *testing.T) {
	server := newScheduleTestServer(t)
	runtime := server.getReActSessionRuntime().(*reActSessionRuntime)
	originalFactory := runtime.newReAct
	var createCount atomic.Int32
	factoryEntered := make(chan struct{})
	allowFactory := make(chan struct{})
	runtime.newReAct = func(options ...aicommon.ConfigOption) (*aireact.ReAct, error) {
		if createCount.Add(1) == 1 {
			close(factoryEntered)
		}
		<-allowFactory
		return originalFactory(options...)
	}

	mockKnowledgeManager, _ := aicommon.NewMockEKManagerAndToken()
	options := []aicommon.ConfigOption{
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithTimelineArchiveStore(&noOpTimelineArchiveStore{}),
		aicommon.WithEnhanceKnowledgeManager(mockKnowledgeManager),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithDisableIncreaseIteration(true),
	}
	const sessionID = "runtime-create-once"
	params := &ypb.AIStartParams{TimelineSessionID: sessionID, DisableAISearchForge: true, DisableToolUse: true}
	type connectResult struct {
		connection ReActConnection
		err        error
	}
	firstResult := make(chan connectResult, 1)
	secondResult := make(chan connectResult, 1)
	go func() {
		connection, err := runtime.connectWithOptions(context.Background(), ConnectRequest{StartParams: params}, nil, false, options...)
		firstResult <- connectResult{connection: connection, err: err}
	}()
	select {
	case <-factoryEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first connection did not enter the ReAct factory")
	}
	go func() {
		connection, err := runtime.connectWithOptions(context.Background(), ConnectRequest{StartParams: params}, nil, false, options...)
		secondResult <- connectResult{connection: connection, err: err}
	}()
	close(allowFactory)

	first := <-firstResult
	second := <-secondResult
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, int32(1), createCount.Load())
	require.True(t, first.connection.CreatedRuntime())
	require.False(t, second.connection.CreatedRuntime())
	react := first.connection.(*reActConnection).react

	reservation, err := runtime.ReserveSession(context.Background(), sessionID, "scheduled-execution")
	require.NoError(t, err)
	scheduled, err := runtime.connectWithOptions(context.Background(), ConnectRequest{
		StartParams: params,
		Reservation: reservation,
	}, nil, false, options...)
	require.NoError(t, err)
	require.False(t, scheduled.CreatedRuntime(), "the scheduled execution must attach to the existing frontend runtime")
	require.NoError(t, scheduled.Send(&ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "scheduled input that must be canceled with its connection",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{{
			Type:  aicommon.USER_FREE_INPUT_UUID,
			Value: "scheduled-execution",
		}},
	}))
	require.NoError(t, scheduled.Close())
	require.Eventually(t, func() bool {
		for _, task := range react.GetRuntimeTasks() {
			if task.GetUserInputUUID() == "scheduled-execution" && !task.IsFinished() {
				return false
			}
		}
		for _, task := range react.GetQueueingTasks() {
			if task.GetUserInputUUID() == "scheduled-execution" && !task.IsFinished() {
				return false
			}
		}
		return true
	}, time.Second, 10*time.Millisecond, "closing an attached scheduler connection must cancel only its admitted task")

	blockedSend := make(chan error, 1)
	go func() {
		blockedSend <- second.connection.Send(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "queued after the reservation"})
	}()
	select {
	case err := <-blockedSend:
		t.Fatalf("unreserved input bypassed the scheduler lease: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	reservation.Release()
	select {
	case err := <-blockedSend:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("input did not resume after the scheduler lease was released")
	}
	_, err = runtime.ReserveSession(context.Background(), sessionID, "racing-execution")
	require.Error(t, err, "an accepted input must make the session busy before ReserveSession can win")

	require.NoError(t, second.connection.Close())
	require.NoError(t, first.connection.Close())
	require.Eventually(t, func() bool {
		_, ok := aireact.GetRunningSession(sessionID)
		return !ok
	}, time.Second, 10*time.Millisecond)
}
