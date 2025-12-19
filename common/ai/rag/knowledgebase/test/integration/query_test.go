package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	// 直接导入以触发 init 函数，替代原来的 depinjector
	_ "github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	_ "github.com/yaklang/yaklang/common/aiforge"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
)

func init() {
	yakit.LoadGlobalNetworkConfig()
}

// TestKnowledgeBaseQuery 测试知识库查询接口
// 包括：
// 1. 基础向量搜索
// 2. 带相似度分数的搜索
// 3. 增强搜索（带假设文档生成）
// 4. 智能问答查询
// 5. 分页查询
// 6. 过滤查询
// 7. 关键词搜索
func TestKnowledgeBaseQuery(t *testing.T) {
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Fatal("Failed to get database connection")
	}

	// 测试知识库名称
	testKBName := "test_query_operations"

	// 清理测试数据
	defer func() {
		t.Log("Cleaning up test data...")

		// 清理知识库条目
		db.Where("knowledge_base_id IN (SELECT id FROM knowledge_base_infos WHERE knowledge_base_name = ?)", testKBName).Delete(&schema.KnowledgeBaseEntry{})

		// 清理知识库
		db.Where("knowledge_base_name = ?", testKBName).Delete(&schema.KnowledgeBaseInfo{})

		// 清理向量集合
		vectorstore.DeleteCollection(db, testKBName)
	}()

	// 准备测试数据
	kb := setupQueryTestData(t, db, testKBName)

	t.Run("TestBasicVectorSearch", func(t *testing.T) {
		testBasicVectorSearch(t, kb)
	})

	t.Run("TestEnhancedSearch", func(t *testing.T) {
		testEnhancedSearch(t, kb)
	})

	t.Run("TestIntelligentQA", func(t *testing.T) {
		testIntelligentQA(t, kb)
	})

	t.Run("TestPaginatedQuery", func(t *testing.T) {
		testPaginatedQuery(t, kb)
	})

	t.Run("TestFilteredQuery", func(t *testing.T) {
		testFilteredQuery(t, kb)
	})

	t.Run("TestKeywordSearch", func(t *testing.T) {
		testKeywordSearch(t, kb)
	})

	t.Run("TestQueryOptions", func(t *testing.T) {
		testQueryOptions(t, kb)
	})
}

// setupQueryTestData 设置查询测试所需的数据
func setupQueryTestData(t *testing.T, db *gorm.DB, kbName string) *knowledgebase.KnowledgeBase {
	t.Log("=== Setting up test data for query operations ===")

	// 创建知识库
	kb, err := knowledgebase.NewKnowledgeBase(db, kbName, "Test KB for query operations", "test")
	if err != nil {
		t.Fatalf("Failed to create knowledge base: %v", err)
	}

	// 准备测试数据
	kbInfo, _ := kb.GetInfo()
	testEntries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "机器学习基础",
			KnowledgeType:    "技术",
			ImportanceScore:  9,
			Keywords:         []string{"机器学习", "人工智能", "算法", "模型"},
			KnowledgeDetails: "机器学习是人工智能的一个分支，它使计算机能够在没有明确编程的情况下学习和改进。主要包括监督学习、无监督学习和强化学习三种类型。常用算法包括线性回归、决策树、神经网络等。",
			Summary:          "机器学习概念、类型和常用算法介绍",
			PotentialQuestions: []string{
				"什么是机器学习？",
				"机器学习有哪些类型？",
				"常用的机器学习算法有哪些？",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "深度学习与神经网络",
			KnowledgeType:    "技术",
			ImportanceScore:  10,
			Keywords:         []string{"深度学习", "神经网络", "卷积神经网络", "循环神经网络"},
			KnowledgeDetails: "深度学习是机器学习的一个子领域，基于人工神经网络进行学习。深度神经网络通过多层神经元模拟人脑神经连接，能够自动学习数据的复杂特征。包括CNN用于图像识别，RNN用于序列数据处理。",
			Summary:          "深度学习原理、神经网络结构和应用场景",
			PotentialQuestions: []string{
				"深度学习和机器学习的区别是什么？",
				"神经网络是如何工作的？",
				"CNN和RNN分别适用于什么场景？",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "自然语言处理技术",
			KnowledgeType:    "技术",
			ImportanceScore:  8,
			Keywords:         []string{"自然语言处理", "NLP", "文本分析", "语言模型"},
			KnowledgeDetails: "自然语言处理(NLP)是人工智能领域的重要分支，专注于让计算机理解、解释和生成人类语言。主要技术包括分词、词性标注、命名实体识别、情感分析、机器翻译等。现代NLP大量使用Transformer架构。",
			Summary:          "NLP技术概述、主要任务和现代发展",
			PotentialQuestions: []string{
				"什么是自然语言处理？",
				"NLP的主要任务有哪些？",
				"Transformer架构的优势是什么？",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "计算机视觉基础",
			KnowledgeType:    "技术",
			ImportanceScore:  8,
			Keywords:         []string{"计算机视觉", "图像处理", "目标检测", "图像分类"},
			KnowledgeDetails: "计算机视觉是让计算机获得人类视觉能力的技术领域。主要任务包括图像分类、目标检测、语义分割、人脸识别等。深度学习特别是卷积神经网络在计算机视觉中取得了突破性进展。",
			Summary:          "计算机视觉技术、主要任务和深度学习应用",
			PotentialQuestions: []string{
				"计算机视觉有哪些应用？",
				"目标检测和图像分类的区别？",
				"卷积神经网络为什么适合图像处理？",
			},
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "强化学习原理",
			KnowledgeType:    "技术",
			ImportanceScore:  7,
			Keywords:         []string{"强化学习", "智能体", "环境交互", "奖励机制"},
			KnowledgeDetails: "强化学习是机器学习的一种范式，智能体通过与环境交互学习最优策略。核心概念包括状态、动作、奖励、策略等。著名算法包括Q-learning、策略梯度、Actor-Critic等。在游戏AI、机器人控制等领域应用广泛。",
			Summary:          "强化学习基本概念、算法和应用领域",
			PotentialQuestions: []string{
				"强化学习的基本要素有哪些？",
				"Q-learning算法是如何工作的？",
				"强化学习在哪些领域有应用？",
			},
		},
	}

	// 添加测试数据到知识库
	for i, entry := range testEntries {
		err = kb.AddKnowledgeEntry(entry)
		if err != nil {
			t.Fatalf("Failed to add test entry %d: %v", i+1, err)
		}
		t.Logf("Added test entry: %s (ID: %d)", entry.KnowledgeTitle, entry.ID)
	}

	// 验证数据已添加
	count, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to count documents: %v", err)
	}
	t.Logf("Test data setup complete. Total documents: %d", count)

	return kb
}

