package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/openai"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestGRPCMUSTPASS_GetAIToolList 测试获取AI工具列表
func TestGRPCMUSTPASS_GetAIToolList(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	//
	tmpName := uuid.NewString()
	c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
		Name:        tmpName,
		Description: uuid.NewString(),
		Content:     uuid.NewString(),
		ToolPath:    uuid.NewString(),
		Keywords:    []string{uuid.NewString()},
	})
	defer func() {
		c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{tmpName},
		})
	}()
	t.Run("GetAllTools", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   10,
				OrderBy: "updated_at",
				Order:   "desc",
			},
		})
		require.NoError(t, err)
		assert.True(t, len(resp.Tools) >= 1, "Should return at least 1 tools")
		assert.Equal(t, tmpName, resp.Tools[0].Name)
	})

	t.Run("NonExistentTool", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: "nonexistent-tool-" + uuid.NewString(),
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		// Should return empty results but not error
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 0, "Should return no tools")
	})
}

// TestGRPCMUSTPASS_WriteDB 测试写入数据库(增删改)
func TestGRPCMUSTPASS_WriteDB(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	flag := uuid.NewString()
	randomName := flag + uuid.NewString()
	var newAiToolID int64
	randomDescription := uuid.NewString()
	randomContent := uuid.NewString()
	randomToolPath := uuid.NewString()
	randomKeywords := []string{uuid.NewString()}
	t.Run("CreateAITool", func(t *testing.T) {
		_, err := c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
			Name:        randomName,
			Description: randomDescription,
			Content:     randomContent,
			ToolPath:    randomToolPath,
			Keywords:    randomKeywords,
		})
		require.NoError(t, err)
		aiListRsp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: randomName,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, randomDescription, aiListRsp.Tools[0].Description)
		assert.Equal(t, randomContent, aiListRsp.Tools[0].Content)
		assert.Equal(t, randomToolPath, aiListRsp.Tools[0].ToolPath)
		assert.Equal(t, randomKeywords, aiListRsp.Tools[0].Keywords)
		assert.Equal(t, randomName, aiListRsp.Tools[0].Name)
		newAiToolID = aiListRsp.Tools[0].ID
	})
	newRandomName := flag + uuid.NewString()
	newRandomDescription := uuid.NewString()
	newRandomContent := "print('test')"
	newRandomToolPath := uuid.NewString()
	newRandomKeywords := []string{uuid.NewString()}
	t.Run("UpdateAITool", func(t *testing.T) {
		// 不更新工具名
		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          newAiToolID,
			Name:        randomName,
			Description: newRandomDescription,
			Content:     newRandomContent,
			ToolPath:    newRandomToolPath,
			Keywords:    newRandomKeywords,
		})
		require.NoError(t, err)
		aiListRsp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: randomName,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, newRandomDescription, aiListRsp.Tools[0].Description)
		assert.Equal(t, newRandomContent, aiListRsp.Tools[0].Content)
		assert.Equal(t, newRandomToolPath, aiListRsp.Tools[0].ToolPath)
		assert.Equal(t, newRandomKeywords, aiListRsp.Tools[0].Keywords)

		// 更新工具名
		_, err = c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          newAiToolID,
			Name:        newRandomName,
			Description: newRandomDescription,
			Content:     newRandomContent,
			ToolPath:    newRandomToolPath,
			Keywords:    newRandomKeywords,
		})
		require.NoError(t, err)
		aiListRsp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: newRandomName,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, newRandomName, aiListRsp.Tools[0].Name)
		assert.Equal(t, newRandomDescription, aiListRsp.Tools[0].Description)
		assert.Equal(t, newRandomContent, aiListRsp.Tools[0].Content)
		assert.Equal(t, newRandomToolPath, aiListRsp.Tools[0].ToolPath)
		assert.Equal(t, newRandomKeywords, aiListRsp.Tools[0].Keywords)

		aiListRsp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: flag,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, aiListRsp.Tools, 1)
	})
	t.Run("DeleteAITool", func(t *testing.T) {
		_, err := c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{randomName, newRandomName},
		})
		require.NoError(t, err)
		aiListRsp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: flag,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, aiListRsp.Tools, 0)
	})
}

