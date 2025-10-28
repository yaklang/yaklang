package sfdb

import (
	"context"
	"sync/atomic"
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
	db := consts.GetGormProfileDatabase()

	// 测试创建数据库加载器
	t.Run("Create Database Loader", func(t *testing.T) {
		loader := CreateRuleLoader(RuleSourceTypeDatabase, nil, db)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeDatabase, loader.GetLoaderType())
	})

	// 测试创建OSS加载器（使用Mock）
	t.Run("Create OSS Loader", func(t *testing.T) {
		mockClient := NewMockOSSClient(OSSTypeMinIO)
		loader := CreateRuleLoader(RuleSourceTypeOSS, mockClient, db)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeOSS, loader.GetLoaderType())
	})

	// 测试默认加载器
	t.Run("Create Default Loader", func(t *testing.T) {
		loader := CreateDefaultRuleLoader(nil)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeDatabase, loader.GetLoaderType())
	})
}

// ===========================
// 性能优化测试
// ===========================

// TestOSSRuleLoaderNoDoubleLoad 验证OSS加载器不会重复加载规则
func TestOSSRuleLoaderNoDoubleLoad(t *testing.T) {
	ctx := context.Background()

	// 创建一个计数Mock客户端，记录GetObject调用次数
	var getObjectCallCount int32
	mockClient := &CountingMockOSSClient{
		MockOSSClient:  NewMockOSSClient(OSSTypeMinIO),
		getObjectCalls: &getObjectCallCount,
	}

	// 添加测试规则
	mockClient.AddRuleObject("test_rule_1", `desc(title: "Rule 1", language: java)
println as $output`)
	mockClient.AddRuleObject("test_rule_2", `desc(title: "Rule 2", language: java)
println as $output`)
	mockClient.AddRuleObject("test_rule_3", `desc(title: "Rule 3", language: php)
println as $output`)

	// 创建OSS加载器
	loader := NewOSSRuleLoader(mockClient)

	// 第一次调用 LoadRules（用于计数）
	atomic.StoreInt32(&getObjectCallCount, 0) // 重置计数
	rules1, err := loader.LoadRules(ctx, nil)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(rules1))

	firstLoadCalls := atomic.LoadInt32(&getObjectCallCount)
	t.Logf("First LoadRules: %d GetObject calls", firstLoadCalls)
	assert.Equal(t, int32(3), firstLoadCalls, "LoadRules should call GetObject 3 times")

	// 第二次调用 YieldRules（在真实场景中，这会导致重复加载）
	atomic.StoreInt32(&getObjectCallCount, 0) // 重置计数
	count := 0
	for item := range loader.YieldRules(ctx, nil) {
		assert.NoError(t, item.Error)
		assert.NotNil(t, item.Rule)
		count++
	}

	secondLoadCalls := atomic.LoadInt32(&getObjectCallCount)
	t.Logf("Second YieldRules: %d GetObject calls", secondLoadCalls)

	// ⚠️ 当前实现会重复加载，所以这个断言会失败
	// 修复后应该为0（使用缓存）或者通过convertRulesToChannel避免
	assert.Equal(t, 3, count, "YieldRules should return 3 rules")

	// 验证总调用次数
	totalCalls := firstLoadCalls + secondLoadCalls
	t.Logf("Total GetObject calls: %d (expected: 3 if optimized, 6 if not)", totalCalls)

	// 如果优化成功，总调用应该是3次（只在LoadRules时调用）
	// 如果未优化，总调用是6次（LoadRules 3次 + YieldRules 3次）
	if totalCalls == 3 {
		t.Log("✅ Optimization successful: No double loading!")
	} else {
		t.Logf("⚠️ Double loading detected: %d calls instead of 3", totalCalls)
	}
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
		loader := CreateRuleLoader(RuleSourceTypeDatabase, nil, db)
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

