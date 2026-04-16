package loop_http_fuzztest

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildLoopHTTPFuzzOverviewReport_SummarizesLargeRuns(t *testing.T) {
	stats := newLoopHTTPFuzzOverviewStats()
	baselineSample := loopHTTPFuzzInterestingSample{
		Index:          1,
		Score:          5,
		StatusCode:     200,
		DurationMs:     120,
		BodyLength:     24,
		HiddenIndex:    "flow-1",
		RequestSummary: "URL: https://example.test/login BODY: [(32) bytes]",
		ResponseRaw:    "HTTP/1.1 200 OK\r\nContent-Length: 24\r\n\r\nhello user",
		ResponseDigest: "  Status Code: 200\n  Content-Length: 24 bytes\n",
	}
	sample := loopHTTPFuzzInterestingSample{
		Index:           2,
		Score:           70,
		StatusCode:      401,
		DurationMs:      1450,
		BodyLength:      0,
		HiddenIndex:     "flow-2",
		Payloads:        []string{"{{payload(pass_top25)}}"},
		RequestSummary:  "URL: https://example.test/login BODY: [(32) bytes]",
		ResponseSummary: "URL: https://example.test/login STATUS: 401 BODY: [(0) bytes]",
		RequestDiff:     "  + password={{payload(pass_top25)}}",
		ResponseDigest:  "  Status Code: 401\n  Content-Length: 0 bytes\n",
		ResponseRaw:     "HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n",
	}

	for i := 0; i < 9; i++ {
		stats.observeSuccess(200, 120, 24, true)
		stats.observeResponseLengthGroup(baselineSample)
	}
	for i := 0; i < 4; i++ {
		stats.observeSuccess(401, 1450, 0, true)
		stats.observeResponseLengthGroup(sample)
	}
	stats.considerInterestingSample(sample)
	stats.finalizeResponseLengthGroups()

	report := buildLoopHTTPFuzzOverviewReport("fuzz_body", stats)
	require.Contains(t, report, "=== Fuzz Overview for fuzz_body ===")
	require.Contains(t, report, "Total Requests: 13")
	require.Contains(t, report, "Saved HTTPFlows: 13")
	require.Contains(t, report, "Status Distribution:")
	require.Contains(t, report, "401: 4")
	require.Contains(t, report, "Response Length Overview: 24B=9, 0B=4")
}

func TestBuildCompressedAnalysisSection_RendersRepresentativePacket(t *testing.T) {
	report := buildCompressedAnalysisSection(
		"compressed body",
		"GET /login HTTP/1.1\r\nHost: example.test\r\n\r\n",
		"HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n",
		"flow-9",
	)

	require.Contains(t, report, "=== Compressed Fuzz Analysis ===")
	require.Contains(t, report, "compressed body")
	require.Contains(t, report, "Representative Packet For Follow-Up Testing")
}

func TestBuildLoopHTTPFuzzVerificationPayload_DoesNotDuplicateAggregateOverview(t *testing.T) {
	payload := buildLoopHTTPFuzzVerificationPayload(
		"=== Fuzz Overview for fuzz_body ===\nTotal Requests: 12\nSaved HTTPFlows: 12",
		"flow-9",
	)

	require.Equal(t, 1, strings.Count(payload, "=== Fuzz Overview for fuzz_body ==="))
	require.Contains(t, payload, "Representative HTTPFlow: flow-9")
}

func TestBuildLoopHTTPFuzzProgressSnapshot_SummarizesCurrentProgress(t *testing.T) {
	stats := newLoopHTTPFuzzOverviewStats()
	stats.observeSuccess(200, 120, 24, true)
	stats.observeSuccess(401, 1450, 0, true)
	stats.observeSuccess(401, 1200, 0, true)
	stats.observeSuccess(401, 1300, 0, true)
	stats.observeError()
	sample := loopHTTPFuzzInterestingSample{
		Index:      2,
		Score:      70,
		StatusCode: 401,
		BodyLength: 0,
	}
	stats.considerInterestingSample(sample)
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{
		Index:      1,
		Score:      5,
		StatusCode: 200,
		BodyLength: 24,
	})
	stats.observeResponseLengthGroup(sample)

	snapshot := buildLoopHTTPFuzzProgressSnapshot("fuzz_body", stats, 401, false)
	require.Contains(t, snapshot, "执行进度：fuzz_body 已处理 5 个请求")
	require.Contains(t, snapshot, "成功 4，失败 1")
	require.Contains(t, snapshot, "已落库 4 条 HTTPFlow")
	require.Contains(t, snapshot, "最近状态 401")
	require.Contains(t, snapshot, "状态分布 401=3, 200=1")
	require.NotContains(t, snapshot, "长度分布")
	require.Contains(t, snapshot, "可疑样本 1 个")
}

func TestFinalizeLoopHTTPFuzzResponseLengthGroups_UsesDominantLengthAsBaseline(t *testing.T) {
	stats := newLoopHTTPFuzzOverviewStats()
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{
		Index:       1,
		Score:       5,
		StatusCode:  200,
		BodyLength:  10,
		ResponseRaw: "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\n0123456789",
	})
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{
		Index:       2,
		Score:       6,
		StatusCode:  200,
		BodyLength:  10,
		ResponseRaw: "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\n0123456789",
	})
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{
		Index:       3,
		Score:       20,
		StatusCode:  401,
		BodyLength:  5,
		ResponseRaw: "HTTP/1.1 401 Unauthorized\r\nContent-Length: 5\r\n\r\nadmin",
	})

	stats.finalizeResponseLengthGroups()

	require.Equal(t, 10, stats.BaselineBodyLength)
	require.True(t, stats.ResponseLengthGroups[10].IsBaseline)
	require.Contains(t, stats.ResponseLengthGroups[10].Sample.ResponseDiff, "baseline representative response")
	require.False(t, stats.ResponseLengthGroups[5].IsBaseline)
	require.NotEmpty(t, stats.ResponseLengthGroups[5].Sample.ResponseDiff)
}

