package aisecretary

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
)

//go:embed fragment_summarizer_prompts/persistent.txt
var summarizerPersistentPrompt string

//go:embed fragment_summarizer_prompts/plan.txt
var summarizerPlanMock string

func init() {
	aiforge.RegisterForgeExecutor("fragment-summarizer", func(
		ctx context.Context,
		items []*ypb.ExecParamItem,
		option ...aicommon.ConfigOption) (*aiforge.ForgeResult, error) {
		var summary string
		limit, err := strconv.Atoi(aiforge.GetCliValueByKey("limit", items))
		if err != nil || limit <= 0 {
			limit = 50
		}
		bp := aiforge.NewForgeBlueprint(
			"fragment-summarizer",
			aiforge.WithPersistentPrompt(fmt.Sprintf(summarizerPersistentPrompt, limit)),
			aiforge.WithPlanMocker(func(config *aid.Coordinator) *aid.PlanResponse {
				result, err := aid.ExtractPlan(config, summarizerPlanMock)
				if err != nil {
					config.EmitError("fragment summarizer plan mock failed: %s", err)
					return nil
				}
				return result
			}),
			aiforge.WithOriginYaklangCliCode(`
cli.String("textSnippet", cli.setRequired(true), cli.help("文本片段内容"))
cli.Int("limit",cli.help("字数限制"))
`),

			aiforge.WithAIOptions(
				aicommon.WithAgreeYOLO(),
				aicommon.WithExtendedActionCallback("summarize", func(config *aicommon.Config, action *aicommon.Action) {
					summary = action.GetString("summary")
				}),
				aid.WithResultHandler(func(config *aid.Coordinator) {}),
			),
		)
		ord, err := bp.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, err
		}
		err = ord.Run()
		if err != nil {
			return nil, err
		}
		return &aiforge.ForgeResult{
			Forge:    bp,
			Formated: summary,
		}, nil
	})
}
