package loop_http_fuzztest

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	loopHTTPFuzzCompressionThreshold = 40 * 1024
	loopHTTPFuzzCompressionTarget    = 20 * 1024
	loopHTTPFuzzTimelinePreviewSize  = 8 * 1024
	loopHTTPFuzzDetailedResultLimit  = 12
	loopHTTPFuzzFrontendDetailLimit  = 6
	loopHTTPFuzzInterestingTopN      = 6
	loopHTTPFuzzProgressEmitInterval = 2 * time.Second
	modifiedPacketContentField       = "modified_packet_content"
)

type loopHTTPFuzzInterestingSample struct {
	Index           int
	Score           int
	StatusCode      int
	DurationMs      int64
	BodyLength      int
	HiddenIndex     string
	Payloads        []string
	RequestSummary  string
	ResponseSummary string
	RequestDiff     string
	ResponseDigest  string
	ResponseRaw     string
	ResponseDiff    string
}

type loopHTTPFuzzResponseLengthGroup struct {
	BodyLength    int
	Count         int
	StatusCounts  map[int]int
	Sample        loopHTTPFuzzInterestingSample
	HasSample     bool
	BestScore     int
	IsBaseline    bool
	BaselineLabel string
}

type loopHTTPFuzzProcessedResult struct {
	RequestRaw      string
	ResponseRaw     string
	RequestSummary  string
	ResponseSummary string
	RequestDiff     string
	ResponseDigest  string
	HiddenIndex     string
	StatusCode      int
	BodyLength      int
	DurationMs      int64
	Payloads        []string
	Sample          loopHTTPFuzzInterestingSample
}

// loopHTTPFuzzDetailRecord 表示少量结果场景下需要保留的一条详细记录。
// Error 非空时表示该条执行失败；否则使用 Processed 渲染完整请求/响应详情。
type loopHTTPFuzzDetailRecord struct {
	Index     int
	Error     string
	Processed loopHTTPFuzzProcessedResult
}

// loopHTTPFuzzReportData 只保存渲染报告所需的结构化数据。
// 为了控制内存与最终输出体积，详细记录只保留限制内的前几条。
type loopHTTPFuzzReportData struct {
	DetailRecords []loopHTTPFuzzDetailRecord
}

// loopHTTPFuzzOverviewStats 保存固定 overview 需要的聚合统计，
// 以及大样本场景下的分组样本和可疑样本摘要。
type loopHTTPFuzzOverviewStats struct {
	TotalRequests        int
	FailedRequests       int
	SavedHTTPFlowCount   int
	SuccessfulResponses  int
	TotalDurationMs      int64
	MinDurationMs        int64
	MaxDurationMs        int64
	TotalBodyLength      int64
	MinBodyLength        int
	MaxBodyLength        int
	BaselineBodyLength   int
	StatusCounts         map[int]int
	ResponseLengthGroups map[int]*loopHTTPFuzzResponseLengthGroup
	InterestingSamples   []loopHTTPFuzzInterestingSample
}

func newLoopHTTPFuzzOverviewStats() *loopHTTPFuzzOverviewStats {
	return &loopHTTPFuzzOverviewStats{
		MinDurationMs:        -1,
		MinBodyLength:        -1,
		BaselineBodyLength:   -1,
		StatusCounts:         make(map[int]int),
		ResponseLengthGroups: make(map[int]*loopHTTPFuzzResponseLengthGroup),
	}
}

// observeError 只维护总量和失败数，不负责保留详细失败文本。
func (s *loopHTTPFuzzOverviewStats) observeError() {
	if s == nil {
		return
	}
	s.TotalRequests++
	s.FailedRequests++
}

// observeSuccess 更新固定 overview 所需的统计信息。
func (s *loopHTTPFuzzOverviewStats) observeSuccess(statusCode int, durationMs int64, bodyLength int, saved bool) {
	if s == nil {
		return
	}
	s.TotalRequests++
	s.SuccessfulResponses++
	if saved {
		s.SavedHTTPFlowCount++
	}
	s.StatusCounts[statusCode]++
	s.TotalDurationMs += durationMs
	if s.MinDurationMs < 0 || durationMs < s.MinDurationMs {
		s.MinDurationMs = durationMs
	}
	if durationMs > s.MaxDurationMs {
		s.MaxDurationMs = durationMs
	}
	s.TotalBodyLength += int64(bodyLength)
	if s.MinBodyLength < 0 || bodyLength < s.MinBodyLength {
		s.MinBodyLength = bodyLength
	}
	if bodyLength > s.MaxBodyLength {
		s.MaxBodyLength = bodyLength
	}
	if s.BaselineBodyLength < 0 {
		s.BaselineBodyLength = bodyLength
	}
}

// considerInterestingSample 保留 topN 可疑样本，供大数据量分析段渲染。
func (s *loopHTTPFuzzOverviewStats) considerInterestingSample(sample loopHTTPFuzzInterestingSample) {
	if s == nil {
		return
	}
	if sample.Score <= 0 {
		return
	}
	s.InterestingSamples = append(s.InterestingSamples, sample)
	sort.SliceStable(s.InterestingSamples, func(i, j int) bool {
		if s.InterestingSamples[i].Score == s.InterestingSamples[j].Score {
			return s.InterestingSamples[i].Index < s.InterestingSamples[j].Index
		}
		return s.InterestingSamples[i].Score > s.InterestingSamples[j].Score
	})
	if len(s.InterestingSamples) > loopHTTPFuzzInterestingTopN {
		s.InterestingSamples = s.InterestingSamples[:loopHTTPFuzzInterestingTopN]
	}
}

// observeResponseLengthGroup 维护响应长度分组及其代表样本。
func (s *loopHTTPFuzzOverviewStats) observeResponseLengthGroup(sample loopHTTPFuzzInterestingSample) {
	if s == nil {
		return
	}
	group, ok := s.ResponseLengthGroups[sample.BodyLength]
	if !ok {
		group = &loopHTTPFuzzResponseLengthGroup{
			BodyLength:   sample.BodyLength,
			StatusCounts: make(map[int]int),
			BestScore:    -1,
		}
		s.ResponseLengthGroups[sample.BodyLength] = group
	}
	group.Count++
	group.StatusCounts[sample.StatusCode]++
	if !group.HasSample || sample.Score > group.BestScore || (sample.Score == group.BestScore && group.Sample.HiddenIndex == "" && sample.HiddenIndex != "") {
		group.Sample = sample
		group.HasSample = true
		group.BestScore = sample.Score
	}
}