type TestAIClient struct {
	rsp string
	openai.GetawayClient
}

func (g *TestAIClient) CheckValid() error {
	return nil
}
func (c *TestAIClient) LoadOption(opts ...aispec.AIConfigOption) {
	return
}
func (g *TestAIClient) Chat(s string, function ...any) (string, error) {
	return g.rsp, nil
}

// _TestGRPCMUSTPASS_GenerateMetadata 测试生成工具元数据
func _TestGRPCMUSTPASS_GenerateMetadata(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	config_bak, _ := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	defer func() {
		client.SetGlobalNetworkConfig(context.Background(), config_bak)
	}()
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	config.AiApiPriority = []string{"gpt9o"}
	config.AppConfigs = []*ypb.ThirdPartyApplicationConfig{
		{
			Type:   "gpt9o",
			APIKey: "test",
		},
	}
	rspData := map[string]any{
		"language":    "chinese",
		"description": uuid.NewString(),
		"keywords":    []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
	}
	rsp, err := json.Marshal(rspData)
	if err != nil {
		t.Fatal(err)
	}

	aispec.Register("gpt9o", func() aispec.AIClient {
		aiclient := &TestAIClient{
			rsp: string(rsp),
		}
		return aiclient
	})
	client.SetGlobalNetworkConfig(context.Background(), config)
	yakit.LoadGlobalNetworkConfig()

	ctx := context.Background()
	resp, err := client.AIToolGenerateMetadata(ctx, &ypb.AIToolGenerateMetadataRequest{
		ToolName: "test",
		Content: `
cli.String("url")
code = ocr.ocr(url)
print("your code is: " + code)
		`,
	})
	require.NoError(t, err)
	assert.Equal(t, rspData["description"], resp.Description)
	assert.Equal(t, rspData["keywords"], resp.Keywords)
}

// TestGRPCMUSTPASS_ToggleAIToolFavorite 测试AI工具收藏功能
func TestGRPCMUSTPASS_ToggleAIToolFavorite(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 创建测试工具
	testToolName := "test-favorite-tool-" + uuid.NewString()
	_, err = c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
		Name:        testToolName,
		Description: "Test tool for favorite functionality",
		Content:     "print('test')",
		ToolPath:    "/test/path",
		Keywords:    []string{"test", "favorite"},
	})
	require.NoError(t, err)

	// 清理测试数据
	defer func() {
		c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{testToolName},
		})
	}()

	t.Run("ToggleFavoriteFromFalseToTrue", func(t *testing.T) {
		// 初始状态应该是非收藏
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.False(t, resp.Tools[0].IsFavorite, "Tool should not be favorite initially")

		// 切换为收藏
		toggleResp, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		assert.True(t, toggleResp.IsFavorite, "Tool should be favorite after toggle")
		assert.Equal(t, "Tool added to favorites", toggleResp.Message)

		// 验证状态已更新
		resp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.True(t, resp.Tools[0].IsFavorite, "Tool should be favorite after toggle")
	})

	t.Run("ToggleFavoriteFromTrueToFalse", func(t *testing.T) {
		// 当前应该是收藏状态（从上一个测试继续）
		// 再次切换，取消收藏
		toggleResp, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		assert.False(t, toggleResp.IsFavorite, "Tool should not be favorite after second toggle")
		assert.Equal(t, "Tool removed from favorites", toggleResp.Message)

		// 验证状态已更新
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.False(t, resp.Tools[0].IsFavorite, "Tool should not be favorite after second toggle")
	})

	t.Run("ToggleNonExistentTool", func(t *testing.T) {
		// 尝试切换不存在的工具
		_, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: "nonexistent-tool-" + uuid.NewString(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AI tool not found")
	})
}

