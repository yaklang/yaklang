package aibalance

import (
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/omnisearch/searchers"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// WebSearchRequest represents the request body for /v1/web-search
type WebSearchRequest struct {
	Query        string `json:"query"`
	SearcherType string `json:"searcher_type"` // "brave", "tavily" or "chatglm", default "brave"
	MaxResults   int    `json:"max_results"`   // default 10
	Page         int    `json:"page"`          // default 1
	PageSize     int    `json:"page_size"`     // default 10
}

// WebSearchResponse represents the response body for /v1/web-search
type WebSearchResponse struct {
	Results      []*ostype.OmniSearchResult `json:"results"`
	Total        int                        `json:"total"`
	SearcherType string                     `json:"searcher_type"`
}

// serveWebSearch handles web search relay requests at /v1/web-search
func (c *ServerConfig) serveWebSearch(conn net.Conn, rawPacket []byte) {
	c.logInfo("starting to handle new web search request")

	// Extract authorization header
	auth := ""
	_, body := lowhttp.SplitHTTPPacket(rawPacket, func(method string, requestUri string, proto string) error {
		c.logInfo("web search request method: %s, URI: %s, Protocol: %s", method, requestUri, proto)
		return nil
	}, func(proto string, code int, codeMsg string) error {
		return nil
	}, func(line string) string {
		k, v := lowhttp.SplitHTTPHeader(line)
		if k == "Authorization" || k == "authorization" {
			auth = v
		}
		return line
	})

	if string(body) == "" {
		c.logError("web search request body is empty")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "request body is empty",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Parse request body
	var reqBody WebSearchRequest
	if err := json.Unmarshal(body, &reqBody); err != nil {
		c.logError("failed to parse web search request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "invalid request body",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if reqBody.Query == "" {
		c.logError("web search query is empty")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "query is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Set defaults
	if reqBody.SearcherType == "" {
		reqBody.SearcherType = "brave"
	}
	if reqBody.MaxResults <= 0 {
		reqBody.MaxResults = 10
	}
	if reqBody.Page <= 0 {
		reqBody.Page = 1
	}
	if reqBody.PageSize <= 0 {
		reqBody.PageSize = 10
	}

	// Validate searcher type
	validTypes := map[string]bool{"brave": true, "tavily": true, "chatglm": true}
	if !validTypes[reqBody.SearcherType] {
		c.logError("invalid searcher type: %s", reqBody.SearcherType)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "searcher_type must be 'brave', 'tavily' or 'chatglm'",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	c.logInfo("web search request: query=%s, type=%s, max_results=%d, page=%d",
		utils.ShrinkString(reqBody.Query, 50), reqBody.SearcherType, reqBody.MaxResults, reqBody.Page)

	// Authenticate: extract Bearer token and validate against AiApiKeys
	value := strings.TrimPrefix(auth, "Bearer ")
	if value == "" {
		c.logError("no valid authentication info provided for web search")
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "authentication required",
				"type":    "authentication_error",
			},
		})
		return
	}

	key, ok := c.Keys.Get(value)
	if !ok {
		c.logError("no matching key configuration found for web search: %s", utils.ShrinkString(value, 8))
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "invalid api key",
				"type":    "authentication_error",
			},
		})
		return
	}

	// Check traffic limit
	trafficAllowed, err := CheckAiApiKeyTrafficLimit(key.Key)
	if err != nil {
		c.logError("failed to check traffic limit for key %s: %v", utils.ShrinkString(key.Key, 8), err)
	} else if !trafficAllowed {
		c.logError("API key %s has exceeded traffic limit", utils.ShrinkString(key.Key, 8))
		c.writeJSONResponse(conn, http.StatusTooManyRequests, map[string]interface{}{
			"error": map[string]string{
				"message": "API key has exceeded traffic limit",
				"type":    "traffic_limit_exceeded",
			},
		})
		return
	}

	// Check if "web-search" is in the allowed models for this key
	if !c.KeyAllowedModels.IsModelAllowed(key.Key, "web-search") {
		c.logError("key %s is not allowed to access web-search", utils.ShrinkString(key.Key, 8))
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]interface{}{
			"error": map[string]string{
				"message": "api key does not have permission for web-search",
				"type":    "permission_error",
			},
		})
		return
	}

	// Get active web search API keys for the requested searcher type
	searchKeys, err := GetActiveWebSearchApiKeysByType(reqBody.SearcherType)
	if err != nil || len(searchKeys) == 0 {
		// Fallback: try all keys of this type (including unhealthy but active ones)
		allKeys, fallbackErr := GetWebSearchApiKeysByType(reqBody.SearcherType)
		if fallbackErr != nil || len(allKeys) == 0 {
			c.logError("no web search api keys configured for type: %s", reqBody.SearcherType)
			c.writeJSONResponse(conn, http.StatusServiceUnavailable, map[string]interface{}{
				"error": map[string]string{
					"message": "no search api keys available for type: " + reqBody.SearcherType,
					"type":    "service_unavailable",
				},
			})
			return
		}
		// Filter only active ones
		activeKeys := make([]*schema.WebSearchApiKey, 0, len(allKeys))
		for _, sk := range allKeys {
			if sk.Active {
				activeKeys = append(activeKeys, sk)
			}
		}
		if len(activeKeys) == 0 {
			c.logError("no active web search api keys for type: %s", reqBody.SearcherType)
			c.writeJSONResponse(conn, http.StatusServiceUnavailable, map[string]interface{}{
				"error": map[string]string{
					"message": "no active search api keys available for type: " + reqBody.SearcherType,
					"type":    "service_unavailable",
				},
			})
			return
		}
		searchKeys = activeKeys
	}

	// Try search with random selection + failover
	results, searchErr := c.tryWebSearchWithKeys(searchKeys, &reqBody)
	if searchErr != nil {
		c.logError("all web search api keys failed for type %s: %v", reqBody.SearcherType, searchErr)
		c.writeJSONResponse(conn, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]string{
				"message": "all search api keys failed: " + searchErr.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}

	c.sendWebSearchResponse(conn, results, reqBody.SearcherType, key.Key)
}

