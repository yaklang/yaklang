package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Wait for the config events emitted after each option has been applied. This
// also synchronizes direct field assertions with the hot-patch consumer.
func newHotPatchTestConfig(t *testing.T, ctx context.Context, opts ...ConfigOption) (*Config, func(*ypb.AIInputEvent, int)) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	applied := make(chan struct{}, 8)
	opts = append(opts, WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.Type == schema.EVENT_TYPE_AID_CONFIG {
			applied <- struct{}{}
		}
	}))
	c := NewTestConfig(ctx, opts...)
	t.Cleanup(func() {
		cancel()
		c.WaitHotPatchLoopStopped()
	})
	return c, func(event *ypb.AIInputEvent, optionCount int) {
		t.Helper()
		c.EventInputChan.SafeFeed(event)
		for i := 0; i < optionCount; i++ {
			select {
			case <-applied:
			case <-ctx.Done():
				t.Fatalf("hot-patch %s did not finish applying %d options: %v", event.HotpatchType, optionCount, ctx.Err())
			}
		}
	}
}

func TestHotPatchConfig(t *testing.T) {
	// Setup config with epm stub
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	require.True(t, c.AllowRequireForUserInteract)
	require.Equal(t, c.AgreePolicy, AgreePolicyManual)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AllowRequireForUserInteract,
		Params: &ypb.AIStartParams{
			DisallowRequireForUserPrompt: true,
		},
	}, 1)
	require.False(t, c.AllowRequireForUserInteract)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AgreePolicy,
		Params: &ypb.AIStartParams{
			ReviewPolicy: string(AgreePolicyYOLO),
		},
	}, 1)
	require.Equal(t, AgreePolicyYOLO, c.AgreePolicy)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RiskControlScore,
		Params: &ypb.AIStartParams{
			AIReviewRiskControlScore: 0.75,
		},
	}, 1)
	require.Equal(t, 0.75, c.AgreeAIScoreMiddle)
	require.Equal(t, 0.55, c.AgreeAIScoreLow)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: false,
		},
	}, 1)
	require.False(t, c.GetEnablePlanAndExec())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_SyncPerceptionTrigger,
		Params: &ypb.AIStartParams{
			SyncPerceptionTrigger: true,
		},
	}, 1)
	require.True(t, c.GetSyncPerceptionTrigger())
}

func TestWaitHotPatchLoopStoppedWaitsForInFlightOption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewTestConfig(ctx)
	c.StartHotPatchLoop(ctx)

	started := make(chan struct{})
	release := make(chan struct{})
	c.HotPatchOptionChan.SafeFeed(func(*Config) error {
		close(started)
		<-release
		return nil
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("hot-patch option did not start")
	}

	cancel()
	waited := make(chan struct{})
	go func() {
		c.WaitHotPatchLoopStopped()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("hot-patch lifecycle wait returned while an option was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("hot-patch lifecycle wait did not finish after the option returned")
	}
}

func TestConfigHotpatch_PersistSessionStartParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "session-hotpatch-persist"
	c, feed := newHotPatchTestConfig(t, ctx, WithPersistentSessionId(sessionID))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, &ypb.AIStartParams{
		EnablePlan:            false,
		SyncPerceptionTrigger: false,
		TimelineSessionID:     sessionID,
	})
	require.NoError(t, err)
	c.StartEventLoop(ctx)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: true,
		},
	}, 1)

	got, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.True(t, got.GetEnablePlan())
	require.False(t, got.GetSyncPerceptionTrigger())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_SyncPerceptionTrigger,
		Params: &ypb.AIStartParams{
			SyncPerceptionTrigger: true,
		},
	}, 1)

	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.True(t, got.GetEnablePlan())
	require.True(t, got.GetSyncPerceptionTrigger())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: false,
		},
	}, 1)

	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.False(t, got.GetEnablePlan())
	require.True(t, got.GetSyncPerceptionTrigger())
}

func TestHotPatch_ExecutionStrategy_TopLevelAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	// Initially strategy fields are at defaults
	require.False(t, c.GetPreferDispatchSubReactAgents())
	require.False(t, c.GetEnableGoalMode())
	require.Equal(t, int64(DefaultMaxSubAgentConcurrency), c.GetMaxSubAgents())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:  true,
				EnableGoalMode:    true,
				GoalMinIterations: 12,
				MaxSubAgents:      7,
			},
		},
	}, 6)

	require.True(t, c.GetPreferDispatchSubReactAgents(), "EnableMultiAgent should be applied on top-level config")
	require.True(t, c.EnableDispatchSubReactAgents, "EnableDispatchSubReactAgents should be applied on top-level config")
	require.True(t, c.GetEnableGoalMode(), "EnableGoalMode should be applied on top-level config")
	require.Equal(t, int64(12), c.GetGoalMinIterations())
	require.Equal(t, int64(7), c.GetMaxSubAgents())
}

