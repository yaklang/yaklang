package test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

// Compare the same 24 requests with sequential network execution in both cases.
// This measures tool invocation/rendering overhead, rather than attributing the
// independent benefit of network concurrency to template rendering.
func BenchmarkHTTPFuzztagInvocation(b *testing.B) {
	for _, batch := range []bool{false, true} {
		name := "individual"
		toolName := "do_http_request"
		if batch {
			name = "batch"
		}
		b.Run(name, func(b *testing.B) {
			source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/http/" + toolName + ".yak")
			require.NoError(b, err)
			metadata := yakscripttools.LoadYakScriptToAiTools(toolName, string(source))
			tool := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})[0]
			var received atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if batch {
					_, err := tool.InvokeWithParams(aitool.InvokeParams{
						"url": server.URL + "/users/{{int(1-12)}}", "fuzztag": true, "concurrent": 1, "max-requests": 24,
						"query": map[string]any{"value": "{{base64({{list(a|b)}})}}"},
					})
					require.NoError(b, err)
				} else {
					for id := 1; id <= 12; id++ {
						for _, payload := range []string{"a", "b"} {
							_, err := tool.InvokeWithParams(aitool.InvokeParams{
								"url": server.URL + "/users/" + strconv.Itoa(id), "fuzztag": true,
								"query": map[string]any{"value": "{{base64(" + payload + ")}}"},
							})
							require.NoError(b, err)
						}
					}
				}
			}
			b.StopTimer()
			require.EqualValues(b, 24*b.N, received.Load())
			calls := 24
			if batch {
				calls = 1
			}
			b.ReportMetric(float64(calls), "tool-calls/op")
			b.ReportMetric(24, "requests/op")
		})
	}
}
