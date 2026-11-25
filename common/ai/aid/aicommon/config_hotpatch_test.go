package aicommon

import (
	"context"
	"encoding/json"
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
		expectedEvents := optionCount
		if hotpatchAffectsAgentPolicy(event.HotpatchType) {
			expectedEvents = 1 // local policy emits only the final configuration
		}
		for i := 0; i < expectedEvents; i++ {
			select {
			case <-applied:
			case <-ctx.Done():
				t.Fatalf("hot-patch %s did not finish applying %d options: %v", event.HotpatchType, expectedEvents, ctx.Err())
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

	// A duration-only strategy starts counting as soon as it is applied.
	deadline := c.GetGoalDeadline()
	require.False(t, deadline.IsZero())
	require.WithinDuration(t, time.Now().Add(time.Hour), deadline, 2*time.Second)
	require.True(t, deadline.After(time.Now()))
	require.False(t, c.IsGoalDeadlinePassed())

	reloadStartedAt := time.Now()
	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:       true,
				EnableGoalMode:         true,
				GoalMinIterations:      3,
				MaxSubAgents:           5,
				GoalDurationSeconds:    60,
				GoalAcceptanceCriteria: "must produce at least 3 vulnerability findings with evidence",
			},
		},
	}, 6)

	require.Equal(t, int64(60), c.GetGoalDurationSeconds())
	reloadedDeadline := c.GetGoalDeadline()
	require.True(t, reloadedDeadline.Before(deadline), "hot reload must replace the active deadline")
	require.WithinDuration(t, reloadStartedAt.Add(time.Minute), reloadedDeadline, 2*time.Second)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:       true,
				EnableGoalMode:         true,
				GoalMinIterations:      3,
				MaxSubAgents:           5,
				GoalDurationSeconds:    60,
				GoalAcceptanceCriteria: "must produce at least 3 vulnerability findings with evidence",
			},
		},
	}, 6)
	require.Equal(t, reloadedDeadline, c.GetGoalDeadline(),
		"reapplying the same strategy must not extend the active deadline")

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   3,
				GoalDurationSeconds: 0,
			},
		},
	}, 6)
	require.True(t, c.GetGoalDeadline().IsZero(), "disabling the time window must clear its deadline")
}

func TestGoalDurationHotReload_StartsImmediatelyAndPreservesRepeatedValue(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))
	require.WithinDuration(t, time.Now().Add(time.Hour), c.GetGoalDeadline(), time.Second)
	require.NoError(t, WithGoalDurationSeconds(60)(c))
	deadline := c.GetGoalDeadline()
	require.WithinDuration(t, time.Now().Add(time.Minute), deadline, time.Second)
	require.NoError(t, WithGoalDurationSeconds(60)(c))
	c.StartGoalDeadline()
	require.Equal(t, deadline, c.GetGoalDeadline(), "repeated settings and legacy start must not extend the window")
	require.EqualValues(t, 60, c.GetGoalDurationSeconds())
	c.GoalDeadline = time.Now().Add(-time.Minute)
	expired := c.GetGoalDeadline()
	require.NoError(t, WithGoalDurationSeconds(60)(c))
	require.Equal(t, expired, c.GetGoalDeadline(), "an expired window must not be restarted by resending the same strategy")
}

func TestHotPatch_GoalTimeAppliesEveryReceivedCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)
	require.NoError(t, WithGoalDurationSeconds(3600)(c))
	c.StartGoalDeadline()
	originalDeadline := c.GetGoalDeadline()
	originalRevision := c.GetGoalTimeState().Revision

	makeEvent := func() *ypb.AIInputEvent {
		return &ypb.AIInputEvent{
			IsConfigHotpatch: true,
			HotpatchType:     HotPatchType_GoalTime,
			GoalTimePatch: &ypb.AIGoalTimePatch{
				Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
				ExtendSeconds: 60,
			},
		}
	}

	feed(makeEvent(), 1)
	extended := c.GetGoalDeadline()
	require.WithinDuration(t, originalDeadline.Add(time.Minute), extended, 2*time.Second)
	require.Equal(t, originalRevision+1, c.GetGoalTimeState().Revision)

	feed(makeEvent(), 1)
	require.WithinDuration(t, originalDeadline.Add(2*time.Minute), c.GetGoalDeadline(), 2*time.Second)
	require.Equal(t, originalRevision+2, c.GetGoalTimeState().Revision)

	info := c.SimpleInfoMap()
	require.Equal(t, c.GetGoalTimeState().Revision, info["GoalTimeRevision"])
	require.Equal(t, true, info["GoalTimeStarted"])
	require.Greater(t, info["GoalDeadlineUnix"].(int64), time.Now().Unix())
}

