package aiknowledge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type entryInput struct {
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	Summary            string   `json:"summary"`
	Type               string   `json:"type"`
	Keywords           []string `json:"keywords"`
	PotentialQuestions []string `json:"potential_questions"`
}

func (e *entryInput) entry(baseID int64) (*schema.KnowledgeBaseEntry, error) {
	if e == nil || strings.TrimSpace(e.Title) == "" || strings.TrimSpace(e.Content) == "" {
		return nil, invalidArgumentf("entry requires non-empty title and content")
	}
	return &schema.KnowledgeBaseEntry{KnowledgeBaseID: baseID, KnowledgeTitle: e.Title, KnowledgeDetails: e.Content, Summary: e.Summary, KnowledgeType: e.Type, Keywords: e.Keywords, PotentialQuestions: e.PotentialQuestions}, nil
}

func (c *config) manageEntry(req manageRequest, base *schema.KnowledgeBaseInfo) (map[string]any, error) {
	if req.Op != "add_entry" && req.EntryID == "" {
		return nil, invalidArgumentf("entry_id is required")
	}
	var old schema.KnowledgeBaseEntry
	if req.Op != "add_entry" {
		if err := c.db.Where("knowledge_base_id = ? AND hidden_index = ?", base.ID, req.EntryID).First(&old).Error; err != nil {
			return nil, fmt.Errorf("entry does not exist in knowledge base %q: %w", req.Name, err)
		}
	}
	var entry *schema.KnowledgeBaseEntry
	if req.Op != "delete_entry" {
		var err error
		entry, err = req.Entry.entry(int64(base.ID))
		if err != nil {
			return nil, err
		}
	}
	kb, err := c.load(req.Name)
	if err != nil {
		return nil, err
	}
	indexed := req.Index == nil || *req.Index
	if indexed && req.Op != "delete_entry" {
		if err := requireEmbedding(kb.GetVectorStore()); err != nil {
			return nil, err
		}
	}
	switch req.Op {
	case "delete_entry":
		err = removeStaleDocuments(c, kb.GetVectorStore(), &old, false)
		if err == nil {
			err = c.db.Where("knowledge_base_id = ? AND hidden_index = ?", base.ID, req.EntryID).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error
		}
	case "add_entry":
		entry.HiddenIndex = uuid.NewString()
		err = c.db.Create(entry).Error
		if err == nil && indexed {
			err = kb.GetVectorStore().Add(entryDocuments(entry)...)
			if err != nil {
				_ = c.db.Unscoped().Delete(entry).Error
			}
		}
		req.EntryID = entry.HiddenIndex
	case "update_entry":
		entry.Model, entry.HiddenIndex = old.Model, old.HiddenIndex
		// Upsert vectors before replacing the row or deleting old indexes. A
		// provider failure leaves the old content and vectors available.
		if indexed {
			err = kb.GetVectorStore().Add(entryDocuments(entry)...)
		}
		// Save all replacement fields, including empty summary/keyword arrays.
		if err == nil {
			err = c.db.Save(entry).Error
		}
		if err == nil {
			err = removeStaleDocuments(c, kb.GetVectorStore(), entry, indexed)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"op": req.Op, "name": req.Name, "entry_id": req.EntryID, "indexed": indexed && req.Op != "delete_entry", "deleted": req.Op == "delete_entry"}, nil
}

func requireEmbedding(store *vectorstore.SQLiteVectorStoreHNSW) error {
	client := store.GetEmbedder()
	if client == nil {
		return fmt.Errorf("embedding service is unavailable")
	}
	if model, ok := client.(interface{ GetModelDimension() int }); ok && model.GetModelDimension() > 0 && model.GetModelDimension() != store.GetCollectionInfo().Dimension {
		return invalidArgumentf("collection dimension %d does not match embedding service dimension %d", store.GetCollectionInfo().Dimension, model.GetModelDimension())
	}
	return nil
}

func entryDocuments(entry *schema.KnowledgeBaseEntry) []*vectorstore.Document {
	meta := map[string]any{schema.META_Data_UUID: entry.HiddenIndex, schema.META_Data_Title: entry.KnowledgeTitle, "knowledge_base_id": entry.KnowledgeBaseID}
	docs := []*vectorstore.Document{{ID: entry.HiddenIndex, Content: strings.Join([]string{entry.KnowledgeTitle, entry.Summary, entry.KnowledgeDetails}, "\n\n"), Type: schema.RAGDocumentType_Knowledge, Metadata: meta}}
	for _, question := range entry.PotentialQuestions {
		if strings.TrimSpace(question) == "" {
			continue
		}
		docs = append(docs, &vectorstore.Document{ID: entry.HiddenIndex + "_question_" + utils.CalcSha1(question), Content: question, Type: schema.RAGDocumentType_QuestionIndex, Metadata: meta})
	}
	return docs
}

func removeStaleDocuments(c *config, store *vectorstore.SQLiteVectorStoreHNSW, entry *schema.KnowledgeBaseEntry, indexed bool) error {
	var documents []schema.VectorStoreDocument
	err := c.db.Model(&schema.VectorStoreDocument{}).Select("document_id, metadata").Where("collection_id = ?", store.GetCollectionInfo().ID).
		Where("document_id IN (?) OR instr(document_id, ?) = 1 OR instr(document_id, ?) = 1", []string{entry.HiddenIndex, fmt.Sprint(entry.ID)}, entry.HiddenIndex+"_question_", entry.HiddenIndex+"_chunk_").Find(&documents).Error
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	if indexed {
		for _, doc := range entryDocuments(entry) {
			keep[doc.ID] = true
		}
	}
	// The embedding backend may split one upsert into _chunk_N documents.
	// Chunk zero carries the new batch size; keep that batch and discard old
	// trailing chunks. A new unsplit document replaces all its old chunks.
	present, chunks := map[string]bool{}, map[string]int{}
	for _, doc := range documents {
		present[doc.DocumentID] = true
	}
	for _, doc := range documents {
		original := utils.InterfaceToString(doc.Metadata["original_doc_id"])
		if keep[original] && !present[original] && utils.InterfaceToBoolean(doc.Metadata["is_chunk"]) && utils.InterfaceToInt(doc.Metadata["chunk_index"]) == 0 {
			chunks[original] = utils.InterfaceToInt(doc.Metadata["total_chunks"])
		}
	}
	stale := []string{}
	for _, doc := range documents {
		if linked := utils.InterfaceToString(doc.Metadata[schema.META_Data_UUID]); linked != "" && linked != entry.HiddenIndex {
			continue
		}
		original := utils.InterfaceToString(doc.Metadata["original_doc_id"])
		keepChunk := utils.InterfaceToBoolean(doc.Metadata["is_chunk"]) && utils.InterfaceToInt(doc.Metadata["chunk_index"]) < chunks[original]
		if !keep[doc.DocumentID] && !keepChunk {
			stale = append(stale, doc.DocumentID)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	return store.Delete(stale...)
}

type buildOptions struct {
	Mode               string  `json:"mode"`
	ChunkSize          int     `json:"chunk_size"`
	Concurrency        int     `json:"concurrency"`
	ExtraPrompt        string  `json:"extra_prompt"`
	OmniMode           string  `json:"omni_mode"`
	OmniModel          string  `json:"omni_model"`
	OmniSegmentSeconds float64 `json:"omni_segment_seconds"`
	OmniPayloadBytes   int64   `json:"omni_payload_bytes"`
}

// Build deliberately waits for persistence after AI analysis. The legacy
// channel builders can publish entries before writes and only log write errors.
func (c *config) build(req manageRequest, base *schema.KnowledgeBaseInfo) (result map[string]any, buildErr error) {
	if (req.SourceFile == "") == (strings.TrimSpace(req.Content) == "") {
		return nil, invalidArgumentf("build requires exactly one of source_file or content")
	}
	if req.IndexOptions != (indexOptions{}) {
		return nil, invalidArgumentf("index_options apply only to create")
	}
	b := req.BuildOptions
	if b.Mode == "" {
		b.Mode = "question_index"
	}
	if b.Mode != "chunks" && b.Mode != "question_index" {
		return nil, invalidArgumentf("build mode must be chunks or question_index")
	}
	if b.ChunkSize == 0 {
		b.ChunkSize = 8192
	}
	if b.Concurrency == 0 {
		b.Concurrency = 2
	}
	if b.ChunkSize < 256 || b.ChunkSize > 1048576 || b.Concurrency < 1 || b.Concurrency > 16 {
		return nil, invalidArgumentf("chunk_size must be 256..1048576 bytes and concurrency 1..16")
	}
	if b.OmniMode == "" {
		b.OmniMode = "auto"
	}
	if b.OmniMode != "auto" && b.OmniMode != "required" && b.OmniMode != "disabled" {
		return nil, invalidArgumentf("omni_mode must be auto, required or disabled")
	}
	if b.OmniSegmentSeconds == 0 {
		b.OmniSegmentSeconds = 20
	}
	if b.OmniPayloadBytes == 0 {
		b.OmniPayloadBytes = 7 * 1024 * 1024
	}
	if b.OmniSegmentSeconds <= 0 || b.OmniSegmentSeconds > 600 || b.OmniPayloadBytes < 1024 || b.OmniPayloadBytes > 10*1024*1024 {
		return nil, invalidArgumentf("omni_segment_seconds must be (0,600] and omni_payload_bytes 1024..10485760")
	}
	if req.SourceFile != "" {
		stat, err := os.Stat(req.SourceFile)
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() {
			return nil, invalidArgumentf("source_file must be a regular file")
		}
	}
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	progress := newBuildProgress(ctx, req.Name, c.progress, 5*time.Second)
	defer func() { progress.finish(buildErr) }()
	progress.event("initializing embedding; mode=%s omni_mode=%s segment_seconds=%g", b.Mode, b.OmniMode, b.OmniSegmentSeconds)
	kb, err := c.load(req.Name)
	if err != nil {
		return nil, err
	}
	if err := requireEmbedding(kb.GetVectorStore()); err != nil {
		return nil, err
	}
	progress.event("embedding ready; starting source analysis")
	var mu sync.Mutex
	var firstErr error
	opts := []any{liteforgeapp.WithAnalyzeContext(ctx), liteforgeapp.WithAnalyzeConcurrency(b.Concurrency), chunkmaker.WithChunkSize(int64(b.ChunkSize)), liteforgeapp.WithExtraPrompt(b.ExtraPrompt),
		liteforgeapp.AnalysisOption(func(cfg *liteforgeapp.AnalysisConfig) { cfg.AnalyzeLog = progress.event }),
		liteforgeapp.WithOmniMediaMode(b.OmniMode), liteforgeapp.WithVideoOmniSegmentSeconds(b.OmniSegmentSeconds), liteforgeapp.WithVideoOmniMaxBase64Bytes(b.OmniPayloadBytes),
		liteforgeapp.WithAnalyzeErrorHandler(func(err error) {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			cancel()
		}),
	}
	if b.OmniModel != "" {
		opts = append(opts, liteforgeapp.WithVideoOmniModel(b.OmniModel))
	}
	if c.ai != nil {
		opts = append(opts, rag.WithAIService(c.ai))
	}
	var input <-chan liteforgeapp.AnalysisResult
	if req.SourceFile != "" {
		input, err = liteforgeapp.AnalyzeFile(req.SourceFile, opts...)
	} else {
		input, err = liteforgeapp.AnalyzeReader(strings.NewReader(req.Content), opts...)
	}
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	count := 0
	for i := 0; i < b.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case analyzed, ok := <-input:
					if !ok {
						return
					}
					chunkID := progress.analyzed.Add(1)
					progress.event("source chunk %d extracted", chunkID)
					entries := []*schema.KnowledgeBaseEntry{{KnowledgeTitle: req.Name, KnowledgeDetails: analyzed.Dump()}}
					var err error
					if b.Mode == "question_index" {
						progress.indexing.Add(1)
						progress.event("generating question index for chunk %d", chunkID)
						entries, err = liteforgeapp.BuildIndexFormAnalyzeResult(analyzed, opts...)
						progress.indexing.Add(-1)
						progress.event("question index finished for chunk %d; entries=%d error=%v", chunkID, len(entries), err)
					}
					if err == nil && len(entries) == 0 {
						err = fmt.Errorf("AI build produced no knowledge entries")
					}
					progress.writing.Add(1)
					progress.event("embedding and persisting chunk %d; entries=%d", chunkID, len(entries))
					for _, entry := range entries {
						if err != nil {
							break
						}
						entry.KnowledgeBaseID, entry.HiddenIndex = int64(base.ID), uuid.NewString()
						if b.Mode == "question_index" {
							if len(entry.PotentialQuestions) == 0 {
								err = fmt.Errorf("AI build produced no index questions")
								break
							}
							entry.HasQuestionIndex = true
							err = kb.AddKnowledgeEntryQuestion(entry, false)
						} else {
							err = kb.AddKnowledgeEntry(entry)
						}
						if err == nil {
							mu.Lock()
							count++
							mu.Unlock()
							progress.persisted.Add(1)
							progress.event("entry persisted for chunk %d", chunkID)
						}
					}
					progress.writing.Add(-1)
					if err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						mu.Unlock()
						cancel()
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, fmt.Errorf("build failed after %d persisted entries (partial writes may remain): %w", count, firstErr)
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("build produced no persisted entries")
	}
	return map[string]any{"op": req.Op, "name": req.Name, "mode": b.Mode, "entries_added": count, "chunk_size": b.ChunkSize, "concurrency": b.Concurrency, "omni_mode": b.OmniMode}, nil
}

func (c *config) rebuild(req manageRequest, base *schema.KnowledgeBaseInfo) (map[string]any, error) {
	kb, err := c.load(req.Name)
	if err != nil {
		return nil, err
	}
	store := kb.GetVectorStore()
	if err := requireEmbedding(store); err != nil {
		return nil, err
	}
	// Do not use the legacy rebuild helper: its document IDs are numeric SQL
	// row IDs rather than the stable hidden indexes consumed by knowledge APIs.
	if err := store.Clear(); err != nil {
		return nil, err
	}
	count, lastID := 0, uint(0)
	for {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		var entries []*schema.KnowledgeBaseEntry
		if err := c.db.Where("knowledge_base_id = ? AND id > ?", base.ID, lastID).Order("id ASC").Limit(100).Find(&entries).Error; err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			break
		}
		for _, entry := range entries {
			if err := c.ctx.Err(); err != nil {
				return nil, err
			}
			if err := store.Add(entryDocuments(entry)...); err != nil {
				return nil, fmt.Errorf("rebuild failed after %d entries; knowledge rows are preserved: %w", count, err)
			}
			count++
			lastID = entry.ID
		}
	}
	return map[string]any{"op": req.Op, "name": req.Name, "entries_indexed": count, "indexed": true}, nil
}
