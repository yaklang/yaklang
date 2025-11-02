package knowledgebase

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// testEmbedder 测试用的嵌入器函数
func testEmbedder(text string) ([]float32, error) {
	// 简单地生成一个固定的向量作为嵌入
	// 根据文本内容生成不同的向量
	if len(text) > 10 {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if len(text) > 5 {
		return []float32{0.0, 1.0, 0.0}, nil
	}
	return []float32{0.0, 0.0, 1.0}, nil
}

// TestNewKnowledgeBase 测试创建知识库（包含KnowledgeBaseInfo）
func TestNewKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := vectorstore.NewVectorStoreDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库，使用 mock 嵌入器
	kb, err := NewKnowledgeBase(
		db,
		"test-kb-with-info",
		"测试知识库",
		"test",
		vectorstore.WithEmbeddingModel("mock-model"),
		vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 验证 KnowledgeBaseInfo 被创建
	kbInfo, err := LoadKnowledgeBase(db, "test-kb-with-info")
	assert.NoError(t, err)
	info, err := kbInfo.GetInfo()
	assert.NoError(t, err)
	assert.Equal(t, "test-kb-with-info", info.KnowledgeBaseName)
	assert.Equal(t, "测试知识库", info.KnowledgeBaseDescription)
	assert.Equal(t, "test", info.KnowledgeBaseType)

	// 验证 RAG Collection 被创建
	assert.True(t, vectorstore.HasCollection(db, "test-kb-with-info"))

	// 再次调用 NewKnowledgeBase，应该直接加载而不创建新的
	kb2, err := NewKnowledgeBase(
		db,
		"test-kb-with-info",
		"不应该被使用的描述",
		"不应该被使用的类型",
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb2)

	// 验证数据库中的信息没有被更新
	kbInfo2, err := LoadKnowledgeBase(db, "test-kb-with-info")
	assert.NoError(t, err)
	info2, err := kbInfo2.GetInfo()
	assert.NoError(t, err)
	assert.Equal(t, "测试知识库", info2.KnowledgeBaseDescription) // 应该还是原来的描述
}

// TestCreateKnowledgeBase 测试创建全新知识库
func TestCreateKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := vectorstore.NewVectorStoreDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建全新知识库
	kb, err := CreateKnowledgeBase(
		db,
		"new-kb",
		"全新知识库",
		"fresh",
		vectorstore.WithEmbeddingModel("mock-model"),
		vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 验证创建成功
	kbInfo, err := LoadKnowledgeBase(db, "new-kb")
	assert.NoError(t, err)
	assert.Equal(t, "new-kb", kbInfo.name)

	// 再次尝试创建同名知识库，应该失败
	kb2, err := CreateKnowledgeBase(
		db,
		"new-kb",
		"重复知识库",
		"duplicate",
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.Error(t, err)
	assert.Nil(t, kb2)
	assert.Contains(t, err.Error(), "已存在")
}

// TestLoadKnowledgeBase 测试加载知识库
func TestLoadKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := vectorstore.NewVectorStoreDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 先创建一个知识库
	kb1, err := NewKnowledgeBase(
		db,
		"load-test-kb",
		"加载测试知识库",
		"load-test",
		vectorstore.WithEmbeddingModel("mock-model"),
		vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb1)

	// 加载已存在的知识库
	kb2, err := LoadKnowledgeBase(
		db,
		"load-test-kb",
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb2)
	assert.Equal(t, "load-test-kb", kb2.GetName())

	// 尝试加载不存在的知识库
	kb3, err := LoadKnowledgeBase(db, "non-existent-kb")
	assert.Error(t, err)
	assert.Nil(t, kb3)
	assert.Contains(t, err.Error(), "不存在")
}

// TestKnowledgeBaseOperations 测试知识库的基本操作
func TestKnowledgeBaseOperations(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := vectorstore.NewVectorStoreDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kb, err := NewKnowledgeBase(
		db,
		"ops-test-kb",
		"操作测试知识库",
		"ops-test",
		vectorstore.WithEmbeddingModel("mock-model"),
		vectorstore.WithModelDimension(3),
		vectorstore.WithEmbeddingClient(vectorstore.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)

	// 添加知识条目
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    1, // 假设这是第一个知识库
		KnowledgeTitle:     "测试知识条目",
		KnowledgeType:      "Test",
		ImportanceScore:    8,
		Keywords:           []string{"测试", "知识库"},
		KnowledgeDetails:   "这是一个测试知识条目的详细内容",
		Summary:            "测试条目摘要",
		SourcePage:         1,
		PotentialQuestions: []string{"什么是测试?", "如何使用知识库?"},
	}

	err = kb.AddKnowledgeEntry(entry)
	assert.NoError(t, err)

	// 搜索知识条目
	results, err := kb.SearchKnowledgeEntries("测试", 5)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.Equal(t, "测试知识条目", results[0].KnowledgeTitle)

	// 获取知识条目列表
	entries, err := kb.ListKnowledgeEntries("", 1, 10)
	assert.NoError(t, err)
	assert.True(t, len(entries) > 0)

	// 获取同步状态
	status, err := kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.True(t, status.InSync)
	assert.Equal(t, 1, status.DatabaseEntries)
	assert.Equal(t, 1, status.RAGDocuments)
}

// 测试添加一个超大文档并查询
func TestAddLargeDocument(t *testing.T) {
	// 创建临时数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	db.AutoMigrate(&schema.KnowledgeBaseEntry{}, &schema.KnowledgeBaseInfo{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kb, err := NewKnowledgeBase(
		db,
		"large-doc-kb",
		"超大文档知识库",
		"large-doc",
		vectorstore.WithEmbeddingModel("mock-model"),
		vectorstore.WithModelDimension(3),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 添加一个超大文档
	doc := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    1,
		KnowledgeTitle:     "超大文档",
		KnowledgeType:      "Test",
		ImportanceScore:    8,
		Keywords:           []string{"超大文档"},
		KnowledgeDetails:   "\\n为什么检索增强生成很重要？\\nLLM 是一项关键的人工智能（AI）技术，为智能聊天机器人和其他自然语言处理（NLP）应用程序提供支持。目标创建能够是通过交叉引用权威知识来源，在各种环境中回答用户问题的机器人。不幸的是，LLM 技术的性质给 LLM 响应带来了不可预测性。此外，LLM 训练数据是静态的，从而为其掌握的知识限定了截止日期。\\n\\nLLM 面临的已知挑战包括：\\n\\n在没有答案的情况下提供虚假信息。\\n在用户需要具体的最新响应时，提供过时或宽泛的信息。\\n依据非权威来源创建响应。\\n由于术语混淆，不同的培训来源使用相同的术语来谈论不同的事情，因此会产生不准确的响应。\\n您可以将大语言模型看作是一个过于热情的新员工，他拒绝随时了解时事，但总是会绝对自信地回答每一个问题。不幸的是，这种态度会对用户的信任产生负面影响，这是您不希望聊天机器人效仿的！\\n\\nRAG 是解决其中一些挑战的一种方法。它会重定向 LLM，从权威的、预先确定的知识来源中检索相关信息。组织可以更好地控制生成的文本输出，并且用户可以深入了解 LLM 如何生成响应。\\n\\n检索增强生成有哪些好处？\\nRAG 技术为组织的生成式人工智能工作带来了多项好处。\\n\\n经济高效的实施\\n聊天机器人开发通常从基础模型开始。基础模型（FM）是在广泛的广义和未标记数据上训练的 API 可访问 LLM。针对组织或领域特定信息重新训练基础模型的计算和财务成本很高。RAG 是一种将新数据引入 LLM 的更加经济高效的方法。它使生成式人工智能技术更广泛地得以获取和使用。\\n\\n当前信息\\n即使 LLM 的原始训练数据来源适合您的需求，但保持相关性也具有挑战性。RAG 允许开发人员为生成模型提供最新的研究、统计数据或新闻。他们可以使用 RAG 将 LLM 直接连接到实时社交媒体提要、新闻网站或其他经常更新的信息来源。LLM 随即可以向用户提供最新信息。\\n\\n增强用户信任度\\nRAG 允许 LLM 通过来源归属来呈现准确的信息。输出可以包括对来源的引文或引用。如果需要进一步说明或更详细的信息，用户也可以自己查找源文档。",
		Summary:            "超大文档摘要",
		SourcePage:         1,
		PotentialQuestions: []string{"什么是超大文档?", "如何使用超大文档?"},
	}

	err = kb.AddKnowledgeEntry(doc)
	assert.NoError(t, err)

	doc2 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    1,
		KnowledgeTitle:     "超大文档2",
		KnowledgeType:      "Test",
		ImportanceScore:    8,
		Keywords:           []string{"超大文档2"},
		KnowledgeDetails:   "\\n为什么检索增强生成很重要？\\nLLM 是一项关键的人工智能（AI）技术，为智能聊天机器人和其他自然语言处理（NLP）应用程序提供支持。目标创建能够是通过交叉引用权威知识来源，在各种环境中回答用户问题的机器人。不幸的是，LLM 技术的性质给 LLM 响应带来了不可预测性。此外，LLM 训练数据是静态的，从而为其掌握的知识限定了截止日期。\\n\\nLLM 面临的已知挑战包括：\\n\\n在没有答案的情况下提供虚假信息。\\n在用户需要具体的最新响应时，提供过时或宽泛的信息。\\n依据非权威来源创建响应。\\n由于术语混淆，不同的培训来源使用相同的术语来谈论不同的事情，因此会产生不准确的响应。\\n您可以将大语言模型看作是一个过于热情的新员工，他拒绝随时了解时事，但总是会绝对自信地回答每一个问题。不幸的是，这种态度会对用户的信任产生负面影响，这是您不希望聊天机器人效仿的！\\n\\nRAG 是解决其中一些挑战的一种方法。它会重定向 LLM，从权威的、预先确定的知识来源中检索相关信息。组织可以更好地控制生成的文本输出，并且用户可以深入了解 LLM 如何生成响应。\\n\\n检索增强生成有哪些好处？\\nRAG 技术为组织的生成式人工智能工作带来了多项好处。\\n\\n经济高效的实施\\n聊天机器人开发通常从基础模型开始。基础模型（FM）是在广泛的广义和未标记数据上训练的 API 可访问 LLM。针对组织或领域特定信息重新训练基础模型的计算和财务成本很高。RAG 是一种将新数据引入 LLM 的更加经济高效的方法。它使生成式人工智能技术更广泛地得以获取和使用。\\n\\n当前信息\\n即使 LLM 的原始训练数据来源适合您的需求，但保持相关性也具有挑战性。RAG 允许开发人员为生成模型提供最新的研究、统计数据或新闻。他们可以使用 RAG 将 LLM 直接连接到实时社交媒体提要、新闻网站或其他经常更新的信息来源。LLM 随即可以向用户提供最新信息。\\n\\n增强用户信任度\\nRAG 允许 LLM 通过来源归属来呈现准确的信息。输出可以包括对来源的引文或引用。如果需要进一步说明或更详细的信息，用户也可以自己查找源文档。",
		Summary:            "超大文档2摘要",
		SourcePage:         1,
		PotentialQuestions: []string{"什么是超大文档2?", "如何使用超大文档2?"},
	}

	err = kb.AddKnowledgeEntry(doc2)
	assert.NoError(t, err)

	results, err := kb.SearchKnowledgeEntries("检索增强", 2)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
}
