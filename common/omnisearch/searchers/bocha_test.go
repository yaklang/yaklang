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

// mockBochaServer starts a mock HTTP server that implements Bocha AI's /v1/web-search API contract.
func mockBochaServer(t *testing.T) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/web-search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Only accept POST
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 405,
				"msg":  "method not allowed",
			})
			return
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 401,
				"msg":  "authentication required",
			})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == "invalid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 401,
				"msg":  "invalid api key",
			})
			return
		}

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 400,
				"msg":  "failed to read request body",
			})
			return
		}
		defer r.Body.Close()

		var req BochaSearchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 400,
				"msg":  "invalid JSON body",
			})
			return
		}

		if req.Query == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 400,
				"msg":  "query is required",
			})
			return
		}

		// Simulate server error for specific queries
		if req.Query == "trigger_error" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 500,
				"msg":  "internal server error",
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

		results := make([]BochaWebResult, 0, count)
		for i := 0; i < count; i++ {
			result := BochaWebResult{
				Name:             fmt.Sprintf("Mock Result %d for '%s'", i+1, req.Query),
				URL:              fmt.Sprintf("https://example.com/result/%d", i+1),
				DisplayURL:       fmt.Sprintf("https://example.com/result/%d", i+1),
				Snippet:          fmt.Sprintf("This is the snippet for result %d matching query '%s'", i+1, req.Query),
				SiteName:         "Example",
				DateLastCrawled:  "2025-01-01T00:00:00Z",
				Language:         "zh-CN",
				IsFamilyFriendly: true,
				IsNavigational:   false,
			}
			if req.Summary {
				result.Summary = fmt.Sprintf("Detailed summary for result %d matching query '%s'", i+1, req.Query)
			}
			results = append(results, result)
		}

		resp := BochaSearchResponse{
			Code:  200,
			LogID: "mock-log-id-123",
			Data: &BochaData{
				Type: "SearchResponse",
				QueryContext: &BochaQueryContext{
					OriginalQuery: req.Query,
				},
				WebPages: &BochaWebPages{
					TotalEstimatedMatches: count * 10,
					Value:                 results,
				},
			},
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

	addr := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	return addr, func() {
		server.Close()
		listener.Close()
	}
}

func TestBochaSearch_Basic(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "test-api-key"
	config.BaseURL = addr + "/v1/web-search"

	client := NewBochaSearchClient(config)
	resp, err := client.Search("Yaklang")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if resp.Data == nil || resp.Data.WebPages == nil {
		t.Fatal("expected non-nil web pages data")
	}

	if len(resp.Data.WebPages.Value) == 0 {
		t.Fatal("expected at least one result")
	}

	if !strings.Contains(resp.Data.WebPages.Value[0].Name, "Yaklang") {
		t.Errorf("expected result to contain 'Yaklang', got: %s", resp.Data.WebPages.Value[0].Name)
	}
}

