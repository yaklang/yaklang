package entityrepos

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEntityRepository_Basic(t *testing.T) {
	db := setupTestDB(t)

	repoName := "test_repo"
	repoDesc := "desc"
	mockEmbedding := rag.NewDefaultMockEmbedding()

	repo, err := GetOrCreateEntityRepository(db, repoName, repoDesc, WithDisableBulkProcess(), rag.WithEmbeddingClient(mockEmbedding))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	entityName := uuid.New().String()
	entityType := uuid.New().String()

	// 创建实体
	entity := &schema.ERModelEntity{
		EntityName: entityName,
		EntityType: entityType,
	}
	err = repo.CreateEntity(entity)
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// 查询实体
	entities, err := repo.queryEntities(&ypb.EntityFilter{
		Names: []string{entityName},
		Types: []string{entityType},
	})
	if err != nil {
		t.Fatalf("failed to query entities: %v", err)
	}
	if len(entities) == 0 {
		t.Fatalf("entity not found")
	}

	// 测试向量索引和查询
	content := mockEmbedding.GenerateRandomText(3)
	err = repo.AddVectorIndex(uuid.NewString(), content)
	if err != nil {
		t.Fatalf("failed to add vector index: %v", err)
	}

	results, err := repo.QueryVector(content, 1)
	if err != nil {
		t.Fatalf("failed to query vector: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("vector query returned no results")
	}
}

func TestEntityRepository_VectorSearchEntity(t *testing.T) {
	db := setupTestDB(t)

	mockEmbedding := rag.NewDefaultMockEmbedding()

	repo, err := GetOrCreateEntityRepository(db, "vector_repo", "desc", WithDisableBulkProcess(), rag.WithEmbeddingClient(mockEmbedding))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	words := mockEmbedding.GenerateRandomWord(2)
	require.Len(t, words, 2)

	entity := &schema.ERModelEntity{
		EntityName: words[0],
		EntityType: words[1],
	}
	err = repo.CreateEntity(entity)
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// 等待异步向量索引
	time.Sleep(200 * time.Millisecond)

	// 测试向量搜索
	found, err := repo.VectorSearchEntity(entity)
	if err != nil {
		t.Fatalf("vector search entity error: %v", err)
	}
	// mock embedding 一般会返回结果
	if len(found) == 0 {
		t.Fatalf("vector search entity not found")
	}
}

func TestEntityRepository_MergeAndSaveEntity(t *testing.T) {
	db := setupTestDB(t)

	mockEmbedding := rag.NewDefaultMockEmbedding()
	repo, err := GetOrCreateEntityRepository(db, "merge_repo", "desc", rag.WithEmbeddingClient(mockEmbedding), WithDisableBulkProcess(), WithSimilarityThreshold(0.6))
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	words := mockEmbedding.GenerateRandomWord(5)
	require.Len(t, words, 5)

	entityName := words[0]
	entityType := words[1]
	entityAttrA := words[2]
	entityAttrB := words[3]
	entityAttrC := words[4]

	entity := &schema.ERModelEntity{
		EntityName: entityName,
		EntityType: entityType,
		Attributes: map[string]any{"a": entityAttrA},
	}
	_, err = repo.MergeAndSaveEntity(entity)
	if err != nil {
		t.Fatalf("merge and save entity failed: %v", err)
	}

	// 通过name精确merge
	entity2 := &schema.ERModelEntity{
		EntityName: words[0],
		EntityType: words[1],
		Attributes: map[string]any{"b": entityAttrB},
	}
	merged, err := repo.MergeAndSaveEntity(entity2)
	if err != nil {
		t.Fatalf("merge and save entity failed: %v", err)
	}

	require.Equal(t, merged.Attributes["a"], entityAttrA, "name merge attribute a should be preserved")
	require.Equal(t, merged.Attributes["b"], entityAttrB, "name merge attribute b should be added")

	// 通过embedding相似度merge
	text, err := mockEmbedding.GenerateSimilarText(merged.String(), 0.8) // 提高相似度限制，避免出现临界情况
	require.NoError(t, err)

	entity3 := &schema.ERModelEntity{
		EntityName: text,
		EntityType: entityType,
		Attributes: map[string]any{"c": entityAttrC},
	}
	merged2, err := repo.MergeAndSaveEntity(entity3)
	if err != nil {
		t.Fatalf("merge and save entity failed: %v", err)
	}

	require.Equal(t, merged2.Attributes["a"], entityAttrA, "embedding merge attribute a should be preserved")
	require.Equal(t, merged2.Attributes["b"], entityAttrB, "embedding merge attribute b should be preserved")
	require.Equal(t, merged2.Attributes["c"], entityAttrC, "embedding merge attribute c should be added")
}

func TestSaveEndpoint_Basic(t *testing.T) {
	db := setupTestDB(t)

	mockEmbedding := rag.NewDefaultMockEmbedding()
	repo, err := GetOrCreateEntityRepository(db, "saveendpoint_repo", "desc", WithDisableBulkProcess(), rag.WithEmbeddingClient(mockEmbedding))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	endpoint := repo.NewSaveEndpoint(ctx)

	// 保存实体A
	entityA := &schema.ERModelEntity{
		EntityName: "entityA",
		EntityType: "typeA",
	}
	err = endpoint.SaveEntity(entityA)
	require.NoError(t, err)

	// 保存实体B
	entityB := &schema.ERModelEntity{
		EntityName: "entityB",
		EntityType: "typeB",
	}
	err = endpoint.SaveEntity(entityB)
	require.NoError(t, err)

	// WaitIndex 能获取到uuid
	uuidA, err := endpoint.WaitIndex("entityA")
	require.NoError(t, err)
	require.NotEmpty(t, uuidA)
	uuidB, err := endpoint.WaitIndex("entityB")
	require.NoError(t, err)
	require.NotEmpty(t, uuidB)
	require.NotEqual(t, uuidA, uuidB)

	// AddRelationship
	err = endpoint.AddRelationship("entityA", "entityB", "relType", "关系类型", map[string]any{"k": "v"})
	require.NoError(t, err)

	// FinishEntitySave 后 WaitIndex 新实体
	endpoint.FinishEntitySave()
	uuidC, err := endpoint.WaitIndex("entityC")
	require.NoError(t, err)
	require.NotEmpty(t, uuidC)
}

func TestSaveEndpoint_WaitIndex(t *testing.T) {
	db := setupTestDB(t)

	mockEmbedding := rag.NewDefaultMockEmbedding()
	repo, err := GetOrCreateEntityRepository(db, "saveendpoint_repo2", "desc", WithDisableBulkProcess(), rag.WithEmbeddingClient(mockEmbedding))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	endpoint := repo.NewSaveEndpoint(ctx)
	var relationType = uuid.NewString()

	// 并发保存实体和关系，不保证顺序
	done := make(chan struct{})
	go func() {
		// 不先保存entityA，直接AddRelationship
		err := endpoint.AddRelationship("entityA", "entityB", relationType, "关系类型", map[string]any{"k": "v"})
		require.NoError(t, err)
		close(done)
	}()

	// 随机延迟后保存entityA和entityB
	time.Sleep(100 * time.Millisecond)
	go func() {
		err := endpoint.SaveEntity(&schema.ERModelEntity{
			EntityName: "entityA",
			EntityType: "typeA",
		})
		require.NoError(t, err)
	}()
	time.Sleep(50 * time.Millisecond)
	go func() {
		err := endpoint.SaveEntity(&schema.ERModelEntity{
			EntityName: "entityB",
			EntityType: "typeB",
		})
		require.NoError(t, err)
	}()

	// 等待关系保存完成
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("AddRelationship did not complete in time")
	}

	// 检查WaitIndex能获取到uuid
	uuidA, err := endpoint.WaitIndex("entityA")
	require.NoError(t, err)
	require.NotEmpty(t, uuidA)
	uuidB, err := endpoint.WaitIndex("entityB")
	require.NoError(t, err)
	require.NotEmpty(t, uuidB)
	require.NotEqual(t, uuidA, uuidB)

	relationship, err := repo.queryRelationship(&ypb.RelationshipFilter{
		SourceEntityIndex: []string{uuidA},
		TargetEntityIndex: []string{uuidB},
	})
	require.NoError(t, err)
	require.Len(t, relationship, 1)
	require.Equal(t, relationship[0].RelationshipType, relationType)

	// FinishEntitySave 后再WaitIndex新实体，自动补充
	endpoint.FinishEntitySave()
	uuidC, err := endpoint.WaitIndex("entityC")
	require.NoError(t, err)
	require.NotEmpty(t, uuidC)
}
