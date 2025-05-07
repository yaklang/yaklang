package aisecretary

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/reducer"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strings"
)

func init() {
	aiforge.RegisterForgeExecutor("long-text-summarizer", func(
		ctx context.Context,
		items []*ypb.ExecParamItem,
		option ...aid.Option) (*aiforge.ForgeResult, error) {
		var textChan chan string
		if longText := aiforge.GetCliValueByKey("text", items); longText != "" {
			textChan = utils.NewTextSplitter().SplitReader(ctx, strings.NewReader(longText))
		} else if filePath := aiforge.GetCliValueByKey("file", items); filePath != "" {
			fileReader, err := os.OpenFile(filePath, os.O_RDONLY, os.ModePerm)
			if err != nil {
				return nil, err
			}
			textChan = utils.NewTextSplitter().SplitReader(ctx, fileReader)
		}

		fragmentSummarize := func(polyData string) string {
			result, err := aiforge.ExecuteForge(
				"fragment-summarizer",
				context.Background(),
				[]*ypb.ExecParamItem{
					{Key: "textSnippet", Value: polyData},
					{Key: "limit", Value: "100"},
				},
				aid.WithDebugPrompt(true),
				aid.WithAICallback(aiforge.GetHoldAICallback()),
			)
			if err != nil {
				return ""
			}
			return result.Formated.(string)
		}

		textReducer := reducer.NewReducer(10, func(data []string) string {
			polyData := strings.Join(data, "\n")
			return fragmentSummarize(polyData)
		})

		for s := range textChan {
			textReducer.Push(fragmentSummarize(s))
		}

		reduceData := strings.Join(textReducer.GetData(), "\n")
		result, err := aiforge.ExecuteForge(
			"fragment-summarizer",
			context.Background(),
			[]*ypb.ExecParamItem{
				{Key: "textSnippet", Value: reduceData},
				{Key: "limit", Value: "300"},
			},
			aid.WithDebugPrompt(true),
			aid.WithAICallback(aiforge.GetHoldAICallback()),
		)
		if err != nil {
			return nil, err
		}
		return &aiforge.ForgeResult{
			Forge: aiforge.NewForgeBlueprint("long-text-summarizer", aiforge.WithOriginYaklangCliCode(`
cli.String("text", cli.setRequired(true), cli.help("长文本内容"))`)),
			Formated: result.Formated,
		}, nil
	})
}
