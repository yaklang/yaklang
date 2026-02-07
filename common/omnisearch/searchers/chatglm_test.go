package searchers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

// mockChatGLMServer starts a mock HTTP server that implements ChatGLM's /api/paas/v4/web_search API contract.
func mockChatGLMServer(t *testing.T) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/paas/v4/web_search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Only accept POST
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "method_not_allowed",
					"message": "method not allowed, only POST is accepted",
				},
			})
			return
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "authentication_error",
					"message": "authentication required",
				},
			})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == "invalid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "authentication_error",
					"message": "invalid api key",
				},
			})
			return
		}

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "invalid_request",
					"message": "failed to read request body",
				},
			})
			return
		}
		defer r.Body.Close()

		var req ChatGLMSearchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "invalid_request",
					"message": "invalid JSON body",
				},
			})
			return
		}

		if req.SearchQuery == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "invalid_request",
					"message": "search_query is required",
				},
			})
			return
		}

		// Simulate error for specific queries
		if req.SearchQuery == "trigger_error" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "internal_error",
					"message": "internal server error",
				},
			})
			return
		}

		// Generate mock results based on count
		count := req.Count
		if count <= 0 {
			count = 10
		}
		if count > 50 {
			count = 50
		}

		results := make([]ChatGLMSearchResult, 0, count)
		for i := 0; i < count; i++ {
			results = append(results, ChatGLMSearchResult{
				Title:       fmt.Sprintf("Result %d for: %s", i+1, req.SearchQuery),
				Content:     fmt.Sprintf("This is the content of result %d for query: %s", i+1, req.SearchQuery),
				Link:        fmt.Sprintf("https://example.com/result/%d", i+1),
				Media:       fmt.Sprintf("example%d.com", i+1),
				Icon:        fmt.Sprintf("https://example.com/icon/%d.png", i+1),
				Refer:       fmt.Sprintf("[%d]", i+1),
				PublishDate: "2026-02-07",
			})
		}

		// Build response
		resp := ChatGLMSearchResponse{
			ID:        "mock-search-id-001",
			Created:   1707307200,
			RequestID: "mock-request-id",
			SearchIntent: []ChatGLMSearchIntent{
				{
					Query:    req.SearchQuery,
					Intent:   "SEARCH_ALWAYS",
					Keywords: req.SearchQuery,
				},
			},
			SearchResult: results,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	addr := listener.Addr().String()
	cleanup := func() {
		server.Close()
		listener.Close()
	}

	return addr, cleanup
}

func TestChatGLMSearch_BasicSearch(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:       "test-api-key",
		BaseURL:      fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:      10,
		MaxResults:   5,
		SearchEngine: "search_std",
	})

	resp, err := client.Search("Yaklang")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response should not be nil")
	}

	if len(resp.SearchResult) != 5 {
		t.Fatalf("expected 5 results, got %d", len(resp.SearchResult))
	}

	if resp.SearchResult[0].Title != "Result 1 for: Yaklang" {
		t.Fatalf("unexpected title: %s", resp.SearchResult[0].Title)
	}

	if resp.SearchResult[0].Link != "https://example.com/result/1" {
		t.Fatalf("unexpected link: %s", resp.SearchResult[0].Link)
	}
}

func TestChatGLMSearch_MissingAPIKey(t *testing.T) {
	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		BaseURL: "http://localhost:1234/api/paas/v4/web_search",
	})

	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestChatGLMSearch_EmptyQuery(t *testing.T) {
	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:  "test-key",
		BaseURL: "http://localhost:1234/api/paas/v4/web_search",
	})

	_, err := client.Search("")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "search query is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestChatGLMSearch_InvalidAPIKey(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:  "invalid-key",
		BaseURL: fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout: 10,
	})

	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	if !strings.Contains(err.Error(), "invalid api key") && !strings.Contains(err.Error(), "status code") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestChatGLMSearch_ServerError(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:     "test-api-key",
		BaseURL:    fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:    10,
		MaxResults: 5,
	})

	_, err := client.Search("trigger_error")
	if err == nil {
		t.Fatal("expected error for server error query")
	}
	if !strings.Contains(err.Error(), "internal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatGLMSearch_CustomParams(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:       "test-api-key",
		BaseURL:      fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:      10,
		MaxResults:   5,
		SearchEngine: "search_std",
	})

	resp, err := client.SearchWithCustomParams("Yaklang", &ChatGLMSearchRequest{
		Count:              20,
		SearchEngine:       "search_pro",
		SearchRecencyFilter: "oneWeek",
		ContentSize:        "high",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(resp.SearchResult) != 20 {
		t.Fatalf("expected 20 results, got %d", len(resp.SearchResult))
	}
}

func TestChatGLMSearch_CountClamping(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:     "test-api-key",
		BaseURL:    fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:    10,
		MaxResults: 100, // exceeds 50 max
	})

	resp, err := client.Search("test")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// Count should be clamped to 50
	if len(resp.SearchResult) > 50 {
		t.Fatalf("expected max 50 results, got %d", len(resp.SearchResult))
	}
}

