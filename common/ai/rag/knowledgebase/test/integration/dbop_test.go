package integration

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"gotest.tools/v3/assert"

	_ "github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
	_ "github.com/yaklang/yaklang/common/aiforge"
)

// TestKnowledgeBaseDBOperation 测试知识库和向量存储的同步操作
// 包括：
// 1. 知识库的CRUD操作
// 2. 知识条目的CRUD操作
// 3. 级联删除操作
// 4. 向量存储同步（如果向量存储可用）
//
// 主要验证以下同步行为：
// - 创建知识后向量存储应该同步增加向量
// - 更新知识后向量存储应该同步更新向量
// - 删除知识后向量存储应该同步删除向量
// - 删除知识库后向量存储应该删除整个集合和所有向量
func TestKnowledgeBaseDBOperation(t *testing.T) {
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Fatal("Failed to get database connection")
	}

	// 清理测试数据
	testKBName := "test_kb_sync_operations"
	defer func() {
		t.Log("Cleaning up test data...")

		// 清理知识库条目
		db.Where("knowledge_base_id IN (SELECT id FROM knowledge_base_infos WHERE knowledge_base_name = ?)", testKBName).Delete(&schema.KnowledgeBaseEntry{})

		// 清理知识库
		db.Where("knowledge_base_name = ?", testKBName).Delete(&schema.KnowledgeBaseInfo{})

		// 清理向量集合
		vectorstore.DeleteCollection(db, testKBName)
	}()

	t.Run("TestKnowledgeBaseCreationAndVectorSync", func(t *testing.T) {
		testKnowledgeBaseCreationAndVectorSync(t, db, testKBName)
	})

	t.Run("TestKnowledgeEntryAdditionAndVectorSync", func(t *testing.T) {
		testKnowledgeEntryAdditionAndVectorSync(t, db, testKBName)
	})

	t.Run("TestKnowledgeEntryUpdateAndVectorSync", func(t *testing.T) {
		testKnowledgeEntryUpdateAndVectorSync(t, db, testKBName)
	})

	t.Run("TestKnowledgeEntryDeletionAndVectorSync", func(t *testing.T) {
		testKnowledgeEntryDeletionAndVectorSync(t, db, testKBName)
	})

	t.Run("TestKnowledgeBaseCascadeDelete", func(t *testing.T) {
		testKnowledgeBaseCascadeDelete(t, db, testKBName)
	})
}

// testKnowledgeBaseCreationAndVectorSync 测试知识库创建时的向量同步
func testKnowledgeBaseCreationAndVectorSync(t *testing.T, db *gorm.DB, kbName string) {
	t.Log("=== Testing Knowledge Base Creation and Vector Sync ===")

	// 1. 创建知识库
	kb, err := knowledgebase.NewKnowledgeBase(db, kbName, "Test KB for sync operations", "test")
	if err != nil {
		t.Fatalf("Failed to create knowledge base: %v", err)
	}

	// 2. 验证知识库在数据库中已创建
	kbInfo, err := kb.GetInfo()
	if err != nil {
		t.Fatalf("Failed to get knowledge base info: %v", err)
	}

	if kbInfo.KnowledgeBaseName != kbName {
		t.Errorf("Expected KB name %s, got %s", kbName, kbInfo.KnowledgeBaseName)
	}

	t.Logf("Knowledge base created: %s (ID: %d)", kbInfo.KnowledgeBaseName, kbInfo.ID)

	// 3. 验证向量集合已创建
	exists := vectorstore.HasCollection(db, kbName)
	if !exists {
		t.Errorf("Vector collection %s should exist after creating knowledge base", kbName)
	} else {
		t.Logf("Vector collection %s created successfully", kbName)
	}

	// 4. 验证向量集合初始状态
	collectionMg := kb.GetVectorStore()
	if collectionMg == nil {
		t.Error("vector store should be available")
	} else {
		count, err := collectionMg.Count()
		if err != nil {
			t.Errorf("Failed to count documents: %v", err)
		} else {
			t.Logf("Initial document count in vector store: %d", count)
		}
		assert.Equal(t, count, 0)
	}
}

