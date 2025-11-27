package loop_http_differ

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// getFuzzRequest retrieves the FuzzHTTPRequest from loop context
func getFuzzRequest(loop *reactloops.ReActLoop) (*mutate.FuzzHTTPRequest, error) {
	fuzzReqAny := loop.GetVariable("fuzz_request")
	if fuzzReqAny == nil {
		return nil, utils.Error("fuzz_request not found in loop context")
	}
	fuzzReq, ok := fuzzReqAny.(*mutate.FuzzHTTPRequest)
	if !ok {
		return nil, utils.Error("fuzz_request is not a valid FuzzHTTPRequest")
	}
	return fuzzReq, nil
}

// executeFuzzAndCompare executes the fuzz request and compares the response with the original
func executeFuzzAndCompare(loop *reactloops.ReActLoop, fuzzResult mutate.FuzzHTTPRequestIf, actionName string) (string, error) {
	isHttpsStr := loop.Get("is_https")
	isHttps := isHttpsStr == "true"

	// Execute the fuzz request
	resultCh, err := fuzzResult.Exec(mutate.WithPoolOpt_Https(isHttps))
	if err != nil {
		return "", utils.Errorf("failed to execute fuzz request: %v", err)
	}

	var results []string
	var diffSummary strings.Builder
	diffSummary.WriteString(fmt.Sprintf("=== Fuzz Results for %s ===\n", actionName))

	originalRequest := loop.Get("original_request")
	count := 0
	maxResults := 10 // Limit results to prevent overwhelming output

	for result := range resultCh {
		if count >= maxResults {
			diffSummary.WriteString(fmt.Sprintf("\n... (more results truncated, showing first %d)\n", maxResults))
			break
		}

		if result.Error != nil {
			diffSummary.WriteString(fmt.Sprintf("\n[%d] Error: %v\n", count+1, result.Error))
			count++
			continue
		}

		// Get request and response
		requestRaw := string(result.RequestRaw)
		responseRaw := string(result.ResponseRaw)

		// Compare request differences
		requestDiff := compareRequests(originalRequest, requestRaw)
		responseSummary := summarizeResponse(responseRaw)

		diffSummary.WriteString(fmt.Sprintf("\n--- Result %d ---\n", count+1))
		diffSummary.WriteString(fmt.Sprintf("Payload: %v\n", result.Payloads))
		diffSummary.WriteString(fmt.Sprintf("Request Changes:\n%s\n", requestDiff))
		diffSummary.WriteString(fmt.Sprintf("Response Summary:\n%s\n", responseSummary))

		// Store last request and response
		loop.Set("last_request", requestRaw)
		loop.Set("last_response", responseRaw)

		results = append(results, fmt.Sprintf("Payload: %v, Status: %s", result.Payloads, getStatusFromResponse(responseRaw)))
		count++
	}

	diffResult := diffSummary.String()
	loop.Set("diff_result", diffResult)

	return diffResult, nil
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

	statusLine, body := lowhttp.SplitHTTPPacketFast([]byte(response))

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("  Status: %s\n", statusLine))

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
func getStatusFromResponse(response string) string {
	statusLine, _ := lowhttp.SplitHTTPPacketFast([]byte(response))
	return statusLine
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
