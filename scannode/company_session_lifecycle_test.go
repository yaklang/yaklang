package scannode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/node"
)

func TestResilienceCompanyTransportLossCancelsWorkAndFencesLateDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := newDispatchAdmissionRegistry()
	registry.SwitchSession("session-a")
	reservation, result := registry.Reserve("session-a", jobExecutionRef{CommandID: "command-a", AttemptID: "attempt-a"}, "identity", time.Now().Add(time.Hour))
	if result != dispatchReserved {
		t.Fatal(result)
	}
	task := &Task{AttemptID: "attempt-a", Ctx: ctx, Cancel: cancel}
	registry.AttachClaim(reservation, task, nil)
	reservation.state = dispatchAdmissionRunning
	bridge := &legionJobBridge{dispatchAdmissions: registry}
	bridge.invalidateCompanyExecution()
	if ctx.Err() == nil {
		t.Fatal("running task survived transport loss")
	}
	if _, result := registry.Reserve("session-a", jobExecutionRef{CommandID: "command-b", AttemptID: "attempt-b"}, "identity", time.Now()); result != dispatchStaleSession {
		t.Fatal("late dispatch was admitted after invalidation")
	}
	registry.SwitchSession("session-b")
	if _, result := registry.Reserve("session-a", jobExecutionRef{CommandID: "command-c", AttemptID: "attempt-c"}, "identity", time.Now()); result != dispatchStaleSession {
		t.Fatal("old session dispatch admitted after reconnect")
	}
}

func TestResilienceCompanyTransportLossClosesAIAndPendingBinds(t *testing.T) {
	manager := newAISessionRuntimeManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	pendingCtx, pendingCancel := context.WithCancel(context.Background())
	driver := &recordingAISessionRuntimeDriver{}
	manager.sessions["ai-1"] = &aiSessionRuntime{cancel: cancel, bindEpoch: 1, handle: &recordingAISessionRuntimeHandle{driver: driver}}
	manager.bindings["ai-1"] = aiSessionBindReservation{cancel: pendingCancel, epoch: 2}
	manager.closeForTransportLoss()
	if ctx.Err() == nil || pendingCtx.Err() == nil {
		t.Fatal("AI execution or pending bind survived revocation")
	}
	if len(manager.sessions) != 0 || len(manager.bindings) != 0 || len(driver.closes) != 1 {
		t.Fatal("AI state not closed")
	}
	if manager.terminalTombstones["ai-1"].epoch != 2 {
		t.Fatal("newer binding fence was downgraded")
	}
}

func TestResilienceCompanyRuntimeRestartStopsOldContainer(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("b", 64)
	docker := &runtimeHostDockerStub{images: map[string]string{imageID: imageID}, containers: make(map[string]runtimeHostContainer)}
	base := t.TempDir()
	executor := newRuntimeHostTestExecutor(t, docker, base)
	command := runtimeHostTestCommand(t)
	command.Metadata.CompanyId = "company-a"
	session := node.SessionState{NodeID: "node-1", SessionID: "node-session-1"}
	result := executor.execute(context.Background(), command, session)
	if !result.Success {
		t.Fatalf("start: %s", result.ErrorMessage)
	}
	restarted := newRuntimeHostTestExecutor(t, docker, base)
	if docker.stops != 1 || len(docker.containers) != 0 {
		t.Fatal("company container continued after restart without reauthentication")
	}
	if replay := restarted.execute(context.Background(), command, session); replay.Success {
		t.Fatal("old generation restarted")
	}
}

func TestResilienceCompanyExecutionContextIsCanceledOnTransportLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bridge := &legionJobBridge{companyExecutionCtx: ctx, companyExecutionCancel: cancel, companyExecutionSession: "session-a"}
	bridge.companyBound.Store(true)
	transportCtx, stopTransport := context.WithCancel(context.Background())
	execution := bridge.companyExecutionContext(transportCtx)
	stopTransport()
	if execution.Err() != nil {
		t.Fatal("credential rotation canceled the authenticated session execution")
	}
	bridge.invalidateCompanyExecution()
	if execution.Err() == nil || bridge.companyExecutionContext(context.Background()).Err() == nil {
		t.Fatal("revoked session execution remained active")
	}
}
