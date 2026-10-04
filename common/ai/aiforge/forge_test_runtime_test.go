package aiforge

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// Rendering/formatting unit tests need Config only. End-to-end tests live in
// aiforge_test and import aireact to exercise the registered production runtime.
type formattingRuntime struct {
	aicommon.AITaskInvokeRuntime
	config *aicommon.Config
}

func (r *formattingRuntime) GetConfig() aicommon.AICallerConfigIf { return r.config }
func registerFormattingRuntime(t *testing.T) {
	t.Helper()
	previous := aicommon.AIRuntimeInvokerGetter
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		return &formattingRuntime{config: aicommon.NewConfig(ctx, opts...)}, nil
	}
	t.Cleanup(func() { aicommon.AIRuntimeInvokerGetter = previous })
}
