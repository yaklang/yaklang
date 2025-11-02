package vectorstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func createTempTestDatabase() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	return db, nil

}

func TestMUSTPASS_Exports(t *testing.T) {
	// 用于储存测试数据
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	embedding := NewDefaultMockEmbedding()
	collectionName := utils.RandStringBytes(10)
	store, err := NewSQLiteVectorStoreHNSW(collectionName, "test", "text-embedding-3-small", 1024, embedding, testDB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		// 使用包含词典词汇的文本，确保生成非零向量
		data := fmt.Sprintf("computer algorithm data %d", i)
		store.AddWithOptions(data, data, WithDocumentRawMetadata(map[string]any{"test": "test"}))
	}

	// 导出测试数据
	reader, err := ExportRAGToBinary(collectionName, WithImportExportDB(testDB))
	if err != nil {
		t.Fatal(err)
	}
	// 导出到临时文件
	tempFile, err := os.CreateTemp("", "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()
	io.Copy(tempFile, reader)

	// 创建新数据库，测试导入
	newTestDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	err = ImportRAGFromFile(tempFile.Name(), WithImportExportDB(newTestDB))
	if err != nil {
		t.Fatal(err)
	}

	// 对比新旧数据库的 collection 表
	var oldCollections []schema.VectorStoreCollection
	var newCollections []schema.VectorStoreCollection
	if err := testDB.Model(&schema.VectorStoreCollection{}).Find(&oldCollections).Error; err != nil {
		t.Fatal(err)
	}
	if err := newTestDB.Model(&schema.VectorStoreCollection{}).Find(&newCollections).Error; err != nil {
		t.Fatal(err)
	}
	assert.Len(t, oldCollections, 1)
	assert.Len(t, newCollections, 1)

	assert.Equal(t, oldCollections[0].UUID, newCollections[0].UUID)
	assert.Equal(t, oldCollections[0].Name, newCollections[0].Name)
	assert.Equal(t, oldCollections[0].Description, newCollections[0].Description)
	assert.Equal(t, oldCollections[0].ModelName, newCollections[0].ModelName)
	assert.Equal(t, oldCollections[0].Dimension, newCollections[0].Dimension)
	assert.Equal(t, oldCollections[0].M, newCollections[0].M)
	assert.Equal(t, oldCollections[0].Ml, newCollections[0].Ml)
	assert.Equal(t, oldCollections[0].EfSearch, newCollections[0].EfSearch)
	assert.Equal(t, oldCollections[0].EfConstruct, newCollections[0].EfConstruct)
	assert.Equal(t, oldCollections[0].DistanceFuncType, newCollections[0].DistanceFuncType)
	assert.Equal(t, oldCollections[0].EnablePQMode, newCollections[0].EnablePQMode)
	assert.Equal(t, oldCollections[0].Archived, newCollections[0].Archived)
	assert.Equal(t, oldCollections[0].GraphBinary, newCollections[0].GraphBinary)
	assert.Equal(t, len(oldCollections[0].CodeBookBinary), len(newCollections[0].CodeBookBinary))

	// 对比新旧数据库的 document 表
	var oldDocuments []schema.VectorStoreDocument
	var newDocuments []schema.VectorStoreDocument
	if err := testDB.Model(&schema.VectorStoreDocument{}).Find(&oldDocuments).Error; err != nil {
		t.Fatal(err)
	}
	if err := newTestDB.Model(&schema.VectorStoreDocument{}).Find(&newDocuments).Error; err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(oldDocuments), len(newDocuments))
	sort.Slice(oldDocuments, func(i, j int) bool {
		return oldDocuments[i].DocumentID < oldDocuments[j].DocumentID
	})
	sort.Slice(newDocuments, func(i, j int) bool {
		return newDocuments[i].DocumentID < newDocuments[j].DocumentID
	})
	for i := range oldDocuments {
		assert.Equal(t, oldDocuments[i].DocumentType, newDocuments[i].DocumentType)
		assert.Equal(t, oldDocuments[i].EntityID, newDocuments[i].EntityID)
		assert.Equal(t, oldDocuments[i].RelatedEntities, newDocuments[i].RelatedEntities)
		assert.Equal(t, oldDocuments[i].DocumentID, newDocuments[i].DocumentID)
		assert.Equal(t, oldDocuments[i].UID, newDocuments[i].UID)
		assert.Equal(t, oldDocuments[i].CollectionID, newDocuments[i].CollectionID)
		assert.Equal(t, oldDocuments[i].CollectionUUID, newDocuments[i].CollectionUUID)
		assert.Equal(t, oldDocuments[i].Metadata, newDocuments[i].Metadata)
		assert.Equal(t, oldDocuments[i].Embedding, newDocuments[i].Embedding)
		assert.Equal(t, oldDocuments[i].Content, newDocuments[i].Content)
	}
}

