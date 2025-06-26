package plugins_rag

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
)

var PLUGIN_RAG_COLLECTION_NAME = "yaklang_plugins_default"

func CreateDefaultSQLiteManager(collectionName string, opts ...aispec.AIConfigOption) (*PluginsRagManager, error) {
	db := consts.GetGormProfileDatabase()
	cfg, err := LoadEmbeddingEndpointConfig()
	if err != nil {
		return nil, err
	}
	opts = append(opts, aispec.WithBaseURL(cfg.BaseURL))
	return NewSQLitePluginsRagManager(db, collectionName, cfg.Model, cfg.Dimension, opts...)
}

// CreateSQLiteManager 创建一个基于 SQLite 的插件 RAG 管理器
func CreateSQLiteManager(collectionName string, modelName string, dimension int, opts ...aispec.AIConfigOption) (*PluginsRagManager, error) {
	db := consts.GetGormProfileDatabase()
	return NewSQLitePluginsRagManager(db, collectionName, modelName, dimension, opts...)
}

// IndexAllPlugins 索引所有未被忽略的插件
func IndexAllPlugins() error {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.IndexAllPlugins()
}

// IndexPlugin 索引单个插件
func IndexPlugin(scriptName string) error {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.IndexPlugin(scriptName)
}

// SearchPlugins 使用自然语言搜索插件
func SearchPlugins(query string, limit int) ([]*PluginSearchResult, error) {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return nil, err
	}
	return manager.SearchPlugins(query, limit)
}

// SearchPlugins 使用自然语言搜索插件
func SearchPluginIds(query string, limit int) ([]int64, error) {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return nil, err
	}
	return manager.SearchPluginsIds(query, limit)
}

func DeleteAllPlugins() error {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.Clear()
}

func DeletePlugin(scriptName string) error {
	manager, err := CreateDefaultSQLiteManager(PLUGIN_RAG_COLLECTION_NAME)
	if err != nil {
		return err
	}
	return manager.RemovePlugin(scriptName)
}

func IsReady() bool {
	db := consts.GetGormProfileDatabase()
	return rag.IsReadyCollection(db, PLUGIN_RAG_COLLECTION_NAME)
}

// 导出函数列表
var Exports = map[string]interface{}{
	// "CreateManager":   CreateDefaultSQLiteManager,
	"IndexAllPlugins": IndexAllPlugins,
	"IndexPlugin":     IndexPlugin,
	"SearchPlugins":   SearchPlugins,
	"IsReady":         IsReady,
}
