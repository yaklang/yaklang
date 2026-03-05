package aihttp

import (
	"net/http"
)

func (gw *AIAgentHTTPGateway) registerRoutes() {

	sub := gw.router.PathPrefix(gw.routePrefix).Subrouter()

	sub.Use(corsMiddleware)

	if gw.enableJWT || gw.enableTOTP {
		sub.Use(gw.authMiddleware)
	}

	sub.HandleFunc("/setting", gw.handleGetSetting).Methods("GET", "OPTIONS")
	sub.HandleFunc("/setting", gw.handleUpdateSetting).Methods("POST", "OPTIONS")

	sub.HandleFunc("/setting/global", gw.handleGetGlobalSetting).Methods("GET", "OPTIONS")
	sub.HandleFunc("/setting/global", gw.handleUpdateGlobalSetting).Methods("POST", "OPTIONS")

	sub.HandleFunc("/setting/aimodels/get", gw.handleListAIModels).Methods("POST", "OPTIONS")
	sub.HandleFunc("/setting/providers/get", gw.handleListAIProviders).Methods("POST", "OPTIONS")
	sub.HandleFunc("/setting/aifocus/get", gw.handleQueryAIFocus).Methods("POST", "OPTIONS")

	sub.HandleFunc("/session", gw.handleCreateSession).Methods("POST", "OPTIONS")
	sub.HandleFunc("/session/all", gw.handleListAllSessions).Methods("GET", "OPTIONS")
	sub.HandleFunc("/session/{run_id}/title", gw.handleUpdateSessionTitle).Methods("POST", "OPTIONS")

	sub.HandleFunc("/run/{run_id}", gw.handleRun).Methods("POST", "OPTIONS")
	sub.HandleFunc("/run/{run_id}/events", gw.handleSSEEvents).Methods("GET", "OPTIONS")
	sub.HandleFunc("/run/{run_id}/events/push", gw.handlePushEvent).Methods("POST", "OPTIONS")
	sub.HandleFunc("/run/{run_id}/cancel", gw.handleCancelRun).Methods("POST", "OPTIONS")

}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-TOTP-Code")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
