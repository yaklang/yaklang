package enhancesearch

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// captureLiteForgeCall 把 enhancesearch 4 个方法实际传给 InvokeLiteForge 的
// (prompt, opts) 抓出来, 同时返回一个能让 GetString / GetStringSlice 顺利取到
// 字段的 mock ForgeResult.
//
// 关键词: enhancesearch P3-X1 hook, RegisterLiteForgeExecuteCallback mock,
//
//	captureLiteForgeCall
type captureLiteForgeCall struct {
	prompt            string
	staticInstruction string
	hasStaticMarker   bool
	rawOpts           []any
}

func installCaptureLiteForgeCallback(t *testing.T, mockResultParams aitool.InvokeParams) *captureLiteForgeCall {
	t.Helper()
	cap := &captureLiteForgeCall{}
	aicommon.RegisterLiteForgeExecuteCallback(func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
		cap.prompt = prompt
		cap.rawOpts = opts
		for _, o := range opts {
			if s, ok := o.(aicommon.LiteForgeStaticInstruction); ok {
				cap.staticInstruction = string(s)
				cap.hasStaticMarker = true
			}
		}
		action := aicommon.NewSimpleAction("call-tool", mockResultParams)
		return &aicommon.ForgeResult{Action: action, Name: "mock"}, nil
	})
	t.Cleanup(func() {
		aicommon.RegisterLiteForgeExecuteCallback(nil)
	})
	return cap
}

// TestEnhanceSearch_StaticInstructionStableAcrossNonces 验证 P3-X1 改造:
// 同一 method 用不同 query 调用, 传给 LiteForge 的 staticInstruction 必须
// byte-identical (不含 nonce / query 拼接), 这样 LiteForge semi-dynamic 段
// 跨调用 hash 才稳定, aicache 可以命中前缀.
//
// 关键词: P3-X1 静态指令稳定性, enhance_section_test, semi-dynamic 段稳定
func TestEnhanceSearch_StaticInstructionStableAcrossNonces(t *testing.T) {
	cases := []struct {
		name         string
		expectStatic string
		invoke       func(t *testing.T, h *LiteForgeSearchHandler, q string)
		mockResult   aitool.InvokeParams
	}{
		{
			name:         "ExtractKeywords",
			expectStatic: extractKeywordsStaticInstruction,
			invoke: func(t *testing.T, h *LiteForgeSearchHandler, q string) {
				_, _ = h.ExtractKeywords(context.Background(), q)
			},
			mockResult: aitool.InvokeParams{
				aicommon.ActionMagicKey: "call-tool",
				"search_keywords":       []string{"k1"},
			},
		},
		{
			name:         "HypotheticalAnswer",
			expectStatic: hydeStaticInstruction,
			invoke: func(t *testing.T, h *LiteForgeSearchHandler, q string) {
				_, _ = h.HypotheticalAnswer(context.Background(), q)
			},
			mockResult: aitool.InvokeParams{
				aicommon.ActionMagicKey: "call-tool",
				"hypothetical_answer":   "stub",
			},
		},
		{
			name:         "SplitQuery",
			expectStatic: splitQueryStaticInstruction,
			invoke: func(t *testing.T, h *LiteForgeSearchHandler, q string) {
				_, _ = h.SplitQuery(context.Background(), q)
			},
			mockResult: aitool.InvokeParams{
				aicommon.ActionMagicKey: "call-tool",
				"sub_questions":         []string{"q1"},
			},
		},
		{
			name:         "GeneralizeQuery",
			expectStatic: generalizeQueryStaticInstruction,
			invoke: func(t *testing.T, h *LiteForgeSearchHandler, q string) {
				_, _ = h.GeneralizeQuery(context.Background(), q)
			},
			mockResult: aitool.InvokeParams{
				aicommon.ActionMagicKey: "call-tool",
				"generalized_query":     []string{"g1"},
			},
		},
	}

	h := NewDefaultSearchHandler()
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cap := installCaptureLiteForgeCallback(t, c.mockResult)

			c.invoke(t, h, "query A about XSS")
			require.True(t, cap.hasStaticMarker,
				"%s must pass aicommon.LiteForgeStaticInstruction marker", c.name)
			firstStatic := cap.staticInstruction
			require.Equal(t, c.expectStatic, firstStatic,
				"%s staticInstruction must equal the package-level constant", c.name)
			require.Equal(t, "query A about XSS", cap.prompt,
				"%s must pass the bare query as InvokeLiteForge first arg, not a templated prompt", c.name)

			c.invoke(t, h, "完全不同的中文 query 关于 SQL 注入")
			require.True(t, cap.hasStaticMarker)
			require.Equal(t, firstStatic, cap.staticInstruction,
				"%s staticInstruction must be byte-identical across different queries", c.name)
			require.Equal(t, "完全不同的中文 query 关于 SQL 注入", cap.prompt,
				"%s second call must still pass bare query as first arg", c.name)
		})
	}
}

