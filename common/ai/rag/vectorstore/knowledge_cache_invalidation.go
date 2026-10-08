package vectorstore

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	yakit.RegisterKnowledgeGraphInvalidator(func(db *gorm.DB, collections []*schema.VectorStoreCollection) {
		for _, collection := range collections {
			GraphWrapperManager.RemoveCollectionFromCache(db, collection)
		}
	})
}
