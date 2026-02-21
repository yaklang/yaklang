package aibalance

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
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
	SearcherType string `json:"searcher_type"` // "brave", "tavily", "chatglm", "bocha", "unifuncs" or "" (auto-select), default ""
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
// Authentication flow:
//   - Has API Key: validate key → traffic limit → TOTP → search → billing
//   - Has Trace-ID only: check AllowFreeUserWebSearch → rate limit → TOTP → search (no billing)
//   - Neither: return 502
func (c *ServerConfig) serveWebSearch(conn net.Conn, rawPacket []byte) {
	// Increment cumulative web search counter (both in-memory and persistent DB)
	atomic.AddInt64(&c.totalWebSearchCount, 1)
	go func() {
		if err := IncrementWebSearchConfigTotalRequests(); err != nil {
			log.Errorf("failed to increment persistent web search counter: %v", err)
		}
	}()

	c.logInfo("starting to handle new web search request")

	// Extract authorization and Trace-ID headers
	auth := ""
	traceID := ""
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
		if k == "Trace-ID" || k == "Trace-Id" || k == "trace-id" {
			traceID = v
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
	if reqBody.MaxResults <= 0 {
		reqBody.MaxResults = 10
	}
	if reqBody.Page <= 0 {
		reqBody.Page = 1
	}
	if reqBody.PageSize <= 0 {
		reqBody.PageSize = 10
	}

	// Validate searcher type (empty string means "auto-select")
	validTypes := map[string]bool{"brave": true, "tavily": true, "chatglm": true, "bocha": true, "unifuncs": true, "": true}
	if !validTypes[reqBody.SearcherType] {
		c.logError("invalid searcher type: %s", reqBody.SearcherType)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "searcher_type must be 'brave', 'tavily', 'chatglm', 'bocha', 'unifuncs' or empty (auto-select)",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	c.logInfo("web search request: query=%s, type=%s, max_results=%d, page=%d, traceID=%s",
		utils.ShrinkString(reqBody.Query, 50), reqBody.SearcherType, reqBody.MaxResults, reqBody.Page, utils.ShrinkString(traceID, 12))

	// Determine authentication mode: API Key vs Trace-ID vs neither
	// Parse Authorization header defensively: only treat as API key when "Bearer" prefix matches
	var apiKeyValue string
	hasApiKey := false
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				apiKeyValue = token
				hasApiKey = true
			}
		}
	}
	hasTraceID := traceID != ""

	// Enforce a reasonable maximum length on Trace-ID to protect in-memory rate limiter
	const maxTraceIDLen = 128
	if hasTraceID && len(traceID) > maxTraceIDLen {
		c.logError("trace id too long: length=%d", len(traceID))
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "trace_id too long",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Branch: neither API Key nor Trace-ID
	if !hasApiKey && !hasTraceID {
		c.logError("web search request has neither API key nor Trace-ID")
		c.writeJSONResponse(conn, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]string{
				"message": "must have trace id or apikey",
				"type":    "authentication_error",
			},
		})
		return
	}

	// isFreeUser indicates this is a free user (Trace-ID only, no API key)
	isFreeUser := !hasApiKey && hasTraceID

	var apiKey *Key

	if hasApiKey {
		// Branch: has API Key — validate key and check traffic limit
		key, ok := c.Keys.Get(apiKeyValue)
		if !ok {
			c.logError("no matching key configuration found for web search: %s", utils.ShrinkString(apiKeyValue, 8))
			c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
				"error": map[string]string{
					"message": "invalid api key",
					"type":    "authentication_error",
				},
			})
			return
		}
		apiKey = key

		// Check traffic limit for API key users
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
	} else {
		// Branch: free user (Trace-ID only, no API key)
		// Check if free user web search is allowed
		wsConfig, err := GetWebSearchConfig()
		if err != nil {
			c.logError("failed to get web search config: %v", err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"error": map[string]string{
					"message": "internal server error",
					"type":    "server_error",
				},
			})
			return
		}

		if !wsConfig.AllowFreeUserWebSearch {
			c.logError("free user web search is disabled, traceID=%s", utils.ShrinkString(traceID, 12))
			c.writeJSONResponse(conn, http.StatusForbidden, map[string]interface{}{
				"error": map[string]string{
					"message": "free user web search is currently disabled",
					"type":    "forbidden",
				},
			})
			return
		}

		// Rate limiting for free users
		allowed, retryAfter := c.webSearchRateLimiter.CheckRateLimit(traceID)
		if !allowed {
			c.logError("rate limit exceeded for traceID=%s, retry after %d seconds", utils.ShrinkString(traceID, 12), retryAfter)
			c.writeJSONResponse(conn, http.StatusTooManyRequests, map[string]interface{}{
				"error": map[string]string{
					"message": fmt.Sprintf("rate limit exceeded, please retry after %d seconds", retryAfter),
					"type":    "rate_limit_exceeded",
				},
			})
			return
		}
	}

	// TOTP verification for web-search (same mechanism as memfit models)
	totpHeader := lowhttp.GetHTTPPacketHeader(rawPacket, "X-Memfit-OTP-Auth")
	if totpHeader == "" {
		c.logError("web search requires TOTP authentication, but X-Memfit-OTP-Auth header is missing")
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "TOTP authentication required for web search. Please provide X-Memfit-OTP-Auth header with base64 encoded TOTP code.",
				"type":    "memfit_totp_auth_required",
			},
		})
		return
	}

	verified, verifyErr := VerifyMemfitTOTP(totpHeader)
	if verifyErr != nil || !verified {
		c.logError("web search TOTP authentication failed: %v", verifyErr)
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "TOTP authentication failed for web search. Please refresh your TOTP secret and try again.",
				"type":    "memfit_totp_auth_failed",
			},
		})
		return
	}
	c.logInfo("TOTP authentication successful for web search request")

	// Resolve search keys: find active keys for the requested type,
	// or auto-select any available type if searcher_type is empty or has no keys
	searchKeys, resolvedType, resolveErr := c.resolveWebSearchKeys(reqBody.SearcherType)
	if resolveErr != nil {
		c.logError("failed to resolve web search keys: %v", resolveErr)
		c.writeJSONResponse(conn, http.StatusServiceUnavailable, map[string]interface{}{
			"error": map[string]string{
				"message": resolveErr.Error(),
				"type":    "service_unavailable",
			},
		})
		return
	}
	// Update the request with the resolved searcher type
	reqBody.SearcherType = resolvedType
	c.logInfo("resolved searcher type: %s (keys: %d)", resolvedType, len(searchKeys))

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

	// Record success for free users (triggers 3-second cooldown)
	if isFreeUser {
		c.webSearchRateLimiter.RecordSuccess(traceID)
		c.logInfo("web search succeeded for free user traceID=%s (no billing)", utils.ShrinkString(traceID, 12))
		// Free user: send response without billing
		c.sendWebSearchResponseNoBilling(conn, results, reqBody.SearcherType)
	} else {
		// Paid user: send response with billing
		c.sendWebSearchResponse(conn, results, reqBody.SearcherType, apiKey.Key, int64(len(body)))
	}
}

