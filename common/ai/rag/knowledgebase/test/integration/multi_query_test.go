package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

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

// TestMultiKnowledgeBaseQuery 测试同时搜索多个知识库的Query接口
func TestMultiKnowledgeBaseQuery(t *testing.T) {
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Fatal("Failed to get database connection")
	}

	// 创建多个测试知识库
	testKBNames := []string{
		"test_multi_kb_security",
		"test_multi_kb_network",
		"test_multi_kb_development",
	}

	// 清理测试数据
	defer func() {
		t.Log("Cleaning up test data...")
		for _, kbName := range testKBNames {
			// 清理知识库条目
			db.Where("knowledge_base_id IN (SELECT id FROM knowledge_base_infos WHERE knowledge_base_name = ?)", kbName).Delete(&schema.KnowledgeBaseEntry{})
			// 清理知识库信息
			db.Where("knowledge_base_name = ?", kbName).Delete(&schema.KnowledgeBaseInfo{})
			// 清理RAG集合
			vectorstore.DeleteCollection(db, kbName)
		}
	}()

	// 创建多个知识库并添加测试数据
	kbs := make([]*knowledgebase.KnowledgeBase, 0, len(testKBNames))

	t.Log("Step 1: Creating multiple knowledge bases...")

	// 定义每个知识库的详细描述
	kbDescriptions := []string{
		"网络安全知识库：包含网络安全攻击防护、漏洞分析、安全工具使用等专业知识，主要用于安全研究人员和渗透测试工程师学习网络安全技术，涵盖SQL注入、XSS攻击、缓冲区溢出、权限提升等安全漏洞类型及其防护措施。",
		"网络协议知识库：涵盖TCP/IP协议栈、HTTP/HTTPS协议、网络路由技术等网络通信相关知识，专门为网络工程师和系统管理员提供网络协议原理、配置方法和故障排除技术，包括网络性能优化、协议分析和网络架构设计。",
		"软件开发知识库：汇集编程语言、软件架构、开发工具、项目管理等软件开发全生命周期知识，为软件开发工程师提供从需求分析到代码实现、测试部署的完整技术指导，涵盖敏捷开发、DevOps实践和代码质量管理。",
	}

	for i, kbName := range testKBNames {
		// 创建知识库，使用详细的描述
		kb, err := knowledgebase.NewKnowledgeBase(db, kbName, kbDescriptions[i], "专业技术")
		if err != nil {
			t.Fatalf("Failed to create knowledge base %s: %v", kbName, err)
		}
		kbs = append(kbs, kb)

		// 为每个知识库添加不同的测试数据
		var entries []*schema.KnowledgeBaseEntry
		switch i {
		case 0: // security knowledge base
			entries = []*schema.KnowledgeBaseEntry{
				{
					KnowledgeTitle:   "SQL注入攻击原理",
					KnowledgeDetails: "SQL注入是一种常见的网络安全漏洞，攻击者通过构造恶意SQL语句来获取数据库敏感信息。",
					KnowledgeType:    "安全",
					Summary:          "SQL注入攻击的基本原理和防护方法",
					ImportanceScore:  90,
					Keywords:         schema.StringArray{"SQL注入", "网络安全", "数据库安全"},
				},
				{
					KnowledgeTitle:   "XSS跨站脚本攻击",
					KnowledgeDetails: "XSS攻击是指攻击者在网页中插入恶意脚本代码，当用户浏览网页时执行恶意代码。",
					KnowledgeType:    "安全",
					Summary:          "XSS攻击的类型和防护策略",
					ImportanceScore:  85,
					Keywords:         schema.StringArray{"XSS", "跨站脚本", "Web安全"},
				},
			}
		case 1: // network knowledge base
			entries = []*schema.KnowledgeBaseEntry{
				{
					KnowledgeTitle:   "TCP协议详解",
					KnowledgeDetails: "TCP是传输控制协议，提供可靠的、面向连接的字节流服务。具有三次握手建立连接的特点。",
					KnowledgeType:    "网络",
					Summary:          "TCP协议的工作原理和特性",
					ImportanceScore:  95,
					Keywords:         schema.StringArray{"TCP", "网络协议", "传输层"},
				},
				{
					KnowledgeTitle:   "HTTP协议基础",
					KnowledgeDetails: "HTTP是超文本传输协议，是Web通信的基础。基于请求-响应模式工作。",
					KnowledgeType:    "网络",
					Summary:          "HTTP协议的基本概念和工作流程",
					ImportanceScore:  88,
					Keywords:         schema.StringArray{"HTTP", "Web协议", "应用层"},
				},
			}
		case 2: // development knowledge base
			entries = []*schema.KnowledgeBaseEntry{
				{
					KnowledgeTitle:   "Go语言并发编程",
					KnowledgeDetails: "Go语言通过goroutine和channel实现并发编程，提供了简洁高效的并发模型。",
					KnowledgeType:    "开发",
					Summary:          "Go语言并发编程的核心概念",
					ImportanceScore:  92,
					Keywords:         schema.StringArray{"Go语言", "并发编程", "goroutine", "channel"},
				},
				{
					KnowledgeTitle:   "Python数据分析",
					KnowledgeDetails: "Python提供了pandas、numpy等强大的数据分析库，适合进行数据处理和分析工作。",
					KnowledgeType:    "开发",
					Summary:          "Python在数据分析领域的应用",
					ImportanceScore:  80,
					Keywords:         schema.StringArray{"Python", "数据分析", "pandas", "numpy"},
				},
			}
		}

		// 批量添加知识条目
		kbInfo, err := kb.GetInfo()
		if err != nil {
			t.Fatalf("Failed to get knowledge base info %s: %v", kbName, err)
		}

		for _, entry := range entries {
			entry.KnowledgeBaseID = int64(kbInfo.ID)
			err := kb.AddKnowledgeEntry(entry)
			if err != nil {
				t.Logf("Warning: Failed to add entry to knowledge base %s: %v", kbName, err)
			}
		}

		t.Logf("Created knowledge base: %s with %d entries", kbName, len(entries))
	}

	// 等待向量索引构建完成
	t.Log("Waiting for vector indexing to complete...")
	time.Sleep(2 * time.Second)

	// 测试同时搜索多个知识库
	t.Log("Step 2: Testing multi-knowledge base query...")

	testCases := []struct {
		name          string
		query         string
		expectedTypes []string // 期望的知识类型
		minResults    int      // 最少期望结果数
	}{
		{
			name:          "Security Query",
			query:         "网络安全漏洞攻击",
			expectedTypes: []string{"安全"},
			minResults:    1,
		},
		{
			name:          "Network Query",
			query:         "网络协议TCP HTTP",
			expectedTypes: []string{"网络"},
			minResults:    1,
		},
		{
			name:          "Development Query",
			query:         "编程语言开发",
			expectedTypes: []string{"开发"},
			minResults:    1,
		},
		{
			name:          "General Query",
			query:         "技术知识",
			expectedTypes: []string{"安全", "网络", "开发"}, // 可能匹配到多种类型
			minResults:    2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing query: %s", tc.query)

			// 创建查询上下文
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// 设置查询选项
			opts := []knowledgebase.QueryOption{
				knowledgebase.WithCtx(ctx),
				knowledgebase.WithLimit(10),
				knowledgebase.WithCollectionLimit(5),
				knowledgebase.WithMsgCallBack(func(result *knowledgebase.SearchKnowledgebaseResult) {
					fmt.Printf("Query callback - Type: %s, Message: %s\n", result.Type, result.Message)
				}),
			}

			// 执行多知识库查询
			resultCh, err := knowledgebase.Query(db, tc.query, opts...)
			if err != nil {
				t.Fatalf("Failed to execute multi-KB query: %v", err)
			}

			var resultCount int
			var finalResults []*schema.KnowledgeBaseEntry
			foundTypes := make(map[string]bool)

			// 收集查询结果
			for result := range resultCh {
				switch result.Type {
				case "message":
					t.Logf("Query message: %s", result.Message)
				case "mid_result":
					resultCount++
					if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
						t.Logf("Mid result: %s (Type: %s)", entry.KnowledgeTitle, entry.KnowledgeType)
						foundTypes[entry.KnowledgeType] = true
					}
				case "result":
					if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
						finalResults = append(finalResults, entry)
						t.Logf("Final result: %s (Type: %s, Score: %d)",
							entry.KnowledgeTitle, entry.KnowledgeType, entry.ImportanceScore)
						foundTypes[entry.KnowledgeType] = true
					}
				}
			}

			// 验证结果
			if len(finalResults) < tc.minResults {
				t.Errorf("Expected at least %d results, got %d", tc.minResults, len(finalResults))
			}

			// 验证是否找到了期望的知识类型
			for _, expectedType := range tc.expectedTypes {
				if len(tc.expectedTypes) == 1 {
					// 如果只期望一种类型，必须找到
					if !foundTypes[expectedType] {
						t.Errorf("Expected to find knowledge type '%s' but didn't", expectedType)
					}
				} else {
					// 如果期望多种类型，至少找到一种即可
					if len(foundTypes) == 0 {
						t.Errorf("Expected to find at least one knowledge type from %v", tc.expectedTypes)
					}
				}
			}

			t.Logf("Query '%s' completed: %d final results, found types: %v",
				tc.query, len(finalResults), getKeys(foundTypes))
		})
	}

	// 测试带过滤器的多知识库查询
	t.Log("Step 3: Testing multi-knowledge base query with filter...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 只返回重要性分数大于85的结果
	filterOpts := []knowledgebase.QueryOption{
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithLimit(5),
		knowledgebase.WithFilter(func(key string, docGetter func() *vectorstore.Document, entryGetter func() (*schema.KnowledgeBaseEntry, error)) bool {
			entry, err := entryGetter()
			if err != nil {
				return false
			}
			return entry.ImportanceScore > 85
		}),
	}

	resultCh, err := knowledgebase.Query(db, "技术知识协议", filterOpts...)
	if err != nil {
		t.Fatalf("Failed to execute filtered multi-KB query: %v", err)
	}

	var filteredResults []*schema.KnowledgeBaseEntry
	for result := range resultCh {
		if result.Type == "result" {
			if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				filteredResults = append(filteredResults, entry)
				if entry.ImportanceScore <= 85 {
					t.Errorf("Filter failed: found entry with importance score %d (should be > 85)", entry.ImportanceScore)
				}
			}
		}
	}

	t.Logf("Filtered query completed: %d results (all with importance > 85)", len(filteredResults))

	// 测试指定集合名称的查询
	t.Log("Step 4: Testing query with specific collection name...")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	specificOpts := []knowledgebase.QueryOption{
		knowledgebase.WithCtx(ctx2),
		knowledgebase.WithCollectionName(testKBNames[0]), // 只搜索第一个知识库（安全相关）
		knowledgebase.WithLimit(5),
	}

	resultCh2, err := knowledgebase.Query(db, "攻击漏洞", specificOpts...)
	if err != nil {
		t.Fatalf("Failed to execute specific collection query: %v", err)
	}

	var specificResults []*schema.KnowledgeBaseEntry
	for result := range resultCh2 {
		if result.Type == "result" {
			if entry, ok := result.Data.(*schema.KnowledgeBaseEntry); ok {
				specificResults = append(specificResults, entry)
				// 应该只包含安全相关的结果
				if entry.KnowledgeType != "安全" {
					t.Errorf("Specific collection query failed: found non-security entry: %s (Type: %s)",
						entry.KnowledgeTitle, entry.KnowledgeType)
				}
			}
		}
	}

	t.Logf("Specific collection query completed: %d results (all should be security-related)", len(specificResults))

	t.Logf("Multi-knowledge base query test completed successfully with %d knowledge bases!", len(kbs))
}

