package airaghttp

import (
	"net/http"
	"strings"
)

// authMiddleware 可选的 Bearer 认证中间件
// 关键词: optional bearer auth, Authorization header
// 仅当 server.authToken 非空时才会被挂载 (见 server.go registerRoutes).
func (s *RAGHTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 预检请求直接放行 (CORS 中间件已经处理 OPTIONS, 这里做兜底)
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if !s.validateBearer(r) {
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateBearer 校验 Authorization: Bearer <token>
func (s *RAGHTTPServer) validateBearer(r *http.Request) bool {
	if s.config.AuthToken == "" {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}

	return strings.TrimSpace(parts[1]) == s.config.AuthToken
}
