package yakgrpc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
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
	db := consts.GetGormProfileDatabase()

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

	// 6.1 清空 collection 的 serial_version_uid，验证 IsImported 仍然为 true（依赖知识库表的 serial_version_uid）
	err = db.Model(&schema.VectorStoreCollection{}).Where("name = ?", importedKBName).Update("serial_version_uid", "").Error
	require.NoError(t, err)
	importedKBResponse2, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Keyword: importedKBName,
	})
	require.NoError(t, err)
	require.Len(t, importedKBResponse2.KnowledgeBases, 1)
	assert.True(t, importedKBResponse2.KnowledgeBases[0].IsImported, "清空 collection serial_version_uid 后，导入的知识库仍应为 true")

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

func TestMUSTPASS_TestIsDefaultFlag(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err, "创建本地客户端失败")

	ctx := context.Background()
	defaultKBName := "test_is_default_true_" + utils.RandStringBytes(6)
	normalKBName := "test_is_default_false_" + utils.RandStringBytes(6)

	// 清理函数，确保测试结束后删除知识库
	defer func() {
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: defaultKBName})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: normalKBName})
	}()

	// 1. 创建一个默认知识库 (IsDefault = true)
	createDefaultResponse, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        defaultKBName,
		Description: "default knowledge base for testing",
		Tags:        []string{"test", "default"},
		IsDefault:   true,
	})
	require.NoError(t, err, "创建默认知识库失败")
	require.True(t, createDefaultResponse.IsSuccess)
	require.NotZero(t, createDefaultResponse.KnowledgeBase.ID)
	defaultKBId := createDefaultResponse.KnowledgeBase.ID

	// 2. 验证创建的知识库 IsDefault 为 true
	assert.True(t, createDefaultResponse.KnowledgeBase.IsDefault, "创建时设置 IsDefault=true，返回应为 true")

	// 3. 查询默认知识库，验证 IsDefault 为 true
	getDefaultResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: defaultKBId,
	})
	require.NoError(t, err, "获取默认知识库失败")
	require.Len(t, getDefaultResponse.KnowledgeBases, 1)
	assert.True(t, getDefaultResponse.KnowledgeBases[0].IsDefault, "查询默认知识库的 IsDefault 应为 true")

	// 4. 创建一个普通知识库 (IsDefault = false，默认值)
	createNormalResponse, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        normalKBName,
		Description: "normal knowledge base for testing",
		Tags:        []string{"test", "normal"},
		IsDefault:   false,
	})
	require.NoError(t, err, "创建普通知识库失败")
	require.True(t, createNormalResponse.IsSuccess)
	require.NotZero(t, createNormalResponse.KnowledgeBase.ID)
	normalKBId := createNormalResponse.KnowledgeBase.ID

	// 5. 验证创建的普通知识库 IsDefault 为 false
	assert.False(t, createNormalResponse.KnowledgeBase.IsDefault, "创建时未设置 IsDefault，返回应为 false")

	// 6. 查询普通知识库，验证 IsDefault 为 false
	getNormalResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: normalKBId,
	})
	require.NoError(t, err, "获取普通知识库失败")
	require.Len(t, getNormalResponse.KnowledgeBases, 1)
	assert.False(t, getNormalResponse.KnowledgeBases[0].IsDefault, "查询普通知识库的 IsDefault 应为 false")

	// 7. 通过 UpdateKnowledgeBase 将普通知识库设置为默认
	_, err = client.UpdateKnowledgeBase(ctx, &ypb.UpdateKnowledgeBaseRequest{
		KnowledgeBaseId:          normalKBId,
		KnowledgeBaseName:        normalKBName,
		KnowledgeBaseDescription: "normal knowledge base updated to default",
		Tags:                     []string{"test", "updated"},
		IsDefault:                true,
	})
	require.NoError(t, err, "更新知识库 IsDefault 失败")

	// 8. 验证更新后的知识库 IsDefault 为 true
	getUpdatedResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: normalKBId,
	})
	require.NoError(t, err, "获取更新后的知识库失败")
	require.Len(t, getUpdatedResponse.KnowledgeBases, 1)
	assert.True(t, getUpdatedResponse.KnowledgeBases[0].IsDefault, "更新后的知识库 IsDefault 应为 true")

	// 9. 将默认知识库取消默认设置
	_, err = client.UpdateKnowledgeBase(ctx, &ypb.UpdateKnowledgeBaseRequest{
		KnowledgeBaseId:          defaultKBId,
		KnowledgeBaseName:        defaultKBName,
		KnowledgeBaseDescription: "default knowledge base updated to normal",
		Tags:                     []string{"test", "updated"},
		IsDefault:                false,
	})
	require.NoError(t, err, "取消默认知识库设置失败")

	// 10. 验证取消默认后的知识库 IsDefault 为 false
	getCancelledResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: defaultKBId,
	})
	require.NoError(t, err, "获取取消默认后的知识库失败")
	require.Len(t, getCancelledResponse.KnowledgeBases, 1)
	assert.False(t, getCancelledResponse.KnowledgeBases[0].IsDefault, "取消默认后的知识库 IsDefault 应为 false")
}
