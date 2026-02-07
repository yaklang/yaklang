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
	"github.com/yaklang/yaklang/common/utils"
)

// mockAiBalanceServer starts a mock HTTP server that implements the /v1/web-search API contract.
// It returns the server address (host:port) and a cleanup function.
func mockAiBalanceServer(t *testing.T) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/web-search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Only accept POST
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "method not allowed",
					"type":    "invalid_request_error",
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
					"message": "authentication required",
					"type":    "authentication_error",
				},
			})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "invalid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "invalid api key",
					"type":    "authentication_error",
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
					"message": "failed to read body",
					"type":    "invalid_request_error",
				},
			})
			return
		}
		defer r.Body.Close()

		var req AiBalanceSearchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "invalid request body",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if req.Query == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "query is required",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Simulate error for specific query
		if req.Query == "trigger-error" {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "all search api keys failed",
					"type":    "upstream_error",
				},
			})
			return
		}

		// Validate searcher type
		searcherType := req.SearcherType
		if searcherType == "" {
			searcherType = "brave"
		}
		if searcherType != "brave" && searcherType != "tavily" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "searcher_type must be 'brave' or 'tavily'",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Generate mock results
		pageSize := req.PageSize
		if pageSize <= 0 {
			pageSize = 10
		}
		page := req.Page
		if page <= 0 {
			page = 1
		}

		// Generate fewer results to simulate real behavior
		resultCount := pageSize
		if resultCount > 5 {
			resultCount = 5
		}

		results := make([]*ostype.OmniSearchResult, 0, resultCount)
		for i := 0; i < resultCount; i++ {
			results = append(results, &ostype.OmniSearchResult{
				Title:   fmt.Sprintf("Result %d for: %s", i+1, req.Query),
				URL:     fmt.Sprintf("https://example.com/%s/%d", req.Query, i+1),
				Content: fmt.Sprintf("Content for result %d, query=%s, type=%s, page=%d", i+1, req.Query, searcherType, page),
				Source:  searcherType,
			})
		}

		resp := &AiBalanceSearchResponse{
			Results:      results,
			Total:        len(results),
			SearcherType: searcherType,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	// Start on a random available port
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

// TestAiBalanceSearchClient_BasicSearch tests basic search functionality through the AiBalance relay
func TestAiBalanceSearchClient_BasicSearch(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	resp, err := client.Search("yaklang", 1, 5)
	if err != nil {
		t.Fatalf("search should succeed: %v", err)
	}

	if resp == nil {
		t.Fatal("response should not be nil")
	}

	if resp.Total == 0 {
		t.Fatal("should have at least one result")
	}

	if resp.SearcherType != "brave" {
		t.Fatalf("expected searcher_type 'brave', got '%s'", resp.SearcherType)
	}

	// Verify result content
	for _, result := range resp.Results {
		if result.Title == "" {
			t.Error("result title should not be empty")
		}
		if result.URL == "" {
			t.Error("result URL should not be empty")
		}
		if !strings.Contains(result.Content, "yaklang") {
			t.Errorf("result content should contain query, got: %s", result.Content)
		}
	}

	t.Logf("basic search returned %d results", resp.Total)
}

// TestAiBalanceSearchClient_TavilyBackend tests search with tavily backend type
func TestAiBalanceSearchClient_TavilyBackend(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "tavily",
		Timeout:             10,
	})

	resp, err := client.Search("security testing", 1, 3)
	if err != nil {
		t.Fatalf("search with tavily backend should succeed: %v", err)
	}

	if resp.SearcherType != "tavily" {
		t.Fatalf("expected searcher_type 'tavily', got '%s'", resp.SearcherType)
	}

	for _, result := range resp.Results {
		if !strings.Contains(result.Content, "tavily") {
			t.Errorf("result should reference tavily backend, got: %s", result.Content)
		}
	}

	t.Logf("tavily backend search returned %d results", resp.Total)
}

