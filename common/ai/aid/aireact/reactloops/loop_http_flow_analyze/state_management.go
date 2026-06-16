package loop_http_flow_analyze

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	// Query 结果映射
	queryResultsMapKey = "query_results_map" // map[string]*QueryResult
	lastQueryNameKey   = "last_query_name"   // string

	// Match 结果映射
	matchResultsMapKey = "match_results_map" // map[string]*MatchResult
	lastMatchNameKey   = "last_match_name"   // string
)

// QueryResult 存储查询结果
type QueryResult struct {
	Name        string    `json:"name"`
	FlowIDs     []int64   `json:"flow_ids"`
	TotalCount  int       `json:"total_count"`
	QueryParams string    `json:"query_params"`
	SummaryFile string    `json:"summary_file"`
	CreatedAt   time.Time `json:"created_at"`
}

// MatchResult 存储匹配结果
type MatchResult struct {
	Name         string    `json:"name"`
	SourceQuery  string    `json:"source_query"` // 引用的查询名称
	FlowIDs      []int64   `json:"flow_ids"`
	MatchedCount int       `json:"matched_count"`
	MatcherDesc  string    `json:"matcher_desc"`
	SummaryFile  string    `json:"summary_file"`
	CreatedAt    time.Time `json:"created_at"`
}

// === Query 结果管理 ===

// saveQueryResult 保存查询结果
func saveQueryResult(loop *reactloops.ReActLoop, result *QueryResult) {
	if loop == nil || result == nil {
		return
	}

	// 获取现有映射
	resultsMap := getQueryResultsMap(loop)

	// 保存结果
	resultsMap[result.Name] = result
	loop.Set(queryResultsMapKey, resultsMap)

	// 更新 last_query
	loop.Set(lastQueryNameKey, result.Name)

	log.Infof("query result saved: name=%s, flows=%d", result.Name, len(result.FlowIDs))
}

// getQueryResult 获取查询结果
func getQueryResult(loop *reactloops.ReActLoop, name string) *QueryResult {
	if loop == nil || name == "" {
		return nil
	}

	// 特殊值：last/last_query
	if name == "last" || name == "last_query" {
		name = loop.Get(lastQueryNameKey)
		if name == "" {
			return nil
		}
	}

	resultsMap := getQueryResultsMap(loop)
	return resultsMap[name]
}

// getQueryResultsMap 获取查询结果映射
func getQueryResultsMap(loop *reactloops.ReActLoop) map[string]*QueryResult {
	if loop == nil {
		return make(map[string]*QueryResult)
	}

	raw := loop.GetVariable(queryResultsMapKey)
	if raw == nil {
		return make(map[string]*QueryResult)
	}

	if m, ok := raw.(map[string]*QueryResult); ok {
		return m
	}

	return make(map[string]*QueryResult)
}

// listQueryResults 列出所有查询结果
func listQueryResults(loop *reactloops.ReActLoop) []*QueryResult {
	resultsMap := getQueryResultsMap(loop)
	results := make([]*QueryResult, 0, len(resultsMap))
	for _, r := range resultsMap {
		results = append(results, r)
	}
	// 按时间排序（最新的在前）
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return results
}

// === Match 结果管理 ===

// saveMatchResult 保存匹配结果
func saveMatchResult(loop *reactloops.ReActLoop, result *MatchResult) {
	if loop == nil || result == nil {
		return
	}

	resultsMap := getMatchResultsMap(loop)
	resultsMap[result.Name] = result
	loop.Set(matchResultsMapKey, resultsMap)
	loop.Set(lastMatchNameKey, result.Name)

	log.Infof("match result saved: name=%s, matched=%d", result.Name, len(result.FlowIDs))
}

// getMatchResult 获取匹配结果
func getMatchResult(loop *reactloops.ReActLoop, name string) *MatchResult {
	if loop == nil || name == "" {
		return nil
	}

	// 特殊值：last/last_match
	if name == "last" || name == "last_match" {
		name = loop.Get(lastMatchNameKey)
		if name == "" {
			return nil
		}
	}

	resultsMap := getMatchResultsMap(loop)
	return resultsMap[name]
}

// getMatchResultsMap 获取匹配结果映射
func getMatchResultsMap(loop *reactloops.ReActLoop) map[string]*MatchResult {
	if loop == nil {
		return make(map[string]*MatchResult)
	}

	raw := loop.GetVariable(matchResultsMapKey)
	if raw == nil {
		return make(map[string]*MatchResult)
	}

	if m, ok := raw.(map[string]*MatchResult); ok {
		return m
	}

	return make(map[string]*MatchResult)
}

// listMatchResults 列出所有匹配结果
func listMatchResults(loop *reactloops.ReActLoop) []*MatchResult {
	resultsMap := getMatchResultsMap(loop)
	results := make([]*MatchResult, 0, len(resultsMap))
	for _, r := range resultsMap {
		results = append(results, r)
	}
	// 按时间排序（最新的在前）
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return results
}

// parseFlowIDs 解析流量 ID 字符串为 int64 切片
func parseFlowIDs(flowIDsStr string) []int64 {
	parts := splitMulti(flowIDsStr)
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// buildSavedQueriesPrompt 构建已保存查询的提示信息
func buildSavedQueriesPrompt(loop *reactloops.ReActLoop) string {
	queries := listQueryResults(loop)
	if len(queries) == 0 {
		return ""
	}

	var out strings.Builder
	for i, q := range queries {
		if i >= 5 { // 只显示最近的 5 个
			break
		}
		out.WriteString(fmt.Sprintf("  - '%s': %d flows (params: %s)\n",
			q.Name, len(q.FlowIDs), q.QueryParams))
	}
	return out.String()
}

// buildSavedMatchesPrompt 构建已保存匹配的提示信息
func buildSavedMatchesPrompt(loop *reactloops.ReActLoop) string {
	matches := listMatchResults(loop)
	if len(matches) == 0 {
		return ""
	}

	var out strings.Builder
	for i, m := range matches {
		if i >= 5 { // 只显示最近的 5 个
			break
		}
		out.WriteString(fmt.Sprintf("  - '%s': %d matched from '%s' (matcher: %s)\n",
			m.Name, len(m.FlowIDs), m.SourceQuery, m.MatcherDesc))
	}
	return out.String()
}
