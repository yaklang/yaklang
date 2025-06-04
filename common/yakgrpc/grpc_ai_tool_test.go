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
	randomDescription := uuid.NewString()
	randomContent := uuid.NewString()
	randomToolPath := uuid.NewString()
	randomKeywords := []string{uuid.NewString()}
	t.Run("CreateAITool", func(t *testing.T) {
		resp, err := c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
			Name:        randomName,
			Description: randomDescription,
			Content:     randomContent,
			ToolPath:    randomToolPath,
			Keywords:    randomKeywords,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.EffectRows)
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
	})
	newRandomName := flag + uuid.NewString()
	newRandomDescription := uuid.NewString()
	newRandomContent := uuid.NewString()
	newRandomToolPath := uuid.NewString()
	newRandomKeywords := []string{uuid.NewString()}
	t.Run("UpdateAITool", func(t *testing.T) {
		// 不更新工具名
		resp, err := c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
			Name:        randomName,
			Description: newRandomDescription,
			Content:     newRandomContent,
			ToolPath:    newRandomToolPath,
			Keywords:    newRandomKeywords,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.EffectRows)
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
		resp, err = c.SaveAITool(ctx, &ypb.SaveAIToolRequest{
			Name:        newRandomName,
			Description: newRandomDescription,
			Content:     newRandomContent,
			ToolPath:    newRandomToolPath,
			Keywords:    newRandomKeywords,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.EffectRows)
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
		assert.Len(t, aiListRsp.Tools, 2)
	})
	t.Run("DeleteAITool", func(t *testing.T) {
		resp, err := c.DeleteAITool(ctx, &ypb.DeleteAIToolRequest{
			ToolNames: []string{randomName, newRandomName},
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.EffectRows)
		aiListRsp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
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
func (g *TestAIClient) Chat(s string, function ...any) (string, error) {
	return g.rsp, nil
}

// TestGRPCMUSTPASS_GenerateMetadata 测试生成工具元数据
func TestGRPCMUSTPASS_GenerateMetadata(t *testing.T) {
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