// TestEnhanceSearch_StaticInstructionHasNoNonceTemplate 验证静态指令不再包含
// 残留的 {{ .nonce }} / {{ .query }} 模板片段; 改造前指令文本里有
// "<|问题_{{ .nonce }}_START|>{{ .query }}<|问题_{{ .nonce }}_END|>" 这种二层
// nonce 嵌入, 改造后必须全部移除 (LiteForge 模板 dynamic 段已自带 NONCE 包装).
//
// 关键词: P3-X1 nonce 拼接清理, enhancesearch 二层 nonce 移除
func TestEnhanceSearch_StaticInstructionHasNoNonceTemplate(t *testing.T) {
	for _, s := range []struct {
		name string
		body string
	}{
		{"extractKeywords", extractKeywordsStaticInstruction},
		{"hyde", hydeStaticInstruction},
		{"splitQuery", splitQueryStaticInstruction},
		{"generalizeQuery", generalizeQueryStaticInstruction},
	} {
		require.NotContains(t, s.body, "{{ .nonce }}",
			"%s staticInstruction must not carry {{ .nonce }} placeholder", s.name)
		require.NotContains(t, s.body, "{{ .query }}",
			"%s staticInstruction must not carry {{ .query }} placeholder", s.name)
		require.NotContains(t, s.body, "<|问题_",
			"%s staticInstruction must not carry the legacy <|问题_NONCE_START|> wrapper", s.name)
	}
}

// TestEnhanceSearch_QueryGoesThroughDynamicNotStatic 验证 query 不会泄漏到
// staticInstruction 里. 用一个高熵唯一 marker 作为 query, 静态指令必须不含它.
//
// 关键词: P3-X1 query 去重, enhancesearch dynamic 段隔离
func TestEnhanceSearch_QueryGoesThroughDynamicNotStatic(t *testing.T) {
	uniqueMarker := "queryMarker-cdr8wcz0vd9-unique"

	h := NewDefaultSearchHandler()

	cases := []struct {
		name       string
		invoke     func(*LiteForgeSearchHandler, string)
		mockResult aitool.InvokeParams
	}{
		{
			"ExtractKeywords",
			func(h *LiteForgeSearchHandler, q string) {
				_, _ = h.ExtractKeywords(context.Background(), q)
			},
			aitool.InvokeParams{aicommon.ActionMagicKey: "call-tool", "search_keywords": []string{}},
		},
		{
			"HypotheticalAnswer",
			func(h *LiteForgeSearchHandler, q string) {
				_, _ = h.HypotheticalAnswer(context.Background(), q)
			},
			aitool.InvokeParams{aicommon.ActionMagicKey: "call-tool", "hypothetical_answer": ""},
		},
		{
			"SplitQuery",
			func(h *LiteForgeSearchHandler, q string) {
				_, _ = h.SplitQuery(context.Background(), q)
			},
			aitool.InvokeParams{aicommon.ActionMagicKey: "call-tool", "sub_questions": []string{}},
		},
		{
			"GeneralizeQuery",
			func(h *LiteForgeSearchHandler, q string) {
				_, _ = h.GeneralizeQuery(context.Background(), q)
			},
			aitool.InvokeParams{aicommon.ActionMagicKey: "call-tool", "generalized_query": []string{}},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cap := installCaptureLiteForgeCallback(t, c.mockResult)
			c.invoke(h, uniqueMarker)
			require.True(t, cap.hasStaticMarker)
			require.False(t, strings.Contains(cap.staticInstruction, uniqueMarker),
				"%s staticInstruction must NOT carry the per-call query marker", c.name)
			require.Equal(t, uniqueMarker, cap.prompt,
				"%s must pass query as InvokeLiteForge first arg (-> dynamic <params_NONCE> 段)", c.name)
		})
	}
}
