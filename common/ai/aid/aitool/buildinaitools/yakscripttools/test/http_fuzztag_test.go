package test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type renderedHTTPRequest struct{ path, query, form, header, body string }

func fuzztagHTTPServer(t *testing.T) (*httptest.Server, chan renderedHTTPRequest) {
	t.Helper()
	received := make(chan renderedHTTPRequest, 600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		_ = r.ParseForm()
		received <- renderedHTTPRequest{r.URL.Path, r.URL.Query().Get("value"), r.PostForm.Get("value"), r.Header.Get("X-Test"), string(body)}
		_, _ = w.Write([]byte("rendered_ok"))
	}))
	t.Cleanup(server.Close)
	return server, received
}

func invokeHTTPFuzztag(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) map[string]any {
	t.Helper()
	result, err := tool.InvokeWithParams(params)
	require.NoError(t, err)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	require.Contains(t, execution.CombinedOutput, "request")
	semantic := utils.InterfaceToGeneralMap(execution.Result)
	cleanupHTTPArtifacts(t, semantic)
	return semantic
}

func TestDoHTTPRequestFuzztagRendersBeforeEncoding(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	params := aitool.InvokeParams{
		"url": server.URL + "/{{lower(TEST)}}", "method": "POST", "fuzztag": true,
		"headers":   map[string]any{"X-Test": "{{base64({{hex(abc)}})}}"},
		"query":     map[string]any{"value": "{{params(payload)}}"},
		"form":      map[string]any{"value": "{{params(payload)}}"},
		"variables": map[string]any{"payload": `a & b+" {{int(1-3)}}`},
	}
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), params)
	require.True(t, utils.InterfaceToBoolean(semantic["response_received"]))
	require.Len(t, received, 1)
	got := <-received
	require.Equal(t, "/test", got.path)
	require.Equal(t, `a & b+" {{int(1-3)}}`, got.query)
	require.Equal(t, got.query, got.form)
	require.Equal(t, "NjE2MjYz", got.header)
}

func TestDoHTTPRequestFuzztagPacketAndLiteralModes(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	host := strings.TrimPrefix(server.URL, "http://")
	for _, enabled := range []bool{false, true} {
		invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
			"packet": "POST /test HTTP/1.1\r\nHost: " + host + "\r\n\r\n{{base64(abc)}}",
			"https":  "no", "fuzztag": enabled,
		})
		require.Len(t, received, 1)
		got := <-received
		if enabled {
			require.Equal(t, "YWJj", got.body)
		} else {
			require.Equal(t, "{{base64(abc)}}", got.body)
		}
	}
}

func TestDoHTTPRequestFuzztagRejectsInvalidBeforeSending(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	for _, template := range []string{"{{base64({{unknown()}})}}", "{{params(missing)}}", "{{unclosed"} {
		_, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{
			"url": server.URL, "query": map[string]any{"value": template}, "fuzztag": true,
		})
		require.Error(t, err, template)
	}
	require.Len(t, received, 0)
}

func TestDoHTTPRequestFuzztagFieldsCartesianAndEncoding(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/users/{{int(1-2)}}", "fuzztag": true, "method": "POST",
		"query":        map[string]any{"value": "{{base64({{params(payload)}})}}"},
		"form":         map[string]any{"value": "{{params::row(payload)}}"},
		"headers":      map[string]any{"X-Test": "{{list::row(first|second)}}"},
		"variables":    map[string]any{"payload": []any{"a & b", `c+" {{int(1-3)}}`}},
		"max-requests": 8, "concurrent": 4,
	})
	// Unsynchronized query payload (2) × paired form/header (2) × ID (2).
	require.Equal(t, 8, utils.InterfaceToInt(semantic["request_count"]))
	require.Equal(t, 8, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Len(t, received, 8)
	counts := map[renderedHTTPRequest]int{}
	for len(received) > 0 {
		counts[<-received]++
	}
	require.Len(t, counts, 8)
	for got := range counts {
		require.Contains(t, []string{"/users/1", "/users/2"}, got.path)
		if got.header == "first" {
			require.Equal(t, "a & b", got.form)
		} else {
			require.Equal(t, `c+" {{int(1-3)}}`, got.form)
			require.Equal(t, "second", got.header)
		}
		require.Contains(t, []string{"YSAmIGI=", "YysiIHt7aW50KDEtMyl9fQ=="}, got.query)
	}
}

