package aihttp

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
)

// registerRoutes sets up all HTTP routes for the gateway
func (gw *AIAgentHTTPGateway) registerRoutes() {
	// Create a subrouter with the prefix
	sub := gw.router.PathPrefix(gw.routePrefix).Subrouter()

	// Apply CORS middleware
	sub.Use(corsMiddleware)

	// Apply authentication middleware if enabled
	if gw.authEnabled {
		sub.Use(gw.authMiddleware)
	}

	// Settings endpoints
	sub.HandleFunc("/setting", gw.handleGetSetting).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/setting", gw.handlePostSetting).Methods(http.MethodPost, http.MethodOptions)

	// Run management endpoints
	sub.HandleFunc("/run", gw.handleCreateRun).Methods(http.MethodPost, http.MethodOptions)
	sub.HandleFunc("/run/{run_id}", gw.handleGetRunResult).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/run/{run_id}/events", gw.handleSSEEvents).Methods(http.MethodGet, http.MethodOptions)
	sub.HandleFunc("/run/{run_id}/events/push", gw.handlePushEvent).Methods(http.MethodPost, http.MethodOptions)
	sub.HandleFunc("/run/{run_id}/cancel", gw.handleCancelRun).Methods(http.MethodPost, http.MethodOptions)

	log.Infof("Registered routes with prefix: %s", gw.routePrefix)
	log.Infof("  GET    %s/setting", gw.routePrefix)
	log.Infof("  POST   %s/setting", gw.routePrefix)
	log.Infof("  POST   %s/run", gw.routePrefix)
	log.Infof("  GET    %s/run/{run_id}", gw.routePrefix)
	log.Infof("  GET    %s/run/{run_id}/events", gw.routePrefix)
	log.Infof("  POST   %s/run/{run_id}/events/push", gw.routePrefix)
	log.Infof("  POST   %s/run/{run_id}/cancel", gw.routePrefix)
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-TOTP-Code")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getRunID extracts the run_id from the request path
func getRunID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["run_id"]
}
