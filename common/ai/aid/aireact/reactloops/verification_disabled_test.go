package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestDisabledPeriodicVerificationCoversToolAndWatchdogPaths(t *testing.T) {
	ctx := context.Background()
	invoker := &verificationGateTestInvoker{MockInvoker: mockcfg.NewMockInvoker(ctx)}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	WithDisablePeriodicVerification(true)(loop)
	task := aicommon.NewStatefulTaskBase("disabled-verification", "query", ctx, nil, true)
	loop.SetCurrentTask(task)
	defer loop.stopVerificationWatchdogForTask(task)

	// Both the unthrottled tool path and a last-iteration checkpoint would
	// otherwise force a call, even though Execute disabled its own timer.
	loop.periodicVerificationInterval = 0
	loop.maxIterations, loop.currentIterationIndex = 1, 1
	result, triggered, err := loop.MaybeVerifyUserSatisfaction(ctx, "query", true, "read_file")
	require.NoError(t, err)
	require.Nil(t, result)
	require.False(t, triggered)
	require.False(t, loop.shouldTriggerAutomaticVerification(loop.buildVerificationRuntimeSnapshot(time.Now())))
	loop.startVerificationWatchdog(task)
	loop.touchVerificationWatchdog()
	require.Nil(t, loop.verificationWatchdogTimer)

	// A queued timer callback must also respect the switch, and rescheduling
	// must clear a previously allocated timer without replacing it.
	loop.verificationWatchdogTimer = time.AfterFunc(time.Hour, func() {})
	loop.verificationMutex.Lock()
	loop.rescheduleVerificationWatchdog(task)
	loop.verificationMutex.Unlock()
	require.Nil(t, loop.verificationWatchdogTimer)
	loop.triggerVerificationWatchdog(task)
	require.Zero(t, invoker.verifyCalls)
	require.Empty(t, invoker.timelineEntries)

	// The switch disables automatic checks, not an explicit verification API.
	result, err = loop.VerifyUserSatisfactionNow(ctx, "query", false, "explicit check")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Nil(t, loop.verificationWatchdogTimer)
}
