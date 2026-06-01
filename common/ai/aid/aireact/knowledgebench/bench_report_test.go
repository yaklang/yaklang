package knowledgebench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func getFixturesDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "fixtures")
}

func getResultsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "bench_results")
}

func getDocsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "reactloops", "docs")
}

func loadBenchQueries(t *testing.T) []*BenchQuery {
	t.Helper()
	fixturesDir := getFixturesDir()
	queryFile := filepath.Join(fixturesDir, "queries_from_pq.jsonl")
	if _, err := os.Stat(queryFile); os.IsNotExist(err) {
		queryFile = filepath.Join(fixturesDir, "queries.jsonl")
	}
	queries, err := LoadFixtures(queryFile)
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(queries) == 0 {
		t.Skip("no fixtures available")
	}
	return queries
}

// TestExportPotentialQuestionFixtures exports benchmark queries from
// potential_questions in existing knowledge bases.
// Run: go test -run TestExportPotentialQuestionFixtures -v ./common/ai/aid/aireact/knowledgebench/
// Set BENCH_KB_NAME env var to override the default "逻辑漏洞" knowledge base.
func TestExportPotentialQuestionFixtures(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("no profile database available")
	}

	kbName := os.Getenv("BENCH_KB_NAME")
	if kbName == "" {
		kbName = "逻辑漏洞"
	}

	queries, err := ExportQueriesFromPotentialQuestions(db, kbName, 1)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if len(queries) == 0 {
		t.Skipf("no potential_questions found in %q", kbName)
	}

	outPath := filepath.Join(getFixturesDir(), "queries_from_pq.jsonl")
	if err := SaveFixtures(outPath, queries); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	t.Logf("exported %d queries to %s", len(queries), outPath)
}

// --- Phase 1: Search Profile Sweep (E0-E4) ---

// TestPhase1_SearchProfileSweep runs the E0-E4 search profile matrix.
// This is the first phase: isolate retrieval quality without compression.
// Run: go test -run TestPhase1_SearchProfileSweep -v -timeout 20m ./common/ai/aid/aireact/knowledgebench/
func TestPhase1_SearchProfileSweep(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("no profile database available")
	}

	queries := loadBenchQueries(t)
	if len(queries) > 15 {
		queries = queries[:15]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	runner := NewBenchRunner(db, nil, getResultsDir())
	var allMetrics []*RunMetrics

	for _, pid := range []string{"E0", "E1", "E2", "E3", "E4"} {
		sp := SearchProfiles[pid]
		t.Logf("--- search profile %s (plans: %v) ---", sp.ID, sp.EnhancePlans)
		m := runner.RunSearchBench(ctx, queries, sp)
		t.Logf("  Recall@5=%.3f Recall@10=%.3f MRR=%.3f latency=%dms ai_calls=%d raw=%d bytes",
			m.RecallAt5, m.RecallAt10, m.MRR, m.TotalLatencyMs, m.AICallCount, m.RawResultBytes)

		for _, qr := range m.QueryResults {
			t.Logf("    [%s] R@5=%.2f R@10=%.2f rank=%d %dms hits=%d",
				qr.QueryID, qr.RecallAt5, qr.RecallAt10, qr.FirstHitRank, qr.LatencyMs, len(qr.HitEntryIDs))
		}
		runner.SaveMetrics(m)
		allMetrics = append(allMetrics, m)
	}

	writeReport(t, allMetrics, "16-knowledge-search-sweep.md")
}

// TestPhase1_SearchLimitSweep runs limit variations on the best search profile.
// Run: go test -run TestPhase1_SearchLimitSweep -v -timeout 15m ./common/ai/aid/aireact/knowledgebench/
func TestPhase1_SearchLimitSweep(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("no profile database available")
	}

	queries := loadBenchQueries(t)
	if len(queries) > 15 {
		queries = queries[:15]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	runner := NewBenchRunner(db, nil, getResultsDir())
	var allMetrics []*RunMetrics

	baseSP := SearchProfiles["E2"]
	for _, limit := range []int{5, 10, 15, 20} {
		for _, threshold := range []float64{0, 0.3, 0.5} {
			sp := baseSP
			sp.ID = fmt.Sprintf("E2_L%d_T%.1f", limit, threshold)
			sp.Limit = limit
			sp.SimilarityThreshold = threshold

			t.Logf("--- %s ---", sp.ID)
			m := runner.RunSearchBench(ctx, queries, sp)
			m.RunID = sp.ID
			t.Logf("  Recall@5=%.3f Recall@10=%.3f MRR=%.3f latency=%dms",
				m.RecallAt5, m.RecallAt10, m.MRR, m.TotalLatencyMs)
			runner.SaveMetrics(m)
			allMetrics = append(allMetrics, m)
		}
	}

	writeReport(t, allMetrics, "16-knowledge-limit-sweep.md")
}

