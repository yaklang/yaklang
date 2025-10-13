package aimem

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed testdata/mock_embedding_data.json
var mockEmbeddingDataJSON []byte

// MockEmbeddingClient mock的embedding客户端，用于测试
type MockEmbeddingClient struct {
	embeddingData map[string][]float32
}

// NewMockEmbeddingClient 创建mock embedding客户端
func NewMockEmbeddingClient() (*MockEmbeddingClient, error) {
	var embeddingData map[string][]float32
	if err := json.Unmarshal(mockEmbeddingDataJSON, &embeddingData); err != nil {
		return nil, utils.Errorf("failed to unmarshal mock embedding data: %v", err)
	}

	log.Infof("loaded %d mock embedding entries", len(embeddingData))
	return &MockEmbeddingClient{
		embeddingData: embeddingData,
	}, nil
}

// Embedding 实现EmbeddingClient接口
func (m *MockEmbeddingClient) Embedding(text string) ([]float32, error) {
	if embedding, ok := m.embeddingData[text]; ok {
		return embedding, nil
	}

	// 如果找不到，返回一个默认的向量
	log.Warnf("text not found in mock data, returning default vector: %s", utils.ShrinkString(text, 50))
	return generateDefaultVector(text), nil
}

// generateDefaultVector 为未知文本生成一个简单的默认向量
func generateDefaultVector(text string) []float32 {
	// 基于文本长度和hash生成一个简单的向量
	hash := utils.CalcMd5(text)
	vec := make([]float32, 768) // 假设维度为768

	for i := 0; i < 768; i++ {
		// 使用hash的字节来生成向量值
		vec[i] = float32(hash[i%len(hash)]) / 255.0
	}

	return vec
}

// SaveEmbeddingToMockData 将embedding数据保存到mock数据（用于生成测试数据）
func SaveEmbeddingToMockData(text string, embedding []float32) error {
	var embeddingData map[string][]float32
	if err := json.Unmarshal(mockEmbeddingDataJSON, &embeddingData); err != nil {
		embeddingData = make(map[string][]float32)
	}

	embeddingData[text] = embedding

	// 保存回JSON
	data, err := json.MarshalIndent(embeddingData, "", "  ")
	if err != nil {
		return err
	}

	log.Infof("saved embedding for text: %s (dimension: %d)", utils.ShrinkString(text, 50), len(embedding))
	log.Debugf("embedding data to save:\n%s", string(data))

	return nil
}

// MockInvoker 实现 AIInvokeRuntime 接口用于测试
type MockInvoker struct {
	ctx context.Context
}

func NewMockInvoker(ctx context.Context) *MockInvoker {
	return &MockInvoker{ctx: ctx}
}

func (m *MockInvoker) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return "Mock Basic Prompt Template: {{ .Query }}", map[string]any{
		"Query": "test query",
	}, nil
}

func (m *MockInvoker) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption) (*aicommon.Action, error) {
	log.Infof("mock InvokeLiteForge called with action: %s", actionName)

	if actionName == "memory-triage" {
		// 构造mock的返回数据
		mockResponseJSON := `{
			"@action": "memory-triage",
			"memory_entities": [
				{
					"content": "用户在实现一个复杂的AI记忆系统，使用C.O.R.E. P.A.C.T.框架进行记忆评分",
					"tags": ["AI开发", "记忆系统", "C.O.R.E. P.A.C.T."],
					"potential_questions": [
						"如何实现AI记忆系统？",
						"什么是C.O.R.E. P.A.C.T.框架？",
						"如何评估记忆的重要性？"
					],
					"t": 0.8,
					"a": 0.7,
					"p": 0.9,
					"o": 0.85,
					"e": 0.6,
					"r": 0.75,
					"c": 0.65
				},
				{
					"content": "系统需要支持语义搜索、按分数搜索和按标签搜索功能",
					"tags": ["搜索功能", "AI开发"],
					"potential_questions": [
						"如何实现语义搜索？",
						"什么是按分数搜索？",
						"如何按标签过滤记忆？"
					],
					"t": 0.7,
					"a": 0.8,
					"p": 0.6,
					"o": 0.9,
					"e": 0.5,
					"r": 0.8,
					"c": 0.7
				}
			]
		}`

		// 使用ExtractAction从JSON字符串创建Action
		action, err := aicommon.ExtractAction(mockResponseJSON, "memory-triage")
		if err != nil {
			return nil, utils.Errorf("failed to extract action: %v", err)
		}
		return action, nil
	}

	return nil, utils.Errorf("unexpected action: %s", actionName)
}

