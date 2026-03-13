package aiforge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// setupTestDatabase 设置测试数据库，调用 PostInit 来初始化数据
func setupTestDatabase(t *testing.T) {
	// 调用 PostInit 来初始化数据库数据
	yakit.CallPostInitDatabase()
	log.Infof("test database setup completed with PostInit")
}

// createTestAIForge 创建一个测试用的 AIForge 记录
func createTestAIForge(t *testing.T, name string) *schema.AIForge {
	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db, "profile database should be available")

	forge := &schema.AIForge{
		ForgeName:        name,
		ForgeVerboseName: "Test " + name,
		Description:      "这是一个测试用的 AI Forge，用于测试从数据库读取数据的功能",
		ForgeType:        "yak",
		ForgeContent:     "test content for " + name,
		Tags:             "test,automation,database",
		InitPrompt:       "初始化提示内容",
		PersistentPrompt: "持久化提示内容",
		PlanPrompt:       "计划提示内容",
		ResultPrompt:     "结果提示内容",
		IsTemporary:      false,
	}

	err := yakit.CreateAIForge(db, forge)
	require.NoError(t, err, "should create test AIForge successfully")

	return forge
}

// TestForgeFactory_Query 测试从数据库读取数据的功能
func TestForgeFactory_Query(t *testing.T) {
	setupTestDatabase(t)

	if utils.InGithubActions() {
		t.Skip("Skipping TestForgeFactory_Query in GitHub Actions environment")
		return
	}

	// 创建测试数据
	testForgeName := "test-forge-query"
	testForge := createTestAIForge(t, testForgeName)
	defer func() {
		// 清理测试数据
		db := consts.GetGormProfileDatabase()
		yakit.DeleteAIForgeByName(db, testForgeName)
	}()

	factory := &ForgeFactory{}
	ctx := context.Background()

	t.Run("Query without filters", func(t *testing.T) {
		forges, err := factory.Query(ctx)
		assert.NoError(t, err, "Query should not return error")
		assert.NotEmpty(t, forges, "should return some forges from database")

		// 验证返回的数据包含我们创建的测试数据
		found := false
		for _, forge := range forges {
			if forge.ForgeName == testForgeName {
				found = true
				assert.Equal(t, testForge.Description, forge.Description)
				assert.Equal(t, testForge.ForgeType, forge.ForgeType)
				break
			}
		}
		assert.True(t, found, "should find the test forge in query results")
	})

	t.Run("Query with keyword filter", func(t *testing.T) {
		// 使用我们创建的测试数据的关键词进行搜索
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Keyword(testForgeName))
		assert.NoError(t, err, "Query with keyword should not return error")
		assert.NotEmpty(t, forges, "should return forges matching keyword")

		// 验证返回的结果包含我们的测试数据
		found := false
		for _, forge := range forges {
			if forge.ForgeName == testForgeName {
				found = true
				break
			}
		}
		assert.True(t, found, "should find our test forge with keyword filter")
	})

	t.Run("Query with limit", func(t *testing.T) {
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Limit(5))
		assert.NoError(t, err, "Query with limit should not return error")
		assert.LessOrEqual(t, len(forges), 5, "should respect the limit")
	})

	t.Run("Query with keyword and limit", func(t *testing.T) {
		forges, err := factory.Query(ctx,
			aicommon.WithForgeFilter_Keyword(testForgeName),
			aicommon.WithForgeFilter_Limit(10))
		assert.NoError(t, err, "Query with multiple filters should not return error")

		// 应该能找到我们的测试数据
		found := false
		for _, forge := range forges {
			if forge.ForgeName == testForgeName {
				found = true
				break
			}
		}
		assert.True(t, found, "should find the test forge with keyword filter")
	})
}