// TestGRPCMUSTPASS_GetAIToolListWithFavorites 测试带收藏过滤的AI工具列表功能
func TestGRPCMUSTPASS_GetAIToolListWithFavorites(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 创建测试工具
	favoriteToolName := "favorite-tool-" + uuid.NewString()
	normalToolName := "normal-tool-" + uuid.NewString()
	testFlag := "test-favorite-flag-" + uuid.NewString()

	// 创建两个工具
	_, err = c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
		Name:        favoriteToolName,
		Description: "Favorite test tool " + testFlag,
		Content:     "print('favorite')",
		ToolPath:    "/test/favorite",
		Keywords:    []string{"test", "favorite", testFlag},
	})
	require.NoError(t, err)

	_, err = c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
		Name:        normalToolName,
		Description: "Normal test tool " + testFlag,
		Content:     "print('normal')",
		ToolPath:    "/test/normal",
		Keywords:    []string{"test", "normal", testFlag},
	})
	require.NoError(t, err)

	// 清理测试数据
	defer func() {
		c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{favoriteToolName, normalToolName},
		})
	}()

	// 将第一个工具设为收藏
	_, err = c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
		ToolName: favoriteToolName,
	})
	require.NoError(t, err)

	t.Run("GetAllTools", func(t *testing.T) {
		// 获取所有工具（不过滤收藏）
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query:         testFlag,
			OnlyFavorites: false,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 2, "Should return both tools")

		// 验证收藏状态
		var favoriteFound, normalFound bool
		for _, tool := range resp.Tools {
			if tool.Name == favoriteToolName {
				assert.True(t, tool.IsFavorite, "Favorite tool should have IsFavorite=true")
				favoriteFound = true
			} else if tool.Name == normalToolName {
				assert.False(t, tool.IsFavorite, "Normal tool should have IsFavorite=false")
				normalFound = true
			}
		}
		assert.True(t, favoriteFound, "Should find favorite tool")
		assert.True(t, normalFound, "Should find normal tool")
	})

	t.Run("GetOnlyFavorites", func(t *testing.T) {
		// 只获取收藏的工具
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query:         testFlag,
			OnlyFavorites: true,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 1, "Should return only favorite tool")
		assert.Equal(t, favoriteToolName, resp.Tools[0].Name)
		assert.True(t, resp.Tools[0].IsFavorite, "Returned tool should be favorite")
	})

	t.Run("GetOnlyFavoritesWithNoFavorites", func(t *testing.T) {
		// 先取消收藏
		_, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: favoriteToolName,
		})
		require.NoError(t, err)

		// 只获取收藏的工具（应该为空）
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query:         testFlag,
			OnlyFavorites: true,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 0, "Should return no tools when no favorites exist")
	})

	t.Run("GetSpecificFavoriteTool", func(t *testing.T) {
		// 重新设为收藏
		_, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: favoriteToolName,
		})
		require.NoError(t, err)

		// 按名称获取特定工具
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: favoriteToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.Equal(t, favoriteToolName, resp.Tools[0].Name)
		assert.True(t, resp.Tools[0].IsFavorite, "Specific tool should show correct favorite status")
	})
}

// TestGRPCMUSTPASS_AIToolFavoriteConsistency 测试收藏状态的一致性
func TestGRPCMUSTPASS_AIToolFavoriteConsistency(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 创建测试工具
	testToolName := "consistency-test-tool-" + uuid.NewString()
	_, err = c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
		Name:        testToolName,
		Description: "Test tool for consistency check",
		Content:     "print('consistency')",
		ToolPath:    "/test/consistency",
		Keywords:    []string{"test", "consistency"},
	})
	require.NoError(t, err)

	// 清理测试数据
	defer func() {
		c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{testToolName},
		})
	}()

	t.Run("IsFavoriteFieldConsistency", func(t *testing.T) {
		// 验证新创建的工具默认不是收藏
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.False(t, resp.Tools[0].IsFavorite, "New tool should not be favorite by default")

		// 设为收藏
		toggleResp, err := c.ToggleAIToolFavorite(ctx, &ypb.ToggleAIToolFavoriteRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		assert.True(t, toggleResp.IsFavorite)

		// 通过不同方式验证状态一致性
		// 1. 按名称查询
		resp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		assert.True(t, resp.Tools[0].IsFavorite, "Tool should be favorite when queried by name")

		// 2. 通过搜索查询
		resp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: "consistency",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		found := false
		for _, tool := range resp.Tools {
			if tool.Name == testToolName {
				assert.True(t, tool.IsFavorite, "Tool should be favorite when found through search")
				found = true
				break
			}
		}
		assert.True(t, found, "Tool should be found in search results")

		// 3. 通过收藏过滤查询
		resp, err = c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query:         "consistency",
			OnlyFavorites: true,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		found = false
		for _, tool := range resp.Tools {
			if tool.Name == testToolName {
				assert.True(t, tool.IsFavorite, "Tool should be favorite when found through favorite filter")
				found = true
				break
			}
		}
		assert.True(t, found, "Tool should be found in favorite filter results")
	})
}

