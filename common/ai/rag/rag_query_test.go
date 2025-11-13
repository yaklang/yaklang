package rag

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

// testEmbedder 测试用的嵌入器函数
func testEmbedder(text string) ([]float32, error) {
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	return mockEmbedding.Embedding(text)
}

// getMapKeys 获取map的所有键
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestRAGQuery(t *testing.T) {
	// 创建测试数据库
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Skip("database not available")
		return
	}

	// 定义三个不同领域的集合
	collections := []struct {
		name        string
		description string
		documents   []vectorstore.Document
	}{
		{
			name:        "cybersecurity_knowledge_" + utils.RandStringBytes(6),
			description: "网络安全知识库：专门收录网络安全攻防技术、漏洞分析、安全工具使用等专业知识，为安全研究人员和渗透测试工程师提供全面的安全技术指导，涵盖Web安全、系统安全、网络防护等多个安全领域的理论和实践内容。",
			documents: []vectorstore.Document{
				{
					ID:      "sec_001",
					Content: "SQL注入攻击是一种常见的Web安全漏洞，攻击者通过在输入字段中插入恶意SQL代码来获取数据库敏感信息。防护措施包括使用参数化查询、输入验证和最小权限原则。",
					Metadata: map[string]any{
						"type":       "vulnerability",
						"category":   "web_security",
						"risk_level": "high",
					},
				},
				{
					ID:      "sec_002",
					Content: "XSS跨站脚本攻击允许攻击者在受害者浏览器中执行恶意脚本。主要分为反射型XSS、存储型XSS和DOM型XSS三种类型。防护方法包括输出编码、内容安全策略(CSP)和输入过滤。",
					Metadata: map[string]any{
						"type":       "vulnerability",
						"category":   "web_security",
						"risk_level": "medium",
					},
				},
			},
		},
		{
			name:        "ai_technology_" + utils.RandStringBytes(6),
			description: "人工智能技术知识库：汇集机器学习、深度学习、自然语言处理等AI前沿技术知识，为AI研究人员和算法工程师提供从基础理论到工程实践的完整技术栈，包括神经网络架构、模型训练、数据处理等核心技术内容。",
			documents: []vectorstore.Document{
				{
					ID:      "ai_001",
					Content: "深度学习是机器学习的一个子领域，使用多层神经网络来学习数据的复杂模式。常见的网络架构包括卷积神经网络(CNN)用于图像处理，循环神经网络(RNN)用于序列数据处理。",
					Metadata: map[string]any{
						"type":       "algorithm",
						"category":   "deep_learning",
						"difficulty": "advanced",
					},
				},
				{
					ID:      "ai_002",
					Content: "自然语言处理(NLP)是AI的重要分支，旨在让计算机理解和生成人类语言。现代NLP技术主要基于Transformer架构，如BERT、GPT等大型语言模型在各种语言任务中表现优异。",
					Metadata: map[string]any{
						"type":       "algorithm",
						"category":   "nlp",
						"difficulty": "advanced",
					},
				},
			},
		},
		{
			name:        "programming_guide_" + utils.RandStringBytes(6),
			description: "编程开发指南知识库：涵盖主流编程语言、开发框架、软件工程实践等开发技术知识，为软件开发工程师提供从语言基础到架构设计的全方位技术指导，包括代码规范、性能优化、项目管理等开发实践经验。",
			documents: []vectorstore.Document{
				{
					ID:      "prog_001",
					Content: "Go语言是Google开发的开源编程语言，以其简洁的语法、高效的并发处理和快速的编译速度著称。Go的goroutine和channel机制为并发编程提供了优雅的解决方案。",
					Metadata: map[string]any{
						"type":        "language",
						"category":    "backend",
						"performance": "high",
					},
				},
				{
					ID:      "prog_002",
					Content: "Python是一种高级解释型编程语言，以其简洁易读的语法和丰富的生态系统闻名。Python在数据科学、Web开发、自动化脚本等领域应用广泛，拥有NumPy、pandas等强大的库支持。",
					Metadata: map[string]any{
						"type":        "language",
						"category":    "general_purpose",
						"performance": "medium",
					},
				},
			},
		},
	}

	// 创建并初始化所有集合
	var ragSystems []*RAGSystem
	var collectionNames []string

	for i, col := range collections {
		t.Logf("Creating collection %d: %s", i+1, col.name)
		ragSystem, err := GetRagSystem(col.name, WithDB(db), WithEmbeddingModel("test"))
		if err != nil {
			t.Logf("Failed to create collection %s (may be expected if embedding service is not available): %v", col.name, err)
			t.Skip("skipping test due to collection creation failure")
			return
		}

		ragSystems = append(ragSystems, ragSystem)
		collectionNames = append(collectionNames, col.name)

		// 添加该集合的文档
		for _, doc := range col.documents {
			err = ragSystem.Add(doc.ID, doc.Content, WithDocumentRawMetadata(doc.Metadata))
			if err != nil {
				t.Fatalf("Failed to add document %s to collection %s: %v", doc.ID, col.name, err)
			}
		}

		t.Logf("Added %d documents to collection: %s", len(col.documents), col.name)
	}

	// 清理资源
	defer func() {
		for _, name := range collectionNames {
			vectorstore.DeleteCollection(db, name)
		}
	}()

	// 等待向量索引构建
	t.Log("Waiting for vector indexing to complete...")
	time.Sleep(3 * time.Second)

	// 测试不同领域的查询
	testQueries := []struct {
		name           string
		query          string
		expectedDomain string
		expectedDocIDs []string
		minResults     int
	}{
		{
			name:           "安全漏洞查询",
			query:          "SQL注入攻击防护",
			expectedDomain: "cybersecurity",
			expectedDocIDs: []string{"sec_001"},
			minResults:     1,
		},
		{
			name:           "AI技术查询",
			query:          "深度学习神经网络",
			expectedDomain: "ai_technology",
			expectedDocIDs: []string{"ai_001"},
			minResults:     1,
		},
		{
			name:           "编程语言查询",
			query:          "Go语言并发编程",
			expectedDomain: "programming",
			expectedDocIDs: []string{"prog_001"},
			minResults:     1,
		},
		{
			name:           "跨领域查询",
			query:          "编程语言学习",
			expectedDomain: "mixed",
			expectedDocIDs: []string{"prog_001", "prog_002"},
			minResults:     1,
		},
	}

	for _, testCase := range testQueries {
		t.Run(testCase.name, func(t *testing.T) {
			t.Logf("Testing query: %s", testCase.query)

			// 测试SimpleQuery - 自动发现集合
			results, err := SimpleQuery(db, testCase.query, 5)
			if err != nil {
				t.Errorf("SimpleQuery failed for '%s': %v", testCase.query, err)
				return
			}

			if len(results) < testCase.minResults {
				t.Errorf("SimpleQuery for '%s' returned %d results, expected at least %d",
					testCase.query, len(results), testCase.minResults)
				return
			}

			t.Logf("SimpleQuery for '%s' returned %d results", testCase.query, len(results))

			// 验证结果相关性
			foundExpectedDoc := false
			for i, result := range results {
				content := result.Document.Content
				if len(content) > 80 {
					content = content[:80] + "..."
				}
				t.Logf("  Result %d: ID=%s, Score=%.3f, Content=%s",
					i+1, result.Document.ID, result.Score, content)

				// 检查是否找到了期望的文档
				for _, expectedID := range testCase.expectedDocIDs {
					if result.Document.ID == expectedID {
						foundExpectedDoc = true
						t.Logf("  ✓ Found expected document: %s", expectedID)
						break
					}
				}
			}

			if !foundExpectedDoc && testCase.expectedDomain != "mixed" {
				t.Logf("Warning: Expected document not found in top results for query '%s'", testCase.query)
			}
		})
	}

	// 测试完整的Query函数（多集合自动发现）
	t.Log("\n=== Testing full Query function with multi-collection discovery ===")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 测试自然语言处理相关查询，应该主要从AI集合中返回结果
	resultCh, err := Query(db, "自然语言处理和Transformer模型",
		WithRAGCtx(ctx),
		WithRAGLimit(5),
		WithRAGCollectionLimit(3), // 最多搜索3个集合
		WithRAGEnhance(""),        // 禁用增强搜索以加快测试
		WithRAGMsgCallBack(func(result *RAGSearchResult) {
			t.Logf("Query callback - Type: %s, Message: %s", result.Type, result.Message)
		}),
	)
	if err != nil {
		t.Errorf("Query failed: %v", err)
		return
	}

	var finalResults []*RAGSearchResult
	var midResults []*RAGSearchResult
	var messageCount int
	var discoveredCollections = make(map[string]bool)

	for result := range resultCh {
		switch result.Type {
		case "message":
			messageCount++
			t.Logf("Status: %s", result.Message)
		case "mid_result":
			midResults = append(midResults, result)
			if doc, ok := result.Data.(*vectorstore.Document); ok {
				discoveredCollections[result.Source] = true
				t.Logf("Mid result from %s: ID=%s, Score=%.3f", result.Source, doc.ID, result.Score)
			}
		case "result":
			finalResults = append(finalResults, result)
			if doc, ok := result.Data.(*vectorstore.Document); ok {
				t.Logf("Final result from %s: ID=%s, Score=%.3f", result.Source, doc.ID, result.Score)
			}
		}
	}

	// 验证测试结果
	if messageCount == 0 {
		t.Error("Query did not produce any status messages")
	}

	if len(finalResults) == 0 {
		t.Error("Query returned no final results")
	} else {
		t.Logf("✓ Query completed successfully: %d status messages, %d mid results, %d final results",
			messageCount, len(midResults), len(finalResults))
	}

	// 验证是否发现了预期的集合
	t.Logf("Discovered collections: %v", getMapKeys(discoveredCollections))

	// 验证结果的相关性
	foundNLPDoc := false
	for _, result := range finalResults {
		if doc, ok := result.Data.(*vectorstore.Document); ok {
			if doc.ID == "ai_002" { // NLP相关文档
				foundNLPDoc = true
				t.Logf("✓ Found expected NLP document with score %.3f", result.Score)
				break
			}
		}
	}

	if !foundNLPDoc {
		t.Logf("Warning: Expected NLP document not found in final results")
	}
}

