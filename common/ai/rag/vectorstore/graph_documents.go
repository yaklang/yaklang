package vectorstore

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/schema"
)

type graphDocumentRef struct {
	id         uint
	documentID string
}

type graphDocumentUID string

func graphDocumentLookupKey(id hnswspec.LazyNodeID) any {
	switch value := id.(type) {
	case []byte:
		return graphDocumentUID(value)
	case string:
		return value
	case int:
		return uint64(value)
	case int32:
		return uint64(value)
	case int64:
		return uint64(value)
	case uint32:
		return uint64(value)
	case uint64:
		return value
	default:
		return nil
	}
}

// Load only identifiers, once per restore, rather than issuing a SQL query
// for every node on every HNSW layer. Vectors and PQ codes remain lazy.
func loadGraphDocumentRefs(db *gorm.DB, collectionID uint) (map[any]graphDocumentRef, error) {
	rows, err := db.Model(&schema.VectorStoreDocument{}).
		Where("collection_id = ?", collectionID).Select("id, document_id, uid").Order("id ASC").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := make(map[any]graphDocumentRef)
	for rows.Next() {
		var ref graphDocumentRef
		var uid []byte
		if err := rows.Scan(&ref.id, &ref.documentID, &uid); err != nil {
			return nil, err
		}
		refs[uint64(ref.id)] = ref
		// Preserve First's lowest-ID behavior for legacy duplicate identifiers.
		if _, exists := refs[ref.documentID]; !exists {
			refs[ref.documentID] = ref
		}
		if uid != nil {
			key := graphDocumentUID(uid)
			if _, exists := refs[key]; !exists {
				refs[key] = ref
			}
		}
	}
	return refs, rows.Err()
}