// resolveWebSearchKeys finds active search API keys for the given searcher type.
// If searcherType is empty or has no available keys, it auto-selects any type that has active keys.
// Returns: active keys, resolved searcher type, error
func (c *ServerConfig) resolveWebSearchKeys(searcherType string) ([]*schema.WebSearchApiKey, string, error) {
	// If a specific type is requested, try to find keys for that type first
	if searcherType != "" {
		keys, err := GetActiveWebSearchApiKeysByType(searcherType)
		if err == nil && len(keys) > 0 {
			return keys, searcherType, nil
		}
		// Also try active-but-unhealthy keys for the specified type
		allKeys, err := GetWebSearchApiKeysByType(searcherType)
		if err == nil {
			activeKeys := filterActiveKeys(allKeys)
			if len(activeKeys) > 0 {
				return activeKeys, searcherType, nil
			}
		}
		c.logInfo("no keys available for requested type '%s', falling back to auto-select", searcherType)
	}

	// Auto-select: find any type that has active keys
	allActiveKeys, err := GetAllActiveWebSearchApiKeys()
	if err != nil {
		return nil, "", utils.Errorf("failed to query active web search keys: %v", err)
	}
	if len(allActiveKeys) == 0 {
		if searcherType != "" {
			return nil, "", utils.Errorf("no search api keys available for type '%s' and no other types have keys either", searcherType)
		}
		return nil, "", utils.Errorf("no active web search api keys configured on this server")
	}

	// Group active keys by type, pick the type with the most keys
	typeKeys := map[string][]*schema.WebSearchApiKey{}
	for _, k := range allActiveKeys {
		typeKeys[k.SearcherType] = append(typeKeys[k.SearcherType], k)
	}

	// Select the type with the most active keys
	bestType := ""
	bestCount := 0
	for t, keys := range typeKeys {
		if len(keys) > bestCount {
			bestType = t
			bestCount = len(keys)
		}
	}

	c.logInfo("auto-selected searcher type '%s' with %d active keys", bestType, bestCount)
	return typeKeys[bestType], bestType, nil
}

