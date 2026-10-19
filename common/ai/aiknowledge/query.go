package aiknowledge

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const maxQueryBases = 20
const queryScopeHelp = `knowledge base scope is required: first call manage_knowledge_base with {"op":"list","limit":20,"offset":0}, then choose one exact name, multiple exact names via names, or explicitly set all=true to search every knowledge base`

type queryRequest struct {
	Name               string   `json:"name"`
	Names              []string `json:"names"`
	All                bool     `json:"all"`
	Query              string   `json:"query"`
	Mode               string   `json:"mode"`
	EntryID            string   `json:"entry_id"`
	Limit              int      `json:"limit"`
	Offset             int      `json:"offset"`
	TokenLimit         int      `json:"token_limit"`
	PerEntryTokenLimit int      `json:"per_entry_token_limit"`
	ContentOffset      int      `json:"content_offset"`
	MinScore           *float64 `json:"min_score"`
}

type Hit struct {
	KnowledgeBase     string   `json:"knowledge_base"`
	EntryID           string   `json:"entry_id"`
	Title             string   `json:"title"`
	Content           string   `json:"content"`
	Score             *float64 `json:"score,omitempty"`
	ContentOffset     int      `json:"content_offset"`
	NextContentOffset int      `json:"next_content_offset"`
	Truncated         bool     `json:"truncated"`
}

type QueryResult struct {
	Name               string `json:"name,omitempty"`
	All                bool   `json:"all,omitempty"`
	KnowledgeBaseCount int    `json:"knowledge_base_count"`
	Mode               string `json:"mode"`
	Results            []Hit  `json:"results"`
	Total              int    `json:"total"`
	HasMore            bool   `json:"has_more"`
	NextOffset         int    `json:"next_offset"`
	TokenLimit         int    `json:"token_limit"`
	UsedTokens         int    `json:"used_tokens"`
	BudgetExhausted    bool   `json:"budget_exhausted"`
}

func (r *queryRequest) validate() error {
	hasName := strings.TrimSpace(r.Name) != ""
	if !hasName && len(r.Names) == 0 && !r.All {
		return fmt.Errorf("%s", queryScopeHelp)
	}
	if (r.All && (hasName || len(r.Names) > 0)) || (hasName && len(r.Names) > 0) {
		return fmt.Errorf("choose exactly one scope: name, names or all=true")
	}
	if hasName {
		r.Names = []string{r.Name}
	}
	if len(r.Names) > maxQueryBases {
		return fmt.Errorf("names supports at most %d knowledge bases per call", maxQueryBases)
	}
	seen := map[string]bool{}
	for _, name := range r.Names {
		if strings.TrimSpace(name) == "" || len(name) > 256 || seen[name] {
			return fmt.Errorf("names must contain distinct non-empty exact names, each at most 256 bytes")
		}
		seen[name] = true
	}
	if r.TokenLimit < 256 || r.TokenLimit > 8192 {
		return fmt.Errorf("token_limit is required and must be 256..8192 (whole JSON response budget)")
	}
	if r.Limit == 0 {
		r.Limit = 5
	}
	if r.PerEntryTokenLimit == 0 {
		r.PerEntryTokenLimit = 512
	}
	if r.Mode == "" {
		r.Mode = "keyword"
	}
	if r.Limit < 1 || r.Limit > 50 || r.Offset < 0 || r.Offset > 100000 || r.ContentOffset < 0 {
		return fmt.Errorf("limit must be 1..50; offset must be 0..100000; content_offset must be non-negative")
	}
	if r.PerEntryTokenLimit < 128 || r.PerEntryTokenLimit > 8192 {
		return fmt.Errorf("per_entry_token_limit must be 128..8192")
	}
	if len(r.Query) > 4096 || len(r.EntryID) > 256 {
		return fmt.Errorf("query must be at most 4096 bytes; entry_id at most 256 bytes")
	}
	if r.Mode != "get" && (r.ContentOffset != 0 || r.EntryID != "") {
		return fmt.Errorf("entry_id and content_offset require mode=get")
	}
	if r.Mode != "semantic" && r.MinScore != nil {
		return fmt.Errorf("min_score requires mode=semantic")
	}
	if r.MinScore != nil && (*r.MinScore < -1 || *r.MinScore > 1) {
		return fmt.Errorf("min_score must be -1..1 (cosine similarity, higher is better)")
	}
	switch r.Mode {
	case "keyword":
	case "get":
		if r.All || len(r.Names) != 1 || r.EntryID == "" || r.Offset != 0 || r.Query != "" {
			return fmt.Errorf("mode=get requires one exact name, entry_id, offset=0 and no query; use the hit's knowledge_base and next_content_offset")
		}
	case "semantic":
		if strings.TrimSpace(r.Query) == "" || r.Offset != 0 {
			return fmt.Errorf("semantic requires query and offset=0; use limit to control top results")
		}
	default:
		return fmt.Errorf("mode must be keyword, semantic or get")
	}
	return nil
}

