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

func _init_recon() {
	cfg := aiforge.NewYakForgeBlueprintConfig("recon", reconInitPrompt, reconPersistentPrompts)
	aiforge.RegisterYakAiForge(cfg)
}
