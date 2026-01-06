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
	kb1Name := "test_is_default_kb1_" + utils.RandStringBytes(6)
	kb2Name := "test_is_default_kb2_" + utils.RandStringBytes(6)
	kb3Name := "test_is_default_kb3_" + utils.RandStringBytes(6)

	// 清理函数，确保测试结束后删除知识库
	defer func() {
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: kb1Name})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: kb2Name})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: kb3Name})
	}()

	// 1. 创建第一个知识库并设置为默认
	createKb1Response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        kb1Name,
		Description: "knowledge base 1 for testing default flag",
		Tags:        []string{"test", "kb1"},
		IsDefault:   true,
	})
	require.NoError(t, err, "创建知识库1失败")
	require.True(t, createKb1Response.IsSuccess)
	require.NotZero(t, createKb1Response.KnowledgeBase.ID)
	kb1Id := createKb1Response.KnowledgeBase.ID

	// 2. 验证知识库1的 IsDefault 为 true
	assert.True(t, createKb1Response.KnowledgeBase.IsDefault, "知识库1创建时设置 IsDefault=true，返回应为 true")

	// 3. 查询知识库1，验证 IsDefault 为 true
	getKb1Response, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: kb1Id,
	})
	require.NoError(t, err, "获取知识库1失败")
	require.Len(t, getKb1Response.KnowledgeBases, 1)
	assert.True(t, getKb1Response.KnowledgeBases[0].IsDefault, "查询知识库1的 IsDefault 应为 true")

	// 4. 创建第二个知识库（不设置为默认）
	createKb2Response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        kb2Name,
		Description: "knowledge base 2 for testing default flag",
		Tags:        []string{"test", "kb2"},
		IsDefault:   false,
	})
	require.NoError(t, err, "创建知识库2失败")
	require.True(t, createKb2Response.IsSuccess)
	require.NotZero(t, createKb2Response.KnowledgeBase.ID)
	kb2Id := createKb2Response.KnowledgeBase.ID

	// 5. 验证知识库2的 IsDefault 为 false
	assert.False(t, createKb2Response.KnowledgeBase.IsDefault, "知识库2创建时未设置 IsDefault，返回应为 false")

	// 6. 将知识库2设置为默认，验证知识库1自动取消默认
	_, err = client.UpdateKnowledgeBase(ctx, &ypb.UpdateKnowledgeBaseRequest{
		KnowledgeBaseId:          kb2Id,
		KnowledgeBaseName:        kb2Name,
		KnowledgeBaseDescription: "knowledge base 2 updated to default",
		Tags:                     []string{"test", "kb2", "default"},
		IsDefault:                true,
	})
	require.NoError(t, err, "将知识库2设置为默认失败")

	// 7. 验证知识库2现在是默认
	getKb2Response, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: kb2Id,
	})
	require.NoError(t, err, "获取知识库2失败")
	require.Len(t, getKb2Response.KnowledgeBases, 1)
	assert.True(t, getKb2Response.KnowledgeBases[0].IsDefault, "知识库2应该是默认知识库")

	// 8. 验证知识库1自动取消了默认（因为只能有一个默认知识库）
	getKb1Response2, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: kb1Id,
	})
	require.NoError(t, err, "获取知识库1失败")
	require.Len(t, getKb1Response2.KnowledgeBases, 1)
	assert.False(t, getKb1Response2.KnowledgeBases[0].IsDefault, "知识库1应该自动取消默认（只能有一个默认知识库）")

	// 9. 创建第三个知识库并直接设置为默认
	createKb3Response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        kb3Name,
		Description: "knowledge base 3 for testing default flag",
		Tags:        []string{"test", "kb3"},
		IsDefault:   true,
	})
	require.NoError(t, err, "创建知识库3失败")
	require.True(t, createKb3Response.IsSuccess)
	assert.True(t, createKb3Response.KnowledgeBase.IsDefault, "知识库3应该是默认知识库")

	// 10. 验证知识库2自动取消了默认
	getKb2Response2, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		KnowledgeBaseId: kb2Id,
	})
	require.NoError(t, err, "获取知识库2失败")
	require.Len(t, getKb2Response2.KnowledgeBases, 1)
	assert.False(t, getKb2Response2.KnowledgeBases[0].IsDefault, "知识库2应该自动取消默认（知识库3被设为默认）")

	// 11. 验证系统中只有一个默认知识库
	allKbResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 100},
	})
	require.NoError(t, err, "获取所有知识库失败")

	defaultCount := 0
	for _, kb := range allKbResponse.KnowledgeBases {
		if kb.IsDefault {
			defaultCount++
		}
	}
	assert.Equal(t, 1, defaultCount, "系统中应该只有一个默认知识库")
}

