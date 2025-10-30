package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed recon_prompts/init.txt
var reconInitPrompt string

//go:embed recon_prompts/persistent.txt
var reconPersistentPrompts string

func newRecon(extraOpt ...aicommon.ConfigOption) *aiforge.ForgeBlueprint {
	var opts []aicommon.ConfigOption
	opts = append(opts, extraOpt...)
	forge := aiforge.NewForgeBlueprint(
		"recon",
		aiforge.WithInitializePrompt(reconInitPrompt),
		aiforge.WithPersistentPrompt(reconPersistentPrompts),
		aiforge.WithAIOptions(opts...),
	)
	return forge
}

func _init_recon() {
	err := aiforge.RegisterForgeExecutor("recon", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		forge := newRecon(option...)
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
	}
}
