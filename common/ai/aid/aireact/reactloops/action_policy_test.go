package reactloops

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/omap"
	"testing"
)

func TestRuntimeActionPolicyChecksInventoryAndExecution(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithReActActionPolicy(func(loop, action string) bool { return loop == "plan" && action == "allowed" }))
	loop := &ReActLoop{config: cfg, loopName: "plan", actions: omap.NewEmptyOrderedMap[string, *LoopAction](), loopActions: omap.NewEmptyOrderedMap[string, LoopActionFactory]()}
	loop.actions.Set("allowed", &LoopAction{ActionType: "allowed"})
	loop.actions.Set("denied", &LoopAction{ActionType: "denied"})
	loop.loopActions.Set("factory_denied", func(aicommon.AIInvokeRuntime) (*LoopAction, error) { t.Fatal("denied factory ran"); return nil, nil })
	require.Equal(t, []string{"allowed"}, loop.GetAllActionNames())
	require.Len(t, loop.GetAllActions(), 1)
	require.False(t, loop.NoActions())
	_, err := loop.GetActionHandler("allowed")
	require.NoError(t, err)
	for _, name := range []string{"denied", "factory_denied"} {
		_, err := loop.GetActionHandler(name)
		require.ErrorContains(t, err, "denied by runtime policy")
	}
	// A dynamically registered action must not bypass the execution gate.
	loop.actions.Set("late", &LoopAction{ActionType: "late"})
	_, err = loop.GetActionHandler("late")
	require.Error(t, err)
	loop.loopName = "other"
	require.True(t, loop.NoActions())
}

func TestRuntimeActionPolicyAbsentPreservesActions(t *testing.T) {
	loop := &ReActLoop{actions: omap.NewEmptyOrderedMap[string, *LoopAction](), loopActions: omap.NewEmptyOrderedMap[string, LoopActionFactory]()}
	loop.actions.Set("custom", &LoopAction{ActionType: "custom"})
	require.Equal(t, []string{"custom"}, loop.GetAllActionNames())
	_, err := loop.GetActionHandler("custom")
	require.NoError(t, err)
}
