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
	newRandomContent := uuid.NewString()
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