func TestHotPatch_ExecutionStrategyEmitsOneCoherentPolicySnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	policyEvents := make(chan *schema.AiOutputEvent, 8)
	c := NewTestConfig(ctx, WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.Type == schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT && event.NodeId == AgentPolicySnapshotNodeID {
			policyEvents <- event
		}
	}))
	t.Cleanup(func() {
		cancel()
		c.WaitHotPatchLoopStopped()
	})
	c.StartEventLoop(ctx)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableMultiAgent:       true,
				EnableGoalMode:         true,
				GoalMinIterations:      11,
				MaxSubAgents:           6,
				GoalDurationSeconds:    120,
				GoalAcceptanceCriteria: "evidence required",
			},
		},
	})

	select {
	case event := <-policyEvents:
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.True(t, snapshot.Goal.Enabled)
		require.EqualValues(t, 11, snapshot.Goal.MinIterations)
		require.Equal(t, "evidence required", snapshot.Goal.AcceptanceCriteria)
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.InDelta(t, 120, snapshot.Goal.TimeLimit.DeadlineAt-snapshot.ServerTime, 1)
		require.True(t, snapshot.MultiAgent.Enabled)
		require.True(t, snapshot.MultiAgent.PreferDispatch)
		require.EqualValues(t, 6, snapshot.MultiAgent.MaxConcurrency)
	case <-ctx.Done():
		t.Fatal("policy snapshot was not emitted")
	}
	select {
	case <-policyEvents:
		t.Fatal("one hotpatch transaction must emit exactly one policy snapshot")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHotPatch_ExecutionStrategyWithoutGoalTimePatchStartsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	policyEvents := make(chan *schema.AiOutputEvent, 4)
	c := NewTestConfig(ctx, WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.Type == schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT && event.NodeId == AgentPolicySnapshotNodeID {
			policyEvents <- event
		}
	}))
	t.Cleanup(func() {
		cancel()
		c.WaitHotPatchLoopStopped()
	})
	c.StartEventLoop(ctx)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 7200,
			},
		},
	})

	select {
	case event := <-policyEvents:
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.True(t, snapshot.Goal.Enabled)
		require.True(t, snapshot.Goal.Effective)
		require.EqualValues(t, 1, snapshot.Goal.MinIterations)
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.EqualValues(t, 7200, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
		require.InDelta(t, 7200, snapshot.Goal.TimeLimit.DeadlineAt-snapshot.ServerTime, 1)
	case <-ctx.Done():
		t.Fatal("agent policy snapshot was not emitted")
	}
	require.False(t, c.GetGoalDeadline().IsZero())
	select {
	case <-policyEvents:
		t.Fatal("one strategy hotpatch must emit exactly one policy snapshot")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHotPatch_ExecutionStrategyAppliesGoalTimePatchInOneDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	policyEvents := make(chan *schema.AiOutputEvent, 4)
	c := NewTestConfig(ctx, WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.Type == schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT && event.NodeId == AgentPolicySnapshotNodeID {
			policyEvents <- event
		}
	}))
	t.Cleanup(func() {
		cancel()
		c.WaitHotPatchLoopStopped()
	})
	c.StartEventLoop(ctx)
	propagated := c.HotPatchBroadcaster.Subscribe()
	t.Cleanup(func() {
		c.HotPatchBroadcaster.Unsubscribe(propagated)
	})

	deadline := time.Now().Add(30 * time.Minute).Unix()
	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 1800,
			},
		},
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
			DeadlineUnix: deadline,
		},
	})

	select {
	case event := <-policyEvents:
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.True(t, snapshot.Goal.Enabled)
		require.EqualValues(t, 1, snapshot.Goal.MinIterations)
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.Equal(t, deadline, snapshot.Goal.TimeLimit.DeadlineAt)
		require.EqualValues(t, 1800, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	case <-ctx.Done():
		t.Fatal("policy snapshot was not emitted")
	}

	require.True(t, c.GetEnableGoalMode())
	require.Equal(t, deadline, c.GetGoalDeadline().Unix())
	select {
	case <-policyEvents:
		t.Fatal("one combined policy dispatch must emit exactly one policy snapshot")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-propagated.OutputChannel():
		t.Fatal("agent policy hotpatch must not be propagated to nested runtimes")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHotPatch_ExecutionStrategySyncSeesCombinedPolicyState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const syncID = "strategy-goal-time-sync"
	syncEvents := make(chan *schema.AiOutputEvent, 4)
	c := NewTestConfig(ctx, WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.IsSync && event.SyncID == syncID &&
			event.Type == schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT &&
			event.NodeId == AgentPolicySnapshotNodeID {
			syncEvents <- event
		}
	}))
	t.Cleanup(func() {
		cancel()
		c.WaitHotPatchLoopStopped()
	})
	c.StartEventLoop(ctx)

	deadline := time.Now().Add(2 * time.Hour).Unix()
	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 7200,
			},
		},
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
			DeadlineUnix: deadline,
		},
	})
	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AGENT_POLICY_SNAPSHOT,
		SyncID:        syncID,
	})

	select {
	case event := <-syncEvents:
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.True(t, snapshot.Goal.Enabled)
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.Equal(t, deadline, snapshot.Goal.TimeLimit.DeadlineAt)
	case <-ctx.Done():
		t.Fatal("agent policy sync response was not emitted")
	}
}

