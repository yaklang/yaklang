package test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	return utils.InterfaceToGeneralMap(execution.Result)
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

func TestDoHTTPRequestFuzztagRejectsMultipleAndInvalidBeforeSending(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	for _, template := range []string{"{{list(a|b)}}", "{{repeat(2)}}", "{{base64({{unknown()}})}}", "{{params(missing)}}", "{{unclosed"} {
		_, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{
			"url": server.URL, "query": map[string]any{"value": template}, "fuzztag": true,
		})
		require.Error(t, err, template)
	}
	require.Len(t, received, 0)
}

func TestBatchDoHTTPRequestFuzztagFieldsCartesianAndEncoding(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getBatchDoHTTPRequestTool(t), aitool.InvokeParams{
		"base-url": server.URL, "paths": "/users/{{int(1-2)}}", "method": "POST",
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

func TestBatchDoHTTPRequestFuzztagPairsPathQueryAndBody(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getBatchDoHTTPRequestTool(t), aitool.InvokeParams{
		"base-url": server.URL, "paths": "/users/{{int::row(1-2)}}", "method": "POST",
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

func TestBatchDoHTTPRequestFuzztagPacketNestedRawAndRepeat(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getBatchDoHTTPRequestTool(t), aitool.InvokeParams{
		"paths": "/test", "packet": "POST {{PATH}} HTTP/1.1\r\nHost: " + strings.TrimPrefix(server.URL, "http://") + "\r\nX-Test: {{base64({{hex({{params(value)}})}})}}\r\n\r\n{{repeat(3)}}{{={\"nested\":{\"value\":\"{{literal}}\"}}=}}",
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

func TestBatchDoHTTPRequestFuzztagAllOrNothingPreflight(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	for _, params := range []aitool.InvokeParams{
		{"base-url": server.URL, "paths": "/valid\n/{{params(missing)}}"},
		{"base-url": server.URL, "paths": "/valid\n/{{base64({{unknown()}})}}"},
		{"base-url": server.URL, "paths": "/valid\n/{{int(1-2)}}", "query": map[string]any{"value": "{{list(a|b)}}"}, "max-requests": 5},
		{"base-url": server.URL, "paths": "/valid\n/next", "body": "{{int(1-3)}}", "max-requests": 5},
	} {
		_, err := getBatchDoHTTPRequestTool(t).InvokeWithParams(params)
		require.Error(t, err)
		require.Len(t, received, 0)
	}
}

func TestBatchDoHTTPRequestFuzztagDisabledAndCanceled(t *testing.T) {
	server, received := fuzztagHTTPServer(t)
	semantic := invokeHTTPFuzztag(t, getBatchDoHTTPRequestTool(t), aitool.InvokeParams{
		"base-url": server.URL, "paths": "/test", "body": "{{unknown}} {{params(missing)}}", "method": "POST", "disable-fuzztag": true,
	})
	require.Equal(t, 1, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Equal(t, "{{unknown}} {{params(missing)}}", (<-received).body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := getBatchDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{"base-url": server.URL, "paths": "/{{int(1-500)}}", "max-requests": 500}, aitool.WithContext(ctx))
	require.Error(t, err)
	require.Len(t, received, 0)
}
