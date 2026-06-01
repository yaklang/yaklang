package knowledgebench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// BenchRunner executes benchmark runs against a real RAG system.
type BenchRunner struct {
	db      *gorm.DB
	invoker aicommon.AIInvokeRuntime
	outDir  string
}

func NewBenchRunner(db *gorm.DB, invoker aicommon.AIInvokeRuntime, outDir string) *BenchRunner {
	return &BenchRunner{db: db, invoker: invoker, outDir: outDir}
}

// RunSearchBench runs search-only benchmark (no compress) to isolate retrieval quality.
func (b *BenchRunner) RunSearchBench(ctx context.Context, queries []*BenchQuery, sp SearchProfile) *RunMetrics {
	metrics := &RunMetrics{
		RunID:     sp.ID,
		Timestamp: time.Now(),
	}

	totalStart := time.Now()
	for _, q := range queries {
		qr := b.runSingleSearchQuery(ctx, q, sp)
		metrics.QueryResults = append(metrics.QueryResults, qr)
		metrics.SearchLatencyMs += qr.LatencyMs
		metrics.AICallCount += qr.AICallCount
		metrics.RawResultBytes += qr.RawBytes
	}
	metrics.TotalLatencyMs = time.Since(totalStart).Milliseconds()
	b.computeAggregateMetrics(metrics)
	return metrics
}

// RunFullBench runs search + compress benchmark.
func (b *BenchRunner) RunFullBench(ctx context.Context, queries []*BenchQuery, cfg RunConfig) *RunMetrics {
	metrics := &RunMetrics{
		RunID:     cfg.RunID,
		Timestamp: time.Now(),
	}

	totalStart := time.Now()
	for _, q := range queries {
		qr := b.runSingleFullQuery(ctx, q, cfg)
		metrics.QueryResults = append(metrics.QueryResults, qr)
		metrics.SearchLatencyMs += qr.LatencyMs
		metrics.AICallCount += qr.AICallCount
		metrics.RawResultBytes += qr.RawBytes
		metrics.CompressedResultBytes += qr.CompressedBytes
	}
	metrics.TotalLatencyMs = time.Since(totalStart).Milliseconds()
	b.computeAggregateMetrics(metrics)
	return metrics
}

func (b *BenchRunner) runSingleSearchQuery(ctx context.Context, q *BenchQuery, sp SearchProfile) *QueryResult {
	qr := &QueryResult{
		QueryID:          q.ID,
		Query:            q.Query,
		Mode:             q.Mode,
		ExpectedEntryIDs: q.Expected,
	}

	start := time.Now()

	opts := []rag.RAGSystemConfigOption{
		rag.WithRAGCtx(ctx),
		rag.WithRAGLimit(sp.Limit),
		rag.WithRAGCollectionNames(q.KB...),
	}
	if sp.SimilarityThreshold > 0 {
		opts = append(opts, rag.WithRAGSimilarityThreshold(sp.SimilarityThreshold))
	}
	if sp.CollectionScoreLimit > 0 {
		opts = append(opts, rag.WithRAGCollectionScoreLimit(sp.CollectionScoreLimit))
	}
	if len(sp.EnhancePlans) > 0 {
		opts = append(opts, rag.WithRAGEnhance(sp.EnhancePlans...))
	} else {
		opts = append(opts, rag.WithRAGEnhance("basic"))
	}

	var hitIDs []string
	var rawBytes int
	var rawTexts []string

	opts = append(opts, rag.WithEveryQueryResultCallback(func(result *vectorstore.ScoredResult) {
		if result == nil || result.Document == nil {
			return
		}
		entryUUID := result.GetKnowledgeEntryUUID()
		if entryUUID != "" {
			hitIDs = append(hitIDs, entryUUID)
		}
		content := result.GetContent()
		rawBytes += len(content)
		rawTexts = append(rawTexts, content)
	}))

	// discard log readers
	opts = append(opts, rag.WithRAGLogReader(func(reader io.Reader) {
		io.Copy(io.Discard, reader)
	}))

	var allResults []*vectorstore.ScoredResult
	opts = append(opts, rag.WithRAGOnQueryFinish(func(results []*vectorstore.ScoredResult) {
		allResults = results
	}))

	resultCh, err := rag.QueryYakitProfile(q.Query, opts...)
	if err != nil {
		log.Warnf("bench search query %q failed: %v", q.ID, err)
	} else {
		for range resultCh {
			// drain the channel
		}
	}

	qr.LatencyMs = time.Since(start).Milliseconds()
	qr.HitEntryIDs = dedup(hitIDs)
	qr.RawBytes = rawBytes
	qr.RawTexts = rawTexts
	qr.RecallAt5 = computeRecall(qr.HitEntryIDs, qr.ExpectedEntryIDs, 5)
	qr.RecallAt10 = computeRecall(qr.HitEntryIDs, qr.ExpectedEntryIDs, 10)
	qr.FirstHitRank = computeFirstHitRank(qr.HitEntryIDs, qr.ExpectedEntryIDs)

	// count AI calls from allResults unique methods (excluding "basic")
	methodsSeen := make(map[string]struct{})
	for _, r := range allResults {
		if r.QueryMethod != "" && r.QueryMethod != "basic" {
			methodsSeen[r.QueryMethod] = struct{}{}
		}
	}
	qr.AICallCount = len(methodsSeen)
	if len(sp.EnhancePlans) > 0 {
		qr.AICallCount = len(sp.EnhancePlans)
	}

	return qr
}

