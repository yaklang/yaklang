package test

import (
	"bytes"
	"context"
	"fmt"
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

const doHTTPRequestToolName = "do_http_request"

func getDoHTTPRequestTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/http/do_http_request.yak")
	if err != nil {
		t.Fatalf("failed to read do_http_request.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(doHTTPRequestToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse do_http_request.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func execTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestDoHTTPRequest_BasicURL(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "response flag not found in stdout")
	assert.Assert(t, strings.Contains(stdout, "request packet"), "request packet output not found in stdout")
	assert.Assert(t, strings.Contains(stdout, "response packet"), "response packet output not found in stdout")
	t.Logf("stdout size: %d bytes", len(stdout))
}

func TestDoHTTPRequest_RequestSmallPrint(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port) + "/test-path",
		"method":  "GET",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "request packet"), "request packet header not found")
	assert.Assert(t, strings.Contains(stdout, "bytes)"), "request size info not found")
	assert.Assert(t, strings.Contains(stdout, "/test-path"), "request path not found in printed request")
	assert.Assert(t, !strings.Contains(stdout, "truncated"), "small request should not be truncated")
}

func TestDoHTTPRequest_RequestLargeTruncate(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tool := getDoHTTPRequestTool(t)

	largeBody := strings.Repeat("X", 6*1024)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port),
		"method":       "POST",
		"body":         largeBody,
		"content-type": "text/plain",
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "request packet"), "request packet header not found")
	assert.Assert(t, strings.Contains(stdout, "truncated"), "large request should contain 'truncated'")
	assert.Assert(t, strings.Contains(stdout, "full request saved to file"), "truncation hint not found")
}

func TestDoHTTPRequest_ResponseSmallNoPattern(t *testing.T) {
	smallBody := "SMALL_RESPONSE_" + utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(smallBody))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, smallBody), "small response body should be fully printed")
	assert.Assert(t, strings.Contains(stdout, "response packet"), "response packet header not found")
	assert.Assert(t, !strings.Contains(stdout, "truncated"), "small response should not be truncated")
}

func TestDoHTTPRequest_ResponseLargeNoPattern(t *testing.T) {
	headMarker := "HEAD_MARKER_" + utils.RandStringBytes(10)
	tailMarker := "TAIL_MARKER_" + utils.RandStringBytes(10)
	largeBody := headMarker + strings.Repeat("A", 12*1024) + tailMarker

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n" + largeBody)
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, headMarker), "head of large response should be printed")
	assert.Assert(t, strings.Contains(stdout, "truncated"), "large response without pattern should be truncated")
	assert.Assert(t, strings.Contains(stdout, "full response saved to file"), "truncation hint not found")
	assert.Assert(t, !strings.Contains(stdout, tailMarker), "tail should not appear in truncated output")
}

func TestDoHTTPRequest_ResponseSmallWithKeyword(t *testing.T) {
	keyword := "UNIQUE_KEYWORD_" + utils.RandStringBytes(10)
	smallBody := "prefix content " + keyword + " suffix content"
	host, port := utils.DebugMockHTTP([]byte(smallBody))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"keyword": keyword,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, smallBody), "small response with pattern should be fully printed")
	assert.Assert(t, strings.Contains(stdout, "searching keyword pattern"), "should show pattern search info")
	assert.Assert(t, strings.Contains(stdout, keyword), "keyword should appear in output")
	assert.Assert(t, strings.Contains(stdout, "keyword match"), "keyword match result should be shown")
}

func TestDoHTTPRequest_ResponseLargeWithKeyword(t *testing.T) {
	keyword := "NEEDLE_" + utils.RandStringBytes(10)
	largeBody := strings.Repeat("B", 6*1024) + keyword + strings.Repeat("C", 6*1024)

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n" + largeBody)
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"keyword": keyword,
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, "too large"), "large response with pattern should show 'too large'")
	assert.Assert(t, strings.Contains(stdout, "skip inline printing"), "should indicate inline printing skipped")
	assert.Assert(t, strings.Contains(stdout, "searching keyword pattern"), "should show pattern search info")
	assert.Assert(t, strings.Contains(stdout, keyword), "keyword should be found in match results")
	assert.Assert(t, strings.Contains(stdout, "keyword match"), "keyword match result should be shown")
}

func TestDoHTTPRequest_ResponseWithRegexp(t *testing.T) {
	marker := "ERR_CODE_42"
	body := "status=ok " + marker + " done"
	host, port := utils.DebugMockHTTP([]byte(body))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port),
		"regexp-match": `ERR_CODE_\d+`,
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "searching regexp pattern"), "should show regexp pattern info")
	assert.Assert(t, strings.Contains(stdout, `ERR_CODE_\d+`), "pattern string should be displayed")
	assert.Assert(t, strings.Contains(stdout, "regexp match"), "regexp match result should be shown")
	assert.Assert(t, strings.Contains(stdout, marker), "matched content should appear")
}

