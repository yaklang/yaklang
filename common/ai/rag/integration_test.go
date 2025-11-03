package rag

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAddAndQuery(t *testing.T) {
	// 创建临时内存数据库
	tempDB, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatal(err)
	}

	// 创建RAG集合
	ragSystem, err := GetRagSystem("test", WithDB(tempDB), WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()))
	if err != nil {
		t.Fatal(err)
	}

	// 添加测试文档
	testDocs := []struct {
		id       string
		content  string
		metadata map[string]any
	}{
		{
			id:      "doc1",
			content: "这是一篇关于人工智能的文章。人工智能是计算机科学的一个分支，致力于创建能够执行通常需要人类智能的任务的系统。",
			metadata: map[string]any{
				"title":    "人工智能简介",
				"category": "技术",
				"author":   "张三",
			},
		},
		{
			id:      "doc2",
			content: "机器学习是人工智能的一个子领域，它使计算机能够在没有明确编程的情况下学习和改进。深度学习是机器学习的一个分支。",
			metadata: map[string]any{
				"title":    "机器学习概述",
				"category": "技术",
				"author":   "李四",
			},
		},
		{
			id:      "doc3",
			content: "自然语言处理是人工智能的一个重要应用领域，涉及计算机与人类语言之间的交互。它包括文本分析、语言理解和生成等技术。",
			metadata: map[string]any{
				"title":    "自然语言处理",
				"category": "技术",
				"author":   "王五",
			},
		},
	}

	// 添加文档到RAG系统
	for _, doc := range testDocs {
		err := ragSystem.Add(doc.id, doc.content, WithDocumentRawMetadata(doc.metadata))
		if err != nil {
			t.Fatalf("添加文档失败 %s: %v", doc.id, err)
		}
	}

	// 测试查询功能
	testQueries := []struct {
		query              string
		expectedMinDocs    int
		expectedFirstDocID string
		expectedDocIDs     []string
		description        string
	}{
		{
			query:              "人工智能",
			expectedMinDocs:    3,
			expectedFirstDocID: "",                               // 不限制第一个文档
			expectedDocIDs:     []string{"doc1", "doc2", "doc3"}, // 应该能找到前3个文档
			description:        "查询人工智能相关文档",
		},
		{
			query:              "机器学习",
			expectedMinDocs:    1,
			expectedFirstDocID: "doc2", // 第一个文档应该是doc2
			expectedDocIDs:     []string{"doc2"},
			description:        "查询机器学习相关文档",
		},
		{
			query:              "自然语言处理",
			expectedMinDocs:    1,
			expectedFirstDocID: "doc3", // 第一个文档应该是doc3
			expectedDocIDs:     []string{"doc3"},
			description:        "查询自然语言处理相关文档",
		},
		{
			query:              "深度学习",
			expectedMinDocs:    1,
			expectedFirstDocID: "doc2", // 第一个文档应该是doc2
			expectedDocIDs:     []string{"doc2"},
			description:        "查询深度学习相关文档",
		},
	}

	// 执行查询测试
	for _, testQuery := range testQueries {
		t.Run(testQuery.description, func(t *testing.T) {
			// 使用QueryWithPage方法进行查询
			results, err := ragSystem.QueryWithPage(testQuery.query, 1, 10)
			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}

			// 验证查询结果数量
			if len(results) < testQuery.expectedMinDocs {
				t.Errorf("查询 '%s' 期望至少 %d 个结果，实际得到 %d 个",
					testQuery.query, testQuery.expectedMinDocs, len(results))
			}

			// 验证第一个文档ID（如果指定了）
			if testQuery.expectedFirstDocID != "" && len(results) > 0 {
				actualFirstID := results[0].Document.ID
				if actualFirstID != testQuery.expectedFirstDocID {
					t.Errorf("查询 '%s' 的第一个结果应该是 %s，实际是 %s",
						testQuery.query, testQuery.expectedFirstDocID, actualFirstID)
				}
			}

			// 验证期望的文档ID是否都在结果中
			if len(testQuery.expectedDocIDs) > 0 {
				resultIDs := make(map[string]bool)
				for _, result := range results {
					resultIDs[result.Document.ID] = true
				}

				for _, expectedID := range testQuery.expectedDocIDs {
					if !resultIDs[expectedID] {
						t.Errorf("查询 '%s' 期望包含文档 %s，但结果中没有找到",
							testQuery.query, expectedID)
					}
				}
			}

			// 打印查询结果用于调试
			t.Logf("查询 '%s' 返回 %d 个结果:", testQuery.query, len(results))
			for i, result := range results {
				t.Logf("  结果 %d: ID=%s, Score=%.4f, Content=%.100s...",
					i+1, result.Document.ID, result.Score, result.Document.Content)

				// 验证结果包含元数据
				if result.Document.Metadata != nil {
					if title, ok := result.Document.Metadata["title"]; ok {
						t.Logf("    标题: %v", title)
					}
					if category, ok := result.Document.Metadata["category"]; ok {
						t.Logf("    分类: %v", category)
					}
				}
			}

			// 验证分数合理性（分数应该在-1到1之间）
			for _, result := range results {
				if result.Score < -1.0 || result.Score > 1.0 {
					t.Errorf("分数超出合理范围 [-1,1]: %f", result.Score)
				}
			}
		})
	}

	// 测试文档数量统计
	count, err := ragSystem.CountDocuments()
	if err != nil {
		t.Fatalf("统计文档数量失败: %v", err)
	}

	// 验证文档数量（应该包含用户添加的文档）
	expectedCount := len(testDocs) // 用户添加的文档数量
	if count < expectedCount {
		t.Errorf("文档数量不足，期望至少 %d，实际得到 %d", expectedCount, count)
	}

	t.Logf("RAG系统中总共有 %d 个文档", count)

	// 验证数据库表结构
	t.Run("验证数据库表结构", func(t *testing.T) {
		// 验证VectorStoreCollection表结构
		t.Run("验证VectorStoreCollection表", func(t *testing.T) {
			var collection schema.VectorStoreCollection

			// 检查表是否存在
			if !tempDB.HasTable(&collection) {
				t.Fatal("VectorStoreCollection表不存在")
			}

			// 验证集合记录是否正确创建
			var collections []schema.VectorStoreCollection
			err := tempDB.Find(&collections).Error
			if err != nil {
				t.Fatalf("查询VectorStoreCollection表失败: %v", err)
			}

			if len(collections) != 1 {
				t.Errorf("期望1个集合记录，实际得到 %d 个", len(collections))
			} else {
				col := collections[0]
				t.Logf("集合信息: Name=%s, Description=%s, Dimension=%d", col.Name, col.Description, col.Dimension)

				// 验证基本字段
				if col.Name != "test" {
					t.Errorf("集合名称错误，期望'test'，实际'%s'", col.Name)
				}
				if col.Description != "测试知识库" {
					t.Errorf("集合描述错误，期望'测试知识库'，实际'%s'", col.Description)
				}
				if col.Dimension <= 0 {
					t.Errorf("向量维度应该大于0，实际为 %d", col.Dimension)
				}
			}
			// 验证GraphBinary是否不为空，大小是否正常
			graphBinary := collections[0].GraphBinary
			if len(graphBinary) == 0 {
				t.Errorf("GraphBinary为空")
			}
			if len(graphBinary) > 1024*4 {
				t.Errorf("GraphBinary大小超过10MB")
			}
		})

		// 验证VectorStoreDocument表结构
		t.Run("验证VectorStoreDocument表", func(t *testing.T) {
			var document schema.VectorStoreDocument

			// 检查表是否存在
			if !tempDB.HasTable(&document) {
				t.Fatal("VectorStoreDocument表不存在")
			}

			// 验证文档记录是否正确创建
			var documents []schema.VectorStoreDocument
			err := tempDB.Find(&documents).Error
			if err != nil {
				t.Fatalf("查询VectorStoreDocument表失败: %v", err)
			}

			t.Logf("VectorStoreDocument表中有 %d 条记录", len(documents))

			// 验证文档记录的基本信息
			for i, doc := range documents {
				t.Logf("文档 %d: ID=%s, CollectionID=%d, Content长度=%d",
					i+1, doc.DocumentID, doc.CollectionID, len(doc.Content))

				// 验证必须字段
				if doc.DocumentID == "" {
					t.Errorf("文档 %d 的DocumentID为空", i+1)
				}
				if doc.CollectionID == 0 {
					t.Errorf("文档 %d 的CollectionID为0", i+1)
				}
				if len(doc.Embedding) == 0 {
					t.Errorf("文档 %d 的Embedding为空", i+1)
				}
				if doc.Content == "" {
					t.Errorf("文档 %d 的Content为空", i+1)
				}

				// 验证元数据
				if doc.Metadata != nil {
					t.Logf("  元数据字段数: %d", len(doc.Metadata))
					for key, value := range doc.Metadata {
						t.Logf("    %s: %v", key, value)
					}
				}
			}
		})
	})

	testCollection, err := GetRagSystem("test", WithDB(tempDB))
	if err != nil {
		t.Fatalf("获取集合失败: %v", err)
	}
	err = testCollection.VectorStore.ConvertToPQMode()
	if err != nil {
		t.Fatalf("转换为PQ模式失败: %v", err)
	}

	testCollection, err = GetRagSystem("test", WithDB(tempDB))
	if err != nil {
		t.Fatalf("获取集合失败: %v", err)
	}

	// 转换为PQ模式后，重新测试搜索功能
	t.Run("PQ模式搜索测试", func(t *testing.T) {
		for _, testQuery := range testQueries {
			t.Run("PQ_"+testQuery.description, func(t *testing.T) {
				// 使用QueryWithPage方法进行查询
				results, err := testCollection.QueryWithPage(testQuery.query, 1, 10)
				if err != nil {
					t.Fatalf("PQ模式查询失败: %v", err)
				}

				// 验证查询结果数量
				if len(results) < testQuery.expectedMinDocs {
					t.Errorf("PQ模式查询 '%s' 期望至少 %d 个结果，实际得到 %d 个",
						testQuery.query, testQuery.expectedMinDocs, len(results))
				}

				// 验证第一个文档ID（如果指定了）
				if testQuery.expectedFirstDocID != "" && len(results) > 0 {
					actualFirstID := results[0].Document.ID
					if actualFirstID != testQuery.expectedFirstDocID {
						t.Errorf("PQ模式查询 '%s' 的第一个结果应该是 %s，实际是 %s",
							testQuery.query, testQuery.expectedFirstDocID, actualFirstID)
					}
				}

				// 验证期望的文档ID是否都在结果中
				if len(testQuery.expectedDocIDs) > 0 {
					resultIDs := make(map[string]bool)
					for _, result := range results {
						resultIDs[result.Document.ID] = true
					}

					for _, expectedID := range testQuery.expectedDocIDs {
						if !resultIDs[expectedID] {
							t.Errorf("PQ模式查询 '%s' 期望包含文档 %s，但结果中没有找到",
								testQuery.query, expectedID)
						}
					}
				}

				// 打印查询结果用于调试
				t.Logf("PQ模式查询 '%s' 返回 %d 个结果:", testQuery.query, len(results))
				for i, result := range results {
					t.Logf("  结果 %d: ID=%s, Score=%.4f, Content=%.100s...",
						i+1, result.Document.ID, result.Score, result.Document.Content)

					// 验证结果包含元数据
					if result.Document.Metadata != nil {
						if title, ok := result.Document.Metadata["title"]; ok {
							t.Logf("    标题: %v", title)
						}
						if category, ok := result.Document.Metadata["category"]; ok {
							t.Logf("    分类: %v", category)
						}
					}
				}

				// 验证分数合理性
				for _, result := range results {
					if result.Score < -1.0 || result.Score > 1.0 {
						t.Errorf("PQ模式分数超出合理范围 [-1,1]: %f", result.Score)
					}
				}
			})
		}
	})

	// 验证转换为PQ模式后VectorStoreCollection表的CodeBookBinary和GraphBinary字段
	t.Run("验证PQ模式二进制数据", func(t *testing.T) {
		var collection schema.VectorStoreCollection

		// 查询集合记录
		err := tempDB.Where("name = ?", "test").First(&collection).Error
		if err != nil {
			t.Fatalf("查询VectorStoreCollection失败: %v", err)
		}

		// 验证CodeBookBinary不为空
		t.Run("验证CodeBookBinary", func(t *testing.T) {
			if len(collection.CodeBookBinary) == 0 {
				t.Error("CodeBookBinary为空，PQ模式转换后应该包含码本数据")
			} else {
				t.Logf("CodeBookBinary大小: %d 字节", len(collection.CodeBookBinary))

				// 验证CodeBookBinary大小合理性（不应该过大或过小）
				if len(collection.CodeBookBinary) < 10 {
					t.Errorf("CodeBookBinary大小过小: %d 字节", len(collection.CodeBookBinary))
				}
				if len(collection.CodeBookBinary) > 1024*1024 { // 1MB
					t.Errorf("CodeBookBinary大小过大: %d 字节", len(collection.CodeBookBinary))
				}
			}
		})

		// 验证GraphBinary不为空
		t.Run("验证GraphBinary", func(t *testing.T) {
			if len(collection.GraphBinary) == 0 {
				t.Error("GraphBinary为空，应该包含图结构数据")
			} else {
				t.Logf("GraphBinary大小: %d 字节", len(collection.GraphBinary))

				// 验证GraphBinary大小合理性
				if len(collection.GraphBinary) < 10 {
					t.Errorf("GraphBinary大小过小: %d 字节", len(collection.GraphBinary))
				}
				if len(collection.GraphBinary) > 1024*1024*10 { // 10MB
					t.Errorf("GraphBinary大小过大: %d 字节", len(collection.GraphBinary))
				}
			}
		})

		// 验证PQ模式标志
		t.Run("验证PQ模式标志", func(t *testing.T) {
			if !collection.EnablePQMode {
				t.Error("EnablePQMode标志应该为true")
			} else {
				t.Log("PQ模式标志正确设置为true")
			}
		})

		// 输出完整的集合信息用于调试
		t.Logf("集合完整信息:")
		t.Logf("  Name: %s", collection.Name)
		t.Logf("  Description: %s", collection.Description)
		t.Logf("  Dimension: %d", collection.Dimension)
		t.Logf("  EnablePQMode: %t", collection.EnablePQMode)
		t.Logf("  CodeBookBinary大小: %d 字节", len(collection.CodeBookBinary))
		t.Logf("  GraphBinary大小: %d 字节", len(collection.GraphBinary))
	})
}
