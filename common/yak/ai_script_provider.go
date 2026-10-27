package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/yakscript"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func init() {
	yakscript.RegisterExecutor(func(ctx context.Context, source string, req *yakscript.Request, emit func(string) error) (any, error) {
		// A fresh VM per invocation isolates script globals and callbacks during
		// overlapping main, verification and auxiliary model calls.
		engine := yaklang.New()
		engine.SetVars(map[string]any{"provider": map[string]any{
			"ReadSSE": yakscript.ReadSSE, "Retry": yakscript.Retry,
			"IsTransientError": yakscript.IsTransientError,
		}})
		if err := engine.SafeEval(ctx, source); err != nil {
			return nil, err
		}
		return engine.SafeCallYakFunction(ctx, "chat", []any{req, emit})
	})
}