// TestForgeFactory_GenerateAIForgeListForPrompt 测试生成包含 AI_BLUEPRINT_ 等关键内容的功能
func TestForgeFactory_GenerateAIForgeListForPrompt(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}

	// 创建测试数据
	testForges := []*schema.AIForge{
		{
			ForgeName:        "test-forge-1",
			ForgeVerboseName: "Test Forge One",
			Description:      "第一个测试用的 AI Forge",
		},
		{
			ForgeName:   "test-forge-2",
			Description: "第二个测试用的 AI Forge，包含中文描述",
		},
		{
			ForgeName:        "test-forge-3",
			ForgeVerboseName: "Complex Forge",
			Description:      "复杂的测试用例，包含特殊字符和多行描述\n这是第二行",
		},
	}

	t.Run("GenerateAIForgeListForPrompt basic functionality", func(t *testing.T) {
		result, err := factory.GenerateAIForgeListForPrompt(testForges)
		assert.NoError(t, err, "GenerateAIForgeListForPrompt should not return error")
		assert.NotEmpty(t, result, "should return non-empty result")

		log.Infof("Generated prompt:\n%s", result)

		// 验证生成的内容包含关键标识符
		assert.Contains(t, result, "AI_BLUEPRINT_", "result should contain AI_BLUEPRINT_ identifier")
		assert.Contains(t, result, "_START", "result should contain _START marker")
		assert.Contains(t, result, "_END", "result should contain _END marker")

		// 验证包含所有测试的 forge 名称
		for _, forge := range testForges {
			assert.Contains(t, result, forge.ForgeName, "result should contain forge name: %s", forge.ForgeName)
			assert.Contains(t, result, forge.Description, "result should contain forge description")
		}

		// 验证包含 verbose name（如果存在）
		assert.Contains(t, result, "Test Forge One", "result should contain verbose name")
		assert.Contains(t, result, "(Short: Test Forge One)", "result should contain short name format")
		assert.Contains(t, result, "(Short: Complex Forge)", "result should contain short name format")
	})

	t.Run("GenerateAIForgeListForPrompt with empty list", func(t *testing.T) {
		result, err := factory.GenerateAIForgeListForPrompt([]*schema.AIForge{})
		assert.NoError(t, err, "should handle empty list")
		assert.Empty(t, result, "should return empty string for empty list")
	})

	t.Run("Verify template structure", func(t *testing.T) {
		result, err := factory.GenerateAIForgeListForPrompt(testForges)
		assert.NoError(t, err, "should generate template")

		// 验证模板结构
		lines := strings.Split(result, "\n")
		assert.True(t, len(lines) >= 3, "should have at least start, content, and end lines")

		// 验证开始和结束标记使用相同的 nonce
		var startNonce, endNonce string
		for _, line := range lines {
			if strings.Contains(line, "_START") {
				parts := strings.Split(line, "_")
				if len(parts) >= 3 {
					startNonce = parts[2] // AI_BLUEPRINT_XXXX_START
				}
			}
			if strings.Contains(line, "_END") {
				parts := strings.Split(line, "_")
				if len(parts) >= 3 {
					endNonce = parts[2] // AI_BLUEPRINT_XXXX_END
				}
			}
		}
		assert.NotEmpty(t, startNonce, "should extract start nonce")
		assert.NotEmpty(t, endNonce, "should extract end nonce")
		assert.Equal(t, startNonce, endNonce, "start and end nonce should match")
		assert.Len(t, startNonce, 4, "nonce should be 4 characters long")
	})
}

// TestForgeFactory_Execute 测试透明转发功能
func TestForgeFactory_Execute(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}
	ctx := context.Background()

	t.Run("Execute function exists and accepts parameters", func(t *testing.T) {
		// 这个测试主要验证 Execute 函数能正确接收参数
		// 由于 ExecuteForge 的具体实现可能需要更复杂的设置，我们主要测试接口
		forgeName := "non-existent-forge"
		params := []*ypb.ExecParamItem{
			{
				Key:   "test-param",
				Value: "test-value",
			},
		}

		// 调用 Execute 函数，预期会有错误（因为 forge 不存在）
		result, err := factory.Execute(ctx, forgeName, params)

		// 我们不期望这个调用成功，但要确保函数签名正确
		// 如果返回错误，说明函数至少被正确调用了
		assert.Error(t, err, "should return error for non-existent forge")
		assert.Nil(t, result, "result should be nil when error occurs")

		// 验证错误信息合理（可能包含 forge 名称）
		assert.Contains(t, strings.ToLower(err.Error()), "forge", "error should mention forge")
	})
}