func TestGoalTimePatch_SupportsExplicitActionsWithoutOneof(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(120)(c))
	originalDeadline := c.GetGoalDeadline()

	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
		ExtendSeconds: 60,
	})(c))
	state := c.GetGoalTimeState()
	require.EqualValues(t, 120, state.DurationSeconds)
	require.True(t, state.Started)
	require.Equal(t, originalDeadline.Add(time.Minute).Unix(), state.DeadlineUnix)

	target := time.Now().Add(5 * time.Minute).Unix()
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
		DeadlineUnix: target,
	})(c))
	state = c.GetGoalTimeState()
	require.True(t, state.Started)
	require.Equal(t, target, state.DeadlineUnix)

	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_DISABLE,
	})(c))
	state = c.GetGoalTimeState()
	require.Zero(t, state.DurationSeconds)
	require.False(t, state.Started)

	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_NEVER_ENDING,
	})(c))
	state = c.GetGoalTimeState()
	require.EqualValues(t, -1, state.DurationSeconds)
	require.True(t, state.NeverEnding)
	require.True(t, state.Started)
}

func TestGoalTimePatch_ExtendingExpiredDeadlineReopensFromNow(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(60)(c))
	c.m.Lock()
	c.GoalDeadline = time.Now().Add(-time.Minute)
	c.m.Unlock()

	startedAt := time.Now()
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
		ExtendSeconds: 60,
	})(c))
	require.WithinDuration(t, startedAt.Add(time.Minute), c.GetGoalDeadline(), 2*time.Second)
}

func TestGoalTimePatch_SetDeadlineEnablesDisabledWindow(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.Zero(t, c.GetGoalDurationSeconds())

	target := time.Now().Add(2 * time.Minute)
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
		DeadlineUnix: target.Unix(),
	})(c))
	state := c.GetGoalTimeState()
	require.True(t, state.Started)
	require.Greater(t, state.DurationSeconds, int64(0))
	require.Equal(t, target.Unix(), state.DeadlineUnix)
}

func TestGoalTimePatch_RejectsInvalidMutations(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.Error(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
	})(c), "zero extension is invalid")
	require.Error(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
		ExtendSeconds: 60,
	})(c), "a disabled time window cannot be extended")
	require.Error(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
	})(c))
	require.Error(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{})(c))
}

func TestGoalTimePatch_MergeDoesNotPersistRuntimeCommand(t *testing.T) {
	base := &ypb.AIStartParams{
		Strategy: &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalDurationSeconds: 1800},
	}
	next, changed := mergeHotpatchSessionStartParams(base, &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_GoalTime,
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
			ExtendSeconds: 60,
		},
	})
	require.False(t, changed)
	require.Same(t, base, next)
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

