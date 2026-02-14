package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockSpeedCallback is a sentinel callback for testing speed priority
func mockSpeedCallback(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
	return &AIResponse{}, nil
}

// mockQualityCallback is a sentinel callback for testing quality priority
func mockQualityCallback(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
	return &AIResponse{}, nil
}

func TestWithLiteForgeSpeedFirst_PromotesExistingSpeedCallback(t *testing.T) {
	// When both callbacks are set independently, WithLiteForgeSpeedFirst should
	// promote SpeedPriorityAICallback to QualityPriorityAICallback
	config := NewConfig(context.Background(),
		WithQualityPriorityAICallback(mockQualityCallback),
		WithSpeedPriorityAICallback(mockSpeedCallback),
	)

	// Apply speed-first option
	err := WithLiteForgeSpeedFirst()(config)
	require.NoError(t, err)

	// After applying, QualityPriority should be overridden to the speed callback
	require.NotNil(t, config.QualityPriorityAICallback, "QualityPriorityAICallback should not be nil")
	require.NotNil(t, config.SpeedPriorityAICallback, "SpeedPriorityAICallback should not be nil")
}

func TestWithLiteForgeQualityFirst_PromotesExistingQualityCallback(t *testing.T) {
	// When both callbacks are set independently, WithLiteForgeQualityFirst should
	// promote QualityPriorityAICallback to SpeedPriorityAICallback
	config := NewConfig(context.Background(),
		WithQualityPriorityAICallback(mockQualityCallback),
		WithSpeedPriorityAICallback(mockSpeedCallback),
	)

	// Apply quality-first option
	err := WithLiteForgeQualityFirst()(config)
	require.NoError(t, err)

	// After applying, SpeedPriority should be overridden to the quality callback
	require.NotNil(t, config.QualityPriorityAICallback, "QualityPriorityAICallback should not be nil")
	require.NotNil(t, config.SpeedPriorityAICallback, "SpeedPriorityAICallback should not be nil")
}

func TestWithLiteForgeSpeedFirst_WithAICallback(t *testing.T) {
	// When WithAICallback sets both to the same callback,
	// WithLiteForgeSpeedFirst should still work (promote speed, which is same as quality)
	config := NewConfig(context.Background(),
		WithAICallback(mockSpeedCallback),
	)

	// Both should be set to the same callback
	require.NotNil(t, config.QualityPriorityAICallback)
	require.NotNil(t, config.SpeedPriorityAICallback)

	// Apply speed-first - should be a no-op effectively since both are the same
	err := WithLiteForgeSpeedFirst()(config)
	require.NoError(t, err)

	require.NotNil(t, config.QualityPriorityAICallback)
	require.NotNil(t, config.SpeedPriorityAICallback)
}

func TestWithLiteForgeQualityFirst_WithAICallback(t *testing.T) {
	// When WithAICallback sets both to the same callback,
	// WithLiteForgeQualityFirst should still work (promote quality, which is same as speed)
	config := NewConfig(context.Background(),
		WithAICallback(mockQualityCallback),
	)

	// Both should be set to the same callback
	require.NotNil(t, config.QualityPriorityAICallback)
	require.NotNil(t, config.SpeedPriorityAICallback)

	// Apply quality-first - should be a no-op effectively since both are the same
	err := WithLiteForgeQualityFirst()(config)
	require.NoError(t, err)

	require.NotNil(t, config.QualityPriorityAICallback)
	require.NotNil(t, config.SpeedPriorityAICallback)
}

func TestWithLiteForgeSpeedFirst_NoCallbacksSet(t *testing.T) {
	// When no callbacks are set and no tiered config,
	// WithLiteForgeSpeedFirst should be a no-op (no panic)
	config := newConfig(context.Background())
	// Explicitly nil out callbacks
	config.QualityPriorityAICallback = nil
	config.SpeedPriorityAICallback = nil

	err := WithLiteForgeSpeedFirst()(config)
	require.NoError(t, err)
}

func TestWithLiteForgeQualityFirst_NoCallbacksSet(t *testing.T) {
	// When no callbacks are set and no tiered config,
	// WithLiteForgeQualityFirst should be a no-op (no panic)
	config := newConfig(context.Background())
	// Explicitly nil out callbacks
	config.QualityPriorityAICallback = nil
	config.SpeedPriorityAICallback = nil

	err := WithLiteForgeQualityFirst()(config)
	require.NoError(t, err)
}

func TestInvokeLiteForgeSpeedPriority_RegisteredCallback(t *testing.T) {
	// Test that InvokeLiteForgeSpeedPriority correctly delegates to InvokeLiteForge
	// by checking it includes WithLiteForgeSpeedFirst in opts
	called := false
	RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*ForgeResult, error) {
		called = true
		// Verify that opts contains at least one ConfigOption (WithLiteForgeSpeedFirst)
		hasConfigOption := false
		for _, opt := range opts {
			if _, ok := opt.(ConfigOption); ok {
				hasConfigOption = true
				break
			}
		}
		require.True(t, hasConfigOption, "InvokeLiteForgeSpeedPriority should include WithLiteForgeSpeedFirst as ConfigOption")
		return &ForgeResult{Action: &Action{}}, nil
	})
	defer RegisterLiteForgeExecuteCallback(nil)

	result, err := InvokeLiteForgeSpeedPriority("test prompt")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, called)
}

func TestInvokeLiteForgeQualityPriority_RegisteredCallback(t *testing.T) {
	// Test that InvokeLiteForgeQualityPriority correctly delegates to InvokeLiteForge
	// by checking it includes WithLiteForgeQualityFirst in opts
	called := false
	RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*ForgeResult, error) {
		called = true
		// Verify that opts contains at least one ConfigOption (WithLiteForgeQualityFirst)
		hasConfigOption := false
		for _, opt := range opts {
			if _, ok := opt.(ConfigOption); ok {
				hasConfigOption = true
				break
			}
		}
		require.True(t, hasConfigOption, "InvokeLiteForgeQualityPriority should include WithLiteForgeQualityFirst as ConfigOption")
		return &ForgeResult{Action: &Action{}}, nil
	})
	defer RegisterLiteForgeExecuteCallback(nil)

	result, err := InvokeLiteForgeQualityPriority("test prompt")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, called)
}

func TestInvokeLiteForgeSpeedPriority_PreservesUserOpts(t *testing.T) {
	// Verify that user-provided opts are preserved alongside the speed priority option
	optCount := 0
	RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*ForgeResult, error) {
		optCount = len(opts)
		return &ForgeResult{Action: &Action{}}, nil
	})
	defer RegisterLiteForgeExecuteCallback(nil)

	// Pass one additional ConfigOption
	_, err := InvokeLiteForgeSpeedPriority("test", WithAICallback(mockSpeedCallback))
	require.NoError(t, err)
	// Should have 2 opts: the user-provided WithAICallback + WithLiteForgeSpeedFirst
	require.Equal(t, 2, optCount, "Should have user opt + speed priority opt")
}
