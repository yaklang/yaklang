package yakit

import (
	"fmt"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newSyntaxFlowRuleTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.LogMode(false)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}, &schema.SyntaxFlowGroup{}).Error)

	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestParseSyntaxFlowInput_LanguageFallback(t *testing.T) {
	t.Run("infer language from rule name", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "java-demo.sf",
			Content:  "",
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.JAVA, rule.Language)
	})

	t.Run("fallback to general", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "demo.sf",
			Content:  "",
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.General, rule.Language)
		require.Equal(t, "demo.sf", rule.RuleName)
	})

	t.Run("keep invalid content for later evaluation", func(t *testing.T) {
		rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
			RuleName: "java-invalid.sf",
			Content:  `invalid syntax here $$$`,
		})
		require.NoError(t, err)
		require.Equal(t, ssaconfig.JAVA, rule.Language)
		require.Equal(t, `invalid syntax here $$$`, rule.Content)
		require.Equal(t, "java-invalid.sf", rule.RuleName)
	})
}

func TestParseSyntaxFlowInput_ParseTagsFromRuleContent(t *testing.T) {
	rule, err := ParseSyntaxFlowInput(&ypb.SyntaxFlowRuleInput{
		RuleName: "java-demo.sf",
		Content: `desc(
	title: "demo"
	lang: java
	tags: "compliance|baseline"
)

println as $result

alert $result for {
	title: "demo"
	tag: "alert|security"
}`,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"compliance", "baseline"}, rule.GetTags())
	require.Contains(t, rule.AlertDesc, "result")
	require.ElementsMatch(t, []string{"alert", "security"}, schema.SplitSyntaxFlowRuleTags(rule.AlertDesc["result"].Tag))
}

func TestFilterSyntaxFlowRule_IncludeAndExcludeTags(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	ruleNames := []string{
		fmt.Sprintf("rule_%s", t.Name()+"_1"),
		fmt.Sprintf("rule_%s", t.Name()+"_2"),
		fmt.Sprintf("rule_%s", t.Name()+"_3"),
	}
	rules := []*schema.SyntaxFlowRule{
		{RuleName: ruleNames[0], Tag: "compliance|security"},
		{RuleName: ruleNames[1], Tag: "security"},
		{RuleName: ruleNames[2], Tag: "quality"},
	}
	for _, rule := range rules {
		require.NoError(t, db.Create(rule).Error)
	}
	t.Cleanup(func() {
		db.Where("rule_name IN (?)", ruleNames).Unscoped().Delete(&schema.SyntaxFlowRule{})
	})

	count, err := QuerySyntaxFlowRuleCount(db, &ypb.SyntaxFlowRuleFilter{Tag: []string{"security"}})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

	count, err = QuerySyntaxFlowRuleCount(db, &ypb.SyntaxFlowRuleFilter{ExcludeTags: []string{"compliance"}})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

	count, err = QuerySyntaxFlowRuleCount(db, &ypb.SyntaxFlowRuleFilter{
		Tag:         []string{"security"},
		ExcludeTags: []string{"compliance"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestQuerySyntaxFlowRuleGroup_RecountWithExcludeTags(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	groupName := fmt.Sprintf("group_%s", t.Name())
	ruleName1 := fmt.Sprintf("rule_%s_1", t.Name())
	ruleName2 := fmt.Sprintf("rule_%s_2", t.Name())

	group := &schema.SyntaxFlowGroup{GroupName: groupName}
	rule1 := &schema.SyntaxFlowRule{RuleName: ruleName1, Tag: "compliance"}
	rule2 := &schema.SyntaxFlowRule{RuleName: ruleName2, Tag: "security"}

	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(rule1).Error)
	require.NoError(t, db.Create(rule2).Error)
	require.NoError(t, db.Model(group).Association("Rules").Append(rule1, rule2).Error)

	t.Cleanup(func() {
		db.Model(group).Association("Rules").Clear()
		db.Where("group_name = ?", groupName).Unscoped().Delete(&schema.SyntaxFlowGroup{})
		db.Where("rule_name IN (?)", []string{ruleName1, ruleName2}).Unscoped().Delete(&schema.SyntaxFlowRule{})
	})

	paging, groups, err := QuerySyntaxFlowRuleGroup(db, &ypb.QuerySyntaxFlowRuleGroupRequest{
		Filter: &ypb.SyntaxFlowRuleGroupFilter{
			GroupNames:   []string{groupName},
			ExcludeTags:  []string{"compliance"},
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 10},
	})
	require.NoError(t, err)
	require.NotNil(t, paging)
	require.Len(t, groups, 1)
	require.EqualValues(t, 1, groups[0].Count)
}
