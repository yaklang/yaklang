package test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gotest.tools/v3/assert"
)

const networkSpaceSearchToolName = "network_space_search"

func getNetworkSpaceSearchTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/recon/network_space_search.yak")
	if err != nil {
		t.Fatalf("failed to read network_space_search.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(networkSpaceSearchToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse network_space_search.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execNetworkSpaceSearchTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Fatalf("tool execution failed: %v\nstderr: %s", err, w2.String())
	}
	return w1.String(), w2.String()
}

func TestNetworkSpaceSearch_ToolMetadata(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	assert.Assert(t, tool != nil, "tool should load successfully")
	assert.Assert(t, tool.Name == networkSpaceSearchToolName || tool.Name != "", "tool name should be set")
	t.Logf("tool loaded: name=%s, description length=%d", tool.Name, len(tool.Description))
}

// Override all six endpoints and credentials before invoking the unchanged Yak
// tool. A missing override cannot silently fall back to a developer's real keys.
func configureLocalSpaceEngines(t *testing.T, endpoint, key string) {
	t.Helper()
	previous := consts.AllThirdPartyApplicationConfig()
	consts.ClearThirdPartyApplicationConfig()
	for _, engine := range []string{"fofa", "shodan", "zoomeye", "hunter", "quake", "zone"} {
		consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
			Type: engine, Domain: endpoint, APIKey: key, UserIdentifier: "fixture@example.invalid",
		})
	}
	t.Cleanup(func() {
		consts.ClearThirdPartyApplicationConfig()
		for _, config := range previous {
			consts.UpdateThirdPartyApplicationConfig(config)
		}
	})
}

func TestNetworkSpaceSearch_InvalidInputsDoNotQuery(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected API request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	configureLocalSpaceEngines(t, server.URL, "fixture-key")
	tool := getNetworkSpaceSearchTool(t)
	for _, tc := range []struct{ engine, query, message string }{
		{"nonexistent_engine", "test query", "invalid engine"},
		{"fofa", " ", "query cannot be empty"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			out, stderr := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{"engine": tc.engine, "query": tc.query})
			require.Contains(t, out+stderr, tc.message)
		})
	}
	require.Zero(t, calls.Load(), "input validation must finish before any API request")
}