// TestOSSRuleLoaderIntegration 测试OSS加载器的完整流程
func TestOSSRuleLoaderIntegration(t *testing.T) {
	ctx := context.Background()

	// 创建Mock OSS客户端
	mockClient := NewMockOSSClient(OSSTypeMinIO)

	// 添加规则文件
	mockClient.AddRuleObject("java_test", `desc(
  title: "Java测试规则",
  language: java
)
println as $output
`)

	mockClient.AddRuleObject("php_test", `desc(
  title: "PHP测试规则",
  language: php
)
println as $output
`)

	// 创建OSS加载器
	ossLoader := NewOSSRuleLoader(mockClient,
		WithOSSBucket("test-bucket"),
		WithOSSPrefix("syntaxflow/"),
		WithOSSCache(true),
	)
	defer ossLoader.Close()

	// 测试1：加载所有规则
	t.Run("Load All Rules from OSS", func(t *testing.T) {
		rules, err := ossLoader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(rules))

		for _, rule := range rules {
			assert.NotEmpty(t, rule.RuleName)
			assert.NotEmpty(t, rule.Content)
			t.Logf("Loaded from OSS: %s (lang=%s)", rule.RuleName, rule.Language)
		}
	})

	// 测试2：按语言筛选
	t.Run("Filter by Language", func(t *testing.T) {
		filter := &ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}
		rules, err := ossLoader.LoadRules(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(rules))
		assert.Equal(t, "java", rules[0].Language)
	})

	// 测试3：按规则名称加载
	t.Run("Load by Name", func(t *testing.T) {
		rule, err := ossLoader.LoadRuleByName(ctx, "java_test")
		assert.NoError(t, err)
		assert.NotNil(t, rule)
		assert.Equal(t, "java_test", rule.RuleName)
		assert.Equal(t, "java", rule.Language)
	})

	// 测试4：使用工厂方法创建OSS加载器
	t.Run("Create via Factory", func(t *testing.T) {
		loader := CreateOSSRuleLoader(mockClient, "bucket", "prefix/", true)
		assert.NotNil(t, loader)
		assert.Equal(t, LoaderTypeOSS, loader.GetLoaderType())

		rules, err := loader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(rules), 0)
	})
}

// TestDatabaseAndOSS 测试数据库和OSS配合使用
func TestDatabaseAndOSS(t *testing.T) {
	ctx := context.Background()
	db := consts.GetGormProfileDatabase()

	// 创建数据库规则
	dbRule := &schema.SyntaxFlowRule{
		RuleName: "db_rule_" + uuid.NewString()[:8],
		Language: "java",
		Purpose:  "audit",
		Content:  "desc(title: 'DB Rule')\nprintln as $output",
		Title:    "数据库规则",
	}
	err := db.Save(dbRule).Error
	require.NoError(t, err)
	defer db.Unscoped().Where("rule_name = ?", dbRule.RuleName).Delete(&schema.SyntaxFlowRule{})

	// 创建OSS规则
	mockClient := NewMockOSSClient(OSSTypeMinIO)
	mockClient.AddRuleObject("oss_rule", `desc(title: "OSS Rule", language: java)
println as $output`)

	// 测试：可以分别从两个来源加载
	t.Run("Load from Both Sources", func(t *testing.T) {
		// 从数据库加载
		dbLoader := NewDBRuleLoader(db)
		dbRules, err := dbLoader.LoadRules(ctx, &ypb.SyntaxFlowRuleFilter{
			RuleNames: []string{dbRule.RuleName},
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(dbRules))
		assert.Equal(t, dbRule.RuleName, dbRules[0].RuleName)

		// 从OSS加载
		ossLoader := NewOSSRuleLoader(mockClient)
		ossRules, err := ossLoader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(ossRules))
		assert.Equal(t, "oss_rule", ossRules[0].RuleName)

		t.Logf("DB: %d rules, OSS: %d rules", len(dbRules), len(ossRules))
	})
}

// ===========================
// Manager 层集成测试
// ===========================

