package aibalanceclient

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// TOTP_SECRET_DB_KEY is the shared database key for storing the aibalance TOTP secret
	// Both the AI GatewayClient and Omnisearch AiBalanceSearchClient use this key
	TOTP_SECRET_DB_KEY = "AIBALANCE_CLIENT_TOTP_SECRET"

	// TRACE_ID_DB_KEY identifies a Profile, not a physical machine or an
	// authenticated user. Copying a Profile also copies its identity.
	TRACE_ID_DB_KEY = "AIBALANCE_CLIENT_TRACE_ID"
)

var (
	secretCache     string
	secretCacheLock sync.RWMutex
)

// GetCachedSecret returns the TOTP secret from memory cache
func GetCachedSecret() string {
	secretCacheLock.RLock()
	defer secretCacheLock.RUnlock()
	return secretCache
}

// SetCachedSecret updates only the memory cache
func SetCachedSecret(secret string) {
	secretCacheLock.Lock()
	secretCache = secret
	secretCacheLock.Unlock()
}

// ClearCache clears the in-memory TOTP secret cache
func ClearCache() {
	secretCacheLock.Lock()
	secretCache = ""
	secretCacheLock.Unlock()
}

// GetSecretFromDB reads the TOTP secret from the shared database
func GetSecretFromDB() string {
	db := consts.GetGormProfileDatabase()
	if db != nil {
		return yakit.GetKey(db, TOTP_SECRET_DB_KEY)
	}
	return ""
}

// SaveSecretToDB saves the TOTP secret to the shared database
func SaveSecretToDB(secret string) {
	db := consts.GetGormProfileDatabase()
	if db != nil {
		if err := yakit.SetKey(db, TOTP_SECRET_DB_KEY, secret); err != nil {
			log.Errorf("failed to save TOTP secret to database: %v", err)
		}
	}
}

// GetOrFetchTOTPSecret returns the TOTP secret with priority:
// 1. Memory cache (fastest, shared across all clients in the same process)
// 2. Database (shared across process restarts, and between clients initialized at different times)
// 3. fetchFunc callback (fetches from the aibalance server as last resort)
//
// If fetchFunc is called and returns a non-empty secret, it is saved to both memory cache and database.
// This ensures that whichever client fetches first, all subsequent clients benefit from the cache.
func GetOrFetchTOTPSecret(fetchFunc func() string) string {
	// 1. Check memory cache
	if s := GetCachedSecret(); s != "" {
		return s
	}

	// 2. Check database (shared with other clients)
	if s := GetSecretFromDB(); s != "" {
		SetCachedSecret(s)
		return s
	}

	// 3. Fetch from server using client-specific fetchFunc
	if fetchFunc != nil {
		s := fetchFunc()
		if s != "" {
			SaveSecret(s)
			return s
		}
	}

	return ""
}

// RefreshTOTPSecret clears all caches, re-fetches the TOTP secret, and saves it.
// This is called when TOTP authentication fails and the secret needs to be refreshed.
func RefreshTOTPSecret(fetchFunc func() string) string {
	ClearCache()

	if fetchFunc == nil {
		return ""
	}

	s := fetchFunc()
	if s != "" {
		SaveSecret(s)
	}
	return s
}

// SaveSecret saves the TOTP secret to both memory cache and database
func SaveSecret(secret string) {
	SetCachedSecret(secret)
	SaveSecretToDB(secret)
}

// GenerateTOTPCode generates a TOTP code using the cached secret.
// If no secret is cached, it calls fetchFunc to obtain one first.
func GenerateTOTPCode(fetchFunc func() string) string {
	secret := GetOrFetchTOTPSecret(fetchFunc)
	if secret == "" {
		return ""
	}
	return twofa.GetUTCCode(secret)
}

// GetTraceID is the compatibility entry point for the persistent Profile UUID.
func GetTraceID() string {
	return GetOrCreateClientUserID()
}

// FetchTOTPSecretFromAIBalance fetches the TOTP secret from the standard aibalance server.
// This is a standalone function that does not depend on the AI GatewayClient,
// and can be used by any client that needs TOTP authentication with aibalance.
func FetchTOTPSecretFromAIBalance() string {
	totpURL := "https://aibalance.yaklang.com/v1/memfit-totp-uuid"

	rawReq := []byte("GET /v1/memfit-totp-uuid HTTP/1.1\r\nHost: aibalance.yaklang.com\r\nAccept: application/json\r\nUser-Agent: yaklang-amap-client\r\n\r\n")
	if id := GetOrCreateClientUserID(); id != "" {
		rawReq = lowhttp.ReplaceHTTPPacketHeader(rawReq, ClientUserIDHeader, id)
	}

	rspIns, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithHttps(true),
		lowhttp.WithRequest(rawReq),
		lowhttp.WithHost("aibalance.yaklang.com"),
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
		log.Errorf("failed to parse TOTP UUID response: %v", err)
		return ""
	}

	uuid := result.UUID
	if uuid == "" {
		log.Errorf("empty TOTP UUID in response from aibalance")
		return ""
	}

	// Remove MEMFIT-AI prefix and suffix
	secret := strings.TrimPrefix(uuid, "MEMFIT-AI")
	secret = strings.TrimSuffix(secret, "MEMFIT-AI")

	log.Debugf("successfully fetched TOTP secret from aibalance for amap proxy")
	return secret
}
