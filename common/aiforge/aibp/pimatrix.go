package aibp

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

type PIMatrixForge struct {
}

//go:embed pimatrix_prompts/persistent.txt
var pimatrixPersistentPrompts string

//go:embed pimatrix_prompts/init.txt
var pimatrixInitPrompt string

//go:embed pimatrix_prompts/result.txt
var pimatrixResultPrompt string

type PIMatrixResult struct {
	Probability float64 `json:"probability"`
	Impact      float64 `json:"impact"`
	Reason      string  `json:"reason"`
	ReasonEn    string  `json:"reason_en"`
}

func NewPIMatrixForge(callback func(result *PIMatrixResult), opts ...aid.Option) *aiforge.ForgeBlueprint {
	forge := aiforge.NewForgeBlueprint(
		"pimatrix",
		aiforge.WithInitializePrompt(pimatrixInitPrompt),
		aiforge.WithPersistentPrompt(pimatrixPersistentPrompts),
		aiforge.WithResultPrompt(pimatrixResultPrompt),
		aiforge.WithResultHandler(func(s string, err error) {
			action, err := aid.ExtractAction(s, "riskscore", "pimatrix")
			if err != nil {
				log.Errorf("Failed to extract action from pimatrix: %s", err)
				return
			}
			prob := action.GetFloat("probability")
			impact := action.GetFloat("impact")
			reason := action.GetString("reason")
			reason_en := action.GetString("reason_en")
			result := &PIMatrixResult{
				Probability: prob,
				Impact:      impact,
				Reason:      reason,
				ReasonEn:    reason_en,
			}
			if callback != nil {
				callback(result)
			} else {
				log.Error("pimatrix result callback not set")
			}
		}),
		aiforge.WithAIDOptions(append(
			opts,
			aid.WithYOLO(true),
		)...),
	)
	return forge
}