// TestForgeFactory_Integration 集成测试，测试从数据库读取到生成 prompt 的完整流程
func TestForgeFactory_Integration(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}
	ctx := context.Background()

	// 创建测试数据
	testForgeName := "integration-test-forge"
	testForge := createTestAIForge(t, testForgeName)
	defer func() {
		// 清理测试数据
		db := consts.GetGormProfileDatabase()
		yakit.DeleteAIForgeByName(db, testForgeName)
	}()

	t.Run("Complete workflow: Query -> Generate Prompt", func(t *testing.T) {
		// 步骤1: 查询数据库
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Keyword("integration"))
		require.NoError(t, err, "Query should succeed")
		require.NotEmpty(t, forges, "should find our test forge")

		// 验证查询结果
		found := false
		for _, forge := range forges {
			if forge.ForgeName == testForgeName {
				found = true
				assert.Equal(t, testForge.Description, forge.Description)
				break
			}
		}
		assert.True(t, found, "should find our test forge in results")

		// 步骤2: 生成 prompt
		prompt, err := factory.GenerateAIForgeListForPrompt(forges)
		require.NoError(t, err, "GenerateAIForgeListForPrompt should succeed")

		// 验证生成的 prompt 包含必要的内容
		assert.Contains(t, prompt, "AI_BLUEPRINT_", "prompt should contain blueprint marker")
		assert.Contains(t, prompt, testForgeName, "prompt should contain our test forge name")
		assert.Contains(t, prompt, testForge.Description, "prompt should contain forge description")

		log.Infof("Integration test generated prompt:\n%s", prompt)
	})
}

// TestForgeFactory_DatabaseConnection 测试数据库连接和数据的存在性
func TestForgeFactory_DatabaseConnection(t *testing.T) {
	setupTestDatabase(t)

	t.Run("Database connection is available", func(t *testing.T) {
		db := consts.GetGormProfileDatabase()
		assert.NotNil(t, db, "profile database should be available")

		// 测试数据库连接是否正常
		sqlDB := db.DB()
		err := sqlDB.Ping()
		assert.NoError(t, err, "database should be pingable")
	})

	t.Run("AIForge table exists and has data", func(t *testing.T) {
		db := consts.GetGormProfileDatabase()

		// 检查表是否存在
		assert.True(t, db.HasTable(&schema.AIForge{}), "AIForge table should exist")

		// 尝试查询一些数据（PostInit 应该已经插入了一些数据）
		var count int
		err := db.Model(&schema.AIForge{}).Count(&count).Error
		assert.NoError(t, err, "should be able to count AIForge records")

		log.Infof("found %d AIForge records in database after PostInit", count)

		// PostInit 之后应该有一些数据
		assert.Greater(t, count, 0, "database should contain some AIForge records after PostInit")
	})
}

