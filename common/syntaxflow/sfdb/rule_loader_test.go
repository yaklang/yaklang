package sfdb

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ===========================
// 基础功能测试
// ===========================

// TestDBRuleLoader 测试数据库规则加载器
func TestDBRuleLoader(t *testing.T) {
	loader := NewDBRuleLoader(nil)
	assert.NotNil(t, loader)
	assert.Equal(t, LoaderTypeDatabase, loader.GetLoaderType())

	ctx := context.Background()

	// 测试LoadRules
	rules, err := loader.LoadRules(ctx, nil)
	assert.NoError(t, err)
	assert.NotNil(t, rules)
	t.Logf("Loaded %d rules from database", len(rules))

	// 测试带筛选条件的LoadRules
	filter := &ypb.SyntaxFlowRuleFilter{
		Language: []string{"java"},
	}
	javaRules, err := loader.LoadRules(ctx, filter)
	assert.NoError(t, err)
	t.Logf("Loaded %d java rules from database", len(javaRules))

	// 如果有规则，测试LoadRuleByName
	if len(rules) > 0 {
		firstRuleName := rules[0].RuleName
		rule, err := loader.LoadRuleByName(ctx, firstRuleName)
		assert.NoError(t, err)
		assert.NotNil(t, rule)
		assert.Equal(t, firstRuleName, rule.RuleName)
		t.Logf("Loaded rule by name: %s", firstRuleName)
	}

	// 测试YieldRules
	count := 0
	for item := range loader.YieldRules(ctx, nil) {
		assert.NotNil(t, item)
		if item.Error != nil {
			t.Logf("Error yielding rule: %v", item.Error)
		} else {
			count++
		}
	}
	t.Logf("Yielded %d rules", count)

	// 测试Close
	err = loader.Close()
	assert.NoError(t, err)
}

// TestRuleLoaderType 测试加载器类型
func TestRuleLoaderType(t *testing.T) {
	// 测试String方法
	assert.Equal(t, "database", LoaderTypeDatabase.String())
	assert.Equal(t, "oss", LoaderTypeOSS.String())

	// 测试IsValid方法
	assert.True(t, LoaderTypeDatabase.IsValid())
	assert.True(t, LoaderTypeOSS.IsValid())
	assert.False(t, RuleLoaderType("invalid").IsValid())
}

// TestContextCancellation 测试上下文取消
func TestContextCancellation(t *testing.T) {
	loader := NewDBRuleLoader(nil)

	// 创建一个已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// LoadRules应该返回上下文错误
	_, err := loader.LoadRules(ctx, nil)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// LoadRuleByName应该返回上下文错误
	_, err = loader.LoadRuleByName(ctx, "test")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestCreateRuleLoader 测试工厂方法
func TestCreateRuleLoader(t *testing.T) {
	// 测试默认加载器
	t.Run("Create Default Loader", func(t *testing.T) {
		loader := CreateDefaultRuleLoader(nil)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeDatabase, loader.GetLoaderType())
	})
}

// ===========================
// 集成测试
// ===========================

// TestRuleLoaderIntegration 集成测试：测试规则加载器与实际规则的配合
func TestRuleLoaderIntegration(t *testing.T) {
	ctx := context.Background()
	db := consts.GetGormProfileDatabase()

	// 准备测试数据：创建几个测试规则
	testRules := []*schema.SyntaxFlowRule{
		{
			RuleName:    "test_integration_java_" + uuid.NewString()[:8],
			Language:    "java",
			Purpose:     "audit",
			Severity:    "high",
			Content:     "desc(title: 'Test Java')\nprintln as $output",
			Title:       "Java测试",
			Description: "测试规则",
		},
		{
			RuleName:    "test_integration_php_" + uuid.NewString()[:8],
			Language:    "php",
			Purpose:     "audit",
			Severity:    "medium",
			Content:     "desc(title: 'Test PHP')\nprintln as $output",
			Title:       "PHP测试",
			Description: "测试规则",
		},
	}

	// 保存测试规则到数据库
	for _, rule := range testRules {
		err := db.Save(rule).Error
		require.NoError(t, err)
	}

	// 清理函数
	defer func() {
		for _, rule := range testRules {
			db.Unscoped().Where("rule_name = ?", rule.RuleName).Delete(&schema.SyntaxFlowRule{})
		}
	}()

	// 测试1：使用数据库加载器加载测试规则
	t.Run("Load from Database", func(t *testing.T) {
		loader := NewDBRuleLoader(db)

		// 加载所有测试规则
		filter := &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{
				testRules[0].RuleName,
				testRules[1].RuleName,
			},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(rules))
		t.Logf("Loaded %d rules from database", len(rules))
	})

	// 测试2：按语言筛选
	t.Run("Filter by Language", func(t *testing.T) {
		loader := NewDBRuleLoader(db)

		filter := &ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
			RuleNames: []string{
				testRules[0].RuleName,
				testRules[1].RuleName,
			},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules)) // 只有1个java规则

		for _, rule := range rules {
			assert.Equal(t, "java", rule.Language)
		}
	})

	// 测试3：使用工厂方法创建
	t.Run("Create via Factory", func(t *testing.T) {
		loader := CreateDefaultRuleLoader(db)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeDatabase, loader.GetLoaderType())

		filter := &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{testRules[0].RuleName},
		}
		rules, err := loader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
	})

	// 测试4：流式加载
	t.Run("Yield Rules", func(t *testing.T) {
		loader := NewDBRuleLoader(db)

		filter := &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{
				testRules[0].RuleName,
				testRules[1].RuleName,
			},
		}

		count := 0
		for item := range loader.YieldRules(ctx, filter) {
			assert.NotNil(t, item)
			if item.Error == nil {
				count++
				assert.NotNil(t, item.Rule)
				t.Logf("Yielded: %s", item.Rule.RuleName)
			}
		}
		assert.Equal(t, 2, count)
	})
}
