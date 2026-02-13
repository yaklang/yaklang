package aibalance

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// amapBaseResponse is the minimal response to check if the amap API returned a key-related error
type amapBaseResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	Infocode string `json:"infocode"`
}

// isAmapKeyError returns true if the infocode indicates a key-related error
// that should trigger failover to the next key.
// See: https://lbs.amap.com/api/webservice/guide/tools/info
func isAmapKeyError(infocode string) bool {
	switch infocode {
	case "10001", // INVALID_USER_KEY
		"10003", // DAILY_QUERY_OVER_LIMIT
		"10004", // ACCESS_TOO_FREQUENT
		"10005", // IP_QUERY_OVER_LIMIT
		"10009", // USERKEY_BINDbindip_IS_BINDIP
		"10010", // IP_BINDINTERFACE_BINDIP
		"10016", // INVALID_USERKEY_NOT_BINDbindip
		"10044": // QUOTA_PLAN_RUN_OUT
		return true
	}
	return false
}

// serveAmap handles transparent proxy requests at /amap/*
// Authentication flow:
//   - TOTP verification required (X-Memfit-OTP-Auth)
//   - Free users (Trace-ID only): rate limited via sleep/wait
//   - API key users: no rate limiting
func (c *ServerConfig) serveAmap(conn net.Conn, rawPacket []byte) {
	// Increment cumulative amap counter
	atomic.AddInt64(&c.totalAmapCount, 1)
	go func() {
		if err := IncrementAmapConfigTotalRequests(); err != nil {
			log.Errorf("failed to increment persistent amap counter: %v", err)
		}
	}()

	c.logInfo("starting to handle new amap proxy request")

	// Extract request info
	var requestPath string
	auth := ""
	traceID := ""
	_, _ = lowhttp.SplitHTTPPacket(rawPacket, func(method string, requestUri string, proto string) error {
		requestPath = requestUri
		c.logInfo("amap proxy request method: %s, URI: %s", method, requestUri)
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

	// Extract the path after /amap/
	amapPath := ""
	if idx := strings.Index(requestPath, "/amap/"); idx >= 0 {
		amapPath = requestPath[idx+len("/amap"):]
	} else if idx := strings.Index(requestPath, "/amap"); idx >= 0 {
		amapPath = requestPath[idx+len("/amap"):]
		if amapPath == "" {
			amapPath = "/"
		}
	}

	if amapPath == "" || amapPath == "/" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "amap API path is required, e.g. /amap/v3/weather/weatherInfo",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	c.logInfo("amap proxy: forwarding path=%s, traceID=%s", amapPath, utils.ShrinkString(traceID, 12))

	// Determine authentication mode
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

	// Enforce max Trace-ID length
	const maxTraceIDLen = 128
	if hasTraceID && len(traceID) > maxTraceIDLen {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{
				"message": "trace_id too long",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// TOTP verification (required for all amap proxy requests)
	totpHeader := lowhttp.GetHTTPPacketHeader(rawPacket, "X-Memfit-OTP-Auth")
	if totpHeader == "" {
		c.logError("amap proxy requires TOTP authentication, but X-Memfit-OTP-Auth header is missing")
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "TOTP authentication required for amap proxy. Please provide X-Memfit-OTP-Auth header with base64 encoded TOTP code.",
				"type":    "memfit_totp_auth_required",
			},
		})
		return
	}

	verified, verifyErr := VerifyMemfitTOTP(totpHeader)
	if verifyErr != nil || !verified {
		c.logError("amap proxy TOTP authentication failed: %v", verifyErr)
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "TOTP authentication failed for amap proxy.",
				"type":    "memfit_totp_auth_failed",
			},
		})
		return
	}
	c.logInfo("TOTP authentication successful for amap proxy request")

	isFreeUser := !hasApiKey

	// Check if free user access is allowed
	if isFreeUser {
		amapConfig, err := GetAmapConfig()
		if err != nil {
			c.logError("failed to get amap config: %v", err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"error": map[string]string{
					"message": "internal server error",
					"type":    "server_error",
				},
			})
			return
		}

		if !amapConfig.AllowFreeUserAmap {
			c.writeJSONResponse(conn, http.StatusForbidden, map[string]interface{}{
				"error": map[string]string{
					"message": "free user amap proxy is currently disabled",
					"type":    "forbidden",
				},
			})
			return
		}

		// Rate limiting: sleep/wait instead of returning 429
		if hasTraceID {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := c.amapRateLimiter.WaitForRateLimit(traceID, ctx); err != nil {
				c.logError("amap rate limit wait timed out for traceID=%s: %v", utils.ShrinkString(traceID, 12), err)
				c.writeJSONResponse(conn, http.StatusGatewayTimeout, map[string]interface{}{
					"error": map[string]string{
						"message": "rate limit wait timed out, please try again later",
						"type":    "rate_limit_timeout",
					},
				})
				return
			}
		}
	} else {
		// Validate API key for paid users
		_, ok := c.Keys.Get(apiKeyValue)
		if !ok {
			c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
				"error": map[string]string{
					"message": "invalid api key",
					"type":    "authentication_error",
				},
			})
			return
		}
	}

	// Get active amap API keys
	amapKeys, err := GetActiveAmapApiKeys()
	if err != nil || len(amapKeys) == 0 {
		// Fallback to all active keys (including unhealthy)
		amapKeys, err = GetAllActiveAmapApiKeys()
		if err != nil || len(amapKeys) == 0 {
			c.logError("no active amap api keys available")
			c.writeJSONResponse(conn, http.StatusServiceUnavailable, map[string]interface{}{
				"error": map[string]string{
					"message": "no amap api keys configured on this server",
					"type":    "service_unavailable",
				},
			})
			return
		}
	}

	c.logInfo("amap proxy: found %d active keys, attempting forward", len(amapKeys))

	// Try each key with random selection + failover
	rspRaw, usedKey, proxyErr := c.tryAmapForwardWithKeys(amapKeys, amapPath)
	if proxyErr != nil {
		c.logError("all amap api keys failed for path %s: %v", amapPath, proxyErr)
		c.writeJSONResponse(conn, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]string{
				"message": "all amap api keys failed: " + proxyErr.Error(),
				"type":    "upstream_error",
			},
		})
		return
	}

	// Record success for rate limiting
	if isFreeUser && hasTraceID {
		c.amapRateLimiter.RecordSuccess(traceID)
	}

	_ = usedKey
	c.logInfo("amap proxy succeeded, path=%s", amapPath)

	// Transparently forward the response to the client
	conn.Write(rspRaw)
}