// testKnowledgeEntryAdditionAndVectorSync 测试知识条目添加时的向量同步
func testKnowledgeEntryAdditionAndVectorSync(t *testing.T, db *gorm.DB, kbName string) {
	t.Log("=== Testing Knowledge Entry Addition and Vector Sync ===")

	// 1. 加载知识库
	kb, err := knowledgebase.LoadKnowledgeBase(db, kbName)
	if err != nil {
		t.Fatalf("Failed to load knowledge base: %v", err)
	}

	// 2. 获取初始文档数量
	initialCount, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to get initial document count: %v", err)
	}
	t.Logf("Initial document count: %d", initialCount)

	// 3. 添加知识条目
	kbInfo, _ := kb.GetInfo()
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  int64(kbInfo.ID),
		KnowledgeTitle:   "Machine Learning Basics",
		KnowledgeType:    "Technology",
		ImportanceScore:  8,
		Keywords:         []string{"machine learning", "AI", "algorithms"},
		KnowledgeDetails: "Machine learning is a subset of artificial intelligence that focuses on algorithms and statistical models that computer systems use to perform tasks without explicit instructions.",
		Summary:          "Introduction to machine learning concepts and applications",
		PotentialQuestions: []string{
			"What is machine learning?",
			"How does machine learning work?",
			"What are the types of machine learning?",
		},
	}

	err = kb.AddKnowledgeEntry(entry)
	if err != nil {
		t.Fatalf("Failed to add knowledge entry: %v", err)
	}

	t.Logf("Knowledge entry added: %s (ID: %d)", entry.KnowledgeTitle, entry.ID)

	// 4. 验证知识条目在数据库中已创建
	retrievedEntry, err := kb.GetKnowledgeEntry(entry.HiddenIndex)
	if err != nil {
		t.Fatalf("Failed to retrieve knowledge entry: %v", err)
	}

	if retrievedEntry.KnowledgeTitle != entry.KnowledgeTitle {
		t.Errorf("Expected title %s, got %s", entry.KnowledgeTitle, retrievedEntry.KnowledgeTitle)
	}

	// 5. 验证向量已同步添加
	newCount, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to get new document count: %v", err)
	}

	expectedCount := initialCount + 1
	if newCount != expectedCount {
		t.Errorf("Expected document count %d, got %d", expectedCount, newCount)
	} else {
		t.Logf("Vector successfully added. Document count: %d -> %d", initialCount, newCount)
	}

	// 6. 验证向量内容
	collectionMg := kb.GetVectorStore()
	doc, exists, err := collectionMg.Get(entry.HiddenIndex)
	if err != nil {
		t.Errorf("Failed to get document from vector store: %v", err)
	} else if !exists {
		t.Error("Document should exist in vector store")
	} else {
		t.Logf("Vector document found: ID=%s, Content length=%d", doc.ID, len(doc.Content))

		// 验证向量内容包含知识条目信息
		if doc.Content == "" {
			t.Error("Vector document content should not be empty")
		}
	}
}

