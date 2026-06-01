package knowledgebench

import (
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
)

// KnowledgePipelineConfig centralizes all tunable parameters for the
// knowledge retrieval and compression pipeline. It replaces scattered
// package-level constants with a single injectable configuration.
//
// Usage:
//   cfg := DefaultKnowledgePipelineConfig()       // production defaults
//   cfg := FastKnowledgePipelineConfig()           // speed-optimized preset
//   cfg := HighRecallKnowledgePipelineConfig()     // quality-optimized preset
//
// After experiment results are in, the winning profile should be set as
// the new default here.
type KnowledgePipelineConfig struct {
	// --- Indexing ---
	IndexFields          string // "title_summary_details", "title_summary", "title_keywords_summary"
	IndexMaxChunkSize    int    // vectorstore chunk size for embeddings (default 800)
	IndexChunkOverlap    int    // chunk overlap (default 100)
	EnableQuestionIndex  bool   // generate question-based index entries

	// --- Search ---
	EnhancePlans         []string // e.g. ["hypothetical_answer", "exact_keyword_search"]
	SearchLimit          int      // max results per sub-query (default 10)
	SimilarityThreshold  float64  // minimum cosine similarity (0 = no filter)
	CollectionScoreLimit float64  // minimum collection-level score (default 0.3)

	// --- Rerank ---
	RerankStrategy       string   // "rrf_only", "rrf_llm_rerank"
	RerankTopN           int      // candidates to feed into LLM rerank (default 15)
	FinalTopK            int      // final result count after rerank (default 10)

	// --- Compression ---
	CompressEnabled      bool
	CompressMaxChunkSize int     // bytes per chunk (default 80KB)
	CompressMaxChunks    int     // max chunks to process (default 20)
	CompressTargetTokens int64   // target token budget (default 10K)
	CompressScoreThresh  float64 // minimum score to keep (default 0.3)

	// --- Loop (knowledge_enhance) ---
	MaxIterations        int     // search loop iterations (default 3)
	MaxSearchesPerLoop   int     // max searches before forced stop (default 5)
	UseSpeedModelForEval bool    // use SpeedPriority for evaluateNextMovements
}

// DefaultKnowledgePipelineConfig returns the current production defaults.
// These values match the hardcoded constants scattered across the codebase.
func DefaultKnowledgePipelineConfig() *KnowledgePipelineConfig {
	return &KnowledgePipelineConfig{
		IndexFields:          "title_summary_details",
		IndexMaxChunkSize:    800,
		IndexChunkOverlap:    100,
		EnableQuestionIndex:  false,

		EnhancePlans:         []string{"hypothetical_answer", "generalize_query", "split_query", "exact_keyword_search"},
		SearchLimit:          10,
		SimilarityThreshold:  0,
		CollectionScoreLimit: 0.3,

		RerankStrategy:       "rrf_only",
		RerankTopN:           0,
		FinalTopK:            10,

		CompressEnabled:      true,
		CompressMaxChunkSize: 80 * 1024,
		CompressMaxChunks:    20,
		CompressTargetTokens: 10 * 1024,
		CompressScoreThresh:  0.3,

		MaxIterations:        3,
		MaxSearchesPerLoop:   5,
		UseSpeedModelForEval: false,
	}
}

// FastKnowledgePipelineConfig returns a speed-optimized preset.
// Reduces AI calls by using fewer enhance plans and smaller compress chunks.
func FastKnowledgePipelineConfig() *KnowledgePipelineConfig {
	return &KnowledgePipelineConfig{
		IndexFields:          "title_summary_details",
		IndexMaxChunkSize:    800,
		IndexChunkOverlap:    100,
		EnableQuestionIndex:  false,

		EnhancePlans:         []string{"hypothetical_answer", "exact_keyword_search"},
		SearchLimit:          10,
		SimilarityThreshold:  0,
		CollectionScoreLimit: 0.3,

		RerankStrategy:       "rrf_only",
		RerankTopN:           0,
		FinalTopK:            10,

		CompressEnabled:      true,
		CompressMaxChunkSize: 40 * 1024,
		CompressMaxChunks:    5,
		CompressTargetTokens: 10 * 1024,
		CompressScoreThresh:  0.3,

		MaxIterations:        2,
		MaxSearchesPerLoop:   3,
		UseSpeedModelForEval: true,
	}
}