// TestGRPCMUSTPASS_CreateAndQueryAITool 测试创建AI工具后查询该工具
func TestGRPCMUSTPASS_CreateAndQueryAITool(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 准备测试数据
	testToolName := "create-query-test-tool-" + uuid.NewString()
	testDescription := "Test tool for create and query - " + uuid.NewString()
	testContent := "print('Hello from test tool')\ncli.String('param1')"
	testToolPath := "/test/create/query/" + uuid.NewString()
	testKeywords := []string{"create", "query", "test", uuid.NewString()}

	// 清理测试数据
	defer func() {
		c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{testToolName},
		})
	}()

	t.Run("CreateAndQueryByName", func(t *testing.T) {
		// 创建工具
		_, err := c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
			Name:        testToolName,
			Description: testDescription,
			Content:     testContent,
			ToolPath:    testToolPath,
			Keywords:    testKeywords,
		})
		require.NoError(t, err)

		// 按名称查询工具
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1, "Should find exactly one tool")

		// 验证工具的所有字段
		tool := resp.Tools[0]
		assert.Equal(t, testToolName, tool.Name, "Tool name should match")
		assert.Equal(t, testDescription, tool.Description, "Tool description should match")
		assert.Equal(t, testContent, tool.Content, "Tool content should match")
		assert.Equal(t, testToolPath, tool.ToolPath, "Tool path should match")
		assert.Equal(t, testKeywords, tool.Keywords, "Tool keywords should match")
		assert.False(t, tool.IsFavorite, "Tool should not be favorite by default")
		assert.Greater(t, tool.ID, int64(0), "Tool ID should be positive")
	})

	t.Run("QueryByKeyword", func(t *testing.T) {
		// 使用关键词查询
		uniqueKeyword := testKeywords[len(testKeywords)-1] // 使用最后一个唯一的关键词
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: uniqueKeyword,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.True(t, len(resp.Tools) >= 1, "Should find at least one tool with the keyword")

		// 验证找到的工具包含我们创建的工具
		found := false
		for _, tool := range resp.Tools {
			if tool.Name == testToolName {
				found = true
				assert.Equal(t, testDescription, tool.Description)
				assert.Equal(t, testContent, tool.Content)
				break
			}
		}
		assert.True(t, found, "Should find the created tool by keyword")
	})

	t.Run("QueryByDescription", func(t *testing.T) {
		// 使用描述中的部分内容查询
		descriptionPart := testDescription[len("Test tool for "):]
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: descriptionPart,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)

		// 验证找到的工具包含我们创建的工具
		found := false
		for _, tool := range resp.Tools {
			if tool.Name == testToolName {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find the created tool by description content")
	})

	t.Run("QueryWithPagination", func(t *testing.T) {
		// 测试分页功能
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: testToolName,
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   1,
				OrderBy: "created_at",
				Order:   "desc",
			},
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 1, "Should respect limit parameter")
		assert.Equal(t, testToolName, resp.Tools[0].Name)
	})

	t.Run("VerifyToolPersistence", func(t *testing.T) {
		// 多次查询验证工具持久化
		for i := 0; i < 3; i++ {
			resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
				ToolName: testToolName,
			})
			require.NoError(t, err)
			require.Len(t, resp.Tools, 1, "Tool should persist across multiple queries")
			assert.Equal(t, testToolName, resp.Tools[0].Name)
		}
	})
}