func TestChatGLMSearch_SearchIntent(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:     "test-api-key",
		BaseURL:    fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:    10,
		MaxResults: 3,
	})

	resp, err := client.Search("test query")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(resp.SearchIntent) == 0 {
		t.Fatal("expected search intent results")
	}

	if resp.SearchIntent[0].Intent != "SEARCH_ALWAYS" {
		t.Fatalf("unexpected intent: %s", resp.SearchIntent[0].Intent)
	}

	if resp.SearchIntent[0].Query != "test query" {
		t.Fatalf("unexpected query in intent: %s", resp.SearchIntent[0].Query)
	}
}

func TestChatGLMSearch_ResultFields(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:     "test-api-key",
		BaseURL:    fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:    10,
		MaxResults: 1,
	})

	resp, err := client.Search("test")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(resp.SearchResult) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.SearchResult))
	}

	r := resp.SearchResult[0]
	if r.Title == "" {
		t.Fatal("title should not be empty")
	}
	if r.Content == "" {
		t.Fatal("content should not be empty")
	}
	if r.Link == "" {
		t.Fatal("link should not be empty")
	}
	if r.Media == "" {
		t.Fatal("media should not be empty")
	}
	if r.Icon == "" {
		t.Fatal("icon should not be empty")
	}
	if r.PublishDate == "" {
		t.Fatal("publish_date should not be empty")
	}
	if r.Refer == "" {
		t.Fatal("refer should not be empty")
	}
}

func TestChatGLMSearch_FormatResults(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:     "test-api-key",
		BaseURL:    fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		Timeout:    10,
		MaxResults: 3,
	})

	formatted, err := client.SearchFormatted("Yaklang")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(formatted, "Result 1 for: Yaklang") {
		t.Fatalf("formatted output missing expected content: %s", formatted)
	}
	if !strings.Contains(formatted, "Found 3 results") {
		t.Fatalf("formatted output missing result count: %s", formatted)
	}
}

func TestChatGLMSearch_ConnectionRefused(t *testing.T) {
	// Use a port that's not listening
	port := utils.GetRandomAvailableTCPPort()
	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:  "test-key",
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d/api/paas/v4/web_search", port),
		Timeout: 2,
	})

	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected connection refused error")
	}
}

// === OmniSearch Integration Tests ===

func TestOmniChatGLMSearch_BasicSearch(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewOmniChatGLMSearchClient()

	if client.GetType() != ostype.SearcherTypeChatGLM {
		t.Fatalf("expected type %s, got %s", ostype.SearcherTypeChatGLM, client.GetType())
	}

	config := &ostype.SearchConfig{
		ApiKey:   "test-api-key",
		BaseURL:  fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		PageSize: 5,
		Page:     1,
		Extra:    map[string]interface{}{},
	}

	results, err := client.Search("Yaklang", config)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	if results[0].Source != "chatglm" {
		t.Fatalf("expected source 'chatglm', got '%s'", results[0].Source)
	}

	if results[0].Title != "Result 1 for: Yaklang" {
		t.Fatalf("unexpected title: %s", results[0].Title)
	}

	if results[0].URL != "https://example.com/result/1" {
		t.Fatalf("unexpected URL: %s", results[0].URL)
	}

	if results[0].FaviconURL != "https://example.com/icon/1.png" {
		t.Fatalf("unexpected favicon: %s", results[0].FaviconURL)
	}
}

