package sfcompletion

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
)

// CompleteRuleDesc 用于给sf rule文件的desc中信息项内容补全，包括title、title_zh、desc、solution等
func CompletegRuleDesc(
	fileName, ruleContent string,
	aiConfig ...aispec.AIConfigOption,
) (string, error) {
	aiCallback := func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
		rsp := config.NewAIResponse()
		go func() {
			defer rsp.Close()
			aiConfig = append(aiConfig, aispec.WithStreamHandler(func(c io.Reader) {
				rsp.EmitOutputStream(c)
			}))
			_, err := ai.Chat(req.GetPrompt(), aiConfig...)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}

	forgeResult, err := aiforge.ExecuteForge(
		"sf_desc_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{
				Key: "file_name", Value: fileName,
			},
			{
				Key: "file_content", Value: ruleContent,
			},
		},
		aid.WithAgreeYOLO(true),
		aid.WithAICallback(aiCallback),
	)
	if err != nil {
		return "", utils.Errorf("complete rule failed: %v", err)
	}
	params := forgeResult.GetInvokeParams("params")
	if params == nil {
		return "", utils.Errorf("complete rule failed: ai response have  no params")
	}

	handler := func(key, value string) string {
		if typ := sfvm.ValidDescItemKeyType(key); typ == sfvm.SFDescKeyType_Unknown {
			return value
		}
		if got := params.GetString(key); got != "" {
			return got
		}
		if got := params.GetInt(key); got != 0 {
			return string(got)
		}
		return value
	}

	var opts []sfvm.RuleFormatOption
	opts = append(opts,
		sfvm.RuleFormatWithRequireDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(handler),
	)
	content, err := sfvm.FormatRule(ruleContent, opts...)
	if err != nil {
		return "", err
	}
	return content, nil
}
