package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed travelmaster_prompts/init.txt
var travelMasterInitPrompt string

//go:embed travelmaster_prompts/persistent.txt
var travelMasterExecutePrompt string

func _init_travelmaster() {
	err := aiforge.RegisterForgeExecutor("travelmaster", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		forge := aiforge.NewForgeBlueprint(
			"travelmaster",
			aiforge.WithInitializePrompt(travelMasterInitPrompt),
			aiforge.WithPersistentPrompt(travelMasterExecutePrompt),
			aiforge.WithAIOptions(option...),
			aiforge.WithTools(yakscripttools.GetYakScriptAiTools("amap")...),
		)

		co, err := forge.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		err = co.Run()
		if err != nil {
			return nil, err
		}
		return &aiforge.ForgeResult{
			Forge: forge,
		}, nil
	})
	if err != nil {
		log.Error("travelmaster init fail", "error", err)
	}
}
