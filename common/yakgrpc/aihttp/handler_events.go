package aihttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleQueryAIEvent(w http.ResponseWriter, r *http.Request) {
	var req ypb.AIEventQueryRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := gw.yakClient.QueryAIEvent(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query AI event failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func (gw *AIAgentHTTPGateway) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	runID := mux.Vars(r)["run_id"]

	session, ok := gw.runManager.Get(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found: "+runID)
		return
	}

	// var since int64
	// if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
	// 	if v, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
	// 		since = v
	// 	}
	// }

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	subID, ch := session.Subscribe()
	defer session.Unsubscribe(subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeSSEData(w, `{"type":"listener_ready","status":"ok","run_id":"`+runID+`"}`)

	if session.Status == RunStatusCompleted || session.Status == RunStatusFailed || session.Status == RunStatusCancelled {
		writeSSEData(w, `{"type":"done","status":"`+string(session.Status)+`"}`)
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-session.ctx.Done():
			writeSSEData(w, `{"type":"done","status":"`+string(session.Status)+`"}`)
			flusher.Flush()
			return
		case event, ok := <-ch:
			if !ok {
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				log.Errorf("marshal event: %v", err)
				continue
			}
			writeSSEData(w, string(data))

			if event.Type == "done" || event.Type == "error" {
				return
			}
		case <-heartbeat.C:
			writeSSEData(w, `{"type":"heartbeat","timestamp":`+strconv.FormatInt(time.Now().Unix(), 10)+`}`)
		}
	}
}

func (gw *AIAgentHTTPGateway) queryHistoricalRunEvents(ctx context.Context, runID string, since int64) ([]RunEvent, error) {
	if gw.yakClient == nil {
		return nil, nil
	}

	const pageSize int64 = 200
	page := int64(1)
	historical := make([]RunEvent, 0, pageSize)

	for {
		resp, err := gw.yakClient.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
			Filter: &ypb.AIEventFilter{
				SessionID: runID,
			},
			Pagination: &ypb.Paging{
				Page:    page,
				Limit:   pageSize,
				OrderBy: "id",
				Order:   "asc",
			},
		})
		if err != nil {
			return nil, err
		}

		if len(resp.Events) == 0 {
			break
		}

		for _, item := range resp.Events {
			if item == nil {
				continue
			}
			event := convertOutputToRunEvent(item)
			if since > 0 && event.Timestamp <= since {
				continue
			}
			historical = append(historical, event)
		}

		if int64(len(resp.Events)) < pageSize {
			break
		}
		page++
	}

	return historical, nil
}
