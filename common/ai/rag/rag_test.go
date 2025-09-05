package rag

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func testEmbedder(text string) ([]float32, error) {
	// 简单地生成一个固定的向量作为嵌入
	// 在实际测试中，我们可以根据文本内容生成不同的向量
	if text == "Yaklang介绍" || text == "什么是Yaklang" || text == "Yaklang是一种安全研究编程语言" {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if text == "RAG介绍" || text == "什么是RAG" || text == "RAG是一种结合检索和生成的AI技术" {
		return []float32{0.0, 1.0, 0.0}, nil
	} else if text == "AI技术" {
		return []float32{0.5, 0.5, 0.0}, nil
	}
	return []float32{0.0, 0.0, 0.0}, nil
}

// 测试文本分块功能
func TestMUSTPASS_ChunkText(t *testing.T) {
	// 测试中文文本
	chineseText := "这是一个测试文本，用于测试文本分块功能。我们需要确保它可以正确地分割成多个块，每个块的大小应该在指定范围内。这样可以更好地处理中文字符。"

	// 测试基本分块（按rune计算）
	chunks := ChunkText(chineseText, 20, 0)
	assert.True(t, len(chunks) > 1, "Should split into multiple chunks")

	// 验证每个块的长度（rune计算）
	for i, chunk := range chunks {
		runeCount := len([]rune(chunk))
		t.Logf("Chunk %d (length: %d runes): %s", i, runeCount, chunk)
		if i < len(chunks)-1 { // 除了最后一个块
			assert.True(t, runeCount <= 25, "Chunk should not exceed max size + boundary adjustment")
		}
	}

	// 测试有重叠的情况
	chunksWithOverlap := ChunkText(chineseText, 20, 5)
	assert.True(t, len(chunksWithOverlap) >= len(chunks), "With overlap should have same or more chunks")

	// 测试英文文本
	englishText := "This is a test text for testing text chunking functionality. We need to ensure it can properly split into multiple chunks."
	englishChunks := ChunkText(englishText, 30, 0)
	assert.True(t, len(englishChunks) > 1, "English text should also split")

	// 测试混合文本（中英文）
	mixedText := "This is English. 这是中文。Mixed text testing for 文本分块功能。"
	mixedChunks := ChunkText(mixedText, 20, 0)
	for i, chunk := range mixedChunks {
		t.Logf("Mixed chunk %d: %s", i, chunk)
		assert.True(t, len([]rune(chunk)) > 0, "Chunk should not be empty")
	}

	// 测试单块情况
	shortText := "短文本"
	singleChunk := ChunkText(shortText, 100, 0)
	assert.Equal(t, 1, len(singleChunk), "Short text should remain as single chunk")
	assert.Equal(t, shortText, singleChunk[0], "Single chunk should match original text")

	// 测试边界情况
	emptyText := ""
	emptyChunks := ChunkText(emptyText, 100, 0)
	assert.Equal(t, 0, len(emptyChunks), "Empty text should return no chunks")

	// 测试标点符号分割
	punctuationText := "第一句话。第二句话！第三句话？第四句话；第五句话，第六句话。"
	punctChunks := ChunkText(punctuationText, 8, 0)
	for i, chunk := range punctChunks {
		t.Logf("Punctuation chunk %d: %s", i, chunk)
	}
	// 应该在标点符号处合理分割
	assert.True(t, len(punctChunks) > 1, "Should split at punctuation marks")
}

// 测试内存向量存储
func TestMUSTPASS_MemoryVectorStore(t *testing.T) {
	// 创建模拟嵌入器
	mockEmbed := NewMockEmbedder(testEmbedder)

	// 创建内存向量存储
	store := NewMemoryVectorStore(mockEmbed)

	// 准备测试文档
	docs := []Document{
		{
			ID:        "doc1",
			Content:   "Yaklang是一种安全研究编程语言",
			Metadata:  map[string]any{"source": "Yaklang介绍"},
			Embedding: []float32{1.0, 0.0, 0.0},
		},
		{
			ID:        "doc2",
			Content:   "RAG是一种结合检索和生成的AI技术",
			Metadata:  map[string]any{"source": "RAG介绍"},
			Embedding: []float32{0.0, 1.0, 0.0},
		},
	}

	// 添加文档
	err := store.Add(docs...)
	assert.NoError(t, err)

	// 测试计数
	count, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// 测试获取特定文档
	doc, exists, err := store.Get("doc1")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "Yaklang是一种安全研究编程语言", doc.Content)

	// 测试搜索
	results, err := store.Search("什么是Yaklang", 1, 5)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "doc1", results[0].Document.ID)     // 第一个结果应该是Yaklang文档
	assert.True(t, results[0].Score > results[1].Score) // Yaklang文档的相似度应该更高

	// 测试删除
	err = store.Delete("doc1")
	assert.NoError(t, err)

	count, err = store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

// 测试RAG系统
func TestMUSTPASS_RAGSystem(t *testing.T) {
	// 创建模拟嵌入器
	mockEmbed := NewMockEmbedder(testEmbedder)

	// 创建内存向量存储
	store := NewMemoryVectorStore(mockEmbed)

	// 创建RAG系统
	ragSystem := NewRAGSystem(mockEmbed, store)

	// 准备测试文档
	docs := []Document{
		{
			ID:       "doc1",
			Content:  "Yaklang是一种安全研究编程语言",
			Metadata: map[string]any{"source": "Yaklang介绍"},
		},
		{
			ID:       "doc2",
			Content:  "RAG是一种结合检索和生成的AI技术",
			Metadata: map[string]any{"source": "RAG介绍"},
		},
	}

	// 添加文档到RAG系统
	err := ragSystem.addDocuments(docs...)
	assert.NoError(t, err)

	// 测试查询
	results, err := ragSystem.QueryWithPage("什么是RAG", 1, 5)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "doc2", results[0].Document.ID) // 第一个结果应该是RAG文档

	// 测试生成提示
	prompt := FormatRagPrompt("什么是RAG?", results, "")
	assert.Contains(t, prompt, "RAG是一种结合检索和生成的AI技术")
	assert.Contains(t, prompt, "问题: 什么是RAG?")
}