func TestOmniChatGLMSearch_WithSearchEngineExtra(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewOmniChatGLMSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "test-api-key",
		BaseURL:  fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		PageSize: 3,
		Page:     1,
		Extra: map[string]interface{}{
			"search_engine":        "search_pro",
			"content_size":         "high",
			"search_recency_filter": "oneWeek",
		},
	}

	results, err := client.Search("test", config)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestOmniChatGLMSearch_Pagination(t *testing.T) {
	addr, cleanup := mockChatGLMServer(t)
	defer cleanup()

	client := NewOmniChatGLMSearchClient()

	// Page 1
	config1 := &ostype.SearchConfig{
		ApiKey:   "test-api-key",
		BaseURL:  fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		PageSize: 5,
		Page:     1,
		Extra:    map[string]interface{}{},
	}

	results1, err := client.Search("test", config1)
	if err != nil {
		t.Fatalf("page 1 search failed: %v", err)
	}

	if len(results1) != 5 {
		t.Fatalf("page 1: expected 5 results, got %d", len(results1))
	}

	// Page 2
	config2 := &ostype.SearchConfig{
		ApiKey:   "test-api-key",
		BaseURL:  fmt.Sprintf("http://%s/api/paas/v4/web_search", addr),
		PageSize: 5,
		Page:     2,
		Extra:    map[string]interface{}{},
	}

	results2, err := client.Search("test", config2)
	if err != nil {
		t.Fatalf("page 2 search failed: %v", err)
	}

	if len(results2) != 5 {
		t.Fatalf("page 2: expected 5 results, got %d", len(results2))
	}

	// Page 2 results should start from result 6
	if results2[0].Title != "Result 6 for: test" {
		t.Fatalf("page 2 first result should be Result 6, got: %s", results2[0].Title)
	}
}

// === Real API Test (requires API key) ===

func TestChatGLMSearch_RealAPI(t *testing.T) {
	// Read API key from file
	apiKeyBytes, err := os.ReadFile(os.ExpandEnv("$HOME/yakit-projects/glm.txt"))
	if err != nil {
		t.Skipf("skipping real API test: cannot read api key file: %v", err)
	}
	apiKey := strings.TrimSpace(string(apiKeyBytes))
	if apiKey == "" {
		t.Skip("skipping real API test: api key is empty")
	}

	client := NewChatGLMSearchClient(&ChatGLMSearchConfig{
		APIKey:              apiKey,
		BaseURL:             "https://open.bigmodel.cn/api/paas/v4/web_search",
		Timeout:             15,
		MaxResults:          5,
		SearchEngine:        "search_std",
		SearchRecencyFilter: "noLimit",
		ContentSize:         "medium",
	})

	resp, err := client.Search("Yaklang")
	if err != nil {
		t.Fatalf("real API search failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response should not be nil")
	}

	t.Logf("ChatGLM search returned %d results for 'Yaklang'", len(resp.SearchResult))
	for i, r := range resp.SearchResult {
		t.Logf("  [%d] %s - %s", i+1, r.Title, r.Link)
		if r.Content != "" {
			content := r.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			t.Logf("       %s", content)
		}
	}

	if len(resp.SearchResult) == 0 {
		t.Error("expected at least one search result for 'Yaklang'")
	}

	// Verify result fields
	for _, r := range resp.SearchResult {
		if r.Title == "" {
			t.Error("result title should not be empty")
		}
		if r.Link == "" {
			t.Error("result link should not be empty")
		}
		if r.Content == "" {
			t.Error("result content should not be empty")
		}
	}
}

func TestOmniChatGLMSearch_RealAPI(t *testing.T) {
	apiKeyBytes, err := os.ReadFile(os.ExpandEnv("$HOME/yakit-projects/glm.txt"))
	if err != nil {
		t.Skipf("skipping real API test: cannot read api key file: %v", err)
	}
	apiKey := strings.TrimSpace(string(apiKeyBytes))
	if apiKey == "" {
		t.Skip("skipping real API test: api key is empty")
	}

	client := NewOmniChatGLMSearchClient()
	config := &ostype.SearchConfig{
		ApiKey:   apiKey,
		PageSize: 10,
		Page:     1,
		Extra: map[string]interface{}{
			"search_engine": "search_std",
			"content_size":  "medium",
		},
	}

	results, err := client.Search("Yaklang", config)
	if err != nil {
		t.Fatalf("real OmniSearch API search failed: %v", err)
	}

	t.Logf("OmniSearch ChatGLM returned %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] %s - %s", i+1, r.Title, r.URL)
	}

	if len(results) == 0 {
		t.Error("expected at least one result from OmniSearch ChatGLM")
	}

	for _, r := range results {
		if r.Source != "chatglm" {
			t.Errorf("expected source 'chatglm', got '%s'", r.Source)
		}
	}
}
