package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
)

func TestFilterSyntaxFlowRuleByMode(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	sourceRule, err := sfdb.CreateRuleByContent("test-mode-source-rule", `
desc(mode: "source", language: general, title: t)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit
`, false)
	require.NoError(t, err)
	ssaRule, err := sfdb.CreateRuleByContent("test-mode-ssa-rule", `
desc(language: general, title: t)
${*}.pattern_regex(/foo/) as $hit
alert $hit
`, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Unscoped().Where("rule_name IN (?)", []string{sourceRule.RuleName, ssaRule.RuleName}).Delete(&schema.SyntaxFlowRule{}).Error
	})
	require.Equal(t, schema.SFR_MODE_SOURCE, sourceRule.Mode)
	require.Equal(t, schema.SFR_MODE_SSA, ssaRule.Mode)

	var sourceNames []string
	err = ApplySyntaxFlowRuleModeFilter(FilterSyntaxFlowRule(db, nil), []string{"source"}).
		Pluck("rule_name", &sourceNames).Error
	require.NoError(t, err)
	require.Contains(t, sourceNames, sourceRule.RuleName)
	require.NotContains(t, sourceNames, ssaRule.RuleName)

	var ssaNames []string
	err = ApplySyntaxFlowRuleModeFilter(FilterSyntaxFlowRule(db, nil), []string{"ssa"}).
		Pluck("rule_name", &ssaNames).Error
	require.NoError(t, err)
	require.Contains(t, ssaNames, ssaRule.RuleName)
	require.NotContains(t, ssaNames, sourceRule.RuleName)
}
