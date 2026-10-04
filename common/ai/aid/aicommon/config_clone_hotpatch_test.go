package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigCloneWithOwnedChannelsDoesNotAcquireHotPatchSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := newConfig(ctx)
	for _, option := range []ConfigOption{WithAppendPersistentContext("caller context"), WithPlanExecTaskConcurrency(3), WithEnableFunctionCallMode(false), WithAICallback(func(c AICallerConfigIf, _ *AIRequest) (*AIResponse, error) { return c.NewAIResponse(), nil })} {
		require.NoError(t, option(parent))
	}
	child := newConfig(ctx)
	owned := child.HotPatchOptionChan
	for _, option := range ConvertConfigToOptionsWithoutHotPatch(parent) {
		require.NoError(t, option(child))
	}
	require.Same(t, owned, child.HotPatchOptionChan)
	require.Equal(t, parent.GetPlanExecTaskConcurrency(), child.GetPlanExecTaskConcurrency())
	require.False(t, child.EnableFunctionCallMode)
	require.Equal(t, parent.PersistentMemory, child.PersistentMemory)
	legacyClone := newConfig(ctx)
	for _, option := range ConvertConfigToOptions(parent) {
		require.NoError(t, option(legacyClone))
	}
	require.NotSame(t, owned, legacyClone.HotPatchOptionChan, "the existing clone API retains its subscription behavior")
	parent.HotPatchBroadcaster.Unsubscribe(legacyClone.HotPatchOptionChan)
}
