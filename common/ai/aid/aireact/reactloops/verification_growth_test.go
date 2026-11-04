package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestVerificationGrowthGate(t *testing.T) {
	for _, tt := range []struct {
		name              string
		iteration, tokens int
		want              bool
	}{
		{"soft growth one iteration", 11, 100000 + verificationAutoTriggerMinPromptDelta, false},
		{"soft growth after cooldown", 13, 100000 + verificationAutoTriggerMinPromptDelta, true},
		{"hard growth bypasses cooldown", 11, 100000 + verificationAutoTriggerHardPromptDelta, true},
		{"compression is not new evidence", 13, 20000, false},
		{"missing measurement is not new evidence", 13, 0, false},
		{"final iteration still verifies", 100, 20000, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			invoker := mockcfg.NewMockInvoker(context.Background())
			loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
			loop.periodicVerificationInterval = 3
			loop.maxIterations = 100
			loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
				GeneratedAt: time.Now(), IterationIndex: 10, LoopPromptTokens: 100000,
			})
			require.Equal(t, tt.want, loop.shouldTriggerAutomaticVerification(&VerificationRuntimeSnapshot{
				IterationIndex: tt.iteration, LoopPromptTokens: tt.tokens,
			}))
		})
	}
}

func TestVerificationGrowthGate_RebasesCompressionWithoutRefreshingWatchdog(t *testing.T) {
	invoker := mockcfg.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 3
	previous := &VerificationRuntimeSnapshot{
		GeneratedAt: time.Now().Add(-time.Minute), IterationIndex: 10, LoopPromptTokens: 100000,
	}
	loop.setVerificationRuntimeSnapshot(previous)
	require.False(t, loop.shouldTriggerAutomaticVerification(&VerificationRuntimeSnapshot{
		IterationIndex: 13,
	}))
	require.Same(t, previous, loop.GetVerificationRuntimeSnapshot(), "unknown tokens must not reset the baseline")
	require.False(t, loop.shouldTriggerAutomaticVerification(&VerificationRuntimeSnapshot{
		IterationIndex: 13, LoopPromptTokens: 20000,
	}))
	rebased := loop.GetVerificationRuntimeSnapshot()
	require.Equal(t, previous.GeneratedAt, rebased.GeneratedAt)
	require.Equal(t, previous.IterationIndex, rebased.IterationIndex)
	require.Equal(t, 100000, previous.LoopPromptTokens, "published snapshots remain immutable")
	require.Equal(t, 20000, rebased.LoopPromptTokens)
	require.True(t, loop.shouldTriggerAutomaticVerification(&VerificationRuntimeSnapshot{
		IterationIndex: 14, LoopPromptTokens: 20000 + verificationAutoTriggerMinPromptDelta,
	}), "new evidence after compression should use the smaller baseline")
}

func TestVerificationGrowthGate_SustainedFileReads(t *testing.T) {
	ctx := context.Background()
	invoker := &verificationGateTestInvoker{MockInvoker: mockcfg.NewMockInvoker(ctx)}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval, loop.maxIterations = 3, 100
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt: time.Now(), IterationIndex: 0, LoopPromptTokens: 20000,
	})
	var triggeredAt []int
	for iteration := 1; iteration <= 12; iteration++ {
		loop.currentIterationIndex = iteration
		loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 20000 + iteration*13000})
		_, triggered, err := loop.MaybeVerifyUserSatisfaction(ctx, "audit sources", true, "read_file")
		require.NoError(t, err)
		if triggered {
			triggeredAt = append(triggeredAt, iteration)
		}
	}
	// Each file crosses the soft gate, but two files cross the hard gate.
	// Exercise the production entry point instead of mirroring its predicate.
	require.Equal(t, []int{2, 4, 6, 8, 10, 12}, triggeredAt)
	require.Equal(t, 6, invoker.verifyCalls)
}
