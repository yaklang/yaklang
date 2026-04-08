package aihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleListAIProviders(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.ListAIProviders(ctx, &ypb.Empty{})
	if err != nil {
		writeError(w, http.StatusBadGateway, "list ai providers failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleQueryAIProviders(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.QueryAIProvidersRequest{}
	if r.Body != nil && r.ContentLength > 0 {
		if err := readJSON(r, req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.QueryAIProvider(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "query ai providers failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleQueryAIFocus(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.QueryAIFocusRequest{}
	if r.Body != nil && r.ContentLength > 0 {
		if err := readJSON(r, req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.QueryAIFocus(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "query ai focus failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
