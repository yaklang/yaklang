package plugins_rag

import (
	"math"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var PLUGIN_RAG_COLLECTION_NAME = "yaklang_plugins_default"

func CreateDefaultSQLiteManager(db *gorm.DB, collectionName string, opts ...aispec.AIConfigOption) (*PluginsRagManager, error) {
	cfg, err := LoadEmbeddingEndpointConfig()
	if err != nil {
		return nil, err
	}
	opts = append(opts, aispec.WithBaseURL(cfg.BaseURL))
	return NewSQLitePluginsRagManager(db, collectionName, cfg.Model, cfg.Dimension, "", opts...)
}

// IndexAllPlugins 索引所有未被忽略的插件
func IndexAllPlugins() error {
	db := consts.GetGormProfileDatabase()
	manager, err := CreateDefaultSQLiteManager(db, PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.IndexAllPlugins()
}

// IndexPlugin 索引单个插件
func IndexPlugin(scriptName string) error {
	db := consts.GetGormProfileDatabase()
	manager, err := CreateDefaultSQLiteManager(db, PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.IndexPlugin(scriptName)
}

// SearchPlugins 使用自然语言搜索插件
func SearchPlugins(query string, limit int) ([]*PluginSearchResult, error) {
	db := consts.GetGormProfileDatabase()
	manager, err := CreateDefaultSQLiteManager(db, PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return nil, err
	}
	return manager.SearchPlugins(query, limit)
}

func IsReady() bool {
	db := consts.GetGormProfileDatabase()
	return rag.CollectionIsExists(db, PLUGIN_RAG_COLLECTION_NAME)
}

// 导出函数列表
var Exports = map[string]interface{}{
	// "CreateManager":   CreateDefaultSQLiteManager,
	"IndexAllPlugins": IndexAllPlugins,
	"IndexPlugin":     IndexPlugin,
	"SearchPlugins":   SearchPlugins,
	"IsReady":         IsReady,
}

// SearchPluginIds 使用自然语言搜索插件ID
func SearchPluginIds(db *gorm.DB, pagination *ypb.Paging, key string) (*bizhelper.Paginator, []string, error) {
	manager, err := CreateDefaultSQLiteManager(db, PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return nil, nil, err
	}
	total, ids, err := manager.SearchPluginsIds(key, int(pagination.GetPage()), int(pagination.GetLimit()))
	if err != nil {
		return nil, nil, err
	}
	return &bizhelper.Paginator{
		TotalRecord: total,
		TotalPage:   int(math.Ceil(float64(total) / float64(pagination.GetLimit()))),
		Records:     ids,
		Offset:      (int(pagination.GetPage()) - 1) * int(pagination.GetLimit()),
		Limit:       int(pagination.GetLimit()),
		Page:        int(pagination.GetPage()),
		PrevPage:    int(pagination.GetPage()) - 1,
		NextPage:    int(pagination.GetPage()) + 1,
	}, ids, nil
}

func init() {
	yakit.RegisterRAGSearchPluginIdsCallback(SearchPluginIds)
}
