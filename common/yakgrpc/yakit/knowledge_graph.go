package yakit

import (
	"sync/atomic"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

type knowledgeGraphInvalidator func(*gorm.DB, []*schema.VectorStoreCollection)

var knowledgeGraphInvalidation atomic.Value

// RegisterKnowledgeGraphInvalidator connects the optional vector engine without
// making database/library code import that engine (which would create a cycle).
func RegisterKnowledgeGraphInvalidator(callback func(*gorm.DB, []*schema.VectorStoreCollection)) {
	knowledgeGraphInvalidation.Store(knowledgeGraphInvalidator(callback))
}

func InvalidateKnowledgeGraphs(db *gorm.DB, collections []*schema.VectorStoreCollection) {
	if callback, ok := knowledgeGraphInvalidation.Load().(knowledgeGraphInvalidator); ok && callback != nil {
		callback(db, collections)
	}
}
