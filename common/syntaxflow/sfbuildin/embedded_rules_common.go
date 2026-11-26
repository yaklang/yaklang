package sfbuildin

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
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

// loadEmbeddedRulesFromFS walks one embedded snapshot and compiles matching
// rules. Both the full and the slim build share this path so their ordering and
// filtering cannot drift.
func loadEmbeddedRulesFromFS(fsInstance filesys_interface.FileSystem, language string, severities []schema.SyntaxFlowSeverity) ([]*schema.SyntaxFlowRule, error) {
	wanted := normalizeEmbeddedRuleSeverities(severities)
	var rules []*schema.SyntaxFlowRule
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info == nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".sf") {
			return nil
		}
		if !embeddedRulePathMatchesLanguage(path, language) {
			return nil
		}
		raw, err := fsInstance.ReadFile(path)
		if err != nil {
			return utils.Wrapf(err, "read embedded rule %s", path)
		}
		rule, err := compileEmbeddedRuleForLanguage(path, string(raw), language, wanted)
		if err != nil {
			return err
		}
		if rule != nil {
			rules = append(rules, rule)
		}
		return nil
	}))
	if err != nil {
		return nil, err
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].RuleName < rules[j].RuleName
	})
	return rules, nil
}
