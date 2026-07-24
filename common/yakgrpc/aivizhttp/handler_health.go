package aivizhttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/schema"
)

// handleHealth 健康检查
// GET /health
func (s *VizHTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:      "ok",
		DBAvailable: s.db != nil,
	}

	if s.db != nil {
		var count int64
		s.db.Model(&schema.AISession{}).Count(&count)
		resp.SessionCount = count
	}

	writeJSON(w, http.StatusOK, resp)
}