// testKnowledgeEntryUpdateAndVectorSync 测试知识条目更新时的向量同步
func testKnowledgeEntryUpdateAndVectorSync(t *testing.T, db *gorm.DB, kbName string) {
	t.Log("=== Testing Knowledge Entry Update and Vector Sync ===")

	// 1. 加载知识库
	kb, err := knowledgebase.LoadKnowledgeBase(db, kbName)
	if err != nil {
		t.Fatalf("Failed to load knowledge base: %v", err)
	}

	// 2. 获取第一个知识条目
	entries, err := kb.ListKnowledgeEntries("", 1, 10)
	if err != nil || len(entries) == 0 {
		t.Fatal("No knowledge entries found for update test")
	}

	entry := entries[0]
	originalTitle := entry.KnowledgeTitle
	originalDetails := entry.KnowledgeDetails

	t.Logf("Updating entry: %s (ID: %d)", originalTitle, entry.ID)

	// 3. 更新知识条目
	entry.KnowledgeTitle = "Advanced Machine Learning Concepts"
	entry.KnowledgeDetails = "Advanced machine learning covers deep learning, neural networks, reinforcement learning, and other sophisticated algorithms that can learn complex patterns from large datasets."
	entry.Summary = "Advanced ML techniques including deep learning and neural networks"

	err = kb.UpdateKnowledgeEntry(entry.HiddenIndex, entry)
	if err != nil {
		t.Fatalf("Failed to update knowledge entry: %v", err)
	}

	// 4. 验证数据库中的更新
	updatedEntry, err := kb.GetKnowledgeEntry(entry.HiddenIndex)
	if err != nil {
		t.Fatalf("Failed to retrieve updated entry: %v", err)
	}

	if updatedEntry.KnowledgeTitle == originalTitle {
		t.Error("Knowledge entry title should have been updated")
	}

	if updatedEntry.KnowledgeTitle != entry.KnowledgeTitle {
		t.Errorf("Expected updated title %s, got %s", entry.KnowledgeTitle, updatedEntry.KnowledgeTitle)
	}

	t.Logf("Entry updated: %s -> %s", originalTitle, updatedEntry.KnowledgeTitle)

	// 5. 验证向量已同步更新
	collectionMg := kb.GetVectorStore()
	doc, exists, err := collectionMg.Get(entry.HiddenIndex)
	if err != nil {
		t.Errorf("Failed to get updated document from vector store: %v", err)
	} else if !exists {
		t.Error("Updated document should exist in vector store")
	} else {
		t.Logf("Updated vector document found: ID=%s, Content length=%d", doc.ID, len(doc.Content))

		// 验证向量内容已更新
		if doc.Content == originalDetails {
			t.Error("Vector content should have been updated")
		}
	}
}

// testKnowledgeEntryDeletionAndVectorSync 测试知识条目删除时的向量同步
func testKnowledgeEntryDeletionAndVectorSync(t *testing.T, db *gorm.DB, kbName string) {
	t.Log("=== Testing Knowledge Entry Deletion and Vector Sync ===")

	// 1. 加载知识库
	kb, err := knowledgebase.LoadKnowledgeBase(db, kbName)
	if err != nil {
		t.Fatalf("Failed to load knowledge base: %v", err)
	}

	// 2. 添加一个临时知识条目用于删除测试
	kbInfo, _ := kb.GetInfo()
	tempEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  int64(kbInfo.ID),
		KnowledgeTitle:   "Temporary Entry for Deletion Test",
		KnowledgeType:    "Test",
		ImportanceScore:  5,
		Keywords:         []string{"test", "deletion"},
		KnowledgeDetails: "This entry will be deleted to test vector synchronization.",
		Summary:          "Temporary entry for deletion testing",
	}

	err = kb.AddKnowledgeEntry(tempEntry)
	if err != nil {
		t.Fatalf("Failed to add temporary entry: %v", err)
	}

	t.Logf("Temporary entry added: %s (ID: %d)", tempEntry.KnowledgeTitle, tempEntry.ID)

	// 3. 获取删除前的文档数量
	beforeCount, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to get document count before deletion: %v", err)
	}

	// 4. 验证向量存在
	collectionMg := kb.GetVectorStore()
	_, exists, err := collectionMg.Get(tempEntry.HiddenIndex)
	if err != nil {
		t.Errorf("Failed to check document existence: %v", err)
	} else if !exists {
		t.Error("Document should exist before deletion")
	}

	// 5. 删除知识条目
	err = kb.DeleteKnowledgeEntry(tempEntry.HiddenIndex)
	if err != nil {
		t.Fatalf("Failed to delete knowledge entry: %v", err)
	}

	t.Logf("Entry deleted: %s", tempEntry.KnowledgeTitle)

	// 6. 验证数据库中的删除
	_, err = kb.GetKnowledgeEntry(tempEntry.HiddenIndex)
	if err == nil {
		t.Error("Knowledge entry should have been deleted from database")
	}

	// 7. 验证向量已同步删除
	afterCount, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to get document count after deletion: %v", err)
	}

	expectedCount := beforeCount - 1
	if afterCount != expectedCount {
		t.Errorf("Expected document count %d after deletion, got %d", expectedCount, afterCount)
	} else {
		t.Logf("Vector successfully deleted. Document count: %d -> %d", beforeCount, afterCount)
	}

	// 8. 验证具体向量已删除
	_, exists, err = collectionMg.Get(tempEntry.HiddenIndex)
	if err != nil {
		t.Errorf("Failed to check document after deletion: %v", err)
	} else if exists {
		t.Error("Document should have been deleted from vector store")
	} else {
		t.Log("Vector document successfully deleted from store")
	}
}