// HighRecallKnowledgePipelineConfig returns a quality-optimized preset.
// Uses all enhance plans and LLM rerank for maximum recall.
func HighRecallKnowledgePipelineConfig() *KnowledgePipelineConfig {
	return &KnowledgePipelineConfig{
		IndexFields:          "title_summary_details",
		IndexMaxChunkSize:    800,
		IndexChunkOverlap:    100,
		EnableQuestionIndex:  true,

		EnhancePlans:         []string{"hypothetical_answer", "generalize_query", "split_query", "exact_keyword_search"},
		SearchLimit:          15,
		SimilarityThreshold:  0,
		CollectionScoreLimit: 0.3,

		RerankStrategy:       "rrf_llm_rerank",
		RerankTopN:           15,
		FinalTopK:            10,

		CompressEnabled:      true,
		CompressMaxChunkSize: 80 * 1024,
		CompressMaxChunks:    20,
		CompressTargetTokens: 10 * 1024,
		CompressScoreThresh:  0.3,

		MaxIterations:        3,
		MaxSearchesPerLoop:   5,
		UseSpeedModelForEval: false,
	}
}

// ToSearchProfile converts the config to a bench SearchProfile.
func (c *KnowledgePipelineConfig) ToSearchProfile() SearchProfile {
	return SearchProfile{
		ID:                   "pipeline",
		EnhancePlans:         c.EnhancePlans,
		Limit:                c.SearchLimit,
		SimilarityThreshold:  c.SimilarityThreshold,
		CollectionScoreLimit: c.CollectionScoreLimit,
	}
}

// ToCompressProfile converts the config to a bench CompressProfile.
func (c *KnowledgePipelineConfig) ToCompressProfile() CompressProfile {
	return CompressProfile{
		ID:                "pipeline",
		Enabled:           c.CompressEnabled,
		MaxChunkSizeBytes: c.CompressMaxChunkSize,
		MaxChunks:         c.CompressMaxChunks,
		TargetTokenSize:   c.CompressTargetTokens,
		ScoreThreshold:    c.CompressScoreThresh,
	}
}

// ToCompressOptions converts to the bench CompressOptions format.
func (c *KnowledgePipelineConfig) ToCompressOptions() CompressOptions {
	return CompressOptions{
		MaxChunkSizeBytes: c.CompressMaxChunkSize,
		MaxChunks:         c.CompressMaxChunks,
		ScoreThreshold:    c.CompressScoreThresh,
		TargetTokenSize:   c.CompressTargetTokens,
	}
}

// ToRAGSearchOptions converts the search portion to rag.RAGSystemConfigOption slice.
func (c *KnowledgePipelineConfig) ToRAGSearchOptions() []rag.RAGSystemConfigOption {
	var opts []rag.RAGSystemConfigOption
	opts = append(opts, rag.WithRAGLimit(c.SearchLimit))
	if c.SimilarityThreshold > 0 {
		opts = append(opts, rag.WithRAGSimilarityThreshold(c.SimilarityThreshold))
	}
	if c.CollectionScoreLimit > 0 {
		opts = append(opts, rag.WithRAGCollectionScoreLimit(c.CollectionScoreLimit))
	}
	if len(c.EnhancePlans) > 0 {
		opts = append(opts, rag.WithRAGEnhance(c.EnhancePlans...))
	}
	return opts
}

// ToIndexOptions converts the index portion to vectorstore CollectionConfigFunc slice.
func (c *KnowledgePipelineConfig) ToIndexOptions() []vectorstore.CollectionConfigFunc {
	var opts []vectorstore.CollectionConfigFunc
	if c.IndexMaxChunkSize > 0 {
		opts = append(opts, vectorstore.WithMaxChunkSize(c.IndexMaxChunkSize))
	}
	return opts
}
