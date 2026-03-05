package aihttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
)

func (gw *AIAgentHTTPGateway) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gw.enableJWT {
			if !gw.validateJWT(r) {
				writeError(w, http.StatusUnauthorized, "invalid or missing JWT token")
				return
			}
		}

		if gw.enableTOTP {
			if !gw.validateTOTP(r) {
				writeError(w, http.StatusUnauthorized, "invalid or missing TOTP code")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (gw *AIAgentHTTPGateway) validateJWT(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}

	tokenStr := parts[1]
	token, _, err := authhack.JwtParse(tokenStr, gw.jwtSecret)
	if err != nil {
		log.Debugf("JWT parse error: %v", err)
		return false
	}

	if token == nil || !token.Valid {
		return false
	}

	return true
}

func (gw *AIAgentHTTPGateway) validateTOTP(r *http.Request) bool {
	code := r.Header.Get("X-TOTP-Code")
	if code == "" {
		return false
	}
	return twofa.VerifyUTCCode(gw.totpSecret, code)
}

func GenerateJWTToken(secret string, expireHours int) (string, error) {
	claims := map[string]any{
		"iss": "ai-agent-gateway",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Duration(expireHours) * time.Hour).Unix(),
	}
	return authhack.JwtGenerate("HS256", claims, "JWT", []byte(secret))
}

func GetCurrentTOTPCode(secret string) string {
	return twofa.GetUTCCode(secret)
}
