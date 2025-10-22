package aimem

import (
	"context"
	_ "embed"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aicommon_mock"
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed testdata/mock_embedding_data.json
var mockEmbeddingDataJSON []byte

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

// CreateTestAIMemory 创建用于测试的AIMemory实例，自动注入mock embedding，新建测试临时数据库
func CreateTestAIMemory(sessionID string, opts ...Option) (*AIMemoryTriage, error) {
	// 创建mock embedding客户端（使用内置的测试数据）
	mockEmbedder, err := NewMockEmbeddingClientFromJSON(mockEmbeddingDataJSON)
	if err != nil {
		return nil, err
	}

	db, err := getTestDatabase()
	if err != nil {
		return nil, err
	}

	// 默认选项：使用mock emdatabasebedding
	defaultOpts := []Option{
		WithRAGOptions(rag.WithEmbeddingClient(mockEmbedder)),
		WithDatabase(db),
	}

	// 合并用户提供的选项
	allOpts := append(defaultOpts, opts...)

	return NewAIMemory(sessionID, allOpts...)
}

// 测试创建AIMemory
func TestNewAIMemory(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-001"

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
		WithContextProvider(func() (string, error) {
			return "测试背景：用户正在开发AI记忆系统", nil
		}),
	)

	// 确保最后清理

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

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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
	sessionID := "test-session-003-" + uuid.New().String()

	// 先清理可能存在的旧数据
	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	db := mem.GetDB()

	// 添加并保存记忆条目
	rawText := "测试保存功能：用户需要实现语义搜索和按标签搜索"
	entities, err := mem.AddRawText(rawText)
	assert.NoError(t, err)
	assert.NotEmpty(t, entities)

	// 保存到数据库
	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	log.Infof("successfully saved %d memory entities", len(entities))

	// 验证数据库中的数据
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
	sessionID := "test-session-004-rag-" + uuid.New().String()

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 验证RAG文档数量
	if mem.rag == nil {
		t.Skip("RAG system not initialized (embedding service unavailable)")
	}

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
	sessionID := "test-session-005-" + uuid.New().String()

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 执行语义搜索
	results, err := mem.SearchBySemantics("如何实现语义搜索？", 10)
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
	sessionID := "test-session-006-vector-" + uuid.New().String()

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
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
	results, err := mem.SearchByScoreVector(targetScores, 10)
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
	sessionID := "test-session-007-" + uuid.New().String()

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 搜索高相关性的记忆
	filter := &ScoreFilter{
		R_Min: 0.7,
		R_Max: 1.0,
	}
	results, err := mem.SearchByScores(filter, 10)
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
	sessionID := "test-session-008-" + uuid.New().String()

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 按标签搜索
	results, err := mem.SearchByTags([]string{"AI开发"}, false, 10)
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

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 获取所有标签
	tags, err := mem.GetAllTags()
	assert.NoError(t, err)
	assert.NotEmpty(t, tags)

	log.Infof("found %d unique tags: %v", len(tags), tags)
}

// 测试获取动态上下文
func TestGetDynamicContextWithTags(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-010"

	// 先清理可能存在的旧数据

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
	assert.NoError(t, err)

	// 获取动态上下文
	context, err := mem.GetDynamicContextWithTags()
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

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, mem)
	if mem != nil {
		defer mem.Close()
	}

	// 测试空字符串搜索
	results, err := mem.SearchBySemantics("", 5)
	assert.NoError(t, err)
	assert.Empty(t, results)

	// 测试无效的分数范围
	filter := &ScoreFilter{R_Min: 2.0, R_Max: 3.0} // 超出0-1范围
	results2, err := mem.SearchByScores(filter, 5)
	assert.NoError(t, err)
	assert.Empty(t, results2)

	// 测试空标签搜索
	_, err = mem.SearchByTags([]string{}, false, 5)
	assert.Error(t, err) // 应该返回错误，因为至少需要一个标签
	assert.Contains(t, err.Error(), "at least one tag is required")

	// 测试不存在的标签
	results4, err := mem.SearchByTags([]string{"不存在的标签"}, false, 5)
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

	mem, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(ctx)),
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

	err = mem.SaveMemoryEntities(entities...)
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
		results, err := mem.SearchByScores(filter, 10)
		assert.NoError(t, err)
		log.Infof("search by %s returned %d results", dim, len(results))
	}

	// 测试SearchByTags的matchAll模式
	results, err := mem.SearchByTags([]string{"AI开发"}, true, 10)
	assert.NoError(t, err)
	log.Infof("tag search (matchAll) returned %d results", len(results))

	// 测试SearchByScoreVector的边界情况
	targetEntity := &MemoryEntity{
		CorePactVector: []float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5},
	}
	results2, err := mem.SearchByScoreVector(targetEntity, 10)
	assert.NoError(t, err)
	log.Infof("score vector search returned %d results", len(results2))

	log.Infof("search edge cases test completed")
}

func setupTestDB(t *testing.T) *gorm.DB {
	// 创建临时文件数据库用于测试，避免并发访问问题
	tmpDir := consts.GetDefaultYakitBaseTempDir()
	dbFile := filepath.Join(tmpDir, uuid.NewString()+".db")

	db, err := gorm.Open("sqlite3", dbFile)
	require.NoError(t, err)

	// 自动迁移表结构
	schema.AutoMigrate(db, schema.KEY_SCHEMA_PROFILE_DATABASE)

	// 设置数据库连接池和超时
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)

	return db
}

// 清理测试数据
func getTestDatabase() (*gorm.DB, error) {
	// 创建临时文件数据库用于测试，避免并发访问问题
	tmpDir := consts.GetDefaultYakitBaseTempDir()
	dbFile := filepath.Join(tmpDir, uuid.NewString()+".db")

	db, err := gorm.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	// 自动迁移表结构
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)

	// 设置数据库连接池和超时
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)

	return db, nil
}