// TestGRPCMUSTPASS_SaveAIToolV2_FixMetadata 测试SaveAIToolV2的fixAIToolMetadata功能
func TestGRPCMUSTPASS_SaveAIToolV2_FixMetadata(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("AutoFixMissingFields", func(t *testing.T) {
		// 测试：如果字段缺失，会自动修复
		toolName := "test-auto-fix-" + uuid.NewString()
		// 提供一个有效的yak脚本但不提供description和keywords
		content := `__DESC__ = "Auto generated description"
__VERBOSE_NAME__ = "Test Plugin Name"
__KEYWORDS__ = "test,auto,fix"

cli.String("url", cli.setRequired(true))
println("Test")
`
		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:    toolName,
			Content: content,
			// 故意不提供 Description 和 Keywords，让fixAIToolMetadata自动填充
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)
		assert.NotNil(t, resp.AITool)

		// 验证自动填充的字段
		assert.Equal(t, toolName, resp.AITool.Name)
		assert.NotEmpty(t, resp.AITool.Description, "Description should be auto-filled")
		assert.NotEmpty(t, resp.AITool.Keywords, "Keywords should be auto-filled")
		assert.NotEmpty(t, resp.AITool.VerboseName, "VerboseName should be auto-filled")
		assert.Equal(t, "Test Plugin Name", resp.AITool.VerboseName)

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})

	t.Run("UseUserProvidedFieldsWhenNotMissing", func(t *testing.T) {
		// 测试：如果字段都不缺失，以用户传入的为准
		toolName := "test-user-provided-" + uuid.NewString()
		userDescription := "User provided description " + uuid.NewString()
		userKeywords := []string{"user", "provided", uuid.NewString()}
		content := `__DESC__ = "Auto description"
__VERBOSE_NAME__ = "Auto Verbose Name"
__KEYWORDS__ = "auto,keywords"

cli.String("url")
println("Test")
`
		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:        toolName,
			Description: userDescription,
			Keywords:    userKeywords,
			Content:     content,
			ToolPath:    "/user/custom/path",
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)
		assert.NotNil(t, resp.AITool)

		// 验证使用用户提供的字段
		assert.Equal(t, toolName, resp.AITool.Name)
		assert.Equal(t, userDescription, resp.AITool.Description, "Should use user provided description")
		assert.Equal(t, userKeywords, resp.AITool.Keywords, "Should use user provided keywords")
		assert.Equal(t, "/user/custom/path", resp.AITool.ToolPath, "Should use user provided path")

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})

	t.Run("FailOnInvalidScript", func(t *testing.T) {
		// 测试：如果解析参数失败，保存应该失败
		toolName := "test-invalid-" + uuid.NewString()
		// 提供一个无效的脚本内容
		invalidContent := "this is not a valid yak script ###@@@ invalid syntax"

		_, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:        toolName,
			Description: "Should fail",
			Content:     invalidContent,
		})
		// 应该返回错误
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fix AI tool metadata", "Should fail with metadata error")

		// 验证工具没有被创建
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 0, "Invalid tool should not be created")
	})

	t.Run("PartialFieldsProvided", func(t *testing.T) {
		// 测试：部分字段提供，部分字段缺失
		toolName := "test-partial-" + uuid.NewString()
		userDescription := "Partial user description " + uuid.NewString()
		content := `__DESC__ = "Auto description for partial"
__VERBOSE_NAME__ = "Partial Verbose Name"
__KEYWORDS__ = "auto,partial"

cli.String("input")
println("Partial test")
`
		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:        toolName,
			Description: userDescription, // 用户提供
			// Keywords 缺失，应该自动填充
			Content: content,
			// ToolPath 缺失，应该自动填充
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)
		assert.NotNil(t, resp.AITool)

		// 验证混合使用用户提供和自动填充的字段
		assert.Equal(t, toolName, resp.AITool.Name)
		assert.Equal(t, userDescription, resp.AITool.Description, "Should use user provided description")
		assert.NotEmpty(t, resp.AITool.Keywords, "Keywords should be auto-filled")
		assert.NotEmpty(t, resp.AITool.VerboseName, "VerboseName should be auto-filled")

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})
}

