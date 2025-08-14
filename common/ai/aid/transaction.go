package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func CallAITransactionWithoutConfig(
	prompt string,
	callAi func(*aicommon.AIRequest) (*aicommon.AIResponse, error),
	postHandler func(rsp *aicommon.AIResponse) error,
	requestOpts ...aicommon.AIRequestOption,
) error {
	return aicommon.CallAITransaction(nil, prompt, callAi, postHandler, requestOpts...)
}

func (c *Config) callAiTransaction(
	prompt string,
	callAi func(*aicommon.AIRequest) (*aicommon.AIResponse, error),
	postHandler func(rsp *aicommon.AIResponse) error,
	requestOpts ...aicommon.AIRequestOption,
) error {
	return aicommon.CallAITransaction(c, prompt, callAi, postHandler, requestOpts...)
}

var retryPromptTemplate = `
{{ .RawPrompt }}

# 错误处理：
注意，你生成的结果在之前已经犯过错误，这是上次失败的原因：
{{ .RetryReason }}
请你在生成结果时，注意不要再犯同样的错误。
# 如何修正？
如果要生成 action/@action JSON 可以参考后面的案例，注意格式遵守：{"@action": "...", ... }
`

func (c *Config) RetryPromptBuilder(rawPrompt string, retryErr error) string {
	if retryErr == nil {
		return rawPrompt
	}
	templateData := map[string]interface{}{
		"RetryReason": retryErr.Error(),
		"RawPrompt":   rawPrompt,
	}
	res, err := c.quickBuildPrompt(retryPromptTemplate, templateData)
	if err != nil {
		c.EmitError("failed to build retry prompt: %v", err)
		return rawPrompt
	}
	return res
}