// 测试TextToDocuments
func TestMUSTPASS_TextToDocuments(t *testing.T) {
	text := "这是一个长文本 需要被分割成多个文档 这样我们可以测试文本到文档的转换功能"
	metadata := map[string]any{"source": "测试文档"}

	docs := TextToDocuments(text, 2, 0, metadata)

	assert.True(t, len(docs) > 1)
	for _, doc := range docs {
		assert.NotEmpty(t, doc.ID)
		assert.NotEmpty(t, doc.Content)
		assert.Equal(t, "测试文档", doc.Metadata["source"])
		assert.Contains(t, doc.Metadata, "chunk_index")
		assert.Contains(t, doc.Metadata, "total_chunks")
		assert.Contains(t, doc.Metadata, "created_at")
	}
}

// 测试FilterResults
func TestMUSTPASS_FilterResults(t *testing.T) {
	results := []SearchResult{
		{Score: 0.9},
		{Score: 0.7},
		{Score: 0.5},
		{Score: 0.3},
	}

	filtered := FilterResults(results, 0.6)
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, 0.9, filtered[0].Score)
	assert.Equal(t, 0.7, filtered[1].Score)
}

type VectorStoreDocument struct {
	// 文档唯一标识符，在整个系统中唯一
	DocumentID string `gorm:"uniqueIndex;not null" json:"document_id"`
}

func TestMUSTPASS_AutoAutomigrateVectorStoreDocument(t *testing.T) {
	t.Skip()
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	db.AutoMigrate(&VectorStoreDocument{})

	err = autoAutomigrateVectorStoreDocument(db)
	assert.NoError(t, err)

	// 测试添加两个id相同的文档
	err = db.Create(&VectorStoreDocument{DocumentID: "test"}).Error
	assert.NoError(t, err)

	err = db.Create(&VectorStoreDocument{DocumentID: "test"}).Error
	assert.NoError(t, err)
}

func TestRequestLocalEmbedding(t *testing.T) {
	// 创建本地模型嵌入客户端，连接到127.0.0.1:11435
	client := NewLocalModelEmbedding(nil, "127.0.0.1:11435")

	text1 := "The capital of China is Beijing."
	text2 := "Gravity is a force that attracts two bodies towards each other. It gives weight to physical objects and is responsible for the movement of planets around the sun."
	text3 := "What is the capital of China?"

	// 获取三个文本的嵌入向量
	embedding1, err := client.Embedding(text1)
	if err != nil {
		t.Fatalf("Failed to get embedding for text1: %v", err)
	}
	log.Infof("Text1 embedding dimension: %d", len(embedding1))

	embedding2, err := client.Embedding(text2)
	if err != nil {
		t.Fatalf("Failed to get embedding for text2: %v", err)
	}
	log.Infof("Text2 embedding dimension: %d", len(embedding2))

	embedding3, err := client.Embedding(text3)
	if err != nil {
		t.Fatalf("Failed to get embedding for text3: %v", err)
	}
	log.Infof("Text3 embedding dimension: %d", len(embedding3))

	// 使用hnsw.CosineDistance计算余弦距离
	// 计算text3与text1的距离
	distance3to1 := hnsw.CosineDistance(func() []float32 {
		return embedding3
	}, func() []float32 {
		return embedding1
	})

	// 计算text3与text2的距离
	distance3to2 := hnsw.CosineDistance(func() []float32 {
		return embedding3
	}, func() []float32 {
		return embedding2
	})

	// 输出结果
	log.Infof("Text1: %s", text1)
	log.Infof("Text2: %s", text2)
	log.Infof("Text3: %s", text3)
	log.Infof("Text1 embedding equal Text2: %t", codec.EncodeBase64(embedding1) == codec.EncodeBase64(embedding2))
	log.Infof("Distance between Text3 and Text1: %.6f", distance3to1)
	log.Infof("Distance between Text3 and Text2: %.6f", distance3to2)

	// 验证距离值在合理范围内（0-2之间）
	assert.True(t, distance3to1 >= 0 && distance3to1 <= 2, "Distance3to1 should be between 0 and 2")
	assert.True(t, distance3to2 >= 0 && distance3to2 <= 2, "Distance3to2 should be between 0 and 2")

	// 由于Text3 "Visionary AI Suite" 在Text2中出现，理论上distance3to2应该小于distance3to1
	log.Infof("Text3 should be more similar to Text2 (contains 'Visionary AI Suite') than to Text1")
	if distance3to2 < distance3to1 {
		log.Infof("✓ Confirmed: Text3 is closer to Text2 (distance: %.6f) than to Text1 (distance: %.6f)", distance3to2, distance3to1)
	} else {
		log.Infof("Note: Text3 is closer to Text1 (distance: %.6f) than to Text2 (distance: %.6f)", distance3to1, distance3to2)
	}
}