func TestMUSTPASS_RAGQueryWithFilter(t *testing.T) {
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Skip("database not available")
		return
	}

	mockEmbed := vectorstore.NewMockEmbedder(testEmbedder)
	collectionName := "test_collection_" + utils.RandStringBytes(8)

	ragSystem, err := NewRAGSystem(WithDB(db), WithName(collectionName), WithEmbeddingModel("test"), WithEmbeddingClient(mockEmbed))
	if err != nil {
		t.Errorf("Failed to create collection: %v", err)
		return
	}

	ragSystem.Add("test", "test", WithDocumentRawMetadata(map[string]any{
		"type": "test",
	}))

	results, err := ragSystem.Query("test", 10)
	if err != nil {
		t.Errorf("Failed to query: %v", err)
		return
	}

	assert.Equal(t, 1, len(results))

	// Test Query function with collection name
	resultsCh, err := Query(db, "test", WithName(collectionName), WithRAGLimit(10))
	if err != nil {
		t.Errorf("Failed to query: %v", err)
		return
	}

	// Consume the results channel
	var queryResults []*RAGSearchResult
	for result := range resultsCh {
		if result.Type == RAGResultTypeResult {
			queryResults = append(queryResults, result)
		}
	}

	assert.Equal(t, 1, len(queryResults))
}