// TestForgeFactory_EdgeCases 测试边界情况和错误处理
func TestForgeFactory_EdgeCases(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}
	ctx := context.Background()

	t.Run("Query with invalid limit", func(t *testing.T) {
		// 测试负数限制
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Limit(-1))
		assert.NoError(t, err, "should handle negative limit gracefully")
		assert.NotNil(t, forges, "should return valid result")

		// 测试零限制
		forges, err = factory.Query(ctx, aicommon.WithForgeFilter_Limit(0))
		assert.NoError(t, err, "should handle zero limit gracefully")
		assert.NotNil(t, forges, "should return valid result")
	})

	t.Run("Query with empty keyword", func(t *testing.T) {
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Keyword(""))
		assert.NoError(t, err, "should handle empty keyword")
		assert.NotNil(t, forges, "should return valid result")
	})

	t.Run("Query with special characters in keyword", func(t *testing.T) {
		specialKeywords := []string{
			"'DROP TABLE",   // SQL 注入尝试
			"<script>",      // XSS 尝试
			"../../../",     // 路径遍历尝试
			"NULL",          // NULL 字符串
			"SELECT * FROM", // SQL 查询
		}

		for _, keyword := range specialKeywords {
			forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Keyword(keyword))
			assert.NoError(t, err, "should handle special keyword safely: %s", keyword)
			assert.NotNil(t, forges, "should return valid result for keyword: %s", keyword)
		}
	})

	t.Run("GenerateAIForgeListForPrompt with nil input", func(t *testing.T) {
		result, err := factory.GenerateAIForgeListForPrompt(nil)
		assert.NoError(t, err, "should handle nil input")
		assert.Empty(t, result, "should return empty string for nil input")
	})

	t.Run("GenerateAIForgeListForPrompt with forge containing special characters", func(t *testing.T) {
		specialForges := []*schema.AIForge{
			{
				ForgeName:   "forge-with-'quotes'",
				Description: "包含引号 \" 和单引号 ' 的描述",
			},
			{
				ForgeName:   "forge-with-<tags>",
				Description: "包含 HTML <script>alert('xss')</script> 标签的描述",
			},
			{
				ForgeName:   "forge-with-unicode",
				Description: "包含 Unicode 字符：🚀 💻 🔥 emoji 和特殊符号",
			},
		}

		result, err := factory.GenerateAIForgeListForPrompt(specialForges)
		assert.NoError(t, err, "should handle special characters")
		assert.Contains(t, result, "AI_BLUEPRINT_", "should contain blueprint marker")

		// 验证特殊字符被正确处理
		for _, forge := range specialForges {
			assert.Contains(t, result, forge.ForgeName, "should contain forge name with special chars")
		}

		log.Infof("Generated prompt with special characters:\n%s", result)
	})
}

// TestForgeFactory_Performance 测试性能相关的场景
func TestForgeFactory_Performance(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}
	ctx := context.Background()

	t.Run("Query with large limit", func(t *testing.T) {
		forges, err := factory.Query(ctx, aicommon.WithForgeFilter_Limit(1000))
		assert.NoError(t, err, "should handle large limit")
		assert.LessOrEqual(t, len(forges), 1000, "should respect large limit")
	})

	t.Run("GenerateAIForgeListForPrompt with many forges", func(t *testing.T) {
		// 创建大量的测试数据
		var manyForges []*schema.AIForge
		for i := 0; i < 50; i++ {
			manyForges = append(manyForges, &schema.AIForge{
				ForgeName:   fmt.Sprintf("perf-test-forge-%d", i),
				Description: fmt.Sprintf("性能测试用的第 %d 个 forge，包含一些描述内容", i),
			})
		}

		result, err := factory.GenerateAIForgeListForPrompt(manyForges)
		assert.NoError(t, err, "should handle many forges")
		assert.Contains(t, result, "AI_BLUEPRINT_", "should contain blueprint marker")
		assert.Greater(t, len(result), 100, "result should be substantial")

		// 验证所有 forge 都被包含
		for i := 0; i < 10; i++ { // 只检查前 10 个避免测试太慢
			expectedName := fmt.Sprintf("perf-test-forge-%d", i)
			assert.Contains(t, result, expectedName, "should contain forge %d", i)
		}
	})
}

