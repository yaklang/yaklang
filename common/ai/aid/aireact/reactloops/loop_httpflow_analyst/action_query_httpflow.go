package loop_httpflow_analyst

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// HTTPFlowQueryResult represents a simplified HTTPFlow result for AI analysis
type HTTPFlowQueryResult struct {
	ID            uint   `json:"id"`
	URL           string `json:"url"`
	Method        string `json:"method"`
	StatusCode    int64  `json:"status_code"`
	ContentType   string `json:"content_type"`
	BodyLength    int64  `json:"body_length"`
	RequestLength int64  `json:"request_length"`
	Duration      int64  `json:"duration_ms"`
	RemoteAddr    string `json:"remote_addr"`
	Tags          string `json:"tags"`
	SourceType    string `json:"source_type"`
	Path          string `json:"path"`
	IsHTTPS       bool   `json:"is_https"`
	CreatedAt     string `json:"created_at"`
}

// HTTPFlowQuerySummary provides aggregated statistics
type HTTPFlowQuerySummary struct {
	TotalCount      int64                 `json:"total_count"`
	StatusCodeDist  map[string]int        `json:"status_code_distribution"`
	MethodDist      map[string]int        `json:"method_distribution"`
	TopHosts        []HostCount           `json:"top_hosts"`
	TopPaths        []PathCount           `json:"top_paths"`
	ContentTypeDist map[string]int        `json:"content_type_distribution"`
	TimeRange       string                `json:"time_range"`
	QueryParams     string                `json:"query_params"`
	Samples         []HTTPFlowQueryResult `json:"samples"`
}

type HostCount struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

type PathCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// queryHTTPFlowAction creates the action for querying HTTPFlow database
var queryHTTPFlowAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"query_httpflow",
		`Query HTTPFlow Database - 查询 HTTP 流量历史

【功能说明】
查询 HTTPFlow 数据库获取 HTTP 流量数据。支持多种过滤条件。
结果会自动保存到本地文件，避免上下文溢出。

【参数说明】
- query_type (必需): 查询类型
  - "count": 统计总数
  - "aggregate": 聚合分析（返回统计分布）
  - "sample": 采样查询（返回具体样本）
  - "search": 关键词搜索
- keyword (可选): 搜索关键词，会在 URL/请求/响应中搜索
- keyword_type (可选): 关键词搜索范围 - "request"/"response"/"" (全部)
- status_code (可选): HTTP 状态码过滤，如 "200" 或 "500,502,503"
- method (可选): HTTP 方法过滤，如 "POST" 或 "GET,POST"
- url_pattern (可选): URL 包含模式
- content_type (可选): Content-Type 包含模式
- source_type (可选): 来源类型 "mitm"/"scan"
- after_timestamp (可选): 起始时间戳（Unix 秒）
- before_timestamp (可选): 结束时间戳（Unix 秒）
- min_body_length (可选): 最小响应体长度
- max_body_length (可选): 最大响应体长度
- limit (可选): 返回数量限制，默认 50
- include_hosts (可选): 包含的主机列表
- exclude_hosts (可选): 排除的主机列表
- reason (必需): 解释为什么执行这个查询

【使用时机】
- 需要统计 HTTP 流量分布时
- 需要查找特定模式的请求时
- 需要采样证据时
- 探索数据库中的流量特征时`,
		[]aitool.ToolOption{
			aitool.WithStringParam("query_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Enum("count", "aggregate", "sample", "search"),
				aitool.WithParam_Description("Query type: count/aggregate/sample/search")),
			aitool.WithStringParam("keyword",
				aitool.WithParam_Description("Search keyword in URL/request/response")),
			aitool.WithStringParam("keyword_type",
				aitool.WithParam_Enum("request", "response", ""),
				aitool.WithParam_Description("Keyword search scope")),
			aitool.WithStringParam("status_code",
				aitool.WithParam_Description("HTTP status code filter, comma separated")),
			aitool.WithStringParam("method",
				aitool.WithParam_Description("HTTP method filter, comma separated")),
			aitool.WithStringParam("url_pattern",
				aitool.WithParam_Description("URL pattern to include")),
			aitool.WithStringParam("content_type",
				aitool.WithParam_Description("Content-Type pattern")),
			aitool.WithStringParam("source_type",
				aitool.WithParam_Description("Source type: mitm/scan")),
			aitool.WithIntegerParam("after_timestamp",
				aitool.WithParam_Description("Start timestamp (Unix seconds)")),
			aitool.WithIntegerParam("before_timestamp",
				aitool.WithParam_Description("End timestamp (Unix seconds)")),
			aitool.WithIntegerParam("min_body_length",
				aitool.WithParam_Description("Minimum response body length")),
			aitool.WithIntegerParam("max_body_length",
				aitool.WithParam_Description("Maximum response body length")),
			aitool.WithIntegerParam("limit",
				aitool.WithParam_Description("Result limit, default 50")),
			aitool.WithStringArrayParam("include_hosts",
				aitool.WithParam_Description("Hosts to include")),
			aitool.WithStringArrayParam("exclude_hosts",
				aitool.WithParam_Description("Hosts to exclude")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Reason for this query")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "reason",
				AINodeId:  "httpflow-query-reason",
			},
		},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			queryType := action.GetString("query_type")
			if queryType == "" {
				return utils.Error("query_httpflow requires 'query_type' parameter")
			}
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			queryType := action.GetString("query_type")
			keyword := action.GetString("keyword")
			keywordType := action.GetString("keyword_type")
			statusCode := action.GetString("status_code")
			method := action.GetString("method")
			urlPattern := action.GetString("url_pattern")
			contentType := action.GetString("content_type")
			sourceType := action.GetString("source_type")
			afterTimestamp := action.GetInt("after_timestamp")
			beforeTimestamp := action.GetInt("before_timestamp")
			minBodyLength := action.GetInt("min_body_length")
			maxBodyLength := action.GetInt("max_body_length")
			limit := action.GetInt("limit")
			includeHosts := action.GetStringSlice("include_hosts")
			excludeHosts := action.GetStringSlice("exclude_hosts")
			reason := action.GetString("reason")

			if limit <= 0 {
				limit = 50
			}
			if limit > 500 {
				limit = 500 // Cap at 500 to prevent context explosion
			}

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Build query request
			req := &ypb.QueryHTTPFlowRequest{
				Pagination: &ypb.Paging{
					Page:    1,
					Limit:   int64(limit),
					OrderBy: "id",
					Order:   "desc",
				},
				Keyword:           keyword,
				KeywordType:       keywordType,
				StatusCode:        statusCode,
				Methods:           method,
				SearchURL:         urlPattern,
				SourceType:        sourceType,
				SearchContentType: contentType,
			}

			// Apply time filters
			if afterTimestamp > 0 {
				req.AfterUpdatedAt = int64(afterTimestamp)
			}
			if beforeTimestamp > 0 {
				req.BeforeUpdatedAt = int64(beforeTimestamp)
			}

			// Apply body length filters
			if minBodyLength > 0 {
				req.AfterBodyLength = int64(minBodyLength)
			}
			if maxBodyLength > 0 {
				req.BeforeBodyLength = int64(maxBodyLength)
			}

			// Apply host filters
			if len(includeHosts) > 0 {
				req.IncludeInUrl = includeHosts
			}
			if len(excludeHosts) > 0 {
				req.ExcludeInUrl = excludeHosts
			}

			// Get database connection
			db := consts.GetGormProjectDatabase()
			if db == nil {
				op.Fail("Cannot access HTTPFlow database: database not initialized")
				return
			}

			// Execute query
			log.Infof("executing HTTPFlow query: type=%s, keyword=%s", queryType, keyword)

			var resultSummary *HTTPFlowQuerySummary
			var queryErr error

			switch queryType {
			case "count":
				resultSummary, queryErr = executeCountQuery(db, req)
			case "aggregate":
				resultSummary, queryErr = executeAggregateQuery(db, req, int(limit))
			case "sample", "search":
				resultSummary, queryErr = executeSampleQuery(db, req, int(limit))
			default:
				resultSummary, queryErr = executeSampleQuery(db, req, int(limit))
			}

			if queryErr != nil {
				log.Errorf("HTTPFlow query failed: %v", queryErr)
				op.Fail(fmt.Sprintf("Query failed: %v", queryErr))
				return
			}

			// Build query parameters string for provenance
			queryParams := fmt.Sprintf("type=%s, keyword=%s, status=%s, method=%s, url=%s, limit=%d",
				queryType, keyword, statusCode, method, urlPattern, limit)
			resultSummary.QueryParams = queryParams

			// Save results to local file
			outputDir := loop.Get("output_directory")
			if outputDir == "" {
				outputDir = os.TempDir()
			}

			queryID := fmt.Sprintf("query_%s_%d", queryType, time.Now().UnixNano())
			resultFile := filepath.Join(outputDir, queryID+".json")

			resultJSON, err := json.MarshalIndent(resultSummary, "", "  ")
			if err != nil {
				log.Errorf("failed to marshal query results: %v", err)
				op.Fail(fmt.Sprintf("Failed to serialize results: %v", err))
				return
			}

			if err := os.WriteFile(resultFile, resultJSON, 0644); err != nil {
				log.Errorf("failed to save query results: %v", err)
				op.Fail(fmt.Sprintf("Failed to save results: %v", err))
				return
			}

			log.Infof("HTTPFlow query results saved to: %s", resultFile)

			// Update query history
			queryHistory := loop.Get("query_history")
			newEntry := fmt.Sprintf("\n[%s] %s\n  Reason: %s\n  Results: %d records, saved to: %s",
				time.Now().Format("15:04:05"), queryParams, reason,
				resultSummary.TotalCount, resultFile)
			loop.Set("query_history", queryHistory+newEntry)

			// Build compact summary for AI context
			summaryText := buildCompactSummary(resultSummary, queryType, reason)

			// Update evidence pack
			evidencePack := loop.Get("evidence_pack")
			newEvidence := fmt.Sprintf("\n\n### 查询证据 [%s]\n%s\n**证据文件**: %s",
				queryID, summaryText, resultFile)
			loop.Set("evidence_pack", evidencePack+newEvidence)

			// Emit summary
			emitter.EmitThoughtStream("httpflow_query", summaryText)
			invoker.AddToTimeline("httpflow_query", summaryText)

			log.Infof("HTTPFlow query completed: %d results", resultSummary.TotalCount)
		},
	)
}

