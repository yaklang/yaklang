package aihttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleGetSetting handles GET /agent/setting
func (gw *AIAgentHTTPGateway) handleGetSetting(w http.ResponseWriter, r *http.Request) {
	setting := gw.GetDefaultSetting()
	writeJSON(w, SettingResponse{
		Setting: setting,
	})
}

// handlePostSetting handles POST /agent/setting
func (gw *AIAgentHTTPGateway) handlePostSetting(w http.ResponseWriter, r *http.Request) {
	var req ypb.AIStartParams
	if err := readJSON(r, &req); err != nil {
		log.Debugf("Failed to parse setting request: %v", err)
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
		return
	}

	gw.SetDefaultSetting(&req)
	log.Infof("Updated AI default settings")

	writeJSON(w, SettingResponse{
		Setting: &req,
	})
}
