package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/aiforge"
)

type PIMatrixForge struct {
}

//go:embed pimatrix_prompts/persistent.txt
var pimatrixPersistentPrompts string

//go:embed pimatrix_prompts/init.txt
var pimatrixInitPrompt string

//go:embed pimatrix_prompts/result.txt
var pimatrixResultPrompt string

func NewPIMatrixForge() *aiforge.ForgeBlueprint {
	forge := aiforge.NewForgeBlueprint(
		aiforge.WithInitializePrompt(pimatrixInitPrompt),
		aiforge.WithPersistentPrompt(pimatrixPersistentPrompts),
		aiforge.WithResultPrompt(pimatrixResultPrompt),
	)
	return forge
}
