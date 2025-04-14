package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

//go:embed pimatrix_prompts/persistent.txt
var pimatrixPersistentPrompts string

//go:embed pimatrix_prompts/init.txt
var pimatrixInitPrompt string

//go:embed pimatrix_prompts/result.txt
var pimatrixResultPrompt string

type PIMatrixResult struct {
	action      *aid.Action
	Probability float64 `json:"probability"`
	Impact      float64 `json:"impact"`
	Reason      string  `json:"reason"`
	ReasonEn    string  `json:"reason_en"`
}

func newPIMatrixForge(callback func(result *PIMatrixResult), opts ...aid.Option) *aiforge.ForgeBlueprint {
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
				action:      action,
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

func init() {
	err := aiforge.RegisterForgeExecutor("pimatrix", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		m := new(sync.Mutex)
		var finalResult *PIMatrixResult
		forge := newPIMatrixForge(func(result *PIMatrixResult) {
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
			return nil, utils.Errorf("pimatrix result is empty")
		}
		result := &aiforge.ForgeResult{
			Action:   finalResult.action,
			Formated: finalResult,
			Forge:    forge,
		}
		if err != nil {
			log.Errorf("pimatrix result failed: %s", err)
		}
		return result, nil
	})
	if err != nil {
		log.Errorf("register pimatrix forge failed: %s", err)
	} else {
		log.Infof("register pimatrix forge success")
	}
}