func requireRuntimeContextLimit(t *testing.T, c *Config, limit int) {
	t.Helper()
	require.Equal(t, limit, c.TimelineContentSizeLimit)
	require.EqualValues(t, limit, c.GetTimelineContentSizeLimit())
	timeline := c.GetTimeline()
	require.NotNil(t, timeline)
	require.EqualValues(t, limit, timeline.GetTimelineContentLimit(), "config and timeline thresholds diverged")
	require.EqualValues(t, limit, c.SimpleInfoMap()["TimelineContentSizeLimit"])
}

func TestWithTimelineContentLimitUpdatesBoundTimeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewTestConfig(ctx)
	timeline := c.GetTimeline()
	timeline.SetTimelineContentLimit(111)

	require.NoError(t, WithTimelineContentLimit(64*1024)(c))

	require.Same(t, timeline, c.GetTimeline())
	requireRuntimeContextLimit(t, c, 64*1024)
}

func TestHotPatch_RuntimeContextSizeLimit_UpdatesTimeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, feed := newHotPatchTestConfig(t, ctx)
	c.StartEventLoop(ctx)

	timeline := c.GetTimeline()
	require.Same(t, timeline, c.GetTimeline())
	timeline.SetTimelineContentLimit(111)
	require.Equal(t, DefaultTimelineContentSizeLimit, c.TimelineContentSizeLimit,
		"precondition: only the timeline object is stale")

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{
			TimelineContentSizeLimit: 20 * 1024, // deprecated field must not win
			RuntimeContextSizeLimit:  128 * 1024,
		},
	}, 1)

	require.Same(t, timeline, c.GetTimeline())
	requireRuntimeContextLimit(t, c, 128*1024)

	timeline.SetTimelineContentLimit(222)
	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{
			TimelineContentSizeLimit: 20 * 1024,
		},
	}, 1)

	require.Same(t, timeline, c.GetTimeline())
	requireRuntimeContextLimit(t, c, DefaultTimelineContentSizeLimit)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{
			RuntimeContextSizeLimit: -1,
		},
	}, 1)
	requireRuntimeContextLimit(t, c, DefaultTimelineContentSizeLimit)
}

func TestRuntimeContextLimitSurvivesSharedTimelineRebind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := NewTestConfig(ctx)
	require.NoError(t, WithTimelineContentLimit(96*1024)(parent))

	child := NewConfig(ctx, ConvertConfigToOptions(parent)...)
	require.Same(t, parent.GetTimeline(), child.GetTimeline())
	requireRuntimeContextLimit(t, child, 96*1024)
}

func TestHotPatch_RuntimeContextSizeLimit_PersistSessionStartParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "session-runtime-context-size-hotpatch"
	c, feed := newHotPatchTestConfig(t, ctx, WithPersistentSessionId(sessionID))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, &ypb.AIStartParams{
		EnablePlan:               true,
		ReviewPolicy:             "manual",
		TimelineContentSizeLimit: 20 * 1024,
		RuntimeContextSizeLimit:  64 * 1024,
		TimelineSessionID:        sessionID,
	})
	require.NoError(t, err)
	c.StartEventLoop(ctx)

	timeline := c.GetTimeline()
	timeline.SetTimelineContentLimit(111)

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{
			TimelineContentSizeLimit: 8 * 1024,
			RuntimeContextSizeLimit:  96 * 1024,
		},
	}, 1)

	requireRuntimeContextLimit(t, c, 96*1024)
	got, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.EqualValues(t, 96*1024, got.GetRuntimeContextSizeLimit())
	require.EqualValues(t, 20*1024, got.GetTimelineContentSizeLimit(), "deprecated field is not a hotpatch target")
	require.True(t, got.GetEnablePlan())
	require.Equal(t, "manual", got.GetReviewPolicy())
	require.Equal(t, sessionID, got.GetTimelineSessionID())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params:           &ypb.AIStartParams{RuntimeContextSizeLimit: 0},
	}, 1)

	requireRuntimeContextLimit(t, c, DefaultTimelineContentSizeLimit)
	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.Zero(t, got.GetRuntimeContextSizeLimit(), "zero must stick so the next start uses the server default")
	require.True(t, got.GetEnablePlan())
	require.Equal(t, "manual", got.GetReviewPolicy())

	feed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RuntimeContextSizeLimit,
		Params:           &ypb.AIStartParams{RuntimeContextSizeLimit: -5},
	}, 1)
	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.Zero(t, got.GetRuntimeContextSizeLimit())
	requireRuntimeContextLimit(t, c, DefaultTimelineContentSizeLimit)
}