// testBasicVectorSearch 测试基础向量搜索
func testBasicVectorSearch(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Basic Vector Search ===")

	testCases := []struct {
		query          string
		expectedCount  int
		expectedTopics []string
	}{
		{
			query:          "机器学习算法",
			expectedCount:  1, // 应该匹配机器学习
			expectedTopics: []string{"机器学习"},
		},
		{
			query:          "图像识别",
			expectedCount:  2, // 应该匹配计算机视觉和深度学习(CNN)
			expectedTopics: []string{"计算机视觉", "深度学习"},
		},
		{
			query:          "语言处理",
			expectedCount:  1, // 应该主要匹配NLP
			expectedTopics: []string{"自然语言处理"},
		},
		{
			query:          "强化学习",
			expectedCount:  1, // 应该匹配强化学习
			expectedTopics: []string{"强化学习"},
		},
	}

	for i, tc := range testCases {
		t.Logf("Test case %d: Query='%s'", i+1, tc.query)

		results, err := kb.SearchKnowledgeEntries(tc.query, tc.expectedCount)
		if err != nil {
			t.Errorf("Search failed for query '%s': %v", tc.query, err)
			continue
		}

		t.Logf("Found %d results for query '%s'", len(results), tc.query)

		if len(results) == 0 {
			t.Errorf("Expected at least some results for query '%s', got 0", tc.query)
			continue
		}

		// 验证结果包含预期主题
		foundTopics := make(map[string]bool)
		for _, result := range results {
			t.Logf("  - %s (Score: N/A)", result.KnowledgeTitle)
			for _, topic := range tc.expectedTopics {
				if strings.Contains(result.KnowledgeTitle, topic) ||
					strings.Contains(result.KnowledgeDetails, topic) {
					foundTopics[topic] = true
				}
			}
		}

		// 检查是否找到了预期的主题
		for _, expectedTopic := range tc.expectedTopics {
			if !foundTopics[expectedTopic] {
				t.Errorf("Expected topic '%s' not found in results for query '%s'", expectedTopic, tc.query)
			}
		}
	}
}

