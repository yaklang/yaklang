package knowledgegraph

import (
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestEntityCreation(t *testing.T) {
	// 测试实体创建
	entity := NewEntity("test_001", "测试实体", EntityTypeTechnology, "这是一个测试实体")

	if entity.ID != "test_001" {
		t.Errorf("Expected ID 'test_001', got '%s'", entity.ID)
	}

	if entity.Name != "测试实体" {
		t.Errorf("Expected name '测试实体', got '%s'", entity.Name)
	}

	if entity.Type != EntityTypeTechnology {
		t.Errorf("Expected type '%s', got '%s'", EntityTypeTechnology, entity.Type)
	}

	// 测试添加别名和标签
	entity.AddAlias("Test Entity")
	entity.AddTag("测试")
	entity.SetProperty("version", "1.0")

	if len(entity.Aliases) != 1 || entity.Aliases[0] != "Test Entity" {
		t.Errorf("Expected alias 'Test Entity', got %v", entity.Aliases)
	}

	if len(entity.Tags) != 1 || entity.Tags[0] != "测试" {
		t.Errorf("Expected tag '测试', got %v", entity.Tags)
	}

	if version, ok := entity.Properties["version"]; !ok || version != "1.0" {
		t.Errorf("Expected property version '1.0', got %v", version)
	}
}

func TestEntityToRAGDocument(t *testing.T) {
	entity := NewEntity("doc_test", "文档测试实体", EntityTypeConcept, "用于测试文档转换的实体")
	entity.AddAlias("Doc Test")
	entity.AddTag("文档")
	entity.SetProperty("test_prop", "test_value")

	doc := entity.ToRAGDocument()

	if doc.ID != entity.ID {
		t.Errorf("Expected document ID '%s', got '%s'", entity.ID, doc.ID)
	}

	// 验证内容包含关键信息
	content := doc.Content
	if !containsAll(content, []string{"实体名称: 文档测试实体", "实体类型: concept", "描述: 用于测试文档转换的实体"}) {
		t.Errorf("Document content missing expected information: %s", content)
	}

	// 验证元数据
	if entityID, ok := doc.Metadata["entity_id"]; !ok || entityID != entity.ID {
		t.Errorf("Expected metadata entity_id '%s', got %v", entity.ID, entityID)
	}

	if entityType, ok := doc.Metadata["entity_type"]; !ok || entityType != string(entity.Type) {
		t.Errorf("Expected metadata entity_type '%s', got %v", entity.Type, entityType)
	}
}

func TestMockDataCreation(t *testing.T) {
	// 测试mock实体创建
	entities := CreateMockEntities()
	if len(entities) == 0 {
		t.Error("CreateMockEntities returned no entities")
	}

	t.Logf("Created %d mock entities", len(entities))

	// 验证实体类型分布
	typeCount := make(map[EntityType]int)
	for _, entity := range entities {
		typeCount[entity.Type]++

		// 验证基本字段
		if entity.ID == "" || entity.Name == "" || entity.Description == "" {
			t.Errorf("Entity %s has empty required fields", entity.ID)
		}
	}

	t.Logf("Entity type distribution: %v", typeCount)
}

func TestRandomEntityGeneration(t *testing.T) {
	count := 10
	entities := GenerateRandomEntities(count)

	if len(entities) != count {
		t.Errorf("Expected %d random entities, got %d", count, len(entities))
	}

	// 验证随机实体的基本属性
	for i, entity := range entities {
		expectedID := "random_" + utils.InterfaceToString(i)
		if entity.ID != expectedID {
			t.Errorf("Expected entity ID '%s', got '%s'", expectedID, entity.ID)
		}

		if entity.Properties["test_id"] != i {
			t.Errorf("Expected test_id %d, got %v", i, entity.Properties["test_id"])
		}

		if generated, ok := entity.Properties["generated"].(bool); !ok || !generated {
			t.Errorf("Expected generated property to be true for entity %s", entity.ID)
		}
	}
}

func TestEntityCollection(t *testing.T) {
	// 创建测试数据库
	db, _ := utils.CreateTempTestDatabaseInMemory()
	if db == nil {
		t.Skip("database not available")
		return
	}

	collectionName := "test_entity_collection_" + utils.RandStringBytes(8)

	// 创建实体集合
	entityCollection, err := NewEntityCollection(db, collectionName)
	if err != nil {
		t.Logf("Failed to create entity collection (may be expected if embedding service is not available): %v", err)
		t.Skip("skipping test due to entity collection creation failure")
		return
	}

	// 清理测试数据
	defer func() {
		// 这里可以添加清理逻辑
	}()

	// 创建测试实体
	testEntities := []*Entity{
		NewEntity("test_import_1", "Docker容器技术", EntityTypeTechnology, "开源的容器化平台，用于应用打包和部署"),
		NewEntity("test_import_2", "SQL注入漏洞", EntityTypeVulnerability, "Web应用中常见的安全漏洞类型"),
		NewEntity("test_import_3", "机器学习概念", EntityTypeConcept, "人工智能的重要分支领域"),
	}

	// 测试导入实体
	t.Log("Testing entity import...")
	err = entityCollection.AddEntities(testEntities...)
	if err != nil {
		t.Fatalf("Failed to import entities: %v", err)
	}

	// 等待向量索引构建
	time.Sleep(2 * time.Second)

	// 测试搜索实体
	t.Log("Testing entity search...")
	searchResults, err := entityCollection.SearchEntities("容器技术", 5)
	if err != nil {
		t.Errorf("Failed to search entities: %v", err)
		return
	}

	t.Logf("Search results for '容器技术': %d entities found", len(searchResults))
	for i, entity := range searchResults {
		t.Logf("Result %d: ID=%s, Name=%s, Type=%s", i+1, entity.ID, entity.Name, entity.Type)
	}

	// 测试按类型搜索
	t.Log("Testing search by type...")
	techResults, err := entityCollection.SearchEntitiesByType(EntityTypeTechnology, "容器", 5)
	if err != nil {
		t.Errorf("Failed to search entities by type: %v", err)
		return
	}

	t.Logf("Technology entities containing '容器': %d found", len(techResults))

	// 测试获取特定实体
	t.Log("Testing get specific entity...")
	entity, err := entityCollection.GetEntity("test_import_1")
	if err != nil {
		t.Errorf("Failed to get entity: %v", err)
		return
	}

	if entity.Name != "Docker容器技术" {
		t.Errorf("Expected entity name 'Docker容器技术', got '%s'", entity.Name)
	}

	// 测试统计实体数量
	count, err := entityCollection.CountEntities()
	if err != nil {
		t.Errorf("Failed to count entities: %v", err)
		return
	}

	t.Logf("Total entities in collection: %d", count)

	if count < len(testEntities) {
		t.Errorf("Expected at least %d entities, got %d", len(testEntities), count)
	}
}

// 辅助函数：检查字符串是否包含所有指定的子串
func containsAll(text string, substrings []string) bool {
	for _, substr := range substrings {
		if !strings.Contains(text, substr) {
			return false
		}
	}
	return true
}
