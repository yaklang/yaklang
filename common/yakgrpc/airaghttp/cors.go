package airaghttp

import "net/http"

// corsMiddleware 放行所有跨域请求, 不做任何 Origin 校验
// 关键词: no cors restriction, allow any origin, preflight OPTIONS 204
// 注意: 这是有意为之的设计 (用户要求随意跨域调用), 部署时请确保该服务处在可信网络.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPermissiveCORSHeaders(w, r)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setPermissiveCORSHeaders 写入完全放开的跨域响应头
func setPermissiveCORSHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

	requestHeaders := r.Header.Get("Access-Control-Request-Headers")
	if requestHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cache-Control, Last-Event-ID")
	}
	w.Header().Set("Access-Control-Max-Age", "86400")
}
