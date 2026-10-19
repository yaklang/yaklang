package aiknowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

func TestKnowledgeQueryExplicitScopeAndGlobalBounds(t *testing.T) {
	_, opts, _, _ := fixture(t)
	for _, name := range []string{"docs", "other", "all"} {
		create(t, opts, name)
		for i := 0; i < 3; i++ {
			manage(t, opts, map[string]any{"op": "add_entry", "name": name, "entry": map[string]any{"title": fmt.Sprintf("%s-%d", name, i), "content": "MATCH_" + name + strings.Repeat("\n引号\"反斜杠\\😀", 80)}})
		}
	}
	query := func(params map[string]any) *QueryResult {
		t.Helper()
		params["token_limit"] = 1024
		params["per_entry_token_limit"] = 256
		r, err := Query(params, opts...)
		require.NoError(t, err)
		raw, err := json.Marshal(r)
		require.NoError(t, err)
		require.LessOrEqual(t, ytoken.CalcTokenCount(string(raw)), r.UsedTokens)
		require.LessOrEqual(t, r.UsedTokens, 1024)
		for _, hit := range r.Results {
			raw, _ := json.Marshal(hit)
			require.LessOrEqual(t, ytoken.CalcTokenCount(string(raw)), 256)
			require.True(t, hit.Truncated)
		}
		return r
	}
	for _, params := range []map[string]any{{}, {"all": false}, {"name": " "}, {"names": []string{}}} {
		_, err := Query(params, opts...)
		require.ErrorContains(t, err, "manage_knowledge_base")
		require.ErrorContains(t, err, `"op":"list"`)
		require.ErrorContains(t, err, "names")
		require.ErrorContains(t, err, "all=true")
	}
	r := query(map[string]any{"name": "all"})
	require.Equal(t, 3, r.Total, "the literal base named all must not act as a wildcard")
	for _, hit := range r.Results {
		require.Equal(t, "all", hit.KnowledgeBase)
	}
	r = query(map[string]any{"names": []string{"other", "docs"}, "limit": 1, "offset": 3})
	require.Equal(t, 6, r.Total)
	require.Len(t, r.Results, 1)
	require.Equal(t, "other", r.Results[0].KnowledgeBase)
	require.Equal(t, 4, r.NextOffset)
	require.True(t, r.HasMore)
	r = query(map[string]any{"all": true, "limit": 2})
	require.Equal(t, 9, r.Total)
	require.Equal(t, 3, r.KnowledgeBaseCount)
	require.Len(t, r.Results, 2, "limit applies globally, not separately per base")
	for _, scope := range []map[string]any{{"all": true}, {"names": []string{"docs", "other"}}} {
		scope["mode"], scope["query"], scope["limit"] = "semantic", "MATCH", 2
		r = query(scope)
		require.Len(t, r.Results, 2)
		require.NotNil(t, r.Results[0].Score)
		require.GreaterOrEqual(t, *r.Results[0].Score, *r.Results[1].Score)
	}
	r = query(map[string]any{"all": true, "limit": 50})
	require.True(t, r.BudgetExhausted)
	require.Less(t, len(r.Results), 9)
	require.Equal(t, len(r.Results), r.NextOffset, "budget exhaustion must not skip unreturned entries")
	_, err := Query(map[string]any{"all": true, "token_limit": 1024}, append(opts, WithContext(cancelledQueryContext()))...)
	require.ErrorIs(t, err, context.Canceled)
}

func cancelledQueryContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestKnowledgeQueryRejectsUnsafeOrAmbiguousParameters(t *testing.T) {
	_, opts, _, _ := fixture(t)
	create(t, opts, "docs")
	for _, fields := range []map[string]any{
		{"all": true}, {"names": []string{"docs"}}, {"names": []string{""}},
		{"names": []string{"docs", "docs"}, "name": ""},
		{"names": []string{"docs", "missing"}, "name": ""},
		{"token_limit": 255}, {"token_limit": 8193}, {"limit": -1}, {"limit": 51},
		{"offset": -1}, {"offset": 100001}, {"query": strings.Repeat("x", 4097)},
		{"per_entry_token_limit": 127}, {"per_entry_token_limit": 8193},
		{"mode": "semantic", "query": "x", "min_score": 1.01},
		{"mode": "semantic", "query": "x", "offset": 1},
		{"min_score": 0.5}, {"entry_id": "x"}, {"unknown": 1},
		{"mode": "get", "entry_id": "x", "name": "", "all": true},
		{"mode": "get", "entry_id": "x", "query": "ignored?"},
	} {
		params := map[string]any{"name": "docs", "token_limit": 1024}
		for key, value := range fields {
			params[key] = value
		}
		_, err := Query(params, opts...)
		require.Error(t, err, "%v", params)
	}
	for i := 1; i <= maxQueryBases; i++ {
		create(t, opts, fmt.Sprintf("base-%d", i))
	}
	_, err := Query(map[string]any{"all": true, "token_limit": 1024}, opts...)
	require.ErrorContains(t, err, "safety limit")
	require.ErrorContains(t, err, "op=list")
}
