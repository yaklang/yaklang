package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge"
)

// NativeOptions installs coordinator helpers without overriding the main-loop action
// protocol. Helper calls never construct a legacy Coordinator.
func NativeOptions() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{aicommon.WithDisableDynamicPlanning(true), aicommon.WithAiAgreeRiskControl(NativeRiskReview), aicommon.WithLiteForgeExecutor(executeNativeHelper)}
}

func WithNativeHelpers() aicommon.ConfigOption {
	return aicommon.WithLiteForgeExecutor(executeNativeHelper)
}

func executeNativeHelper(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
	return liteforge.ExecuteTyped(prompt, opts...)
}