func TestDoHTTPRequestFuzztagPairsPathQueryAndBody(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/users/{{int::row(1-2)}}", "fuzztag": true, "method": "POST",
		"query":        map[string]any{"value": "{{list::row(a & b|c+d)}}"},
		"body":         `{"user":{"id":{{int::row(1-2)}}}}`,
		"headers":      map[string]any{"X-Test": "{{list::row(one|two)}}"},
		"max-requests": 2, "concurrent": 2,
	})
	require.Equal(t, 2, utils.InterfaceToInt(semantic["request_count"]))
	require.Equal(t, 2, utils.InterfaceToInt(semantic["response_received_count"]))
	got := []renderedHTTPRequest{<-received, <-received}
	require.ElementsMatch(t, []renderedHTTPRequest{
		{path: "/users/1", query: "a & b", header: "one", body: `{"user":{"id":1}}`},
		{path: "/users/2", query: "c+d", header: "two", body: `{"user":{"id":2}}`},
	}, got)
}

func TestDoHTTPRequestFuzztagPacketNestedRawAndRepeat(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"fuzztag": true, "packet": "POST /test HTTP/1.1\r\nHost: " + strings.TrimPrefix(server.URL, "http://") + "\r\nX-Test: {{base64({{hex({{params(value)}})}})}}\r\n\r\n{{repeat(3)}}{{={\"nested\":{\"value\":\"{{literal}}\"}}=}}",
		"variables": map[string]any{"value": "abc"}, "https": "no", "max-requests": 3, "concurrent": 3,
	})
	require.Equal(t, 3, utils.InterfaceToInt(semantic["request_count"]))
	require.Equal(t, 3, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Len(t, received, 3)
	for len(received) > 0 {
		got := <-received
		require.Equal(t, "NjE2MjYz", got.header)
		require.Equal(t, `{"nested":{"value":"{{literal}}"}}`, got.body)
	}
}

func TestDoHTTPRequestFuzztagAllOrNothingPreflight(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	for _, params := range []aitool.InvokeParams{
		{"url": server.URL + "/valid\n/{{params(missing)}}", "fuzztag": true},
		{"url": server.URL + "/valid\n/{{base64({{unknown()}})}}", "fuzztag": true},
		{"url": server.URL + "/{{int(1-3)}}", "fuzztag": true, "query": map[string]any{"value": "{{list(a|b)}}"}, "max-requests": 5},
		{"url": server.URL + "/{{list(valid|next)}}", "fuzztag": true, "body": "{{int(1-3)}}", "max-requests": 5},
	} {
		_, err := getDoHTTPRequestTool(t).InvokeWithParams(params)
		require.Error(t, err)
		require.Len(t, received, 0)
	}
}

func TestDoHTTPRequestFuzztagDisabledAndCanceled(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/test", "fuzztag": false, "body": "{{unknown}} {{params(missing)}}", "method": "POST",
	})
	require.True(t, utils.InterfaceToBoolean(semantic["response_received"]))
	require.Equal(t, "{{unknown}} {{params(missing)}}", (<-received).body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{"url": server.URL + "/{{int(1-500)}}", "fuzztag": true, "max-requests": 500}, aitool.WithContext(ctx))
	require.Error(t, err)
	require.Len(t, received, 0)
}

