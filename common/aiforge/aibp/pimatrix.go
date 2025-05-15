package aibp

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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

//go:embed pimatrix_prompts/plan.txt
var pimatrixPlanMock string

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
		aiforge.WithPlanMocker(func(config *aid.Config) *aid.PlanResponse {
			result, err := aid.ExtractPlan(config, pimatrixPlanMock)
			if err != nil {
				config.EmitError("pimatrix plan mock failed: %s", err)
				return nil
			}
			return result
		}),
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
			aid.WithDisableToolUse(true),
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

	err = aiforge.RegisterLiteForge(
		"pimatrix-quick",
		aiforge.WithLiteForge_Prompt(`# What's S-M-A-R-T
SMART 代表：1. Specific（具体的） 2. Measurable（可衡量的） 3. Achievable（可实现的） 4. Relevant（相关的） 5. Time-bound（有时限的）。
SMART 是一个用于设定目标和评估目标达成度的标准。它帮助人们设定清晰、可行和可衡量的目标，以便更好地规划和实现个人或团队的愿景和任务。
请你在用户输入和执行任务的时候，引导用户从这几个角度考虑。
## 注意
1. 你运行在一个由外部思维链约束的任务中，尽量保持输出简短，保留任务相关元素，避免冗长描述`),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithNumberParam(
				"probability",
				aitool.WithParam_Min(0.0),
				aitool.WithParam_Max(0.999),
				aitool.WithParam_Description("Likelihood of risk occurrence"),
			),
			aitool.WithNumberParam(
				"impact",
				aitool.WithParam_Min(0.0),
				aitool.WithParam_Max(0.999),
				aitool.WithParam_Description("Magnitude of negative consequences"),
			),
			aitool.WithStringParam(
				"reason_zh",
				aitool.WithParam_MaxLength(100),
				aitool.WithParam_Description("Reason in Chinese"),
			),
			aitool.WithStringParam(
				"reason_en",
				aitool.WithParam_MaxLength(100),
				aitool.WithParam_Description("Reason in English"),
			),
		),
	)
	if err != nil {
		log.Errorf("register pimatrix forge failed: %s", err)
	}
}