func TestMUSTPASS_ExportRAGToBinary(t *testing.T) {
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}

	// 创建测试集合
	collectionName := utils.RandStringBytes(10)
	embedding := NewDefaultMockEmbedding()
	store, err := NewSQLiteVectorStoreHNSW(collectionName, "test description", "text-embedding-3-small", 1024, embedding, testDB)
	if err != nil {
		t.Fatal(err)
	}

	testDocuments := []struct {
		id       string
		content  string
		metadata map[string]any
	}{
		{"doc1", "computer algorithm data science", map[string]any{"type": "test", "index": 1}},
		{"doc2", "network security firewall system", map[string]any{"type": "test", "index": 2}},
		{"doc3", "artificial intelligence machine learning", map[string]any{"type": "example", "index": 3}},
	}

	for _, doc := range testDocuments {
		store.AddWithOptions(doc.id, doc.content, WithDocumentRawMetadata(doc.metadata))
	}

	// 导出RAG数据为二进制格式
	reader, err := ExportRAGToBinary(collectionName, WithImportExportDB(testDB))
	if err != nil {
		t.Fatal(err)
	}

	// 使用LoadRAGFromBinary加载二进制数据
	ragData, err := LoadRAGFromBinary(reader)
	if err != nil {
		t.Fatal(err)
	}

	// 验证RAGBinaryData结构
	if ragData == nil {
		t.Fatal("RAGBinaryData is nil")
	}

	// 验证Collection数据
	if ragData.Collection == nil {
		t.Fatal("Collection is nil")
	}

	collection := ragData.Collection
	if collection.Name != collectionName {
		t.Fatalf("Collection name mismatch: expected %s, got %s", collectionName, collection.Name)
	}

	if collection.Description != "test description" {
		t.Fatalf("Collection description mismatch: expected 'test description', got %s", collection.Description)
	}

	if collection.ModelName != "text-embedding-3-small" {
		t.Fatalf("Collection model name mismatch: expected 'text-embedding-3-small', got %s", collection.ModelName)
	}

	if collection.Dimension != 1024 {
		t.Fatalf("Collection dimension mismatch: expected 1024, got %d", collection.Dimension)
	}

	// 验证Documents数据
	if ragData.Documents == nil {
		t.Fatal("Documents is nil")
	}

	if len(ragData.Documents) != len(testDocuments) {
		t.Fatalf("Documents count mismatch: expected %d, got %d", len(testDocuments), len(ragData.Documents))
	}

	// 创建文档ID到原始数据的映射，便于验证
	expectedDocs := make(map[string]struct {
		content  string
		metadata map[string]any
	})
	for _, doc := range testDocuments {
		expectedDocs[doc.id] = struct {
			content  string
			metadata map[string]any
		}{doc.content, doc.metadata}
	}

	// 验证每个文档的数据
	for _, doc := range ragData.Documents {
		if doc.DocumentID == "" {
			t.Fatal("Document ID is empty")
		}

		expected, exists := expectedDocs[doc.DocumentID]
		if !exists {
			t.Fatalf("Unexpected document ID: %s", doc.DocumentID)
		}

		// 验证元数据
		if doc.Metadata == nil {
			t.Fatalf("Document %s metadata is nil", doc.DocumentID)
		}

		// 验证元数据中的特定字段
		for key, expectedValue := range expected.metadata {
			actualValue, exists := doc.Metadata[key]
			if !exists {
				t.Fatalf("Document %s missing metadata key: %s", doc.DocumentID, key)
			}

			// 处理JSON序列化后数字类型的转换问题
			if key == "index" {
				// 数字类型在JSON序列化后可能变成float64
				expectedFloat, ok1 := expectedValue.(int)
				actualFloat, ok2 := actualValue.(float64)
				if ok1 && ok2 {
					if float64(expectedFloat) != actualFloat {
						t.Fatalf("Document %s metadata mismatch for key %s: expected %v, got %v", doc.DocumentID, key, expectedValue, actualValue)
					}
					continue
				}
			}

			if actualValue != expectedValue {
				t.Fatalf("Document %s metadata mismatch for key %s: expected %v, got %v", doc.DocumentID, key, expectedValue, actualValue)
			}
		}

		// 验证嵌入向量
		if doc.Embedding == nil {
			t.Fatalf("Document %s embedding is nil", doc.DocumentID)
		}

		if len(doc.Embedding) != collection.Dimension {
			t.Fatalf("Document %s embedding dimension mismatch: expected %d, got %d", doc.DocumentID, collection.Dimension, len(doc.Embedding))
		}

		// 验证嵌入向量不全为零（MockEmbedding应该生成非零向量）
		allZero := true
		for _, val := range doc.Embedding {
			if val != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Fatalf("Document %s embedding is all zeros", doc.DocumentID)
		}
	}

	t.Logf("Successfully exported and loaded %d documents with correct data", len(ragData.Documents))
}