func TestMUSTPASS_TestOnlyIsDefaultFilter(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err, "创建本地客户端失败")

	ctx := context.Background()
	defaultKBName := "test_only_default_filter_default_" + utils.RandStringBytes(6)
	normalKB1Name := "test_only_default_filter_normal1_" + utils.RandStringBytes(6)
	normalKB2Name := "test_only_default_filter_normal2_" + utils.RandStringBytes(6)

	// 清理函数，确保测试结束后删除知识库
	defer func() {
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: defaultKBName})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: normalKB1Name})
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: normalKB2Name})
	}()

	// 1. 创建一个默认知识库
	createDefaultResponse, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        defaultKBName,
		Description: "default knowledge base for OnlyIsDefault filter testing",
		Tags:        []string{"test", "default"},
		IsDefault:   true,
	})
	require.NoError(t, err, "创建默认知识库失败")
	require.True(t, createDefaultResponse.IsSuccess)
	defaultKBId := createDefaultResponse.KnowledgeBase.ID

	// 2. 创建两个普通知识库
	createNormal1Response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        normalKB1Name,
		Description: "normal knowledge base 1 for OnlyIsDefault filter testing",
		Tags:        []string{"test", "normal"},
		IsDefault:   false,
	})
	require.NoError(t, err, "创建普通知识库1失败")
	require.True(t, createNormal1Response.IsSuccess)

	createNormal2Response, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        normalKB2Name,
		Description: "normal knowledge base 2 for OnlyIsDefault filter testing",
		Tags:        []string{"test", "normal"},
		IsDefault:   false,
	})
	require.NoError(t, err, "创建普通知识库2失败")
	require.True(t, createNormal2Response.IsSuccess)

	// 3. 使用 OnlyIsDefault=true 过滤，应该只返回默认知识库
	onlyDefaultResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Pagination:    &ypb.Paging{Page: 1, Limit: 100},
		OnlyIsDefault: true,
	})
	require.NoError(t, err, "使用 OnlyIsDefault 过滤失败")

	// 验证只返回一个结果，且是默认知识库
	require.Equal(t, int64(1), onlyDefaultResponse.Total, "OnlyIsDefault=true 应该只返回1个结果")
	require.Len(t, onlyDefaultResponse.KnowledgeBases, 1, "OnlyIsDefault=true 应该只返回1个知识库")
	assert.Equal(t, defaultKBId, onlyDefaultResponse.KnowledgeBases[0].ID, "返回的应该是默认知识库")
	assert.True(t, onlyDefaultResponse.KnowledgeBases[0].IsDefault, "返回的知识库 IsDefault 应该为 true")
	assert.Equal(t, defaultKBName, onlyDefaultResponse.KnowledgeBases[0].KnowledgeBaseName, "返回的知识库名称应该匹配")

	// 4. 不使用 OnlyIsDefault 过滤，应该返回所有知识库
	allResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Pagination:    &ypb.Paging{Page: 1, Limit: 100},
		OnlyIsDefault: false,
	})
	require.NoError(t, err, "获取所有知识库失败")

	// 验证返回的知识库数量大于1（至少包含我们创建的3个）
	assert.GreaterOrEqual(t, len(allResponse.KnowledgeBases), 3, "不使用 OnlyIsDefault 应该返回多个知识库")

	// 5. 使用 OnlyIsDefault=true 结合 Keyword 过滤
	onlyDefaultWithKeywordResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Keyword:       defaultKBName,
		Pagination:    &ypb.Paging{Page: 1, Limit: 100},
		OnlyIsDefault: true,
	})
	require.NoError(t, err, "使用 OnlyIsDefault 和 Keyword 过滤失败")

	// 应该返回匹配关键词且是默认的知识库
	require.Len(t, onlyDefaultWithKeywordResponse.KnowledgeBases, 1, "OnlyIsDefault=true + Keyword 应该返回1个知识库")
	assert.Equal(t, defaultKBId, onlyDefaultWithKeywordResponse.KnowledgeBases[0].ID, "返回的应该是默认知识库")

	// 6. 使用 OnlyIsDefault=true 结合不存在的关键词
	onlyDefaultWithWrongKeywordResponse, err := client.GetKnowledgeBase(ctx, &ypb.GetKnowledgeBaseRequest{
		Keyword:       "nonexistent_keyword_" + utils.RandStringBytes(10),
		Pagination:    &ypb.Paging{Page: 1, Limit: 100},
		OnlyIsDefault: true,
	})
	require.NoError(t, err, "使用 OnlyIsDefault 和不存在的 Keyword 过滤失败")

	// 应该返回空结果
	assert.Len(t, onlyDefaultWithWrongKeywordResponse.KnowledgeBases, 0, "OnlyIsDefault=true + 不存在的 Keyword 应该返回空结果")
}

