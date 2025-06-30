package plugins_rag

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/utils"
)

// NewSQLitePluginsRagManager 创建一个基于 SQLite 向量存储的插件 RAG 管理器
func NewSQLitePluginsRagManager(db *gorm.DB, collectionName string, modelName string, dimension int, opts ...aispec.AIConfigOption) (*PluginsRagManager, error) {
	if collectionName == "" {
		collectionName = PLUGIN_RAG_COLLECTION_NAME
	}

	// 创建基于 SQLite 的 RAG 系统
	ragSystem, err := rag.NewDefaultSQLiteRAGSystem(db, collectionName, modelName, dimension, opts...)
	if err != nil {
		return nil, utils.Errorf("创建基于 SQLite 的 RAG 系统失败: %v", err)
	}

	// 创建插件 RAG 管理器
	return NewPluginsRagManager(db, ragSystem, collectionName), nil
}
