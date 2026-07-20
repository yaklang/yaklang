package loop_yaklangcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

const (
	sampleBudgetBytes     = 24 * 1024
	sampleLLMFallbackRaw  = 50 * 1024
	sampleMaxSnippetBytes = 1200
	grepMaxHitsPerPattern = 15
	ragMaxHits            = 20
)

const (
	sampleSourceGrep = "grep"
	sampleSourceRAG  = "rag"
)

// SampleHit is a unified search result item for init and runtime pipelines.
type SampleHit struct {
	Source  string
	Pattern string
	FileName string
	Line    int
	Score   float64
	Content string
}

// SearchManifest records queries covered during init pre-search.
type SearchManifest struct {
	GrepPatterns      []string `json:"grep_patterns"`
	SemanticQuestions []string `json:"semantic_questions"`
	CoveredAtInit     bool     `json:"covered_at_init"`
}

func NewSearchManifest(grepPatterns, semanticQuestions []string) SearchManifest {
	return SearchManifest{
		GrepPatterns:      append([]string(nil), grepPatterns...),
		SemanticQuestions: append([]string(nil), semanticQuestions...),
		CoveredAtInit:     true,
	}
}

func (m SearchManifest) JSON() string {
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(raw)
}

func ParseSearchManifest(raw string) SearchManifest {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SearchManifest{}
	}
	var m SearchManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return SearchManifest{}
	}
	return m
}

func FormatManifestForPrompt(raw string) string {
	m := ParseSearchManifest(raw)
	if !m.CoveredAtInit && len(m.GrepPatterns) == 0 && len(m.SemanticQuestions) == 0 {
		return ""
	}
	var b strings.Builder
	if len(m.GrepPatterns) > 0 {
		b.WriteString("Grep: ")
		b.WriteString(strings.Join(m.GrepPatterns, ", "))
	}
	if len(m.SemanticQuestions) > 0 {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString("Semantic: ")
		b.WriteString(strings.Join(m.SemanticQuestions, ", "))
	}
	return b.String()
}

func GrepResultsToSampleHits(pattern string, results []*ziputil.GrepResult, maxHits int) []SampleHit {
	if maxHits <= 0 {
		maxHits = grepMaxHitsPerPattern
	}
	limit := len(results)
	if limit > maxHits {
		limit = maxHits
	}
	hits := make([]SampleHit, 0, limit)
	for i := 0; i < limit; i++ {
		result := results[i]
		score := 1.0 - float64(i)/float64(limit+1)
		hits = append(hits, SampleHit{
			Source:   sampleSourceGrep,
			Pattern:  pattern,
			FileName: result.FileName,
			Line:     result.LineNumber,
			Score:    score,
			Content:  formatGrepResultContent(result),
		})
	}
	return hits
}