func newLoopHTTPFuzzReportData() *loopHTTPFuzzReportData {
	return &loopHTTPFuzzReportData{}
}

// observeError 只在详细结果上限内记录失败详情，避免大批量错误撑爆报告。
func (r *loopHTTPFuzzReportData) observeError(index int, resultErr error) {
	if r == nil || resultErr == nil || index <= 0 || index > loopHTTPFuzzDetailedResultLimit {
		return
	}
	r.DetailRecords = append(r.DetailRecords, loopHTTPFuzzDetailRecord{
		Index: index,
		Error: resultErr.Error(),
	})
}

// observeDetailedResult 只在详细结果上限内保留完整请求/响应详情。
func (r *loopHTTPFuzzReportData) observeDetailedResult(resultIndex int, processed loopHTTPFuzzProcessedResult) {
	if r == nil || resultIndex <= 0 || resultIndex > loopHTTPFuzzDetailedResultLimit {
		return
	}
	processed.Sample.Index = resultIndex
	r.DetailRecords = append(r.DetailRecords, loopHTTPFuzzDetailRecord{
		Index:     resultIndex,
		Processed: processed,
	})
}

// buildLoopHTTPFuzzOverviewReport 负责渲染固定概况段。
// 这部分内容不会参与压缩，避免每次压缩都重复处理稳定的概况信息。
func buildLoopHTTPFuzzOverviewReport(actionName string, stats *loopHTTPFuzzOverviewStats) string {
	if stats == nil {
		return ""
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("=== Fuzz Overview for %s ===\n", actionName))
	out.WriteString(fmt.Sprintf("Total Requests: %d\n", stats.TotalRequests))
	out.WriteString(fmt.Sprintf("Failed Requests: %d\n", stats.FailedRequests))
	out.WriteString(fmt.Sprintf("Saved HTTPFlows: %d\n", stats.SavedHTTPFlowCount))

	if len(stats.StatusCounts) > 0 {
		out.WriteString("Status Distribution:\n")
		statuses := make([]int, 0, len(stats.StatusCounts))
		for statusCode := range stats.StatusCounts {
			statuses = append(statuses, statusCode)
		}
		sort.SliceStable(statuses, func(i, j int) bool {
			if stats.StatusCounts[statuses[i]] == stats.StatusCounts[statuses[j]] {
				return statuses[i] < statuses[j]
			}
			return stats.StatusCounts[statuses[i]] > stats.StatusCounts[statuses[j]]
		})
		for _, statusCode := range statuses {
			out.WriteString(fmt.Sprintf("- %s: %d\n", formatLoopHTTPFuzzStatusCode(statusCode), stats.StatusCounts[statusCode]))
		}
	}

	if stats.SuccessfulResponses > 0 {
		avgDuration := stats.TotalDurationMs / int64(stats.SuccessfulResponses)
		avgBodyLength := stats.TotalBodyLength / int64(stats.SuccessfulResponses)
		out.WriteString(fmt.Sprintf("Duration Stats: avg=%d ms min=%d ms max=%d ms\n", avgDuration, stats.MinDurationMs, stats.MaxDurationMs))
		out.WriteString(fmt.Sprintf("Response Body Stats: avg=%d bytes min=%d bytes max=%d bytes\n", avgBodyLength, stats.MinBodyLength, stats.MaxBodyLength))
	}

	if lengthPreview := stats.responseLengthPreview(6); lengthPreview != "" {
		out.WriteString(fmt.Sprintf("Response Length Overview: %s\n", lengthPreview))
	}

	return strings.TrimSpace(out.String())
}

