package loop_http_fuzztest

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

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

func formatLoopHTTPFuzzStatusCode(statusCode int) string {
	if statusCode <= 0 {
		return "(no status code)"
	}
	return fmt.Sprintf("%d", statusCode)
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

func formatLoopHTTPFuzzTopStatusCounts(counts map[int]int, maxItems int) string {
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

// summarizeResponse creates a summary of the HTTP response
func summarizeResponse(response string) string {
	if response == "" {
		return "  (empty response)"
	}

	_, body := lowhttp.SplitHTTPPacketFast([]byte(response))
	statusCode := getStatusFromResponse(response)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("  Status Code: %s\n", formatLoopHTTPFuzzStatusCode(statusCode)))

	contentLength := len(body)
	summary.WriteString(fmt.Sprintf("  Content-Length: %d bytes\n", contentLength))

	if contentLength > 0 {
		bodyPreview := utils.ShrinkString(string(body), 200)
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		summary.WriteString(fmt.Sprintf("  Body Preview: %s\n", bodyPreview))
	}

	return summary.String()
}

// getStatusFromResponse extracts status code from response
func getStatusFromResponse(response string) int {
	return lowhttp.ExtractStatusCodeFromResponse([]byte(response))
}