func formatGrepResultContent(result *ziputil.GrepResult) string {
	var b strings.Builder
	if len(result.ContextBefore) > 0 {
		for _, line := range result.ContextBefore {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString(result.Line)
	b.WriteString("\n")
	if len(result.ContextAfter) > 0 {
		for _, line := range result.ContextAfter {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func RAGResultsToSampleHits(question string, results []rag.SearchResult, maxHits int) []SampleHit {
	if maxHits <= 0 {
		maxHits = ragMaxHits
	}
	limit := len(results)
	if limit > maxHits {
		limit = maxHits
	}
	hits := make([]SampleHit, 0, limit)
	for i := 0; i < limit; i++ {
		result := results[i]
		content := ragResultContent(result)
		if content == "" {
			continue
		}
		score := result.Score
		if score <= 0 {
			score = 1.0 - float64(i)/float64(limit+1)
		}
		hits = append(hits, SampleHit{
			Source:   sampleSourceRAG,
			Pattern:  question,
			FileName: ragResultFileName(result),
			Line:     i + 1,
			Score:    score,
			Content:  content,
		})
	}
	return hits
}

func ragResultContent(result rag.SearchResult) string {
	if result.KnowledgeBaseEntry != nil {
		return strings.TrimSpace(result.KnowledgeBaseEntry.KnowledgeDetails)
	}
	if result.Document != nil {
		return strings.TrimSpace(result.Document.Content)
	}
	return ""
}

func ragResultFileName(result rag.SearchResult) string {
	if result.KnowledgeBaseEntry != nil {
		return result.KnowledgeBaseEntry.KnowledgeTitle
	}
	if result.Document != nil {
		return result.Document.ID
	}
	return "rag"
}

func sampleHitKey(hit SampleHit) string {
	normalized := strings.TrimSpace(hit.Content)
	if len(normalized) > 256 {
		normalized = normalized[:256]
	}
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%s:%d:%s", hit.FileName, hit.Line, hex.EncodeToString(sum[:8]))
}

func tokenOverlapScore(query, content string) float64 {
	queryTokens := tokenizeForOverlap(query)
	if len(queryTokens) == 0 {
		return 0
	}
	contentLower := strings.ToLower(content)
	matched := 0
	for token := range queryTokens {
		if strings.Contains(contentLower, token) {
			matched++
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func tokenizeForOverlap(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, part := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r > 127
	}) {
		part = strings.TrimSpace(part)
		if len(part) < 2 {
			continue
		}
		tokens[part] = struct{}{}
	}
	return tokens
}

func enrichSampleScores(hits []SampleHit, query string) {
	for i := range hits {
		overlap := tokenOverlapScore(query, hits[i].Content)
		if hits[i].Source == sampleSourceGrep {
			hits[i].Score = hits[i].Score*0.7 + overlap*0.3
		} else {
			hits[i].Score = hits[i].Score*0.85 + overlap*0.15
		}
	}
}

func truncateSnippet(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= sampleMaxSnippetBytes {
		return content
	}
	return content[:sampleMaxSnippetBytes] + "\n[... truncated ...]"
}

func formatSampleHit(hit SampleHit) string {
	header := fmt.Sprintf("[%s] %s:%d", hit.Source, hit.FileName, hit.Line)
	if hit.Pattern != "" {
		header += fmt.Sprintf(" (query: %s)", hit.Pattern)
	}
	return header + "\n" + truncateSnippet(hit.Content)
}

// RankAndTrimSamples deduplicates, scores, and trims hits to fit budget bytes.
func RankAndTrimSamples(hits []SampleHit, query string, budget int) string {
	if budget <= 0 {
		budget = sampleBudgetBytes
	}
	if len(hits) == 0 {
		return ""
	}

	enrichSampleScores(hits, query)

	deduped := make(map[string]SampleHit, len(hits))
	for _, hit := range hits {
		key := sampleHitKey(hit)
		existing, ok := deduped[key]
		if !ok || hit.Score > existing.Score {
			deduped[key] = hit
		}
	}

	ordered := make([]SampleHit, 0, len(deduped))
	for _, hit := range deduped {
		ordered = append(ordered, hit)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Score > ordered[j].Score
	})

	var b strings.Builder
	used := 0
	for _, hit := range ordered {
		block := formatSampleHit(hit)
		if used > 0 && used+len(block)+2 > budget {
			break
		}
		if used > 0 {
			b.WriteString("\n\n")
			used += 2
		}
		b.WriteString(block)
		used += len(block)
	}
	return b.String()
}

type sampleCompressor interface {
	CompressLongTextWithDestination(ctx context.Context, input any, destination string, targetByteSize int64) (string, error)
}

// MaybeCompressSamples applies deterministic trim first; LLM compress only when raw exceeds sampleLLMFallbackRaw.
func MaybeCompressSamples(ctx context.Context, raw string, query string, invoker sampleCompressor) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= sampleBudgetBytes {
		return raw
	}
	if invoker == nil || len(raw) <= sampleLLMFallbackRaw {
		return utils.ShrinkTextBlock(raw, sampleBudgetBytes)
	}
	compressed, err := invoker.CompressLongTextWithDestination(ctx, raw, query, sampleBudgetBytes)
	if err != nil {
		log.Warnf("MaybeCompressSamples: compress failed: %v, using shrink fallback", err)
		return utils.ShrinkTextBlock(raw, sampleBudgetBytes)
	}
	if strings.TrimSpace(compressed) == "" {
		return utils.ShrinkTextBlock(raw, sampleBudgetBytes)
	}
	return compressed
}

// FinalizeSearchResults ranks hits and optionally applies rare LLM compression.
func FinalizeSearchResults(ctx context.Context, hits []SampleHit, query string, invoker sampleCompressor) string {
	trimmed := RankAndTrimSamples(hits, query, sampleBudgetBytes)
	if trimmed == "" {
		return ""
	}
	return MaybeCompressSamples(ctx, trimmed, query, invoker)
}

// --- search policy & duplicate query (merged from search_policy.go / duplicate_query.go) ---

func normalizeGrepPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.ToLower(pattern)
	pattern = strings.ReplaceAll(pattern, "\\\\", "\\")
	return pattern
}

func normalizeSemanticQuestion(question string) string {
	question = strings.TrimSpace(question)
	question = strings.ToLower(question)
	return question
}

func hasInitialSamples(loop interface{ Get(string) string }) bool {
	if loop.Get("init_samples_ready") == "true" {
		return true
	}
	return strings.TrimSpace(loop.Get("initial_code_samples")) != ""
}

// hasBlockingLintErrors is true when the last write/modify reported Error-level static analysis issues.
func hasBlockingLintErrors(loop interface{ Get(string) string }) bool {
	return loop.Get("yak_lint_ok") == "false"
}

// hasFailedSelfTest is true when the last YAK_MAIN self-test failed.
func hasFailedSelfTest(loop interface{ Get(string) string }) bool {
	return loop.Get(loopVarYakRunOK) == "false"
}

// needsSampleResearch unlocks mid-loop grep/yakdoc re-query after lint or self-test failure.
func needsSampleResearch(loop interface{ Get(string) string }) bool {
	return hasBlockingLintErrors(loop) || hasFailedSelfTest(loop)
}

func loadSearchManifest(loop interface{ Get(string) string }) SearchManifest {
	return ParseSearchManifest(loop.Get("init_search_manifest"))
}

func GrepAlreadyCovered(loop interface{ Get(string) string }, pattern string) (bool, string) {
	if needsSampleResearch(loop) {
		return false, ""
	}
	if !hasInitialSamples(loop) {
		return false, ""
	}
	manifest := loadSearchManifest(loop)
	if len(manifest.GrepPatterns) == 0 {
		return false, ""
	}
	normalized := normalizeGrepPattern(pattern)
	for _, p := range manifest.GrepPatterns {
		if normalizeGrepPattern(p) == normalized {
			msg := fmt.Sprintf(`【Init 已覆盖】搜索模式 "%s" 已在初始化阶段预检索。

请直接参考 reactive_data 中的「预检索代码样例（Init 已完成）」段落，禁止重复相同 grep。

若需要不同角度，请修改 pattern 或使用 semantic_search_yaklang_samples。`, pattern)
			return true, msg
		}
	}
	return false, ""
}

func SemanticAlreadyCovered(loop interface{ Get(string) string }, questions []string) (bool, string) {
	if needsSampleResearch(loop) {
		return false, ""
	}
	if !hasInitialSamples(loop) {
		return false, ""
	}
	manifest := loadSearchManifest(loop)
	if len(manifest.SemanticQuestions) == 0 {
		return false, ""
	}
	if len(questions) == 0 {
		return false, ""
	}

	initSet := make(map[string]struct{}, len(manifest.SemanticQuestions))
	for _, q := range manifest.SemanticQuestions {
		initSet[normalizeSemanticQuestion(q)] = struct{}{}
	}
	allCovered := true
	for _, q := range questions {
		if _, ok := initSet[normalizeSemanticQuestion(q)]; !ok {
			allCovered = false
			break
		}
	}
	if !allCovered {
		return false, ""
	}
	msg := fmt.Sprintf(`【Init 已覆盖】语义问题已在初始化阶段预检索：

%s

请直接参考 reactive_data 中的「预检索代码样例（Init 已完成）」段落，禁止重复相同 semantic 搜索。`, strings.Join(questions, "\n"))
	return true, msg
}

func shortGrepSuggestion(count int, pattern string) string {
	if count < 3 {
		return fmt.Sprintf("【提示】仅找到 %d 条匹配，可考虑扩大 pattern 或使用 semantic_search。", count)
	}
	if count > 15 {
		return fmt.Sprintf("【提示】找到 %d 条匹配，已裁剪为 top 结果；可精确化 pattern。", count)
	}
	return fmt.Sprintf("【提示】找到 %d 条匹配，可基于样例开始编码。", count)
}

func shortSemanticSuggestion(count int) string {
	if count < 5 {
		return fmt.Sprintf("【提示】仅找到 %d 条语义匹配，可调整问题或降低 score_threshold。", count)
	}
	if count > 20 {
		return fmt.Sprintf("【提示】找到 %d 条语义匹配，已裁剪为 top 结果。", count)
	}
	return fmt.Sprintf("【提示】找到 %d 条语义匹配，可基于样例开始编码。", count)
}

func rejectDuplicateQuery(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator, timelineKey, queryKey, currentQuery, msg string) bool {
	// Allow re-running the same grep/semantic/yakdoc query while fixing lint or self-test failures.
	if needsSampleResearch(loop) {
		return false
	}
	last := loop.Get(queryKey)
	if last == "" || last != currentQuery {
		return false
	}
	invoker := loop.GetInvoker()
	invoker.AddToTimeline(timelineKey, msg)
	op.Feedback(msg)
	op.Continue()
	return true
}
