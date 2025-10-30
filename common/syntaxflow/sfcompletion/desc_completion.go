package sfcompletion

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strconv"

	"io"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	_ "github.com/yaklang/yaklang/common/aiforge/aibp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CompleteRuleDesc 用于给sf rule文件的desc中信息项内容补全，包括title、title_zh、desc、solution等
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
		aicommon.WithAgreeYOLO(),
		aicommon.WithAICallback(aiCallback),
	)
	if err != nil {
		return "", utils.Errorf("complete rule failed: %v", err)
	}
	alertResult, err := aiforge.ExecuteForge("sf_alert_completion", context.Background(), []*ypb.ExecParamItem{
		{
			Key: "file_name", Value: fileName,
		},
		{
			Key: "file_content", Value: ruleContent,
		},
	},
		aicommon.WithAgreeYOLO(),
		aicommon.WithAICallback(aiCallback))
	if err != nil {
		return "", err
	}
	descParams := forgeResult.GetInvokeParams("params")
	alertParams := alertResult.GetInvokeParams("params")
	if descParams == nil {
		return "", utils.Errorf("complete rule failed: ai response have  no params")
	}

	handler := func(key, value string) string {
		if typ := sfvm.ValidDescItemKeyType(key); typ == sfvm.SFDescKeyType_Unknown {
			return value
		}
		if got := descParams.GetString(key); got != "" {
			return got
		}
		if got := descParams.GetInt(key); got != 0 {
			return strconv.FormatInt(got, 10)
		}
		return value
	}
	var opts []sfvm.RuleFormatOption
	opts = append(opts,
		sfvm.RuleFormatWithRequireInfoDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(handler),
		sfvm.RuleFormatWithAlertHandler(func(name, key, value string) string {
			array := alertParams.GetObjectArray("alert")
			if len(array) == 0 {
				return value
			}
			for _, invokeParams := range array {
				if name != invokeParams.GetString("name") {
					continue
				}
				aiVal := invokeParams.GetString(key)
				if aiVal == "" {
					return value
				}
				return aiVal
			}
			return value
		}),
		sfvm.RuleFormatWithRequireAlertDescKeyType(sfvm.GetAlertDescKeyType()...),
	)
	content, err := sfvm.FormatRule(ruleContent, opts...)
	if err != nil {
		return "", err
	}
	return content, nil
}
