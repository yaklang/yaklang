package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestHTTPFeedbackBatchPreview(t *testing.T) {
	tool := getBatchDoHTTPRequestTool(t)
	for _, body := range []string{"", "ascii", strings.Repeat("中文🙂", 40), "a\xff\x00b"} {
		for _, limit := range []int{0, 1, 100, 300, 4096} {
			t.Run(fmt.Sprintf("bytes=%d/limit=%d", len(body), limit), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.Equal(t, "POST", r.Method)
					_, _ = w.Write([]byte(body))
				}))
				defer server.Close()
				var stdout, stderr bytes.Buffer
				raw, err := tool.Callback(context.Background(), aitool.InvokeParams{
					"base-url": server.URL, "paths": "/a\n/b", "method": "POST", "max-body-size": limit,
				}, nil, &stdout, &stderr)
				require.NoError(t, err, stderr.String())
				require.EqualValues(t, 2, calls.Load(), "preview must never replay POST")
				require.True(t, utf8.ValidString(stdout.String()))
				result := utils.InterfaceToGeneralMap(raw)
				require.Equal(t, 2, utils.InterfaceToInt(result["response_received_count"]))
				for _, item := range utils.InterfaceToSliceInterface(result["items"]) {
					m := utils.InterfaceToGeneralMap(item)
					name := utils.InterfaceToString(m["response_file"])
					packet, err := os.ReadFile(name)
					require.NoError(t, err)
					require.True(t, bytes.HasSuffix(packet, []byte(body)), "full bytes must survive rendering")
					require.NoError(t, os.Remove(name))
				}
			})
		}
	}
}

func TestHTTPFeedbackRootAndEmptyHeaders(t *testing.T) {
	batch := getBatchDoHTTPRequestTool(t)
	single := getDoHTTPRequestTool(t)
	for _, prefix := range []string{"", "/", "/cms", "/cms/", "/中文"} {
		t.Run(prefix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := strings.TrimSuffix(prefix, "/")
				if want == "" {
					want = "/"
				}
				require.Equal(t, want, r.URL.Path)
				values, present := r.Header["Cookie"]
				require.True(t, present, "empty cookie must not be dropped or restored")
				require.Equal(t, []string{""}, values)
				require.Equal(t, "中文:tail", r.Header.Get("X-Test"))
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()
			var stdout, stderr bytes.Buffer
			_, err := batch.Callback(context.Background(), aitool.InvokeParams{
				"base-url": server.URL, "paths": "/", "prefix": prefix,
				"headers": map[string]any{"Cookie": "", "X-Test": "中文:tail"},
			}, nil, &stdout, &stderr)
			require.NoError(t, err, stderr.String())
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, []string{""}, r.Header["Cookie"])
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	for _, headers := range []any{map[string]any{"Cookie": ""}, "Cookie:", "Cookie:   "} {
		var stdout, stderr bytes.Buffer
		_, err := single.Callback(context.Background(), aitool.InvokeParams{"url": server.URL, "headers": headers}, nil, &stdout, &stderr)
		require.NoError(t, err, stderr.String())
	}
}

func TestHTTPFeedbackRejectsBeforeCallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tool    *aitool.Tool
		invalid []aitool.InvokeParams
		valid   []aitool.InvokeParams
	}{
		{"single", getDoHTTPRequestTool(t), []aitool.InvokeParams{
			{}, {"url": "http://localhost", "headers": ":"}, {"url": "http://localhost", "headers": map[string]any{"": ""}}, {"url": " "}, {"url": "http://localhost", "packet": "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"},
			{"url": "http://localhost", "method": `GET,"timeout":10`},
		}, []aitool.InvokeParams{{"url": "http://localhost"}, {"packet": "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"}, {"url": "http://localhost", "method": "PROPFIND"}}},
		{"batch", getBatchDoHTTPRequestTool(t), []aitool.InvokeParams{
			{}, {"base-url": "http://localhost"}, {"paths": "/"}, {"paths": "http://localhost/a\n/relative"},
			{"base-url": "http://localhost", "paths": "/", "max-body-size": -1},
			{"base-url": "http://localhost", "packet": "GET {{PATH}} HTTP/1.1\r\nHost: localhost\r\n\r\n", "paths": "/"},
		}, []aitool.InvokeParams{{"base-url": "http://localhost", "paths": "/"}, {"paths": "http://localhost/a\nhttps://localhost/b"}, {"packet": "GET {{PATH}} HTTP/1.1\r\nHost: localhost\r\n\r\n", "paths": "/"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var invoked atomic.Bool
			tc.tool.Callback = func(context.Context, aitool.InvokeParams, *aitool.ToolRuntimeConfig, io.Writer, io.Writer) (any, error) {
				invoked.Store(true)
				return nil, nil
			}
			for _, params := range tc.invalid {
				raw, marshalErr := json.Marshal(map[string]any{"@action": "call-tool", "tool": tc.tool.Name, "params": params})
				require.NoError(t, marshalErr)
				valid, _ := tc.tool.ValidateJSONString(string(raw))
				require.False(t, valid, "outer action schema must preserve parameter constraints")
				result, err := tc.tool.InvokeWithParams(params)
				require.Error(t, err, "%v", params)
				require.False(t, result.Success)
				require.False(t, invoked.Load(), "invalid params must not start callback: %v", params)
			}
			for _, params := range tc.valid {
				ok, errs := tc.tool.ValidateParams(params)
				require.True(t, ok, "%v: %v", params, errs)
			}
		})
	}
}