func (b *BenchRunner) runSingleFullQuery(ctx context.Context, q *BenchQuery, cfg RunConfig) *QueryResult {
	qr := b.runSingleSearchQuery(ctx, q, cfg.Search)

	if !cfg.Compress.Enabled || qr.RawBytes == 0 {
		return qr
	}

	compStart := time.Now()

	rawText := strings.Join(qr.RawTexts, "\n\n")
	if rawText == "" {
		return qr
	}

	compressed, err := b.invoker.CompressLongTextWithDestination(
		ctx, rawText, q.Query, cfg.Compress.TargetTokenSize,
	)
	if err != nil {
		log.Warnf("bench compress for query %q failed: %v", q.ID, err)
		return qr
	}

	qr.CompressedBytes = len(compressed)
	compLatency := time.Since(compStart).Milliseconds()
	qr.LatencyMs += compLatency

	return qr
}

func (b *BenchRunner) computeAggregateMetrics(m *RunMetrics) {
	if len(m.QueryResults) == 0 {
		return
	}

	var sumR5, sumR10, sumRR float64
	totalTokens := 0
	for _, qr := range m.QueryResults {
		sumR5 += qr.RecallAt5
		sumR10 += qr.RecallAt10
		if qr.FirstHitRank > 0 {
			sumRR += 1.0 / float64(qr.FirstHitRank)
		}
		if qr.CompressedBytes > 0 {
			totalTokens += ytoken.CalcTokenCount(strings.Repeat("x", qr.CompressedBytes))
		}
	}
	n := float64(len(m.QueryResults))
	m.RecallAt5 = sumR5 / n
	m.RecallAt10 = sumR10 / n
	m.MRR = sumRR / n
	m.FinalTokenCount = totalTokens
}

// SaveMetrics writes run metrics to a JSONL file.
func (b *BenchRunner) SaveMetrics(m *RunMetrics) error {
	if b.outDir == "" {
		return nil
	}
	if err := os.MkdirAll(b.outDir, 0755); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/metrics_%s.jsonl", b.outDir, m.RunID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(m)
	f.Write(data)
	f.WriteString("\n")
	return nil
}

// --- LLM Rerank ---