// filterActiveKeys returns only active keys from the given list
func filterActiveKeys(keys []*schema.WebSearchApiKey) []*schema.WebSearchApiKey {
	active := make([]*schema.WebSearchApiKey, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			active = append(active, k)
		}
	}
	return active
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
	case "bocha":
		client := searchers.NewOmniBochaSearchClient()
		return client.Search(req.Query, config)
	case "unifuncs":
		client := searchers.NewOmniUnifuncsSearchClient()
		return client.Search(req.Query, config)
	default:
		return nil, utils.Errorf("unsupported searcher type: %s", req.SearcherType)
	}
}

// sendWebSearchResponse sends the web search response and updates API key stats with traffic billing
func (c *ServerConfig) sendWebSearchResponse(conn net.Conn, results []*ostype.OmniSearchResult, searcherType string, apiKey string, inputBytes int64) {
	resp := &WebSearchResponse{
		Results:      results,
		Total:        len(results),
		SearcherType: searcherType,
	}

	// Update the caller's API key stats asynchronously
	respBytes, _ := json.Marshal(resp)
	outputBytes := int64(len(respBytes))
	go func() {
		if err := UpdateAiApiKeyStats(apiKey, inputBytes, outputBytes, true); err != nil {
			log.Errorf("failed to update api key stats for web search: %v", err)
		}
		// Increment web search specific counter
		if err := IncrementAiApiKeyWebSearchCount(apiKey); err != nil {
			log.Errorf("failed to increment web search count for api key: %v", err)
		}
		// Update traffic usage with model multiplier for "web-search"
		multiplier := GetModelTrafficMultiplier("web-search")
		totalTraffic := inputBytes + outputBytes
		adjustedTraffic := int64(float64(totalTraffic) * multiplier)
		if err := UpdateAiApiKeyTrafficUsed(apiKey, adjustedTraffic); err != nil {
			log.Errorf("failed to update traffic usage for web search: %v", err)
		} else {
			log.Infof("web-search traffic usage updated: key=%s, input=%d, output=%d, multiplier=%.2f, adjusted=%d bytes",
				utils.ShrinkString(apiKey, 8), inputBytes, outputBytes, multiplier, adjustedTraffic)
		}
	}()

	c.writeJSONResponse(conn, http.StatusOK, resp)
}

// sendWebSearchResponseNoBilling sends the web search response without any billing (for free users)
func (c *ServerConfig) sendWebSearchResponseNoBilling(conn net.Conn, results []*ostype.OmniSearchResult, searcherType string) {
	resp := &WebSearchResponse{
		Results:      results,
		Total:        len(results),
		SearcherType: searcherType,
	}
	c.writeJSONResponse(conn, http.StatusOK, resp)
}
