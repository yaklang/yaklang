package aibp

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
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
	action      *aicommon.Action
	Probability float64 `json:"probability"`
	Impact      float64 `json:"impact"`
	Reason      string  `json:"reason"`
	ReasonEn    string  `json:"reason_en"`
}

func init() {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(`# What's S-M-A-R-T
SMART 代表：1. Specific（具体的） 2. Measurable（可衡量的） 3. Achievable（可实现的） 4. Relevant（相关的） 5. Time-bound（有时限的）。
SMART 是一个用于设定目标和评估目标达成度的标准。它帮助人们设定清晰、可行和可衡量的目标，以便更好地规划和实现个人或团队的愿景和任务。
请你在用户输入和执行任务的时候，引导用户从这几个角度考虑。
## 注意
1. 你运行在一个由外部思维链约束的任务中，尽量保持输出简短，保留任务相关元素，避免冗长描述`),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithNumberParam(
				"probability",
				aitool.WithParam_Required(true),
				aitool.WithParam_Min(0.001),
				aitool.WithParam_Max(0.999),
				aitool.WithParam_Description("Likelihood of risk occurrence"),
			),
			aitool.WithNumberParam(
				"impact",
				aitool.WithParam_Required(true),
				aitool.WithParam_Min(0.001),
				aitool.WithParam_Max(0.999),
				aitool.WithParam_Description("Magnitude of negative consequences"),
			),
			aitool.WithStringParam(
				"reason_zh",
				aitool.WithParam_Required(true),
				aitool.WithParam_MaxLength(100),
				aitool.WithParam_Description("Reason in Chinese"),
			),
			aitool.WithStringParam(
				"reason_en",
				aitool.WithParam_Required(true),
				aitool.WithParam_MaxLength(100),
				aitool.WithParam_Description("Reason in English"),
			),
		),
	}

	err := aiforge.RegisterAIDBuildInForge("pimatrix", lfopts...)
	if err != nil {
		log.Errorf("register pimatrix forge failed: %s", err)
	}
	err = aiforge.RegisterLiteForge("pimatrix-quick", lfopts...)
	if err != nil {
		log.Errorf("register pimatrix forge failed: %s", err)
	}
}
