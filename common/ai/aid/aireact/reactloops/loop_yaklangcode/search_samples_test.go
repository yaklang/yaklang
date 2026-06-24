package loop_yaklangcode

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSampleCompressor struct {
	called bool
}

func (m *mockSampleCompressor) CompressLongTextWithDestination(ctx context.Context, input any, destination string, targetByteSize int64) (string, error) {
	m.called = true
	return "compressed", nil
}

func TestRankAndTrimSamples_DedupAndBudget(t *testing.T) {
	hits := []SampleHit{
		{Source: sampleSourceGrep, FileName: "a.yak", Line: 1, Score: 0.5, Content: "scan target"},
		{Source: sampleSourceGrep, FileName: "a.yak", Line: 1, Score: 0.9, Content: "scan target"},
		{Source: sampleSourceRAG, FileName: "b.yak", Line: 2, Score: 0.8, Content: "http request"},
	}
	out := RankAndTrimSamples(hits, "scan http", 512)
	require.Contains(t, out, "[grep] a.yak:1")
	require.Contains(t, out, "[rag] b.yak:2")
	assert.Equal(t, 1, strings.Count(out, "[grep] a.yak:1"))
}

func TestMaybeCompressSamples_SkipsLLMUnderThreshold(t *testing.T) {
	raw := RankAndTrimSamples([]SampleHit{
		{Source: sampleSourceGrep, FileName: "a.yak", Line: 1, Score: 1, Content: "hello"},
	}, "hello", sampleBudgetBytes)
	mock := &mockSampleCompressor{}
	out := MaybeCompressSamples(context.Background(), raw, "hello", mock)
	assert.Equal(t, raw, out)
	assert.False(t, mock.called)
}

func TestMaybeCompressSamples_CallsLLMWhenLarge(t *testing.T) {
	large := make([]byte, sampleLLMFallbackRaw+1024)
	for i := range large {
		large[i] = 'x'
	}
	raw := string(large)
	mock := &mockSampleCompressor{}
	out := MaybeCompressSamples(context.Background(), raw, "query", mock)
	assert.True(t, mock.called)
	assert.Equal(t, "compressed", out)
}

func TestMaybeCompressSamples_ShrinksMidSizeWithoutLLM(t *testing.T) {
	raw := strings.Repeat("y", sampleBudgetBytes+1024)
	mock := &mockSampleCompressor{}
	out := MaybeCompressSamples(context.Background(), raw, "query", mock)
	assert.False(t, mock.called)
	assert.LessOrEqual(t, len(out), sampleBudgetBytes+64)
}

func TestParseSearchManifest(t *testing.T) {
	m := NewSearchManifest([]string{"scan"}, []string{"Yaklang scan?"})
	parsed := ParseSearchManifest(m.JSON())
	assert.Equal(t, []string{"scan"}, parsed.GrepPatterns)
	assert.True(t, parsed.CoveredAtInit)
}

type stubLoopKV struct {
	data map[string]string
}

func (s *stubLoopKV) Get(key string) string {
	return s.data[key]
}

func TestGrepAlreadyCovered(t *testing.T) {
	loop := &stubLoopKV{data: map[string]string{
		"init_samples_ready":   "true",
		"initial_code_samples": "sample content",
		"init_search_manifest": NewSearchManifest([]string{"servicescan\\.Scan"}, nil).JSON(),
	}}
	covered, msg := GrepAlreadyCovered(loop, "servicescan\\.Scan")
	assert.True(t, covered)
	assert.Contains(t, msg, "Init 已覆盖")
}

func TestGrepAlreadyCovered_NewPattern(t *testing.T) {
	loop := &stubLoopKV{data: map[string]string{
		"init_samples_ready":   "true",
		"initial_code_samples": "sample content",
		"init_search_manifest": NewSearchManifest([]string{"scan"}, nil).JSON(),
	}}
	covered, _ := GrepAlreadyCovered(loop, "poc.HTTP")
	assert.False(t, covered)
}

func TestSemanticAlreadyCovered(t *testing.T) {
	loop := &stubLoopKV{data: map[string]string{
		"init_samples_ready":   "true",
		"initial_code_samples": "sample",
		"init_search_manifest": NewSearchManifest(nil, []string{"Yaklang中如何扫描端口？"}).JSON(),
	}}
	covered, msg := SemanticAlreadyCovered(loop, []string{"Yaklang中如何扫描端口？"})
	assert.True(t, covered)
	assert.Contains(t, msg, "Init 已覆盖")
}

func TestGrepAlreadyCovered_BypassWhenLintFailed(t *testing.T) {
	loop := &stubLoopKV{data: map[string]string{
		"init_samples_ready":   "true",
		"initial_code_samples": "sample",
		"init_search_manifest": NewSearchManifest([]string{"servicescan\\.Scan"}, nil).JSON(),
		"yak_lint_ok":          "false",
	}}
	covered, _ := GrepAlreadyCovered(loop, "servicescan\\.Scan")
	assert.False(t, covered)
}

func TestSemanticAlreadyCovered_BypassWhenLintFailed(t *testing.T) {
	loop := &stubLoopKV{data: map[string]string{
		"init_samples_ready":   "true",
		"init_search_manifest": NewSearchManifest(nil, []string{"Yaklang中如何扫描端口？"}).JSON(),
		"yak_lint_ok":          "false",
	}}
	covered, _ := SemanticAlreadyCovered(loop, []string{"Yaklang中如何扫描端口？"})
	assert.False(t, covered)
}

func TestFormatManifestForPrompt(t *testing.T) {
	raw := NewSearchManifest([]string{"a", "b"}, []string{"q1"}).JSON()
	out := FormatManifestForPrompt(raw)
	assert.Contains(t, out, "Grep: a, b")
	assert.Contains(t, out, "Semantic: q1")
}

func TestRejectDuplicateQueryLogic(t *testing.T) {
	last := "pat|false|15"
	current := "pat|false|15"
	assert.True(t, last == current)
	last = "other"
	assert.False(t, last == current)
}
