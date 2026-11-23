package sfbuildin

import (
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
)

func normalizeEmbeddedRuleSeverities(severities []schema.SyntaxFlowSeverity) map[schema.SyntaxFlowSeverity]struct{} {
	if len(severities) == 0 {
		return nil
	}
	wanted := make(map[schema.SyntaxFlowSeverity]struct{}, len(severities))
	for _, severity := range severities {
		wanted[schema.ValidSeverityType(severity)] = struct{}{}
	}
	return wanted
}

func embeddedRulePathMatchesLanguage(path, language string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(path), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	return strings.Contains("/"+normalized+"/", "/"+strings.ToLower(language)+"/")
}

func compileEmbeddedRuleForLanguage(path, raw, language string, severities map[schema.SyntaxFlowSeverity]struct{}) (*schema.SyntaxFlowRule, error) {
	rule, err := sfdb.CheckSyntaxFlowRuleContent(raw)
	if err != nil {
		return nil, utils.Wrapf(err, "compile embedded rule %s", path)
	}
	if !strings.EqualFold(string(rule.Language), language) {
		return nil, nil
	}
	if len(severities) > 0 {
		if _, ok := severities[schema.ValidSeverityType(rule.Severity)]; !ok {
			return nil, nil
		}
	}
	rule.IsBuildInRule = true
	return rule, nil
}
