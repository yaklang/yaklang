package syntaxflow

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

var existingRuleIDPattern = regexp.MustCompile(`(?m)rule_id:\s*"([^"]*)"`)

// extractExistingRuleID 从规则文本中读取已有 rule_id（文本匹配，不做语法编译）。
func extractExistingRuleID(ruleContent string) string {
	m := existingRuleIDPattern.FindStringSubmatch(ruleContent)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// MergeBeautificationResults 将 AI 美化的 desc/alert 输出合并到原始规则内容中。
// rule_id：原规则已有则保留，否则自动生成 UUID。
func MergeBeautificationResults(descParams, alertParams aitool.InvokeParams, ruleContent string) (string, error) {
	if descParams == nil {
		return "", nil // 调用方应校验
	}

	existingRuleID := extractExistingRuleID(ruleContent)
	ruleID := existingRuleID
	if ruleID == "" {
		if got := descParams.GetString("rule_id"); got != "" {
			ruleID = got
		} else {
			ruleID = uuid.NewString()
		}
	}

	descHandler := func(key, value string) string {
		typ := sfvm.ValidDescItemKeyType(key)
		if typ == sfvm.SFDescKeyType_Rule_Id {
			if existingRuleID != "" {
				return existingRuleID
			}
			if got := descParams.GetString("rule_id"); got != "" {
				return got
			}
			return ruleID
		}
		// level 禁止被 AI 覆盖（主 desc 块）
		if typ == sfvm.SFDescKeyType_Level {
			return value
		}
		if typ == sfvm.SFDescKeyType_Unknown {
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
		// alert level 禁止被 AI 覆盖
		if typ := sfvm.ValidDescItemKeyType(key); typ == sfvm.SFDescKeyType_Level {
			return value
		}
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

	return sfvm.FormatRuleForBeautification(ruleContent,
		sfvm.RuleFormatWithRuleID(ruleID),
		sfvm.RuleFormatWithRequireInfoDescKeyType(sfvm.GetSupplyInfoDescKeyType()...),
		sfvm.RuleFormatWithDescHandler(descHandler),
		sfvm.RuleFormatWithAlertHandler(alertHandler),
		sfvm.RuleFormatWithRequireAlertDescKeyType(sfvm.GetAlertDescKeyType()...),
	)
}

// MergeBeautificationResultsForYak 供 Yak 调用，descMap/alertMap 为 map[string]any。
func MergeBeautificationResultsForYak(descMap, alertMap any, ruleContent string) (string, error) {
	descParams := aitool.InvokeParams(utils.InterfaceToGeneralMap(descMap))
	alertParams := aitool.InvokeParams(utils.InterfaceToGeneralMap(alertMap))
	return MergeBeautificationResults(descParams, alertParams, ruleContent)
}
