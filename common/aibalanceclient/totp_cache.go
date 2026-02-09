package aibalanceclient

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// TOTP_SECRET_DB_KEY is the shared database key for storing the aibalance TOTP secret
	// Both the AI GatewayClient and Omnisearch AiBalanceSearchClient use this key
	TOTP_SECRET_DB_KEY = "AIBALANCE_CLIENT_TOTP_SECRET"

	// TRACE_ID_DB_KEY is the database key for storing the persistent client Trace-ID
	// Once generated, this ID is bound to this client installation and never changes.
	// This prevents users from bypassing rate limiting by restarting the process.
	TRACE_ID_DB_KEY = "AIBALANCE_CLIENT_TRACE_ID"
)

var (
	secretCache     string
	secretCacheLock sync.RWMutex

	traceIDCache     string
	traceIDCacheLock sync.RWMutex
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

// ==================== Persistent Trace-ID ====================
// The Trace-ID is generated once per client installation and persisted to the local database.
// It binds the client to a stable identity for server-side rate limiting.
// Unlike TOTP secrets which can be refreshed, Trace-ID is immutable once created.

// GetTraceID returns the persistent Trace-ID for this client.
// Priority: memory cache -> database -> generate new UUID and persist.
// Once generated, the Trace-ID never changes across process restarts.
func GetTraceID() string {
	// 1. Check memory cache
	traceIDCacheLock.RLock()
	cached := traceIDCache
	traceIDCacheLock.RUnlock()
	if cached != "" {
		return cached
	}

	// 2. Check database
	db := consts.GetGormProfileDatabase()
	if db != nil {
		if stored := yakit.GetKey(db, TRACE_ID_DB_KEY); stored != "" {
			traceIDCacheLock.Lock()
			traceIDCache = stored
			traceIDCacheLock.Unlock()
			return stored
		}
	}

	// 3. Generate new UUID and persist
	newID := generateUUID()
	traceIDCacheLock.Lock()
	traceIDCache = newID
	traceIDCacheLock.Unlock()

	if db != nil {
		if err := yakit.SetKey(db, TRACE_ID_DB_KEY, newID); err != nil {
			log.Errorf("failed to persist Trace-ID to database: %v", err)
		}
	}

	log.Infof("generated new persistent Trace-ID: %s", newID)
	return newID
}

// generateUUID creates a new UUID v4 string without importing uuid package
func generateUUID() string {
	// Use crypto/rand for a proper UUID v4
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback: use timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	// Set version 4 and variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
