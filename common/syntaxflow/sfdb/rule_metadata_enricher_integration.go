package sfdb

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin/standards"
)

// enrichRuleTags builds atomic tags for a rule (replaces enrichRuleGroups).
func enrichRuleTags(rule *schema.SyntaxFlowRule, filePath string) []string {
	enricher, err := standards.GetGlobalEnricher()
	if err != nil {
		log.Warnf("get metadata enricher failed: %v, skip tag enrichment", err)
		return nil
	}
	return enricher.EnrichAtomicTags(rule.RuleName, filePath, rule.CWE)
}

// enrichRuleGroups is deprecated; kept for tests that still call the old path.
func enrichRuleGroups(rule *schema.SyntaxFlowRule, filePath string) []string {
	enricher, err := standards.GetGlobalEnricher()
	if err != nil {
		log.Warnf("get metadata enricher failed: %v, skip group enrichment", err)
		return nil
	}
	return enricher.EnrichGroupNames(rule.RuleName, filePath, rule.CWE)
}