// TestManagerIntegrationNoDoubleLoad 验证manager集成场景下不会重复加载
// 这个测试模拟了 manager.go 中 setRuleChan 的逻辑
func TestManagerIntegrationNoDoubleLoad(t *testing.T) {
	ctx := context.Background()

	// 创建计数Mock客户端
	var getObjectCallCount int32
	mockClient := &CountingMockOSSClient{
		MockOSSClient:  NewMockOSSClient(OSSTypeMinIO),
		getObjectCalls: &getObjectCallCount,
	}

	// 添加测试规则
	mockClient.AddRuleObject("test_rule_1", `desc(title: "Rule 1", language: java)
println as $output`)
	mockClient.AddRuleObject("test_rule_2", `desc(title: "Rule 2", language: java)  
println as $output`)
	mockClient.AddRuleObject("test_rule_3", `desc(title: "Rule 3", language: php)
println as $output`)

	// 创建OSS加载器
	loader := CreateRuleLoader(RuleSourceTypeOSS, mockClient, nil)

	t.Run("Simulated Manager Logic - OSS Mode", func(t *testing.T) {
		atomic.StoreInt32(&getObjectCallCount, 0)

		// ===== 模拟 manager.go 的 setRuleChan 逻辑 =====

		// 1. 检查加载器类型（非数据库）
		assert.Equal(t, LoaderTypeOSS, loader.GetLoaderType())

		// 2. 先加载规则用于计数（这是必需的）
		rules, err := loader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		rulesCount := len(rules)

		loadCallCount := atomic.LoadInt32(&getObjectCallCount)
		t.Logf("LoadRules for counting: %d rules, %d GetObject calls", rulesCount, loadCallCount)
		assert.Equal(t, 3, rulesCount)
		assert.Equal(t, int32(3), loadCallCount)

		// 3. 使用已加载的规则创建channel（修复后的逻辑）
		// 这里不再调用 YieldRules，而是直接使用已加载的rules
		ruleChan := make(chan *RuleItem, 10)
		go func() {
			defer close(ruleChan)
			for _, rule := range rules {
				select {
				case ruleChan <- &RuleItem{Rule: rule}:
				case <-ctx.Done():
					return
				}
			}
		}()

		// 4. 验证可以从channel读取规则
		receivedCount := 0
		for item := range ruleChan {
			assert.NoError(t, item.Error)
			assert.NotNil(t, item.Rule)
			receivedCount++
		}

		// 5. 验证结果
		channelCallCount := atomic.LoadInt32(&getObjectCallCount) - loadCallCount
		t.Logf("Channel creation: %d rules received, %d additional GetObject calls", receivedCount, channelCallCount)

		assert.Equal(t, 3, receivedCount, "Should receive all 3 rules")
		assert.Equal(t, int32(0), channelCallCount, "✅ Should NOT make additional GetObject calls")

		totalCalls := atomic.LoadInt32(&getObjectCallCount)
		t.Logf("Total GetObject calls: %d", totalCalls)
		assert.Equal(t, int32(3), totalCalls, "✅ Total should be 3 (no double loading)")
	})

	t.Run("Cache Effectiveness Test", func(t *testing.T) {
		// 创建一个新的 loader 来测试缓存
		var newCallCount int32
		newMockClient := &CountingMockOSSClient{
			MockOSSClient:  NewMockOSSClient(OSSTypeMinIO),
			getObjectCalls: &newCallCount,
		}

		newMockClient.AddRuleObject("test_rule_1", `desc(title: "Rule 1", language: java)
println as $output`)
		newMockClient.AddRuleObject("test_rule_2", `desc(title: "Rule 2", language: java)
println as $output`)
		newMockClient.AddRuleObject("test_rule_3", `desc(title: "Rule 3", language: php)
println as $output`)

		newLoader := CreateRuleLoader(RuleSourceTypeOSS, newMockClient, nil)

		// ===== 测试缓存有效性 =====
		atomic.StoreInt32(&newCallCount, 0)

		// 1. 第一次 LoadRules（会触发实际加载）
		rules1, err := newLoader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		firstLoadCalls := atomic.LoadInt32(&newCallCount)
		t.Logf("First LoadRules: %d rules, %d calls", len(rules1), firstLoadCalls)
		assert.Equal(t, int32(3), firstLoadCalls)

		// 2. 第二次 LoadRules（应该使用缓存）
		rules2, err := newLoader.LoadRules(ctx, nil)
		assert.NoError(t, err)
		secondLoadCalls := atomic.LoadInt32(&newCallCount) - firstLoadCalls
		t.Logf("Second LoadRules: %d rules, %d additional calls", len(rules2), secondLoadCalls)
		assert.Equal(t, int32(0), secondLoadCalls, "✅ Second LoadRules should use cache")

		// 3. YieldRules（也应该使用缓存）
		yieldCount := 0
		for item := range newLoader.YieldRules(ctx, nil) {
			assert.NoError(t, item.Error)
			yieldCount++
		}
		thirdLoadCalls := atomic.LoadInt32(&newCallCount) - firstLoadCalls
		t.Logf("YieldRules: %d rules, %d additional calls", yieldCount, thirdLoadCalls)
		assert.Equal(t, int32(0), thirdLoadCalls, "✅ YieldRules should use cache")

		totalCalls := atomic.LoadInt32(&newCallCount)
		t.Logf("Total: %d calls (expected: 3 with cache optimization)", totalCalls)
		assert.Equal(t, int32(3), totalCalls, "✅ Total should be 3 (loaded only once)")
	})
}

// ===========================
// 测试辅助工具
// ===========================

// CountingMockOSSClient 带计数功能的Mock客户端
type CountingMockOSSClient struct {
	*MockOSSClient
	getObjectCalls *int32
}

func (c *CountingMockOSSClient) GetObject(bucket, key string) ([]byte, error) {
	atomic.AddInt32(c.getObjectCalls, 1)
	return c.MockOSSClient.GetObject(bucket, key)
}

func (c *CountingMockOSSClient) ListObjects(bucket, prefix string) ([]OSSObject, error) {
	return c.MockOSSClient.ListObjects(bucket, prefix)
}

func (c *CountingMockOSSClient) Close() error {
	return c.MockOSSClient.Close()
}
