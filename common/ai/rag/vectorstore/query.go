package vectorstore

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// QueryWithPage 根据查询文本检索相关文档并返回结果
func (r *SQLiteVectorStoreHNSW) QueryWithPage(query string, page, limit int) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	return r.Search(query, page, limit)
}

func (r *SQLiteVectorStoreHNSW) QueryWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	results, err := r.SearchWithFilter(query, page, limit, func(key string, getDoc func() *Document) bool {
		if filter != nil {
			return filter(key, getDoc)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// FuzzRawSearch Sql 文本模糊搜索（非语义）
func (r *SQLiteVectorStoreHNSW) FuzzRawSearch(ctx context.Context, keywords string, limit int) (<-chan SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", keywords, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	return r.FuzzSearch(ctx, keywords, limit)
}

// Query is short for QueryTopN
func (r *SQLiteVectorStoreHNSW) Query(query string, topN int, limits ...float64) ([]SearchResult, error) {
	return r.QueryTopN(query, topN, limits...)
}

// QueryTopN 根据查询文本检索相关文档并返回结果
func (r *SQLiteVectorStoreHNSW) QueryTopN(query string, topN int, limits ...float64) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query top_n %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	if topN <= 0 {
		topN = 20
	}

	var page = 1
	var limit float64 = -1
	if len(limits) > 0 {
		limit = limits[0]
	}

	if limit >= 1 {
		topN = utils.Max(topN, int(limit))
		log.Warnf("limit should be less than 1, got %f, using -1 instead, use topN: %v (Max(topN, int(limit:%v)))", limit, topN, limit)
		limit = -1
	}

	log.Infof("start to search in vector storage with query: %#v", query)
	results, err := r.Search(query, page, topN)
	if err != nil {
		return nil, err
	}

	var filteredResults []SearchResult
	for _, result := range results {
		if limit < 0 || result.Score >= limit {
			filteredResults = append(filteredResults, result)
		}
	}

	return filteredResults, nil
}
