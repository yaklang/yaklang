package yaklib

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	YakitExports["QueryCybersecurityRisk"] = QueryCybersecurityRisk
	YakitExports["QueryKnowledge"] = QueryKnowledge
	YakitExports["QueryPayloads"] = QueryPayloads
	YakitExports["ManagePayloads"] = ManagePayloads
	YakitExports["ManageKnowledge"] = ManageKnowledge
}

func decodeAIResourceFilter(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if len(raw) > 1<<20 || !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return fmt.Errorf("filter must be a JSON object of at most 1 MiB")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid resource filter: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("filter must contain exactly one JSON object")
	}
	return nil
}

func aiResourcePage(c *aiQueryConfig, hits []map[string]any, total int) map[string]any {
	return map[string]any{"hits": hits, "total": total, "offset": c.offset, "next_offset": c.offset + len(hits), "has_more": c.offset+len(hits) < total}
}

func aiResourceContent(hit map[string]any, content string, c *aiQueryConfig) {
	value, next, truncated := aiContentSlice(content, c.contentOffset, c.contentLimit)
	hit["content"], hit["content_offset"], hit["next_content_offset"], hit["truncated"] = value, c.contentOffset, next, truncated
}

func aiUnquote(s string) string {
	if raw, err := strconv.Unquote(s); err == nil {
		return raw
	}
	return s
}

