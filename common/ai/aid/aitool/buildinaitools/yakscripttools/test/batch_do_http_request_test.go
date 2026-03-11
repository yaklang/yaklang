package test

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const batchDoHTTPRequestToolName = "batch_do_http_request"

func getBatchDoHTTPRequestTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/http/batch_do_http_request.yak")
	if err != nil {
		t.Fatalf("failed to read batch_do_http_request.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(batchDoHTTPRequestToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse batch_do_http_request.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func execBatchTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

// Test 1: Basic batch GET requests - verify summary output
func TestBatchDoHTTPRequest_BasicGet(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("response_body"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/path1,/path2",
		"concurrent":  2,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "Total paths to test: 2"), "should show 2 paths")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "should show 2 successful requests")
	assert.Assert(t, strings.Contains(stdout, "/path1"), "path1 should appear")
	assert.Assert(t, strings.Contains(stdout, "/path2"), "path2 should appear")
}

// Test 2: Prefix parameter
func TestBatchDoHTTPRequest_Prefix(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("ok"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "users,admin",
		"prefix":      "/v1/api/",
		"concurrent":  1,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "Applied path prefix: /v1/api/"), "should show prefix applied")
	assert.Assert(t, strings.Contains(stdout, "/v1/api/users"), "prefixed path users should appear")
	assert.Assert(t, strings.Contains(stdout, "/v1/api/admin"), "prefixed path admin should appear")
}

// Test 3: Single custom header via headers parameter
func TestBatchDoHTTPRequest_SingleHeader(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "X-Custom-Header: custom-value-123") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nheader_received")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nheader_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":   "http://" + host + ":" + strconv.Itoa(port),
		"paths":      "/test",
		"headers":    "X-Custom-Header: custom-value-123",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "header_received"), "server should receive the custom header")
	assert.Assert(t, strings.Contains(stdout, "Success: 1"), "request with header should succeed")
}

// Test 4: Multiple custom headers via multi-line headers parameter
func TestBatchDoHTTPRequest_MultipleHeaders(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		hasAuth := strings.Contains(reqStr, "Authorization: Bearer token123")
		hasReqID := strings.Contains(reqStr, "X-Request-Id: abc456")
		if hasAuth && hasReqID {
			return []byte("HTTP/1.1 200 OK\r\n\r\nboth_headers_found")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nheaders_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":   "http://" + host + ":" + strconv.Itoa(port),
		"paths":      "/test",
		"headers":    "Authorization: Bearer token123\nX-Request-Id: abc456",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "both_headers_found"), "server should receive both custom headers")
	assert.Assert(t, strings.Contains(stdout, "Success: 1"), "request with multiple headers should succeed")
}

// Test 5: Exclude status codes
func TestBatchDoHTTPRequest_ExcludeCodes(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(200)
			w.Write([]byte("success"))
			return
		}
		w.WriteHeader(404)
		w.Write([]byte("not_found"))
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/ok,/missing",
		"exclude-code":  "404",
		"concurrent":    2,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "success"), "200 response should be shown")
	assert.Assert(t, strings.Contains(stdout, "Filtered: 1"), "404 should be filtered")
}

// Test 6: Include only specific status codes
func TestBatchDoHTTPRequest_IncludeCodes(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(200)
			w.Write([]byte("success"))
			return
		}
		w.WriteHeader(201)
		w.Write([]byte("created"))
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/ok,/created",
		"include-code":  "200",
		"concurrent":    2,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "success"), "200 response should be shown")
	assert.Assert(t, strings.Contains(stdout, "Filtered: 1"), "201 should be filtered when include-code=200")
}

// Test 7: POST method with body
func TestBatchDoHTTPRequest_PostWithBody(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "POST") && strings.Contains(reqStr, `"test":"value"`) {
			return []byte("HTTP/1.1 200 OK\r\n\r\npost_ok")
		}
		return []byte("HTTP/1.1 400 Bad Request\r\n\r\npost_failed")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/submit",
		"method":        "POST",
		"body":          `{"test":"value"}`,
		"content-type":  "application/json",
		"concurrent":    1,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "post_ok"), "POST with body should succeed")
}

