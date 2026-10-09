package aimemory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Search reads an existing memory set. It does not initialize triage,
// create collections or update injection. Returned items expose raw readable Dump text.
func Search(query string, opts ...Option) ([]*Item, error) {
	c := &options{namespace: "default", mode: "hybrid", limit: 5, tokens: 1500, ctx: context.Background()}
	for _, opt := range opts {
		opt(c)
	}
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > 1024 {
		return nil, fmt.Errorf("query must contain 1 to 1024 characters")
	}
	if c.limit < 1 || c.limit > 20 || c.tokens < 64 || c.tokens > 8192 {
		return nil, fmt.Errorf("memory limit must be 1..20 and token limit must be 64..8192")
	}
	if c.mode != "bm25" && c.mode != "vector" && c.mode != "hybrid" {
		return nil, fmt.Errorf("memory search mode must be bm25, vector or hybrid")
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("memory search context is nil")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.db == nil {
		c.db = consts.GetGormProjectDatabase()
	}
	empty := []*Item{}
	if !c.db.HasTable(&schema.AIMemoryEntity{}) {
		return empty, nil
	}
	live := c.db.Model(&schema.AIMemoryEntity{}).Where("session_id = ? AND (expires_at IS NULL OR expires_at > ?)", c.namespace, time.Now())
	var count int
	if err := live.Count(&count).Error; err != nil || count == 0 {
		return empty, err
	}
	var collection *schema.VectorStoreCollection
	if c.db.HasTable(&schema.VectorStoreCollection{}) {
		var err error
		collection, err = yakit.GetRAGCollectionInfoByName(c.db, "ai-memory-"+c.namespace)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	scores := map[string]float64{}
	add := func(ids []string) {
		seen := map[string]bool{}
		rank := 0
		for _, id := range ids {
			if id == "" || seen[id] {
				continue
			}
			seen[id], rank = true, rank+1
			scores[id] += 1 / float64(60+rank)
		}
	}
	idsFromDocuments := func(docs []*schema.VectorStoreDocument) []string {
		ids := []string{}
		for _, doc := range docs {
			if id, ok := doc.Metadata["memory_id"].(string); ok {
				ids = append(ids, id)
			}
		}
		return ids
	}
	candidates := c.limit * 8
	if c.mode != "vector" {
		if collection != nil {
			docs, err := yakit.SearchVectorStoreDocumentBM25(c.db, &yakit.VectorDocumentFilter{CollectionUUID: collection.UUID, Keywords: []string{query}}, candidates, 0)
			if err != nil {
				return nil, err
			}
			add(idsFromDocuments(docs))
		}
		// Indexed questions may omit content/tags, including older unindexed rows.
		pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query) + "%"
		var rows []*schema.AIMemoryEntity
		if err := live.Where("(content LIKE ? ESCAPE '\\' OR tags LIKE ? ESCAPE '\\' OR potential_questions LIKE ? ESCAPE '\\')", pattern, pattern, pattern).Order("id DESC").Limit(candidates).Find(&rows).Error; err != nil {
			return nil, err
		}
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row.MemoryID)
		}
		add(ids)
	}
	if c.mode != "bm25" {
		ids, err := vectorMemoryIDs(c.db, collection, query, candidates)
		if err != nil {
			if c.mode == "vector" {
				return nil, err
			}
			log.Warnf("memory vector search unavailable; using lexical results: %v", err)
		} else {
			add(ids)
		}
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return empty, nil
	}
	_, rows, err := yakit.QueryAIMemoryEntityPaging(live, &ypb.AIMemoryEntityFilter{SessionID: c.namespace, MemoryID: ids}, &ypb.Paging{Page: 1, Limit: int64(len(ids))})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if scores[rows[i].MemoryID] == scores[rows[j].MemoryID] {
			return rows[i].MemoryID < rows[j].MemoryID
		}
		return scores[rows[i].MemoryID] > scores[rows[j].MemoryID]
	})
	if len(rows) > c.limit {
		rows = rows[:c.limit]
	}
	result := empty
	remaining := c.tokens
	for _, row := range rows {
		if remaining <= 0 {
			break
		}
		row.Content = aicommon.ShrinkByTokens(row.Content, remaining)
		if row.Content == "" || remaining <= 0 {
			break
		}
		remaining -= ytoken.CalcTokenCount(row.Content)
		result = append(result, &Item{AIMemoryEntity: row})
	}
	return result, nil
}

func vectorMemoryIDs(db *gorm.DB, collection *schema.VectorStoreCollection, query string, limit int) ([]string, error) {
	if collection == nil {
		return nil, fmt.Errorf("memory vector index is unavailable")
	}
	store, err := vectorstore.LoadCollection(db, collection.Name, vectorstore.WithLazyLoadEmbeddingClient(),
		vectorstore.WithTryRebuildHNSWIndex(false), vectorstore.WithAutoDeleteCorruptedRAG(false), vectorstore.WithEnableAutoUpdateGraphInfos(false))
	if err != nil {
		return nil, err
	}
	if store.GetEmbedder() == nil {
		return nil, fmt.Errorf("memory embedding service is unavailable")
	}
	hits, err := store.Search(query, 1, limit)
	ids := []string{}
	for _, hit := range hits {
		if hit.Document != nil {
			if id, ok := hit.Document.Metadata["memory_id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids, err
}
