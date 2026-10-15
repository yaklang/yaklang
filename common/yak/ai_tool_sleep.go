package yak

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// Bind only this AI invocation. A long native sleep must yield to tool stop so
// the VM can observe cancellation and run its deferred cleanup instructions.
func bindAIToolSleep(engine *antlr4yak.Engine, ctx context.Context) {
	if ctx == nil {
		return
	}
	sleep := func(seconds float64) {
		timer := time.NewTimer(utils.FloatSecondDuration(seconds))
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	timeExports := make(map[string]any)
	if original, ok := engine.Var("time").(map[string]any); ok {
		for name, value := range original {
			timeExports[name] = value
		}
	}
	timeExports["Sleep"], timeExports["sleep"] = sleep, sleep
	engine.SetVars(map[string]any{
		"sleep": sleep,
		"time":  timeExports,
	})
}
