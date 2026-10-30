package yak

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func init() {
	yakscript.RegisterExecutor(func(ctx context.Context, source string, req *yakscript.Request, emit func(string) error) (any, error) {
		// Every attempt gets its own VM. Public APIs come from ai.Exports,
		// shared with documentation and language tooling, not injected maps.
		engine := yaklang.New()
		if err := engine.SafeEval(ctx, source); err != nil {
			return nil, &aispec.ModelCallFailure{Kind: "script", Cause: err}
		}
		value, err := engine.SafeCallYakFunction(ctx, "chat", []any{req, emit})
		if returned, ok := value.(error); ok && err == nil {
			err, value = returned, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		failure := req.Failure()
		if failure == nil && err != nil {
			failure = &aispec.ModelCallFailure{Kind: "script", Cause: err}
		}
		if failure == nil {
			return value, nil
		}
		failure.Attempt = httptrace.GetScope(ctx).Attempt
		if _, present := engine.GetVar("shouldRetry"); present && failure.Kind != "script" && failure.Kind != "configuration" {
			decision, hookErr := engine.SafeCallYakFunction(ctx, "shouldRetry", []any{req, failure})
			if hookErr != nil {
				return nil, &aispec.ModelCallFailure{Kind: "script", Cause: fmt.Errorf("shouldRetry: %w", hookErr)}
			}
			if decision != nil {
				typed, ok := decision.(*aispec.RetryDecision)
				if !ok {
					return nil, &aispec.ModelCallFailure{Kind: "script", Cause: fmt.Errorf("shouldRetry must return ai.Retry, ai.StopRetry or nil, got %T", decision)}
				}
				failure.Decision = typed
			}
		}
		req.SetFailure(failure)
		return nil, failure
	})
}