// Test 8: Sequential execution with delay
func TestBatchDoHTTPRequest_DelaySequential(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("test"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/a,/b",
		"delay-seconds": 0.1,
		"concurrent":    5, // Should be overridden to 1
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "forcing sequential execution"), "should indicate sequential mode")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "should complete requests successfully")
}

// Test 9: Keyword matching
func TestBatchDoHTTPRequest_KeywordMatch(t *testing.T) {
	keyword := "UNIQUE_KEYWORD_" + utils.RandStringBytes(10)
	body := "prefix " + keyword + " suffix"
	host, port := utils.DebugMockHTTP([]byte(body))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/test",
		"keyword":     keyword,
		"concurrent":  1,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, keyword), "keyword should be found")
}

// Test 10: Redirect handling
func TestBatchDoHTTPRequest_Redirect(t *testing.T) {
	flag := "FINAL_PAGE_" + utils.RandStringBytes(10)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(200)
			w.Write([]byte(flag))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":        "http://" + host + ":" + strconv.Itoa(port),
		"paths":           "/start",
		"redirect-times":  3,
		"concurrent":      1,
		"timeout":         10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "should follow redirect and get final response")
}

// Test 11: No redirect
func TestBatchDoHTTPRequest_NoRedirect(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(200)
			w.Write([]byte("final_page"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":        "http://" + host + ":" + strconv.Itoa(port),
		"paths":           "/start",
		"redirect-times":  0,
		"concurrent":      1,
		"timeout":         10,
	})

	assert.Assert(t, !strings.Contains(stdout, "final_page"), "should not follow redirect when redirect-times=0")
	assert.Assert(t, strings.Contains(stdout, "302") || strings.Contains(stdout, "Found"), "should show redirect status")
}

// Test 12: Concurrent vs sequential execution
func TestBatchDoHTTPRequest_ConcurrentMode(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("test"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/a,/b,/c",
		"concurrent":  3,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "concurrent batch requests"), "should indicate concurrent mode")
	assert.Assert(t, strings.Contains(stdout, "Success: 3"), "all 3 requests should succeed")
}

// Test 13: Error handling for unreachable host
func TestBatchDoHTTPRequest_ErrorHandling(t *testing.T) {
	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://127.0.0.1:59999", // Unreachable port
		"paths":       "/test",
		"concurrent":  1,
		"timeout":     2, // Short timeout
	})

	assert.Assert(t, strings.Contains(stdout, "Errors: 1"), "should report 1 error for unreachable host")
}

// Test 14: Content-Type header
func TestBatchDoHTTPRequest_ContentType(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "Content-Type: application/xml") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nctype_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nctype_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/submit",
		"method":        "POST",
		"body":          "<root/>",
		"content-type":  "application/xml",
		"concurrent":    1,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "ctype_ok"), "server should receive correct content-type")
}

// Test 15: Results summary table
func TestBatchDoHTTPRequest_ResultsSummary(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("test"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/a,/b",
		"concurrent":  2,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "Results Summary Table"), "should show summary table")
	assert.Assert(t, strings.Contains(stdout, "Index"), "should show Index column")
	assert.Assert(t, strings.Contains(stdout, "Path"), "should show Path column")
	assert.Assert(t, strings.Contains(stdout, "Status"), "should show Status column")
	assert.Assert(t, strings.Contains(stdout, "Size"), "should show Size column")
}

// Test 16: HTTPS mode forcing
func TestBatchDoHTTPRequest_HttpsMode(t *testing.T) {
	flag := "HTTPS_TEST_" + utils.RandStringBytes(10)
	host, port := utils.DebugMockHTTP([]byte(flag))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/test",
		"https":       "no",
		"concurrent":  1,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "https=no should work for plain HTTP")
}

// Test 17: Query parameters
func TestBatchDoHTTPRequest_QueryParams(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "foo=bar") && strings.Contains(reqStr, "baz=qux") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nquery_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nquery_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/search",
		"query-params":  "foo=bar&baz=qux",
		"concurrent":    1,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "query_ok"), "query params should be sent correctly")
}

