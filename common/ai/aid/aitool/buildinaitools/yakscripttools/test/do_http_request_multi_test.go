package test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func cleanupHTTPArtifacts(t *testing.T, result map[string]any) {
	t.Helper()
	paths := []string{utils.InterfaceToString(result["output_file"])}
	for _, raw := range utils.InterfaceToSliceInterface(result["items"]) {
		item := utils.InterfaceToGeneralMap(raw)
		paths = append(paths, utils.InterfaceToString(item["request_file"]), utils.InterfaceToString(item["response_file"]))
	}
	t.Cleanup(func() {
		for _, path := range paths {
			if path != "" {
				_ = os.Remove(path)
			}
		}
	})
}

func TestDoHTTPRequestMultiReportAndLiteralPathArrays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("endpoint=" + r.URL.Path + " token=match-" + strings.TrimPrefix(r.URL.Path, "/")))
	}))
	t.Cleanup(server.Close)
	result, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{
		"url": server.URL + "{{params(path)}}", "variables": map[string]any{"path": []any{"/one", "/two"}},
		"fuzztag": true, "keyword": "token=", "regexp-match": "match-[a-z]+", "concurrent": 2,
	})
	require.NoError(t, err)
	execution := result.Data.(*aitool.ToolExecutionResult)
	for _, text := range []string{"Batch Request Summary", "Total: 2", "Responses: 2", "Filtered: 0", "Results Summary Table", "request #1 packet", "request #2 packet", "Response:", "Keyword Match", "Regexp Match", "match-one", "match-two", "Full response:", "Results saved to:"} {
		require.Contains(t, execution.CombinedOutput, text)
	}
	semantic := utils.InterfaceToGeneralMap(execution.Result)
	cleanupHTTPArtifacts(t, semantic)
	require.Equal(t, 2, utils.InterfaceToInt(semantic["request_completed_count"]))
	report, err := os.ReadFile(utils.InterfaceToString(semantic["output_file"]))
	require.NoError(t, err)
	require.Contains(t, string(report), "Results Summary Table")
	items := utils.InterfaceToSliceInterface(semantic["items"])
	require.Len(t, items, 2)
	for i, path := range []string{"/one", "/two"} {
		item := utils.InterfaceToGeneralMap(items[i])
		require.Equal(t, path, item["path"])
		require.True(t, utils.InterfaceToBoolean(item["request_completed"]))
		require.Contains(t, item["response_preview"], "endpoint="+path)
		require.Contains(t, strings.Join(utils.InterfaceToStringSlice(item["keyword_matches"]), ""), "endpoint="+path)
		require.Contains(t, strings.Join(utils.InterfaceToStringSlice(item["regexp_matches"]), ""), "match-"+strings.TrimPrefix(path, "/"))
		packet, err := os.ReadFile(utils.InterfaceToString(item["response_file"]))
		require.NoError(t, err)
		require.Contains(t, string(packet), "endpoint="+path)
	}
}

func TestDoHTTPRequestMultiResponseFiltersPreserveEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			w.WriteHeader(404)
			_, _ = w.Write([]byte("missing-only-marker"))
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 500)))
		default:
			_, _ = w.Write([]byte("tiny-only-marker"))
		}
	}))
	t.Cleanup(server.Close)
	for _, tc := range []struct {
		name, include, exclude, size string
		filtered                     int
	}{
		{"excluded-code", "", "404", "", 1},
		{"included-code", "404", "", "", 2},
		{"excluded-size", "", "", "400-600", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{
				"url": server.URL + "{{params(path)}}", "variables": map[string]any{"path": []any{"/missing", "/big", "/small"}}, "fuzztag": true,
				"include-code": tc.include, "exclude-code": tc.exclude, "exclude-size": tc.size,
			})
			require.NoError(t, err)
			execution := result.Data.(*aitool.ToolExecutionResult)
			semantic := utils.InterfaceToGeneralMap(execution.Result)
			cleanupHTTPArtifacts(t, semantic)
			require.Equal(t, 3, utils.InterfaceToInt(semantic["response_received_count"]))
			require.Equal(t, tc.filtered, utils.InterfaceToInt(semantic["filtered_count"]))
			distribution := utils.InterfaceToGeneralMap(semantic["status_code_distribution"])
			require.Equal(t, 1, utils.InterfaceToInt(distribution["404"]))
			require.Equal(t, 2, utils.InterfaceToInt(distribution["200"]))
			for _, raw := range utils.InterfaceToSliceInterface(semantic["items"]) {
				item := utils.InterfaceToGeneralMap(raw)
				require.FileExists(t, utils.InterfaceToString(item["response_file"]))
				if utils.InterfaceToBoolean(item["filtered"]) {
					reason := tc.name
					if tc.include != "" {
						reason = "not-in-include-code"
					}
					require.Equal(t, reason, item["filter_reason"])
				}
			}
			if tc.name == "excluded-code" {
				require.NotContains(t, execution.CombinedOutput, "missing-only-marker")
			}
			if tc.name == "included-code" {
				require.NotContains(t, execution.CombinedOutput, "tiny-only-marker")
			}
		})
	}
}

func TestDoHTTPRequestMultiDelayAndCompleteTargets(t *testing.T) {
	var lock sync.Mutex
	var arrivals []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		arrivals = append(arrivals, time.Now())
		lock.Unlock()
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)
	semantic := invokeHTTPFuzztag(t, getDoHTTPRequestTool(t), aitool.InvokeParams{
		"url": "{{params(target)}}", "variables": map[string]any{"target": []any{server.URL + "/a,b", server.URL + "/中文"}},
		"fuzztag": true, "concurrent": 5, "delay-seconds": 0.05,
	})
	cleanupHTTPArtifacts(t, semantic)
	require.Equal(t, 2, utils.InterfaceToInt(semantic["response_received_count"]))
	lock.Lock()
	defer lock.Unlock()
	require.Len(t, arrivals, 2)
	require.GreaterOrEqual(t, arrivals[1].Sub(arrivals[0]), 40*time.Millisecond)
	items := utils.InterfaceToSliceInterface(semantic["items"])
	require.Contains(t, utils.InterfaceToGeneralMap(items[0])["url"], "/a,b")
}

func TestDoHTTPRequestMultiPreviewControls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 200))) }))
	t.Cleanup(server.Close)
	for _, limit := range []int{0, 50} {
		result, err := getDoHTTPRequestTool(t).InvokeWithParams(aitool.InvokeParams{"url": server.URL + "/{{int(1-2)}}", "fuzztag": true, "max-body-size": limit})
		require.NoError(t, err)
		execution := result.Data.(*aitool.ToolExecutionResult)
		semantic := utils.InterfaceToGeneralMap(execution.Result)
		cleanupHTTPArtifacts(t, semantic)
		for _, raw := range utils.InterfaceToSliceInterface(semantic["items"]) {
			item := utils.InterfaceToGeneralMap(raw)
			require.Len(t, utils.InterfaceToString(item["response_preview"]), limit)
			require.True(t, utils.InterfaceToBoolean(item["preview_truncated"]))
			packet, err := os.ReadFile(utils.InterfaceToString(item["response_file"]))
			require.NoError(t, err)
			require.Contains(t, string(packet), strings.Repeat("x", 200))
		}
		if limit == 0 {
			require.NotContains(t, execution.CombinedOutput, "Response:")
		} else {
			require.Contains(t, execution.CombinedOutput, "Response (truncated):")
		}
	}
}
