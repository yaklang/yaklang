package sfcompletion

import (
	"context"
	_ "embed"
	"io"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	_ "github.com/yaklang/yaklang/common/aiforge/aibp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CompleteRuleDesc 用于给 sf rule 文件的 desc 中信息项内容补全，包括 title、title_zh、desc、solution 等。
// 委托 sf_desc_completion 执行，将 AI 输出合并到规则内容后返回，不写回。
func CompleteRuleDesc(
	fileName, ruleContent string,
	aiConfig ...aispec.AIConfigOption,
) (string, error) {
	aiCallback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

	if fileName == "" {
		return "", utils.Errorf("complete rule failed: fileName is required")
	}
	if ruleContent == "" {
		return "", utils.Errorf("complete rule failed: ruleContent is required")
	}
	forgeResult, err := aiforge.ExecuteForge(
		"sf_desc_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "file_name", Value: fileName},
			{Key: "file_content", Value: ruleContent},
		},
		aicommon.WithAgreeYOLO(),
		aicommon.WithAICallback(aiCallback),
	)
	if err != nil {
		return "", utils.Errorf("complete rule failed: %v", err)
	}
	descParams := aitool.InvokeParams{
		"title":     forgeResult.GetString("title"),
		"title_zh":  forgeResult.GetString("title_zh"),
		"desc":      forgeResult.GetString("desc"),
		"solution":  forgeResult.GetString("solution"),
		"reference": forgeResult.GetString("reference"),
		"cwe":       forgeResult.GetInt("cwe"),
	}
	merged, err := syntaxflow.MergeCompletionResults(descParams, nil, ruleContent)
	if err != nil {
		return "", utils.Errorf("merge completion results failed: %v", err)
	}
	return merged, nil
}
