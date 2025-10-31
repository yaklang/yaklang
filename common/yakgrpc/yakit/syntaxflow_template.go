package yakit

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var tpl string = `desc(
	title: %s
	type: audit
	level: %s
	risk: "%s"
	desc: <<<DESC

DESC
	rule_id: "%s"
)

%s

`

var part string = `
%s

desc(
	lang: %s
	alert_%s: 1
	"file://unsafe.%s": <<<UNSAFE
%s
UNSAFE
	"file://safe.%s": <<<SAFE
%s
SAFE
)
`

func createRuleByTemplate(ruleInput *ypb.SyntaxFlowRuleAutoInput) string {
	var ret string
	var keys string

	language := strings.TrimSpace(strings.ToLower(ruleInput.GetLanguage()))
	ruleName := strings.TrimSpace(ruleInput.GetRuleName())

	ruleSubjects := ruleInput.GetRuleSubjects()
	ruleSafeTests := ruleInput.GetRuleSafeTests()
	ruleUnSafeTests := ruleInput.GetRuleUnSafeTests()
	ruleLevels := ruleInput.GetRuleLevels()

	// derive basic fields
	level := deriveLevel(ruleLevels)
	risk := ruleInput.GetRiskType()
	if risk == "" {
		risk = ""
	}
	rule_id := uuid.NewString()
	ext := languageToExt(language)

	titleVal := ruleName
	if titleVal == "" {
		titleVal = fmt.Sprintf("Auto Generated %s Rule", strings.Title(language))
	}

	// 如果没有提供 subjects，使用默认值
	if len(ruleSubjects) == 0 {
		ruleSubjects = []string{"any() as $entry"}
	}

	// 安全访问数组，避免越界
	getSafeTest := func(arr []string, idx int) string {
		if idx < len(arr) {
			return arr[idx]
		}
		return ""
	}

	for i := range ruleSubjects {
		keys += fmt.Sprintf(part,
			ruleSubjects[i],                 // %s subject(s)
			language,                        // %s lang
			level,                           // %s level
			ext,                             // %s main file ext
			getSafeTest(ruleUnSafeTests, i), // %s unsafe code
			ext,                             // %s safe http client ext
			getSafeTest(ruleSafeTests, i),   // %s safe code
		)
	}

	ret = fmt.Sprintf(
		tpl,
		fmt.Sprintf("\"%s\"", escapeQuotes(titleVal)), // %s title quoted
		level,   // %s level
		risk,    // %s risk
		rule_id, // %s rule_id
		keys,    // %s keys
	)
	return ret
}

func deriveLevel(levels []string) string {
	// prefer highest severity if provided; default to info
	rank := map[string]int{"critical": 5, "high": 4, "middle": 3, "medium": 3, "low": 2, "info": 1}
	best := "info"
	bestRank := 0
	for _, l := range levels {
		ll := strings.ToLower(strings.TrimSpace(l))
		if r, ok := rank[ll]; ok && r > bestRank {
			best = ll
			bestRank = r
		}
	}
	return best
}

func languageToExt(lang string) string {
	switch strings.ToLower(lang) {
	case "golang", "go":
		return "go"
	case "java":
		return "java"
	case "php":
		return "php"
	case "javascript", "js", "node":
		return "js"
	case "python", "py":
		return "py"
	default:
		return "txt"
	}
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}