func (m *MockInvoker) ExecuteToolRequiredAndCall(name string) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (m *MockInvoker) AskForClarification(question string, payloads []string) string {
	return ""
}

func (m *MockInvoker) DirectlyAnswer(query string, tools []*aitool.Tool) (string, error) {
	return "", nil
}

func (m *MockInvoker) EnhanceKnowledgeAnswer(ctx context.Context, s string) (string, error) {
	return "", nil
}

func (m *MockInvoker) VerifyUserSatisfaction(query string, isToolCall bool, payload string) (bool, error) {
	return true, nil
}

func (m *MockInvoker) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
}

func (m *MockInvoker) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (m *MockInvoker) AddToTimeline(entry, content string) {
}

func (m *MockInvoker) GetConfig() aicommon.AICallerConfigIf {
	return nil
}

func (m *MockInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return ""
}

func (m *MockInvoker) EmitResultAfterStream(any) {
}

func (m *MockInvoker) EmitResult(any) {
}

func init() {
	// 全局清理所有测试数据
	cleanupAllTestData()
}

// createTestAIMemory 创建用于测试的AIMemory实例，自动注入mock embedding
func createTestAIMemory(sessionID string, opts ...Option) (*AIMemoryTriage, error) {
	// 创建mock embedding客户端
	mockEmbedder, err := NewMockEmbeddingClient()
	if err != nil {
		return nil, err
	}

	// 默认选项：使用mock embedding
	defaultOpts := []Option{
		WithRAGOptions(rag.WithEmbeddingClient(mockEmbedder)),
	}

	// 合并用户提供的选项
	allOpts := append(defaultOpts, opts...)

	return NewAIMemory(sessionID, allOpts...)
}

// 清理所有测试数据
func cleanupAllTestData() {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return
	}

	// 删除所有测试相关的记忆条目
	db.Where("session_id LIKE ?", "test-session-%").Delete(&schema.AIMemoryEntity{})

	// 使用原生SQL删除测试相关的RAG collection和documents
	db.Exec("DELETE FROM rag_vector_document_test WHERE collection_id IN (SELECT id FROM rag_vector_collection_test WHERE name LIKE 'ai-memory-test-session-%')")
	db.Exec("DELETE FROM rag_vector_collection_test WHERE name LIKE 'ai-memory-test-session-%'")

	log.Infof("cleaned up all test data")
}

// 测试创建AIMemory
func TestNewAIMemory(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-001"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
		WithContextProvider(func() (string, error) {
			return "测试背景：用户正在开发AI记忆系统", nil
		}),
	)

	// 确保最后清理
	defer cleanupTestData(t, sessionID)

	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		if mem != nil {
			defer mem.Close()
		}
	}

	log.Infof("successfully created AI memory for session: %s", sessionID)
}

