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
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
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

type loopHTTPFuzzAggregateStats struct {
	TotalRequests        int
	FailedRequests       int
	SavedHTTPFlowCount   int
	DetailedResultsShown int
	OmittedDetails       int
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

func newLoopHTTPFuzzAggregateStats() *loopHTTPFuzzAggregateStats {
	return &loopHTTPFuzzAggregateStats{
		MinDurationMs:        -1,
		MinBodyLength:        -1,
		BaselineBodyLength:   -1,
		StatusCounts:         make(map[int]int),
		ResponseLengthGroups: make(map[int]*loopHTTPFuzzResponseLengthGroup),
	}
}

func (s *loopHTTPFuzzAggregateStats) allowDetailedResult() bool {
	return s != nil && s.DetailedResultsShown < loopHTTPFuzzDetailedResultLimit
}

func (s *loopHTTPFuzzAggregateStats) markDetailedResultWritten() {
	if s == nil {
		return
	}
	s.DetailedResultsShown++
}

func (s *loopHTTPFuzzAggregateStats) markDetailedResultOmitted() {
	if s == nil {
		return
	}
	s.OmittedDetails++
}

func (s *loopHTTPFuzzAggregateStats) observeError() {
	if s == nil {
		return
	}
	s.TotalRequests++
	s.FailedRequests++
}

func (s *loopHTTPFuzzAggregateStats) observeSuccess(statusCode int, durationMs int64, bodyLength int, saved bool) {
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

func (s *loopHTTPFuzzAggregateStats) considerInterestingSample(sample loopHTTPFuzzInterestingSample) {
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

func (s *loopHTTPFuzzAggregateStats) observeResponseLengthGroup(sample loopHTTPFuzzInterestingSample) {
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

func scoreLoopHTTPFuzzInterestingSample(statusCode int, durationMs int64, bodyLength int, baselineBodyLength int, responseRaw string) int {
	score := 0
	switch {
	case statusCode >= 500:
		score += 90
	case statusCode >= 400:
		score += 45
	case statusCode >= 300:
		score += 20
	}

	if baselineBodyLength >= 0 {
		delta := abs(bodyLength - baselineBodyLength)
		if baselineBodyLength == 0 {
			if delta > 0 {
				score += 35
			}
		} else if delta > baselineBodyLength/2 {
			score += 35
		} else if delta > baselineBodyLength/4 {
			score += 18
		}
	}

	switch {
	case durationMs >= 3000:
		score += 40
	case durationMs >= 1000:
		score += 20
	case durationMs >= 500:
		score += 10
	}

	responseLower := strings.ToLower(responseRaw)
	for _, keyword := range []string{
		"sql", "syntax error", "exception", "stack trace", "traceback",
		"unauthorized", "forbidden", "access denied", "permission denied",
		"welcome", "login success", "token", "debug",
	} {
		if strings.Contains(responseLower, keyword) {
			score += 25
			break
		}
	}

	return score
}

func writeLoopHTTPFuzzDetailTruncationNotice(diffSummary, analysisSummary *strings.Builder, omitted int) {
	if omitted <= 0 {
		return
	}
	msg := fmt.Sprintf("\n--- Detailed Results Truncated ---\nDetailed request/response logs were limited to the first %d results. The remaining %d results are summarized in the aggregate report below because all HTTP flows have already been saved to the database.\n", loopHTTPFuzzDetailedResultLimit, omitted)
	diffSummary.WriteString(msg)
	analysisSummary.WriteString(msg)
}

func buildLoopHTTPFuzzAggregateReport(actionName string, stats *loopHTTPFuzzAggregateStats) string {
	if stats == nil {
		return ""
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("=== Aggregate Summary for %s ===\n", actionName))
	out.WriteString(fmt.Sprintf("Total Requests: %d\n", stats.TotalRequests))
	out.WriteString(fmt.Sprintf("Failed Requests: %d\n", stats.FailedRequests))
	out.WriteString(fmt.Sprintf("Saved HTTPFlows: %d\n", stats.SavedHTTPFlowCount))
	if stats.OmittedDetails > 0 {
		out.WriteString(fmt.Sprintf("Detailed Results Shown: %d (omitted %d additional detailed entries; full traffic remains stored in HTTPFlow)\n", stats.DetailedResultsShown, stats.OmittedDetails))
	}

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

	if len(stats.ResponseLengthGroups) > 0 {
		out.WriteString("Response Length Groups:\n")
		for _, group := range buildLoopHTTPFuzzResponseLengthGroups(stats) {
			out.WriteString(fmt.Sprintf("- %d bytes: %d responses", group.BodyLength, group.Count))
			if group.IsBaseline {
				out.WriteString(" [baseline]")
			}
			if statusPreview := buildLoopHTTPFuzzStatusPreviewFromCounts(group.StatusCounts, 4); statusPreview != "" {
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

	return strings.TrimSpace(out.String())
}

func buildLoopHTTPFuzzVerificationOverview(actionName string, stats *loopHTTPFuzzAggregateStats, representativeHiddenIndex string) string {
	aggregateReport := strings.TrimSpace(buildLoopHTTPFuzzAggregateReport(actionName, stats))
	if aggregateReport == "" && strings.TrimSpace(representativeHiddenIndex) == "" {
		return ""
	}

	var out strings.Builder
	out.WriteString("=== Fuzz Overview For Next-Step Analysis ===\n")
	if aggregateReport != "" {
		out.WriteString(aggregateReport)
		out.WriteByte('\n')
	}
	if strings.TrimSpace(representativeHiddenIndex) != "" {
		out.WriteString(fmt.Sprintf("Representative HTTPFlow: %s\n", representativeHiddenIndex))
	}
	return strings.TrimSpace(out.String())
}

func formatLoopHTTPFuzzStatusCode(statusCode int) string {
	if statusCode <= 0 {
		return "(no status code)"
	}
	return fmt.Sprintf("%d", statusCode)
}

func buildLoopHTTPFuzzResponseLengthGroups(stats *loopHTTPFuzzAggregateStats) []*loopHTTPFuzzResponseLengthGroup {
	if stats == nil || len(stats.ResponseLengthGroups) == 0 {
		return nil
	}
	groups := make([]*loopHTTPFuzzResponseLengthGroup, 0, len(stats.ResponseLengthGroups))
	for _, group := range stats.ResponseLengthGroups {
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

func finalizeLoopHTTPFuzzResponseLengthGroups(stats *loopHTTPFuzzAggregateStats) {
	if stats == nil || len(stats.ResponseLengthGroups) == 0 {
		return
	}
	groups := buildLoopHTTPFuzzResponseLengthGroups(stats)
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
		if group.Count == baselineGroup.Count && group.BodyLength == stats.BaselineBodyLength {
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
			stats.BaselineBodyLength = group.BodyLength
			group.Sample.ResponseDiff = "  (baseline representative response)"
			continue
		}
		group.Sample.ResponseDiff = buildLoopHTTPFuzzResponseDiffFromBaseline(baselineGroup.Sample.ResponseRaw, group.Sample.ResponseRaw)
	}
}

func buildLoopHTTPFuzzResponseDiffFromBaseline(baselineResponseRaw, sampleResponseRaw string) string {
	baselineResponseRaw = strings.TrimSpace(baselineResponseRaw)
	sampleResponseRaw = strings.TrimSpace(sampleResponseRaw)
	if baselineResponseRaw == "" || sampleResponseRaw == "" {
		return ""
	}
	if baselineResponseRaw == sampleResponseRaw {
		return "  (same as baseline representative response)"
	}

	_, baselineBody := lowhttp.SplitHTTPPacketFast([]byte(baselineResponseRaw))
	_, sampleBody := lowhttp.SplitHTTPPacketFast([]byte(sampleResponseRaw))
	left := string(baselineBody)
	right := string(sampleBody)
	if strings.TrimSpace(left) == "" && strings.TrimSpace(right) == "" {
		left = baselineResponseRaw
		right = sampleResponseRaw
	}

	diffText, err := yakdiff.DiffToString(left, right)
	if err == nil && strings.TrimSpace(diffText) != "" {
		return strings.TrimSpace(diffText)
	}
	return compareRequests(left, right)
}

func buildLoopHTTPFuzzStatusPreviewFromCounts(counts map[int]int, maxItems int) string {
	if len(counts) == 0 || maxItems <= 0 {
		return ""
	}
	statuses := make([]int, 0, len(counts))
	for statusCode := range counts {
		statuses = append(statuses, statusCode)
	}
	sort.SliceStable(statuses, func(i, j int) bool {
		if counts[statuses[i]] == counts[statuses[j]] {
			return statuses[i] < statuses[j]
		}
		return counts[statuses[i]] > counts[statuses[j]]
	})
	if len(statuses) > maxItems {
		statuses = statuses[:maxItems]
	}
	parts := make([]string, 0, len(statuses))
	for _, statusCode := range statuses {
		parts = append(parts, fmt.Sprintf("%s=%d", formatLoopHTTPFuzzStatusCode(statusCode), counts[statusCode]))
	}
	return strings.Join(parts, ", ")
}

type loopHTTPFuzzProgressReporter struct {
	loop       *reactloops.ReActLoop
	taskID     string
	actionName string
	lastEmitAt time.Time
}

func newLoopHTTPFuzzProgressReporter(loop *reactloops.ReActLoop, taskID, actionName string) *loopHTTPFuzzProgressReporter {
	return &loopHTTPFuzzProgressReporter{
		loop:       loop,
		taskID:     taskID,
		actionName: actionName,
	}
}

func (r *loopHTTPFuzzProgressReporter) allowDetailedFrontendEvent(resultIndex int) bool {
	return r != nil && resultIndex > 0 && resultIndex <= loopHTTPFuzzFrontendDetailLimit
}

func (r *loopHTTPFuzzProgressReporter) maybeEmit(stats *loopHTTPFuzzAggregateStats, lastStatusCode int, force bool) {
	if r == nil || r.loop == nil || strings.TrimSpace(r.taskID) == "" || stats == nil {
		return
	}
	if !force && stats.TotalRequests <= loopHTTPFuzzFrontendDetailLimit {
		return
	}

	now := time.Now()
	if !force && !r.lastEmitAt.IsZero() && now.Sub(r.lastEmitAt) < loopHTTPFuzzProgressEmitInterval {
		return
	}

	snapshot := buildLoopHTTPFuzzProgressSnapshot(r.actionName, stats, lastStatusCode, force)
	if strings.TrimSpace(snapshot) == "" {
		return
	}
	emitFuzzStage(r.loop, r.taskID, snapshot)
	r.lastEmitAt = now
}

func buildLoopHTTPFuzzProgressSnapshot(actionName string, stats *loopHTTPFuzzAggregateStats, lastStatusCode int, finished bool) string {
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
	if statusPreview := buildLoopHTTPFuzzStatusPreview(stats, 3); statusPreview != "" {
		out.WriteString(fmt.Sprintf("，状态分布 %s", statusPreview))
	}
	if lengthPreview := buildLoopHTTPFuzzResponseLengthPreview(stats, 3); lengthPreview != "" {
		out.WriteString(fmt.Sprintf("，长度分布 %s", lengthPreview))
	}
	if len(stats.InterestingSamples) > 0 {
		out.WriteString(fmt.Sprintf("，可疑样本 %d 个", len(stats.InterestingSamples)))
	}
	out.WriteString("。")
	return out.String()
}

func buildLoopHTTPFuzzStatusPreview(stats *loopHTTPFuzzAggregateStats, maxItems int) string {
	if stats == nil || maxItems <= 0 {
		return ""
	}
	return buildLoopHTTPFuzzStatusPreviewFromCounts(stats.StatusCounts, maxItems)
}

func buildLoopHTTPFuzzResponseLengthPreview(stats *loopHTTPFuzzAggregateStats, maxItems int) string {
	if stats == nil || len(stats.ResponseLengthGroups) == 0 || maxItems <= 0 {
		return ""
	}
	groups := buildLoopHTTPFuzzResponseLengthGroups(stats)
	if len(groups) > maxItems {
		groups = groups[:maxItems]
	}
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, fmt.Sprintf("%dB=%d", group.BodyLength, group.Count))
	}
	return strings.Join(parts, ", ")
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

// getFuzzRequest retrieves the FuzzHTTPRequest from loop context
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

// executeFuzzAndCompare executes the fuzz request, keeps the full request/response archive,
// emits compact user-visible summaries, and asks AI whether fuzzing should continue.
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

	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("开始执行 %s，HTTPFlow 会落库并保留完整请求/响应。", actionName))
	progressReporter := newLoopHTTPFuzzProgressReporter(loop, streamTaskID, actionName)

	// Execute the fuzz request
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

	var diffSummary strings.Builder
	diffSummary.WriteString(fmt.Sprintf("=== Fuzz Results for %s ===\n", actionName))
	var analysisSummary strings.Builder
	analysisSummary.WriteString(fmt.Sprintf("=== 漏洞测试分析：%s ===\n", actionName))

	originalRequest := getCurrentRequestRaw(loop)
	stats := newLoopHTTPFuzzAggregateStats()
	resultCount := 0
	representativeRequest := ""
	representativeResponse := ""
	representativeHiddenIndex := ""
	collectedPayloads := make([]string, 0)
	representativeStatusCode := 0
	bestRepresentativeScore := -1

	for result := range resultCh {
		resultCount++
		if result.Error != nil {
			stats.observeError()
			if stats.allowDetailedResult() {
				errMsg := fmt.Sprintf("\n--- Result %d ---\nError: %v\n", resultCount, result.Error)
				diffSummary.WriteString(errMsg)
				analysisSummary.WriteString(errMsg)
				stats.markDetailedResultWritten()
			} else {
				stats.markDetailedResultOmitted()
			}
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 第 %d 个测试请求执行失败：%v", actionName, resultCount, result.Error))
			progressReporter.maybeEmit(stats, 0, false)
			continue
		}

		// Get request and response
		requestRaw := string(result.RequestRaw)
		responseRaw := string(result.ResponseRaw)
		requestURL, requestStreamSummary := buildHTTPRequestStreamSummary(requestRaw, isHttps)
		responseStreamSummary := buildHTTPResponseStreamSummary(responseRaw, requestURL)
		collectedPayloads = append(collectedPayloads, result.Payloads...)

		if progressReporter.allowDetailedFrontendEvent(resultCount) && streamTaskID != "" {
			emitPacketSummary(loop, streamTaskID, actionName, resultCount, "request", requestStreamSummary)
			emitPacketSummary(loop, streamTaskID, actionName, resultCount, "response", responseStreamSummary)
		}

		hiddenIndex := ""
		if result.LowhttpResponse != nil {
			hiddenIndex = strings.TrimSpace(result.LowhttpResponse.HiddenIndex)
			if hiddenIndex != "" {
				loop.Set("last_httpflow_hidden_index", hiddenIndex)
				if progressReporter.allowDetailedFrontendEvent(resultCount) && runtimeID != "" {
					loop.GetEmitter().EmitYakitHTTPFlow(runtimeID, hiddenIndex)
				}
			}
		}

		// Compare request differences
		requestDiff := compareRequests(originalRequest, requestRaw)
		responseSummary := summarizeResponse(responseRaw)
		statusCode := getStatusFromResponse(responseRaw)
		_, responseBody := lowhttp.SplitHTTPPacketFast([]byte(responseRaw))
		bodyLength := len(responseBody)
		loop.Set("last_request", requestRaw)
		loop.Set("last_request_summary", requestStreamSummary)
		loop.Set("last_response", responseRaw)
		loop.Set("last_response_summary", responseStreamSummary)

		stats.observeSuccess(statusCode, result.DurationMs, bodyLength, hiddenIndex != "")
		sampleScore := scoreLoopHTTPFuzzInterestingSample(statusCode, result.DurationMs, bodyLength, stats.BaselineBodyLength, responseRaw)
		sample := loopHTTPFuzzInterestingSample{
			Index:           resultCount,
			Score:           sampleScore,
			StatusCode:      statusCode,
			DurationMs:      result.DurationMs,
			BodyLength:      bodyLength,
			HiddenIndex:     hiddenIndex,
			Payloads:        append([]string(nil), result.Payloads...),
			RequestSummary:  requestStreamSummary,
			ResponseSummary: responseStreamSummary,
			RequestDiff:     requestDiff,
			ResponseDigest:  responseSummary,
			ResponseRaw:     responseRaw,
		}
		stats.considerInterestingSample(sample)
		stats.observeResponseLengthGroup(sample)

		if representativeRequest == "" || sampleScore > bestRepresentativeScore || (sampleScore == bestRepresentativeScore && representativeHiddenIndex == "" && hiddenIndex != "") {
			bestRepresentativeScore = sampleScore
			representativeRequest = requestRaw
			representativeResponse = responseRaw
			representativeHiddenIndex = hiddenIndex
			representativeStatusCode = statusCode
		}

		if stats.allowDetailedResult() {
			diffSummary.WriteString(fmt.Sprintf("\n--- Result %d ---\n", resultCount))
			analysisSummary.WriteString(fmt.Sprintf("\n--- Result %d ---\n", resultCount))
			diffSummary.WriteString(fmt.Sprintf("Payload: %v\n", result.Payloads))
			analysisSummary.WriteString(fmt.Sprintf("Payload: %v\n", result.Payloads))
			diffSummary.WriteString(fmt.Sprintf("Duration: %d ms\n", result.DurationMs))
			analysisSummary.WriteString(fmt.Sprintf("Duration: %d ms\n", result.DurationMs))
			diffSummary.WriteString(fmt.Sprintf("Status: %s\n", formatLoopHTTPFuzzStatusCode(statusCode)))
			analysisSummary.WriteString(fmt.Sprintf("Status: %s\n", formatLoopHTTPFuzzStatusCode(statusCode)))
			diffSummary.WriteString(fmt.Sprintf("Request Summary: %s\n", requestStreamSummary))
			analysisSummary.WriteString(fmt.Sprintf("Request Summary: %s\n", requestStreamSummary))
			diffSummary.WriteString(fmt.Sprintf("Response Summary: %s\n", responseStreamSummary))
			analysisSummary.WriteString(fmt.Sprintf("Response Summary: %s\n", responseStreamSummary))
			if hiddenIndex != "" {
				diffSummary.WriteString(fmt.Sprintf("Saved HTTPFlow: %s\n", hiddenIndex))
				analysisSummary.WriteString(fmt.Sprintf("Saved HTTPFlow: %s\n", hiddenIndex))
			}
			diffSummary.WriteString(fmt.Sprintf("Request Changes:\n%s\n", requestDiff))
			analysisSummary.WriteString(fmt.Sprintf("Request Changes:\n%s\n", requestDiff))
			diffSummary.WriteString(fmt.Sprintf("Response Summary:\n%s\n", responseSummary))
			analysisSummary.WriteString(fmt.Sprintf("Response Summary:\n%s\n", responseSummary))
			diffSummary.WriteString("Request Packet:\n")
			diffSummary.WriteString(requestRaw)
			diffSummary.WriteString("\nResponse Packet:\n")
			diffSummary.WriteString(responseRaw)
			diffSummary.WriteRune('\n')
			stats.markDetailedResultWritten()
		} else {
			stats.markDetailedResultOmitted()
		}

		progressReporter.maybeEmit(stats, statusCode, false)
	}

	finalizeLoopHTTPFuzzResponseLengthGroups(stats)
	progressReporter.maybeEmit(stats, representativeStatusCode, true)

	if representativeRequest != "" || representativeResponse != "" {
		loop.Set("representative_request", representativeRequest)
		loop.Set("representative_response", representativeResponse)
		loop.Set("representative_httpflow_hidden_index", representativeHiddenIndex)
	}

	if resultCount == 0 {
		diffSummary.WriteString("\n(no results returned by the fuzz execution)\n")
		analysisSummary.WriteString("\n(no results returned by the fuzz execution)\n")
	}
	if stats.SavedHTTPFlowCount > 0 {
		diffSummary.WriteString(fmt.Sprintf("\nSaved %d HTTP flow records to database and linked them to the current AI runtime/task event context.\n", stats.SavedHTTPFlowCount))
		analysisSummary.WriteString(fmt.Sprintf("\nSaved %d HTTP flow records to database and linked them to the current AI runtime/task event context.\n", stats.SavedHTTPFlowCount))
	}
	if stats.OmittedDetails > 0 {
		writeLoopHTTPFuzzDetailTruncationNotice(&diffSummary, &analysisSummary, stats.OmittedDetails)
	}
	aggregateReport := buildLoopHTTPFuzzAggregateReport(actionName, stats)
	if aggregateReport != "" {
		diffSummary.WriteString("\n\n")
		diffSummary.WriteString(aggregateReport)
		diffSummary.WriteRune('\n')
		analysisSummary.WriteString("\n\n")
		analysisSummary.WriteString(aggregateReport)
		analysisSummary.WriteRune('\n')
	}

	fullDiffResult := diffSummary.String()
	analysisResult := analysisSummary.String()
	loop.Set("diff_result_full", fullDiffResult)
	loop.Set("diff_result_compressed", analysisResult)
	verificationOverview := buildLoopHTTPFuzzVerificationOverview(actionName, stats, representativeHiddenIndex)

	feedbackResult := analysisResult
	compressedResult := ""
	if len(fullDiffResult) > loopHTTPFuzzCompressionThreshold && invoker != nil {
		emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 结果超过 40KB，开始生成压缩报告并检查所有数据包。", actionName))
		compressionTarget := buildFuzzCompressionTarget(loop, actionName)
		compressed, compressErr := invoker.CompressLongTextWithDestination(taskCtx, fullDiffResult, compressionTarget, loopHTTPFuzzCompressionTarget)
		if compressErr != nil {
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告失败，回退到原始测试结果。", actionName))
		} else {
			compressedResult = compressed
			feedbackResult = buildCompressedFeedbackReport(verificationOverview, compressed, representativeRequest, representativeResponse, representativeHiddenIndex)
			loop.Set("diff_result_compressed", feedbackResult)
			emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 压缩报告完成，准备验证是否达到安全测试目标。", actionName))
		}
	}

	verifyResult, verificationText, err := verifyFuzzCompletion(loop, taskCtx, streamTaskID, actionName, verificationOverview, feedbackResult, compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex)
	if err != nil {
		return "", nil, err
	}
	if verificationText != "" {
		feedbackResult += "\n\n" + verificationText
		loop.Set("verification_result", verificationText)
	}

	actionResultSummary := fmt.Sprintf("共执行 %d 次测试，保存 %d 条 HTTPFlow。代表性响应状态：%s", resultCount, stats.SavedHTTPFlowCount, formatLoopHTTPFuzzStatusCode(representativeStatusCode))
	if representativeStatusCode == 0 {
		actionResultSummary = fmt.Sprintf("共执行 %d 次测试，保存 %d 条 HTTPFlow。", resultCount, stats.SavedHTTPFlowCount)
	}
	verificationSummary := buildLoopHTTPFuzzVerificationSummary(verifyResult)
	actionRecord := recordLoopHTTPFuzzAction(loop, actionName, paramSummary, actionResultSummary, verificationSummary, representativeHiddenIndex, collectedPayloads)
	actionFeedback := buildLoopHTTPFuzzActionFeedback(actionRecord)
	feedbackResult = actionFeedback + "\n\n" + feedbackResult

	loop.Set("diff_result", feedbackResult)
	persistLoopHTTPFuzzSessionContext(loop, actionName)

	return feedbackResult, verifyResult, nil
}

func buildFuzzCompressionTarget(loop *reactloops.ReActLoop, actionName string) string {
	task := loop.GetCurrentTask()
	userInput := ""
	if task != nil {
		userInput = strings.TrimSpace(task.GetUserInput())
	}
	if userInput == "" {
		userInput = "HTTP 安全模糊测试"
	}
	return fmt.Sprintf("用户正在执行 HTTP 安全模糊测试，当前步骤是 %s。你的核心目标是分析漏洞，而不是复述数据包。请覆盖所有请求/响应对，重点归纳疑似漏洞类型、触发依据、差异模式、可复现代表性数据包、以及下一步验证动作。原始目标：%s", actionName, userInput)
}

func buildCompressedFeedbackReport(overview, compressed, representativeRequest, representativeResponse, representativeHiddenIndex string) string {
	var out strings.Builder
	if strings.TrimSpace(overview) != "" {
		out.WriteString(strings.TrimSpace(overview))
		out.WriteString("\n\n")
	}
	out.WriteString("=== Compressed Fuzz Report ===\n")
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

func verifyFuzzCompletion(loop *reactloops.ReActLoop, taskCtx context.Context, streamTaskID, actionName, verificationOverview, feedbackResult, compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex string) (*aicommon.VerifySatisfactionResult, string, error) {
	invoker := loop.GetInvoker()
	task := loop.GetCurrentTask()
	if invoker == nil || task == nil {
		return nil, "", nil
	}

	payload := feedbackResult
	if compressedResult != "" {
		payload = buildCompressedFeedbackReport(verificationOverview, compressedResult, representativeRequest, representativeResponse, representativeHiddenIndex)
	} else if strings.TrimSpace(verificationOverview) != "" && !strings.Contains(feedbackResult, verificationOverview) {
		payload = verificationOverview + "\n\n" + feedbackResult
	}

	emitFuzzStage(loop, streamTaskID, fmt.Sprintf("%s 测试结果已准备完成，开始验证是否达到当前安全测试目标。", actionName))
	verifyResult, err := invoker.VerifyUserSatisfaction(taskCtx, task.GetUserInput(), true, payload)
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

// compareRequests compares two HTTP requests and returns the differences
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

// summarizeResponse creates a summary of the HTTP response
func summarizeResponse(response string) string {
	if response == "" {
		return "  (empty response)"
	}

	_, body := lowhttp.SplitHTTPPacketFast([]byte(response))
	statusCode := getStatusFromResponse(response)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("  Status Code: %s\n", formatLoopHTTPFuzzStatusCode(statusCode)))

	// Get content length
	contentLength := len(body)
	summary.WriteString(fmt.Sprintf("  Content-Length: %d bytes\n", contentLength))

	// Show first part of body if not too long
	if contentLength > 0 {
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		summary.WriteString(fmt.Sprintf("  Body Preview: %s\n", bodyPreview))
	}

	return summary.String()
}

// getStatusFromResponse extracts status code from response
func getStatusFromResponse(response string) int {
	statusCode := lowhttp.ExtractStatusCodeFromResponse([]byte(response))
	return statusCode
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