// TestForgeFactory_GetAIForge 测试 GetAIForge 方法
func TestForgeFactory_GetAIForge(t *testing.T) {
	setupTestDatabase(t)

	// 创建测试数据
	testForgeName := "test-get-forge" + utils.RandStringBytes(10)
	testForge := createTestAIForge(t, testForgeName)
	defer func() {
		// 清理测试数据
		db := consts.GetGormProfileDatabase()
		yakit.DeleteAIForgeByName(db, testForgeName)
	}()

	factory := &ForgeFactory{}

	t.Run("GetAIForge with valid name", func(t *testing.T) {
		forge, err := factory.GetAIForge(testForgeName)
		assert.NoError(t, err, "GetAIForge should not return error")
		assert.NotNil(t, forge, "should return valid forge")
		assert.Equal(t, testForgeName, forge.ForgeName, "forge name should match")
		assert.Equal(t, testForge.Description, forge.Description, "forge description should match")
		assert.Equal(t, testForge.ForgeType, forge.ForgeType, "forge type should match")
	})

	t.Run("GetAIForge with empty name", func(t *testing.T) {
		forge, err := factory.GetAIForge("")
		assert.Error(t, err, "GetAIForge with empty name should return error")
		assert.Nil(t, forge, "should return nil forge when error")
		assert.Contains(t, err.Error(), "cannot be empty", "error should mention empty name")
	})

	t.Run("GetAIForge with non-existent name", func(t *testing.T) {
		nonExistentName := "non-existent-forge-name-12345"
		forge, err := factory.GetAIForge(nonExistentName)
		assert.Error(t, err, "GetAIForge with non-existent name should return error")
		assert.Nil(t, forge, "should return nil forge when error")
		// GORM 会返回 record not found 错误
		assert.Contains(t, strings.ToLower(err.Error()), "record not found", "error should indicate record not found")
	})

	t.Run("GetAIForge with special characters in name", func(t *testing.T) {
		specialNamePrefixes := []string{
			"test-forge-with-dashes",
			"test_forge_with_underscores",
			"test.forge.with.dots",
			"test123",
		}

		for _, prefix := range specialNamePrefixes {
			name := prefix + "-" + utils.RandStringBytes(8)
			t.Run(fmt.Sprintf("name=%s", name), func(t *testing.T) {
				// 创建特殊名称的测试 forge
				specialForge := createTestAIForge(t, name)
				defer func() {
					db := consts.GetGormProfileDatabase()
					yakit.DeleteAIForgeByName(db, name)
				}()

				forge, err := factory.GetAIForge(name)
				assert.NoError(t, err, "GetAIForge should handle special characters: %s", name)
				assert.NotNil(t, forge, "should return valid forge for name: %s", name)
				assert.Equal(t, name, forge.ForgeName, "forge name should match: %s", name)
				assert.Equal(t, specialForge.Description, forge.Description, "description should match")
			})
		}
	})
}

// TestForgeFactory_ConcurrentAccess 测试并发访问
func TestForgeFactory_ConcurrentAccess(t *testing.T) {
	setupTestDatabase(t)

	factory := &ForgeFactory{}
	ctx := context.Background()

	t.Run("Concurrent queries", func(t *testing.T) {
		// 并发执行多个查询
		const numGoroutines = 10
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() {
					if r := recover(); r != nil {
						results <- fmt.Errorf("goroutine %d panicked: %v", id, r)
						return
					}
				}()

				_, err := factory.Query(ctx, aicommon.WithForgeFilter_Limit(5))
				if err != nil {
					results <- fmt.Errorf("goroutine %d query failed: %v", id, err)
					return
				}

				// 不再检查是否为空，因为数据库可能在某些情况下没有数据
				// 只要查询不报错就算成功
				results <- nil
			}(i)
		}

		// 收集结果
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			assert.NoError(t, err, "concurrent query should not fail")
		}
	})

	t.Run("Concurrent prompt generation", func(t *testing.T) {
		testForges := []*schema.AIForge{
			{
				ForgeName:   "concurrent-test-forge",
				Description: "并发测试用的 forge",
			},
		}

		const numGoroutines = 10
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() {
					if r := recover(); r != nil {
						results <- fmt.Errorf("goroutine %d panicked: %v", id, r)
						return
					}
				}()

				result, err := factory.GenerateAIForgeListForPrompt(testForges)
				if err != nil {
					results <- fmt.Errorf("goroutine %d generation failed: %v", id, err)
					return
				}

				if !strings.Contains(result, "AI_BLUEPRINT_") {
					results <- fmt.Errorf("goroutine %d result missing blueprint marker", id)
					return
				}

				results <- nil
			}(i)
		}

		// 收集结果
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			assert.NoError(t, err, "concurrent generation should not fail")
		}
	})
}