// tryAmapForwardWithKeys attempts to forward the request using the provided amap keys.
// Keys are randomly shuffled; on key-related failure, the next key is tried.
func (c *ServerConfig) tryAmapForwardWithKeys(keys []*schema.AmapApiKey, amapPath string) ([]byte, *schema.AmapApiKey, error) {
	// Shuffle keys
	shuffled := make([]*schema.AmapApiKey, len(keys))
	copy(shuffled, keys)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	var lastErr error
	for _, sk := range shuffled {
		c.logInfo("trying amap forward with key ID=%d", sk.ID)

		startTime := time.Now()
		rspRaw, err := c.doAmapForward(sk, amapPath)
		latencyMs := time.Since(startTime).Milliseconds()

		if err != nil {
			c.logError("amap forward failed with key ID=%d: %v", sk.ID, err)
			go func(keyID uint) {
				if updateErr := UpdateAmapApiKeyStats(keyID, false, latencyMs); updateErr != nil {
					log.Errorf("failed to update amap key stats: %v", updateErr)
				}
			}(sk.ID)
			lastErr = err
			continue
		}

		// Check if the response indicates a key-related error
		body := lowhttp.GetHTTPPacketBody(rspRaw)
		if len(body) > 0 {
			var baseResp amapBaseResponse
			if json.Unmarshal(body, &baseResp) == nil && baseResp.Status == "0" && isAmapKeyError(baseResp.Infocode) {
				c.logError("amap key error with key ID=%d: infocode=%s, info=%s", sk.ID, baseResp.Infocode, baseResp.Info)
				go func(keyID uint) {
					if updateErr := UpdateAmapApiKeyStats(keyID, false, latencyMs); updateErr != nil {
						log.Errorf("failed to update amap key stats: %v", updateErr)
					}
				}(sk.ID)
				lastErr = fmt.Errorf("key error: infocode=%s, info=%s", baseResp.Infocode, baseResp.Info)
				continue
			}
		}

		// Success
		go func(keyID uint) {
			if updateErr := UpdateAmapApiKeyStats(keyID, true, latencyMs); updateErr != nil {
				log.Errorf("failed to update amap key stats: %v", updateErr)
			}
		}(sk.ID)

		c.logInfo("amap forward succeeded with key ID=%d, latency=%dms", sk.ID, latencyMs)
		return rspRaw, sk, nil
	}

	return nil, nil, lastErr
}

// doAmapForward performs the actual HTTP forward to restapi.amap.com
func (c *ServerConfig) doAmapForward(sk *schema.AmapApiKey, amapPath string) ([]byte, error) {
	// Parse the path and query string
	pathAndQuery := amapPath
	if !strings.HasPrefix(pathAndQuery, "/") {
		pathAndQuery = "/" + pathAndQuery
	}

	// Replace or add the key parameter in the query string
	if strings.Contains(pathAndQuery, "key=") {
		// Replace existing key parameter
		parts := strings.SplitN(pathAndQuery, "?", 2)
		if len(parts) == 2 {
			params := strings.Split(parts[1], "&")
			var newParams []string
			keyFound := false
			for _, p := range params {
				if strings.HasPrefix(p, "key=") {
					newParams = append(newParams, "key="+sk.APIKey)
					keyFound = true
				} else {
					newParams = append(newParams, p)
				}
			}
			if !keyFound {
				newParams = append(newParams, "key="+sk.APIKey)
			}
			pathAndQuery = parts[0] + "?" + strings.Join(newParams, "&")
		}
	} else {
		// Add key parameter
		if strings.Contains(pathAndQuery, "?") {
			pathAndQuery += "&key=" + sk.APIKey
		} else {
			pathAndQuery += "?key=" + sk.APIKey
		}
	}

	// Build raw HTTP request
	rawReq := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: restapi.amap.com\r\nUser-Agent: yaklang-aibalance\r\nAccept: application/json\r\n\r\n", pathAndQuery)

	rspIns, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithHttps(true),
		lowhttp.WithRequest([]byte(rawReq)),
		lowhttp.WithHost("restapi.amap.com"),
		lowhttp.WithTimeout(15*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to forward to amap: %v", err)
	}

	return rspIns.RawPacket, nil
}