func TestDoHTTPRequest_PacketMode(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if strings.Contains(string(req), "X-Custom-Test") {
			return []byte("HTTP/1.1 200 OK\r\n\r\n" + flag)
		}
		return []byte("HTTP/1.1 400 Bad Request\r\n\r\nmissing header")
	})
	tool := getDoHTTPRequestTool(t)

	packet := fmt.Sprintf("GET /packet-test HTTP/1.1\r\nHost: %s:%d\r\nX-Custom-Test: yes\r\n\r\n", host, port)
	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"packet":  packet,
		"https":   "no",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "packet mode response should contain flag")
	assert.Assert(t, strings.Contains(stdout, "request packet"), "request should be printed in packet mode")
	assert.Assert(t, strings.Contains(stdout, "X-Custom-Test"), "custom header should appear in request output")
}

func TestDoHTTPRequest_CustomHeaders(t *testing.T) {
	receivedHeader := ""
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "X-My-Header: test-value-123") {
			receivedHeader = "found"
			return []byte("HTTP/1.1 200 OK\r\n\r\nheader_received")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nheader_missing")
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"headers": "X-My-Header: test-value-123",
		"timeout": 10,
	})

	_ = receivedHeader
	assert.Assert(t, strings.Contains(stdout, "X-My-Header"), "custom header should appear in request packet output")
	assert.Assert(t, strings.Contains(stdout, "header_received"), "server should receive the custom header")
}

func TestDoHTTPRequest_PostBody(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, `"name":"test"`) {
			return []byte("HTTP/1.1 200 OK\r\n\r\nbody_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nbody_missing")
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port),
		"method":       "POST",
		"body":         `{"name":"test"}`,
		"content-type": "application/json",
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "body_ok"), "server should receive the body")
	assert.Assert(t, strings.Contains(stdout, `"name":"test"`), "body should appear in request packet output")
}

func TestDoHTTPRequest_QueryParams(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "foo=bar") && strings.Contains(reqStr, "baz=qux") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nquery_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nquery_missing")
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port) + "/search",
		"query-params": "foo=bar&baz=qux",
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "query_ok"), "server should receive query params")
	assert.Assert(t, strings.Contains(stdout, "foo=bar"), "query param should appear in request output")
}

func TestDoHTTPRequest_PostParams(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "user=admin") && strings.Contains(reqStr, "pass=secret") {
			return []byte("HTTP/1.1 200 OK\r\n\r\npost_params_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\npost_params_missing")
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":         "http://" + host + ":" + strconv.Itoa(port),
		"method":      "POST",
		"post-params": "user=admin&pass=secret",
		"timeout":     10,
	})

	assert.Assert(t, strings.Contains(stdout, "post_params_ok"), "server should receive post params")
	assert.Assert(t, strings.Contains(stdout, "user=admin"), "post param should appear in request output")
}

func TestDoHTTPRequest_Redirect(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(200)
			w.Write([]byte(flag))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":            "http://" + host + ":" + strconv.Itoa(port) + "/start",
		"redirect-times": 3,
		"timeout":        10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "should follow redirect and get final response")
	assert.Assert(t, strings.Contains(stdout, "redirect"), "redirect info should appear in stdout")
}

func TestDoHTTPRequest_NoRedirect(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(200)
			w.Write([]byte("final_page"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":            "http://" + host + ":" + strconv.Itoa(port) + "/start",
		"redirect-times": 0,
		"timeout":        10,
	})

	assert.Assert(t, !strings.Contains(stdout, "final_page"), "should not follow redirect when redirect-times=0")
	assert.Assert(t, strings.Contains(stdout, "302") || strings.Contains(stdout, "Found"), "should show redirect status code")
}

func TestDoHTTPRequest_ShowRequest(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port),
		"show-request": "yes",
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "request packet"), "request should always be printed")
	assert.Assert(t, strings.Contains(stdout, "saved to"), "show-request=yes should save request to file")
}

func TestDoHTTPRequest_Timeout(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {}
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"timeout": 2,
	})

	assert.Assert(t,
		strings.Contains(stdout, "failed") || strings.Contains(stdout, "timeout") || strings.Contains(stdout, "context deadline"),
		"timeout should cause a failure message, got: %s", stdout)
}

func TestDoHTTPRequest_ContentType(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)
		if strings.Contains(reqStr, "Content-Type: application/xml") {
			return []byte("HTTP/1.1 200 OK\r\n\r\nctype_ok")
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\nctype_missing")
	})
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":          "http://" + host + ":" + strconv.Itoa(port),
		"method":       "POST",
		"body":         "<root/>",
		"content-type": "application/xml",
		"timeout":      10,
	})

	assert.Assert(t, strings.Contains(stdout, "ctype_ok"), "server should receive correct content-type")
	assert.Assert(t, strings.Contains(stdout, "application/xml"), "content-type should appear in request output")
}

func TestDoHTTPRequest_HttpsMode(t *testing.T) {
	flag := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTP([]byte(flag))
	tool := getDoHTTPRequestTool(t)

	stdout, _ := execTool(t, tool, aitool.InvokeParams{
		"url":     "http://" + host + ":" + strconv.Itoa(port),
		"https":   "no",
		"timeout": 10,
	})

	assert.Assert(t, strings.Contains(stdout, flag), "https=no should work for plain HTTP")
}
