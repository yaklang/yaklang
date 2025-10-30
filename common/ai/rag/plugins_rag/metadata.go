package plugins_rag

import (
	"context"
	"fmt"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed prompt/init.txt
var generateMetadataPrompt string

func GenerateYakScriptMetadata(forgeContent string) (*GenerateResult, error) {
	var lfopts []aiforge.LiteForgeOption
	lfopts = append(lfopts,
		aiforge.WithLiteForge_Prompt(generateMetadataPrompt))
	lfopts = append(lfopts, aiforge.WithLiteForge_OutputSchema(
		aitool.WithStringParam("language", aitool.WithParam_Required(true), aitool.WithParam_Description("语言，固定为chinese")),
		aitool.WithStringParam("description", aitool.WithParam_Required(true), aitool.WithParam_Description("脚本功能描述")),
		aitool.WithStringArrayParam("keywords", aitool.WithParam_Required(true), aitool.WithParam_Description("关键词数组")),
	))

	lfopts = append(lfopts, aiforge.WithExtendLiteForge_AIOption(
	// aid.WithDebugPrompt(true),
	))

	lf, err := aiforge.NewLiteForge("generate_metadata", lfopts...)
	if err != nil {
		return nil, err
	}
	result, err := lf.Execute(context.Background(), []*ypb.ExecParamItem{
		{
			Key:   "query",
			Value: forgeContent,
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Action == nil {
		return nil, fmt.Errorf("extract action failed")
	}

	// Extract the result
	params := result.Action.GetInvokeParams("params")
	language := params.GetString("language")
	description := params.GetString("description")
	keywords := params.GetStringSlice("keywords")

	return &GenerateResult{
		Language:    language,
		Description: description,
		Keywords:    keywords,
	}, nil
}

type GenerateResult struct {
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}
