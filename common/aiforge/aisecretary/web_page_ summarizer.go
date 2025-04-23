package aisecretary

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yaklangtools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func newWebPageSummarizerForge(callback func(string)) *aiforge.ForgeBlueprint {
	yaklangTools, err := yaklangtools.CreateYaklangTools()
	if err != nil {
		log.Errorf("create yaklang tools: %v", err)
		return nil
	}
	fileOp, err := fstools.CreateSystemFSTools()
	if err != nil {
		log.Errorf("create system fs tools tools: %v", err)
		return nil
	}
	extTools := append(fileOp, yaklangTools...)

	forge := aiforge.NewForgeBlueprint(
		"yaklang-reviewer",
		aiforge.WithTools(extTools...),
		aiforge.WithInitializePrompt(_initPrompt),
		aiforge.WithPlanMocker(func(config *aid.Config) *aid.PlanResponse {
			res, err := aid.ExtractPlan(config, _codeReviewPlanMock)
			if err != nil {
				config.EmitError("yak review plan mock failed: %s", err)
				return nil
			}
			return res
		}),

		aiforge.WithPersistentPrompt(_persistentPrompt),
		aiforge.WithOriginYaklangCliCode(fmt.Sprintf(`
cli.String("%s", cli.setRequired(true),cli.setVerboseName("yaklang代码"), cli.help("代码内容"))
cli.check()
`, magicCode)),
		aiforge.WithAIDOptions(
			aid.WithAgreeManual(),
			aid.WithResultHandler(func(config *aid.Config) {
				code, _ := config.GetMemory().UserDataGet("code")
				callback(code)
			}),
			aid.WithExtendedActionCallback("set-code", func(config *aid.Config, action *aid.Action) {
				codeContent := action.GetString("content")
				config.GetMemory().StoreUserData(magicCode, codeContent)
			}),
			aid.WithAgreeAIAssistant(&aid.AIAssistant{
				Callback: func(ctx context.Context, config *aid.Config) (*aid.AIAssistantResult, error) {
					m := config.GetMemory()
					_, eventIns, ok := m.GetInteractiveEventLast()
					if !ok {
						return nil, utils.Error("Interactive Event Not Found")
					}
					res := &aid.AIAssistantResult{}
					if eventIns.InteractiveEvent.Type == aid.EVENT_TYPE_TASK_REVIEW_REQUIRE {
						res.Param = analyzeToolCallResult(m)
					}
					return res, nil
				},
			}),
		),
	)
	return forge
}