// testEnhancedSearch 测试增强搜索（带假设文档生成）
func testEnhancedSearch(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Enhanced Search ===")

	query := "如何选择合适的机器学习算法？"

	// 使用context来控制超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 收集搜索过程中的消息
	var messages []string
	var results []*schema.KnowledgeBaseEntry

	resultCh, err := kb.SearchKnowledgeEntriesWithEnhance(query,
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithLimit(3),
		knowledgebase.WithMsgCallBack(func(result *knowledgebase.SearchKnowledgebaseResult) {
			t.Logf("Enhanced search message: %s", result.Message)
			messages = append(messages, result.Message)
		}),
	)
	if err != nil {
		t.Fatalf("Enhanced search failed: %v", err)
	}

	// 收集所有结果
	for result := range resultCh {
		if result.Type == "result" {
			if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				results = append(results, entry)
				t.Logf("Enhanced search result: %s", entry.KnowledgeTitle)
			}
		}
	}

	// 验证搜索过程
	if len(messages) == 0 {
		t.Error("Expected search process messages, got none")
	}

	// 验证关键消息
	hasHypothetical := false
	hasSearchStart := false
	for _, msg := range messages {
		if strings.Contains(msg, "假设文档") {
			hasHypothetical = true
		}
		if strings.Contains(msg, "开始搜索") {
			hasSearchStart = true
		}
	}

	if !hasHypothetical {
		t.Error("Expected hypothetical document generation message")
	}
	if !hasSearchStart {
		t.Error("Expected search start message")
	}

	// 验证搜索结果
	if len(results) == 0 {
		t.Error("Expected enhanced search results, got 0")
		return
	}

	t.Logf("Enhanced search returned %d results", len(results))

	// 验证结果相关性
	relevantCount := 0
	for _, result := range results {
		if strings.Contains(result.KnowledgeTitle, "机器学习") ||
			strings.Contains(result.KnowledgeDetails, "算法") {
			relevantCount++
		}
	}

	if relevantCount == 0 {
		t.Error("No relevant results found in enhanced search")
	}
}

// testIntelligentQA 测试智能问答查询
func testIntelligentQA(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Intelligent QA ===")

	// 测试问题
	questions := []string{
		"什么是深度学习？",
		"机器学习有哪些主要类型？",
		"计算机视觉的应用有哪些？",
	}

	for i, question := range questions {
		t.Logf("QA Test %d: Question='%s'", i+1, question)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		answer, err := kb.Query(question,
			knowledgebase.WithCtx(ctx),
			knowledgebase.WithLimit(2),
		)

		if err != nil {
			t.Errorf("QA query failed for question '%s': %v", question, err)
			continue
		}

		t.Logf("Answer length: %d characters", len(answer))

		if len(answer) == 0 {
			t.Errorf("Expected non-empty answer for question '%s'", question)
			continue
		}

		// 简单验证答案质量
		if len(answer) < 10 {
			t.Errorf("Answer seems too short for question '%s': %s", question, answer)
		}

		t.Logf("Answer preview: %s...", answer[:min(100, len(answer))])
	}
}

// testPaginatedQuery 测试分页查询
func testPaginatedQuery(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Paginated Query ===")

	// 测试ListKnowledgeEntries分页功能
	testCases := []struct {
		keyword string
		page    int
		limit   int
	}{
		{"", 1, 3},   // 获取前3个
		{"", 2, 2},   // 获取第二页的2个
		{"学习", 1, 5}, // 关键词搜索
	}

	for i, tc := range testCases {
		t.Logf("Pagination test %d: keyword='%s', page=%d, limit=%d", i+1, tc.keyword, tc.page, tc.limit)

		results, err := kb.ListKnowledgeEntries(tc.keyword, tc.page, tc.limit)
		if err != nil {
			t.Errorf("Paginated query failed: %v", err)
			continue
		}

		t.Logf("Found %d results for page %d", len(results), tc.page)

		// 验证结果数量不超过限制
		if len(results) > tc.limit {
			t.Errorf("Results count %d exceeds limit %d", len(results), tc.limit)
		}

		// 验证关键词过滤
		if tc.keyword != "" {
			for _, result := range results {
				if !strings.Contains(result.KnowledgeTitle, tc.keyword) &&
					!strings.Contains(result.KnowledgeDetails, tc.keyword) &&
					!containsKeyword(result.Keywords, tc.keyword) {
					t.Logf("Result may not match keyword '%s': %s", tc.keyword, result.KnowledgeTitle)
				}
			}
		}

		for _, result := range results {
			t.Logf("  - %s", result.KnowledgeTitle)
		}
	}
}

