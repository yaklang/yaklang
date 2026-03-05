package syntaxflow

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// MergeCompletionResults 将 sf_desc_completion 和 sf_alert_completion 的 AI 输出合并到规则内容中。
// 供 sf_rule_completion 与 CompleteRuleDesc 共用，避免重复实现合并逻辑。
func MergeCompletionResults(descParams, alertParams aitool.InvokeParams, ruleContent string) (string, error) {
	if descParams == nil {
		return "", nil // 调用方应校验
	}

	descHandler := func(key, value string) string {
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

	normalizeAlertName := func(s string) string {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "$") {
			return s[1:]
		}
		return s
	}

	alertHandler := func(name, key, value string) string {
		if alertParams == nil {
			return value
		}
		array := alertParams.GetObjectArray("alert")
		if len(array) == 0 {
			return value
		}
		nameNorm := normalizeAlertName(name)
		for _, invokeParams := range array {
			aiName := normalizeAlertName(invokeParams.GetString("name"))
			if nameNorm != aiName {
				continue
			}
			aiVal := invokeParams.GetString(key)
			if aiVal == "" {
				return value
			}
			return aiVal
		}
		return value
	}

	return sfvm.FormatRule(ruleContent,
		sfvm.RuleFormatWithRequireInfoDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(descHandler),
		sfvm.RuleFormatWithAlertHandler(alertHandler),
		sfvm.RuleFormatWithRequireAlertDescKeyType(sfvm.GetAlertDescKeyType()...),
	)
}
