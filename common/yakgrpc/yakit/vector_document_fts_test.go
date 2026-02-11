package yakit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSearchVectorStoreDocumentBM25_SQLiteFTS5(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)

	if err := EnsureVectorStoreDocumentFTS5(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}
	if !db.HasTable(VectorDocumentVTableName()) {
		t.Skip("fts5 table not available")
	}

	collection := &schema.VectorStoreCollection{
		Name:      "test_collection",
		ModelName: "test",
		Dimension: 1,
	}
	require.NoError(t, db.Create(collection).Error)

	d1 := &schema.VectorStoreDocument{
		DocumentType:    schema.RAGDocumentType_Knowledge,
		DocumentID:      "doc1",
		CollectionID:    collection.ID,
		CollectionUUID:  collection.UUID,
		Metadata:        schema.MetadataMap{"tag": "tcp", "source": "alpha"},
		Content:         "yaklang fts5 is working",
		RuntimeID:       "rt1",
		RelatedEntities: "",
	}
	d2 := &schema.VectorStoreDocument{
		DocumentType:    schema.RAGDocumentType_Knowledge,
		DocumentID:      "doc2",
		CollectionID:    collection.ID,
		CollectionUUID:  collection.UUID,
		Metadata:        schema.MetadataMap{"tag": "http", "source": "beta"},
		Content:         "nothing special here",
		RuntimeID:       "rt2",
		RelatedEntities: "",
	}
	require.NoError(t, db.Create(d1).Error)
	require.NoError(t, db.Create(d2).Error)

	got, err := SearchVectorStoreDocumentBM25(db, &VectorDocumentFilter{Keywords: []string{"tcp"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "doc1", got[0].DocumentID)

	got, err = SearchVectorStoreDocumentBM25(db, &VectorDocumentFilter{Keywords: []string{"yaklang"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "doc1", got[0].DocumentID)
}