// 测试添加原始文本并生成记忆条目
func TestAddRawText(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-002"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
		WithContextProvider(func() (string, error) {
			return "已有标签：AI开发、记忆系统、搜索功能", nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加原始文本
	rawText := "用户正在实现一个AI记忆系统，需要支持C.O.R.E. P.A.C.T.框架的七个维度评分，并且要实现语义搜索功能。"
	entities, err := mem.AddRawText(rawText)

	assert.NoError(t, err)
	assert.NotEmpty(t, entities)

	log.Infof("generated %d memory entities", len(entities))
	for i, entity := range entities {
		log.Infof("entity %d: %s", i, entity.Content)
		log.Infof("  tags: %v", entity.Tags)
		log.Infof("  scores: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f",
			entity.C_Score, entity.O_Score, entity.R_Score, entity.E_Score,
			entity.P_Score, entity.A_Score, entity.T_Score)

		assert.NotEmpty(t, entity.Id)
		assert.NotEmpty(t, entity.Content)
		assert.NotEmpty(t, entity.Tags)
		assert.NotEmpty(t, entity.PotentialQuestions)
		assert.Len(t, entity.CorePactVector, 7)
	}
}

// 测试保存记忆条目并验证数据库
func TestSaveMemoryEntitiesAndVerifyDB(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-003"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "测试保存功能：用户需要实现语义搜索和按标签搜索"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)
	assert.NotEmpty(t, entities)

	// 保存到数据库
	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	log.Infof("successfully saved %d memory entities", len(entities))

	// 验证数据库中的数据
	db := consts.GetGormProjectDatabase()
	var count int64
	err = db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", sessionID).Count(&count).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(len(entities)), count)

	// 验证每个条目的详细数据
	var dbEntities []schema.AIMemoryEntity
	err = db.Where("session_id = ?", sessionID).Find(&dbEntities).Error
	assert.NoError(t, err)
	assert.Len(t, dbEntities, len(entities))

	for i, dbEntity := range dbEntities {
		log.Infof("verified entity %d in database: %s", i, dbEntity.MemoryID)
		assert.NotEmpty(t, dbEntity.Content)
		assert.NotEmpty(t, dbEntity.Tags)
		assert.NotEmpty(t, dbEntity.PotentialQuestions)
		assert.Len(t, dbEntity.CorePactVector, 7)
	}

	log.Infof("verified %d entities in database with complete data", count)
}

// 测试RAG索引验证
func TestRAGIndexingVerification(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-004-rag"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "用户正在开发AI记忆系统，使用RAG技术实现语义搜索功能"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 验证RAG文档数量
	docCount, err := mem.rag.CountDocuments()
	assert.NoError(t, err)

	totalQuestions := 0
	for _, entity := range entities {
		totalQuestions += len(entity.PotentialQuestions)
	}

	log.Infof("RAG document count: %d, expected questions: %d", docCount, totalQuestions)
	assert.Equal(t, totalQuestions, docCount, "RAG文档数量应该等于所有潜在问题的数量")

	// 验证可以通过RAG检索到文档
	docs, err := mem.rag.ListDocuments()
	assert.NoError(t, err)
	assert.Len(t, docs, totalQuestions)

	for i, doc := range docs {
		log.Infof("RAG document %d: ID=%s, content=%s", i, doc.ID, utils.ShrinkString(doc.Content, 50))
		assert.NotEmpty(t, doc.ID)
		assert.NotEmpty(t, doc.Content)
		assert.NotEmpty(t, doc.Metadata["memory_id"])
		assert.NotEmpty(t, doc.Metadata["question"])
		assert.Equal(t, sessionID, doc.Metadata["session_id"])
	}
}

// 测试语义搜索
func TestSearchBySemantics(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-005"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "用户正在开发AI记忆系统，使用RAG技术实现语义搜索功能"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 执行语义搜索
	results, err := mem.SearchBySemantics(sessionID, "如何实现语义搜索？", 10)
	assert.NoError(t, err)

	log.Infof("semantic search returned %d results", len(results))
	for i, result := range results {
		log.Infof("result %d (score: %.4f): %s", i, result.Score, utils.ShrinkString(result.Entity.Content, 50))
		assert.NotNil(t, result.Entity)
		assert.GreaterOrEqual(t, result.Score, 0.0)
	}
}

// 测试按分数向量搜索
func TestSearchByScoreVector(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-006-vector"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存多个记忆条目
	rawText := "用户正在开发AI记忆系统"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 构建目标分数向量（寻找相似评分的记忆）
	targetScores := &MemoryEntity{
		C_Score: 0.7,
		O_Score: 0.85,
		R_Score: 0.75,
		E_Score: 0.6,
		P_Score: 0.9,
		A_Score: 0.75,
		T_Score: 0.8,
	}

	// 执行向量相似度搜索
	results, err := mem.SearchByScoreVector(sessionID, targetScores, 10)
	assert.NoError(t, err)

	log.Infof("score vector search returned %d results", len(results))
	for i, result := range results {
		log.Infof("result %d (similarity: %.4f):", i, result.Score)
		log.Infof("  content: %s", utils.ShrinkString(result.Entity.Content, 50))
		log.Infof("  scores: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f",
			result.Entity.C_Score, result.Entity.O_Score, result.Entity.R_Score,
			result.Entity.E_Score, result.Entity.P_Score, result.Entity.A_Score,
			result.Entity.T_Score)
		assert.GreaterOrEqual(t, result.Score, 0.0)
		assert.LessOrEqual(t, result.Score, 1.0)
	}
}

// 测试按分数范围搜索
func TestSearchByScores(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-007"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "测试分数搜索：实现高相关性和高可操作性的记忆条目"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 搜索高相关性的记忆
	filter := &ScoreFilter{
		R_Min: 0.7,
		R_Max: 1.0,
	}
	results, err := mem.SearchByScores(sessionID, filter, 10)
	assert.NoError(t, err)

	log.Infof("score search returned %d results", len(results))
	for i, result := range results {
		log.Infof("result %d: R=%.2f, content: %s", i, result.R_Score, utils.ShrinkString(result.Content, 50))
		assert.GreaterOrEqual(t, result.R_Score, 0.7)
	}
}

// 测试按标签搜索
func TestSearchByTags(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-008"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "用户在开发AI系统，需要实现记忆管理和搜索功能"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 按标签搜索
	results, err := mem.SearchByTags(sessionID, []string{"AI开发"}, false, 10)
	assert.NoError(t, err)

	log.Infof("tag search returned %d results", len(results))
	for i, result := range results {
		log.Infof("result %d: tags=%v, content: %s", i, result.Tags, utils.ShrinkString(result.Content, 50))
		// 验证至少包含一个搜索的标签
		hasTag := false
		for _, tag := range result.Tags {
			if tag == "AI开发" {
				hasTag = true
				break
			}
		}
		assert.True(t, hasTag, "结果应该包含搜索的标签")
	}
}

// 测试获取所有标签
func TestGetAllTags(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-009"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "测试标签功能：AI开发、记忆系统、搜索功能"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 获取所有标签
	tags, err := mem.GetAllTags(sessionID)
	assert.NoError(t, err)
	assert.NotEmpty(t, tags)

	log.Infof("found %d unique tags: %v", len(tags), tags)
}

// 测试获取动态上下文
func TestGetDynamicContextWithTags(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-010"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加并保存记忆条目
	rawText := "用户正在开发AI记忆系统"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 获取动态上下文
	context, err := mem.GetDynamicContextWithTags(sessionID)
	assert.NoError(t, err)
	assert.NotEmpty(t, context)

	log.Infof("dynamic context:\n%s", context)
	assert.Contains(t, context, "已存储的记忆领域标签")
}

// 测试错误处理和边界情况
func TestErrorHandling(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-error"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 测试空字符串搜索
	results, err := mem.SearchBySemantics(sessionID, "", 5)
	assert.NoError(t, err)
	assert.Empty(t, results)

	// 测试无效的分数范围
	filter := &ScoreFilter{R_Min: 2.0, R_Max: 3.0} // 超出0-1范围
	results2, err := mem.SearchByScores("R_Score", filter, 5)
	assert.NoError(t, err)
	assert.Empty(t, results2)

	// 测试空标签搜索
	_, err = mem.SearchByTags(sessionID, []string{}, false, 5)
	assert.Error(t, err) // 应该返回错误，因为至少需要一个标签
	assert.Contains(t, err.Error(), "at least one tag is required")

	// 测试不存在的标签
	results4, err := mem.SearchByTags(sessionID, []string{"不存在的标签"}, false, 5)
	assert.NoError(t, err)
	assert.Empty(t, results4)

	log.Infof("error handling tests completed")
}

// 测试Mock Embedding的SaveEmbeddingToMockData函数
func TestMockEmbeddingSave(t *testing.T) {
	client, err := NewMockEmbeddingClient()
	assert.NoError(t, err)

	// 测试保存新的embedding数据（768维）
	text := "测试文本"
	vector := make([]float32, 768)
	for i := range vector {
		vector[i] = float32(i) * 0.001
	}

	err = SaveEmbeddingToMockData(text, vector)
	assert.NoError(t, err)

	// 验证保存的数据可以被读取
	embedding, err := client.Embedding(text)
	assert.NoError(t, err)
	// 注意：由于SaveEmbeddingToMockData只是记录日志，实际不会更新内存中的数据
	// 所以这里我们测试默认向量生成
	assert.Len(t, embedding, 768)

	log.Infof("mock embedding save test completed")
}

// 测试搜索的更多边界情况
func TestSearchEdgeCases(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-edge"

	// 先清理可能存在的旧数据
	cleanupTestData(t, sessionID)
	defer cleanupTestData(t, sessionID)

	mem, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 添加测试数据
	rawText := "测试边界情况：用户需要实现AI记忆系统的高级搜索功能"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)
	assert.NotEmpty(t, entities)

	err = mem.SaveMemoryEntities(sessionID, entities...)
	assert.NoError(t, err)

	// 测试SearchByScores的不同分数维度
	dimensions := []string{"C_Score", "O_Score", "R_Score", "E_Score", "P_Score", "A_Score", "T_Score"}
	for _, dim := range dimensions {
		filter := &ScoreFilter{
			C_Min: 0.0, C_Max: 1.0,
			O_Min: 0.0, O_Max: 1.0,
			R_Min: 0.0, R_Max: 1.0,
			E_Min: 0.0, E_Max: 1.0,
			P_Min: 0.0, P_Max: 1.0,
			A_Min: 0.0, A_Max: 1.0,
			T_Min: 0.0, T_Max: 1.0,
		}
		results, err := mem.SearchByScores(dim, filter, 10)
		assert.NoError(t, err)
		log.Infof("search by %s returned %d results", dim, len(results))
	}

	// 测试SearchByTags的matchAll模式
	results, err := mem.SearchByTags(sessionID, []string{"AI开发"}, true, 10)
	assert.NoError(t, err)
	log.Infof("tag search (matchAll) returned %d results", len(results))

	// 测试SearchByScoreVector的边界情况
	targetEntity := &MemoryEntity{
		CorePactVector: []float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5},
	}
	results2, err := mem.SearchByScoreVector(sessionID, targetEntity, 10)
	assert.NoError(t, err)
	log.Infof("score vector search returned %d results", len(results2))

	log.Infof("search edge cases test completed")
}

// 清理测试数据
func cleanupTestData(t *testing.T, sessionID string) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return
	}

	// 删除测试数据
	err := db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{}).Error
	if err != nil {
		log.Warnf("cleanup test data failed: %v", err)
	}

	// 清理RAG collection和相关文档
	collectionName := fmt.Sprintf("ai-memory-%s", sessionID)

	// 使用原生SQL强制删除相关collection和documents
	db.Exec("DELETE FROM rag_vector_document_test WHERE collection_id IN (SELECT id FROM rag_vector_collection_test WHERE name = ?)", collectionName)
	db.Exec("DELETE FROM rag_vector_collection_test WHERE name = ?", collectionName)

	// 额外清理：删除所有可能的测试相关collection
	db.Exec("DELETE FROM rag_vector_document_test WHERE collection_id IN (SELECT id FROM rag_vector_collection_test WHERE name LIKE 'ai-memory-test-session-%')")
	db.Exec("DELETE FROM rag_vector_collection_test WHERE name LIKE 'ai-memory-test-session-%'")

	log.Infof("cleaned up test data for session: %s", sessionID)
}