// TestAiBalanceSearchClient_Pagination tests pagination parameters are correctly passed
func TestAiBalanceSearchClient_Pagination(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	// Test page 2
	resp, err := client.Search("pagination-test", 2, 3)
	if err != nil {
		t.Fatalf("paginated search should succeed: %v", err)
	}

	if resp == nil || resp.Total == 0 {
		t.Fatal("should return results for page 2")
	}

	// Verify the page parameter was passed correctly
	for _, result := range resp.Results {
		if !strings.Contains(result.Content, "page=2") {
			t.Errorf("result content should reference page=2, got: %s", result.Content)
		}
	}

	t.Logf("pagination test returned %d results for page 2", resp.Total)
}

// TestAiBalanceSearchClient_AuthenticationRequired tests missing API key
func TestAiBalanceSearchClient_AuthenticationRequired(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "", // No API key
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	_, err := client.Search("test", 1, 5)
	if err == nil {
		t.Fatal("search without API key should fail")
	}

	if !strings.Contains(err.Error(), "api key") {
		t.Fatalf("error should mention api key, got: %v", err)
	}

	t.Logf("correctly rejected: %v", err)
}

// TestAiBalanceSearchClient_InvalidToken tests invalid Bearer token
func TestAiBalanceSearchClient_InvalidToken(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "invalid-token",
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	_, err := client.Search("test", 1, 5)
	if err == nil {
		t.Fatal("search with invalid token should fail")
	}

	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("error should mention invalid api key, got: %v", err)
	}

	t.Logf("correctly rejected invalid token: %v", err)
}

// TestAiBalanceSearchClient_EmptyQuery tests empty query validation
func TestAiBalanceSearchClient_EmptyQuery(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	_, err := client.Search("", 1, 5)
	if err == nil {
		t.Fatal("search with empty query should fail")
	}

	if !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("error should mention query required, got: %v", err)
	}

	t.Logf("correctly rejected empty query: %v", err)
}

// TestAiBalanceSearchClient_UpstreamError tests handling of upstream errors
func TestAiBalanceSearchClient_UpstreamError(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://%s", addr),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "brave",
		Timeout:             10,
	})

	_, err := client.Search("trigger-error", 1, 5)
	if err == nil {
		t.Fatal("search with trigger-error query should fail")
	}

	if !strings.Contains(err.Error(), "all search api keys failed") {
		t.Fatalf("error should mention upstream failure, got: %v", err)
	}

	t.Logf("correctly handled upstream error: %v", err)
}

// TestAiBalanceSearchClient_ConnectionRefused tests behavior when server is not reachable
func TestAiBalanceSearchClient_ConnectionRefused(t *testing.T) {
	// Use a port that is not listening
	port := utils.GetRandomAvailableTCPPort()
	client := NewAiBalanceSearchClient(&AiBalanceSearchConfig{
		BaseURL:             fmt.Sprintf("http://127.0.0.1:%d", port),
		APIKey:              "test-bearer-token",
		BackendSearcherType: "brave",
		Timeout:             3,
	})

	_, err := client.Search("test", 1, 5)
	if err == nil {
		t.Fatal("search to unreachable server should fail")
	}

	t.Logf("correctly handled connection error: %v", err)
}

