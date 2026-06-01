package knowledgebench

import (
	"fmt"
	"time"
)

// SearchProfile defines RAG search parameters for one experiment run.
type SearchProfile struct {
	ID                     string   `json:"id"`
	EnhancePlans           []string `json:"enhance_plans"`
	Limit                  int      `json:"limit"`
	SimilarityThreshold    float64  `json:"similarity_threshold"`
	CollectionScoreLimit   float64  `json:"collection_score_limit"`
}

// CompressProfile defines compression parameters for one experiment run.
type CompressProfile struct {
	ID                  string  `json:"id"`
	Enabled             bool    `json:"enabled"`
	MaxChunkSizeBytes   int     `json:"max_chunk_size_bytes"`
	MaxChunks           int     `json:"max_chunks"`
	TargetTokenSize     int64   `json:"target_token_size"`
	ScoreThreshold      float64 `json:"score_threshold"`
}

// RerankProfile defines rerank strategy for one experiment run.
type RerankProfile struct {
	ID           string `json:"id"`
	Strategy     string `json:"strategy"` // "rrf_only", "rrf_llm_rerank", "rrf_llm_rerank_compress_top3"
	RerankTopN   int    `json:"rerank_top_n"`
	FinalTopK    int    `json:"final_top_k"`
}

// IndexProfile defines indexing parameters (used for rebuild experiments).
type IndexProfile struct {
	ID                   string `json:"id"`
	Fields               string `json:"fields"` // "title_summary_details", "title_summary", "title_keywords_summary"
	MaxChunkSize         int    `json:"max_chunk_size"`
	Overlap              int    `json:"overlap"`
	EnableQuestionIndex  bool   `json:"enable_question_index"`
}

// RunConfig combines profiles for a single experiment run.
type RunConfig struct {
	RunID    string          `json:"run_id"`
	Search   SearchProfile   `json:"search"`
	Compress CompressProfile `json:"compress"`
	Rerank   RerankProfile   `json:"rerank"`
	Index    IndexProfile    `json:"index"`
}

// RunMetrics records the measured results for one run.
type RunMetrics struct {
	RunID string `json:"run_id"`

	// quality
	RecallAt5      float64 `json:"recall_at_5"`
	RecallAt10     float64 `json:"recall_at_10"`
	MRR            float64 `json:"mrr"`

	// efficiency
	TotalLatencyMs   int64 `json:"total_latency_ms"`
	SearchLatencyMs  int64 `json:"search_latency_ms"`
	CompressLatencyMs int64 `json:"compress_latency_ms"`
	RerankLatencyMs  int64 `json:"rerank_latency_ms"`
	AICallCount      int   `json:"ai_call_count"`

	// token usage
	RawResultBytes       int   `json:"raw_result_bytes"`
	CompressedResultBytes int  `json:"compressed_result_bytes"`
	FinalTokenCount      int   `json:"final_token_count"`

	// per-query details
	QueryResults []*QueryResult `json:"query_results"`

	Timestamp time.Time `json:"timestamp"`
}

// QueryResult holds per-query metrics.
type QueryResult struct {
	QueryID          string   `json:"query_id"`
	Query            string   `json:"query"`
	Mode             string   `json:"mode"`
	HitEntryIDs      []string `json:"hit_entry_ids"`
	ExpectedEntryIDs []string `json:"expected_entry_ids"`
	RecallAt5        float64  `json:"recall_at_5"`
	RecallAt10       float64  `json:"recall_at_10"`
	FirstHitRank     int      `json:"first_hit_rank"` // 0 = not found
	LatencyMs        int64    `json:"latency_ms"`
	AICallCount      int      `json:"ai_call_count"`
	RawBytes         int      `json:"raw_bytes"`
	CompressedBytes  int      `json:"compressed_bytes"`
	RawTexts         []string `json:"-"` // not persisted, used for compress input
}

// --- Predefined search profiles (E0-E4) ---

var SearchProfiles = map[string]SearchProfile{
	"E0": {ID: "E0", EnhancePlans: nil, Limit: 10, SimilarityThreshold: 0, CollectionScoreLimit: 0.3},
	"E1": {ID: "E1", EnhancePlans: []string{"hypothetical_answer"}, Limit: 10, SimilarityThreshold: 0, CollectionScoreLimit: 0.3},
	"E2": {ID: "E2", EnhancePlans: []string{"hypothetical_answer", "exact_keyword_search"}, Limit: 10, SimilarityThreshold: 0, CollectionScoreLimit: 0.3},
	"E3": {ID: "E3", EnhancePlans: []string{"hypothetical_answer", "generalize_query", "split_query"}, Limit: 10, SimilarityThreshold: 0, CollectionScoreLimit: 0.3},
	"E4": {ID: "E4", EnhancePlans: []string{"hypothetical_answer", "generalize_query", "split_query", "exact_keyword_search"}, Limit: 10, SimilarityThreshold: 0, CollectionScoreLimit: 0.3},
}

// --- Predefined compress profiles (C0-C5) ---

var CompressProfiles = map[string]CompressProfile{
	"C0": {ID: "C0", Enabled: false},
	"C1": {ID: "C1", Enabled: true, MaxChunkSizeBytes: 80 * 1024, MaxChunks: 20, TargetTokenSize: 10 * 1024, ScoreThreshold: 0.3},
	"C2": {ID: "C2", Enabled: true, MaxChunkSizeBytes: 40 * 1024, MaxChunks: 10, TargetTokenSize: 10 * 1024, ScoreThreshold: 0.3},
	"C3": {ID: "C3", Enabled: true, MaxChunkSizeBytes: 80 * 1024, MaxChunks: 20, TargetTokenSize: 6 * 1024, ScoreThreshold: 0.3},
	"C4": {ID: "C4", Enabled: true, MaxChunkSizeBytes: 80 * 1024, MaxChunks: 20, TargetTokenSize: 10 * 1024, ScoreThreshold: 0.4},
	"C5": {ID: "C5", Enabled: true, MaxChunkSizeBytes: 80 * 1024, MaxChunks: 5, TargetTokenSize: 10 * 1024, ScoreThreshold: 0.3},
}

// --- Predefined rerank profiles (R0-R2) ---

var RerankProfiles = map[string]RerankProfile{
	"R0": {ID: "R0", Strategy: "rrf_only", RerankTopN: 0, FinalTopK: 10},
	"R1": {ID: "R1", Strategy: "rrf_llm_rerank", RerankTopN: 15, FinalTopK: 10},
	"R2": {ID: "R2", Strategy: "rrf_llm_rerank_compress_top3", RerankTopN: 15, FinalTopK: 10},
}

// --- Predefined index profiles (I0-I3) ---

var IndexProfiles = map[string]IndexProfile{
	"I0": {ID: "I0", Fields: "title_summary_details", MaxChunkSize: 800, Overlap: 100, EnableQuestionIndex: false},
	"I1": {ID: "I1", Fields: "title_summary", MaxChunkSize: 800, Overlap: 100, EnableQuestionIndex: false},
	"I2": {ID: "I2", Fields: "title_keywords_summary", MaxChunkSize: 800, Overlap: 100, EnableQuestionIndex: false},
	"I3": {ID: "I3", Fields: "title_summary_details", MaxChunkSize: 800, Overlap: 100, EnableQuestionIndex: true},
}

// BuildRunID generates a unique run ID from profiles.
func BuildRunID(search SearchProfile, compress CompressProfile, rerank RerankProfile) string {
	return fmt.Sprintf("%s_%s_%s_L%d_T%.1f",
		search.ID, compress.ID, rerank.ID,
		search.Limit, search.SimilarityThreshold)
}