// executeCountQuery performs a count-only query
func executeCountQuery(db interface{}, req *ypb.QueryHTTPFlowRequest) (*HTTPFlowQuerySummary, error) {
	gormDB := consts.GetGormProjectDatabase()
	if gormDB == nil {
		return nil, utils.Error("database not available")
	}

	queryDB := yakit.BuildHTTPFlowQuery(gormDB.Model(&schema.HTTPFlow{}), req)

	var count int64
	if err := queryDB.Count(&count).Error; err != nil {
		return nil, utils.Wrapf(err, "count query failed")
	}

	return &HTTPFlowQuerySummary{
		TotalCount: count,
		TimeRange:  buildTimeRangeString(req),
	}, nil
}

// executeAggregateQuery performs aggregation analysis
func executeAggregateQuery(db interface{}, req *ypb.QueryHTTPFlowRequest, limit int) (*HTTPFlowQuerySummary, error) {
	gormDB := consts.GetGormProjectDatabase()
	if gormDB == nil {
		return nil, utils.Error("database not available")
	}

	queryDB := yakit.FilterHTTPFlow(gormDB.Model(&schema.HTTPFlow{}), req)

	// Get total count
	var totalCount int64
	queryDB.Count(&totalCount)

	summary := &HTTPFlowQuerySummary{
		TotalCount:      totalCount,
		StatusCodeDist:  make(map[string]int),
		MethodDist:      make(map[string]int),
		ContentTypeDist: make(map[string]int),
		TimeRange:       buildTimeRangeString(req),
	}

	// Status code distribution
	type StatusCount struct {
		StatusCode int64
		Count      int64
	}
	var statusCounts []StatusCount
	queryDB.Select("status_code, count(*) as count").Group("status_code").Order("count desc").Limit(20).Find(&statusCounts)
	for _, sc := range statusCounts {
		summary.StatusCodeDist[fmt.Sprintf("%d", sc.StatusCode)] = int(sc.Count)
	}

	// Method distribution
	type MethodCount struct {
		Method string
		Count  int64
	}
	var methodCounts []MethodCount
	gormDB2 := yakit.FilterHTTPFlow(gormDB.Model(&schema.HTTPFlow{}), req)
	gormDB2.Select("method, count(*) as count").Group("method").Order("count desc").Find(&methodCounts)
	for _, mc := range methodCounts {
		summary.MethodDist[mc.Method] = int(mc.Count)
	}

	// Top hosts (extracted from URL)
	type HostCountResult struct {
		RemoteAddr string
		Count      int64
	}
	var hostCounts []HostCountResult
	gormDB3 := yakit.FilterHTTPFlow(gormDB.Model(&schema.HTTPFlow{}), req)
	gormDB3.Select("remote_addr, count(*) as count").Group("remote_addr").Order("count desc").Limit(10).Find(&hostCounts)
	for _, hc := range hostCounts {
		summary.TopHosts = append(summary.TopHosts, HostCount{Host: hc.RemoteAddr, Count: int(hc.Count)})
	}

	// Get sample flows
	var flows []*schema.HTTPFlow
	gormDB4 := yakit.BuildHTTPFlowQuery(gormDB.Model(&schema.HTTPFlow{}), req)
	gormDB4.Limit(10).Find(&flows)

	for _, flow := range flows {
		summary.Samples = append(summary.Samples, HTTPFlowQueryResult{
			ID:         flow.ID,
			URL:        flow.Url,
			Method:     flow.Method,
			StatusCode: flow.StatusCode,
			Path:       flow.Path,
			BodyLength: flow.BodyLength,
			RemoteAddr: flow.RemoteAddr,
			Tags:       flow.Tags,
			SourceType: flow.SourceType,
			IsHTTPS:    flow.IsHTTPS,
			CreatedAt:  flow.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return summary, nil
}

// executeSampleQuery performs sample/search query
func executeSampleQuery(db interface{}, req *ypb.QueryHTTPFlowRequest, limit int) (*HTTPFlowQuerySummary, error) {
	gormDB := consts.GetGormProjectDatabase()
	if gormDB == nil {
		return nil, utils.Error("database not available")
	}

	// Get paging and flows
	paging, flows, err := yakit.QueryHTTPFlow(gormDB, req)
	if err != nil {
		return nil, utils.Wrapf(err, "sample query failed")
	}

	summary := &HTTPFlowQuerySummary{
		TotalCount: int64(paging.TotalRecord),
		TimeRange:  buildTimeRangeString(req),
		Samples:    make([]HTTPFlowQueryResult, 0, len(flows)),
	}

	for _, flow := range flows {
		summary.Samples = append(summary.Samples, HTTPFlowQueryResult{
			ID:            flow.ID,
			URL:           flow.Url,
			Method:        flow.Method,
			StatusCode:    flow.StatusCode,
			ContentType:   flow.ContentType,
			BodyLength:    flow.BodyLength,
			RequestLength: flow.RequestLength,
			Duration:      flow.Duration / int64(time.Millisecond),
			Path:          flow.Path,
			RemoteAddr:    flow.RemoteAddr,
			Tags:          flow.Tags,
			SourceType:    flow.SourceType,
			IsHTTPS:       flow.IsHTTPS,
			CreatedAt:     flow.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// Also collect basic statistics
	summary.StatusCodeDist = make(map[string]int)
	summary.MethodDist = make(map[string]int)
	for _, s := range summary.Samples {
		statusKey := fmt.Sprintf("%d", s.StatusCode)
		summary.StatusCodeDist[statusKey]++
		summary.MethodDist[s.Method]++
	}

	return summary, nil
}

// buildTimeRangeString builds a human-readable time range string
func buildTimeRangeString(req *ypb.QueryHTTPFlowRequest) string {
	var parts []string
	if req.AfterUpdatedAt > 0 {
		parts = append(parts, fmt.Sprintf("after %s", time.Unix(req.AfterUpdatedAt, 0).Format("2006-01-02 15:04")))
	}
	if req.BeforeUpdatedAt > 0 {
		parts = append(parts, fmt.Sprintf("before %s", time.Unix(req.BeforeUpdatedAt, 0).Format("2006-01-02 15:04")))
	}
	if len(parts) == 0 {
		return "all time"
	}
	return strings.Join(parts, ", ")
}

// buildCompactSummary builds a compact summary for AI context
func buildCompactSummary(summary *HTTPFlowQuerySummary, queryType, reason string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**查询目的**: %s\n", reason))
	sb.WriteString(fmt.Sprintf("**总记录数**: %d\n", summary.TotalCount))
	sb.WriteString(fmt.Sprintf("**时间范围**: %s\n", summary.TimeRange))

	if len(summary.StatusCodeDist) > 0 {
		sb.WriteString("**状态码分布**: ")
		var statusParts []string
		for code, count := range summary.StatusCodeDist {
			statusParts = append(statusParts, fmt.Sprintf("%s(%d)", code, count))
		}
		sb.WriteString(strings.Join(statusParts, ", "))
		sb.WriteString("\n")
	}

	if len(summary.MethodDist) > 0 {
		sb.WriteString("**方法分布**: ")
		var methodParts []string
		for method, count := range summary.MethodDist {
			methodParts = append(methodParts, fmt.Sprintf("%s(%d)", method, count))
		}
		sb.WriteString(strings.Join(methodParts, ", "))
		sb.WriteString("\n")
	}

	if len(summary.TopHosts) > 0 {
		sb.WriteString("**Top 主机**: ")
		var hostParts []string
		for _, h := range summary.TopHosts[:min(5, len(summary.TopHosts))] {
			hostParts = append(hostParts, fmt.Sprintf("%s(%d)", h.Host, h.Count))
		}
		sb.WriteString(strings.Join(hostParts, ", "))
		sb.WriteString("\n")
	}

	if len(summary.Samples) > 0 {
		sb.WriteString(fmt.Sprintf("**样本数量**: %d 条\n", len(summary.Samples)))
		sb.WriteString("**样本预览**:\n")
		for i, s := range summary.Samples[:min(5, len(summary.Samples))] {
			sb.WriteString(fmt.Sprintf("  %d. [ID:%d] %s %s -> %d (%dms)\n",
				i+1, s.ID, s.Method, truncateString(s.URL, 60), s.StatusCode, s.Duration))
		}
	}

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
