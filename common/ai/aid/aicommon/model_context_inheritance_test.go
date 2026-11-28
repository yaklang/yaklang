package aicommon

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Subagents may wait for an execution slot after their options are captured.
// Their callback and explicit model binding must describe the same submission.
func TestInheritTieredAICallbackSnapshotsExplicitModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := newConfig(ctx)
	var calls int
	require.NoError(t, WithQualityPriorityAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return nil, nil
	})(parent))
	model := &ypb.AIModelConfig{ContextOwnerId: uuid.NewString(), ModelName: "model-a", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}}
	require.NoError(t, WithPrimaryModelContext(model)(parent))
	key := aispec.ModelContextIdentity(model)
	inherit := WithInheritTieredAICallback(parent, true)

	// Neither replacing the parent's binding/callback nor mutating the original
	// protobuf may retarget an already queued child.
	require.NoError(t, WithPrimaryModelContext(&ypb.AIModelConfig{ModelName: "model-b"})(parent))
	require.NoError(t, WithQualityPriorityAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		t.Error("queued child used the replacement callback")
		return nil, nil
	})(parent))
	model.ModelName = "mutated-after-submission"
	child := newConfig(ctx)
	require.NoError(t, inherit(child))
	_, name, childKey := child.PrimaryModelContext()
	require.Equal(t, "model-a", name)
	require.Equal(t, key, childKey)
	factory := child.GetRawAICallbacks().QualityPriorityRaw
	require.NotNil(t, factory)
	cb, err := factory(nil)
	require.NoError(t, err)
	require.NotNil(t, cb)
	_, err = cb(child, NewAIRequest("probe"))
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// Reusing the option creates independent explicit bindings for siblings.
	child.primaryModelContextConfig.ModelName = "child-local-change"
	sibling := newConfig(ctx)
	require.NoError(t, inherit(sibling))
	_, name, siblingKey := sibling.PrimaryModelContext()
	require.Equal(t, "model-a", name)
	require.Equal(t, key, siblingKey)
}

func TestInheritTieredAICallbackPreservesContextOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "custom-callback"
		if enabled {
			name = "global-quality"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			parent := newConfig(ctx)
			require.NoError(t, WithQualityPriorityAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) { return nil, nil })(parent))
			parent.modelContextSettingsEnabled = enabled
			inherit := WithInheritTieredAICallback(parent, true)
			parent.modelContextSettingsEnabled = !enabled
			child := newConfig(ctx)
			require.NoError(t, inherit(child))
			require.Equal(t, enabled, child.modelContextSettingsEnabled)
			require.Nil(t, child.primaryModelContextConfig, "global routing must remain dynamic, not become an explicit model binding")
			require.NotNil(t, child.GetRawAICallbacks().QualityPriorityRaw)
		})
	}
}

func TestSubForgeExecutionPolicyInheritsModelContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := newConfig(ctx)
	require.NoError(t, WithQualityPriorityAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) { return nil, nil })(parent))
	model := &ypb.AIModelConfig{ContextOwnerId: uuid.NewString(), ModelName: "subforge-model", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}}
	require.NoError(t, WithPrimaryModelContext(model)(parent))
	child := newConfig(ctx)
	for _, option := range SubForgeExecutionPolicy(parent) {
		require.NoError(t, option(child))
	}
	_, _, key := child.PrimaryModelContext()
	require.Equal(t, aispec.ModelContextIdentity(model), key)
}