// 测试BigTextPlan功能
func TestMUSTPASS_BigTextPlan(t *testing.T) {
	// 创建模拟嵌入器，模拟文本过长时的错误
	mockEmbed := &MockEmbedder{
		MockEmbedderFunc: func(text string) ([]float32, error) {
			// 模拟长文本会失败，短文本成功
			if len([]rune(text)) > 50 {
				return nil, fmt.Errorf("text too long: %d runes,%w", len([]rune(text)), embedding.ErrInputTooLarge)
			}
			// 为短文本生成简单的嵌入向量
			return []float32{float32(len(text)), 0.5, 0.1}, nil
		},
	}

	// 创建内存向量存储
	store := NewMemoryVectorStore(mockEmbed)

	// 创建RAG系统
	ragSystem := NewRAGSystem(mockEmbed, store)

	// 准备一个长文本（会触发BigTextPlan）
	longText := "这是一个非常长的测试文本，用于测试BigTextPlan功能。" +
		"这个文本故意写得很长，以便触发嵌入生成失败的情况。" +
		"在这种情况下，系统应该自动使用BigTextPlan来处理这个文档。" +
		"我们将测试两种不同的策略：chunkText和chunkTextAndAvgPooling。"

	// 测试chunkText策略
	t.Run("ChunkText Strategy", func(t *testing.T) {
		ragSystem.SetBigTextPlan(BigTextPlanChunkText)
		ragSystem.MaxChunkSize = 20
		ragSystem.ChunkOverlap = 10
		err := ragSystem.Add("long_doc_1", longText)
		assert.NoError(t, err)

		// 检查文档数量，应该创建多个分块文档
		count, err := store.Count()
		assert.NoError(t, err)
		assert.True(t, count > 1, "Should create multiple chunk documents")

		// 检索文档
		docs, err := store.List()
		assert.NoError(t, err)

		// 验证分块文档的元数据
		chunkFound := false
		for _, doc := range docs {
			if isChunk, ok := doc.Metadata["is_chunk"].(bool); ok && isChunk {
				chunkFound = true
				assert.Contains(t, doc.Metadata, "original_doc_id")
				assert.Contains(t, doc.Metadata, "chunk_index")
				assert.Contains(t, doc.Metadata, "total_chunks")
				assert.Equal(t, "long_doc_1", doc.Metadata["original_doc_id"])
			}
		}
		assert.True(t, chunkFound, "Should find at least one chunk document")

		// 清理
		ragSystem.ClearDocuments()
	})

	// 测试短文本（不会触发BigTextPlan）
	t.Run("Short Text No BigTextPlan", func(t *testing.T) {
		shortText := "短文本"

		err := ragSystem.Add("short_doc", shortText)
		assert.NoError(t, err)

		count, err := store.Count()
		assert.NoError(t, err)
		assert.Equal(t, 1, count, "Short text should create exactly one document")

		doc, exists, err := store.Get("short_doc")
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, shortText, doc.Content)

		// 短文本不应该有分块或池化标记
		assert.Nil(t, doc.Metadata["is_chunk"])
		assert.Nil(t, doc.Metadata["is_pooled"])
	})
}

// 测试平均池化功能
func TestMUSTPASS_AveragePooling(t *testing.T) {
	// 测试空向量
	result := averagePooling([][]float32{})
	assert.Nil(t, result)

	// 测试单个向量
	single := [][]float32{{1.0, 2.0, 3.0}}
	result = averagePooling(single)
	assert.Equal(t, []float32{1.0, 2.0, 3.0}, result)

	// 测试多个向量的平均
	multiple := [][]float32{
		{1.0, 2.0, 3.0},
		{3.0, 4.0, 5.0},
		{5.0, 6.0, 7.0},
	}
	result = averagePooling(multiple)
	expected := []float32{3.0, 4.0, 5.0} // (1+3+5)/3, (2+4+6)/3, (3+5+7)/3
	assert.Equal(t, expected, result)

	// 测试维度不匹配的情况（应该跳过不匹配的向量）
	mismatched := [][]float32{
		{1.0, 2.0, 3.0},
		{3.0, 4.0}, // 维度不匹配
		{5.0, 6.0, 7.0},
	}
	result = averagePooling(mismatched)
	// 应该只计算匹配的向量: (1+5)/2, (2+6)/2, (3+7)/2
	expected = []float32{3.0, 4.0, 5.0}
	assert.Equal(t, expected, result)
}

