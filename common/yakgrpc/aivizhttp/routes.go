package aivizhttp

import (
	"net/http"

	"github.com/gorilla/mux"
)

// registerRoutes 注册路由 (统一前缀, 放开跨域, 可选认证)
func (s *VizHTTPServer) registerRoutes() {
	s.router = mux.NewRouter()
	sub := s.router.PathPrefix(s.config.RoutePrefix).Subrouter()

	sub.Use(corsMiddleware)
	if s.config.AuthToken != "" {
		sub.Use(s.authMiddleware)
	}

	// 健康检查
	sub.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet, http.MethodOptions)

	// Session 列表/详情
	sub.HandleFunc("/sessions", s.handleListSessions).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/sessions/{sessionId}", s.handleSessionDetail).Methods(http.MethodGet, http.MethodOptions)

	// 活跃 session (内存注册表, 不依赖 DB)
	sub.HandleFunc("/live", s.handleListLiveSessions).Methods(http.MethodGet, http.MethodOptions)

	// 事件流 (历史 + 实时 SSE)
	sub.HandleFunc("/sessions/{sessionId}/events", s.handleSessionEvents).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/sessions/{sessionId}/stream", s.handleSSEStream).Methods(http.MethodGet, http.MethodOptions)

	// 工具调用汇总
	sub.HandleFunc("/sessions/{sessionId}/tools", s.handleSessionTools).Methods(http.MethodGet, http.MethodOptions)

	// 统计
	sub.HandleFunc("/sessions/{sessionId}/stats", s.handleSessionStats).Methods(http.MethodGet, http.MethodOptions)

	// 时间线
	sub.HandleFunc("/sessions/{sessionId}/timeline", s.handleSessionTimeline).Methods(http.MethodGet, http.MethodOptions)

	// Context 投影 (参考 kimi-code context-projector.ts)
	sub.HandleFunc("/sessions/{sessionId}/context", s.handleSessionContext).Methods(http.MethodGet, http.MethodOptions)

	// 执行轨迹树
	sub.HandleFunc("/sessions/{sessionId}/trajectory", s.handleSessionTrajectory).Methods(http.MethodGet, http.MethodOptions)

	// 内置前端页面 (不走鉴权)
	if s.config.ServeFrontend {
		s.router.HandleFunc("/", s.handleFrontend).Methods(http.MethodGet)
		s.router.HandleFunc("/index.html", s.handleFrontend).Methods(http.MethodGet)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Vary", "Origin")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	requestHeaders := r.Header.Get("Access-Control-Request-Headers")
	if requestHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cache-Control, Last-Event-ID")
	}
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (s *VizHTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		if token != s.config.AuthToken {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleFrontend 返回内置 dashboard 页面
func (s *VizHTTPServer) handleFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(renderFrontendHTML(s.config.RoutePrefix, DefaultTitle)))
}
