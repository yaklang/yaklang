package aibalanceclient

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// TOTP_SECRET_DB_KEY is the shared database key for storing the aibalance TOTP secret
	// Both the AI GatewayClient and Omnisearch AiBalanceSearchClient use this key
	TOTP_SECRET_DB_KEY = "AIBALANCE_CLIENT_TOTP_SECRET"
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