func Test_AddDocuments(t *testing.T) {
	testRag, err := Get("testRag")
	assert.NoError(t, err)

	doc := `\n为什么检索增强生成很重要？\nLLM 是一项关键的人工智能（AI）技术，为智能聊天机器人和其他自然语言处理（NLP）应用程序提供支持。目标创建能够是通过交叉引用权威知识来源，在各种环境中回答用户问题的机器人。不幸的是，LLM 技术的性质给 LLM 响应带来了不可预测性。此外，LLM 训练数据是静态的，从而为其掌握的知识限定了截止日期。\n\nLLM 面临的已知挑战包括：\n\n在没有答案的情况下提供虚假信息。\n在用户需要具体的最新响应时，提供过时或宽泛的信息。\n依据非权威来源创建响应。\n由于术语混淆，不同的培训来源使用相同的术语来谈论不同的事情，因此会产生不准确的响应。\n您可以将大语言模型看作是一个过于热情的新员工，他拒绝随时了解时事，但总是会绝对自信地回答每一个问题。不幸的是，这种态度会对用户的信任产生负面影响，这是您不希望聊天机器人效仿的！\n\nRAG 是解决其中一些挑战的一种方法。它会重定向 LLM，从权威的、预先确定的知识来源中检索相关信息。组织可以更好地控制生成的文本输出，并且用户可以深入了解 LLM 如何生成响应。\n\n检索增强生成有哪些好处？\nRAG 技术为组织的生成式人工智能工作带来了多项好处。\n\n经济高效的实施\n聊天机器人开发通常从基础模型开始。基础模型（FM）是在广泛的广义和未标记数据上训练的 API 可访问 LLM。针对组织或领域特定信息重新训练基础模型的计算和财务成本很高。RAG 是一种将新数据引入 LLM 的更加经济高效的方法。它使生成式人工智能技术更广泛地得以获取和使用。\n\n当前信息\n即使 LLM 的原始训练数据来源适合您的需求，但保持相关性也具有挑战性。RAG 允许开发人员为生成模型提供最新的研究、统计数据或新闻。他们可以使用 RAG 将 LLM 直接连接到实时社交媒体提要、新闻网站或其他经常更新的信息来源。LLM 随即可以向用户提供最新信息。\n\n增强用户信任度\nRAG 允许 LLM 通过来源归属来呈现准确的信息。输出可以包括对来源的引文或引用。如果需要进一步说明或更详细的信息，用户也可以自己查找源文档。这可以增加对您的生成式人工智能解决方案的信任和信心。\n\n更多开发人员控制权\n借助 RAG，开发人员可以更高效地测试和改进他们的聊天应用程序。他们可以控制和更改 LLM 的信息来源，以适应不断变化的需求或跨职能使用。开发人员还可以将敏感信息的检索限制在不同的授权级别内，并确保 LLM 生成适当的响应。此外，如果 LLM 针对特定问题引用了错误的信息来源，他们还可以进行故障排除并进行修复。组织可以更自信地为更广泛的应用程序实施生成式人工智能技术。\n\n检索增强生成的工作原理是什么？\n如果没有 RAG，LLM 会接收用户输入，并根据训练信息（即其已知信息）创建响应。如果有 RAG，则会引入一个信息检索组件，利用用户输入首先从新数据源提取信息。用户查询和相关信息都提供给 LLM。LLM 使用新知识及其训练数据来创建更好的响应。以下各部分概述了该过程。\n\n创建外部数据\nLLM 原始训练数据集之外的新数据称为外部数据。它可以来自多个数据来源，例如 API、数据库或文档存储库。数据可能以各种格式存在，例如文件、数据库记录或长篇文本。另一种称为嵌入语言模型的人工智能技术将数据转换为数字表示形式并将其存储在向量数据库中。这个过程会创建一个生成式人工智能模型可以理解的知识库。\n\n检索相关信息\n下一步是执行相关性搜索。用户查询将转换为向量表示形式，并与向量数据库匹配。例如，考虑一个可以回答组织的人力资源问题的智能聊天机器人。如果员工搜索：“我有多少年假？”，系统将检索年假政策文档以及员工个人过去的休假记录。将会返回这些与员工输入的内容高度相关的特定文档。相关性是使用数学向量计算和表示法计算得出并确立的。\n\n增强 LLM 提示\n接下来，RAG 模型通过在上下文中添加检索到的相关数据来增强用户输入（或提示）。此步骤使用提示工程技术与 LLM 进行有效沟通。增强提示允许大语言模型为用户查询生成准确的答案。\n\n更新外部数据\n下一个问题可能是——如果外部数据过时了怎么办？ 要维护当前信息以供检索，请异步更新文档并更新文档的嵌入表示形式。您可以通过自动化实时流程或定期批处理来执行此操作。这是数据分析中常见的挑战——可以使用不同的数据科学方法进行变更管理。\n\n下图显示了将 RAG 与 LLM 配合使用的概念流程。\n\n为什么检索增强生成很重要？\nLLM 是一项关键的人工智能（AI）技术，为智能聊天机器人和其他自然语言处理（NLP）应用程序提供支持。目标创建能够是通过交叉引用权威知识来源，在各种环境中回答用户问题的机器人。不幸的是，LLM 技术的性质给 LLM 响应带来了不可预测性。此外，LLM 训练数据是静态的，从而为其掌握的知识限定了截止日期。\n\nLLM 面临的已知挑战包括：\n\n在没有答案的情况下提供虚假信息。\n在用户需要具体的最新响应时，提供过时或宽泛的信息。\n依据非权威来源创建响应。\n由于术语混淆，不同的培训来源使用相同的术语来谈论不同的事情，因此会产生不准确的响应。\n您可以将大语言模型看作是一个过于热情的新员工，他拒绝随时了解时事，但总是会绝对自信地回答每一个问题。不幸的是，这种态度会对用户的信任产生负面影响，这是您不希望聊天机器人效仿的！\n\nRAG 是解决其中一些挑战的一种方法。它会重定向 LLM，从权威的、预先确定的知识来源中检索相关信息。组织可以更好地控制生成的文本输出，并且用户可以深入了解 LLM 如何生成响应。\n\n检索增强生成有哪些好处？\nRAG 技术为组织的生成式人工智能工作带来了多项好处。\n\n经济高效的实施\n聊天机器人开发通常从基础模型开始。基础模型（FM）是在广泛的广义和未标记数据上训练的 API 可访问 LLM。针对组织或领域特定信息重新训练基础模型的计算和财务成本很高。RAG 是一种将新数据引入 LLM 的更加经济高效的方法。它使生成式人工智能技术更广泛地得以获取和使用。\n\n当前信息\n即使 LLM 的原始训练数据来源适合您的需求，但保持相关性也具有挑战性。RAG 允许开发人员为生成模型提供最新的研究、统计数据或新闻。他们可以使用 RAG 将 LLM 直接连接到实时社交媒体提要、新闻网站或其他经常更新的信息来源。LLM 随即可以向用户提供最新信息。\n\n增强用户信任度\nRAG 允许 LLM 通过来源归属来呈现准确的信息。输出可以包括对来源的引文或引用。如果需要进一步说明或更详细的信息，用户也可以自己查找源文档。这可以增加对您的生成式人工智能解决方案的信任和信心。\n\n更多开发人员控制权\n借助 RAG，开发人员可以更高效地测试和改进他们的聊天应用程序。他们可以控制和更改 LLM 的信息来源，以适应不断变化的需求或跨职能使用。开发人员还可以将敏感信息的检索限制在不同的授权级别内，并确保 LLM 生成适当的响应。此外，如果 LLM 针对特定问题引用了错误的信息来源，他们还可以进行故障排除并进行修复。组织可以更自信地为更广泛的应用程序实施生成式人工智能技术。\n\n检索增强生成的工作原理是什么？\n如果没有 RAG，LLM 会接收用户输入，并根据训练信息（即其已知信息）创建响应。如果有 RAG，则会引入一个信息检索组件，利用用户输入首先从新数据源提取信息。用户查询和相关信息都提供给 LLM。LLM 使用新知识及其训练数据来创建更好的响应。以下各部分概述了该过程。\n\n创建外部数据\nLLM 原始训练数据集之外的新数据称为外部数据。它可以来自多个数据来源，例如 API、数据库或文档存储库。数据可能以各种格式存在，例如文件、数据库记录或长篇文本。另一种称为嵌入语言模型的人工智能技术将数据转换为数字表示形式并将其存储在向量数据库中。这个过程会创建一个生成式人工智能模型可以理解的知识库。\n\n检索相关信息\n下一步是执行相关性搜索。用户查询将转换为向量表示形式，并与向量数据库匹配。例如，考虑一个可以回答组织的人力资源问题的智能聊天机器人。如果员工搜索：“我有多少年假？”，系统将检索年假政策文档以及员工个人过去的休假记录。将会返回这些与员工输入的内容高度相关的特定文档。相关性是使用数学向量计算和表示法计算得出并确立的。\n\n增强 LLM 提示\n接下来，RAG 模型通过在上下文中添加检索到的相关数据来增强用户输入（或提示）。此步骤使用提示工程技术与 LLM 进行有效沟通。增强提示允许大语言模型为用户查询生成准确的答案。\n\n更新外部数据\n下一个问题可能是——如果外部数据过时了怎么办？ 要维护当前信息以供检索，请异步更新文档并更新文档的嵌入表示形式。您可以通过自动化实时流程或定期批处理来执行此操作。这是数据分析中常见的挑战——可以使用不同的数据科学方法进行变更管理。\n\n下图显示了将 RAG 与 LLM 配合使用的概念流程。\n\n为什么检索增强生成很重要？\nLLM 是一项关键的人工智能（AI）技术，为智能聊天机器人和其他自然语言处理（NLP）应用程序提供支持。目标创建能够是通过交叉引用权威知识来源，在各种环境中回答用户问题的机器人。不幸的是，LLM 技术的性质给 LLM 响应带来了不可预测性。此外，LLM 训练数据是静态的，从而为其掌握的知识限定了截止日期。\n\nLLM 面临的已知挑战包括：\n\n在没有答案的情况下提供虚假信息。\n在用户需要具体的最新响应时，提供过时或宽泛的信息。\n依据非权威来源创建响应。\n由于术语混淆，不同的培训来源使用相同的术语来谈论不同的事情，因此会产生不准确的响应。\n您可以将大语言模型看作是一个过于热情的新员工，他拒绝随时了解时事，但总是会绝对自信地回答每一个问题。不幸的是，这种态度会对用户的信任产生负面影响，这是您不希望聊天机器人效仿的！\n\nRAG 是解决其中一些挑战的一种方法。它会重定向 LLM，从权威的、预先确定的知识来源中检索相关信息。组织可以更好地控制生成的文本输出，并且用户可以深入了解 LLM 如何生成响应。\n\n检索增强生成有哪些好处？\nRAG 技术为组织的生成式人工智能工作带来了多项好处。\n\n经济高效的实施\n聊天机器人开发通常从基础模型开始。基础模型（FM）是在广泛的广义和未标记数据上训练的 API 可访问 LLM。针对组织或领域特定信息重新训练基础模型的计算和财务成本很高。RAG 是一种将新数据引入 LLM 的更加经济高效的方法。它使生成式人工智能技术更广泛地得以获取和使用。\n\n当前信息\n即使 LLM 的原始训练数据来源适合您的需求，但保持相关性也具有挑战性。RAG 允许开发人员为生成模型提供最新的研究、统计数据或新闻。他们可以使用 RAG 将 LLM 直接连接到实时社交媒体提要、新闻网站或其他经常更新的信息来源。LLM 随即可以向用户提供最新信息。\n\n增强用户信任度\nRAG 允许 LLM 通过来源归属来呈现准确的信息。输出可以包括对来源的引文或引用。如果需要进一步说明或更详细的信息，用户也可以自己查找源文档。这可以增加对您的生成式人工智能解决方案的信任和信心。\n\n更多开发人员控制权\n借助 RAG，开发人员可以更高效地测试和改进他们的聊天应用程序。他们可以控制和更改 LLM 的信息来源，以适应不断变化的需求或跨职能使用。开发人员还可以将敏感信息的检索限制在不同的授权级别内，并确保 LLM 生成适当的响应。此外，如果 LLM 针对特定问题引用了错误的信息来源，他们还可以进行故障排除并进行修复。组织可以更自信地为更广泛的应用程序实施生成式人工智能技术。\n\n检索增强生成的工作原理是什么？\n如果没有 RAG，LLM 会接收用户输入，并根据训练信息（即其已知信息）创建响应。如果有 RAG，则会引入一个信息检索组件，利用用户输入首先从新数据源提取信息。用户查询和相关信息都提供给 LLM。LLM 使用新知识及其训练数据来创建更好的响应。以下各部分概述了该过程。\n\n创建外部数据\nLLM 原始训练数据集之外的新数据称为外部数据。它可以来自多个数据来源，例如 API、数据库或文档存储库。数据可能以各种格式存在，例如文件、数据库记录或长篇文本。另一种称为嵌入语言模型的人工智能技术将数据转换为数字表示形式并将其存储在向量数据库中。这个过程会创建一个生成式人工智能模型可以理解的知识库。\n\n检索相关信息\n下一步是执行相关性搜索。用户查询将转换为向量表示形式，并与向量数据库匹配。例如，考虑一个可以回答组织的人力资源问题的智能聊天机器人。如果员工搜索：“我有多少年假？”，系统将检索年假政策文档以及员工个人过去的休假记录。将会返回这些与员工输入的内容高度相关的特定文档。相关性是使用数学向量计算和表示法计算得出并确立的。\n\n增强 LLM 提示\n接下来，RAG 模型通过在上下文中添加检索到的相关数据来增强用户输入（或提示）。此步骤使用提示工程技术与 LLM 进行有效沟通。增强提示允许大语言模型为用户查询生成准确的答案。\n\n更新外部数据\n下一个问题可能是——如果外部数据过时了怎么办？ 要维护当前信息以供检索，请异步更新文档并更新文档的嵌入表示形式。您可以通过自动化实时流程或定期批处理来执行此操作。这是数据分析中常见的挑战——可以使用不同的数据科学方法进行变更管理。\n\n下图显示了将 RAG 与 LLM 配合使用的概念流程。\n\n检索增强生成和语义搜索有什么区别？\n语义搜索可以完善 RAG 结果，适用于想要在其 LLM 应用程序中添加大量外部知识源的组织。现代企业在各种系统中存储大量信息，例如手册、常见问题、研究报告、客户服务指南和人力资源文档存储库等。上下文检索在规模上具有挑战性，因此会降低生成输出质量。\n\n语义搜索技术可以扫描包含不同信息的大型数据库，并更准确地检索数据。例如，他们可以回答诸如“去年在机械维修上花了多少钱？”之类的问题，方法是将问题映射到相关文档并返回特定文本而不是搜索结果。然后，开发人员可以使用该答案为 LLM 提供更多上下文。\n\nRAG 中的传统或关键字搜索解决方案对知识密集型任务产生的结果有限。开发人员在手动准备数据时还必须处理单词嵌入、文档分块和其他复杂问题。相比之下，语义搜索技术可以完成知识库准备的所有工作，因此开发人员不必这样做。它们还生成语义相关的段落和按相关性排序的标记词，以最大限度地提高 RAG 有效载荷的质量。\n\nAWS 如何支持您的检索增强生成需求？\nAmazon Bedrock 是一项完全托管的服务，提供多种高性能基础模型以及多种功能，用于构建生成式人工智能应用程序，同时简化开发并维护隐私和安全。借助 Amazon Bedrock 的知识库，您只需点击几下即可将基础模型连接到您的 RAG 数据来源。向量转换、检索和改进的输出生成均自动处理。\n\n对于管理自己的 RAG 的组织来说，Amazon Kendra 是一项由机器学习提供支持的高精度企业搜索服务。它提供了经过优化的 Kendra 检索 API，您可以将其与 Amazon Kendra 的高精度语义排名器一起使用，作为 RAG 工作流程的企业检索器。例如，使用检索 API，您可以：\n\n检索多达 100 个语义相关的段落，每个段落最多包含 200 个标记词，按相关性排序。\n使用预构建的连接器连接到常用数据技术，例如 Amazon Simple Storage Service、SharePoint、Confluence 和其他网站。\n支持多种文档格式，例如 HTML、Word、PowerPoint、PDF、Excel 和文本文件。\n根据最终用户权限允许的文档筛选响应。\n亚马逊还为想要构建更多自定义生成式人工智能解决方案的组织提供了选项。Amazon SageMaker JumpStart 是一个机器学习中心，包含基础模型、内置算法和预构建的机器学习解决方案，只需点击几下即可轻松部署。您可以通过参考现有的 SageMaker 笔记本和代码示例来加快 RAG 的实施。\n\n立即创建免费账户，开始在 AWS 上使用检索增强生成\n\nSkip to main content\n新增功能\n\n生成式 AI 执行指南\n\n了解详情\n关于我们\n合作伙伴\n支持\n|CN\n登录\nElasticsearch\n解决方案\n企业\n资源\n定价\n文档\n\n搜索\n\n联系销售人员\n\n什么是 RAG（检索增强生成）？\n超越 RAG 基础功能\n检索增强生成 (RAG) 的定义\n那什么是信息检索呢？\nAI 语言模型的演变\nRAG 如何运作？\nRAG 优势\n检索增强生成与微调的对比\n检索增强生成的挑战和局限\n检索增强生成的未来趋势\n检索增强生成和 Elasticsearch\n浏览更多的 RAG 资源\n检索增强生成 (RAG) 的定义\n检索增强生成 (RAG) 是一种使用来自私有或专有数据源的信息来补充文本生成的技术。它将检索模型（设计用于搜索大型数据集或知识库）和生成模型（例如大型语言模型 (LLM)，此类模型会使用检索到的信息生成可供阅读的文本回复）结合在一起。\n\n通过从更多数据源添加背景信息，以及通过训练来补充 LLM 的原始知识库，检索增强生成能够提高搜索体验的相关性。这能够改善大型语言模型的输出，但又无需重新训练模型。额外信息源的范围很广，从训练 LLM 时并未用到的互联网上的新信息，到专有商业背景信息，或者属于企业的机密内部文档，都会包含在内。\n\nRAG 对于诸如回答问题和内容生成等任务，具有极大价值，因为它能支持生成式 AI 系统使用外部信息源生成更准确且更符合语境的回答。它会实施搜索检索方法（通常是语义搜索或混合搜索）来回应用户的意图并提供更相关的结果。\n\n深入研究检索增强生成 (RAG)，以及这个方法如何将您的专有实时数据与生成式 AI 模型关联起来，以获得更好的最终用户体验和准确性。\n\n\n\n那什么是信息检索呢？\n信息检索 (IR) 指从知识源或数据集搜索并提取相关信息的过程。这一过程特别像使用搜索引擎在互联网上查找信息。您输入查询，系统会检索并为您呈现最有可能包含您正在查找的信息的文档或网页。\n\n信息检索涉及使用相关技术来高效索引并搜索大型数据集，这让人们能够更轻松地从海量的可用数据中访问他们所需的特定信息。除了用于网络搜索引擎，IR 还经常用于数字图书馆、文档管理系统以及各种各样的信息访问应用程序。\n\nAI 语言模型的演变\nAI 语言模型的演变图\n\nAI 语言模型在过去这些年发生了巨大演变：\n\n在 1950 和 1960 年代，这一领域还处于萌芽阶段，使用的是基于规则的基础系统，这一系统对语言的理解能力有限。\n20 世纪 70 年代和 80 年代引入了专家系统：这些专家系统会将人类知识进行编码以解决问题，但是在语言学能力方面仍十分有限。\n在 1990 年代，统计学方法开始盛行，其会使用数据驱动型方法来完成语言任务。\n到了 2000 年代，机器学习技术—例如支持向量机（在一个高维度空间内对不同类型的文本数据进行分类）—开始出现，尽管深度学习仍处于早期阶段。\n在 2010 年代，深度学习出现了巨大转变。转换器架构通过使用注意力机制，改变了自然语言处理；通过注意力机制，模型能够在处理输入序列时专注于输入序列的不同部分。\n当今，转换器模型通过预测词汇序列中随后将会出现的词汇，能够以模拟人类语言的方式处理数据。这些模型为这一领域带来了变革，并促生了 LLM（例如谷歌的 BERT（基于转换器的双向编码器表示））的兴起。\n\n我们看到业界正在将大型预训练模型与设计用于具体任务的专业模型相结合。诸如 RAG 等模型仍在继续获得关注，将生成式 AI 语言模型的范围扩展到了标准训练的边界以外。在 2022 年，OpenAI 推出了 ChatGPT，这可以说是最为人熟知的基于转换器架构的 LLM。ChatGPT 的竞争对手有聊天式的基础模型，例如谷歌的 Bard，以及微软的 Bing Chat。Meta 的 Llama 2 并非面向消费者的聊天机器人，而是一个开源 LLM，免费提供给熟悉 LLM 运行原理的研究人员。\n\n将预训练模型关联至开源 LLM 的 AI 供应链\n\n相关内容：选择 LLM：2024 年开源 LLM 入门指南\n\nRAG 如何运作？\n检索增强生成是一个多步式流程，始于检索，然后推进到生成。下面介绍了其运作方式：\n\n检索\n\nRAG 从输入查询开始。这可以是用户的问题，或者需要详细回复的任意一段文本。\n检索模型会从知识库、数据库或外部来源（或者同时从多个来源）抓取相关信息。模型在何处搜索取决于输入查询所询问的内容。检索到的这一信息现在可以作为模型所需要的任何事实或背景信息的参考来源。\n检索到的信息会被转化为高维度空间中的向量。这些知识向量存储在向量数据库中。\n向量模型会基于与输入查询的相关性，对检索到的信息进行排序。分数最高的文档或段落会被选中，以进行进一步的处理。\n生成\n\n接下来，生成模型（例如 LLM）会使用检索到的信息来生成文本回复。\n生成的文本可能会经过额外的后处理步骤，以确保其语法正确，语义连贯。\n整体而言，这些回复更加准确，也更符合语境，因为这些回复使用的是检索模型所提供的补充信息。在缺少公共互联网数据的专业领域，这一功能尤其重要。\nrag-in-action.jpeg\n\nRAG 优势\n相比于单独运行的语言模型，检索增强生成有数项优势。下面列举了它可以从哪些方面改进文本生成和回复：\n\nRAG 会确保您的模型能够访问最新、最及时的事实和相关信息，因为它能定期更新外部参考信息。这能确保：它所生成的回复会纳入可能与提出查询的用户相关的最新信息。您还可以实施文档级安全性来控制数据流中数据的访问权限，并限制特定文档的安全许可。\nRAG 是更具有成本效益的选项，因为它需要的计算和存储都更少，这意味着您无需拥有自己的 LLM，也无需花费时间和资金对您的模型进行微调。\n声称数据准确固然很简答，但要证明数据准确却不简单。RAG 可以引用外部来源并将其提供给用户，以便用户为其回复提供支持性信息。如果愿意的话，用户还可以评估来源，以确认他们所收到的回复是否准确。\n虽然由 LLM 提供支持的聊天机器人可以提供比之前的脚本式回复更加个性化的答案，但 RAG 可以提供更加量身定制的答案。这是因为，RAG 在通过衡量意图来生成答案时，能够使用搜索检索方法（通常是语义搜索）来参考一系列基于背景信息得出的要点。\n当遇到训练时未出现过的复杂查询时，LLM 有时候会“出现幻觉”，提供不准确的回复。对于模糊性查询，RAG 可以更准确地进行回复，因为它的答案基于来自相关数据源的更多参考资料。\nRAG 模型用途多样，可用于执行各种各样的自然语言处理任务，包括对话系统、内容生成，以及信息检索。\n在任何人造 AI 中，偏见都会是一个问题。RAG 在回答时可以帮助减少偏见，因为它依赖的是经过筛查的外部来源。\n检索增强生成与微调的对比\n检索增强生成和微调是训练 AI 语言模型的两种不同方法。RAG 会将检索大量外部知识的过程与文本生成结合在一起，而微调则专注于狭窄的数据范围以实现不同的目的。\n\n在微调过程中，系统会使用专门数据对预训练模型进一步加以训练，以便其能适用于任务子集。这一过程涉及基于新数据集修改模型的权重和参数，让模型学习特定于任务的模式，同时保留来自最初预训练模型的知识。\n\n微调可用于各种类型的 AI。一个基本的例子是识别小猫，具体而言是识别网络上猫的照片。在基于语言的模型中，除了文本生成，微调还能够协助完成诸如文本分类、情感分析和命名实体识别等事务。但是，这一过程可能会极其耗费时间和资金。RAG 能够加速完成这一过程，并且由于计算和存储需求较低，还能降低时间和资金成本。\n\n由于能够访问外部资源，RAG 在处理下列任务时尤其有用：需要纳入来自网络或企业知识库的实时或动态信息，以便生成明智的回复。微调有不同的优势：如果当前任务定义明确，并且目标是仅优化该任务的性能，那么微调会非常高效。两种技术的共同优势是：无需针对每个任务都从零开始训练 LLM。\n\n检索增强生成的挑战和局限\n虽然 RAG 能提供巨大优势，但也存在数项挑战和局限：\n\nRAG 依赖于外部知识。如果检索到的信息不正确，RAG 就会生成不准确的结果。\nRAG 的检索部分涉及在大型知识库或网络上进行搜索，这从计算量方面来看，不仅费用高昂，而且速度慢，尽管相比于微调，速度还是快一些，费用也要低一些。\n要将检索和生成部分无缝集成到一起，这需要进行精心设计和优化，而设计和优化可能会在训练和部署方面造成潜在难题。\n当处理敏感数据时，从外部来源检索信息可能带来隐私问题。由于需要遵守隐私和合规要求，这也可能会限制 RAG 能够访问的来源。然而，这能通过文档级访问权限加以解决；文档级访问权限指您可以向特定角色赋予访问和安全许可。\nRAG 的基础是基于事实的准确性。它可能难以生成富有想象力或虚构性质的内容，这限制了它在创意内容生成领域的使用。\n检索增强生成的未来趋势\n检索增强生成的未来趋势是，专注于提高 RAG 技术的效率，并让其更加适用于各种应用程序。下面是值得关注的一些趋势：\n\n个性化\nRAG 模型将会继续纳入用户特定的知识。这将让 RAG 生成更加个性化的回复，尤其是在内容推荐和虚拟助手等应用程序方面。\n\n可定制的行为\n除了个性化，用户自己还能对 RAG 模型的行为和回复方式拥有更多掌控权，这有助于用户获得他们正在寻找的结果。\n\n可扩展性\n与现在相比，RAG 将能够处理更大量的数据和用户互动。\n\n混合模型\n将 RAG 与其他 AI 技术（例如强化学习）相集成，这将能够促生用途更加多样、更加符合语境的系统，而且这些系统能够同时处理各种数据类型和任务。\n\n实时和低延迟部署\n随着 RAG 模型在检索速度和响应时间方面越来越出色，其在需要快速回复的应用程序（例如聊天机器人和虚拟助手）中将会用得越来越多。\n\n深入了解 2024 年技术搜索趋势。请观看此网络研讨会，了解 2024 年的最佳实践、新兴方法以及热门趋势对开发人员的影响。\n\n检索增强生成和 Elasticsearch\n借助 Elasticsearch，您可以针对生成式 AI 应用、网站、客户或员工体验，打造基于 RAG 的搜索功能。Elasticsearch 可提供完整的工具包，以便您能：\n\n存储并搜索专有数据，以及搜索可从中提取背景信息的其他外部知识库\n使用各种方法（文本、向量、混合，或语义搜索），基于您的数据生成高度相关的搜索结果\n为您的用户提供更加准确的回复并打造更加引人入胜的体验\n了解 Elasticsearch 可以如何针对您的业务改进生成式 AI\n\n浏览更多的 RAG 资源\n探索 AI Playground\n超越 RAG 基础功能\nElasticsearch – 适用于 RAG 的相关度最高的搜索引擎\n选择 LLM：2024 年开源 LLM 入门指南\nAI 搜索算法的解释\n如何创建聊天机器人：AI 驱动世界中开发人员的“宜与忌”\n2024 年技术趋势：搜索和生成式 AI 技术的发展趋势\n更快地构建原型并与 LLM 集成\n全球下载量最大的向量数据库 — Elasticsearch\n揭开 ChatGPT 的神秘面纱：构建 AI 搜索的不同方法\n检索与毒药的对比 — 对抗 AI 供应链攻击\nElastic home\n关注我们\nElastic's LinkedIn page\nElastic's YouTube page\nElastic's Facebook page\nElastic's Twitter page\nElastic's GitHub page\n关于我们\n关于 Elastic\n领导团队\n博客\n新闻编辑室\n加入我们\n招贤纳士\n招聘门户\n招聘方式\n合作伙伴\n寻找合作伙伴\n合作伙伴登录\n申请访问权限\n成为合作伙伴\n信任和安全性\n法律\n信任中心\n隐私\n贸易合规性\n道德与合规\n投资者关系\n投资者资源\n治理\n金融\n股票\n卓越奖\n往届获奖者\nElasticON 之旅\n成为赞助商\n所有活动\n商标使用条款隐私网站地图\n© 2025.Elasticsearch B.V. 版权所有\n\n本网站及所有相关内容、软件、讨论论坛`
	err = testRag.Add("test", doc)
	assert.NoError(t, err)

	docs, err := testRag.ListDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(docs))
	assert.Equal(t, doc, docs[0].Content)
}