// buildLoopHTTPFuzzLargeRunAnalysisReport 负责渲染大样本场景的分析段：
// 主要包含长度分组代表样本，以及 topN 可疑样本摘要。
func buildLoopHTTPFuzzLargeRunAnalysisReport(stats *loopHTTPFuzzOverviewStats) string {
	if stats == nil {
		return ""
	}

	var out strings.Builder
	out.WriteString("=== Large-Run Analysis ===\n")
	wroteContent := false
	if stats.shouldUseResponseLengthAnalysis() && len(stats.ResponseLengthGroups) > 0 {
		wroteContent = true
		out.WriteString("Response Length Groups:\n")
		for _, group := range stats.sortedResponseLengthGroups() {
			out.WriteString(fmt.Sprintf("- %d bytes: %d responses", group.BodyLength, group.Count))
			if group.IsBaseline {
				out.WriteString(" [baseline]")
			}
			if statusPreview := formatLoopHTTPFuzzTopStatusCounts(group.StatusCounts, 4); statusPreview != "" {
				out.WriteString(fmt.Sprintf(" (statuses: %s)", statusPreview))
			}
			out.WriteByte('\n')
			if group.HasSample {
				if group.Sample.HiddenIndex != "" {
					out.WriteString(fmt.Sprintf("  Sample HTTPFlow: %s\n", group.Sample.HiddenIndex))
				}
				if len(group.Sample.Payloads) > 0 {
					out.WriteString(fmt.Sprintf("  Sample Payloads: %s\n", shrinkLoopHTTPFuzzList(group.Sample.Payloads, 4, 200)))
				}
				if group.Sample.RequestSummary != "" {
					out.WriteString(fmt.Sprintf("  Sample Request Summary: %s\n", group.Sample.RequestSummary))
				}
				if group.IsBaseline && strings.TrimSpace(group.BaselineLabel) != "" {
					out.WriteString(fmt.Sprintf("  %s\n", group.BaselineLabel))
				}
				if strings.TrimSpace(group.Sample.ResponseDiff) != "" {
					out.WriteString("  Sample Diff From Baseline:\n")
					out.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(group.Sample.ResponseDiff, 400), "    "))
					out.WriteByte('\n')
				}
			}
		}
	}

	if len(stats.InterestingSamples) > 0 {
		if wroteContent {
			out.WriteByte('\n')
		}
		wroteContent = true
		out.WriteString("Interesting Samples:\n")
		for idx, sample := range stats.InterestingSamples {
			out.WriteString(fmt.Sprintf("%d. score=%d status=%s duration=%d ms body=%d bytes\n", idx+1, sample.Score, formatLoopHTTPFuzzStatusCode(sample.StatusCode), sample.DurationMs, sample.BodyLength))
			if sample.HiddenIndex != "" {
				out.WriteString(fmt.Sprintf("   HTTPFlow: %s\n", sample.HiddenIndex))
			}
			if len(sample.Payloads) > 0 {
				out.WriteString(fmt.Sprintf("   Payloads: %s\n", shrinkLoopHTTPFuzzList(sample.Payloads, 4, 200)))
			}
			if sample.RequestSummary != "" {
				out.WriteString(fmt.Sprintf("   Request Summary: %s\n", sample.RequestSummary))
			}
			if sample.ResponseSummary != "" {
				out.WriteString(fmt.Sprintf("   Response Summary: %s\n", sample.ResponseSummary))
			}
			if sample.RequestDiff != "" {
				out.WriteString("   Request Changes:\n")
				out.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(sample.RequestDiff, 240), "     "))
				out.WriteByte('\n')
			}
			if sample.ResponseDigest != "" {
				out.WriteString("   Response Digest:\n")
				out.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(sample.ResponseDigest, 240), "     "))
				out.WriteByte('\n')
			}
		}
	}

	if !wroteContent {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// buildLoopHTTPFuzzDetailedPacketReport 负责渲染小样本场景的详细包结果。
// 这里只读取循环中预先裁剪过的 detail records，不再额外扩容。
func buildLoopHTTPFuzzDetailedPacketReport(reportData *loopHTTPFuzzReportData) string {
	if reportData == nil || len(reportData.DetailRecords) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("=== Detailed Packet Results ===\n")
	for _, detail := range reportData.DetailRecords {
		if strings.TrimSpace(detail.Error) != "" {
			out.WriteString(fmt.Sprintf("\n--- Result %d ---\n", detail.Index))
			out.WriteString(fmt.Sprintf("Error: %s\n", detail.Error))
			continue
		}
		appendLoopHTTPFuzzRenderedDetailedResult(&out, detail.Index, detail.Processed)
	}
	return strings.TrimSpace(out.String())
}

// appendLoopHTTPFuzzRenderedDetailedResult 负责把一条结构化结果渲染成详细包文本。
func appendLoopHTTPFuzzRenderedDetailedResult(out *strings.Builder, resultIndex int, processed loopHTTPFuzzProcessedResult) {
	if out == nil {
		return
	}
	out.WriteString(fmt.Sprintf("\n--- Result %d ---\n", resultIndex))
	out.WriteString(fmt.Sprintf("Payload: %v\n", processed.Payloads))
	out.WriteString(fmt.Sprintf("Duration: %d ms\n", processed.DurationMs))
	out.WriteString(fmt.Sprintf("Status: %s\n", formatLoopHTTPFuzzStatusCode(processed.StatusCode)))
	out.WriteString(fmt.Sprintf("Request Summary: %s\n", processed.RequestSummary))
	out.WriteString(fmt.Sprintf("Response Summary: %s\n", processed.ResponseSummary))
	if processed.HiddenIndex != "" {
		out.WriteString(fmt.Sprintf("Saved HTTPFlow: %s\n", processed.HiddenIndex))
	}
	out.WriteString(fmt.Sprintf("Request Changes:\n%s\n", processed.RequestDiff))
	out.WriteString(fmt.Sprintf("Response Summary:\n%s\n", processed.ResponseDigest))
	out.WriteString("Request Packet:\n")
	out.WriteString(processed.RequestRaw)
	out.WriteString("\nResponse Packet:\n")
	out.WriteString(processed.ResponseRaw)
	out.WriteRune('\n')
}

// buildLoopHTTPFuzzAnalysisSection 根据本轮结果规模选择渲染详细包段或大样本分析段。
func buildLoopHTTPFuzzAnalysisSection(stats *loopHTTPFuzzOverviewStats, reportData *loopHTTPFuzzReportData) string {
	if stats == nil {
		return ""
	}
	if stats.TotalRequests == 0 {
		return "=== Detailed Packet Results ===\n(no results returned by the fuzz execution)"
	}
	if stats.TotalRequests <= loopHTTPFuzzDetailedResultLimit {
		return buildLoopHTTPFuzzDetailedPacketReport(reportData)
	}
	return buildLoopHTTPFuzzLargeRunAnalysisReport(stats)
}

// joinLoopHTTPFuzzReportSections 把多个非空段落按空行拼接成最终文档。
func joinLoopHTTPFuzzReportSections(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if trimmed := strings.TrimSpace(section); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n\n")
}

// buildLoopHTTPFuzzVerificationPayload 在最终反馈文档基础上补齐代表性 HTTPFlow，
// 供 VerifyUserSatisfaction 使用，避免验证输入缺少定位样本。
func buildLoopHTTPFuzzVerificationPayload(feedbackResult, representativeHiddenIndex string) string {
	payload := strings.TrimSpace(feedbackResult)
	if payload == "" {
		if strings.TrimSpace(representativeHiddenIndex) == "" {
			return ""
		}
		return fmt.Sprintf("Representative HTTPFlow: %s", strings.TrimSpace(representativeHiddenIndex))
	}
	if strings.TrimSpace(representativeHiddenIndex) != "" && !strings.Contains(payload, representativeHiddenIndex) {
		payload += fmt.Sprintf("\n\nRepresentative HTTPFlow: %s", strings.TrimSpace(representativeHiddenIndex))
	}
	return payload
}

func (s *loopHTTPFuzzOverviewStats) sortedResponseLengthGroups() []*loopHTTPFuzzResponseLengthGroup {
	if s == nil || len(s.ResponseLengthGroups) == 0 {
		return nil
	}
	groups := make([]*loopHTTPFuzzResponseLengthGroup, 0, len(s.ResponseLengthGroups))
	for _, group := range s.ResponseLengthGroups {
		if group != nil {
			groups = append(groups, group)
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].BodyLength < groups[j].BodyLength
		}
		return groups[i].Count > groups[j].Count
	})
	return groups
}

func (s *loopHTTPFuzzOverviewStats) shouldUseResponseLengthAnalysis() bool {
	if s == nil || len(s.ResponseLengthGroups) < 2 {
		return false
	}
	groups := s.sortedResponseLengthGroups()
	if len(groups) < 2 {
		return false
	}

	dominantCount := groups[0].Count
	switch {
	case s.TotalRequests > loopHTTPFuzzDetailedResultLimit:
		return true
	case s.SuccessfulResponses >= 15:
		return true
	case dominantCount >= 10:
		return true
	default:
		return false
	}
}

