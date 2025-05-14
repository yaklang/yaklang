package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
)

//go:embed recon_prompts/init.txt
var reconInitPrompt string

//go:embed recon_prompts/persistent.txt
var reconPersistentPrompts string

func newRecon(extraOpt ...aid.Option) *aiforge.ForgeBlueprint {
	var opts []aid.Option
	opts = append(opts, extraOpt...)
	forge := aiforge.NewForgeBlueprint(
		"recon",
		aiforge.WithInitializePrompt(reconInitPrompt),
		aiforge.WithPersistentPrompt(reconPersistentPrompts),
		aiforge.WithAIDOptions(opts...),
	)
	return forge
}

func init() {
	err := aiforge.RegisterForgeExecutor("recon", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
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
