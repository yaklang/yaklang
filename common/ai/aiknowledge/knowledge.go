// Package aiknowledge exposes the profile knowledge base through bounded,
// explicitly scoped operations shared by Yak scripts and AI tools.
package aiknowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type config struct {
	db        *gorm.DB
	ctx       context.Context
	embedding aispec.EmbeddingCaller
	ai        aicommon.AICallbackType
}

type Option func(*config)

func WithDatabase(db *gorm.DB) Option        { return func(c *config) { c.db = db } }
func WithContext(ctx context.Context) Option { return func(c *config) { c.ctx = ctx } }
func WithEmbeddingClient(client aispec.EmbeddingCaller) Option {
	return func(c *config) { c.embedding = client }
}
func WithAIService(ai aicommon.AICallbackType) Option { return func(c *config) { c.ai = ai } }

var Exports = map[string]any{
	"Manage":    Manage,
	"Query":     Query,
	"db":        WithDatabase,
	"ctx":       WithContext,
	"aiService": WithAIService,
}

func newConfig(opts []Option) (*config, error) {
	c := &config{ctx: context.Background()}
	for _, opt := range opts {
		opt(c)
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("knowledge base requires a profile database and context")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.db == nil {
		c.db = consts.GetGormProfileDatabase()
	}
	if c.db == nil {
		return nil, fmt.Errorf("knowledge base requires a profile database")
	}
	return c, nil
}

func decode(input map[string]any, target any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

type metadata struct {
	Description *string   `json:"description"`
	Type        *string   `json:"type"`
	Tags        *[]string `json:"tags"`
	IsDefault   *bool     `json:"is_default"`
}

type indexOptions struct {
	EmbeddingModel string  `json:"embedding_model"`
	Dimension      int     `json:"dimension"`
	M              int     `json:"m"`
	Ml             float64 `json:"ml"`
	EfSearch       int     `json:"ef_search"`
	EfConstruct    int     `json:"ef_construct"`
}

func (o indexOptions) options() ([]vectorstore.CollectionConfigFunc, error) {
	if o.Dimension < 0 || o.Dimension > 65536 {
		return nil, fmt.Errorf("dimension must be 1..65536 when provided")
	}
	if o.M < 0 || o.M > 256 || o.Ml < 0 || o.Ml > 1 || o.EfSearch < 0 || o.EfSearch > 100000 || o.EfConstruct < 0 || o.EfConstruct > 100000 {
		return nil, fmt.Errorf("invalid HNSW parameters: m=1..256, ml=(0,1], ef_search/ef_construct positive")
	}
	opts := []vectorstore.CollectionConfigFunc{vectorstore.WithLazyLoadEmbeddingClient(), vectorstore.WithAutoDeleteCorruptedRAG(false), vectorstore.WithDisableEmbedCollectionInfo(true)}
	if o.EmbeddingModel != "" {
		opts = append(opts, vectorstore.WithEmbeddingModel(o.EmbeddingModel))
	}
	if o.Dimension != 0 {
		opts = append(opts, vectorstore.WithModelDimension(o.Dimension))
	}
	if o.M != 0 || o.Ml != 0 || o.EfSearch != 0 || o.EfConstruct != 0 {
		m, ml, search, construct := o.M, o.Ml, o.EfSearch, o.EfConstruct
		if m == 0 {
			m = 16
		}
		if ml == 0 {
			ml = 0.25
		}
		if search == 0 {
			search = 64
		}
		if construct == 0 {
			construct = 64
		}
		opts = append(opts, vectorstore.WithHNSWParameters(m, ml, search, construct))
	}
	return opts, nil
}

func (c *config) load(name string) (*knowledgebase.KnowledgeBase, error) {
	opts, _ := (indexOptions{}).options()
	if c.embedding != nil {
		opts = append(opts, vectorstore.WithEmbeddingClient(c.embedding))
	}
	return knowledgebase.LoadKnowledgeBase(c.db, name, opts...)
}

type baseInfo struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Tags        []string `json:"tags"`
	IsDefault   bool     `json:"is_default"`
}

func info(kb *schema.KnowledgeBaseInfo) baseInfo {
	tags := []string{}
	if kb.Tags != "" {
		tags = strings.Split(kb.Tags, ",")
	}
	return baseInfo{kb.ID, kb.KnowledgeBaseName, kb.KnowledgeBaseDescription, kb.KnowledgeBaseType, tags, kb.IsDefault}
}

type manageRequest struct {
	Op           string       `json:"op"`
	Name         string       `json:"name"`
	Keyword      string       `json:"keyword"`
	Limit        int          `json:"limit"`
	Offset       int          `json:"offset"`
	Metadata     metadata     `json:"metadata"`
	IndexOptions indexOptions `json:"index_options"`
	EntryID      string       `json:"entry_id"`
	Entry        *entryInput  `json:"entry"`
	Index        *bool        `json:"index"`
	SourceFile   string       `json:"source_file"`
	Content      string       `json:"content"`
	BuildOptions buildOptions `json:"build_options"`
}

// Manage lists/creates/updates/deletes profile knowledge bases and manages their
// entries and file builds. Except for list, name is always an exact existing
// knowledge base name; create is the only operation that may create a base.
func Manage(input map[string]any, opts ...Option) (map[string]any, error) {
	var req manageRequest
	if err := decode(input, &req); err != nil {
		return nil, err
	}
	c, err := newConfig(opts)
	if err != nil {
		return nil, err
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 1 || req.Limit > 100 || req.Offset < 0 {
		return nil, fmt.Errorf("limit must be 1..100 and offset non-negative")
	}
	if req.Op == "list" {
		query := c.db.Model(&schema.KnowledgeBaseInfo{})
		if req.Keyword != "" {
			query = query.Where("instr(knowledge_base_name, ?) > 0 OR instr(knowledge_base_description, ?) > 0", req.Keyword, req.Keyword)
		}
		var total int
		if err := query.Count(&total).Error; err != nil {
			return nil, err
		}
		rows := []schema.KnowledgeBaseInfo{}
		if err := query.Order("id ASC").Limit(req.Limit).Offset(req.Offset).Find(&rows).Error; err != nil {
			return nil, err
		}
		bases := make([]baseInfo, 0, len(rows))
		for i := range rows {
			bases = append(bases, info(&rows[i]))
		}
		next := req.Offset + len(rows)
		return map[string]any{"op": req.Op, "knowledge_bases": bases, "total": total, "has_more": next < total, "next_offset": next}, nil
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required (exact knowledge base name)")
	}
	result := map[string]any{"op": req.Op, "name": req.Name}
	if req.Op == "create" {
		colOpts, err := req.IndexOptions.options()
		if err != nil {
			return nil, err
		}
		if c.embedding != nil {
			colOpts = append(colOpts, vectorstore.WithEmbeddingClient(c.embedding))
		}
		description, typ := "", "default"
		if req.Metadata.Description != nil {
			description = *req.Metadata.Description
		}
		if req.Metadata.Type != nil {
			typ = *req.Metadata.Type
		}
		kb, err := knowledgebase.CreateKnowledgeBase(c.db, req.Name, description, typ, colOpts...)
		if err != nil {
			return nil, err
		}
		// Register the same RAG ID and entity repository as Yakit-created bases.
		// Reuse the configured store so loading cannot replace its model settings.
		if _, err := rag.Get(req.Name, rag.WithDB(c.db), rag.WithRAGCtx(c.ctx), rag.WithVectorStore(kb.GetVectorStore()), rag.WithLazyLoadEmbeddingClient(true), rag.WithDescription(description), rag.WithKnowledgeBaseType(typ)); err != nil {
			return nil, err
		}
		if err := c.updateMetadata(kb.GetID(), req.Metadata); err != nil {
			return nil, err
		}
		base, err := yakit.GetKnowledgeBase(c.db, kb.GetID())
		if err != nil {
			return nil, err
		}
		result["knowledge_base"] = info(base)
		return result, nil
	}
	if req.IndexOptions != (indexOptions{}) {
		return nil, fmt.Errorf("index_options apply only to create")
	}
	base, err := yakit.GetKnowledgeBaseByName(c.db, req.Name)
	if err != nil {
		return nil, fmt.Errorf("knowledge base %q: %w", req.Name, err)
	}
	switch req.Op {
	case "get":
		result["knowledge_base"] = info(base)
		var count int
		if err := c.db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", base.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		result["entry_count"] = count
		collection, err := vectorstore.GetCollectionInfo(c.db, req.Name)
		if err != nil {
			return nil, err
		}
		result["collection"] = collection
	case "update":
		if req.Metadata == (metadata{}) {
			return nil, fmt.Errorf("update requires metadata fields")
		}
		if err := c.updateMetadata(int64(base.ID), req.Metadata); err != nil {
			return nil, err
		}
		updated, err := yakit.GetKnowledgeBase(c.db, int64(base.ID))
		if err != nil {
			return nil, err
		}
		result["knowledge_base"] = info(updated)
	case "delete":
		if err := rag.DeleteRAG(c.db, req.Name); err != nil {
			return nil, err
		}
		result["deleted"] = true
	case "add_entry", "update_entry", "delete_entry":
		return c.manageEntry(req, base)
	case "build":
		return c.build(req, base)
	case "rebuild_index":
		return c.rebuild(req, base)
	default:
		return nil, fmt.Errorf("unknown knowledge base operation %q", req.Op)
	}
	return result, c.ctx.Err()
}

func (c *config) updateMetadata(id int64, meta metadata) error {
	return utils.GormTransaction(c.db, func(tx *gorm.DB) error {
		changes := map[string]any{}
		if meta.Description != nil {
			changes["knowledge_base_description"] = *meta.Description
		}
		if meta.Type != nil {
			changes["knowledge_base_type"] = *meta.Type
		}
		if meta.Tags != nil {
			changes["tags"] = strings.Join(*meta.Tags, ",")
		}
		if meta.IsDefault != nil {
			if *meta.IsDefault {
				if err := tx.Model(&schema.KnowledgeBaseInfo{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
					return err
				}
			}
			changes["is_default"] = *meta.IsDefault
		}
		if len(changes) == 0 {
			return nil
		}
		return tx.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", id).Updates(changes).Error
	})
}
