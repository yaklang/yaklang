package aisecretary

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed fragment_summarizer_prompts/persistent.txt
var summarizerPersistentPrompt string

//go:embed fragment_summarizer_prompts/plan.txt
var summarizerPlanMock string

func init() {
	aiforge.RegisterForgeExecutor("fragment-summarizer", func(
		ctx context.Context,
		items []*ypb.ExecParamItem,
		option ...aid.Option) (*aiforge.ForgeResult, error) {
		var summary string
		bp := aiforge.NewForgeBlueprint(
			"fragment-summarizer",
			aiforge.WithPersistentPrompt(summarizerPersistentPrompt),
			aiforge.WithPlanMocker(func(config *aid.Config) *aid.PlanResponse {
				result, err := aid.ExtractPlan(config, summarizerPlanMock)
				if err != nil {
					config.EmitError("fragment summarizer plan mock failed: %s", err)
					return nil
				}
				return result
			}),
			aiforge.WithOriginYaklangCliCode(`
cli.String("textSnippet", cli.setRequired(true), cli.help("文本片段内容"))
`),

			aiforge.WithAIDOptions(
				aid.WithYOLO(true),
				aid.WithExtendedActionCallback("summarize", func(config *aid.Config, action *aid.Action) {
					summary = action.GetString("summary")
				})),
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
