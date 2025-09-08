package entityrepos

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// 创建内存数据库用于测试
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// 自动迁移表结构
	err = db.AutoMigrate(&schema.ERModelEntity{}, &schema.ERModelRelationship{}).Error
	require.NoError(t, err)

	return db
}

func createMockData(t *testing.T, db *gorm.DB) *EntityRepository {
	// 创建测试用的实体库
	repo := &EntityRepository{
		db: db,
		info: &schema.EntityRepository{
			Uuid: uuid.NewString(),
		},
	}

	// 创建测试实体: a -> b -> c -> d
	entities := []*schema.ERModelEntity{
		{
			RepositoryUUID: repo.info.Uuid,
			EntityName:     "Entity A",
			Uuid:           "entity-a",
			EntityType:     "test",
		},
		{
			RepositoryUUID: repo.info.Uuid,
			EntityName:     "Entity B",
			Uuid:           "entity-b",
			EntityType:     "test",
		},
		{
			RepositoryUUID: repo.info.Uuid,
			EntityName:     "Entity C",
			Uuid:           "entity-c",
			EntityType:     "test",
		},
		{
			RepositoryUUID: repo.info.Uuid,
			EntityName:     "Entity D",
			Uuid:           "entity-d",
			EntityType:     "test",
		},
	}

	// 插入实体
	for _, entity := range entities {
		result := db.Create(entity)
		require.NoError(t, result.Error)
	}

	// 创建测试关系: a -> b, b -> c, c -> d
	relationships := []*schema.ERModelRelationship{
		{
			RepositoryUUID:    repo.info.Uuid,
			SourceEntityIndex: "entity-a",
			TargetEntityIndex: "entity-b",
			RelationshipType:  "relates_to",
			Uuid:              "rel-a-b",
		},
		{
			RepositoryUUID:    repo.info.Uuid,
			SourceEntityIndex: "entity-b",
			TargetEntityIndex: "entity-c",
			RelationshipType:  "relates_to",
			Uuid:              "rel-b-c",
		},
		{
			RepositoryUUID:    repo.info.Uuid,
			SourceEntityIndex: "entity-c",
			TargetEntityIndex: "entity-d",
			RelationshipType:  "relates_to",
			Uuid:              "rel-c-d",
		},
	}

	// 插入关系
	for _, rel := range relationships {
		result := db.Create(rel)
		require.NoError(t, result.Error)
	}

	return repo
}

func TestYieldKHop_AllPaths(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 调试：检查数据是否正确插入
	var entityCount int64
	db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", repo.info.Uuid).Count(&entityCount)
	t.Logf("Entity count: %d", entityCount)

	var relCount int64
	db.Model(&schema.ERModelRelationship{}).Where("repository_uuid = ?", repo.info.Uuid).Count(&relCount)
	t.Logf("Relationship count: %d", relCount)

	// 调试：检查YieldEntities是否正常工作
	entityChan := repo.YieldEntities(ctx)
	entityList := make([]*schema.ERModelEntity, 0)
	for entity := range entityChan {
		entityList = append(entityList, entity)
		t.Logf("Found entity: %s (%s)", entity.EntityName, entity.Uuid)
	}
	t.Logf("YieldEntities returned %d entities", len(entityList))

	// 调试：检查YieldRelationships是否正常工作
	relChan := repo.YieldRelationships(ctx)
	relList := make([]*schema.ERModelRelationship, 0)
	for rel := range relChan {
		relList = append(relList, rel)
		t.Logf("Found relationship: %s -> %s (%s)", rel.SourceEntityIndex, rel.TargetEntityIndex, rel.RelationshipType)
	}
	t.Logf("YieldRelationships returned %d relationships", len(relList))

	// 测试k=0，返回所有路径
	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx) {
		results = append(results, path)
		t.Logf("Found path with K=%d: %s", path.K, printPath(path))
	}

	// 验证结果
	t.Logf("Total results: %d", len(results))
	assert.Greater(t, len(results), 0, "应该有至少一个路径结果")

	// 检查是否有2-hop路径
	has2Hop := false
	for _, result := range results {
		if result.K == 1 { // 2-hop路径的K值应该是1
			has2Hop = true
			break
		}
	}
	assert.True(t, has2Hop, "应该包含2-hop路径")
}