func TestDoHTTPRequestFuzztagRangeAndEncodingMultiOutput(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	result, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{
		"url": server.URL + "/users/{{int(1-10)}}", "fuzztag": true,
		"query":        map[string]any{"value": "{{base64({{hex(abc)}})}}"},
		"max-requests": 10, "concurrent": 3, "keyword": "rendered_ok", "regexp-match": "rendered_[a-z]+",
	})
	require.NoError(t, err)
	execution := result.Data.(*aitool.ToolExecutionResult)
	require.Contains(t, execution.CombinedOutput, "multi-packet summary")
	require.NotContains(t, execution.CombinedOutput, "mode: URL")
	require.NotContains(t, execution.CombinedOutput, "response packet (")
	semantic := utils.InterfaceToGeneralMap(execution.Result)
	cleanupHTTPArtifacts(t, semantic)
	require.Equal(t, "multi", semantic["mode"])
	require.Equal(t, 10, utils.InterfaceToInt(semantic["request_count"]))
	require.Equal(t, 10, utils.InterfaceToInt(semantic["request_sent_count"]))
	require.Equal(t, 10, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Zero(t, utils.InterfaceToInt(semantic["transport_error_count"]))
	statuses := utils.InterfaceToGeneralMap(semantic["status_code_distribution"])
	require.Equal(t, 10, utils.InterfaceToInt(statuses["200"]))
	items := utils.InterfaceToSliceInterface(semantic["items"])
	require.Len(t, items, 10)
	for i, raw := range items {
		item := utils.InterfaceToGeneralMap(raw)
		require.Equal(t, i+1, utils.InterfaceToInt(item["idx"]))
		require.Contains(t, item["url"], fmt.Sprintf("/users/%d?", i+1))
		require.Contains(t, item["request"], "value=NjE2MjYz")
		require.Contains(t, item["response"], "rendered_ok")
		require.Equal(t, 1, utils.InterfaceToInt(item["keyword_match_count"]))
		require.Equal(t, 1, utils.InterfaceToInt(item["regexp_match_count"]), "item=%v", item)
	}
	require.Len(t, received, 10)
	paths := map[string]bool{}
	for len(received) > 0 {
		got := <-received
		paths[got.path] = true
		require.Equal(t, "NjE2MjYz", got.query)
	}
	require.Len(t, paths, 10)
}

func TestDoHTTPRequestFuzztagPairsURLQueryFormHeaders(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/users/{{int::row(1-2)}}", "method": "POST", "fuzztag": true,
		"query":   map[string]any{"value": "{{list::row(a & b|c+d)}}"},
		"form":    map[string]any{"value": "{{list::row(a & b|c+d)}}"},
		"headers": map[string]any{"X-Test": "{{list::row(one|two)}}"}, "concurrent": 2,
	})
	require.Equal(t, 2, utils.InterfaceToInt(semantic["request_count"]))
	require.Len(t, received, 2)
	require.ElementsMatch(t, []renderedHTTPRequest{
		{path: "/users/1", query: "a & b", form: "a & b", header: "one", body: "value=a+%26+b"},
		{path: "/users/2", query: "c+d", form: "c+d", header: "two", body: "value=c%2Bd"},
	}, []renderedHTTPRequest{<-received, <-received})
}

func TestDoHTTPRequestFuzztagPacketCartesianAndDuplicates(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"packet":    "POST /users/{{int(1-2)}}?value={{urlescape({{base64({{params(value)}})}})}} HTTP/1.1\r\nHost: " + strings.TrimPrefix(server.URL, "http://") + "\r\nX-Test: {{hex(abc)}}\r\n\r\n{{repeat(2)}}{{=literal {{int(1-10)}}=}}",
		"variables": map[string]any{"value": "a & b"}, "https": "no", "fuzztag": true, "concurrent": 2,
	})
	require.Equal(t, 4, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Len(t, received, 4)
	counts := map[string]int{}
	for len(received) > 0 {
		got := <-received
		counts[got.path]++
		require.Equal(t, "YSAmIGI=", got.query)
		require.Equal(t, "616263", got.header)
		require.Equal(t, "literal {{int(1-10)}}", got.body)
	}
	require.Equal(t, map[string]int{"/users/1": 2, "/users/2": 2}, counts)
}