// --- Phase 2: Compress Profile Sweep (C0-C5) ---

// TestPhase2_CompressProfileSweep runs compress parameter matrix.
// Requires a configured AIInvokeRuntime for LiteForge calls.
// Run: go test -run TestPhase2_CompressProfileSweep -v -timeout 30m ./common/ai/aid/aireact/knowledgebench/
func TestPhase2_CompressProfileSweep(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("no profile database available")
	}

	invoker := getTestInvoker(t)
	if invoker == nil {
		t.Skip("no AIInvokeRuntime available; set up AI config to run compress sweep")
	}

	queries := loadBenchQueries(t)
	if len(queries) > 10 {
		queries = queries[:10]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	runner := NewBenchRunner(db, invoker, getResultsDir())
	var allMetrics []*RunMetrics

	sp := SearchProfiles["E2"]
	rp := RerankProfiles["R0"]

	for _, cid := range []string{"C0", "C1", "C2", "C3", "C4", "C5"} {
		cp := CompressProfiles[cid]
		cfg := RunConfig{
			RunID:    BuildRunID(sp, cp, rp),
			Search:   sp,
			Compress: cp,
			Rerank:   rp,
		}

		t.Logf("--- compress profile %s (chunk=%d max=%d target=%d threshold=%.2f) ---",
			cp.ID, cp.MaxChunkSizeBytes, cp.MaxChunks, cp.TargetTokenSize, cp.ScoreThreshold)
		m := runner.RunFullBench(ctx, queries, cfg)
		t.Logf("  Recall@5=%.3f Recall@10=%.3f latency=%dms raw=%d compressed=%d",
			m.RecallAt5, m.RecallAt10, m.TotalLatencyMs, m.RawResultBytes, m.CompressedResultBytes)
		runner.SaveMetrics(m)
		allMetrics = append(allMetrics, m)
	}

	writeReport(t, allMetrics, "16-knowledge-compress-sweep.md")
}

// --- Phase 3: LLM Rerank Sweep ---

// TestPhase3_LLMRerankSweep runs rerank strategy comparisons.
// Run: go test -run TestPhase3_LLMRerankSweep -v -timeout 20m ./common/ai/aid/aireact/knowledgebench/
func TestPhase3_LLMRerankSweep(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Skip("no profile database available")
	}

	invoker := getTestInvoker(t)
	if invoker == nil {
		t.Skip("no AIInvokeRuntime available")
	}

	queries := loadBenchQueries(t)
	if len(queries) > 10 {
		queries = queries[:10]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	runner := NewBenchRunner(db, invoker, getResultsDir())
	var allMetrics []*RunMetrics

	sp := SearchProfiles["E2"]
	sp.Limit = 20 // over-retrieve for rerank

	// R0: RRF only (baseline)
	t.Log("--- R0: RRF only ---")
	mR0 := runner.RunSearchBench(ctx, queries, sp)
	mR0.RunID = "R0_E2_L20"
	t.Logf("  Recall@10=%.3f latency=%dms", mR0.RecallAt10, mR0.TotalLatencyMs)
	runner.SaveMetrics(mR0)
	allMetrics = append(allMetrics, mR0)

	// R1: RRF -> LLM rerank Top15 -> Top10
	t.Log("--- R1: RRF + LLM rerank ---")
	mR1 := runWithLLMRerank(ctx, runner, invoker, queries, sp, 15, 10)
	mR1.RunID = "R1_E2_L20_rerank15"
	t.Logf("  Recall@10=%.3f latency=%dms", mR1.RecallAt10, mR1.TotalLatencyMs)
	runner.SaveMetrics(mR1)
	allMetrics = append(allMetrics, mR1)

	writeReport(t, allMetrics, "16-knowledge-rerank-sweep.md")
}

