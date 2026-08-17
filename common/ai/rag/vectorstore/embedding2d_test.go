package vectorstore

import (
	"fmt"
	"hash/fnv"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// newDistinctMockEmbedder 返回对每个文本生成确定性且互不相同向量的 mock embedder，
// 避免依赖词典式 mock 的具体词表内容。
func newDistinctMockEmbedder() EmbeddingClient {
	return NewMockEmbedder(func(text string) ([]float32, error) {
		vec := make([]float32, 1024)
		h := fnv.New64a()
		_, _ = h.Write([]byte(text))
		seed := h.Sum64()
		for i := range vec {
			// 简单确定性伪随机，保证不同文本产生不同向量
			seed = seed*6364136223846793005 + 1442695040888963407
			vec[i] = float32(int64(seed>>33)%1000) / 1000.0
		}
		return vec, nil
	})
}

func TestMUSTPASS_Collection2DPoints(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	collectionName := utils.RandStringBytes(10)
	store, err := GetCollection(testDB, collectionName, WithEmbeddingClient(newDistinctMockEmbedder()))
	require.NoError(t, err)

	documentIDs := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("document-%d", i)
		content := fmt.Sprintf("unique document content %d about yak security topic %d", i, i)
		require.NoError(t, store.AddWithOptions(id, content))
		documentIDs[id] = struct{}{}
	}

	points, err := Collection2DPoints(testDB, collectionName)
	require.NoError(t, err)
	require.Len(t, points, len(documentIDs), "every document should have one 2d point")

	seen := make(map[string]struct{})
	distinctCoords := map[[2]float32]struct{}{}
	for _, p := range points {
		_, ok := documentIDs[p.DocumentID]
		assert.True(t, ok, "point should reference an added document, got %s", p.DocumentID)
		seen[p.DocumentID] = struct{}{}
		assert.NotEqual(t, string(DocumentTypeCollectionInfo), p.DocumentID, "collection info document must be excluded")
		assert.False(t, math.IsNaN(float64(p.X)) || math.IsInf(float64(p.X), 0), "x should be finite")
		assert.False(t, math.IsNaN(float64(p.Y)) || math.IsInf(float64(p.Y), 0), "y should be finite")
		assert.NotContains(t, p.ContentPreview, "\n", "content preview should collapse newlines")
		assert.LessOrEqual(t, len([]rune(p.ContentPreview)), embedding2DContentPreviewLen+3, "content preview should be truncated")
		distinctCoords[[2]float32{p.X, p.Y}] = struct{}{}
	}
	assert.Len(t, seen, len(documentIDs), "all document ids should be covered")
	assert.Greater(t, len(distinctCoords), 1, "distinct documents should not collapse to a single point")

	// PCA 使用固定随机种子，同一集合重复计算结果应一致
	again, err := Collection2DPoints(testDB, collectionName)
	require.NoError(t, err)
	require.Len(t, again, len(points))
	for i := range points {
		assert.Equal(t, points[i].DocumentID, again[i].DocumentID)
		assert.Equal(t, points[i].X, again[i].X, "x should be deterministic")
		assert.Equal(t, points[i].Y, again[i].Y, "y should be deterministic")
	}
}

func TestMUSTPASS_Collection2DPointsSkipsDocumentsWithoutEmbedding(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	collectionName := utils.RandStringBytes(10)
	store, err := GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	require.NoError(t, err)
	require.NoError(t, store.AddWithOptions("with-embedding", "document with embedding"))

	// 直接插入一条没有向量的文档行，模拟历史脏数据
	require.NoError(t, testDB.Create(&schema.VectorStoreDocument{
		DocumentID:     "without-embedding",
		CollectionID:   store.collection.ID,
		CollectionUUID: store.collection.UUID,
		Content:        "legacy document without embedding",
	}).Error)

	points, err := Collection2DPoints(testDB, collectionName)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, "with-embedding", points[0].DocumentID)
	assert.NotEmpty(t, points[0].ContentPreview)
}

func TestMUSTPASS_Collection2DPointsEmptyCollection(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	collectionName := utils.RandStringBytes(10)
	_, err = GetCollection(testDB, collectionName, WithEmbeddingClient(NewDefaultMockEmbedding()))
	require.NoError(t, err)

	// 新建集合只包含 collection info 文档（会被排除），应返回空切片而不是报错
	points, err := Collection2DPoints(testDB, collectionName)
	require.NoError(t, err)
	assert.Empty(t, points)
}

func TestMUSTPASS_Collection2DPointsCollectionNotFound(t *testing.T) {
	testDB, err := createTempTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	_, err = Collection2DPoints(testDB, "no-such-collection")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-collection")
}
