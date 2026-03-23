package sfdb

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin/standards"
)

// enrichRuleGroups 为规则生成增强的分组列表
// 基于 CWE、文件路径等信息自动匹配标准分组（OWASP、框架等）
func enrichRuleGroups(rule *schema.SyntaxFlowRule, filePath string) []string {
	// 获取全局元数据增强器
	enricher, err := standards.GetGlobalEnricher()
	if err != nil {
		log.Warnf("get metadata enricher failed: %v, skip group enrichment", err)
		return nil
	}

	// 调用增强器生成分组名称
	enrichedGroups := enricher.EnrichGroupNames(
		rule.RuleName,
		filePath,
		rule.CWE,
	)

	if len(enrichedGroups) > 0 {
		log.Debugf("enriched %d groups for rule %s: %v", len(enrichedGroups), rule.RuleName, enrichedGroups)
	}

	return enrichedGroups
}
