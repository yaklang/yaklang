// Package aiknowledge exposes the profile knowledge base through bounded,
// explicitly scoped operations shared by Yak scripts and AI tools.
package aiknowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type config struct {
	db        *gorm.DB
	ctx       context.Context
	embedding aispec.EmbeddingCaller
	ai        aicommon.AICallbackType
	progress  func(string)
	manage    manageRequest
	query     queryRequest
}

type Option func(*config)

func WithDatabase(db *gorm.DB) Option        { return func(c *config) { c.db = db } }
func WithContext(ctx context.Context) Option { return func(c *config) { c.ctx = ctx } }
func WithEmbeddingClient(client aispec.EmbeddingCaller) Option {
	return func(c *config) { c.embedding = client }
}
func WithAIService(ai aicommon.AICallbackType) Option { return func(c *config) { c.ai = ai } }

// WithProgress receives stage changes and a heartbeat every five seconds during
// a build, including counts of successfully persisted entries.
func WithProgress(handler func(string)) Option { return func(c *config) { c.progress = handler } }

func configure(opts []Option) *config {
	c := &config{ctx: context.Background()}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func newConfig(opts []Option) (*config, error) {
	c := configure(opts)
	return c, c.initialize()
}

func (c *config) initialize() error {
	if c.ctx == nil {
		return fmt.Errorf("knowledge base requires a profile database and context")
	}
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.db == nil {
		c.db = consts.GetGormProfileDatabase()
	}
	if c.db == nil {
		return fmt.Errorf("knowledge base requires a profile database")
	}
	return nil
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
		if err := tx.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", id).Updates(changes).Error; err != nil {
			return err
		}
		if meta.Description != nil {
			var base schema.KnowledgeBaseInfo
			if err := tx.Select("knowledge_base_name").Where("id = ?", id).First(&base).Error; err != nil {
				return err
			}
			return tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", base.KnowledgeBaseName).
				Update("description", *meta.Description).Error
		}
		return nil
	})
}