func TestGenerateQuestionIndexForKnowledgeBase(t *testing.T) {
	t.Skip("缺少对 ai 和 mebedding 的 mock，暂时跳过")
	client, err := NewLocalClient()
	require.NoError(t, err, "创建本地客户端失败")

	ctx := context.Background()
	kbName := "test_gen_q_idx_" + utils.RandStringBytes(6)

	// 清理
	defer func() {
		client.DeleteKnowledgeBase(ctx, &ypb.DeleteKnowledgeBaseRequest{Name: kbName})
	}()

	// 1. 创建知识库
	createResp, err := client.CreateKnowledgeBaseV2(ctx, &ypb.CreateKnowledgeBaseV2Request{
		Name:        kbName,
		Description: "test description",
		Tags:        []string{"test"},
	})
	require.NoError(t, err)
	require.True(t, createResp.IsSuccess)
	kbId := createResp.KnowledgeBase.ID

	// 2. 添加条目
	_, err = client.CreateKnowledgeBaseEntry(ctx, &ypb.CreateKnowledgeBaseEntryRequest{
		KnowledgeBaseID:  kbId,
		KnowledgeTitle:   "Test Entry",
		KnowledgeType:    "text",
		KnowledgeDetails: "This is a test entry content for generating question index.",
		ImportanceScore:  5,
	})
	require.NoError(t, err)

	// 获取刚才创建的 entry 的 HiddenIndex
	searchResp, err := client.SearchKnowledgeBaseEntry(ctx, &ypb.SearchKnowledgeBaseEntryRequest{
		KnowledgeBaseId: kbId,
		Pagination:      &ypb.Paging{Page: 1, Limit: 10},
	})
	require.NoError(t, err)
	require.NotEmpty(t, searchResp.KnowledgeBaseEntries)
	entryHiddenIndex := searchResp.KnowledgeBaseEntries[0].HiddenIndex

	// 3. 调用 GenerateQuestionIndexForKnowledgeBase (指定条目)
	stream, err := client.GenerateQuestionIndexForKnowledgeBase(ctx, &ypb.GenerateQuestionIndexForKnowledgeBaseRequest{
		KnowledgeBaseId: kbId,
		HiddenIndex:     entryHiddenIndex,
	})
	require.NoError(t, err)

	// 对于单个条目生成，目前服务端实现可能不发送任何消息直接返回EOF（或者只发送错误）
	// 我们只要确保没有返回错误即可
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "接收流消息时出错")
	}

	// 4. 调用 GenerateQuestionIndexForKnowledgeBase (全量)
	streamAll, err := client.GenerateQuestionIndexForKnowledgeBase(ctx, &ypb.GenerateQuestionIndexForKnowledgeBaseRequest{
		KnowledgeBaseId: kbId,
	})
	require.NoError(t, err)

	receivedMessages := false
	for {
		resp, err := streamAll.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "接收流消息时出错")
		receivedMessages = true
		t.Logf("收到进度: %.2f%%, 消息: %s", resp.Percent, resp.Message)
	}
	assert.True(t, receivedMessages, "全量生成应该收到进度消息")
}