func (s *loopHTTPFuzzOverviewStats) finalizeResponseLengthGroups() {
	if s == nil || len(s.ResponseLengthGroups) == 0 {
		return
	}
	groups := s.sortedResponseLengthGroups()
	if len(groups) == 0 {
		return
	}
	baselineGroup := groups[0]
	for _, group := range groups {
		if group == nil {
			continue
		}
		if group.Count > baselineGroup.Count {
			baselineGroup = group
			continue
		}
		if group.Count == baselineGroup.Count && group.BodyLength == s.BaselineBodyLength {
			baselineGroup = group
		}
	}
	if baselineGroup == nil {
		return
	}

	for _, group := range groups {
		if group == nil {
			continue
		}
		group.IsBaseline = group.BodyLength == baselineGroup.BodyLength
		group.BaselineLabel = ""
		if group.IsBaseline {
			group.BaselineLabel = fmt.Sprintf("Baseline group selected by dominant body length: %d bytes (%d responses).", group.BodyLength, group.Count)
			s.BaselineBodyLength = group.BodyLength
			group.Sample.ResponseDiff = "  (baseline representative response)"
			continue
		}
		group.Sample.ResponseDiff = buildLoopHTTPFuzzResponseDiffFromBaseline(baselineGroup.Sample.ResponseRaw, group.Sample.ResponseRaw)
	}
}

type loopHTTPFuzzThrottleEmitter struct {
	loop       *reactloops.ReActLoop
	taskID     string
	actionName string
	throttle   func(func())
}

func (r *loopHTTPFuzzThrottleEmitter) allowEmitDetailHttpFlow(resultIndex int) bool {
	return r != nil && resultIndex > 0 && resultIndex <= loopHTTPFuzzFrontendDetailLimit
}

func (r *loopHTTPFuzzThrottleEmitter) emitProgress(stats *loopHTTPFuzzOverviewStats, lastStatusCode int, force bool) {
	if r == nil || r.loop == nil || strings.TrimSpace(r.taskID) == "" || stats == nil {
		return
	}
	if !force && stats.TotalRequests <= loopHTTPFuzzFrontendDetailLimit {
		return
	}

	snapshot := buildLoopHTTPFuzzProgressSnapshot(r.actionName, stats, lastStatusCode, force)
	if strings.TrimSpace(snapshot) == "" {
		return
	}
	emit := func() {
		emitFuzzStage(r.loop, r.taskID, snapshot)
	}
	if force || r.throttle == nil {
		emit()
		return
	}
	r.throttle(emit)
}

func buildLoopHTTPFuzzProgressSnapshot(actionName string, stats *loopHTTPFuzzOverviewStats, lastStatusCode int, finished bool) string {
	if stats == nil || stats.TotalRequests <= 0 {
		return ""
	}

	stateLabel := "执行进度"
	if finished {
		stateLabel = "执行完成"
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s：%s 已处理 %d 个请求", stateLabel, actionName, stats.TotalRequests))
	if stats.SuccessfulResponses > 0 || stats.FailedRequests > 0 {
		out.WriteString(fmt.Sprintf("，成功 %d，失败 %d", stats.SuccessfulResponses, stats.FailedRequests))
	}
	if stats.SavedHTTPFlowCount > 0 {
		out.WriteString(fmt.Sprintf("，已落库 %d 条 HTTPFlow", stats.SavedHTTPFlowCount))
	}
	if lastStatusCode > 0 {
		out.WriteString(fmt.Sprintf("，最近状态 %s", formatLoopHTTPFuzzStatusCode(lastStatusCode)))
	}
	if stats.SuccessfulResponses > 0 {
		out.WriteString(fmt.Sprintf("，平均响应 %d ms", stats.TotalDurationMs/int64(stats.SuccessfulResponses)))
	}
	if statusPreview := formatLoopHTTPFuzzTopStatusCounts(stats.StatusCounts, 3); statusPreview != "" {
		out.WriteString(fmt.Sprintf("，状态分布 %s", statusPreview))
	}
	if stats.shouldUseResponseLengthAnalysis() {
		if lengthPreview := stats.responseLengthPreview(3); lengthPreview != "" {
			out.WriteString(fmt.Sprintf("，长度分布 %s", lengthPreview))
		}
	}
	if len(stats.InterestingSamples) > 0 {
		out.WriteString(fmt.Sprintf("，可疑样本 %d 个", len(stats.InterestingSamples)))
	}
	out.WriteString("。")
	return out.String()
}

func (s *loopHTTPFuzzOverviewStats) responseLengthPreview(maxItems int) string {
	if s == nil || len(s.ResponseLengthGroups) == 0 || maxItems <= 0 {
		return ""
	}
	groups := s.sortedResponseLengthGroups()
	if len(groups) > maxItems {
		groups = groups[:maxItems]
	}
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, fmt.Sprintf("%dB=%d", group.BodyLength, group.Count))
	}
	return strings.Join(parts, ", ")
}

func buildLoopHTTPFuzzProcessedResult(resultIndex int, result *mutate.HttpResult, originalRequest string, baselineBodyLength int) loopHTTPFuzzProcessedResult {
	requestRaw := string(result.RequestRaw)
	responseRaw := string(result.ResponseRaw)
	requestURL, requestSummary := buildHTTPRequestStreamSummary(requestRaw, result.Request.TLS != nil)
	responseSummary := buildHTTPResponseStreamSummary(responseRaw, requestURL)
	requestDiff := compareRequests(originalRequest, requestRaw)
	responseDigest := summarizeResponse(responseRaw)
	statusCode := getStatusFromResponse(responseRaw)
	_, responseBody := lowhttp.SplitHTTPPacketFast([]byte(responseRaw))
	bodyLength := len(responseBody)
	hiddenIndex := ""
	if result.LowhttpResponse != nil {
		hiddenIndex = strings.TrimSpace(result.LowhttpResponse.HiddenIndex)
	}
	score := scoreLoopHTTPFuzzInterestingSample(statusCode, result.DurationMs, bodyLength, baselineBodyLength, responseRaw)

	return loopHTTPFuzzProcessedResult{
		RequestRaw:      requestRaw,
		ResponseRaw:     responseRaw,
		RequestSummary:  requestSummary,
		ResponseSummary: responseSummary,
		RequestDiff:     requestDiff,
		ResponseDigest:  responseDigest,
		HiddenIndex:     hiddenIndex,
		StatusCode:      statusCode,
		BodyLength:      bodyLength,
		DurationMs:      result.DurationMs,
		Payloads:        append([]string(nil), result.Payloads...),
		Sample: loopHTTPFuzzInterestingSample{
			Index:           resultIndex,
			Score:           score,
			StatusCode:      statusCode,
			DurationMs:      result.DurationMs,
			BodyLength:      bodyLength,
			HiddenIndex:     hiddenIndex,
			Payloads:        append([]string(nil), result.Payloads...),
			RequestSummary:  requestSummary,
			ResponseSummary: responseSummary,
			RequestDiff:     requestDiff,
			ResponseDigest:  responseDigest,
			ResponseRaw:     responseRaw,
		},
	}
}

