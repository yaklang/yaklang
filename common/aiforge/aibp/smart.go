package aibp

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/aiforge"
)

//go:embed smart_prompts/persistent.txt
var smartPersistentPrompts string

//go:embed smart_prompts/init.txt
var smartInitPrompt string

//go:embed smart_prompts/result.txt
var smartResultPrompt string

//go:embed smart_prompts/plan.txt
var smartPlanMock string

type SmartSuggestion struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

type SmartResult struct {
	action      *aicommon.Action
	Suggestions []*SmartSuggestion
}

func _init_smart() {
	cfg := aiforge.NewYakForgeBlueprintConfig("smart", smartInitPrompt, smartPersistentPrompts)
	cfg.WithPlanPrompt(smartPlanMock)
	cfg.WithResultPrompt(smartResultPrompt)

	optCfg := aiforge.NewYakForgeBlueprintAIDOptionsConfig()
	optCfg.WithDisableToolUse(true)
	optCfg.WithYOLO(true)
	cfg.WithAIDOptions(optCfg)
	cfg.WithActionName("smart")

	aiforge.RegisterYakAiForge(cfg)
}