func TestHotPatch_ExecutionStrategy_AppliesAllFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	// Initially strategy fields are at defaults
	require.False(t, c.GetPreferDispatchSubReactAgents())
	require.False(t, c.GetEnableGoalMode())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:  true,
				EnableGoalMode:    true,
				GoalMinIterations: 12,
				MaxSubAgents:      7,
			},
		},
	}, 6)

	// All four strategy fields should be applied via hotpatch.
	require.True(t, c.GetPreferDispatchSubReactAgents())
	require.True(t, c.EnableDispatchSubReactAgents)
	require.True(t, c.GetEnableGoalMode())
	require.Equal(t, int64(12), c.GetGoalMinIterations())
	require.Equal(t, int64(7), c.GetMaxSubAgents())
}

func TestHotPatch_ExecutionStrategy_PersistSessionStartParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "session-strategy-hotpatch-persist"
	c, feed := newHotPatchTestConfig(t, ctx, WithPersistentSessionId(sessionID))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, &ypb.AIStartParams{
		TimelineSessionID: sessionID,
	})
	require.NoError(t, err)
	c.StartEventLoop(ctx)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:  true,
				EnableGoalMode:    true,
				GoalMinIterations: 15,
				MaxSubAgents:      8,
			},
		},
	}, 6)

	got, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.NotNil(t, got.GetStrategy())
	require.True(t, got.GetStrategy().GetEnableMultiAgent())
	require.True(t, got.GetStrategy().GetEnableGoalMode())
	require.Equal(t, int64(15), got.GetStrategy().GetGoalMinIterations())
	require.Equal(t, int64(8), got.GetStrategy().GetMaxSubAgents())
}

func TestHotPatch_ExecutionStrategy_NilStrategyNoOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := NewTestConfig(ctx)
	opts := c.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params:           &ypb.AIStartParams{}, // Strategy is nil
	})
	require.Empty(t, opts, "nil Strategy should produce no options")
}

func TestHotPatch_ExecutionStrategy_WithDurationAndCriteria(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:       true,
				EnableGoalMode:         true,
				GoalMinIterations:      3,
				MaxSubAgents:           5,
				GoalDurationSeconds:    3600,
				GoalAcceptanceCriteria: "must produce at least 3 vulnerability findings with evidence",
			},
		},
	}, 6)

	require.True(t, c.GetEnableGoalMode())
	require.Equal(t, int64(3600), c.GetGoalDurationSeconds())
	require.Equal(t, "must produce at least 3 vulnerability findings with evidence", c.GetGoalAcceptanceCriteria())

	// Deadline should not be started yet (lazy start)
	require.True(t, c.GetGoalDeadline().IsZero(), "deadline should be lazily started")

	// Start deadline and check it's in the future
	c.StartGoalDeadline()
	deadline := c.GetGoalDeadline()
	require.False(t, deadline.IsZero())
	require.True(t, deadline.After(time.Now()))
	require.False(t, c.IsGoalDeadlinePassed())
}

func TestHotPatch_ExecutionStrategy_NeverEndingDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   3,
				GoalDurationSeconds: -1, // never-ending
			},
		},
	}, 6)

	require.Equal(t, int64(-1), c.GetGoalDurationSeconds())

	c.StartGoalDeadline()
	deadline := c.GetGoalDeadline()
	require.False(t, deadline.IsZero())
	// Far-future sentinel
	require.True(t, deadline.Year() >= 9999)
	require.False(t, c.IsGoalDeadlinePassed())
}

func TestHotPatch_ExecutionStrategy_PersistWithDurationAndCriteria(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "session-strategy-duration-persist"
	c, feed := newHotPatchTestConfig(t, ctx, WithPersistentSessionId(sessionID))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, &ypb.AIStartParams{
		TimelineSessionID: sessionID,
	})
	require.NoError(t, err)
	c.StartEventLoop(ctx)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:         true,
				GoalMinIterations:      3,
				GoalDurationSeconds:    1800,
				GoalAcceptanceCriteria: "must complete code audit with risk ratings",
			},
		},
	}, 6)

	got, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.NotNil(t, got.GetStrategy())
	require.Equal(t, int64(1800), got.GetStrategy().GetGoalDurationSeconds())
	require.Equal(t, "must complete code audit with risk ratings", got.GetStrategy().GetGoalAcceptanceCriteria())
}