func TestNetworkSpaceSearch_QueryFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		requireQuery := r.URL.Query().Get("key")
		if requireQuery != "" {
			t.Errorf("missing-key fixture received key %q", requireQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"error":"API key not configured"}`)
	}))
	t.Cleanup(server.Close)
	configureLocalSpaceEngines(t, server.URL, "")
	out, stderr := execNetworkSpaceSearchTool(t, getNetworkSpaceSearchTool(t), aitool.InvokeParams{"engine": "shodan", "query": "apache"})
	require.EqualValues(t, 1, calls.Load())
	require.Contains(t, out+stderr, "query failed")
	require.Contains(t, out+stderr, "API key not configured")
	require.Contains(t, out+stderr, "HINT")
	require.NotContains(t, out, "Search Complete")
}

func TestNetworkSpaceSearch_AllValidEngineNames(t *testing.T) {
	tool := getNetworkSpaceSearchTool(t)
	for _, engine := range []string{"fofa", "shodan", "zoomeye", "hunter", "quake", "zone"} {
		t.Run(engine, func(t *testing.T) {
			var calls, searches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				query, key, response := "", "", ""
				expectedMethod := http.MethodGet
				switch engine {
				case "fofa":
					key = r.URL.Query().Get("key")
					if r.URL.Path == "/api/v1/info/my" {
						_, _ = io.WriteString(w, `{}`)
						return
					}
					if r.URL.Path != "/api/v1/search/all" {
						t.Errorf("unexpected FOFA path: %s", r.URL.Path)
					}
					raw, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("qbase64"))
					if err != nil {
						t.Error(err)
					}
					query = string(raw)
					if r.URL.Query().Get("email") != "fixture@example.invalid" {
						t.Error("FOFA email missing")
					}
					response = `{"results":[["http://127.0.0.1:8080","fixture-title","127.0.0.1","",8080,"CN","city"],["http://127.0.0.2:8080","unexpected-second-result","127.0.0.2","",8080,"CN","city"]]}`
				case "shodan":
					key = r.URL.Query().Get("key")
					if r.URL.Path == "/api-info" {
						_, _ = io.WriteString(w, `{}`)
						return
					}
					if r.URL.Path != "/shodan/host/search" {
						t.Errorf("unexpected Shodan path: %s", r.URL.Path)
					}
					query = r.URL.Query().Get("query")
					response = `{"total":2,"matches":[{"ip":2130706433,"port":8080,"http":{"title":"fixture-title"}},{"ip":2130706434,"port":8080,"http":{"title":"unexpected-second-result"}}]}`
				case "zoomeye":
					key = r.Header.Get("API-KEY")
					query = r.URL.Query().Get("query")
					if r.URL.Path != "/host/search" {
						t.Errorf("unexpected ZoomEye path: %s", r.URL.Path)
					}
					response = `{"matches":[{"ip":"127.0.0.1","portinfo":{"port":8080,"title":["fixture-title"]}},{"ip":"127.0.0.2","portinfo":{"port":8080,"title":["unexpected-second-result"]}}]}`
				case "hunter":
					key = r.URL.Query().Get("api-key")
					raw, err := base64.RawURLEncoding.DecodeString(r.URL.Query().Get("search"))
					if err != nil {
						t.Error(err)
					}
					query = string(raw)
					if r.URL.Path != "/openApi/search" {
						t.Errorf("unexpected Hunter path: %s", r.URL.Path)
					}
					response = `{"code":200,"data":{"total":2,"arr":[{"ip":"127.0.0.1","port":8080,"web_title":"fixture-title"},{"ip":"127.0.0.2","port":8080,"web_title":"unexpected-second-result"}]}}`
				case "quake", "zone":
					expectedMethod = http.MethodPost
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var params map[string]any
					if err := json.Unmarshal(body, &params); err != nil {
						t.Error(err)
					}
					query, _ = params["query"].(string)
					if engine == "quake" {
						key = r.Header.Get("X-QuakeToken")
						if r.URL.Path != "/api/v3/search/quake_service" {
							t.Errorf("unexpected Quake path: %s", r.URL.Path)
						}
						response = `{"code":0,"data":[{"ip":"127.0.0.1","port":8080,"service":{"http":{"title":"fixture-title"}}},{"ip":"127.0.0.2","port":8080,"service":{"http":{"title":"unexpected-second-result"}}}]}`
					} else {
						key, _ = params["zone_key_id"].(string)
						if r.URL.Path != "/api/data/" {
							t.Errorf("unexpected Zone path: %s", r.URL.Path)
						}
						response = `{"code":0,"data":[{"ip":"127.0.0.1","port":8080,"title":"fixture-title"},{"ip":"127.0.0.2","port":8080,"title":"unexpected-second-result"}]}`
					}
				}
				if r.Method != expectedMethod || query != "fixture query" || key != "fixture-key" {
					t.Errorf("incorrect %s request: method=%s query=%q key=%q", engine, r.Method, query, key)
					http.Error(w, "invalid fixture request", http.StatusBadRequest)
					return
				}
				searches.Add(1)
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(server.Close)
			configureLocalSpaceEngines(t, server.URL, "fixture-key")
			out, stderr := execNetworkSpaceSearchTool(t, tool, aitool.InvokeParams{"engine": engine, "query": "fixture query", "max-records": 1, "max-page": 1})
			require.Empty(t, stderr)
			require.Contains(t, out, "Search Complete")
			require.Contains(t, out, "Results found: 1")
			require.Contains(t, out, "127.0.0.1:8080")
			require.Contains(t, out, "fixture-title")
			require.NotContains(t, out, "unexpected-second-result")
			require.NotContains(t, out, "query failed")
			require.EqualValues(t, 1, searches.Load(), "max-records must stop further requests")
			expectedCalls := int32(1)
			if engine == "fofa" || engine == "shodan" {
				expectedCalls++
			}
			require.Equal(t, expectedCalls, calls.Load())
		})
	}
}
