package sfcompletion

import (
	"context"
	_ "embed"
	"io"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak" // ensure ExecuteForge callback registered
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CompleteRule 使用 sf_rule_completion forge 补全 SyntaxFlow 规则的 desc 与 alert 块，返回合并后的规则文本。
func CompleteRule(
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
	result, err := aicommon.ExecuteForgeFromDB(
		"sf_rule_completion",
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
	merged := extractRuleContent(result)
	if merged == "" {
		return "", utils.Errorf("complete rule failed: forge returned empty rule_content")
	}
	return merged, nil
}

// extractRuleContent 从 sf_rule_completion forge 返回值中提取 rule_content。
// forge 返回 {"params": {"rule_content": "..."}}
func extractRuleContent(result any) string {
	if result == nil {
		return ""
	}
	m := utils.InterfaceToGeneralMap(result)
	if m == nil {
		return ""
	}
	params := utils.MapGetMapRaw(m, "params")
	if params == nil {
		return ""
	}
	return utils.MapGetString(params, "rule_content")
}

// CompleteRuleDesc 为保持兼容，委托 CompleteRule 执行（现同时补全 desc 与 alert）。
func CompleteRuleDesc(fileName, ruleContent string, aiConfig ...aispec.AIConfigOption) (string, error) {
	return CompleteRule(fileName, ruleContent, aiConfig...)
}