func (c *config) queryBases(req queryRequest) ([]*schema.KnowledgeBaseInfo, error) {
	query := c.db.Model(&schema.KnowledgeBaseInfo{})
	if !req.All {
		query = query.Where("knowledge_base_name IN (?)", req.Names)
	}
	var bases []*schema.KnowledgeBaseInfo
	if err := query.Select("id, knowledge_base_name").Order("id ASC").Limit(maxQueryBases + 1).Find(&bases).Error; err != nil {
		return nil, err
	}
	if len(bases) > maxQueryBases {
		return nil, fmt.Errorf("all=true exceeds the %d-base safety limit; call manage_knowledge_base with op=list, then query explicit names in batches of at most %d", maxQueryBases, maxQueryBases)
	}
	if !req.All && len(bases) != len(req.Names) {
		return nil, fmt.Errorf("one or more knowledge base names do not exist; call manage_knowledge_base with op=list to inspect exact names")
	}
	for _, base := range bases {
		if len(base.KnowledgeBaseName) > 256 {
			return nil, fmt.Errorf("knowledge base name exceeds the 256-byte query safety limit")
		}
	}
	return bases, c.ctx.Err()
}

type queryCandidate struct {
	entry *schema.KnowledgeBaseEntry
	base  string
	score *float64
}

