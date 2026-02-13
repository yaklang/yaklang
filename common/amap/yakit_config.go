package amap

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type YakitAmapConfig struct {
	ApiKey string `app:"name:api_key,verbose:ApiKey,desc:APIKey,required:true,id:1"`
}

func LoadAmapKeywordFromYakit() (string, error) {
	cfg := &YakitAmapConfig{}
	err := consts.GetThirdPartyApplicationConfig("amap", cfg)
	if err != nil {
		return "", err
	}
	return cfg.ApiKey, nil
}

// isProductionAIBalance checks if the server base URL is the production aibalance server.
func isProductionAIBalance(serverBase string) bool {
	return strings.Contains(serverBase, "aibalance.yaklang.com")
}

// LoadAmapTOTPHeader generates the X-Memfit-OTP-Auth header value for aibalance proxy authentication.
// The TOTP secret is shared across all services (ai, web-search, amap) on the same aibalance server.
//
// Strategy:
//   - Production server (aibalance.yaklang.com): reuse aibalanceclient shared cache
//     (the secret is the same as AI gateway and web-search, no extra fetch needed if already cached)
//   - Custom server (e.g., local 127.0.0.1:8223): fetch fresh from target server every time,
//     because the shared cache may contain a different (production) server's secret.
func LoadAmapTOTPHeader(amapBaseURL string) (string, error) {
	// Derive the server base URL by stripping the /amap suffix
	// e.g. "http://127.0.0.1:8223/amap" -> "http://127.0.0.1:8223"
	// e.g. "https://aibalance.yaklang.com/amap" -> "https://aibalance.yaklang.com"
	serverBase := strings.TrimSuffix(amapBaseURL, "/")
	serverBase = strings.TrimSuffix(serverBase, "/amap")
	serverBase = strings.TrimSuffix(serverBase, "/")
	totpURL := serverBase + "/v1/memfit-totp-uuid"

	var totpCode string

	if isProductionAIBalance(serverBase) {
		// Production: reuse the shared TOTP cache (memory -> database -> fetch).
		// The secret is the same for ai/web-search/amap, so if any client has
		// already fetched it, we skip the network request entirely.
		log.Infof("amap TOTP: using shared cache for production server")
		totpCode = aibalanceclient.GenerateTOTPCode(func() string {
			return fetchTOTPSecretFromURL(totpURL)
		})
	} else {
		// Custom server (local testing, staging, etc.): always fetch fresh from
		// the target server. The shared cache may hold a production secret that
		// won't match this server.
		log.Infof("amap TOTP: fetching fresh secret from custom server %s", serverBase)
		secret := fetchTOTPSecretFromURL(totpURL)
		if secret != "" {
			totpCode = twofa.GetUTCCode(secret)
		}
	}

	if totpCode == "" {
		return "", nil
	}

	// Base64 encode the TOTP code (same format as AI gateway and web-search clients)
	return base64.StdEncoding.EncodeToString([]byte(totpCode)), nil
}

// RefreshAmapTOTPHeader clears any cached secret, re-fetches from the target server,
// and returns a fresh X-Memfit-OTP-Auth header value.
// Called by doRequest when TOTP authentication fails (same pattern as AI Gateway's
// refreshTOTPSecretAndSave and Web Search's refreshTOTPSecret).
func RefreshAmapTOTPHeader(serverBase string) string {
	totpURL := serverBase + "/v1/memfit-totp-uuid"

	if isProductionAIBalance(serverBase) {
		// Production: clear shared cache and re-fetch
		log.Infof("amap TOTP refresh: clearing shared cache and re-fetching from production")
		newCode := aibalanceclient.RefreshTOTPSecret(func() string {
			return fetchTOTPSecretFromURL(totpURL)
		})
		if newCode == "" {
			return ""
		}
		// RefreshTOTPSecret returns the new secret, not the TOTP code.
		// Generate a fresh code from it.
		code := twofa.GetUTCCode(newCode)
		if code == "" {
			return ""
		}
		return base64.StdEncoding.EncodeToString([]byte(code))
	}

	// Custom server: fetch fresh directly
	log.Infof("amap TOTP refresh: re-fetching from custom server %s", serverBase)
	secret := fetchTOTPSecretFromURL(totpURL)
	if secret == "" {
		return ""
	}
	code := twofa.GetUTCCode(secret)
	if code == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(code))
}

// fetchTOTPSecretFromURL fetches the TOTP UUID from the given URL.
// This follows the same protocol as aibalanceclient.FetchTOTPSecretFromAIBalance
// and AiBalanceSearchClient.fetchTOTPSecretFromServer, but targets an arbitrary server.
func fetchTOTPSecretFromURL(totpURL string) string {
	log.Infof("fetching TOTP secret from: %s", totpURL)

	isHTTPS := strings.HasPrefix(totpURL, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(totpURL, "https://"), "http://")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}

	rawReq := []byte("GET /v1/memfit-totp-uuid HTTP/1.1\r\nHost: " + host + "\r\nAccept: application/json\r\nUser-Agent: yaklang-amap-client\r\n\r\n")

	rspIns, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithHttps(isHTTPS),
		lowhttp.WithRequest(rawReq),
		lowhttp.WithHost(host),
		lowhttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Errorf("failed to fetch TOTP UUID from %s: %v", totpURL, err)
		return ""
	}

	body := lowhttp.GetHTTPPacketBody(rspIns.RawPacket)
	var result struct {
		UUID   string `json:"uuid"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Errorf("failed to parse TOTP UUID response from %s: %v", totpURL, err)
		return ""
	}

	if result.UUID == "" {
		log.Errorf("empty TOTP UUID in response from %s", totpURL)
		return ""
	}

	// Remove MEMFIT-AI prefix and suffix (same as aibalanceclient.FetchTOTPSecretFromAIBalance)
	secret := strings.TrimPrefix(result.UUID, "MEMFIT-AI")
	secret = strings.TrimSuffix(secret, "MEMFIT-AI")

	log.Infof("successfully fetched TOTP secret from %s", totpURL)
	return secret
}