// TestOmniAiBalanceSearchClient_SearchClientInterface tests the OmniAiBalanceSearchClient
// which implements the ostype.SearchClient interface
func TestOmniAiBalanceSearchClient_SearchClientInterface(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	omniClient := NewOmniAiBalanceSearchClient()

	// Verify type
	if omniClient.GetType() != ostype.SearcherTypeAiBalance {
		t.Fatalf("expected type 'aibalance', got '%s'", omniClient.GetType())
	}

	// Search with config
	config := &ostype.SearchConfig{
		ApiKey:   "test-bearer-token",
		BaseURL:  fmt.Sprintf("http://%s", addr),
		Page:     1,
		PageSize: 5,
		Extra:    map[string]interface{}{},
	}

	results, err := omniClient.Search("omnisearch test", config)
	if err != nil {
		t.Fatalf("omni search should succeed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("should have at least one result")
	}

	for _, result := range results {
		if result.Title == "" {
			t.Error("result title should not be empty")
		}
		if !strings.Contains(result.Content, "omnisearch test") {
			t.Errorf("result content should contain query, got: %s", result.Content)
		}
	}

	t.Logf("omni search returned %d results", len(results))
}

// TestOmniAiBalanceSearchClient_BackendSearcherTypeViaExtra tests specifying backend searcher type via Extra map
func TestOmniAiBalanceSearchClient_BackendSearcherTypeViaExtra(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	omniClient := NewOmniAiBalanceSearchClient()

	// Use Extra map to specify tavily backend
	config := &ostype.SearchConfig{
		ApiKey:   "test-bearer-token",
		BaseURL:  fmt.Sprintf("http://%s", addr),
		Page:     1,
		PageSize: 3,
		Extra: map[string]interface{}{
			"backend_searcher_type": "tavily",
		},
	}

	results, err := omniClient.Search("backend test", config)
	if err != nil {
		t.Fatalf("search with tavily backend via Extra should succeed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("should have at least one result")
	}

	// Verify results reference tavily
	for _, result := range results {
		if !strings.Contains(result.Content, "tavily") {
			t.Errorf("result should reference tavily backend, got: %s", result.Content)
		}
	}

	t.Logf("backend searcher type via Extra returned %d results with tavily backend", len(results))
}

// TestOmniAiBalanceSearchClient_DefaultBackendIsBrave tests that default backend is brave
func TestOmniAiBalanceSearchClient_DefaultBackendIsBrave(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	omniClient := NewOmniAiBalanceSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "test-bearer-token",
		BaseURL:  fmt.Sprintf("http://%s", addr),
		Page:     1,
		PageSize: 3,
		Extra:    map[string]interface{}{},
	}

	results, err := omniClient.Search("default backend test", config)
	if err != nil {
		t.Fatalf("search with default backend should succeed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("should have at least one result")
	}

	// Verify results reference brave (default)
	for _, result := range results {
		if !strings.Contains(result.Content, "brave") {
			t.Errorf("result should reference brave backend by default, got: %s", result.Content)
		}
	}

	t.Logf("default backend (brave) returned %d results", len(results))
}

// TestOmniAiBalanceSearchClient_AuthError tests authentication error through OmniSearchClient
func TestOmniAiBalanceSearchClient_AuthError(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	omniClient := NewOmniAiBalanceSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "invalid-token",
		BaseURL:  fmt.Sprintf("http://%s", addr),
		Page:     1,
		PageSize: 5,
		Extra:    map[string]interface{}{},
	}

	_, err := omniClient.Search("auth error test", config)
	if err == nil {
		t.Fatal("search with invalid token should fail through omni client")
	}

	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("error should mention invalid api key, got: %v", err)
	}

	t.Logf("omni client correctly rejected invalid token: %v", err)
}

// TestOmniAiBalanceSearchClient_ResultFieldsComplete tests that all result fields are properly populated
func TestOmniAiBalanceSearchClient_ResultFieldsComplete(t *testing.T) {
	addr, cleanup := mockAiBalanceServer(t)
	defer cleanup()

	omniClient := NewOmniAiBalanceSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "test-bearer-token",
		BaseURL:  fmt.Sprintf("http://%s", addr),
		Page:     1,
		PageSize: 3,
		Extra:    map[string]interface{}{},
	}

	results, err := omniClient.Search("field test", config)
	if err != nil {
		t.Fatalf("search should succeed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("should have results")
	}

	for i, result := range results {
		if result.Title == "" {
			t.Errorf("result[%d] Title should not be empty", i)
		}
		if result.URL == "" {
			t.Errorf("result[%d] URL should not be empty", i)
		}
		if result.Content == "" {
			t.Errorf("result[%d] Content should not be empty", i)
		}
		if result.Source == "" {
			t.Errorf("result[%d] Source should not be empty", i)
		}
	}

	t.Logf("all %d results have complete fields", len(results))
}
