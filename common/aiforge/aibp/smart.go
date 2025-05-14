package aibp

import (
	"context"
	"sync"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	action      *aid.Action
	Suggestions []*SmartSuggestion
}

func newSmartForge(callback func(result *SmartResult), opts ...aid.Option) *aiforge.ForgeBlueprint {
	forge := aiforge.NewForgeBlueprint(
		"smart",
		aiforge.WithPlanMocker(func(config *aid.Config) *aid.PlanResponse {
			plan, err := aid.ExtractPlan(config, smartPlanMock)
			if err != nil {
				config.EmitError("mock SMART Plan failed: %v", err)
				return nil
			}
			return plan
		}),
		aiforge.WithInitializePrompt(smartInitPrompt),
		aiforge.WithPersistentPrompt(smartPersistentPrompts),
		aiforge.WithResultPrompt(smartResultPrompt),
		aiforge.WithResultHandler(func(s string, err error) {
			action, err := aid.ExtractAction(s, "smart")
			if err != nil {
				log.Errorf("Failed to extract action from smart: %s", err)
				return
			}
			result := &SmartResult{
				action: action,
			}
			for _, sug := range action.GetInvokeParamsArray("suggestions") {
				result.Suggestions = append(result.Suggestions, &SmartSuggestion{
					Prompt:      sug.GetString("prompt"),
					Description: sug.GetString("description"),
				})
			}
			if callback != nil {
				callback(result)
			} else {
				log.Error("smart result callback not set")
			}
		}),
		aiforge.WithAIDOptions(append(
			opts,
			aid.WithAgreeYOLO(true),
			aid.WithDisableToolUse(true),
		)...),
	)
	return forge
}

func init() {
	err := aiforge.RegisterForgeExecutor("smart", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		m := new(sync.Mutex)
		var finalResult *SmartResult
		forge := newSmartForge(func(result *SmartResult) {
			m.Lock()
			finalResult = result
			m.Unlock()
		}, option...)
		ins, err := forge.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, utils.Errorf("create coordinator failed: %s", err)
		}
		err = ins.Run()
		if finalResult == nil {
			return nil, utils.Errorf("smart result is empty")
		}
		result := &aiforge.ForgeResult{
			Action: finalResult.action,
		}
		if err != nil {
			return nil, utils.Errorf("smart run failed: %s", err)
		}
		return result, nil
	})
	if err != nil {
		log.Errorf("Failed to register smart forge: %s", err)
	}
}
