package vectorstore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func documentLookupPlan(t *testing.T, db *gorm.DB, predicate string, args ...any) string {
	t.Helper()
	rows, err := db.Raw("EXPLAIN QUERY PLAN SELECT embedding FROM rag_vector_document_v1 WHERE "+predicate+" AND deleted_at IS NULL ORDER BY id LIMIT 1", args...).Rows()
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(plan, "\n")
}

func TestMUSTPASS_VectorDocumentLookupIndexes(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreDocument{}).Error)
	require.Contains(t, documentLookupPlan(t, db, "document_id = ? AND collection_id = ?", "same-id", 1), "idx_rag_vector_document_collection_document",
		"lazy graph loading and document upsert must not scan every row in a collection")
	require.Contains(t, documentLookupPlan(t, db, "uid = ?", []byte("legacy-uid")), "idx_rag_vector_document_uid",
		"legacy binary graphs use UID lookups")

	// An existing dirty database may contain duplicate keys. Index migration
	// must remain non-unique and preserve every row.
	for _, index := range []string{"idx_rag_vector_document_collection_document", "idx_rag_vector_document_uid"} {
		require.NoError(t, db.Model(&schema.VectorStoreDocument{}).RemoveIndex(index).Error)
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: "same-id", CollectionID: 1,
			CollectionUUID: "same-collection", UID: []byte("legacy-uid"), Content: fmt.Sprint(i)}).Error)
	}
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreDocument{}).Error)
	var count int
	require.NoError(t, db.Model(&schema.VectorStoreDocument{}).Count(&count).Error)
	require.Equal(t, 3, count)
	require.Contains(t, documentLookupPlan(t, db, "document_id = ? AND collection_id = ?", "same-id", 1), "idx_rag_vector_document_collection_document")
	require.Contains(t, documentLookupPlan(t, db, "uid = ?", []byte("legacy-uid")), "idx_rag_vector_document_uid")
}
