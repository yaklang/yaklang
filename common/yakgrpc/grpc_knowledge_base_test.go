package yakgrpc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMUSTPASS_KnowledgeBaseCRUD(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatalf("创建本地客户端失败: %v", err)
	}
	knowledgeBaseName := "test_knowledge_base" + utils.RandStringBytes(6)
	ctx := context.Background()
	response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        knowledgeBaseName,
		Description: "test_knowledge_base_description",
		Tags:        []string{"test_knowledge_base_type"},
	})
	if err != nil {
		t.Fatalf("创建知识库失败: %v", err)
	}

	assert.True(t, response.IsSuccess)
	assert.NotZero(t, response.KnowledgeBase.ID)

	client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: response.KnowledgeBase.ID,
	})
	if err != nil {
		t.Fatalf("获取知识库信息失败: %v", err)
	}
	assert.NotNil(t, response.KnowledgeBase)
	assert.Equal(t, response.KnowledgeBase.KnowledgeBaseName, knowledgeBaseName)
	assert.Equal(t, response.KnowledgeBase.KnowledgeBaseDescription, "test_knowledge_base_description")
	assert.Equal(t, response.KnowledgeBase.Tags, []string{"test_knowledge_base_type"})

	_, err = client.UpdateKnowledgeBase(ctx, &ypb.UpdateKnowledgeBaseRequest{
		KnowledgeBaseId:          response.KnowledgeBase.ID,
		KnowledgeBaseName:        knowledgeBaseName + "_new",
		KnowledgeBaseDescription: "test_knowledge_base_description_new",
		Tags:                     []string{"test_knowledge_base_type_new", "test_knowledge_base_type_new_2"},
	})
	if err != nil {
		t.Fatalf("更新知识库信息失败: %v", err)
	}

	getResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: response.KnowledgeBase.ID,
	})
	if err != nil {
		t.Fatalf("获取知识库信息失败: %v", err)
	}
	assert.Len(t, getResponse.KnowledgeBases, 1)
	assert.Equal(t, getResponse.KnowledgeBases[0].KnowledgeBaseName, knowledgeBaseName+"_new")
	assert.Equal(t, getResponse.KnowledgeBases[0].KnowledgeBaseDescription, "test_knowledge_base_description_new")
	assert.Equal(t, getResponse.KnowledgeBases[0].Tags, []string{"test_knowledge_base_type_new", "test_knowledge_base_type_new_2"})

	deleteResponse, err := client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{
		Name: response.KnowledgeBase.KnowledgeBaseName,
	})
	if err != nil {
		t.Fatalf("删除知识库失败: %v", err)
	}
	assert.True(t, deleteResponse.Ok)
}

func TestMUSTPASS_TestImportedFlag(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err, "创建本地客户端失败")

	ctx := context.Background()
	originalKBName := "test_imported_flag_original"
	importedKBName := "test_imported_flag_imported"

	// 清理函数，确保测试结束后删除知识库和临时文件
	defer func() {
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: originalKBName})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: importedKBName})
	}()

	// 1. 创建一个知识库
	createResponse, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        originalKBName,
		Description: "original knowledge base for testing imported flag",
		Tags:        []string{"test"},
	})
	require.NoError(t, err, "创建知识库失败")
	require.True(t, createResponse.IsSuccess)
	require.NotZero(t, createResponse.KnowledgeBase.ID)
	originalKBId := createResponse.KnowledgeBase.ID

	// 2. 增加一条测试数据
	_, err = client.CreateKnowledgeBaseEntry(ctx, &ypb.CreateKnowledgeBaseEntryRequest{
		KnowledgeBaseID:  originalKBId,
		KnowledgeTitle:   "test entry",
		KnowledgeType:    "text",
		ImportanceScore:  5,
		Keywords:         []string{"test", "entry"},
		KnowledgeDetails: "this is a test entry for imported flag testing",
		Summary:          "test entry summary",
	})
	require.NoError(t, err, "创建知识库条目失败")

	// 3. 导出知识库到临时文件
	tempDir := t.TempDir()
	exportPath := filepath.Join(tempDir, "test_knowledge_base.rag")

	exportStream, err := client.ExportKnowledgeBase(ctx, &ypb.ExportKnowledgeBaseRequest{
		KnowledgeBaseId: originalKBId,
		TargetPath:      exportPath,
	})
	require.NoError(t, err, "导出知识库失败")

	// 等待导出完成
	for {
		_, err := exportStream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "导出知识库过程中失败")
	}

	// 验证导出文件存在
	_, err = os.Stat(exportPath)
	require.NoError(t, err, "导出文件不存在")

	// 4. 导入知识库，使用新名称
	importStream, err := client.ImportKnowledgeBase(ctx, &ypb.ImportKnowledgeBaseRequest{
		NewKnowledgeBaseName: importedKBName,
		InputPath:            exportPath,
	})
	require.NoError(t, err, "导入知识库失败")

	// 等待导入完成
	for {
		_, err := importStream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "导入知识库过程中失败")
	}

	// 5. 查询原始知识库，验证 IsImported 为 false
	originalKBResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: originalKBId,
	})
	require.NoError(t, err, "获取原始知识库失败")
	require.Len(t, originalKBResponse.KnowledgeBases, 1)
	assert.False(t, originalKBResponse.KnowledgeBases[0].IsImported, "原始知识库的 IsImported 应该为 false")

	// 6. 查询导入的知识库，验证 IsImported 为 true
	importedKBResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Keyword: importedKBName,
	})
	require.NoError(t, err, "获取导入知识库失败")
	require.Len(t, importedKBResponse.KnowledgeBases, 1)
	assert.True(t, importedKBResponse.KnowledgeBases[0].IsImported, "导入的知识库的 IsImported 应该为 true")
	assert.Equal(t, importedKBName, importedKBResponse.KnowledgeBases[0].KnowledgeBaseName)

	// 7. 删除导入的知识库
	deleteResponse, err := client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{
		Name: importedKBName,
	})
	require.NoError(t, err, "删除导入知识库失败")
	assert.True(t, deleteResponse.Ok)

	// 8. 删除原始知识库
	deleteResponse, err = client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{
		Name: originalKBName,
	})
	require.NoError(t, err, "删除原始知识库失败")
	assert.True(t, deleteResponse.Ok)
}