// TestMultiKnowledgeBaseQueryPerformance 测试多知识库查询的性能
func TestMultiKnowledgeBaseQueryPerformance(t *testing.T) {
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Fatal("Failed to get database connection")
	}

	// 创建较多的知识库用于性能测试
	numKBs := 5
	entriesPerKB := 10
	testKBNames := make([]string, numKBs)

	for i := 0; i < numKBs; i++ {
		testKBNames[i] = utils.RandStringBytes(8) + "_perf_test"
	}

	// 清理测试数据
	defer func() {
		t.Log("Cleaning up performance test data...")
		for _, kbName := range testKBNames {
			db.Where("knowledge_base_id IN (SELECT id FROM knowledge_base_infos WHERE knowledge_base_name = ?)", kbName).Delete(&schema.KnowledgeBaseEntry{})
			db.Where("knowledge_base_name = ?", kbName).Delete(&schema.KnowledgeBaseInfo{})
			vectorstore.DeleteCollection(db, kbName)
		}
	}()

	t.Logf("Creating %d knowledge bases with %d entries each...", numKBs, entriesPerKB)
	start := time.Now()

	// 定义性能测试知识库的详细描述
	perfKBDescriptions := []string{
		"云计算技术知识库：专注于云原生架构、容器化技术、微服务架构等现代云计算技术栈，为云架构师和DevOps工程师提供AWS、Azure、Kubernetes等平台的实战指导和最佳实践。",
		"人工智能算法知识库：涵盖机器学习、深度学习、自然语言处理等AI技术领域，为算法工程师和数据科学家提供从理论基础到实际应用的完整技术体系，包括TensorFlow、PyTorch等框架使用。",
		"数据库系统知识库：包含关系型数据库、NoSQL数据库、分布式数据库等数据存储技术，为数据库管理员和后端开发工程师提供MySQL、PostgreSQL、MongoDB等数据库的设计、优化和运维知识。",
		"移动应用开发知识库：专门收录iOS、Android移动应用开发技术，为移动开发工程师提供原生开发、跨平台开发、移动UI设计等全栈移动开发技术指导和项目实战经验。",
		"区块链技术知识库：汇集区块链原理、智能合约开发、加密货币技术等前沿技术，为区块链开发工程师提供以太坊、比特币、Hyperledger等平台的开发实践和技术架构知识。",
	}

	// 创建知识库和数据
	for i, kbName := range testKBNames {
		description := perfKBDescriptions[i%len(perfKBDescriptions)]
		kb, err := knowledgebase.NewKnowledgeBase(db, kbName, description, "技术专业")
		if err != nil {
			t.Fatalf("Failed to create KB %d: %v", i, err)
		}

		// 为每个知识库添加条目
		kbInfo, err := kb.GetInfo()
		if err != nil {
			t.Fatalf("Failed to get knowledge base info for KB %d: %v", i, err)
		}

		for j := 0; j < entriesPerKB; j++ {
			title := utils.RandStringBytes(10) + " knowledge"
			details := "This is test knowledge entry number " + utils.RandStringBytes(20)
			entry := &schema.KnowledgeBaseEntry{
				KnowledgeBaseID:  int64(kbInfo.ID),
				KnowledgeTitle:   title,
				KnowledgeDetails: details,
				KnowledgeType:    "test",
				Summary:          "test summary",
				ImportanceScore:  80 + j,
				Keywords:         schema.StringArray{"test", "knowledge"},
			}
			err := kb.AddKnowledgeEntry(entry)
			if err != nil {
				t.Logf("Warning: Failed to add entry %d to KB %d: %v", j, i, err)
			}
		}
	}

	setupTime := time.Since(start)
	t.Logf("Setup completed in %v", setupTime)

	// 等待索引构建
	time.Sleep(3 * time.Second)

	// 执行性能测试
	t.Log("Starting performance test...")
	start = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := []knowledgebase.QueryOption{
		knowledgebase.WithCtx(ctx),
		knowledgebase.WithLimit(20),
		knowledgebase.WithCollectionLimit(numKBs),
	}

	resultCh, err := knowledgebase.Query(db, "test knowledge", opts...)
	if err != nil {
		t.Fatalf("Performance query failed: %v", err)
	}

	var totalResults int
	for result := range resultCh {
		if result.Type == "result" {
			totalResults++
		}
	}

	queryTime := time.Since(start)
	t.Logf("Performance test completed:")
	t.Logf("  - Setup time: %v", setupTime)
	t.Logf("  - Query time: %v", queryTime)
	t.Logf("  - Knowledge bases: %d", numKBs)
	t.Logf("  - Entries per KB: %d", entriesPerKB)
	t.Logf("  - Total entries: %d", numKBs*entriesPerKB)
	t.Logf("  - Results found: %d", totalResults)
	t.Logf("  - Query throughput: %.2f results/second", float64(totalResults)/queryTime.Seconds())

	// 性能断言 - 查询时间不应超过30秒
	if queryTime > 30*time.Second {
		t.Errorf("Query took too long: %v (should be < 30s)", queryTime)
	}

	// 应该找到一些结果
	if totalResults == 0 {
		t.Error("No results found in performance test")
	}
}

// getKeys 获取map的所有键
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