// Test 18: Multiple paths with different responses
func TestBatchDoHTTPRequest_MultiplePaths(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/users":
			w.WriteHeader(200)
			w.Write([]byte("users_list"))
		case "/api/posts":
			w.WriteHeader(200)
			w.Write([]byte("posts_list"))
		default:
			w.WriteHeader(404)
			w.Write([]byte("not_found"))
		}
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/api/users,/api/posts",
		"concurrent":  2,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "users_list"), "users endpoint should respond")
	assert.Assert(t, strings.Contains(stdout, "posts_list"), "posts endpoint should respond")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "both requests should succeed")
}

// Test 19: Empty paths validation
func TestBatchDoHTTPRequest_EmptyPathsValidation(t *testing.T) {
	tool := getBatchDoHTTPRequestTool(t)
	stdout, stderr := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://example.com",
		"paths":       "", // Empty paths
		"concurrent":  1,
		"timeout":     10,
	})

	// Should have error in stderr or stdout about missing paths
	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(combined, "no valid paths") ||
		strings.Contains(combined, "at least one") ||
		strings.Contains(combined, "Error"),
		"should report error for empty paths")
}

// Test 20: Custom headers (non-curl style)
func TestBatchDoHTTPRequest_CustomHeaders(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "X-Custom: value123") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nheader_received")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nheader_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":    "http://" + host + ":" + strconv.Itoa(port),
		"paths":       "/test",
		"headers":     "X-Custom: value123",
		"concurrent":  1,
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "header_received"), "custom header should be sent")
}

// Test 21: Packet mode with {{PATH}} substitution
func TestBatchDoHTTPRequest_PacketMode(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("packet_path:" + r.URL.Path))
	})

	tool := getBatchDoHTTPRequestTool(t)
	packet := "GET {{PATH}} HTTP/1.1\r\nHost: " + host + ":" + strconv.Itoa(port) + "\r\nUser-Agent: TestAgent\r\n\r\n"

	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"packet":     packet,
		"paths":      "/pkt_a,/pkt_b",
		"https":      "no",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "packet_path:/pkt_a"), "packet mode should substitute path_a")
	assert.Assert(t, strings.Contains(stdout, "packet_path:/pkt_b"), "packet mode should substitute path_b")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "both packet mode requests should succeed")
}

// Test 22: Full URLs in paths (no base-url needed)
func TestBatchDoHTTPRequest_FullUrlsInPaths(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("fullurl_path:" + r.URL.Path))
	})

	tool := getBatchDoHTTPRequestTool(t)
	baseAddr := "http://" + host + ":" + strconv.Itoa(port)
	paths := baseAddr + "/full_a," + baseAddr + "/full_b"

	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"paths":      paths,
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "fullurl_path:/full_a"), "full URL path_a should work")
	assert.Assert(t, strings.Contains(stdout, "fullurl_path:/full_b"), "full URL path_b should work")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "both full URL requests should succeed")
}

// Test 23: Newline-separated paths
func TestBatchDoHTTPRequest_NewlinePaths(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("nl_ok"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":   "http://" + host + ":" + strconv.Itoa(port),
		"paths":      "/nl_a\n/nl_b\n/nl_c",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "Total paths to test: 3"), "should parse 3 paths from newlines")
	assert.Assert(t, strings.Contains(stdout, "Success: 3"), "all 3 newline-separated paths should succeed")
}

// Test 24: Exclude size ranges
func TestBatchDoHTTPRequest_ExcludeSize(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.WriteHeader(200)
			w.Write([]byte(strings.Repeat("X", 500)))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("tiny"))
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":     "http://" + host + ":" + strconv.Itoa(port),
		"paths":        "/big,/small",
		"exclude-size": "400-600",
		"concurrent":   1,
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "Filtered: 1"), "500-byte response should be filtered by exclude-size")
	assert.Assert(t, strings.Contains(stdout, "tiny"), "small response should be shown")
}

// Test 25: Regexp matching in batch responses
func TestBatchDoHTTPRequest_RegexpMatch(t *testing.T) {
	flag := "ERR_CODE_" + utils.RandStringBytes(5)
	body := "status=ok " + flag + " done"
	host, port := utils.DebugMockHTTP([]byte(body))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":     "http://" + host + ":" + strconv.Itoa(port),
		"paths":        "/test",
		"regexp-match": `ERR_CODE_\w+`,
		"concurrent":   1,
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "regexp match should find the flag")
	assert.Assert(t, strings.Contains(stdout, "Regexp Match"), "should show regexp match section")
}