// testFilteredQuery 测试过滤查询
func testFilteredQuery(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Filtered Query ===")

	query := "机器学习技术"

	// 使用过滤器只返回重要性评分>=8的结果
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var filteredResults []*schema.KnowledgeBaseEntry
	var allResults []*schema.KnowledgeBaseEntry

	// 先测试无过滤器的搜索
	resultCh1, err := kb.SearchKnowledgeEntriesWithEnhance(query,
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithLimit(5),
	)
	if err != nil {
		t.Fatalf("Unfiltered search failed: %v", err)
	}

	for result := range resultCh1 {
		if result.Type == "result" {
			if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				allResults = append(allResults, entry)
			}
		}
	}

	// 测试带过滤器的搜索
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	resultCh2, err := kb.SearchKnowledgeEntriesWithEnhance(query,
		knowledgebase.WithCtx(ctx2),
		knowledgebase.WithLimit(5),
		knowledgebase.WithFilter(func(key string, docGetter func() *vectorstore.Document, entryGetter func() (*schema.KnowledgeBaseEntry, error)) bool {
			entry, err := entryGetter()
			if err != nil {
				return false
			}
			// 只返回重要性评分>=8的条目
			return entry.ImportanceScore >= 8
		}),
	)
	if err != nil {
		t.Fatalf("Filtered search failed: %v", err)
	}

	for result := range resultCh2 {
		if result.Type == "result" {
			if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				filteredResults = append(filteredResults, entry)
			}
		}
	}

	t.Logf("Unfiltered results: %d", len(allResults))
	t.Logf("Filtered results: %d", len(filteredResults))

	// 验证过滤器效果
	for _, result := range filteredResults {
		if result.ImportanceScore < 8 {
			t.Errorf("Filtered result has importance score %d, expected >=8", result.ImportanceScore)
		}
		t.Logf("  - %s (Score: %d)", result.KnowledgeTitle, result.ImportanceScore)
	}

	// 过滤后的结果应该不多于未过滤的结果
	if len(filteredResults) > len(allResults) {
		t.Error("Filtered results should not exceed unfiltered results")
	}
}

// testKeywordSearch 测试关键词搜索
func testKeywordSearch(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Keyword Search ===")

	// 测试不同关键词的搜索效果
	keywordTests := []struct {
		keyword       string
		expectedCount int
	}{
		{"深度学习", 1}, // 应该主要匹配深度学习条目
		{"神经网络", 1}, // 应该匹配深度学习
		{"图像", 1},   // 应该匹配计算机视觉
		{"语言", 1},   // 应该匹配NLP
		{"算法", 2},   // 应该匹配多个条目
	}

	for _, test := range keywordTests {
		t.Logf("Keyword search test: '%s'", test.keyword)

		// 使用ListKnowledgeEntries进行关键词搜索
		results, err := kb.ListKnowledgeEntries(test.keyword, 1, 10)
		if err != nil {
			t.Errorf("Keyword search failed for '%s': %v", test.keyword, err)
			continue
		}

		t.Logf("Keyword '%s' found %d results", test.keyword, len(results))

		if len(results) == 0 {
			t.Errorf("Expected results for keyword '%s', got 0", test.keyword)
			continue
		}

		// 验证结果相关性
		for _, result := range results {
			relevant := strings.Contains(result.KnowledgeTitle, test.keyword) ||
				strings.Contains(result.KnowledgeDetails, test.keyword) ||
				containsKeyword(result.Keywords, test.keyword)

			if !relevant {
				t.Logf("Result may not be relevant to keyword '%s': %s", test.keyword, result.KnowledgeTitle)
			}

			t.Logf("  - %s", result.KnowledgeTitle)
		}
	}
}

// testQueryOptions 测试查询选项
func testQueryOptions(t *testing.T, kb *knowledgebase.KnowledgeBase) {
	t.Log("=== Testing Query Options ===")

	query := "机器学习算法"

	// 测试不同的限制选项
	limits := []int{1, 2, 5}

	for _, limit := range limits {
		t.Logf("Testing limit: %d", limit)

		results, err := kb.SearchKnowledgeEntries(query, limit)
		if err != nil {
			t.Errorf("Search with limit %d failed: %v", limit, err)
			continue
		}

		t.Logf("Limit %d returned %d results", limit, len(results))

		// 验证结果数量不超过限制
		if len(results) > limit {
			t.Errorf("Results count %d exceeds limit %d", len(results), limit)
		}
	}

	// 测试上下文超时
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond) // 很短的超时
	defer cancel()

	_, err := kb.SearchKnowledgeEntriesWithEnhance(query,
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithLimit(1),
	)

	// 期望超时错误或快速完成
	if err != nil {
		t.Logf("Context timeout test: %v (expected)", err)
	} else {
		t.Log("Context timeout test: completed quickly")
	}
}

// 辅助函数
func containsKeyword(keywords []string, target string) bool {
	for _, keyword := range keywords {
		if strings.Contains(keyword, target) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