// testKnowledgeBaseCascadeDelete 测试知识库级联删除
func testKnowledgeBaseCascadeDelete(t *testing.T, db *gorm.DB, kbName string) {
	t.Log("=== Testing Knowledge Base Cascade Delete ===")

	// 1. 加载知识库
	kb, err := knowledgebase.LoadKnowledgeBase(db, kbName)
	if err != nil {
		t.Fatalf("Failed to load knowledge base: %v", err)
	}

	// 2. 添加多个知识条目
	kbInfo, _ := kb.GetInfo()
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "Deep Learning",
			KnowledgeType:    "Technology",
			ImportanceScore:  9,
			Keywords:         []string{"deep learning", "neural networks"},
			KnowledgeDetails: "Deep learning is a subset of machine learning based on artificial neural networks with multiple layers.",
			Summary:          "Introduction to deep learning and neural networks",
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "Natural Language Processing",
			KnowledgeType:    "Technology",
			ImportanceScore:  8,
			Keywords:         []string{"NLP", "language", "processing"},
			KnowledgeDetails: "Natural Language Processing is a field of AI that focuses on the interaction between computers and human language.",
			Summary:          "Overview of NLP techniques and applications",
		},
		{
			KnowledgeBaseID:  int64(kbInfo.ID),
			KnowledgeTitle:   "Computer Vision",
			KnowledgeType:    "Technology",
			ImportanceScore:  8,
			Keywords:         []string{"computer vision", "image processing"},
			KnowledgeDetails: "Computer vision is a field of AI that trains computers to interpret and understand the visual world.",
			Summary:          "Introduction to computer vision and image processing",
		},
	}

	for i, entry := range entries {
		err = kb.AddKnowledgeEntry(entry)
		if err != nil {
			t.Fatalf("Failed to add entry %d: %v", i+1, err)
		}
		t.Logf("Added entry: %s (ID: %d)", entry.KnowledgeTitle, entry.ID)
	}

	// 3. 获取删除前的状态
	beforeDocCount, err := kb.CountDocuments()
	if err != nil {
		t.Fatalf("Failed to get document count before cascade delete: %v", err)
	}

	beforeEntries, err := kb.ListKnowledgeEntries("", 1, 100)
	if err != nil {
		t.Fatalf("Failed to list entries before cascade delete: %v", err)
	}

	t.Logf("Before cascade delete - Entries: %d, Documents: %d", len(beforeEntries), beforeDocCount)

	// 4. 验证向量集合存在
	exists := vectorstore.HasCollection(db, kbName)
	if !exists {
		t.Error("Vector collection should exist before deletion")
	}

	// 5. 执行知识库删除（级联删除）
	err = kb.Drop()
	if err != nil {
		t.Fatalf("Failed to drop knowledge base: %v", err)
	}

	t.Logf("Knowledge base %s dropped successfully", kbName)

	// 6. 验证知识库信息已删除
	_, err = knowledgebase.LoadKnowledgeBase(db, kbName)
	if err == nil {
		t.Error("Knowledge base should have been deleted")
	}

	// 7. 验证所有知识条目已删除
	var entryCount int64
	db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbInfo.ID).Count(&entryCount)
	if entryCount != 0 {
		t.Errorf("Expected 0 knowledge entries after cascade delete, got %d", entryCount)
	} else {
		t.Log("All knowledge entries successfully deleted")
	}

	// 8. 验证向量集合已删除
	exists = vectorstore.HasCollection(db, kbName)
	if exists {
		t.Error("Vector collection should have been deleted")
	} else {
		t.Log("Vector collection successfully deleted")
	}

	// 9. 验证所有向量文档已删除
	var docCount int64
	db.Model(&schema.VectorStoreDocument{}).
		Joins("JOIN vector_store_collections ON vector_store_documents.collection_id = vector_store_collections.id").
		Where("vector_store_collections.name = ?", kbName).
		Count(&docCount)

	if docCount != 0 {
		t.Errorf("Expected 0 vector documents after cascade delete, got %d", docCount)
	} else {
		t.Log("All vector documents successfully deleted")
	}

	t.Log("=== Cascade delete test completed successfully ===")
}