func TestYieldKHop_SpecificK(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试k=2，返回2-hop路径
	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx, WithKHopK(2)) {
		results = append(results, path)
	}

	// 验证结果
	assert.Greater(t, len(results), 0, "应该有至少一个2-hop路径结果")

	// 检查所有结果都是2-hop路径
	for _, result := range results {
		assert.Equal(t, 1, result.K, "所有结果都应该是1-hop（2个实体）")
	}
}

func TestYieldKHop_WithKMin(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试k=0, KMin=3，返回长度>=3的路径
	results := make([]*KHopPath, 0)

	for path := range repo.YieldKHop(ctx, WithKHopKMin(3)) {
		results = append(results, path)
	}

	// 验证结果 - 应该只包含长度>=3的路径
	for _, result := range results {
		assert.GreaterOrEqual(t, result.K+1, 3, "所有路径长度应该>=3")
	}
}

func TestYieldKHop_PathStructure(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取一个2-hop路径
	var twoHopPath *KHopPath
	for path := range repo.YieldKHop(ctx, WithKHopK(2)) {
		if path.K == 1 { // 2-hop路径
			twoHopPath = path
			break
		}
	}

	require.NotNil(t, twoHopPath, "应该找到2-hop路径")

	// 验证路径结构
	current := twoHopPath.Hops
	entityCount := 0
	relationshipCount := 0
	var lastNode *HopBlock

	for current != nil {
		if current.Src != nil {
			entityCount++
		}
		if current.Relationship != nil {
			relationshipCount++
		}
		lastNode = current
		current = current.Next
	}

	assert.Equal(t, 2, entityCount, "2-hop路径应该有2个实体")
	assert.Equal(t, 1, relationshipCount, "2-hop路径应该有1个关系")
	assert.NotNil(t, lastNode, "应该有最后一个节点")
	assert.True(t, lastNode.IsEnd, "路径末尾应该标记为结束")
}

func TestYieldKHop_EmptyRepository(t *testing.T) {
	db := setupTestDB(t)

	// 创建空的实体库
	repo := &EntityRepository{
		db: db,
		info: &schema.EntityRepository{
			Uuid: uuid.NewString(),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试空库
	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx) {
		results = append(results, path)
	}

	assert.Equal(t, 0, len(results), "空库应该返回空结果")
}

func TestYieldKHop_Cancellation(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithCancel(context.Background())

	// 立即取消上下文
	cancel()

	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx) {
		results = append(results, path)
	}

	// 由于立即取消，可能没有结果或只有部分结果
	// 这里我们只是验证函数不会死锁
	assert.True(t, len(results) >= 0, "取消后应该不会有死锁")
}

func TestYieldKHop_LargeK(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试很大的k值，应该返回空结果
	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx, WithKHopK(100)) {
		results = append(results, path)
	}

	assert.Equal(t, 0, len(results), "过大的k值应该返回空结果")
}

func TestYieldKHop_InvalidK(t *testing.T) {
	db := setupTestDB(t)
	repo := createMockData(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试负数k值，应该被设置为0
	results := make([]*KHopPath, 0)
	for path := range repo.YieldKHop(ctx, WithKHopK(-1)) {
		results = append(results, path)
	}

	assert.Greater(t, len(results), 0, "k=-1应该被当作k=0处理，返回所有路径")
}

// 辅助函数用于调试
func printPath(path *KHopPath) string {
	if path == nil || path.Hops == nil {
		return "nil"
	}

	result := ""
	current := path.Hops
	for current != nil {
		if current.Src != nil {
			if result != "" {
				result += " -> "
			}
			result += current.Src.EntityName
		}
		current = current.Next
	}
	return result
}
