package aiknowledge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type queryRequest struct {
	Name          string `json:"name"`
	Query         string `json:"query"`
	Mode          string `json:"mode"`
	EntryID       string `json:"entry_id"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
	TokenLimit    int    `json:"token_limit"`
	ContentOffset int    `json:"content_offset"`
}

type Hit struct {
	EntryID           string   `json:"entry_id"`
	Title             string   `json:"title"`
	Content           string   `json:"content"`
	Score             *float64 `json:"score,omitempty"`
	ContentOffset     int      `json:"content_offset"`
	NextContentOffset int      `json:"next_content_offset"`
	Truncated         bool     `json:"truncated"`
}

type QueryResult struct {
	Name            string `json:"name"`
	Mode            string `json:"mode"`
	Results         []Hit  `json:"results"`
	Total           int    `json:"total"`
	HasMore         bool   `json:"has_more"`
	NextOffset      int    `json:"next_offset"`
	TokenLimit      int    `json:"token_limit"`
	UsedTokens      int    `json:"used_tokens"`
	BudgetExhausted bool   `json:"budget_exhausted"`
}

// Query never selects a base implicitly or creates one on a miss. token_limit
// bounds the sum of JSON-serialized hits, including their metadata. Long bodies
// can be continued by exact entry_id and Unicode character content_offset.
func Query(input map[string]any, opts ...Option) (*QueryResult, error) {
	var req queryRequest
	if err := decode(input, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required (exact knowledge base name)")
	}
	if req.TokenLimit < 128 || req.TokenLimit > 32768 {
		return nil, fmt.Errorf("token_limit is required and must be 128..32768")
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	if req.Mode == "" {
		req.Mode = "keyword"
	}
	if req.Limit < 1 || req.Limit > 100 || req.Offset < 0 || req.ContentOffset < 0 {
		return nil, fmt.Errorf("limit must be 1..100; offset and content_offset non-negative")
	}
	if req.Mode != "get" && req.ContentOffset != 0 {
		return nil, fmt.Errorf("content_offset requires mode=get and entry_id")
	}
	c, err := newConfig(opts)
	if err != nil {
		return nil, err
	}
	base, err := yakit.GetKnowledgeBaseByName(c.db, req.Name)
	if err != nil {
		return nil, fmt.Errorf("knowledge base %q: %w", req.Name, err)
	}
	rows := []*schema.KnowledgeBaseEntry{}
	scores := map[string]float64{}
	total := 0
	query := c.db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", base.ID)
	switch req.Mode {
	case "keyword":
		if req.Query != "" {
			query = query.Where("instr(knowledge_title, ?) > 0 OR instr(knowledge_details, ?) > 0 OR instr(summary, ?) > 0 OR instr(keywords, ?) > 0", req.Query, req.Query, req.Query, req.Query)
		}
		if err := query.Count(&total).Error; err != nil {
			return nil, err
		}
		if err := query.Order("id ASC").Offset(req.Offset).Limit(req.Limit).Find(&rows).Error; err != nil {
			return nil, err
		}
	case "get":
		if req.EntryID == "" || req.Offset != 0 {
			return nil, fmt.Errorf("mode=get requires entry_id and offset=0")
		}
		var entry schema.KnowledgeBaseEntry
		if err := query.Where("hidden_index = ?", req.EntryID).First(&entry).Error; err != nil {
			return nil, err
		}
		rows, total = append(rows, &entry), 1
	case "semantic":
		if strings.TrimSpace(req.Query) == "" || req.Offset != 0 {
			return nil, fmt.Errorf("semantic requires query and offset=0; use limit to control top results")
		}
		kb, err := c.load(req.Name)
		if err != nil {
			return nil, err
		}
		if err := requireEmbedding(kb.GetVectorStore()); err != nil {
			return nil, err
		}
		matches, err := kb.GetVectorStore().SearchWithFilter(req.Query, 1, req.Limit*4, func(_ string, getDoc func() *vectorstore.Document) bool {
			doc := getDoc()
			return doc != nil && (doc.Type == schema.RAGDocumentType_Knowledge || doc.Type == schema.RAGDocumentType_QuestionIndex || utils.InterfaceToInt(doc.Metadata["knowledge_base_id"]) == int(base.ID))
		})
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			if err := c.ctx.Err(); err != nil {
				return nil, err
			}
			id := utils.InterfaceToString(match.Document.Metadata[schema.META_Data_UUID])
			if id == "" {
				id = utils.InterfaceToString(match.Document.Metadata["original_doc_id"])
			}
			if id == "" {
				id = match.Document.ID
			}
			var entry schema.KnowledgeBaseEntry
			err := c.db.Where("knowledge_base_id = ? AND hidden_index = ?", base.ID, id).First(&entry).Error
			// Older Yakit index rebuilds used the SQL primary key as document ID.
			// Resolve that legacy identity only inside the explicitly named base.
			if gorm.IsRecordNotFoundError(err) {
				if rowID, parseErr := strconv.ParseUint(id, 10, 64); parseErr == nil && rowID > 0 {
					err = c.db.Where("knowledge_base_id = ? AND id = ?", base.ID, rowID).First(&entry).Error
				}
			}
			if err != nil {
				return nil, fmt.Errorf("semantic index refers to missing entry %q: %w", id, err)
			}
			if _, seen := scores[entry.HiddenIndex]; seen {
				continue
			}
			scores[entry.HiddenIndex] = match.Score
			rows = append(rows, &entry)
			if len(rows) == req.Limit {
				break
			}
		}
		total = len(rows)
	default:
		return nil, fmt.Errorf("mode must be keyword, semantic or get")
	}
	result := &QueryResult{Name: req.Name, Mode: req.Mode, Results: []Hit{}, Total: total, NextOffset: req.Offset, TokenLimit: req.TokenLimit}
	for _, row := range rows {
		content := []rune(row.KnowledgeDetails)
		if req.ContentOffset > len(content) {
			return nil, fmt.Errorf("content_offset exceeds entry length %d", len(content))
		}
		title := []rune(row.KnowledgeTitle)
		if len(title) > 256 {
			title = title[:256]
		}
		hit := Hit{EntryID: row.HiddenIndex, Title: string(title), ContentOffset: req.ContentOffset, NextContentOffset: req.ContentOffset}
		if score, ok := scores[row.HiddenIndex]; ok {
			hit.Score = &score
		}
		remaining := req.TokenLimit - result.UsedTokens
		hit.Content = aicommon.ShrinkByTokens(string(content[req.ContentOffset:]), remaining)
		hit.NextContentOffset += len([]rune(hit.Content))
		hit.Truncated = hit.NextContentOffset < len(content)
		// Include JSON escaping and metadata in the budget, not just the body.
		for {
			data, err := json.Marshal(hit)
			if err != nil {
				return nil, err
			}
			tokens := ytoken.CalcTokenCount(string(data))
			if tokens <= remaining {
				if hit.Truncated && hit.Content == "" {
					result.BudgetExhausted = true
					break
				}
				result.UsedTokens += tokens
				result.Results = append(result.Results, hit)
				break
			}
			if hit.Content == "" {
				result.BudgetExhausted = true
				break
			}
			body := []rune(hit.Content)
			hit.Content = string(body[:len(body)*3/4])
			hit.NextContentOffset = req.ContentOffset + len([]rune(hit.Content))
			hit.Truncated = true
		}
		if result.BudgetExhausted {
			break
		}
	}
	result.NextOffset += len(result.Results)
	result.HasMore = result.NextOffset < total
	if len(result.Results) < len(rows) {
		result.BudgetExhausted = true
	}
	return result, c.ctx.Err()
}