// Query requires an explicit scope, never creates a missing base, and applies
// one limit and one token budget across the complete scope. Even the response
// envelope, JSON escaping and continuation metadata count towards token_limit.
func Query(input map[string]any, opts ...Option) (*QueryResult, error) {
	var req queryRequest
	if err := decode(input, &req); err != nil {
		return nil, err
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	c, err := newConfig(opts)
	if err != nil {
		return nil, err
	}
	bases, err := c.queryBases(req)
	if err != nil {
		return nil, err
	}
	result := &QueryResult{Name: req.Name, All: req.All, KnowledgeBaseCount: len(bases), Mode: req.Mode, Results: []Hit{}, NextOffset: req.Offset, TokenLimit: req.TokenLimit}
	var candidates []queryCandidate
	if len(bases) > 0 {
		if req.Mode == "semantic" {
			candidates, err = c.semanticCandidates(req, bases)
			result.Total = len(candidates) // Bounded ANN candidates, not an exhaustive count.
			if len(candidates) > req.Limit {
				candidates = candidates[:req.Limit]
			}
		} else {
			candidates, result.Total, err = c.contentCandidates(req, bases)
		}
		if err != nil {
			return nil, err
		}
	}
	for _, candidate := range candidates {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		row := candidate.entry
		// Semantic ranking reads IDs first and hydrates only the global top limit.
		if req.Mode == "semantic" {
			if err := c.db.Where("id = ? AND knowledge_base_id = ?", row.ID, row.KnowledgeBaseID).First(row).Error; err != nil {
				return nil, err
			}
		}
		content := []rune(row.KnowledgeDetails)
		if req.ContentOffset > len(content) {
			return nil, fmt.Errorf("content_offset exceeds entry length %d", len(content))
		}
		title := []rune(row.KnowledgeTitle)
		if len(title) > 128 {
			title = title[:128]
		}
		hit := Hit{KnowledgeBase: candidate.base, EntryID: row.HiddenIndex, Title: string(title), Score: candidate.score, ContentOffset: req.ContentOffset, NextContentOffset: req.ContentOffset}
		hit.Content = aicommon.ShrinkByTokens(string(content[req.ContentOffset:]), min(req.TokenLimit, req.PerEntryTokenLimit))
		accepted := false
		for {
			hit.NextContentOffset = req.ContentOffset + len([]rune(hit.Content))
			hit.Truncated = hit.NextContentOffset < len(content)
			result.Results = append(result.Results, hit)
			result.NextOffset = req.Offset + len(result.Results)
			result.HasMore = result.NextOffset < result.Total
			result.UsedTokens = result.tokenEstimate()
			data, _ := json.Marshal(hit)
			if result.UsedTokens <= req.TokenLimit && ytoken.CalcTokenCount(string(data)) <= req.PerEntryTokenLimit && (!hit.Truncated || hit.Content != "") {
				accepted = true
				break
			}
			result.Results = result.Results[:len(result.Results)-1]
			if hit.Content == "" {
				break
			}
			body := []rune(hit.Content)
			hit.Content = string(body[:len(body)*3/4])
		}
		if !accepted {
			result.BudgetExhausted = true
			break
		} // Never skip an unreturned hit.
	}
	result.NextOffset = req.Offset + len(result.Results)
	result.HasMore = result.NextOffset < result.Total
	result.UsedTokens = result.tokenEstimate()
	if result.UsedTokens > req.TokenLimit {
		return nil, fmt.Errorf("token_limit is too small for response metadata; increase token_limit")
	}
	return result, c.ctx.Err()
}

// Reserve eight tokens for the self-referential used_tokens value. The estimate
// is deliberately conservative, including the complete serialized envelope.
func (r QueryResult) tokenEstimate() int {
	r.UsedTokens = 0
	raw, _ := json.Marshal(r)
	return ytoken.CalcTokenCount(string(raw)) + 8
}

func (c *config) contentCandidates(req queryRequest, bases []*schema.KnowledgeBaseInfo) ([]queryCandidate, int, error) {
	ids, names := make([]uint, 0, len(bases)), map[uint]string{}
	for _, base := range bases {
		ids = append(ids, base.ID)
		names[base.ID] = base.KnowledgeBaseName
	}
	query := c.db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id IN (?)", ids)
	if req.Mode == "get" {
		query = query.Where("hidden_index = ?", req.EntryID)
	} else if req.Query != "" {
		query = query.Where("instr(knowledge_title, ?) > 0 OR instr(knowledge_details, ?) > 0 OR instr(summary, ?) > 0 OR instr(keywords, ?) > 0", req.Query, req.Query, req.Query, req.Query)
	}
	total := 0
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if req.Mode == "get" && total == 0 {
		return nil, 0, fmt.Errorf("entry_id not found in the specified knowledge base")
	}
	var rows []*schema.KnowledgeBaseEntry
	if err := query.Order("id ASC").Offset(req.Offset).Limit(req.Limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	var candidates []queryCandidate
	for _, row := range rows {
		candidates = append(candidates, queryCandidate{entry: row, base: names[uint(row.KnowledgeBaseID)]})
	}
	return candidates, total, c.ctx.Err()
}

func (c *config) semanticCandidates(req queryRequest, bases []*schema.KnowledgeBaseInfo) ([]queryCandidate, error) {
	var candidates []queryCandidate
	seen := map[uint]bool{}
	for _, base := range bases {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		kb, err := c.load(base.KnowledgeBaseName)
		if err != nil {
			return nil, err
		}
		if err := requireEmbedding(kb.GetVectorStore()); err != nil {
			return nil, err
		}
		matches, err := kb.GetVectorStore().SearchWithFilter(req.Query, 1, min(req.Limit*4, 100), func(_ string, getDoc func() *vectorstore.Document) bool {
			doc := getDoc()
			return doc != nil && (doc.Type == schema.RAGDocumentType_Knowledge || doc.Type == schema.RAGDocumentType_QuestionIndex || utils.InterfaceToInt(doc.Metadata["knowledge_base_id"]) == int(base.ID))
		})
		if err != nil {
			return nil, fmt.Errorf("semantic search in %q: %w", base.KnowledgeBaseName, err)
		}
		for _, match := range matches {
			if err := c.ctx.Err(); err != nil {
				return nil, err
			}
			if math.IsNaN(match.Score) || math.IsInf(match.Score, 0) {
				return nil, fmt.Errorf("semantic search returned a non-finite score in %q", base.KnowledgeBaseName)
			}
			if req.MinScore != nil && match.Score < *req.MinScore {
				continue
			}
			id := utils.InterfaceToString(match.Document.Metadata[schema.META_Data_UUID])
			if id == "" {
				id = utils.InterfaceToString(match.Document.Metadata["original_doc_id"])
			}
			if id == "" {
				id = match.Document.ID
			}
			var entry schema.KnowledgeBaseEntry
			lookup := c.db.Select("id, hidden_index, knowledge_base_id").Where("knowledge_base_id = ?", base.ID)
			err := lookup.Where("hidden_index = ?", id).First(&entry).Error
			// Older Yakit rebuilds used SQL primary keys as vector document IDs.
			if gorm.IsRecordNotFoundError(err) {
				if rowID, parseErr := strconv.ParseUint(id, 10, 64); parseErr == nil && rowID > 0 {
					err = lookup.Where("id = ?", rowID).First(&entry).Error
				}
			}
			if err != nil {
				return nil, fmt.Errorf("semantic index refers to missing entry %q: %w", id, err)
			}
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			score := match.Score
			candidates = append(candidates, queryCandidate{entry: &entry, base: base.KnowledgeBaseName, score: &score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if *candidates[i].score == *candidates[j].score {
			return candidates[i].entry.ID < candidates[j].entry.ID
		}
		return *candidates[i].score > *candidates[j].score
	})
	return candidates, nil
}