func TestBochaSearch_MissingAPIKey(t *testing.T) {
	config := NewDefaultBochaConfig()
	config.APIKey = ""

	client := NewBochaSearchClient(config)
	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBochaSearch_EmptyQuery(t *testing.T) {
	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"

	client := NewBochaSearchClient(config)
	_, err := client.Search("")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBochaSearch_InvalidAPIKey(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "invalid-key"
	config.BaseURL = addr + "/v1/web-search"

	client := NewBochaSearchClient(config)
	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestBochaSearch_ServerError(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"
	config.BaseURL = addr + "/v1/web-search"

	client := NewBochaSearchClient(config)
	_, err := client.Search("trigger_error")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestBochaSearch_CustomParams(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"
	config.BaseURL = addr + "/v1/web-search"
	config.Summary = true

	client := NewBochaSearchClient(config)
	resp, err := client.SearchWithCustomParams("Yaklang", &BochaSearchRequest{
		Count:     5,
		Freshness: "oneYear",
		Summary:   true,
	})
	if err != nil {
		t.Fatalf("search with custom params failed: %v", err)
	}

	if len(resp.Data.WebPages.Value) != 5 {
		t.Errorf("expected 5 results, got %d", len(resp.Data.WebPages.Value))
	}

	// Check summary is present when requested
	if resp.Data.WebPages.Value[0].Summary == "" {
		t.Error("expected non-empty summary when summary=true")
	}
}

func TestBochaSearch_CountClamping(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"
	config.BaseURL = addr + "/v1/web-search"
	config.MaxResults = 100 // exceeds max

	client := NewBochaSearchClient(config)
	resp, err := client.Search("test")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(resp.Data.WebPages.Value) > 50 {
		t.Errorf("expected at most 50 results, got %d", len(resp.Data.WebPages.Value))
	}
}

func TestBochaSearch_Formatted(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"
	config.BaseURL = addr + "/v1/web-search"

	client := NewBochaSearchClient(config)
	formatted, err := client.SearchFormatted("Yaklang")
	if err != nil {
		t.Fatalf("formatted search failed: %v", err)
	}

	if !strings.Contains(formatted, "Yaklang") {
		t.Error("expected formatted output to contain query term")
	}

	if !strings.Contains(formatted, "example.com") {
		t.Error("expected formatted output to contain result URLs")
	}
}

func TestBochaSearch_ConnectionRefused(t *testing.T) {
	config := NewDefaultBochaConfig()
	config.APIKey = "test-key"
	config.BaseURL = "http://127.0.0.1:" + fmt.Sprint(utils.GetRandomAvailableTCPPort()) + "/v1/web-search"
	config.Timeout = 2

	client := NewBochaSearchClient(config)
	_, err := client.Search("test")
	if err == nil {
		t.Fatal("expected error when server is not reachable")
	}
}

func TestOmniBochaSearchClient_Integration(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	client := NewOmniBochaSearchClient()

	if client.GetType() != ostype.SearcherTypeBocha {
		t.Errorf("expected type 'bocha', got: %s", client.GetType())
	}

	config := &ostype.SearchConfig{
		ApiKey:   "test-key",
		BaseURL:  addr + "/v1/web-search",
		PageSize: 5,
		Page:     1,
	}

	results, err := client.Search("Yaklang", config)
	if err != nil {
		t.Fatalf("omni search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	if results[0].Source != "bocha" {
		t.Errorf("expected source 'bocha', got: %s", results[0].Source)
	}
}

func TestOmniBochaSearchClient_Pagination(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	client := NewOmniBochaSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "test-key",
		BaseURL:  addr + "/v1/web-search",
		PageSize: 5,
		Page:     2,
	}

	results, err := client.Search("Yaklang", config)
	if err != nil {
		t.Fatalf("pagination search failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results for page 2 with pageSize 5, got %d", len(results))
	}
}

func TestOmniBochaSearchClient_ExtraParams(t *testing.T) {
	addr, cleanup := mockBochaServer(t)
	defer cleanup()

	client := NewOmniBochaSearchClient()

	config := &ostype.SearchConfig{
		ApiKey:   "test-key",
		BaseURL:  addr + "/v1/web-search",
		PageSize: 3,
		Page:     1,
		Extra: map[string]interface{}{
			"freshness": "oneWeek",
			"summary":   true,
		},
	}

	results, err := client.Search("Yaklang", config)
	if err != nil {
		t.Fatalf("search with extra params failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// When summary is requested, Content should be the summary
	if !strings.Contains(results[0].Content, "summary") {
		t.Logf("Content: %s", results[0].Content)
	}
}

// TestBochaSearch_RealAPI tests against the real Bocha API.
// Requires API key in ~/yakit-projects/bocha.txt
func TestBochaSearch_RealAPI(t *testing.T) {
	keyPath := os.ExpandEnv("$HOME/yakit-projects/bocha.txt")
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Skipf("skipping real API test: no API key found at %s", keyPath)
	}
	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		t.Skip("skipping real API test: API key is empty")
	}

	t.Run("BasicSearch", func(t *testing.T) {
		config := NewDefaultBochaConfig()
		config.APIKey = apiKey
		config.MaxResults = 5

		client := NewBochaSearchClient(config)
		resp, err := client.Search("Yaklang")
		if err != nil {
			t.Fatalf("real API search failed: %v", err)
		}

		if resp.Data == nil || resp.Data.WebPages == nil {
			t.Fatal("expected non-nil web pages")
		}

		t.Logf("got %d results for 'Yaklang'", len(resp.Data.WebPages.Value))
		for i, r := range resp.Data.WebPages.Value {
			t.Logf("  %d. %s - %s", i+1, r.Name, r.URL)
		}
	})

	t.Run("OmniSearch", func(t *testing.T) {
		client := NewOmniBochaSearchClient()
		results, err := client.Search("Yaklang", &ostype.SearchConfig{
			ApiKey:   apiKey,
			PageSize: 5,
			Page:     1,
			Extra: map[string]interface{}{
				"freshness": "oneYear",
				"summary":   true,
			},
		})
		if err != nil {
			t.Fatalf("real omni search failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("expected at least one result")
		}

		t.Logf("got %d OmniSearch results", len(results))
		for i, r := range results {
			t.Logf("  %d. [%s] %s - %s", i+1, r.Source, r.Title, r.URL)
		}
	})
}