// QueryCybersecurityRisk reuses Yakit's risk filters without changing read state.
// An omitted WaitingVerified includes both states; an explicit bool selects one.
func QueryCybersecurityRisk(filterJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	req := &ypb.QueryRisksRequest{}
	if err := decodeAIResourceFilter(filterJSON, req); err != nil {
		return nil, err
	}
	if req.Token != "" || req.Pagination != nil {
		return nil, fmt.Errorf("Token/Pagination are unsupported; use limit/offset")
	}
	if req.IsRead != "" && req.IsRead != "true" && req.IsRead != "false" {
		return nil, fmt.Errorf("IsRead must be true or false")
	}
	db, closeDB, err := aiHTTPDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	var supplied map[string]json.RawMessage
	if strings.TrimSpace(filterJSON) != "" {
		_ = json.Unmarshal([]byte(filterJSON), &supplied)
	}
	explicitWaiting := false
	for key, value := range supplied {
		if strings.EqualFold(key, "WaitingVerified") {
			if string(value) != "true" && string(value) != "false" {
				return nil, fmt.Errorf("WaitingVerified must be a boolean")
			}
			explicitWaiting = true
		}
	}
	query := yakit.FilterByQueryRisks(db, req)
	if !explicitWaiting {
		other := &ypb.QueryRisksRequest{}
		if err := decodeAIResourceFilter(filterJSON, other); err != nil {
			return nil, err
		}
		other.WaitingVerified = !req.WaitingVerified
		query = db.Model(&schema.Risk{}).Where("id IN (?) OR id IN (?)", query.Select("id").SubQuery(), yakit.FilterByQueryRisks(db, other).Select("id").SubQuery())
	}
	if req.IsRead == "true" {
		query = query.Where("is_read = ?", true)
	}
	if req.FromId > 0 {
		query = query.Where("id > ?", req.FromId)
	}
	if req.UntilId > 0 {
		query = query.Where("id <= ?", req.UntilId)
	}
	var total int
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	fields := "id, created_at, title, title_verbose, url, ip, port, severity, risk_type, risk_type_verbose, tags, runtime_id, from_yak_script, waiting_verified, is_read, cve, program_name, result_id"
	detail := len(req.Ids) == 1
	if detail {
		fields += ", description, solution, parameter, payload, details, quoted_request, quoted_response, packet_pairs"
	}
	var rows []*schema.Risk
	if err := query.Select(fields).Order("id DESC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		hit := map[string]any{"risk_id": row.ID, "title": row.Title, "title_verbose": row.TitleVerbose, "url": row.Url, "ip": row.IP, "port": row.Port, "severity": row.Severity, "risk_type": row.RiskType, "risk_type_verbose": row.RiskTypeVerbose, "tags": row.Tags, "runtime_id": row.RuntimeId, "from_plugin": row.FromYakScript, "waiting_verified": row.WaitingVerified, "is_read": row.IsRead, "cve": row.CVE, "program_name": row.ProgramName, "result_id": row.ResultID, "created_at": row.CreatedAt}
		if detail {
			flowIDs := make([]int64, 0)
			for _, pair := range row.PacketPairs {
				if pair != nil && pair.HTTPFlowId > 0 && len(flowIDs) < 50 {
					flowIDs = append(flowIDs, pair.HTTPFlowId)
				}
			}
			hit["http_flow_ids"] = flowIDs
			raw, err := json.Marshal(map[string]any{"description": row.Description, "solution": row.Solution, "parameter": row.Parameter, "payload": row.Payload, "details": aiUnquote(row.Details), "request": aiUnquote(row.QuotedRequest), "response": aiUnquote(row.QuotedResponse), "packet_pairs": row.PacketPairs})
			if err != nil {
				return nil, err
			}
			aiResourceContent(hit, string(raw), c)
			hit["content_format"] = "json"
		}
		hits = append(hits, hit)
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	result := aiResourcePage(c, hits, total)
	result["project_id"] = c.projectID
	result["next_step"] = "Read one risk with risk_id; follow http_flow_ids using query_http_history with the same project_id. Continue truncated content with content_offset."
	return result, nil
}

type aiKnowledgeFilter struct {
	Source          string `json:"source"`
	Query           string `json:"query"`
	Collection      string `json:"collection"`
	KnowledgeBaseID int64  `json:"knowledge_base_id"`
	EntryID         int64  `json:"entry_id"`
	EntryUUID       string `json:"entry_uuid"`
	DocumentID      string `json:"document_id"`
}

// QueryKnowledge searches original entries, or BM25 question/document indices,
// and provides stable IDs so the AI can retrieve the source after discovering it.
func QueryKnowledge(filterJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	f := aiKnowledgeFilter{Source: "entries"}
	if err := decodeAIResourceFilter(filterJSON, &f); err != nil {
		return nil, err
	}
	if f.KnowledgeBaseID < 0 || f.EntryID < 0 {
		return nil, fmt.Errorf("knowledge IDs must be nonnegative")
	}
	if c.projectID != 0 {
		return nil, fmt.Errorf("knowledge uses the runtime profile database; project_id is unsupported")
	}
	db := c.profileDB
	if db == nil {
		return nil, fmt.Errorf("profile database is unavailable")
	}
	if f.Source != "entries" && f.Source != "collections" && f.Source != "documents" && f.Source != "vector_collections" {
		return nil, fmt.Errorf("source must be entries, collections, vector_collections or documents")
	}
	if (f.Source != "entries" && (f.EntryID != 0 || f.EntryUUID != "" || f.KnowledgeBaseID != 0)) || (f.Source != "documents" && f.DocumentID != "") {
		return nil, fmt.Errorf("entry selectors require source=entries; document_id requires source=documents; knowledge_base_id requires source=entries")
	}
	hits := make([]map[string]any, 0)
	var total int
	switch f.Source {
	case "vector_collections":
		q := db.Model(&schema.VectorStoreCollection{})
		if f.Query != "" {
			q = q.Where("instr(lower(COALESCE(name,'') || ' ' || COALESCE(description,'')),lower(?)) > 0", f.Query)
		}
		if f.Collection != "" {
			q = q.Where("name = ?", f.Collection)
		}
		if err := q.Count(&total).Error; err != nil {
			return nil, err
		}
		var rows []*schema.VectorStoreCollection
		if err := q.Select("id, name, description, uuid, rag_id").Order("id DESC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			var count int
			if err := db.Model(&schema.VectorStoreDocument{}).Where("collection_uuid = ?", row.UUID).Count(&count).Error; err != nil {
				return nil, err
			}
			hit := map[string]any{"name": row.Name, "collection_uuid": row.UUID, "rag_id": row.RAGID, "document_count": count}
			aiResourceContent(hit, row.Description, c)
			hits = append(hits, hit)
		}
	case "collections":
		q := db.Model(&schema.KnowledgeBaseInfo{})
		if f.Query != "" {
			q = q.Where("instr(lower(COALESCE(knowledge_base_name,'') || ' ' || COALESCE(knowledge_base_description,'')),lower(?)) > 0", f.Query)
		}
		if f.Collection != "" {
			q = q.Where("knowledge_base_name = ?", f.Collection)
		}
		if err := q.Count(&total).Error; err != nil {
			return nil, err
		}
		var rows []*schema.KnowledgeBaseInfo
		if err := q.Order("id DESC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			var count int
			if err := db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", row.ID).Count(&count).Error; err != nil {
				return nil, err
			}
			description, _, _ := aiContentSlice(row.KnowledgeBaseDescription, 0, c.contentLimit)
			hits = append(hits, map[string]any{"knowledge_base_id": row.ID, "name": row.KnowledgeBaseName, "description": description, "type": row.KnowledgeBaseType, "rag_id": row.RAGID, "entry_count": count})
		}
	case "entries":
		q := yakit.FilterKnowledgeBaseEntry(db.Model(&schema.KnowledgeBaseEntry{}), &ypb.SearchKnowledgeBaseEntryFilter{KnowledgeBaseId: f.KnowledgeBaseID})
		if f.Collection != "" {
			kb, err := yakit.GetKnowledgeBaseByName(db, f.Collection)
			if err != nil {
				return nil, err
			}
			q = q.Where("knowledge_base_id = ?", kb.ID)
		}
		if f.EntryID > 0 {
			q = q.Where("id = ?", f.EntryID)
		}
		if f.EntryUUID != "" {
			q = q.Where("hidden_index = ?", f.EntryUUID)
		}
		if f.Query != "" {
			q = q.Where("instr(lower(COALESCE(knowledge_title,'') || ' ' || COALESCE(knowledge_details,'') || ' ' || COALESCE(keywords,'') || ' ' || COALESCE(summary,'') || ' ' || COALESCE(potential_questions,'')), lower(?)) > 0", f.Query)
		}
		if err := q.Count(&total).Error; err != nil {
			return nil, err
		}
		var rows []*schema.KnowledgeBaseEntry
		if err := q.Select("id, knowledge_base_id, hidden_index, knowledge_title, knowledge_type, importance_score, summary, knowledge_details, source_page").Order("importance_score DESC, id DESC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			summary, _, _ := aiContentSlice(row.Summary, 0, 256)
			hit := map[string]any{"entry_id": row.ID, "entry_uuid": row.HiddenIndex, "knowledge_base_id": row.KnowledgeBaseID, "title": row.KnowledgeTitle, "type": row.KnowledgeType, "importance": row.ImportanceScore, "summary": summary, "source_page": row.SourcePage}
			aiResourceContent(hit, row.KnowledgeDetails, c)
			hits = append(hits, hit)
		}
	case "documents":
		filter := &yakit.VectorDocumentFilter{}
		if f.Collection != "" {
			var collection schema.VectorStoreCollection
			if err := db.Select("uuid").Where("name = ?", f.Collection).First(&collection).Error; err != nil {
				return nil, err
			}
			filter.CollectionUUID = collection.UUID
		}
		if f.DocumentID != "" {
			filter.DocumentIDs = []string{f.DocumentID}
		}
		if f.Query != "" {
			filter.Keywords = []string{f.Query}
		}
		literalFallback := len([]rune(strings.TrimSpace(f.Query))) < 3 || !db.HasTable(yakit.VectorDocumentVTableName())
		var rows []*schema.VectorStoreDocument
		// Searching does not need embeddings or PQ codes; keep them out of memory.
		docDB := db.Select("id, document_id, collection_uuid, document_type, metadata, content")
		if literalFallback {
			err = yakit.FilterVectorDocuments(docDB, filter).Order("id DESC").Limit(c.limit + 1).Offset(c.offset).Find(&rows).Error
		} else {
			rows, err = yakit.SearchVectorStoreDocumentBM25(docDB, filter, c.limit+1, c.offset)
		}
		if err != nil {
			return nil, err
		}
		hasMore := len(rows) > c.limit
		if hasMore {
			rows = rows[:c.limit]
		}
		for _, row := range rows {
			entryUUID, _ := row.Metadata.GetKnowledgeEntryUUID()
			title, _ := row.Metadata.GetTitle()
			hit := map[string]any{"document_id": row.DocumentID, "collection_uuid": row.CollectionUUID, "entry_uuid": entryUUID, "title": title, "document_type": row.DocumentType}
			aiResourceContent(hit, row.Content, c)
			hits = append(hits, hit)
		}
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		result := map[string]any{"hits": hits, "source": f.Source, "offset": c.offset, "next_offset": c.offset + len(hits), "has_more": hasMore, "search_mode": "bm25", "literal_fallback": literalFallback}
		result["next_step"] = "Use entry_uuid with source=entries to read the original knowledge; use content_offset to continue long content. Use source=vector_collections to find RAG names for collection."
		return result, nil
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	result := aiResourcePage(c, hits, total)
	result["source"] = f.Source
	result["next_step"] = "Read entries by entry_id or entry_uuid; continue truncated content with content_offset. Search source=documents for BM25 question/document indices; source=vector_collections discovers their collection names. collection is the exact knowledge base name for entries."
	return result, nil
}
