package aibalance

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// CONST_MEMFIT_TOTP_SECRET is the key used to store TOTP secret in database
	CONST_MEMFIT_TOTP_SECRET = "MEMFIT_AI_TOTP_SECRET_KEY"
	// MEMFIT_AI_PREFIX is the prefix used to wrap the UUID
	MEMFIT_AI_PREFIX = "MEMFIT-AI"
)

var (
	totpSecretCache     string
	totpSecretCacheLock sync.RWMutex
)

// ErrMemfitTOTPAuthFailed is the error returned when TOTP authentication fails
// Clients can check for this specific error to trigger TOTP secret refresh
var ErrMemfitTOTPAuthFailed = fmt.Errorf("Memfit TOTP authentication failed")

// InitMemfitTOTP initializes the TOTP system
// If no TOTP secret exists in database, it generates a new one
// This should be called during server startup
func InitMemfitTOTP() error {
	log.Info("Initializing Memfit TOTP authentication system...")

	// Try to load existing secret from database
	secret := yakit.GetKey(GetDB(), CONST_MEMFIT_TOTP_SECRET)

	if secret == "" {
		// No secret exists, generate a new one
		log.Info("No TOTP secret found, generating new one...")
		newSecret, err := GenerateNewTOTPSecret()
		if err != nil {
			return fmt.Errorf("failed to generate TOTP secret: %w", err)
		}
		secret = newSecret
		log.Info("New TOTP secret generated and saved to database")
	} else {
		log.Info("TOTP secret loaded from database")
	}

	// Cache the secret in memory
	totpSecretCacheLock.Lock()
	totpSecretCache = secret
	totpSecretCacheLock.Unlock()

	log.Info("Memfit TOTP authentication system initialized successfully")
	return nil
}

// GenerateNewTOTPSecret generates a new UUID as TOTP secret and saves it to database
func GenerateNewTOTPSecret() (string, error) {
	// Generate a new UUID as the secret
	newUUID := uuid.New().String()

	// Save to database
	err := yakit.SetKey(GetDB(), CONST_MEMFIT_TOTP_SECRET, newUUID)
	if err != nil {
		return "", fmt.Errorf("failed to save TOTP secret to database: %w", err)
	}

	// Update cache
	totpSecretCacheLock.Lock()
	totpSecretCache = newUUID
	totpSecretCacheLock.Unlock()

	log.Infof("New TOTP secret generated: %s", newUUID)
	return newUUID, nil
}

// GetTOTPSecret returns the current TOTP secret
func GetTOTPSecret() string {
	totpSecretCacheLock.RLock()
	defer totpSecretCacheLock.RUnlock()

	if totpSecretCache == "" {
		// If cache is empty, try to load from database
		secret := yakit.GetKey(GetDB(), CONST_MEMFIT_TOTP_SECRET)
		if secret != "" {
			totpSecretCacheLock.RUnlock()
			totpSecretCacheLock.Lock()
			totpSecretCache = secret
			totpSecretCacheLock.Unlock()
			totpSecretCacheLock.RLock()
		}
	}

	return totpSecretCache
}

// GetWrappedTOTPUUID returns the TOTP UUID wrapped with MEMFIT-AI prefix/suffix
// Format: MEMFIT-AI<uuid>MEMFIT-AI
func GetWrappedTOTPUUID() string {
	secret := GetTOTPSecret()
	if secret == "" {
		return ""
	}
	return fmt.Sprintf("%s%s%s", MEMFIT_AI_PREFIX, secret, MEMFIT_AI_PREFIX)
}

// IsMemfitModel checks if the model name indicates a Memfit model that requires TOTP
func IsMemfitModel(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(modelName), "memfit-")
}

// VerifyMemfitTOTP verifies the TOTP code from the X-Memfit-OTP-Auth header
// The header value should be base64 encoded TOTP code
// Allows 60 seconds of time drift (WindowSize = 4, which is 2 windows before and 2 after)
func VerifyMemfitTOTP(authHeader string) (bool, error) {
	if authHeader == "" {
		return false, fmt.Errorf("missing X-Memfit-OTP-Auth header")
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(authHeader)
	if err != nil {
		return false, fmt.Errorf("failed to decode base64 auth header: %w", err)
	}

	totpCode := strings.TrimSpace(string(decoded))
	if totpCode == "" {
		return false, fmt.Errorf("empty TOTP code after decoding")
	}

	// Get the secret
	secret := GetTOTPSecret()
	if secret == "" {
		return false, fmt.Errorf("TOTP secret not initialized")
	}

	// Create TOTP config with larger window size for 60 second tolerance
	// Default window is 30 seconds, WindowSize = 4 means ±2 windows = ±60 seconds
	config := twofa.NewTOTPConfig(secret)
	config.WindowSize = 4 // Allow 60 seconds drift (2 windows before + 2 windows after)

	// Verify the code
	result, err := config.Authenticate(totpCode)
	if err != nil {
		log.Warnf("TOTP authentication error: %v", err)
		return false, ErrMemfitTOTPAuthFailed
	}

	if !result {
		log.Warnf("TOTP authentication failed: invalid code")
		return false, ErrMemfitTOTPAuthFailed
	}

	log.Infof("TOTP authentication successful")
	return true, nil
}

// RefreshTOTPSecret regenerates the TOTP secret
// This is useful when clients report TOTP authentication failures
func RefreshTOTPSecret() (string, error) {
	log.Info("Refreshing TOTP secret due to authentication failure...")
	return GenerateNewTOTPSecret()
}

// GetCurrentTOTPCode returns the current TOTP code for the secret
// This is mainly for debugging/admin purposes
func GetCurrentTOTPCode() string {
	secret := GetTOTPSecret()
	if secret == "" {
		return ""
	}
	return twofa.GetUTCCode(secret)
}
