package aireact

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

// Measure reusable prompt prefixes with deterministic native calls. This is
// a regression measure of prefix stability, not a provider cache-hit reading.
func TestPlanExec_PrefixCacheStableWithMockedTieredAI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const toolName = "mock_plan_exec_prefix_cache_tool"
	var calls atomic.Int32
	tool, err := aitool.New(toolName, aitool.WithSimpleCallback(func(_ aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
		stage := calls.Add(1)
		_, _ = io.WriteString(stdout, strings.Repeat("deterministic tool observation; session evidence and artifacts remain available.\n", 20))
		return map[string]any{"stage": stage, "summary": "Verified deterministic execution", "evidence": strings.Repeat("stable observation\n", 30)}, nil
	}))
	require.NoError(t, err)
	model := newNativePlanTestModel(toolName, 4)
	probe := newPlanExecPromptProbe()
	var mu sync.Mutex
	roleHashes := map[string]string{}
	ins, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(),
		aicommon.WithAgreeYOLO(), aicommon.WithTools(tool),
		aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected original callback: %s", req.GetCallerLabel())
		}),
		aicommon.WithQualityPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rec := probe.Observe(req.GetPrompt())
			role := "coordinator"
			if strings.Contains(req.GetPrompt(), "Execute the assigned frozen plan task.") {
				role = "pe_task"
			}
			mu.Lock()
			if previous, ok := roleHashes[role]; ok {
				require.Equal(t, previous, rec.HighStaticHash, "high-static changed within %s", role)
			}
			roleHashes[role] = rec.HighStaticHash
			mu.Unlock()
			rsp, handled, err := model(c, req, "mock-intelligent-planexec")
			require.True(t, handled, "coordinator and worker require native function calls")
			return rsp, err
		}),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected speed fallback: %s", req.GetCallerLabel())
		}))
	require.NoError(t, err)
	require.NoError(t, ins.PlanAndExecute(ctx, "Execute four dependent deterministic checks and review every result"))
	require.EqualValues(t, 4, calls.Load(), "each worker must execute its tool exactly once")
	records := probe.Records()
	diagnostics := formatPlanExecProbeDiagnostics(records)
	require.GreaterOrEqual(t, len(records), 20, diagnostics)
	for _, rec := range records {
		require.NotContains(t, rec.Sections, aiprojection.SectionRaw, diagnostics)
		require.Contains(t, rec.Sections, aiprojection.SectionHighStatic, diagnostics)
		require.Contains(t, rec.Sections, aiprojection.SectionDynamic, diagnostics)
		require.NotEmpty(t, rec.HighStaticHash, diagnostics)
	}
	total, hits := summarizePlanExecProbe(records)
	require.Positive(t, total)
	require.GreaterOrEqual(t, planExecRatio(hits, total), 0.30, diagnostics)
	require.Len(t, roleHashes, 2, "exercise both coordinator and worker prompts")
	t.Log(diagnostics)
}

type planExecPromptProbe struct {
	mu      sync.Mutex
	history []*planExecPromptHistory
	records []*planExecPromptRecord
}

type planExecPromptHistory struct {
	hashes   []string
	contents []string
}

type planExecPromptRecord struct {
	Seq               int
	TotalPromptTokens int
	HitPrefixTokens   int
	TokenHitRatio     float64
	PrefixHitChunks   int
	Sections          []string
	HighStaticHash    string
}

func newPlanExecPromptProbe() *planExecPromptProbe {
	return &planExecPromptProbe{}
}

