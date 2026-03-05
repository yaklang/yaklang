package aihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleGetGlobalSetting(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := gw.yakClient.GetGlobalNetworkConfig(ctx, &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		writeError(w, http.StatusBadGateway, "get global network config failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleUpdateGlobalSetting(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.GlobalNetworkConfig{}
	if err := readJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if _, err := gw.yakClient.SetGlobalNetworkConfig(ctx, req); err != nil {
		writeError(w, http.StatusBadGateway, "set global network config failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, req)
}

func (gw *AIAgentHTTPGateway) handleGetThirdPartyAppConfigTemplate(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := gw.yakClient.GetThirdPartyAppConfigTemplate(ctx, &ypb.Empty{})
	if err != nil {
		writeError(w, http.StatusBadGateway, "get third party app config template failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
