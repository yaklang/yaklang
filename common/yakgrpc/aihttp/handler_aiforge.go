package aihttp

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type generalProgressReceiver interface {
	Recv() (*ypb.GeneralProgress, error)
}

func (gw *AIAgentHTTPGateway) handleCreateAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.CreateAIForgeRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.CreateAIForge(ctx, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "create ai forge failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleUpdateAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.UpdateAIForgeRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.UpdateAIForge(ctx, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "update ai forge failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleDeleteAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.AIForgeFilter
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.DeleteAIForge(ctx, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "delete ai forge failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleQueryAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req := &ypb.QueryAIForgeRequest{}
	if err := readOptionalProtoJSON(r, req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.QueryAIForge(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "query ai forge failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleGetAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.GetAIForgeRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := gw.yakClient.GetAIForge(ctx, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "get ai forge failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleExportAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.ExportAIForgeRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	stream, err := gw.yakClient.ExportAIForge(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "export ai forge failed: "+err.Error())
		return
	}

	streamGeneralProgressSSE(w, stream)
}

func (gw *AIAgentHTTPGateway) handleImportAIForge(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	var req ypb.ImportAIForgeRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	stream, err := gw.yakClient.ImportAIForge(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "import ai forge failed: "+err.Error())
		return
	}

	streamGeneralProgressSSE(w, stream)
}

func streamGeneralProgressSSE(w http.ResponseWriter, stream generalProgressReceiver) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		progress, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Errorf("receive general progress failed: %v", err)
			_ = writeProtoSSEData(w, &ypb.GeneralProgress{
				Percent:     0,
				Message:     err.Error(),
				MessageType: "error",
			})
			return
		}
		if progress == nil {
			continue
		}
		if err := writeProtoSSEData(w, progress); err != nil {
			log.Errorf("marshal general progress failed: %v", err)
			return
		}
	}
}