func syncLoopHTTPFuzzLastResultState(loop *reactloops.ReActLoop, processed loopHTTPFuzzProcessedResult, runtimeID string, progressReporter *loopHTTPFuzzThrottleEmitter, resultIndex int) {
	if loop == nil {
		return
	}
	loop.Set("last_request", processed.RequestRaw)
	loop.Set("last_request_summary", processed.RequestSummary)
	loop.Set("last_response", processed.ResponseRaw)
	loop.Set("last_response_summary", processed.ResponseSummary)
	if strings.TrimSpace(processed.HiddenIndex) != "" {
		loop.Set("last_httpflow_hidden_index", processed.HiddenIndex)
	}

	if progressReporter == nil || !progressReporter.allowEmitDetailHttpFlow(resultIndex) || strings.TrimSpace(progressReporter.taskID) == "" {
		return
	}
	emitPacketSummary(loop, progressReporter.taskID, progressReporter.actionName, resultIndex, "request", processed.RequestSummary)
	emitPacketSummary(loop, progressReporter.taskID, progressReporter.actionName, resultIndex, "response", processed.ResponseSummary)
	if runtimeID != "" && strings.TrimSpace(processed.HiddenIndex) != "" {
		loop.GetEmitter().EmitYakitHTTPFlow(runtimeID, processed.HiddenIndex)
	}
}

func shouldUpdateLoopHTTPFuzzRepresentative(processed loopHTTPFuzzProcessedResult, bestRepresentativeScore int, representativeHiddenIndex string, representativeRequest string) bool {
	if representativeRequest == "" {
		return true
	}
	if processed.Sample.Score > bestRepresentativeScore {
		return true
	}
	return processed.Sample.Score == bestRepresentativeScore && representativeHiddenIndex == "" && processed.HiddenIndex != ""
}

func newLoopFuzzRequest(taskCtx context.Context, runtime aicommon.AIInvokeRuntime, rawPacket []byte, isHTTPS bool) (*mutate.FuzzHTTPRequest, error) {
	opts := []mutate.BuildFuzzHTTPRequestOption{
		mutate.OptHTTPS(isHTTPS),
		mutate.OptSource(loopHTTPFuzztestHTTPSource),
	}
	if runtime != nil {
		if cfg := runtime.GetConfig(); cfg != nil {
			if runtimeID := cfg.GetRuntimeId(); runtimeID != "" {
				opts = append(opts, mutate.OptRuntimeId(runtimeID))
			}
		}
	}
	if taskCtx != nil {
		opts = append(opts, mutate.OptContext(taskCtx))
	}
	return mutate.NewFuzzHTTPRequest(rawPacket, opts...)
}

func storeLoopFuzzRequestState(loop *reactloops.ReActLoop, fuzzReq *mutate.FuzzHTTPRequest, requestRaw []byte, isHTTPS bool) {
	_, originalSummary := buildHTTPRequestStreamSummary(string(requestRaw), isHTTPS)
	state := loopHTTPFuzzRequestState{
		RawRequest: string(requestRaw),
		IsHTTPS:    isHTTPS,
		Summary:    originalSummary,
		Version:    1,
	}
	loop.Set("fuzz_request", fuzzReq)
	loop.Set(loopHTTPFuzzRequestStateKey, state)
	loop.Set(loopHTTPFuzzRequestVersionKey, state.Version)
	loop.Set(loopHTTPFuzzRequestSourceActionKey, "")
	loop.Set(loopHTTPFuzzRequestChangeReasonKey, "")
	loop.Set("original_request", string(requestRaw))
	loop.Set("original_request_summary", originalSummary)
	loop.Set("current_request", string(requestRaw))
	loop.Set("current_request_summary", originalSummary)
	loop.Set("previous_request", "")
	loop.Set("previous_request_summary", "")
	loop.Set("request_change_summary", "")
	loop.Set("request_modification_reason", "")
	loop.Set("request_review_decision", "")
	loop.Set("is_https", utils.InterfaceToString(isHTTPS))
	loop.Set("bootstrap_source", "")
	resetLoopHTTPFuzzExecutionState(loop)
}

func getCurrentRequestRaw(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if state := getLoopHTTPFuzzRequestState(loop); state != nil && strings.TrimSpace(state.RawRequest) != "" {
		return state.RawRequest
	}
	currentRequest := strings.TrimSpace(loop.Get("current_request"))
	if currentRequest != "" {
		return currentRequest
	}
	return strings.TrimSpace(loop.Get("original_request"))
}

func getCurrentRequestSummary(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	if state := getLoopHTTPFuzzRequestState(loop); state != nil && strings.TrimSpace(state.Summary) != "" {
		return state.Summary
	}
	summary := strings.TrimSpace(loop.Get("current_request_summary"))
	if summary != "" {
		return summary
	}
	return strings.TrimSpace(loop.Get("original_request_summary"))
}

func setLoopCurrentRequestState(loop *reactloops.ReActLoop, fuzzReq *mutate.FuzzHTTPRequest, requestRaw []byte, isHTTPS bool) {
	if loop == nil {
		return
	}
	_, summary := buildHTTPRequestStreamSummary(string(requestRaw), isHTTPS)
	version := 1
	if currentState := getLoopHTTPFuzzRequestState(loop); currentState != nil {
		version = max(currentState.Version, 1)
	}
	loop.Set("fuzz_request", fuzzReq)
	state := loopHTTPFuzzRequestState{
		RawRequest:   string(requestRaw),
		IsHTTPS:      isHTTPS,
		Summary:      summary,
		Version:      version,
		SourceAction: loop.Get(loopHTTPFuzzRequestSourceActionKey),
		ChangeReason: loop.Get(loopHTTPFuzzRequestChangeReasonKey),
	}
	loop.Set(loopHTTPFuzzRequestStateKey, state)
	loop.Set(loopHTTPFuzzRequestVersionKey, state.Version)
	loop.Set("current_request", string(requestRaw))
	loop.Set("current_request_summary", summary)
	loop.Set("is_https", utils.InterfaceToString(isHTTPS))
}

