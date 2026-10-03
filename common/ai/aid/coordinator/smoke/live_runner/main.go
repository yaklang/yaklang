// Run the comprehensive Yak/aim experiment with real configured model responses.
// This bridge scripts only one human plan approval, never a model decision.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := consts.InitializeYakitDatabase("", "", ""); err != nil {
		return err
	}
	if err := yakit.CallPostInitDatabase(); err != nil {
		return err
	}
	source, err := os.ReadFile("common/ai/aid/coordinator/smoke/comprehensive_live.yak")
	if err != nil {
		return err
	}
	engine := yak.NewScriptEngine(1)
	engine.RegisterEngineHooks(func(e *antlr4yak.Engine) error {
		e.SetVars(map[string]any{
			"BENCH_CONFIRM_PLAN": func(op aicommon.AIEngineOperator, endpoint string) error {
				return op.SendInputEvent(&ypb.AIInputEvent{
					IsInteractiveMessage: true, InteractiveId: endpoint,
					InteractiveJSONInput: `{"suggestion":"continue"}`,
				})
			},
		})
		return nil
	})
	_, err = engine.ExecuteExWithContext(context.Background(), string(source), nil)
	return err
}