// TestGRPCMUSTPASS_UpdateAITool_FixMetadata 测试UpdateAITool的fixAIToolMetadata功能
func TestGRPCMUSTPASS_UpdateAITool_FixMetadata(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 先创建一个工具用于更新测试
	toolName := "test-update-fix-" + uuid.NewString()
	initialContent := `__DESC__ = "Initial description"
__VERBOSE_NAME__ = "Initial Verbose Name"
__KEYWORDS__ = "initial,test"

cli.String("param1")
println("Initial")
`
	createResp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
		Name:        toolName,
		Description: "Initial description",
		Keywords:    []string{"initial", "test"},
		Content:     initialContent,
	})
	require.NoError(t, err)
	toolID := createResp.AITool.ID

	// 清理
	defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
		ToolNames: []string{toolName},
	})

	t.Run("UpdateWithAutoFixMissingFields", func(t *testing.T) {
		// 更新时不提供某些字段，应该自动修复
		newContent := `__DESC__ = "Updated auto description"
__VERBOSE_NAME__ = "Updated Verbose Name"
__KEYWORDS__ = "updated,auto,fix"

cli.String("newparam")
println("Updated")
`
		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:      toolID,
			Name:    toolName,
			Content: newContent,
			// 不提供 Description 和 Keywords，让fixAIToolMetadata自动填充
		})
		require.NoError(t, err)

		// 验证更新后的字段
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)

		tool := resp.Tools[0]
		assert.NotEmpty(t, tool.Description, "Description should be auto-filled")
		assert.NotEmpty(t, tool.Keywords, "Keywords should be auto-filled")
		assert.Equal(t, "Updated Verbose Name", tool.VerboseName)
	})

	t.Run("UpdateWithUserProvidedFields", func(t *testing.T) {
		// 更新时提供所有字段，应该使用用户提供的
		userDescription := "User updated description " + uuid.NewString()
		userKeywords := []string{"user", "updated", uuid.NewString()}
		newContent := `__DESC__ = "Should not use this description"
__VERBOSE_NAME__ = "Should Not Use This"
__KEYWORDS__ = "should,not,use"

cli.String("userparam")
println("User update")
`
		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          toolID,
			Name:        toolName,
			Description: userDescription,
			Keywords:    userKeywords,
			Content:     newContent,
			ToolPath:    "/user/updated/path",
		})
		require.NoError(t, err)

		// 验证使用用户提供的字段
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)

		tool := resp.Tools[0]
		assert.Equal(t, userDescription, tool.Description, "Should use user provided description")
		assert.Equal(t, userKeywords, tool.Keywords, "Should use user provided keywords")
		assert.Equal(t, "/user/updated/path", tool.ToolPath, "Should use user provided path")
	})

	t.Run("UpdateFailOnInvalidScript", func(t *testing.T) {
		// 更新时提供无效脚本，应该失败
		invalidContent := "invalid script content ###@@@ syntax error"

		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          toolID,
			Name:        toolName,
			Description: "Should fail",
			Content:     invalidContent,
		})
		// 应该返回错误
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fix AI tool metadata", "Should fail with metadata error")

		// 验证工具没有被更新（保持原来的内容）
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)
		// 内容应该保持之前的有效内容
		assert.NotEqual(t, invalidContent, resp.Tools[0].Content, "Content should not be updated to invalid script")
	})

	t.Run("UpdatePartialFields", func(t *testing.T) {
		// 更新时部分字段提供，部分缺失
		userDescription := "Partial update description " + uuid.NewString()
		newContent := `__DESC__ = "Partial auto description"
__VERBOSE_NAME__ = "Partial Update Verbose"
__KEYWORDS__ = "partial,update,auto"

cli.String("partialParam")
println("Partial update")
`
		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          toolID,
			Name:        toolName,
			Description: userDescription, // 用户提供
			// Keywords 缺失，应该自动填充
			Content: newContent,
			// ToolPath 缺失，应该自动填充
		})
		require.NoError(t, err)

		// 验证混合使用
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)

		tool := resp.Tools[0]
		assert.Equal(t, userDescription, tool.Description, "Should use user provided description")
		assert.NotEmpty(t, tool.Keywords, "Keywords should be auto-filled")
		assert.Equal(t, "Partial Update Verbose", tool.VerboseName)
	})

	t.Run("UpdateWithEmptyFieldsGetAutoFilled", func(t *testing.T) {
		// 测试显式传入空字符串时，应该被自动填充
		newContent := `__DESC__ = "Auto fill for empty fields"
__VERBOSE_NAME__ = "Empty Fields Verbose"
__KEYWORDS__ = "empty,auto,fill"

cli.String("emptyParam")
println("Empty fields test")
`
		_, err := c.UpdateAITool(ctx, &ypb.UpdateAIToolRequest{
			ID:          toolID,
			Name:        toolName,
			Description: "", // 显式传空
			Keywords:    []string{},
			Content:     newContent,
			ToolPath:    "", // 显式传空
		})
		require.NoError(t, err)

		// 验证空字段被自动填充
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: toolName,
		})
		require.NoError(t, err)
		require.Len(t, resp.Tools, 1)

		tool := resp.Tools[0]
		assert.NotEmpty(t, tool.Description, "Empty description should be auto-filled")
		assert.NotEmpty(t, tool.Keywords, "Empty keywords should be auto-filled")
		assert.NotEmpty(t, tool.ToolPath, "Empty path should be auto-filled")
	})
}