func buildRequestModificationFeedback(previousRequest, modifiedRequest []byte, isHTTPS bool, reason, reviewDecision string) string {
	previousSummary := "(none)"
	modifiedSummary := "(none)"
	if len(previousRequest) > 0 {
		_, previousSummary = buildHTTPRequestStreamSummary(string(previousRequest), isHTTPS)
	}
	if len(modifiedRequest) > 0 {
		_, modifiedSummary = buildHTTPRequestStreamSummary(string(modifiedRequest), isHTTPS)
	}

	var out strings.Builder
	out.WriteString("HTTP 数据包修改完成。\n\n")
	if strings.TrimSpace(reason) != "" {
		out.WriteString("=== 修改原因 ===\n")
		out.WriteString(strings.TrimSpace(reason))
		out.WriteString("\n\n")
	}
	out.WriteString("=== 审核结果 ===\n")
	if strings.TrimSpace(reviewDecision) == "" {
		reviewDecision = "auto_applied"
	}
	out.WriteString(reviewDecision)
	out.WriteString("\n\n")
	out.WriteString("=== 修改前摘要 ===\n")
	out.WriteString(previousSummary)
	out.WriteString("\n\n")
	out.WriteString("=== 修改后摘要 ===\n")
	out.WriteString(modifiedSummary)
	out.WriteString("\n\n")
	out.WriteString("=== Merge 变化 ===\n")
	out.WriteString(compareRequests(string(previousRequest), string(modifiedRequest)))
	out.WriteString("\n")
	out.WriteString("\n=== 当前生效数据包 ===\n")
	out.WriteString(string(modifiedRequest))
	out.WriteString("\n")
	return out.String()
}

func buildReviewDecisionLabel(decision string) string {
	switch strings.TrimSpace(strings.ToLower(decision)) {
	case "approved_by_user":
		return "已人工确认并应用新数据包"
	case "rejected_by_user":
		return "人工审核拒绝，保留旧数据包"
	default:
		return "无需人工审核，已直接应用新数据包"
	}
}

func reviewSuggestionApproved(suggestion string) bool {
	s := strings.TrimSpace(strings.ToLower(suggestion))
	if s == "" {
		return false
	}
	if strings.Contains(s, "reject") || strings.Contains(s, "拒绝") || strings.Contains(s, "保留旧") {
		return false
	}
	return strings.Contains(s, "accept") || strings.Contains(s, "approve") || strings.Contains(s, "同意") || strings.Contains(s, "确认") || strings.Contains(s, "使用新") || strings.Contains(s, "应用新")
}

func getLoopTaskContext(loop *reactloops.ReActLoop) context.Context {
	if loop == nil {
		return nil
	}
	task := loop.GetCurrentTask()
	if task == nil {
		return nil
	}
	return task.GetContext()
}

// getFuzzRequest 从循环上下文里读取当前生效的 FuzzHTTPRequest。
func getFuzzRequest(loop *reactloops.ReActLoop) (*mutate.FuzzHTTPRequest, error) {
	fuzzReqAny := loop.GetVariable("fuzz_request")
	if fuzzReqAny == nil {
		return nil, utils.Error("fuzz_request not found in loop context. Auto bootstrap from user input may have failed; provide a URL/raw HTTP packet or call set_http_request first")
	}
	fuzzReq, ok := fuzzReqAny.(*mutate.FuzzHTTPRequest)
	if !ok {
		return nil, utils.Error("fuzz_request is not a valid FuzzHTTPRequest")
	}
	return fuzzReq, nil
}