func TestBuildLoopHTTPFuzzOverviewReport_SkipsLengthAnalysisForSmallRuns(t *testing.T) {
	stats := newLoopHTTPFuzzOverviewStats()
	stats.observeSuccess(200, 100, 12, true)
	stats.observeSuccess(200, 110, 18, true)
	stats.observeSuccess(200, 120, 27, true)
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{BodyLength: 12, StatusCode: 200, ResponseRaw: "HTTP/1.1 200 OK\r\n\r\na"})
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{BodyLength: 18, StatusCode: 200, ResponseRaw: "HTTP/1.1 200 OK\r\n\r\nbb"})
	stats.observeResponseLengthGroup(loopHTTPFuzzInterestingSample{BodyLength: 27, StatusCode: 200, ResponseRaw: "HTTP/1.1 200 OK\r\n\r\nccc"})
	stats.finalizeResponseLengthGroups()

	report := buildLoopHTTPFuzzOverviewReport("fuzz_get_params", stats)
	require.Contains(t, report, "Response Length Overview: 12B=1, 18B=1, 27B=1")
}

func TestBuildLoopHTTPFuzzLargeRunAnalysisReport_RendersGroupsAndInterestingSamples(t *testing.T) {
	stats := newLoopHTTPFuzzOverviewStats()
	baselineSample := loopHTTPFuzzInterestingSample{
		Index:          1,
		Score:          5,
		StatusCode:     200,
		DurationMs:     120,
		BodyLength:     24,
		HiddenIndex:    "flow-1",
		RequestSummary: "URL: https://example.test/login BODY: [(32) bytes]",
		ResponseRaw:    "HTTP/1.1 200 OK\r\nContent-Length: 24\r\n\r\nhello user",
		ResponseDigest: "  Status Code: 200\n  Content-Length: 24 bytes\n",
	}
	sample := loopHTTPFuzzInterestingSample{
		Index:           2,
		Score:           70,
		StatusCode:      401,
		DurationMs:      1450,
		BodyLength:      0,
		HiddenIndex:     "flow-2",
		Payloads:        []string{"{{payload(pass_top25)}}"},
		RequestSummary:  "URL: https://example.test/login BODY: [(32) bytes]",
		ResponseSummary: "URL: https://example.test/login STATUS: 401 BODY: [(0) bytes]",
		RequestDiff:     "  + password={{payload(pass_top25)}}",
		ResponseDigest:  "  Status Code: 401\n  Content-Length: 0 bytes\n",
		ResponseRaw:     "HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n",
	}

	for i := 0; i < 9; i++ {
		stats.observeSuccess(200, 120, 24, true)
		stats.observeResponseLengthGroup(baselineSample)
	}
	for i := 0; i < 4; i++ {
		stats.observeSuccess(401, 1450, 0, true)
		stats.observeResponseLengthGroup(sample)
	}
	stats.considerInterestingSample(sample)
	stats.finalizeResponseLengthGroups()

	report := buildLoopHTTPFuzzLargeRunAnalysisReport(stats)
	require.Contains(t, report, "=== Large-Run Analysis ===")
	require.Contains(t, report, "Response Length Groups:")
	require.Contains(t, report, "- 24 bytes: 9 responses [baseline] (statuses: 200=9)")
	require.Contains(t, report, "Baseline group selected by dominant body length: 24 bytes (9 responses).")
	require.Contains(t, report, "- 0 bytes: 4 responses (statuses: 401=4)")
	require.Contains(t, report, "Sample HTTPFlow: flow-2")
	require.Contains(t, report, "Sample Diff From Baseline:")
	require.Contains(t, report, "Interesting Samples:")
	require.Contains(t, report, "HTTPFlow: flow-2")
	require.Contains(t, report, "{{payload(pass_top25)}}")
}

func TestBuildLoopHTTPFuzzDetailedPacketReport_RendersStoredResults(t *testing.T) {
	reportData := newLoopHTTPFuzzReportData()
	reportData.observeError(1, errors.New("network timeout"))
	reportData.observeDetailedResult(2, loopHTTPFuzzProcessedResult{
		RequestRaw:      "GET /debug?id=1 HTTP/1.1\r\nHost: example.test\r\n\r\n",
		ResponseRaw:     "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok",
		RequestSummary:  "URL: https://example.test/debug?id=1 BODY: [(0) bytes]",
		ResponseSummary: "URL: https://example.test/debug?id=1 STATUS: 200 BODY: [(2) bytes]",
		RequestDiff:     "  + id=1",
		ResponseDigest:  "  Status Code: 200\n  Content-Length: 2 bytes\n",
		HiddenIndex:     "flow-2",
		StatusCode:      200,
		DurationMs:      180,
		Payloads:        []string{"1"},
		Sample: loopHTTPFuzzInterestingSample{
			Index: 2,
		},
	})

	report := buildLoopHTTPFuzzDetailedPacketReport(reportData)
	require.Contains(t, report, "=== Detailed Packet Results ===")
	require.Contains(t, report, "--- Result 1 ---")
	require.Contains(t, report, "Error:")
	require.Contains(t, report, "--- Result 2 ---")
	require.Contains(t, report, "Saved HTTPFlow: flow-2")
	require.Contains(t, report, "Request Packet:")
	require.Contains(t, report, "Response Packet:")
}
