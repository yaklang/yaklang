package aihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleListAIModels(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.ListAiModelRequest{}
	if err := readJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.GetConfig() == "" {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.ListAiModel(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list ai models failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
