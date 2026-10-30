package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/yakscript"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func init() {
	yakscript.RegisterExecutor(func(ctx context.Context, source string, req *yakscript.Request, emit func(string) error) (any, error) {
		// A fresh VM isolates invocation state. Public helpers come from ai.Exports,
		// the same library table used by ordinary Yak language tooling.
		engine := yaklang.New()
		if err := engine.SafeEval(ctx, source); err != nil {
			return nil, err
		}
		return engine.SafeCallYakFunction(ctx, "chat", []any{req, emit})
	})
}