func TestMUSTPASS_RAGQuery(t *testing.T) {

	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Skip("database not available")
		return
	}
	/*
		测试逻辑：
		原始输入是一个uuid，基本不存在任何语义相关性。相似度近似0，即在原始输入的情况下应该没有任何返回结果。
		开启增强搜索后，通过mock embedding生成与各自对应集合中的文档有一定相似度的文本。
		- Hypothetical Answer生成的文本与集合中文档的相似度最高，设置为0.8
		- Generalize Query生成的文本与集合中文档的相似度次之，设置为0.6
		- Split Query生成的文本与集合中文档的相似度最低，设置为0.4

		test 1：相似度阈值设置为0.4，确保只要生成的文本与集合中的文档有一定的相似度就能命中。预期 应得到 三个集合的三个文档
		test 2：相似度阈值设置为0.6，确保只有Hypothetical Answer和Generalize Query生成的文本能命中。预期 应得到 两个集合的两个文档
		test 3：相似度阈值设置为0.8，确保只有Hypothetical Answer生成的文本能命中。预期 应得到 一个集合的一个文档
	*/
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	HighSimilarThresh := 0.8
	MidSimilarThresh := 0.6
	LowSimilarThresh := 0.4

	contentHypotheticalAnswer := mockEmbedding.GenerateRandomText(40)
	contentGeneralizeQuery := mockEmbedding.GenerateRandomText(40)
	contentSplitQuery := mockEmbedding.GenerateRandomText(40)

	uuidHypotheticalAnswer := uuid.NewString()
	uuidGeneralizeQuery := uuid.NewString()
	uuidSplitQuery := uuid.NewString()

	enhanceHandler := enhancesearch.NewMockSearchHandler()

	enhanceHandler.SetHypotheticalAnswerFunc(func(ctx context.Context, query string) (string, error) {
		return mockEmbedding.GenerateSimilarText(contentHypotheticalAnswer, HighSimilarThresh)
	})

	enhanceHandler.SetGeneralizeQueryFunc(func(ctx context.Context, query string) ([]string, error) {
		result, err := mockEmbedding.GenerateSimilarText(contentGeneralizeQuery, MidSimilarThresh)
		if err != nil {
			return nil, err
		}
		return []string{
			result,
		}, nil
	})

	enhanceHandler.SetSplitQueryFunc(func(ctx context.Context, query string) ([]string, error) {
		result, err := mockEmbedding.GenerateSimilarText(contentSplitQuery, LowSimilarThresh)
		if err != nil {
			return nil, err
		}
		return []string{
			result,
		}, nil
	})

	// 定义三个不同领域的集合
	collections := []struct {
		name        string
		description string
		document    vectorstore.Document
	}{
		{
			name:        "A" + utils.RandStringBytes(6),
			description: "测试知识库A",
			document: vectorstore.Document{
				ID:      uuidHypotheticalAnswer,
				Content: contentHypotheticalAnswer,
			},
		},
		{
			name:        "B" + utils.RandStringBytes(6),
			description: "测试知识库B",
			document: vectorstore.Document{
				ID:      uuidGeneralizeQuery,
				Content: contentGeneralizeQuery,
			},
		},
		{
			name:        "C" + utils.RandStringBytes(6),
			description: "测试知识库C",
			document: vectorstore.Document{
				ID:      uuidSplitQuery,
				Content: contentSplitQuery,
			},
		},
	}

	// 创建并初始化所有集合
	var ragSystems []*RAGSystem
	var collectionNames []string

	for i, col := range collections {
		t.Logf("Creating collection %d: %s", i+1, col.name)
		ragSystem, err := NewRAGSystem(WithDB(db), WithName(col.name), WithDescription(col.description), WithEmbeddingClient(mockEmbedding))
		if err != nil {
			t.Logf("Failed to create collection %s (may be expected if embedding service is not available): %v", col.name, err)
			t.Skip("skipping test due to collection creation failure")
			return
		}

		ragSystems = append(ragSystems, ragSystem)
		collectionNames = append(collectionNames, col.name)

		doc := col.document
		err = ragSystem.Add(doc.ID, doc.Content, WithDocumentRawMetadata(doc.Metadata))
		if err != nil {
			t.Fatalf("Failed to add document %s to collection %s: %v", doc.ID, col.name, err)
		}

		t.Logf("Added %s document to collection: %s", doc.ID, col.name)
	}

	// 清理资源
	defer func() {
		for _, name := range collectionNames {
			vectorstore.DeleteCollection(db, name)
		}
	}()

	t.Run("test low", func(t *testing.T) {
		results, err := SimpleQuery(db, uuid.NewString(), 5,
			WithRAGEnhanceSearchHandler(enhanceHandler),
			WithRAGSystemLoadConfig(vectorstore.WithEmbeddingClient(mockEmbedding)),
			WithRAGSimilarityThreshold(LowSimilarThresh))
		require.NoError(t, err)
		require.Len(t, results, 3)
		require.Contains(t, lo.Map(results, func(item *vectorstore.SearchResult, index int) string {
			return item.Document.ID
		}), uuidHypotheticalAnswer, uuidGeneralizeQuery, uuidSplitQuery)
	})

	t.Run("test mid", func(t *testing.T) {
		results, err := SimpleQuery(db, uuid.NewString(), 5,
			WithRAGEnhanceSearchHandler(enhanceHandler),
			WithRAGSystemLoadConfig(vectorstore.WithEmbeddingClient(mockEmbedding)),
			WithRAGSimilarityThreshold(MidSimilarThresh))
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Contains(t, lo.Map(results, func(item *vectorstore.SearchResult, index int) string {
			return item.Document.ID
		}), uuidHypotheticalAnswer, uuidGeneralizeQuery)
	})

	t.Run("test high", func(t *testing.T) {
		results, err := SimpleQuery(db, uuid.NewString(), 5,
			WithRAGEnhanceSearchHandler(enhanceHandler),
			WithRAGSystemLoadConfig(vectorstore.WithEmbeddingClient(mockEmbedding)),
			WithRAGSimilarityThreshold(HighSimilarThresh))
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Contains(t, lo.Map(results, func(item *vectorstore.SearchResult, index int) string {
			return item.Document.ID
		}), uuidHypotheticalAnswer)
	})

}
