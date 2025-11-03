package generate_index_tool

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// CreateIndexManager 创建索引管理器的便捷函数
func CreateIndexManager(db *gorm.DB, collectionName, description string, optFuncs ...OptionFunc) (*IndexManager, error) {
	// 应用选项
	options := ApplyOptions(nil, optFuncs...)

	// 创建RAG系统
	ragSystem, err := rag.GetRagSystem(collectionName, rag.WithDB(db), rag.WithDescription(description))
	if err != nil {
		return nil, utils.Errorf("创建RAG系统失败: %v", err)
	}

	return NewIndexManager(db, ragSystem, collectionName, options), nil
}

// CreateIndexManagerWithAI 创建带AI配置的索引管理器
func CreateIndexManagerWithAI(db *gorm.DB, collectionName, description string, aiOpts []aispec.AIConfigOption, optFuncs ...OptionFunc) (*IndexManager, error) {
	// 应用选项
	options := ApplyOptions(nil, optFuncs...)

	// 准备RAG选项
	ragOptions := make([]any, len(aiOpts))
	for i, opt := range aiOpts {
		ragOptions[i] = opt
	}

	// 创建RAG系统
	ragSystem, err := rag.GetRagSystem(collectionName, rag.WithDB(db), rag.WithDescription(description), rag.WithAIOptions(aiOpts...))
	if err != nil {
		return nil, utils.Errorf("创建RAG系统失败: %v", err)
	}

	return NewIndexManager(db, ragSystem, collectionName, options), nil
}

// QuickIndexScripts 快速索引脚本的便捷函数
func QuickIndexScripts(db *gorm.DB, collectionName string, scripts []*schema.YakScript, optFuncs ...OptionFunc) (*IndexResult, error) {
	// 创建索引管理器
	manager, err := CreateIndexManager(db, collectionName, "脚本向量库", optFuncs...)
	if err != nil {
		return nil, err
	}

	// 转换为可索引项
	items := ConvertScriptsToIndexableItems(scripts)

	// 执行索引
	return manager.IndexItems(context.Background(), items)
}
