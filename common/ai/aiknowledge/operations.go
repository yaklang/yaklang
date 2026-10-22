package aiknowledge

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// List returns a bounded page of knowledge base metadata, without reading content.
func List(opts ...Option) (map[string]any, error) {
	c, err := newConfig(opts)
	if err != nil {
		return nil, err
	}
	r := c.manage
	if r.Limit == 0 {
		r.Limit = 20
	}
	if r.Limit < 1 || r.Limit > 100 || r.Offset < 0 {
		return nil, invalidArgumentf("limit must be 1..100 and offset non-negative")
	}
	query := c.db.Model(&schema.KnowledgeBaseInfo{})
	if r.Keyword != "" {
		query = query.Where("instr(knowledge_base_name, ?) > 0 OR instr(knowledge_base_description, ?) > 0", r.Keyword, r.Keyword)
	}
	var total int
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := []schema.KnowledgeBaseInfo{}
	if err := query.Order("id ASC").Limit(r.Limit).Offset(r.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	bases := make([]baseInfo, 0, len(rows))
	for i := range rows {
		bases = append(bases, info(&rows[i]))
	}
	next := r.Offset + len(rows)
	return map[string]any{"op": "list", "knowledge_bases": bases, "total": total, "has_more": next < total, "next_offset": next}, c.ctx.Err()
}

// Add creates a new exact name. Metadata and index settings are optional;
// creating an existing name fails and does not contact the embedding provider.
func Add(name string, opts ...Option) (map[string]any, error) {
	if strings.TrimSpace(name) == "" {
		return nil, invalidArgumentf("name is required (exact knowledge base name)")
	}
	c, err := newConfig(opts)
	if err != nil {
		return nil, err
	}
	r := c.manage
	colOpts, err := r.IndexOptions.options()
	if err != nil {
		return nil, withErrorCode("INVALID_ARGUMENT", err)
	}
	if c.embedding != nil {
		colOpts = append(colOpts, vectorstore.WithEmbeddingClient(c.embedding))
	}
	description, kind := "", "default"
	if r.Metadata.Description != nil {
		description = *r.Metadata.Description
	}
	if r.Metadata.Type != nil {
		kind = *r.Metadata.Type
	}
	kb, err := knowledgebase.CreateKnowledgeBase(c.db, name, description, kind, colOpts...)
	if err != nil {
		return nil, err
	}
	if _, err := rag.Get(name, rag.WithDB(c.db), rag.WithRAGCtx(c.ctx), rag.WithVectorStore(kb.GetVectorStore()), rag.WithLazyLoadEmbeddingClient(true), rag.WithDescription(description), rag.WithKnowledgeBaseType(kind)); err != nil {
		return nil, err
	}
	if err := c.updateMetadata(kb.GetID(), r.Metadata); err != nil {
		return nil, err
	}
	base, err := yakit.GetKnowledgeBase(c.db, kb.GetID())
	if err != nil {
		return nil, err
	}
	return map[string]any{"op": "create", "name": name, "knowledge_base": info(base)}, c.ctx.Err()
}

func namedConfig(name string, opts []Option) (*config, *schema.KnowledgeBaseInfo, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil, invalidArgumentf("name is required (exact knowledge base name)")
	}
	c, err := newConfig(opts)
	if err != nil {
		return nil, nil, err
	}
	if c.manage.IndexOptions != (indexOptions{}) {
		return nil, nil, invalidArgumentf("index_options apply only to create")
	}
	base, err := yakit.GetKnowledgeBaseByName(c.db, name)
	if err != nil {
		return nil, nil, fmt.Errorf("knowledge base %q: %w", name, err)
	}
	c.manage.Name = name
	return c, base, nil
}

// Get returns metadata, entry count and stored index configuration for one base.
func Get(name string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	var count int
	if err := c.db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", base.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	collection, err := vectorstore.GetCollectionInfo(c.db, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"op": "get", "name": name, "knowledge_base": info(base), "entry_count": count, "collection": collection}, c.ctx.Err()
}

// Update applies only supplied metadata options, including empty values/false.
func Update(name string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	if c.manage.Metadata == (metadata{}) {
		return nil, invalidArgumentf("update requires metadata fields")
	}
	if err := c.updateMetadata(int64(base.ID), c.manage.Metadata); err != nil {
		return nil, err
	}
	updated, err := yakit.GetKnowledgeBase(c.db, int64(base.ID))
	if err != nil {
		return nil, err
	}
	return map[string]any{"op": "update", "name": name, "knowledge_base": info(updated)}, c.ctx.Err()
}

// Delete removes a named base and its associated entries, vectors and entities.
func Delete(name string, opts ...Option) (map[string]any, error) {
	c, _, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	if err := rag.DeleteRAG(c.db, name); err != nil {
		return nil, err
	}
	return map[string]any{"op": "delete", "name": name, "deleted": true}, c.ctx.Err()
}

// AddEntry adds and, by default, indexes one complete entry.
func AddEntry(name, title, content string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.entry().Title, c.entry().Content = title, content
	c.manage.Op = "add_entry"
	return c.manageEntry(c.manage, base)
}

// UpdateEntry replaces the exact entry in this base, preserving its stable ID.
func UpdateEntry(name, id, title, content string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.entry().Title, c.entry().Content = title, content
	c.manage.Op, c.manage.EntryID = "update_entry", id
	return c.manageEntry(c.manage, base)
}

func DeleteEntry(name, id string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.manage.Op, c.manage.EntryID = "delete_entry", id
	return c.manageEntry(c.manage, base)
}

// Build appends knowledge built from content to an existing exact base.
// buildMode("question_index") uses AI; buildMode("chunks") splits directly.
func Build(name, content string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.manage.Op, c.manage.Content, c.manage.SourceFile = "build", content, ""
	return c.build(c.manage, base)
}

// BuildFromFile builds from an engine-local regular file and waits for writes.
func BuildFromFile(name, path string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.manage.Op, c.manage.SourceFile, c.manage.Content = "build", path, ""
	return c.build(c.manage, base)
}

func RebuildIndex(name string, opts ...Option) (map[string]any, error) {
	c, base, err := namedConfig(name, opts)
	if err != nil {
		return nil, err
	}
	c.manage.Op = "rebuild_index"
	return c.rebuild(c.manage, base)
}