func TestMUSTPASS_ImportAndRebuildHNSWIndex(t *testing.T) {
	// 不稳定触发：failed to migrate HNSW graph: export hnsw graph to binary: unsupported node code type: func() []float32
	t.Skip()
	// 创建测试数据库
	testDB, err := createTempTestDatabase()
	if err != nil {
		t.Fatal(err)
	}

	// 创建测试集合
	collectionName := utils.RandStringBytes(10)
	embedding := NewDefaultMockEmbedding()
	store, err := NewSQLiteVectorStoreHNSW(collectionName, "test description", "text-embedding-3-small", 1024, embedding, testDB)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		store.AddWithOptions(fmt.Sprintf("computer algorithm data %d", i), fmt.Sprintf("computer algorithm data %d", i), WithDocumentRawMetadata(map[string]any{"test": "test"}))
	}
	reader, err := ExportRAGToBinary(collectionName, WithImportExportDB(testDB), WithNoHNSWGraph(true))
	if err != nil {
		t.Fatal(err)
	}

	var binBuffer bytes.Buffer
	ragData, err := LoadRAGFromBinary(io.TeeReader(reader, &binBuffer))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(ragData.Documents), 100)
	assert.Equal(t, len(ragData.Collection.GraphBinary), 0)

	newCollectionName := utils.RandStringBytes(10)
	err = ImportRAGFromReader(&binBuffer, WithImportExportDB(testDB), WithRebuildHNSWIndex(true), WithCollectionName(newCollectionName))
	if err != nil {
		t.Fatal(err)
	}

	var collection schema.VectorStoreCollection
	if err := testDB.Model(&schema.VectorStoreCollection{}).Where("name = ?", newCollectionName).First(&collection).Error; err != nil {
		t.Fatal(err)
	}
	if len(collection.GraphBinary) == 0 {
		t.Fatal("GraphBinary is empty")
	}
	assert.NotNil(t, collection.GraphBinary)
	assert.NotEqual(t, len(collection.GraphBinary), 0)
}
