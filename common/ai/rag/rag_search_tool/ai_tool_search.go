package rag_search_tool

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/generate_index_tool"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const AIToolVectorIndexName = "AI_TOOL_VECTOR_INDEX"
const ForgeVectorIndexName = "FORGE_VECTOR_INDEX"

var SimpleLiteForge contracts.LiteForge

func NewRAGSearcher[T searchtools.AISearchable](name string) (searchtools.AISearcher[T], error) {
	db := consts.GetGormProfileDatabase()
	ragSystem, err := rag.GetRagSystem(name, rag.WithDB(db))
	if err != nil {
		return nil, utils.Errorf("load collection failed: %v", err)
	}
	return func(query string, searchList []T) ([]T, error) {
		if ragSystem == nil {
			return nil, utils.Errorf("ragSystem is not initialized")
		}
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("failed to query with page query: %s: %v", query, err)
				fmt.Println(utils.ErrorStack(err))
			}
		}()

		searchListMap := map[string]T{}
		for _, tool := range searchList {
			searchListMap[tool.GetName()] = tool
		}

		results, err := ragSystem.QueryWithFilter(query, 1, 20, func(key string, getDoc func() *vectorstore.Document) bool {
			if _, ok := searchListMap[key]; ok {
				return true
			}
			return false
		})
		if err != nil {
			return nil, err
		}
		results = lo.Filter(results, func(item *rag.SearchResult, _ int) bool {
			return item.Score > 0.5
		})
		resTools := []T{}
		for _, result := range results {
			resTools = append(resTools, searchListMap[result.Document.ID])
		}

		return resTools, nil
	}, nil
}

func NewMergeSearchr[T searchtools.AISearchable](searchs ...searchtools.AISearcher[T]) searchtools.AISearcher[T] {
	disableSearcherIndex := []int{}
	return func(query string, searchList []T) ([]T, error) {
		for i, search := range searchs {
			if slices.Contains(disableSearcherIndex, i) {
				continue
			}
			if search == nil {
				continue
			}
			results, err := search(query, searchList)
			if err != nil {
				disableSearcherIndex = append(disableSearcherIndex, i)
				continue
			}
			return results, nil
		}
		return nil, utils.Errorf("no valid searcher found")
	}
}

// NewComprehensiveSearcher 综合 RAG 和 关键词 查询工具
func NewComprehensiveSearcher[T searchtools.AISearchable](name string, chatToAiFunc func(string) (io.Reader, error)) searchtools.AISearcher[T] {
	searchs := []searchtools.AISearcher[T]{}
	ragSearcher, err := NewRAGSearcher[T](name)
	if err != nil {
		log.Errorf("failed to create RAG searcher: %v", err)
	}
	keywordSearcher := searchtools.NewKeyWordSearcher[T](chatToAiFunc)

	searchs = append(searchs, ragSearcher, keywordSearcher)
	return NewMergeSearchr(searchs...)
}

// BuildVectorIndexForSearcher 构建向量索引
func BuildVectorIndexForSearcher[T searchtools.AISearchable](db *gorm.DB, collectionName string, tools []T) (*rag.RAGSystem, error) {
	// TODO: 差量更新索引
	if SimpleLiteForge == nil {
		return nil, utils.Errorf("SimpleLiteForge is not initialized")
	}
	manager, err := generate_index_tool.CreateIndexManager(db, AIToolVectorIndexName, "AITool index", generate_index_tool.WithConcurrentWorkers(1), generate_index_tool.WithAIProcessor(SimpleLiteForge))
	if err != nil {
		return nil, utils.Errorf("create index manager failed: %v", err)
	}

	allToolItems := []generate_index_tool.IndexableItem{}
	for _, tool := range tools {
		content := fmt.Sprintf("名称: %s\n描述: %s\n关键词: %s", tool.GetName(), tool.GetDescription(), strings.Join(tool.GetKeywords(), ", "))
		allToolItems = append(allToolItems, generate_index_tool.NewCommonIndexableItem(
			tool.GetName(),
			content,
			map[string]interface{}{
				"name": tool.GetName(),
			},
			tool.GetName()))
	}
	_, err = manager.IndexItems(context.Background(), allToolItems)
	if err != nil {
		return nil, utils.Errorf("index items failed: %v", err)
	}

	ragSystem, err := rag.GetRagSystem(collectionName, rag.WithDB(db))
	if err != nil {
		return nil, utils.Errorf("load collection failed: %v", err)
	}
	return ragSystem, nil
}
