package yakit

import (
	"fmt"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newSyntaxFlowRuleTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}, &schema.SyntaxFlowGroup{}).Error)
	return db
}

func uniqueRuleName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, ksuid.New().String())
}

func insertSyntaxFlowRule(t *testing.T, db *gorm.DB, rule *schema.SyntaxFlowRule) {
	t.Helper()
	require.NoError(t, db.Create(rule).Error)
}

func requireSingleRuleNamed(t *testing.T, rules []*schema.SyntaxFlowRule, wantRuleName string) {
	t.Helper()
	require.Len(t, rules, 1)
	require.Equal(t, wantRuleName, rules[0].RuleName)
}

func querySyntaxFlowRules(t *testing.T, db *gorm.DB, filter *ypb.SyntaxFlowRuleFilter) []*schema.SyntaxFlowRule {
	t.Helper()
	_, rules, err := QuerySyntaxFlowRule(db, &ypb.QuerySyntaxFlowRuleRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 10},
		Filter:     filter,
	})
	require.NoError(t, err)
	return rules
}

func TestQuerySyntaxFlowRule_RuleNamesFuzzy(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	ruleName := uniqueRuleName("test-rule")
	// decoy: same title fragment but different rule_name — must not be returned
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: uniqueRuleName("decoy-xss"),
		TitleZh:  "XSS跨站脚本",
		Language: ssaconfig.JAVA,
		Content:  "println as $output",
	})
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: ruleName,
		TitleZh:  "SQL注入检测",
		Language: ssaconfig.JAVA,
		Content:  "println as $output",
	})

	requireSingleRuleNamed(t, querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{
		RuleNames: []string{ruleName},
	}), ruleName)

	requireSingleRuleNamed(t, querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{
		RuleNames: []string{"SQL注入"},
	}), ruleName)
}

func TestQuerySyntaxFlowRule_KeywordFullTextFuzzy(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	ruleName := uniqueRuleName("java-sqli")
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: uniqueRuleName("decoy-title-only"),
		TitleZh:  "标题不应被全文搜到",
		Language: ssaconfig.JAVA,
		Content:  "unrelated body",
	})
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: ruleName,
		TitleZh:  "标题不应被全文搜到",
		Language: ssaconfig.JAVA,
		Content:  "content mentions SQL语句拼接 here",
	})

	requireSingleRuleNamed(t, querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{
		Keyword: "SQL语句",
	}), ruleName)

	rules := querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{Keyword: "标题"})
	require.Empty(t, rules)
}

func TestQuerySyntaxFlowRule_RuleNamesChineseReflection(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	titleZh := "审计Java中Class.forName的不安全反射调用"
	ruleName := uniqueRuleName("java-reflection-audit")
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: uniqueRuleName("decoy-java-audit"),
		TitleZh:  "Java 基础代码审计",
		Language: ssaconfig.JAVA,
		Content:  "println as $output",
	})
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: ruleName,
		TitleZh:  titleZh,
		Language: ssaconfig.JAVA,
		Content:  `message: "检测到Java中不安全的反射调用"`,
	})

	for _, term := range []string{"反射", "不安全反射"} {
		requireSingleRuleNamed(t, querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{term},
		}), ruleName)
	}
}

func TestQuerySyntaxFlowRule_KeywordCommaOR(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	sqliName := uniqueRuleName("java-sql-injection-audit")
	xssName := uniqueRuleName("java-xss-audit")
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: sqliName,
		Language: ssaconfig.JAVA,
		Content:  "detect SQL注入 patterns",
	})
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: xssName,
		Language: ssaconfig.JAVA,
		Content:  "detect XSS跨站脚本",
	})
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: uniqueRuleName("decoy-unrelated"),
		Language: ssaconfig.JAVA,
		Content:  "nothing useful",
	})

	rules := querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{Keyword: "SQL注入,XSS"})
	require.Len(t, rules, 2)
	got := map[string]struct{}{}
	for _, r := range rules {
		got[r.RuleName] = struct{}{}
	}
	require.Contains(t, got, sqliName)
	require.Contains(t, got, xssName)
}

func TestQuerySyntaxFlowRule_WithGroupAndRuleNames(t *testing.T) {
	db := newSyntaxFlowRuleTestDB(t)
	groupName := uniqueRuleName("golang-audit-group")
	group := &schema.SyntaxFlowGroup{GroupName: groupName}
	require.NoError(t, db.Create(group).Error)

	ruleName := uniqueRuleName("go-http-src")
	insertSyntaxFlowRule(t, db, &schema.SyntaxFlowRule{
		RuleName: uniqueRuleName("decoy-http"),
		TitleZh:  "审计Golang HTTP输入点",
		Language: ssaconfig.GO,
		Content:  "println as $output",
	})
	rule := &schema.SyntaxFlowRule{
		RuleName: ruleName,
		TitleZh:  "审计Golang HTTP输入点",
		Language: ssaconfig.GO,
		Content:  "println as $output",
	}
	require.NoError(t, db.Create(rule).Error)
	require.NoError(t, db.Model(rule).Association("Groups").Append(group).Error)

	requireSingleRuleNamed(t, querySyntaxFlowRules(t, db, &ypb.SyntaxFlowRuleFilter{
		GroupNames: []string{groupName},
		RuleNames:  []string{"HTTP输入"},
	}), ruleName)
}
