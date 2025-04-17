package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed sqlinject_prompts/init.txt
var sqlInjectInitPrompt string

//go:embed sqlinject_prompts/persistent.txt
var sqlInjectExecutePrompt string

func init() {
	err := aiforge.RegisterForgeExecutor("sqlinject", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		forge := aiforge.NewForgeBlueprint(
			"sqlinjet",
			aiforge.WithInitializePrompt(sqlInjectInitPrompt),
			aiforge.WithPersistentPrompt(sqlInjectExecutePrompt),
			aiforge.WithAIDOptions(option...),
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
		log.Error("recon init fail", "error", err)
	} else {
		log.Infof("recon init success")
	}
}