// TestGRPCMUSTPASS_FixAIToolMetadata_EdgeCases 测试fixAIToolMetadata的边界情况
func TestGRPCMUSTPASS_FixAIToolMetadata_EdgeCases(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("ComplexYakScriptWithMultipleParams", func(t *testing.T) {
		// 测试包含多个参数的复杂脚本
		toolName := "test-complex-" + uuid.NewString()
		complexContent := `__DESC__ = "A complex plugin with multiple parameters"
__VERBOSE_NAME__ = "Complex Plugin"
__KEYWORDS__ = "complex,multiple,params"

url = cli.String("url", cli.setRequired(true))
timeout = cli.Int("timeout", cli.setDefault(30))
headers = cli.StringSlice("headers")
method = cli.String("method", cli.setDefault("GET"))

println("URL:", url)
println("Timeout:", timeout)
`
		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:    toolName,
			Content: complexContent,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)
		assert.NotNil(t, resp.AITool)

		// 验证复杂参数被正确解析
		assert.NotEmpty(t, resp.AITool.Description)
		assert.NotEmpty(t, resp.AITool.Keywords)
		assert.Equal(t, "Complex Plugin", resp.AITool.VerboseName)

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})

	t.Run("MinimalValidScript", func(t *testing.T) {
		// 测试最小有效脚本
		toolName := "test-minimal-" + uuid.NewString()
		minimalContent := `println("hello")`

		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:    toolName,
			Content: minimalContent,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})

	t.Run("ScriptWithSpecialCharacters", func(t *testing.T) {
		// 测试包含特殊字符的脚本
		toolName := "test-special-" + uuid.NewString()
		specialContent := `__DESC__ = "包含中文和特殊字符 @#$%"
__VERBOSE_NAME__ = "Special 特殊 Plugin 插件"
__KEYWORDS__ = "特殊,中文,special"

cli.String("参数1")
println("测试特殊字符: !@#$%^&*()")
`
		resp, err := c.SaveAIToolV2(ctx, &ypb.SaveAIToolRequest{
			Name:    toolName,
			Content: specialContent,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsSuccess)
		assert.Equal(t, "Special 特殊 Plugin 插件", resp.AITool.VerboseName)

		// 清理
		defer c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		})
	})
}
