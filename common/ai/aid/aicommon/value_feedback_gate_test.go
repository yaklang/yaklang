package aicommon

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// registerCountingValueFeedbackSubmitter 注册一个计数 submitter 并返回读取计数
// 的函数；测试结束后恢复原 submitter。
func registerCountingValueFeedbackSubmitter(t *testing.T) func() int {
	t.Helper()
	valueFeedbackSubmitterMu.Lock()
	previousSubmitter := valueFeedbackSubmitter
	valueFeedbackSubmitterMu.Unlock()

	var mu sync.Mutex
	count := 0
	RegisterValueFeedbackSubmitter(func(_ *Config, _ *ValueFeedbackRecord) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	t.Cleanup(func() { RegisterValueFeedbackSubmitter(previousSubmitter) })

	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

// TestSubmitValueFeedback_DisableValueFeedbackGatesConvergePoint 验证汇聚点门控：
// DisableValueFeedback=true 时 SubmitValueFeedback 直接短路，submitter 不被触达。
func TestSubmitValueFeedback_DisableValueFeedbackGatesConvergePoint(t *testing.T) {
	getCount := registerCountingValueFeedbackSubmitter(t)

	disabled := NewConfig(context.Background(), WithDisableValueFeedbackSubmission(true))
	enabled := NewConfig(context.Background(), WithDisableValueFeedbackSubmission(false))

	SubmitValueFeedback(disabled, &ValueFeedbackRecord{FocusMode: ReviewFocusModeGeneric})
	require.Equal(t, 0, getCount(), "disabled config must not reach the submitter")

	SubmitValueFeedback(enabled, &ValueFeedbackRecord{FocusMode: ReviewFocusModeGeneric})
	require.Equal(t, 1, getCount(), "enabled config must reach the submitter")
}

// TestSubmitToolReviewValueFeedback_DisabledConfigNoSubmission 验证 tool_review
// 通路（原本完全无门控、审计日志中泄漏最多的通路）在 config 禁用后不再提交。
func TestSubmitToolReviewValueFeedback_DisabledConfigNoSubmission(t *testing.T) {
	getCount := registerCountingValueFeedbackSubmitter(t)

	cfg := NewConfig(context.Background(), WithDisableValueFeedbackSubmission(true))
	ep := cfg.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	cfg.SubmitToolReviewValueFeedback(ep, "review required: run rm -rf?", nil, nil)
	require.Equal(t, 0, getCount(), "tool_review submission must be gated by DisableValueFeedback")
}

// TestConvertConfigToOptions_PropagatesValueFeedbackDisable 验证 config 级禁用
// 经 ConvertConfigToOptions 传播到任意深度的子 invoker。
func TestConvertConfigToOptions_PropagatesValueFeedbackDisable(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableValueFeedbackSubmission(true))
	childOpts := ConvertConfigToOptions(parent)
	child := NewConfig(context.Background(), childOpts...)
	require.True(t, child.DisableValueFeedback, "child config must inherit DisableValueFeedback")

	grandchildOpts := ConvertConfigToOptions(child)
	grandchild := NewConfig(context.Background(), grandchildOpts...)
	require.True(t, grandchild.DisableValueFeedback, "grandchild config must inherit DisableValueFeedback at any depth")
}

// TestConvertConfigToOptions_PropagatesPeriodicVerificationDisable 验证周期验证
// 的 config 级禁用同样传播，且 config entry 被写入（reactloop 侧按 entry 读取）。
func TestConvertConfigToOptions_PropagatesPeriodicVerificationDisable(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisablePeriodicVerification(true))
	require.True(t, parent.DisablePeriodicVerification)
	require.Equal(t, true, parent.GetConfigBool("DisablePeriodicVerification"))

	childOpts := ConvertConfigToOptions(parent)
	child := NewConfig(context.Background(), childOpts...)
	require.True(t, child.DisablePeriodicVerification, "child config must inherit DisablePeriodicVerification")
	require.Equal(t, true, child.GetConfigBool("DisablePeriodicVerification"))
}

// TestNewConfigPeriodicVerificationEntryDefaultOff 验证默认不开（不改变全局默认）。
func TestNewConfigPeriodicVerificationEntryDefaultOff(t *testing.T) {
	cfg := NewConfig(context.Background())
	require.False(t, cfg.DisableValueFeedback)
	require.False(t, cfg.DisablePeriodicVerification)
}