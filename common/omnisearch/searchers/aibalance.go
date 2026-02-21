package searchers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// AiBalanceSearchConfig contains the configuration for the AiBalance web search relay client
type AiBalanceSearchConfig struct {
	// BaseURL is the AiBalance server URL (default: http://127.0.0.1:80)
	BaseURL string
	// APIKey is the Bearer token for AiBalance authentication
	APIKey string
	// BackendSearcherType specifies which backend searcher to use via AiBalance ("brave", "tavily", "chatglm", "bocha" or "unifuncs")
	BackendSearcherType string
	// Proxy is an optional proxy for connecting to AiBalance
	Proxy string
	// Timeout is the request timeout in seconds
	Timeout float64
}

// NewDefaultAiBalanceConfig returns a default AiBalance configuration
func NewDefaultAiBalanceConfig() *AiBalanceSearchConfig {
	return &AiBalanceSearchConfig{
		BaseURL:             "https://aibalance.yaklang.com",
		BackendSearcherType: "", // empty means "auto" - let the server choose based on available keys
		Timeout:             30,
	}
}

// AiBalanceSearchClient implements web search via an AiBalance relay server
type AiBalanceSearchClient struct {
	Config *AiBalanceSearchConfig
}

// NewAiBalanceSearchClient creates a new AiBalance search client with the given config
func NewAiBalanceSearchClient(config *AiBalanceSearchConfig) *AiBalanceSearchClient {
	return &AiBalanceSearchClient{Config: config}
}

// NewDefaultAiBalanceSearchClient creates a new AiBalance search client with default configuration
func NewDefaultAiBalanceSearchClient() *AiBalanceSearchClient {
	return NewAiBalanceSearchClient(NewDefaultAiBalanceConfig())
}

// AiBalanceSearchRequest represents the JSON request body for /v1/web-search
type AiBalanceSearchRequest struct {
	Query        string `json:"query"`
	SearcherType string `json:"searcher_type"`
	MaxResults   int    `json:"max_results"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}

// AiBalanceSearchResponse represents the JSON response body from /v1/web-search
type AiBalanceSearchResponse struct {
	Results      []*ostype.OmniSearchResult `json:"results"`
	Total        int                        `json:"total"`
	SearcherType string                     `json:"searcher_type"`
}

// AiBalanceErrorResponse represents an error response from the AiBalance server
type AiBalanceErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// fetchTOTPSecretFromServer fetches the TOTP UUID from the aibalance server
func (c *AiBalanceSearchClient) fetchTOTPSecretFromServer() string {
	baseURL := strings.TrimSuffix(c.Config.BaseURL, "/v1/web-search")
	baseURL = strings.TrimSuffix(baseURL, "/")
	totpURL := baseURL + "/v1/memfit-totp-uuid"

	opts := []lowhttp.LowhttpOpt{}
	if c.Config.Timeout > 0 {
		opts = append(opts, lowhttp.WithTimeoutFloat(c.Config.Timeout))
	}
	if c.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(c.Config.Proxy))
	}

	raw, err := Request("GET", totpURL, map[string]string{
		"Accept": "application/json",
	}, nil, nil, opts...)
	if err != nil {
		log.Errorf("failed to fetch TOTP UUID from aibalance server: %v", err)
		return ""
	}

	body := lowhttp.GetHTTPPacketBody(raw)
	var result struct {
		UUID   string `json:"uuid"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Errorf("failed to parse TOTP UUID response: %v", err)
		return ""
	}

	uuid := result.UUID
	if uuid == "" {
		log.Errorf("empty TOTP UUID in response from aibalance server")
		return ""
	}

	// Remove MEMFIT-AI prefix and suffix
	secret := strings.TrimPrefix(uuid, "MEMFIT-AI")
	secret = strings.TrimSuffix(secret, "MEMFIT-AI")
	return secret
}

// generateTOTPCode generates a TOTP code using the shared cached secret (aibalanceclient package)
// The cache is shared with the AI GatewayClient
func (c *AiBalanceSearchClient) generateTOTPCode() string {
	return aibalanceclient.GenerateTOTPCode(c.fetchTOTPSecretFromServer)
}

// refreshTOTPSecret clears the shared cache and re-fetches the TOTP secret from server
func (c *AiBalanceSearchClient) refreshTOTPSecret() {
	aibalanceclient.RefreshTOTPSecret(c.fetchTOTPSecretFromServer)
}

// isTOTPAuthError checks if the error is a TOTP authentication failure
func isTOTPAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "memfit_totp_auth_failed") ||
		strings.Contains(errStr, "memfit_totp_auth_required") ||
		strings.Contains(errStr, "Memfit TOTP authentication failed")
}

// Search performs a search through the AiBalance web search relay
// Supports TOTP authentication with automatic secret refresh and retry on auth failure
func (c *AiBalanceSearchClient) Search(query string, page, pageSize int) (*AiBalanceSearchResponse, error) {
	// Note: APIKey is optional now; free users can use Trace-ID only
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// Build request body
	reqBody := &AiBalanceSearchRequest{
		Query:        query,
		SearcherType: c.Config.BackendSearcherType,
		MaxResults:   pageSize,
		Page:         page,
		PageSize:     pageSize,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Build the full URL for the web search endpoint
	baseURL := strings.TrimSuffix(c.Config.BaseURL, "/v1/web-search")
	baseURL = strings.TrimSuffix(baseURL, "/")
	searchURL := baseURL + "/v1/web-search"

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{}
	if c.Config.Timeout > 0 {
		opts = append(opts, lowhttp.WithTimeoutFloat(c.Config.Timeout))
	}
	if c.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(c.Config.Proxy))
	}

	// First attempt
	resp, err := c.doSearchRequest(searchURL, bodyBytes, opts)
	if err != nil && isTOTPAuthError(err) {
		// TOTP authentication failed, refresh secret and retry once
		log.Infof("TOTP authentication failed for aibalance web search, refreshing secret and retrying...")
		c.refreshTOTPSecret()
		resp, err = c.doSearchRequest(searchURL, bodyBytes, opts)
	}
	return resp, err
}

// doSearchRequest sends the actual HTTP request to aibalance with TOTP authentication
func (c *AiBalanceSearchClient) doSearchRequest(searchURL string, bodyBytes []byte, opts []lowhttp.LowhttpOpt) (*AiBalanceSearchResponse, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "Yaklang-AiBalance-OmniSearch/1.0",
		"Trace-ID":     aibalanceclient.GetTraceID(),
	}

	// Add Authorization header only if API key is configured
	if c.Config.APIKey != "" {
		headers["Authorization"] = "Bearer " + c.Config.APIKey
	}

	// Add TOTP header for aibalance authentication
	totpCode := c.generateTOTPCode()
	if totpCode != "" {
		encodedCode := base64.StdEncoding.EncodeToString([]byte(totpCode))
		headers["X-Memfit-OTP-Auth"] = encodedCode
	}

	// Send POST request
	raw, err := Request("POST", searchURL, headers, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to aibalance: %v", err)
	}

	// Get response body and status code
	body := lowhttp.GetHTTPPacketBody(raw)
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)

	if statusCode != 200 {
		// Try to parse error response
		var errResp AiBalanceErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("aibalance web search failed (status %d): %s [%s]",
				statusCode, errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("aibalance web search failed with status code %d: %s",
			statusCode, string(body))
	}

	// Parse the success response
	var resp AiBalanceSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse aibalance search response: %v", err)
	}

	return &resp, nil
}
