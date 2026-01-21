package aihttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
)

const (
	// HeaderAuthorization is the standard authorization header
	HeaderAuthorization = "Authorization"
	// HeaderTOTPCode is the header for TOTP code
	HeaderTOTPCode = "X-TOTP-Code"
	// BearerPrefix is the prefix for Bearer token
	BearerPrefix = "Bearer "
)

// jwtAuthMiddleware validates JWT token from Authorization header
func (gw *AIAgentHTTPGateway) jwtAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(HeaderAuthorization)
		if authHeader == "" {
			writeUnauthorized(w, "missing authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			writeUnauthorized(w, "invalid authorization header format, expected 'Bearer <token>'")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, BearerPrefix)
		if tokenStr == "" {
			writeUnauthorized(w, "empty token")
			return
		}

		// Parse and validate JWT token
		token, key, err := authhack.JwtParse(tokenStr, gw.jwtSecret)
		if err != nil {
			log.Debugf("JWT parse error: %v", err)
			writeUnauthorized(w, "invalid token: "+err.Error())
			return
		}

		if key == nil || !token.Valid {
			writeUnauthorized(w, "token validation failed")
			return
		}

		// Check expiration if exp claim exists
		if claims, ok := token.Claims.(*authhack.OMapClaims); ok && claims != nil {
			if exp := claims.GetExact("exp"); exp != nil {
				expFloat, ok := exp.(float64)
				if ok && time.Now().Unix() > int64(expFloat) {
					writeUnauthorized(w, "token expired")
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// totpAuthMiddleware validates TOTP code from X-TOTP-Code header
func (gw *AIAgentHTTPGateway) totpAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.Header.Get(HeaderTOTPCode)
		if code == "" {
			writeUnauthorized(w, "missing X-TOTP-Code header")
			return
		}

		// Verify TOTP code
		if !twofa.VerifyUTCCode(gw.totpSecret, code) {
			log.Debugf("TOTP verification failed for code: %s", code)
			writeUnauthorized(w, "invalid TOTP code")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// writeUnauthorized writes a 401 Unauthorized response
func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	writeJSON(w, map[string]interface{}{
		"error":   "unauthorized",
		"message": message,
	})
}

// GenerateJWTToken generates a JWT token for authentication
// This is a helper function for clients to get a valid token
func GenerateJWTToken(secret string, claims map[string]interface{}, expiresIn time.Duration) (string, error) {
	if claims == nil {
		claims = make(map[string]interface{})
	}

	// Set expiration
	claims["exp"] = time.Now().Add(expiresIn).Unix()
	claims["iat"] = time.Now().Unix()

	return authhack.JwtGenerate("HS256", claims, "JWT", []byte(secret))
}

// GetCurrentTOTPCode returns the current TOTP code for the given secret
// This is a helper function for clients to get the current valid code
func GetCurrentTOTPCode(secret string) string {
	return twofa.GetUTCCode(secret)
}
