package airaghttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
)

// searchRequest /search 请求体
type searchRequest struct {
	Query       string   `json:"query"`
	Collections []string `json:"collections"`
	Limit       int      `json:"limit"`
}

// searchResultItem 单条搜索结果
type searchResultItem struct {
	Content string      `json:"content"`
	Score   float64     `json:"score"`
	Source  string      `json:"source"`
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
}

// handleSearch POST /search 同步向量搜索 (无 AI 问答, 快速返回)
// 关键词: sync vector search, rag.Query, knowledge base retrieve
func (s *RAGHTTPServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read request body failed: "+err.Error())
		return
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
			return
		}
	}

	if req.Query == "" {
		writeJSONError(w, http.StatusBadRequest, "missing query")
		return
	}

	collections := req.Collections
	if len(collections) == 0 {
		collections = s.readyCollections
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.config.Timeout)*time.Second)
	defer cancel()

	log.Infof("/search query=%q collections=%v limit=%d", req.Query, collections, limit)

	opts := []rag.RAGSystemConfigOption{
		rag.WithRAGCtx(ctx),
		rag.WithRAGCollectionNames(collections...),
		rag.WithRAGLimit(limit),
	}
	if s.embeddingClient != nil {
		opts = append(opts, rag.WithEmbeddingClient(s.embeddingClient))
	}

	ch, err := rag.Query(s.db, req.Query, opts...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "search failed: "+err.Error())
		return
	}

	results := make([]searchResultItem, 0)
	for item := range ch {
		if item == nil {
			continue
		}
		// 仅收集最终结果条目, 丢弃 message / mid_result 等过程性消息
		if item.Type != rag.RAGResultTypeResult {
			continue
		}
		results = append(results, searchResultItem{
			Content: item.GetContent(),
			Score:   item.Score,
			Source:  item.Source,
			Type:    item.Type,
			Data:    item.Data,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"query":   req.Query,
		"total":   len(results),
		"results": results,
	})
}