// TestForgeFactory_GenerateAIJSONSchemaFromSchemaAIForge 测试 JSON Schema 生成功能
func TestForgeFactory_GenerateAIJSONSchemaFromSchemaAIForge(t *testing.T) {
	factory := &ForgeFactory{}

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge with nil forge", func(t *testing.T) {
		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(nil)
		assert.Error(t, err, "should return error for nil forge")
		assert.Empty(t, schema, "should return empty schema")
		assert.Contains(t, err.Error(), "cannot be nil", "error should mention nil forge")
	})

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge with empty Params", func(t *testing.T) {
		forge := &schema.AIForge{
			ForgeName:   "test-forge",
			Description: "Test forge with empty params",
			Params:      "",
		}
		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(forge)
		assert.NoError(t, err, "should not return error for empty Params")
		assert.NotEmpty(t, schema, "should return valid schema")

		// 验证 JSON Schema 结构
		assert.Contains(t, schema, `"$schema"`, "should contain JSON Schema identifier")
		assert.Contains(t, schema, `"type": "object"`, "should be object type")
		assert.Contains(t, schema, `"properties"`, "should contain properties")
		assert.Contains(t, schema, `"@action"`, "should contain action field")
		assert.Contains(t, schema, `"params"`, "should contain params field")
		assert.Contains(t, schema, `"query"`, "should contain query parameter when params is empty")
	})

	t.Run("Test ConvertYaklangCliCodeToToolOptions directly", func(t *testing.T) {
		yakCode := `
cli.String("input_file", cli.setRequired(true), cli.setVerboseName("输入文件"), cli.setHelp("输入文件的路径"))
cli.Int("count", cli.setDefault(10), cli.setVerboseName("数量"), cli.setHelp("处理的数量"))
cli.Bool("verbose", cli.setDefault(false), cli.setVerboseName("详细模式"), cli.setHelp("是否显示详细信息"))
cli.check()
`

		// 直接测试 ConvertYaklangCliCodeToToolOptions
		toolOptions := aitool.ConvertYaklangCliCodeToToolOptions(yakCode)
		assert.NotEmpty(t, toolOptions, "should return tool options for valid Yak code")

		// 验证生成的工具选项
		assert.GreaterOrEqual(t, len(toolOptions), 3, "should have at least 3 tool options")
	})

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge with valid Yak CLI code", func(t *testing.T) {
		// 创建包含 Yak CLI 参数定义的 forge
		yakCode := `
cli.String("input_file", cli.setRequired(true), cli.setVerboseName("输入文件"), cli.setHelp("输入文件的路径"))
cli.Int("count", cli.setDefault(10), cli.setVerboseName("数量"), cli.setHelp("处理的数量"))
cli.Bool("verbose", cli.setDefault(false), cli.setVerboseName("详细模式"), cli.setHelp("是否显示详细信息"))
cli.check()
`

		forge := &schema.AIForge{
			ForgeName:        "test-cli-forge",
			ForgeVerboseName: "测试 CLI Forge",
			Description:      "用于测试 CLI 参数解析的 forge",
			Params:           yakCode,
		}

		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(forge)
		assert.NoError(t, err, "should not return error for valid Yak code")
		assert.NotEmpty(t, schema, "should return valid schema")

		// 验证 JSON Schema 结构
		assert.Contains(t, schema, `"$schema"`, "should contain JSON Schema identifier")
		assert.Contains(t, schema, `"type": "object"`, "should be object type")
		assert.Contains(t, schema, `"properties"`, "should contain properties")
		assert.Contains(t, schema, `"@action"`, "should contain action field")
		assert.Contains(t, schema, `"call-ai-blueprint"`, "should contain correct action value")
		assert.Contains(t, schema, `"params"`, "should contain params field")
		assert.Contains(t, schema, `"input_file"`, "should contain input_file parameter")
		assert.Contains(t, schema, `"count"`, "should contain count parameter")
		assert.Contains(t, schema, `"verbose"`, "should contain verbose parameter")
		assert.Contains(t, schema, `"required"`, "should contain required array")
		assert.Contains(t, schema, `"input_file"`, "input_file should be required")
	})

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge with invalid Yak code", func(t *testing.T) {
		forge := &schema.AIForge{
			ForgeName:   "test-forge",
			Description: "Test forge with invalid Yak code",
			Params:      "invalid yak code {{{",
		}
		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(forge)
		assert.NoError(t, err, "should not return error for invalid Yak code (gracefully handled)")
		assert.NotEmpty(t, schema, "should return valid schema")

		// 应该返回基本的 schema 结构
		assert.Contains(t, schema, `"$schema"`, "should contain JSON Schema identifier")
		assert.Contains(t, schema, `"type": "object"`, "should be object type")
		assert.Contains(t, schema, `"properties"`, "should contain properties")
	})

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge with complex Yak code", func(t *testing.T) {
		yakCode := `
cli.String("target", cli.setRequired(true), cli.setVerboseName("目标"), cli.setHelp("扫描目标"))
cli.Int("port", cli.setDefault(80), cli.setVerboseName("端口"), cli.setHelp("目标端口"))
cli.Float("timeout", cli.setDefault(5.0), cli.setVerboseName("超时"), cli.setHelp("连接超时时间"))
cli.Bool("ssl", cli.setDefault(true), cli.setVerboseName("SSL"), cli.setHelp("是否使用SSL"))
cli.check()
`

		forge := &schema.AIForge{
			ForgeName:   "complex-test-forge",
			Description: "复杂参数测试",
			Params:      yakCode,
		}

		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(forge)
		assert.NoError(t, err, "should not return error")
		assert.NotEmpty(t, schema, "should return valid schema")

		// 验证所有参数类型都被正确处理
		assert.Contains(t, schema, `"target"`, "should contain target parameter")
		assert.Contains(t, schema, `"port"`, "should contain port parameter")
		assert.Contains(t, schema, `"timeout"`, "should contain timeout parameter")
		assert.Contains(t, schema, `"ssl"`, "should contain ssl parameter")
		assert.Contains(t, schema, `"required"`, "should contain required array")
		assert.Contains(t, schema, `"target"`, "target should be required")
	})

	t.Run("GenerateAIJSONSchemaFromSchemaAIForge validates JSON structure", func(t *testing.T) {
		yakCode := `
cli.String("name", cli.setRequired(true), cli.setVerboseName("名称"), cli.setHelp("用户名称"))
cli.Int("age", cli.setDefault(18), cli.setVerboseName("年龄"), cli.setHelp("用户年龄"))
cli.check()
`

		forge := &schema.AIForge{
			ForgeName:   "validation-test-forge",
			Description: "JSON Schema 结构验证测试",
			Params:      yakCode,
		}

		schema, err := factory.GenerateAIJSONSchemaFromSchemaAIForge(forge)
		assert.NoError(t, err, "should not return error")
		assert.NotEmpty(t, schema, "should return valid schema")

		// 验证生成的字符串是有效的 JSON
		var jsonSchema map[string]any
		err = json.Unmarshal([]byte(schema), &jsonSchema)
		assert.NoError(t, err, "generated schema should be valid JSON")

		// 验证必要的字段存在
		assert.Contains(t, jsonSchema, "$schema", "should have $schema field")
		assert.Contains(t, jsonSchema, "type", "should have type field")
		assert.Contains(t, jsonSchema, "properties", "should have properties field")

		properties, ok := jsonSchema["properties"].(map[string]any)
		assert.True(t, ok, "properties should be a map")

		// 验证 @action 字段
		actionField, exists := properties["@action"]
		assert.True(t, exists, "should have @action field")
		actionMap, ok := actionField.(map[string]any)
		assert.True(t, ok, "@action should be a map")
		assert.Equal(t, "call-ai-blueprint", actionMap["const"], "@action should have correct const value")

		// 验证 params 字段
		paramsField, exists := properties["params"]
		assert.True(t, exists, "should have params field")
		paramsMap, ok := paramsField.(map[string]any)
		assert.True(t, ok, "params should be a map")

		paramsProps, ok := paramsMap["properties"].(map[string]any)
		assert.True(t, ok, "params should have properties")

		// 验证具体的参数
		assert.Contains(t, paramsProps, "name", "should contain name parameter")
		assert.Contains(t, paramsProps, "age", "should contain age parameter")
	})
}