func (p *planExecPromptProbe) Observe(prompt string) *planExecPromptRecord {
	split := aiprojection.Split(prompt)
	hashes := make([]string, 0, len(split.Chunks))
	contents := make([]string, 0, len(split.Chunks))
	sections := make([]string, 0, len(split.Chunks))
	seenSections := make(map[string]struct{}, len(split.Chunks))

	totalPromptTokens := ytoken.CalcTokenCount(prompt)
	highStaticHash := ""
	for _, chunk := range split.Chunks {
		if chunk == nil {
			continue
		}
		hashes = append(hashes, chunk.Hash)
		contents = append(contents, chunk.Content)
		if _, ok := seenSections[chunk.Section]; !ok {
			sections = append(sections, chunk.Section)
			seenSections[chunk.Section] = struct{}{}
		}
		if chunk.Section == aiprojection.SectionHighStatic && highStaticHash == "" {
			highStaticHash = chunk.Hash
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bestLCP := 0
	bestPrefixBytes := 0
	bestPrefixContent := ""
	for _, prev := range p.history {
		if prev == nil {
			continue
		}
		lcp := planExecCommonPrefixLen(hashes, prev.hashes)
		prefixContent, prefixBytes := planExecBuildPrefixContent(contents, prev.contents, lcp)
		if prefixBytes > bestPrefixBytes || (prefixBytes == bestPrefixBytes && lcp > bestLCP) {
			bestLCP = lcp
			bestPrefixBytes = prefixBytes
			bestPrefixContent = prefixContent
		}
	}

	hitPrefixTokens := ytoken.CalcTokenCount(bestPrefixContent)

	record := &planExecPromptRecord{
		Seq:               len(p.records) + 1,
		TotalPromptTokens: totalPromptTokens,
		HitPrefixTokens:   hitPrefixTokens,
		TokenHitRatio:     planExecRatio(hitPrefixTokens, totalPromptTokens),
		PrefixHitChunks:   bestLCP,
		Sections:          sections,
		HighStaticHash:    highStaticHash,
	}

	p.records = append(p.records, record)
	p.history = append(p.history, &planExecPromptHistory{
		hashes:   hashes,
		contents: contents,
	})

	return record
}

func (p *planExecPromptProbe) Records() []*planExecPromptRecord {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*planExecPromptRecord, 0, len(p.records))
	for _, rec := range p.records {
		cp := *rec
		cp.Sections = append([]string(nil), rec.Sections...)
		out = append(out, &cp)
	}
	return out
}

func planExecCommonPrefixLen(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func planExecBuildPrefixContent(current, previous []string, matchedChunks int) (string, int) {
	var prefix strings.Builder
	prefixBytes := 0

	for i := 0; i < matchedChunks && i < len(current); i++ {
		prefix.WriteString(current[i])
		prefixBytes += len(current[i])
	}

	if matchedChunks < len(current) && matchedChunks < len(previous) {
		partialBytes := planExecStringCommonPrefixLen(current[matchedChunks], previous[matchedChunks])
		if partialBytes > 0 {
			partial := planExecTrimToValidUTF8Prefix(current[matchedChunks][:partialBytes])
			prefix.WriteString(partial)
			prefixBytes += len(partial)
		}
	}

	return prefix.String(), prefixBytes
}

func planExecStringCommonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func planExecTrimToValidUTF8Prefix(s string) string {
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func planExecRatio(hit, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func formatPlanExecProbeDiagnostics(records []*planExecPromptRecord) string {
	if len(records) == 0 {
		return "no intelligent prompts observed"
	}
	totalPromptTokens, totalHitPrefixTokens := summarizePlanExecProbe(records)
	lines := make([]string, 0, len(records)+1)
	lines = append(lines, fmt.Sprintf(
		"total_prompt_tokens=%d total_hit_prefix_tokens=%d global_hit_token_ratio=%.4f",
		totalPromptTokens,
		totalHitPrefixTokens,
		planExecRatio(totalHitPrefixTokens, totalPromptTokens),
	))
	for _, rec := range records {
		lines = append(lines, formatPlanExecProbeRecord(rec))
	}
	return strings.Join(lines, "\n")
}

func formatPlanExecProbeRecord(rec *planExecPromptRecord) string {
	if rec == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"seq=%d total_tokens=%d hit_prefix_tokens=%d token_hit_ratio=%.4f prefix_hit_chunks=%d sections=%s high_static=%s",
		rec.Seq,
		rec.TotalPromptTokens,
		rec.HitPrefixTokens,
		rec.TokenHitRatio,
		rec.PrefixHitChunks,
		strings.Join(rec.Sections, ","),
		shortPlanExecHash(rec.HighStaticHash),
	)
}

func summarizePlanExecProbe(records []*planExecPromptRecord) (int, int) {
	var totalPromptTokens int
	var totalHitPrefixTokens int
	for _, rec := range records {
		if rec == nil {
			continue
		}
		totalPromptTokens += rec.TotalPromptTokens
		totalHitPrefixTokens += rec.HitPrefixTokens
	}
	return totalPromptTokens, totalHitPrefixTokens
}

func shortPlanExecHash(hash string) string {
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}