// executeFuzzAndCompare 是 HTTP fuzz 循环的核心执行入口。
// 它的职责分三步：
// 1. 执行 fuzz，并在循环中只收集结构化统计与受限详细记录。
// 2. 结束后根据数据规模渲染 overview + analysis 文档。
// 3. 在需要时只压缩 analysis 段，再把最终反馈送去满意度验证。
func executeFuzzAndCompare(loop *reactloops.ReActLoop, fuzzResult mutate.FuzzHTTPRequestIf, actionName string, paramSummary string) (string, *aicommon.VerifySatisfactionResult, error) {
	isHttpsStr := loop.Get("is_https")
	isHttps := isHttpsStr == "true"
	task := loop.GetCurrentTask()
	taskID := ""
	streamTaskID := ""
	var taskCtx context.Context
	if task != nil {
		taskID = task.GetId()
		streamTaskID = taskID
		if streamTaskID == "" {
			streamTaskID = task.GetIndex()
		}
		taskCtx = task.GetContext()
	}
	runtimeID := ""
	invoker := loop.GetInvoker()
	if invoker != nil {
		if cfg := invoker.GetConfig(); cfg != nil {
			runtimeID = cfg.GetRuntimeId()
		}
	}
	if taskCtx == nil {
		taskCtx = context.Background()
	}

	// 进度事件仍然边执行边发，但正文报告改成最后统一渲染。
	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("开始执行 %s，HTTPFlow 会落库并保留完整请求/响应。", actionName))
	progressEmitter := &loopHTTPFuzzThrottleEmitter{
		loop:       loop,
		taskID:     streamTaskID,
		actionName: actionName,
		throttle:   utils.NewThrottle(loopHTTPFuzzProgressEmitInterval.Seconds()),
	}

	// 真实执行阶段只保留结构化数据，不在循环里直接拼接长文本报告。
	execOpts := []mutate.HttpPoolConfigOption{
		mutate.WithPoolOpt_Https(isHttps),
		mutate.WithPoolOpt_RuntimeId(runtimeID),
		mutate.WithPoolOpt_Source(loopHTTPFuzztestHTTPSource),
		mutate.WithPoolOpt_SaveHTTPFlow(true),
	}
	if taskCtx != nil {
		execOpts = append(execOpts, mutate.WithPoolOpt_Context(taskCtx))
	}
	resultCh, err := fuzzResult.Exec(execOpts...)
	if err != nil {
		return "", nil, utils.Errorf("failed to execute fuzz request: %v", err)
	}

	originalRequest := getCurrentRequestRaw(loop)
	overview := newLoopHTTPFuzzOverviewStats()
	reportData := newLoopHTTPFuzzReportData()
	representativeRequest := ""
	representativeResponse := ""
	representativeHiddenIndex := ""
	collectedPayloads := make([]string, 0)
	representativeStatusCode := 0
	bestRepresentativeScore := -1

	for result := range resultCh {
		// 结果编号按 1 开始，便于报告、前端事件和样本索引统一。
		resultIndex := overview.TotalRequests + 1
		if result.Error != nil {
			overview.observeError()
			reportData.observeError(resultIndex, result.Error)
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 第 %d 个测试请求执行失败：%v", actionName, resultIndex, result.Error))
			progressEmitter.emitProgress(overview, 0, false)
			continue
		}

		processed := buildLoopHTTPFuzzProcessedResult(resultIndex, result, originalRequest, overview.BaselineBodyLength)
		collectedPayloads = append(collectedPayloads, processed.Payloads...)
		syncLoopHTTPFuzzLastResultState(loop, processed, runtimeID, progressEmitter, resultIndex)

		overview.observeSuccess(processed.StatusCode, processed.DurationMs, processed.BodyLength, processed.HiddenIndex != "")
		overview.considerInterestingSample(processed.Sample)
		overview.observeResponseLengthGroup(processed.Sample)

		if shouldUpdateLoopHTTPFuzzRepresentative(processed, bestRepresentativeScore, representativeHiddenIndex, representativeRequest) {
			bestRepresentativeScore = processed.Sample.Score
			representativeRequest = processed.RequestRaw
			representativeResponse = processed.ResponseRaw
			representativeHiddenIndex = processed.HiddenIndex
			representativeStatusCode = processed.StatusCode
		}

		reportData.observeDetailedResult(resultIndex, processed)

		progressEmitter.emitProgress(overview, processed.StatusCode, false)
	}

	overview.finalizeResponseLengthGroups()
	progressEmitter.emitProgress(overview, representativeStatusCode, true)

	if representativeRequest != "" || representativeResponse != "" {
		loop.Set("representative_request", representativeRequest)
		loop.Set("representative_response", representativeResponse)
		loop.Set("representative_httpflow_hidden_index", representativeHiddenIndex)
	}

	overviewReport := buildLoopHTTPFuzzOverviewReport(actionName, overview)
	analysisSection := buildLoopHTTPFuzzAnalysisSection(overview, reportData)

	loop.Set("diff_result_compressed", "")

	if len(strings.TrimSpace(analysisSection)) > loopHTTPFuzzCompressionThreshold && invoker != nil {
		emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 结果超过 40KB，开始生成压缩报告并检查所有数据包。", actionName))
		compressionTarget := buildFuzzCompressionTarget(loop, actionName)
		compressed, compressErr := invoker.CompressLongTextWithDestination(taskCtx, analysisSection, compressionTarget, loopHTTPFuzzCompressionTarget)
		if compressErr != nil {
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告失败，回退到原始测试结果。", actionName))
		} else {
			analysisSection = buildCompressedAnalysisSection(compressed, representativeRequest, representativeResponse, representativeHiddenIndex)
			loop.Set("diff_result_compressed", analysisSection)
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告完成，准备验证是否达到安全测试目标。", actionName))
		}
	}

	fullReport := joinLoopHTTPFuzzReportSections(overviewReport, analysisSection)
	loop.Set("diff_result_full", fullReport)
	loop.Set("diff_result_analysis", fullReport)
	feedbackResult := fullReport

	verificationPayload := buildLoopHTTPFuzzVerificationPayload(feedbackResult, representativeHiddenIndex)
	verifyResult, verificationText, err := verifyFuzzCompletion(loop, taskCtx, streamTaskID, actionName, verificationPayload, representativeHiddenIndex)
	if err != nil {
		return "", nil, err
	}
	if verificationText != "" {
		feedbackResult += "\n\n" + verificationText
		loop.Set("verification_result", verificationText)
	}

	actionResultSummary := fmt.Sprintf("共执行 %d 次测试，保存 %d 条 HTTPFlow。代表性响应状态：%s", overview.TotalRequests, overview.SavedHTTPFlowCount, formatLoopHTTPFuzzStatusCode(representativeStatusCode))
	if representativeStatusCode == 0 {
		actionResultSummary = fmt.Sprintf("共执行 %d 次测试，保存 %d 条 HTTPFlow。", overview.TotalRequests, overview.SavedHTTPFlowCount)
	}
	verificationSummary := buildLoopHTTPFuzzVerificationSummary(verifyResult)
	actionRecord := recordLoopHTTPFuzzAction(loop, actionName, paramSummary, actionResultSummary, verificationSummary, representativeHiddenIndex, collectedPayloads)
	actionFeedback := buildLoopHTTPFuzzActionFeedback(actionRecord)
	feedbackResult = actionFeedback + "\n\n" + feedbackResult

	loop.Set("diff_result", feedbackResult)
	persistLoopHTTPFuzzSessionContext(loop, actionName)

	return feedbackResult, verifyResult, nil
}

// buildFuzzCompressionTarget 只描述“可压缩分析段”的压缩目标，不包含固定 overview。
func buildFuzzCompressionTarget(loop *reactloops.ReActLoop, actionName string) string {
	task := loop.GetCurrentTask()
	userInput := ""
	if task != nil {
		userInput = strings.TrimSpace(task.GetUserInput())
	}
	if userInput == "" {
		userInput = "HTTP 安全模糊测试"
	}
	return fmt.Sprintf("用户正在执行 HTTP 安全模糊测试，当前步骤是 %s。你将收到的是可压缩分析段，不包含固定概况。你的核心目标是分析漏洞，而不是复述数据包。请覆盖所有请求/响应对，重点归纳疑似漏洞类型、触发依据、差异模式、可复现代表性数据包、以及下一步验证动作。原始目标：%s", actionName, userInput)
}

// buildCompressedAnalysisSection 只渲染压缩后的分析段。
// 固定 overview 会在外层通过 joinLoopHTTPFuzzReportSections 单独拼接。
func buildCompressedAnalysisSection(compressed, representativeRequest, representativeResponse, representativeHiddenIndex string) string {
	var out strings.Builder
	out.WriteString("=== Compressed Fuzz Analysis ===\n")
	out.WriteString(compressed)
	if representativeRequest != "" || representativeResponse != "" {
		out.WriteString("\n\n=== Representative Packet For Follow-Up Testing ===\n")
		if representativeHiddenIndex != "" {
			out.WriteString(fmt.Sprintf("HTTPFlow: %s\n", representativeHiddenIndex))
		}
		if representativeRequest != "" {
			out.WriteString("Request:\n")
			out.WriteString(representativeRequest)
			out.WriteRune('\n')
		}
		if representativeResponse != "" {
			out.WriteString("Response:\n")
			out.WriteString(representativeResponse)
		}
	}
	return out.String()
}

