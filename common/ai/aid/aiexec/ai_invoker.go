package aiexec

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

var AIRuntimeInvokerGetter func(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error)

func RegisterDefaultAIRuntimeInvoker(getter func(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error)) {
	AIRuntimeInvokerGetter = getter
}