// tryWebSearchWithKeys attempts to perform a web search using the provided keys
// Keys are randomly shuffled, and on failure, the next key is tried
func (c *ServerConfig) tryWebSearchWithKeys(keys []*schema.WebSearchApiKey, req *WebSearchRequest) ([]*ostype.OmniSearchResult, error) {
	// Copy and randomly shuffle the keys
	shuffled := make([]*schema.WebSearchApiKey, len(keys))
	copy(shuffled, keys)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	var lastErr error
	for _, sk := range shuffled {
		c.logInfo("trying web search with key ID=%d, type=%s", sk.ID, sk.SearcherType)

		startTime := time.Now()
		results, err := c.doWebSearch(sk, req)
		latencyMs := time.Since(startTime).Milliseconds()

		if err != nil {
			c.logError("web search failed with key ID=%d: %v", sk.ID, err)
			// Update stats: failure
			if updateErr := UpdateWebSearchApiKeyStats(sk.ID, false, latencyMs); updateErr != nil {
				c.logError("failed to update web search key stats: %v", updateErr)
			}
			lastErr = err
			continue
		}

		// Success: update stats
		if updateErr := UpdateWebSearchApiKeyStats(sk.ID, true, latencyMs); updateErr != nil {
			c.logError("failed to update web search key stats: %v", updateErr)
		}

		c.logInfo("web search succeeded with key ID=%d, returned %d results, latency=%dms",
			sk.ID, len(results), latencyMs)
		return results, nil
	}

	return nil, lastErr
}

// doWebSearch performs the actual search using the appropriate searcher client
func (c *ServerConfig) doWebSearch(sk *schema.WebSearchApiKey, req *WebSearchRequest) ([]*ostype.OmniSearchResult, error) {
	// Determine proxy: key-level proxy takes priority, then global proxy
	proxy := sk.Proxy
	if proxy == "" {
		proxy = c.WebSearchProxy
	}

	config := &ostype.SearchConfig{
		ApiKey:   sk.APIKey,
		Page:     req.Page,
		PageSize: req.PageSize,
		Proxy:    proxy,
		BaseURL:  sk.BaseURL,
	}

	switch req.SearcherType {
	case "brave":
		client := searchers.NewOmniBraveSearchClient()
		return client.Search(req.Query, config)
	case "tavily":
		client := searchers.NewOmniTavilySearchClient()
		return client.Search(req.Query, config)
	case "chatglm":
		client := searchers.NewOmniChatGLMSearchClient()
		return client.Search(req.Query, config)
	default:
		return nil, utils.Errorf("unsupported searcher type: %s", req.SearcherType)
	}
}

// sendWebSearchResponse sends the web search response and updates API key stats
func (c *ServerConfig) sendWebSearchResponse(conn net.Conn, results []*ostype.OmniSearchResult, searcherType string, apiKey string) {
	resp := &WebSearchResponse{
		Results:      results,
		Total:        len(results),
		SearcherType: searcherType,
	}

	// Update the caller's API key stats asynchronously
	respBytes, _ := json.Marshal(resp)
	go func() {
		if err := UpdateAiApiKeyStats(apiKey, int64(len(searcherType)), int64(len(respBytes)), true); err != nil {
			log.Errorf("failed to update api key stats for web search: %v", err)
		}
		// Increment web search specific counter
		if err := IncrementAiApiKeyWebSearchCount(apiKey); err != nil {
			log.Errorf("failed to increment web search count for api key: %v", err)
		}
	}()

	c.writeJSONResponse(conn, http.StatusOK, resp)
}