func verifyFuzzCompletion(loop *reactloops.ReActLoop, taskCtx context.Context, streamTaskID, actionName, verificationPayload, representativeHiddenIndex string) (*aicommon.VerifySatisfactionResult, string, error) {
	invoker := loop.GetInvoker()
	task := loop.GetCurrentTask()
	if invoker == nil || task == nil {
		return nil, "", nil
	}

	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 测试结果已准备完成，开始验证是否达到当前安全测试目标。", actionName))
	verifyResult, err := invoker.VerifyUserSatisfaction(taskCtx, task.GetUserInput(), true, verificationPayload)
	if err != nil {
		return nil, "", utils.Wrap(err, "verify fuzz completion")
	}

	var verifySummary strings.Builder
	verifySummary.WriteString("=== Verification ===\n")
	verifySummary.WriteString(fmt.Sprintf("Satisfied: %v\n", verifyResult.Satisfied))
	verifySummary.WriteString(fmt.Sprintf("Reasoning: %s\n", verifyResult.Reasoning))
	if next := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); next != "" {
		verifySummary.WriteString(fmt.Sprintf("Next Steps: %s\n", next))
	}
	if representativeHiddenIndex != "" {
		verifySummary.WriteString(fmt.Sprintf("Representative HTTPFlow: %s\n", representativeHiddenIndex))
	}

	state := "未完成，需要继续测试。"
	if verifyResult.Satisfied {
		state = "已达到当前安全测试目标。"
	}
	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 目标验证完成：%s", actionName, state))

	return verifyResult, verifySummary.String(), nil
}

func emitFuzzStage(loop *reactloops.ReActLoop, taskID, msg string) {
	if loop == nil || taskID == "" || loop.GetEmitter() == nil || strings.TrimSpace(msg) == "" {
		return
	}
	_, _ = loop.GetEmitter().EmitDefaultStreamEvent("thought", bytes.NewBufferString(msg), taskID)
}

func emitPacketSummary(loop *reactloops.ReActLoop, taskID, actionName string, index int, stage, summary string) {
	if loop == nil || taskID == "" || loop.GetEmitter() == nil || strings.TrimSpace(summary) == "" {
		return
	}
	message := fmt.Sprintf("[%s #%d][%s] %s", actionName, index, stage, summary)
	_, _ = loop.GetEmitter().EmitDefaultStreamEvent("http_flow", bytes.NewBufferString(message), taskID)
}

func buildHTTPRequestStreamSummary(requestRaw string, isHTTPS bool) (string, string) {
	requestURL := extractRequestURL(requestRaw, isHTTPS)
	_, body := lowhttp.SplitHTTPPacketFast([]byte(requestRaw))
	return requestURL, fmt.Sprintf("URL: %s BODY: [(%d) bytes]", requestURL, len(body))
}

func buildHTTPResponseStreamSummary(responseRaw, requestURL string) string {
	status := getStatusFromResponse(responseRaw)
	_, body := lowhttp.SplitHTTPPacketFast([]byte(responseRaw))
	if status == 0 {
		return fmt.Sprintf("URL: %s BODY: [(%d) bytes]", requestURL, len(body))
	}
	return fmt.Sprintf("URL: %s STATUS: %d BODY: [(%d) bytes]", requestURL, status, len(body))
}

func extractRequestURL(requestRaw string, isHTTPS bool) string {
	urlObj, err := lowhttp.ExtractURLFromHTTPRequestRaw([]byte(requestRaw), isHTTPS)
	if err == nil && urlObj != nil && urlObj.String() != "" {
		return urlObj.String()
	}

	scheme := "http"
	if isHTTPS {
		scheme = "https"
	}
	if fallback := strings.TrimSpace(lowhttp.GetUrlFromHTTPRequest(scheme, []byte(requestRaw))); fallback != "" {
		return fallback
	}
	return "(unknown url)"
}

func buildFuzzTimelineSummary(summary string) string {
	if len(summary) <= loopHTTPFuzzTimelinePreviewSize {
		return summary
	}
	return utils.ShrinkTextBlock(summary, loopHTTPFuzzTimelinePreviewSize)
}

func applyFuzzVerificationOutcome(loop *reactloops.ReActLoop, operator *reactloops.LoopActionHandlerOperator, diffResult string, verifyResult *aicommon.VerifySatisfactionResult) {
	markLoopHTTPFuzzLastAction(loop, getLoopHTTPFuzzLastAction(loop))
	if verifyResult == nil {
		operator.Feedback(diffResult)
		return
	}

	loop.PushSatisfactionRecordWithCompletedTaskIndex(
		verifyResult.Satisfied,
		verifyResult.Reasoning,
		verifyResult.CompletedTaskIndex,
		verifyResult.NextMovements,
		verifyResult.Evidence,
		verifyResult.OutputFiles,
		verifyResult.EvidenceOps,
	)

	if verifyResult.Satisfied {
		operator.Exit()
		return
	}

	operator.Feedback(diffResult)
	operator.Continue()
}

func buildLoopHTTPFuzzVerificationSummary(verifyResult *aicommon.VerifySatisfactionResult) string {
	if verifyResult == nil {
		return ""
	}
	state := "未达到当前目标"
	if verifyResult.Satisfied {
		state = "已达到当前目标"
	}
	if strings.TrimSpace(verifyResult.Reasoning) == "" {
		return state
	}
	return fmt.Sprintf("%s；%s", state, strings.TrimSpace(verifyResult.Reasoning))
}

// compareRequests 用于对比两个 HTTP 请求，返回逐行差异。
func compareRequests(original, modified string) string {
	originalLines := strings.Split(strings.TrimSpace(original), "\n")
	modifiedLines := strings.Split(strings.TrimSpace(modified), "\n")

	var diff strings.Builder
	maxLines := max(len(originalLines), len(modifiedLines))

	for i := 0; i < maxLines; i++ {
		origLine := ""
		modLine := ""
		if i < len(originalLines) {
			origLine = strings.TrimSpace(originalLines[i])
		}
		if i < len(modifiedLines) {
			modLine = strings.TrimSpace(modifiedLines[i])
		}

		if origLine != modLine {
			if origLine != "" {
				diff.WriteString(fmt.Sprintf("  - %s\n", origLine))
			}
			if modLine != "" {
				diff.WriteString(fmt.Sprintf("  + %s\n", modLine))
			}
		}
	}

	if diff.Len() == 0 {
		return "  (no changes)"
	}
	return diff.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
