package loop_http_flow_analyze

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestQueryResultManagement(t *testing.T) {
	// 创建一个测试 loop（模拟）
	loop := &reactloops.ReActLoop{}

	// 测试保存和获取查询结果
	result := &QueryResult{
		Name:        "test_query",
		FlowIDs:     []int64{1, 2, 3},
		TotalCount:  100,
		QueryParams: "keyword=test",
		SummaryFile: "/tmp/test.txt",
	}

	saveQueryResult(loop, result)

	// 按名称获取
	retrieved := getQueryResult(loop, "test_query")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test_query", retrieved.Name)
	assert.Equal(t, 3, len(retrieved.FlowIDs))

	// 使用 "last" 快捷方式
	lastResult := getQueryResult(loop, "last")
	assert.NotNil(t, lastResult)
	assert.Equal(t, "test_query", lastResult.Name)

	// 列出所有查询
	queries := listQueryResults(loop)
	assert.Equal(t, 1, len(queries))
}

func TestMatchResultManagement(t *testing.T) {
	loop := &reactloops.ReActLoop{}

	// 测试保存和获取匹配结果
	result := &MatchResult{
		Name:         "test_match",
		SourceQuery:  "test_query",
		FlowIDs:      []int64{1, 2},
		MatchedCount: 2,
		MatcherDesc:  "keywords=error",
		SummaryFile:  "/tmp/match.txt",
	}

	saveMatchResult(loop, result)

	// 按名称获取
	retrieved := getMatchResult(loop, "test_match")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test_match", retrieved.Name)
	assert.Equal(t, 2, retrieved.MatchedCount)

	// 使用 "last" 快捷方式
	lastMatch := getMatchResult(loop, "last_match")
	assert.NotNil(t, lastMatch)
	assert.Equal(t, "test_match", lastMatch.Name)
}

func TestSecurityPatterns(t *testing.T) {
	// 测试内置安全模式
	sqlPattern := getSecurityPattern("sql_injection")
	assert.NotNil(t, sqlPattern)
	assert.Equal(t, "SQL Injection", sqlPattern.Name)
	assert.True(t, len(sqlPattern.Matchers) > 0)

	xssPattern := getSecurityPattern("xss")
	assert.NotNil(t, xssPattern)
	assert.Equal(t, "Cross-Site Scripting", xssPattern.Name)

	sensitivePattern := getSecurityPattern("sensitive_data")
	assert.NotNil(t, sensitivePattern)
	assert.Equal(t, "Sensitive Data Exposure", sensitivePattern.Name)

	// 测试不存在的模式
	unknown := getSecurityPattern("unknown_pattern")
	assert.Nil(t, unknown)
}

func TestSimplifiedMatcherConversion(t *testing.T) {
	// 测试简化 matcher 到 YakMatcher 的转换
	simplified := &SimplifiedMatcher{
		Type:     "word",
		Patterns: []string{"error", "failed"},
		Scope:    "response",
		MatchAll: true,
		Negative: false,
	}

	yakMatcher := convertSimplifiedToYakMatcher(simplified)
	assert.NotNil(t, yakMatcher)
	assert.Equal(t, "word", yakMatcher.MatcherType)
	assert.Equal(t, "response", yakMatcher.Scope)
	assert.Equal(t, "and", yakMatcher.Condition)
	assert.Equal(t, 2, len(yakMatcher.Group))
	assert.False(t, yakMatcher.Negative)
}

func TestDescribeSimplifiedMatchers(t *testing.T) {
	matchers := []SimplifiedMatcher{
		{
			Type:     "word",
			Patterns: []string{"error", "failed"},
			Scope:    "response",
		},
		{
			Type:     "status",
			Patterns: []string{"500", "502"},
		},
	}

	desc := describeSimplifiedMatchers(matchers)
	assert.Contains(t, desc, "word/response")
	assert.Contains(t, desc, "status")
}

func TestParseFlowIDs(t *testing.T) {
	// 测试解析流量 ID
	ids := parseFlowIDs("123,456,789")
	assert.Equal(t, 3, len(ids))
	assert.Equal(t, int64(123), ids[0])
	assert.Equal(t, int64(456), ids[1])
	assert.Equal(t, int64(789), ids[2])

	// 测试带空格
	ids2 := parseFlowIDs("123, 456 , 789")
	assert.Equal(t, 3, len(ids2))

	// 测试空字符串
	ids3 := parseFlowIDs("")
	assert.Equal(t, 0, len(ids3))
}