// LLMRerankTopK uses InvokeSpeedPriorityLiteForge to batch-score candidates.
// Each candidate is presented as title+summary+keywords (<=200 chars each).
func LLMRerankTopK(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	query string,
	candidates []*RerankCandidate,
	topK int,
) ([]*RerankCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if topK <= 0 || topK > len(candidates) {
		topK = len(candidates)
	}

	dNonce := utils.RandStringBytes(4)
	var candidateBlock strings.Builder
	for i, c := range candidates {
		summary := utils.ShrinkString(c.Summary, 200)
		keywords := utils.ShrinkString(strings.Join(c.Keywords, ", "), 100)
		candidateBlock.WriteString(fmt.Sprintf("[%d] id=%s title=%s\n", i+1, c.EntryID, c.Title))
		if summary != "" {
			candidateBlock.WriteString(fmt.Sprintf("    summary: %s\n", summary))
		}
		if keywords != "" {
			candidateBlock.WriteString(fmt.Sprintf("    keywords: %s\n", keywords))
		}
	}

	promptTemplate := `<|USER_QUERY_%s|>
%s
<|USER_QUERY_END_%s|>

<|CANDIDATES_%s|>
%s
<|CANDIDATES_END_%s|>

<|INSTRUCT_%s|>
Rate the relevance of each candidate to the user query.
Output a JSON array "scores" with objects {index, score} where score is 0.00-1.00.
Only include candidates with score >= 0.10.
Sort by score descending.
<|INSTRUCT_END_%s|>
`

	prompt := fmt.Sprintf(promptTemplate,
		dNonce, query, dNonce,
		dNonce, candidateBlock.String(), dNonce,
		dNonce, dNonce,
	)

	result, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"llm-rerank",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"scores",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("relevance scores sorted descending"),
				},
				nil,
				aitool.WithNumberParam("index", aitool.WithParam_Description("1-based candidate index")),
				aitool.WithNumberParam("score", aitool.WithParam_Description("relevance 0.00-1.00")),
			),
		},
	)
	if err != nil {
		return nil, utils.Errorf("LLM rerank failed: %v", err)
	}
	if result == nil {
		return candidates[:topK], nil
	}

	scoreItems := result.GetInvokeParamsArray("scores")

	type scored struct {
		idx   int
		score float64
	}
	var scoredList []scored
	for _, item := range scoreItems {
		idx := int(item.GetFloat("index")) - 1
		score := item.GetFloat("score")
		if idx >= 0 && idx < len(candidates) && score >= 0.10 {
			scoredList = append(scoredList, scored{idx: idx, score: score})
		}
	}

	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	var out []*RerankCandidate
	for i, s := range scoredList {
		if i >= topK {
			break
		}
		c := candidates[s.idx]
		c.RerankScore = s.score
		out = append(out, c)
	}

	if len(out) == 0 && len(candidates) > 0 {
		if topK > len(candidates) {
			topK = len(candidates)
		}
		return candidates[:topK], nil
	}

	return out, nil
}

// RerankCandidate holds a knowledge entry summary for LLM reranking.
type RerankCandidate struct {
	EntryID      string   `json:"entry_id"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Keywords     []string `json:"keywords"`
	Content      string   `json:"content"`
	OriginalRank int      `json:"original_rank"`
	RerankScore  float64  `json:"rerank_score"`
}

// --- Metric helpers ---

func computeRecall(hitIDs, expectedIDs []string, k int) float64 {
	if len(expectedIDs) == 0 {
		return 1.0
	}
	if k > len(hitIDs) {
		k = len(hitIDs)
	}
	topK := make(map[string]struct{}, k)
	for i := 0; i < k; i++ {
		topK[hitIDs[i]] = struct{}{}
	}
	hits := 0
	for _, eid := range expectedIDs {
		if _, ok := topK[eid]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(expectedIDs))
}

func computeFirstHitRank(hitIDs, expectedIDs []string) int {
	if len(expectedIDs) == 0 {
		return 0
	}
	expectedSet := make(map[string]struct{}, len(expectedIDs))
	for _, eid := range expectedIDs {
		expectedSet[eid] = struct{}{}
	}
	for i, hid := range hitIDs {
		if _, ok := expectedSet[hid]; ok {
			return i + 1
		}
	}
	return 0
}

func dedup(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