// Test 26: Max body size truncation
func TestBatchDoHTTPRequest_MaxBodySize(t *testing.T) {
	largeBody := strings.Repeat("Y", 200)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n" + largeBody)
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":      "http://" + host + ":" + strconv.Itoa(port),
		"paths":         "/test",
		"max-body-size": 50,
		"concurrent":    1,
		"timeout":       10,
	})

	assert.Assert(t, strings.Contains(stdout, "truncated"), "response should be truncated when exceeding max-body-size")
}

// Test 27: Mixed comma and newline path separator
func TestBatchDoHTTPRequest_MixedPathSeparators(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte("mix_ok"))

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":   "http://" + host + ":" + strconv.Itoa(port),
		"paths":      "/a,/b\n/c,/d\n/e",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "Total paths to test: 5"), "should parse 5 paths from mixed separators")
	assert.Assert(t, strings.Contains(stdout, "Success: 5"), "all 5 mixed-separator paths should succeed")
}

// Test 28: Packet mode with explicit base-host override
func TestBatchDoHTTPRequest_PacketBaseHost(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("basehost_ok:" + r.URL.Path))
	})

	tool := getBatchDoHTTPRequestTool(t)
	// Packet has a dummy Host, but base-host overrides the actual connection target
	packet := "GET {{PATH}} HTTP/1.1\r\nHost: " + host + ":" + strconv.Itoa(port) + "\r\n\r\n"

	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"packet":    packet,
		"paths":     "/bh_test",
		"base-host": host + ":" + strconv.Itoa(port),
		"https":     "no",
		"concurrent": 1,
		"timeout":   10,
	})

	assert.Assert(t, strings.Contains(stdout, "basehost_ok:/bh_test"), "base-host should route request correctly")
	assert.Assert(t, strings.Contains(stdout, "Success: 1"), "packet mode with base-host should succeed")
}

// Test 29: Full URLs in paths with custom headers (regression test for shared-params fix)
func TestBatchDoHTTPRequest_FullUrlWithHeaders(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "X-Full-Url-Header: present") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nfullurl_header_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nfullurl_header_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	fullUrl := "http://" + host + ":" + strconv.Itoa(port) + "/full_with_header"

	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"paths":      fullUrl,
		"headers":    "X-Full-Url-Header: present",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "fullurl_header_ok"), "custom headers should be applied to full URL paths")
	assert.Assert(t, strings.Contains(stdout, "Success: 1"), "full URL with headers should succeed")
}

// Test 30: Full URLs in paths with body and content-type
func TestBatchDoHTTPRequest_FullUrlWithBody(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "POST") && strings.Contains(reqStr, `"key":"val"`) &&
			strings.Contains(reqStr, "Content-Type: application/json") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nfullurl_body_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nfullurl_body_missing")
	})

	tool := getBatchDoHTTPRequestTool(t)
	fullUrl := "http://" + host + ":" + strconv.Itoa(port) + "/full_post"

	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"paths":        fullUrl,
		"method":       "POST",
		"body":         `{"key":"val"}`,
		"content-type": "application/json",
		"concurrent":   1,
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "fullurl_body_ok"), "body and content-type should be applied to full URL paths")
}

// Test 31: Prefix with leading-slash paths
func TestBatchDoHTTPRequest_PrefixWithSlashPaths(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("got:" + r.URL.Path))
	})

	tool := getBatchDoHTTPRequestTool(t)
	stdout, _ := execBatchTool(t, tool, aitool.InvokeParams{
		"base-url":   "http://" + host + ":" + strconv.Itoa(port),
		"paths":      "/users,admin",
		"prefix":     "api/v2",
		"concurrent": 1,
		"timeout":    10,
	})

	assert.Assert(t, strings.Contains(stdout, "/api/v2/users"), "prefix should handle paths with leading slash")
	assert.Assert(t, strings.Contains(stdout, "/api/v2/admin"), "prefix should handle paths without leading slash")
	assert.Assert(t, strings.Contains(stdout, "Success: 2"), "both prefixed requests should succeed")
}