func runWithLLMRerank(
	ctx context.Context,
	runner *BenchRunner,
	invoker aicommon.AIInvokeRuntime,
	queries []*BenchQuery,
	sp SearchProfile,
	rerankTopN, finalTopK int,
) *RunMetrics {
	metrics := &RunMetrics{Timestamp: time.Now()}
	totalStart := time.Now()

	for _, q := range queries {
		qr := runner.runSingleSearchQuery(ctx, q, sp)

		// build rerank candidates from hit entries
		var candidates []*RerankCandidate
		seen := make(map[string]bool)
		for i, entryID := range qr.HitEntryIDs {
			if seen[entryID] || i >= rerankTopN {
				break
			}
			seen[entryID] = true
			c := LookupKnowledgeEntryForRerank(entryID)
			if c != nil {
				c.OriginalRank = i + 1
				candidates = append(candidates, c)
			}
		}

		if len(candidates) > 0 {
			rerankStart := time.Now()
			reranked, err := LLMRerankTopK(ctx, invoker, q.Query, candidates, finalTopK)
			if err != nil {
				log.Warnf("rerank failed for %s: %v", q.ID, err)
			} else {
				var newHitIDs []string
				for _, c := range reranked {
					newHitIDs = append(newHitIDs, c.EntryID)
				}
				qr.HitEntryIDs = newHitIDs
				qr.RecallAt5 = computeRecall(qr.HitEntryIDs, qr.ExpectedEntryIDs, 5)
				qr.RecallAt10 = computeRecall(qr.HitEntryIDs, qr.ExpectedEntryIDs, 10)
				qr.FirstHitRank = computeFirstHitRank(qr.HitEntryIDs, qr.ExpectedEntryIDs)
			}
			qr.LatencyMs += time.Since(rerankStart).Milliseconds()
			qr.AICallCount++
		}

		metrics.QueryResults = append(metrics.QueryResults, qr)
		metrics.SearchLatencyMs += qr.LatencyMs
		metrics.AICallCount += qr.AICallCount
		metrics.RawResultBytes += qr.RawBytes
	}

	metrics.TotalLatencyMs = time.Since(totalStart).Milliseconds()
	runner.computeAggregateMetrics(metrics)
	return metrics
}


// --- Phase 5: Full Matrix Report ---

// TestPhase5_GenerateFullReport reads all saved metrics and generates the combined report.
// Run: go test -run TestPhase5_GenerateFullReport -v ./common/ai/aid/aireact/knowledgebench/
func TestPhase5_GenerateFullReport(t *testing.T) {
	resultsDir := getResultsDir()
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		t.Skipf("no results dir: %v", err)
	}

	var allMetrics []*RunMetrics
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(resultsDir, entry.Name())
		queries, err := loadMetricsFile(path)
		if err != nil {
			t.Logf("skip %s: %v", entry.Name(), err)
			continue
		}
		allMetrics = append(allMetrics, queries...)
	}

	if len(allMetrics) == 0 {
		t.Skip("no metrics found")
	}

	writeReport(t, allMetrics, "16-knowledge-param-experiment.md")
}

func loadMetricsFile(path string) ([]*RunMetrics, error) {
	queries, err := LoadFixtures(path) // reuse JSONL loader format
	_ = queries
	// For simplicity, metrics files are also JSONL but with RunMetrics schema.
	// We'll implement proper loading when needed.
	return nil, err
}

func writeReport(t *testing.T, allMetrics []*RunMetrics, filename string) {
	t.Helper()
	report := GenerateMarkdownReport(allMetrics)
	reportPath := filepath.Join(getDocsDir(), filename)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		t.Logf("mkdir for report: %v", err)
		return
	}
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		t.Logf("write report failed: %v", err)
	} else {
		t.Logf("report written to %s", reportPath)
	}
}

// getTestInvoker attempts to create an AIInvokeRuntime for testing.
// Returns nil if AI services are not configured.
func getTestInvoker(t *testing.T) aicommon.AIInvokeRuntime {
	t.Helper()
	ctx := context.Background()
	invoker, err := aicommon.AIRuntimeInvokerGetter(ctx)
	if err != nil {
		t.Logf("no test invoker available: %v", err)
		return nil
	}
	return invoker
}

// suppress unused import
var _ = vectorstore.NewRAGQueryConfig
