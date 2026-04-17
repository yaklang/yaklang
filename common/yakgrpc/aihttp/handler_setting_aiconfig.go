package aihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleGetAIGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := gw.yakClient.GetAIGlobalConfig(ctx, &ypb.Empty{})
	if err != nil {
		writeError(w, http.StatusBadGateway, "get ai global config failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleUpdateAIGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.AIGlobalConfig{}
	if err := readJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if _, err := gw.yakClient.SetAIGlobalConfig(ctx, req); err != nil {
		writeError(w, http.StatusBadGateway, "set ai global config failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, req)
}

func (gw *AIAgentHTTPGateway) handleAIConfigHealthCheck(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.AIConfigHealthCheckRequest{}
	if err := readProtoJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	resp, err := gw.yakClient.AIConfigHealthCheck(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ai config health check failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}
