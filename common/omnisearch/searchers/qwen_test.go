package searchers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

func mockQwenServer(t *testing.T, assertRequest func(*testing.T, *qwenRequest)) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/services/aigc/text-generation/generation", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"code":    "InvalidApiKey",
				"message": "missing api key",
			})
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req qwenRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if assertRequest != nil {
			assertRequest(t, &req)
		}

		resp := map[string]any{
			"output": map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": "这是 DashScope 的综合回答。",
					},
					"finish_reason": "stop",
					"index":         0,
				}},
				"search_info": map[string]any{
					"extra_tool_info": []map[string]any{{
						"tool":   "vertical-search",
						"result": "这是官方直接返回的网页正文片段。",
					}},
					"search_results": []map[string]any{
						{
							"index":     1,
							"title":     "Yaklang 官方介绍",
							"url":       "https://yaklang.com/intro",
							"site_name": "yaklang",
							"icon":      "https://yaklang.com/favicon.ico",
						},
						{
							"index":     2,
							"title":     "Yaklang 文档",
							"url":       "https://yaklang.com/docs",
							"site_name": "yaklang",
							"icon":      "https://yaklang.com/docs.ico",
							"content":   "这是官方直接返回的网页正文片段。",
						},
						{
							"index":     3,
							"title":     "Yaklang 仓库",
							"url":       "https://github.com/yaklang/yaklang",
							"site_name": "GitHub",
							"icon":      "https://github.com/favicon.ico",
						},
						{
							"index":     4,
							"title":     "Yaklang 新闻",
							"url":       "https://example.com/yaklang-news",
							"site_name": "example",
							"icon":      "https://example.com/favicon.ico",
						},
					},
				},
			},
			"usage": map[string]any{
				"plugins": map[string]any{
					"search": map[string]any{
						"count":    1,
						"strategy": "max",
					},
				},
			},
			"request_id": "mock-qwen-request",
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start qwen mock server: %v", err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	cleanup := func() {
		server.Close()
		listener.Close()
	}

	return fmt.Sprintf("http://%s/api/v1/services/aigc/text-generation/generation", listener.Addr().String()), cleanup
}

func TestQwenSearch_NativeSearchInfo(t *testing.T) {
	baseURL, cleanup := mockQwenServer(t, func(t *testing.T, req *qwenRequest) {
		if req.Model != "qwen-plus" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		if req.Input == nil || len(req.Input.Messages) != 1 {
			t.Fatalf("unexpected input messages: %+v", req.Input)
		}
		if req.Input.Messages[0].Role != "user" || req.Input.Messages[0].Content != "Yaklang 是什么？" {
			t.Fatalf("unexpected message: %+v", req.Input.Messages[0])
		}
		if req.Parameters == nil {
			t.Fatal("parameters should not be nil")
		}
		if !req.Parameters.EnableSearch {
			t.Fatal("enable_search should be true")
		}
		if req.Parameters.ResultFormat != "message" {
			t.Fatalf("unexpected result_format: %s", req.Parameters.ResultFormat)
		}
		if req.Parameters.EnableThinking == nil || *req.Parameters.EnableThinking {
			t.Fatal("enable_thinking should be explicitly false")
		}
		if req.Parameters.SearchOptions == nil {
			t.Fatal("search_options should not be nil")
		}
		if !req.Parameters.SearchOptions.EnableSource {
			t.Fatal("enable_source should be true")
		}
		if !req.Parameters.SearchOptions.ForcedSearch {
			t.Fatal("forced_search should be true")
		}
		if req.Parameters.SearchOptions.SearchStrategy != "max" {
			t.Fatalf("unexpected search_strategy: %s", req.Parameters.SearchOptions.SearchStrategy)
		}
		if !req.Parameters.SearchOptions.EnableCitation {
			t.Fatal("enable_citation should be true")
		}
		if req.Parameters.SearchOptions.CitationFormat != "[ref_<number>]" {
			t.Fatalf("unexpected citation_format: %s", req.Parameters.SearchOptions.CitationFormat)
		}
		if !req.Parameters.SearchOptions.EnableSearchExtension {
			t.Fatal("enable_search_extension should be true")
		}
	})
	defer cleanup()

	client := NewQwenSearchClient(&QwenSearchConfig{
		APIKey:                "test-key",
		BaseURL:               baseURL,
		Model:                 "qwen-plus",
		Timeout:               10,
		SearchStrategy:        "max",
		ForcedSearch:          true,
		MaxTokens:             256,
		EnableCitation:        true,
		CitationFormat:        "[ref_<number>]",
		EnableSearchExtension: true,
	})

	resp, err := client.Search("Yaklang 是什么？")
	if err != nil {
		t.Fatalf("qwen search failed: %v", err)
	}
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.Summary != "这是 DashScope 的综合回答。" {
		t.Fatalf("unexpected summary: %s", resp.Summary)
	}
	if len(resp.SearchResults) != 4 {
		t.Fatalf("expected 4 search results, got %d", len(resp.SearchResults))
	}
	if resp.SearchResults[1].URL != "https://yaklang.com/docs" {
		t.Fatalf("unexpected url: %s", resp.SearchResults[1].URL)
	}
	if resp.SearchResults[1].Content != "这是官方直接返回的网页正文片段。" {
		t.Fatalf("unexpected content: %s", resp.SearchResults[1].Content)
	}
	if len(resp.ExtraToolInfo) != 1 {
		t.Fatalf("expected extra_tool_info, got %+v", resp.ExtraToolInfo)
	}
}

func TestQwenSearch_MissingAPIKey(t *testing.T) {
	client := NewQwenSearchClient(&QwenSearchConfig{
		BaseURL: "http://127.0.0.1:1/api/v1/services/aigc/text-generation/generation",
	})

	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error for missing api key")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOmniQwenSearchClient_PaginationAndOfficialContent(t *testing.T) {
	baseURL, cleanup := mockQwenServer(t, nil)
	defer cleanup()

	client := NewOmniQwenSearchClient()
	results, err := client.Search("Yaklang 是什么？", &ostype.SearchConfig{
		ApiKey:   "test-key",
		BaseURL:  baseURL,
		PageSize: 2,
		Page:     2,
		Extra: map[string]any{
			"model":         "qwen-plus",
			"forced_search": true,
		},
	})
	if err != nil {
		t.Fatalf("omni qwen search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].URL != "https://github.com/yaklang/yaklang" {
		t.Fatalf("unexpected paged url: %s", results[0].URL)
	}
	if results[0].Content != "[vertical-search]\n这是官方直接返回的网页正文片段。" {
		t.Fatalf("expected official extra_tool_info content fallback, got %s", results[0].Content)
	}
	if results[0].Summary != "这是 DashScope 的综合回答。" {
		t.Fatalf("unexpected summary: %s", results[0].Summary)
	}
	if results[0].Source != "qwen" {
		t.Fatalf("unexpected source: %s", results[0].Source)
	}
	data, ok := results[1].Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", results[1].Data)
	}
	if data["site_name"] != "example" {
		t.Fatalf("unexpected site_name in data: %+v", data)
	}
	if data["display_content"] != "[vertical-search]\n这是官方直接返回的网页正文片段。" {
		t.Fatalf("unexpected display_content in data: %+v", data)
	}
}