func TestDoHTTPRequestFuzztagLimitAndCancellationSendNothing(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	for _, params := range []aitool.InvokeParams{
		{"url": server.URL + "/{{int(1-10)}}", "fuzztag": true, "max-requests": 9},
		{"packet": "GET /{{int(1-10)}} HTTP/1.1\r\nHost: " + strings.TrimPrefix(server.URL, "http://") + "\r\n\r\n", "fuzztag": true, "max-requests": 9},
		{"url": server.URL + "/{{int(1-2)}}", "query": map[string]any{"value": "{{list(a|b)}}"}, "fuzztag": true, "max-requests": 3},
	} {
		_, err := getDoHTTPRequestTool(t).InvokeWithParams(params)
		require.Error(t, err)
		require.Len(t, received, 0)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{"url": server.URL + "/{{int(1-10)}}", "fuzztag": true}, aitool.WithContext(ctx))
	require.Error(t, err)
	require.Len(t, received, 0)
}

func TestDoHTTPRequestFuzztagMixedOutcomesAndNoFormRetry(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.URL.Path == "/1" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("required field"))
			return
		}
		if r.URL.Path == "/2" {
			w.WriteHeader(500)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(server.Close)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/{{int(1-3)}}", "method": "POST", "body": `{"value":"test"}`,
		"content-type": "application/json", "fuzztag": true, "concurrent": 3, "timeout": 1,
	})
	require.Equal(t, int32(3), count.Load())
	require.Equal(t, 3, utils.InterfaceToInt(semantic["request_sent_count"]))
	require.Equal(t, 2, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Equal(t, 1, utils.InterfaceToInt(semantic["transport_error_count"]))
	items := utils.InterfaceToSliceInterface(semantic["items"])
	require.Equal(t, 400, utils.InterfaceToInt(utils.InterfaceToGeneralMap(items[0])["status_code"]))
	require.Equal(t, 500, utils.InterfaceToInt(utils.InterfaceToGeneralMap(items[1])["status_code"]))
	require.NotEmpty(t, utils.InterfaceToGeneralMap(items[2])["transport_error"])
}

func TestDoHTTPRequestFuzztagConcurrencyAndRedirectIsolation(t *testing.T) {
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/start/") {
			arrived <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			http.Redirect(w, r, "/final/"+strings.TrimPrefix(r.URL.Path, "/start/"), 302)
			return
		}
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	tool := getDoHTTPRequestTool(t)
	done := make(chan *aitool.ToolResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := tool.InvokeWithParams(aitool.InvokeParams{"url": server.URL + "/start/{{int(1-2)}}", "fuzztag": true, "concurrent": 2, "timeout": 5})
		done <- result
		errs <- err
	}()
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(4 * time.Second):
			t.Fatal("both requests must be in flight concurrently")
		}
	}
	release <- struct{}{}
	release <- struct{}{}
	require.NoError(t, <-errs)
	execution := (<-done).Data.(*aitool.ToolExecutionResult)
	items := utils.InterfaceToSliceInterface(utils.InterfaceToGeneralMap(execution.Result)["items"])
	require.Len(t, items, 2)
	for i, raw := range items {
		item := utils.InterfaceToGeneralMap(raw)
		require.Equal(t, 1, utils.InterfaceToInt(item["redirect_count"]))
		require.Contains(t, item["response"], fmt.Sprintf("/final/%d", i+1))
	}
}

func TestDoHTTPRequestFuzztagMultiPacketFilesAndPreview(t *testing.T) {
	response := strings.Repeat("x", 6000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
	t.Cleanup(server.Close)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": server.URL + "/{{int(1-2)}}", "fuzztag": true, "save-packet": true, "concurrent": 1, "regexp-match": "[",
	})
	items := utils.InterfaceToSliceInterface(semantic["items"])
	require.Len(t, items, 2)
	for i, raw := range items {
		item := utils.InterfaceToGeneralMap(raw)
		require.False(t, utils.InterfaceToBoolean(item["regexp_valid"]))
		require.True(t, utils.InterfaceToBoolean(item["response_truncated"]))
		require.Len(t, utils.InterfaceToString(item["response"]), 5120)
		for _, field := range []string{"request_file", "response_file"} {
			path := utils.InterfaceToString(item[field])
			require.NotEmpty(t, path)
			t.Cleanup(func() { _ = os.Remove(path) })
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			if field == "response_file" {
				require.Contains(t, string(data), response)
			} else {
				require.Contains(t, string(data), fmt.Sprintf("GET /%d HTTP/1.1", i+1))
			}
		}
	}
}
