package aihttp

import (
	"context"
	"net/http"
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
		var err error
		session, _, err = gw.ensureReusableSession(runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
			return
		}
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

	if err := writeProtoSSEData(w, newSystemOutputEvent("listener_ready")); err != nil {
		log.Errorf("marshal listener_ready event: %v", err)
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-session.ctx.Done():
			if err := writeProtoSSEData(w, buildTerminalRunEvent(session.Status, session.Error)); err != nil {
				log.Errorf("marshal terminal event: %v", err)
			}
			return
		case event, ok := <-ch:
			if !ok {
				return
			}

			if err := writeProtoSSEData(w, event); err != nil {
				log.Errorf("marshal event: %v", err)
				continue
			}

			if isTerminalRunEventType(event.GetType()) {
				return
			}
		case <-heartbeat.C:
			if err := writeProtoSSEData(w, newSystemOutputEvent("heartbeat")); err != nil {
				log.Errorf("marshal heartbeat event: %v", err)
			}
		}
	}
}

func (gw *AIAgentHTTPGateway) queryHistoricalRunEvents(ctx context.Context, runID string, since int64) ([]*ypb.AIOutputEvent, error) {
	if gw.yakClient == nil {
		return nil, nil
	}

	const pageSize int64 = 200
	page := int64(1)
	historical := make([]*ypb.AIOutputEvent, 0, pageSize)

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
			event := normalizeOutputEvent(item)
			if since > 0 && event.GetTimestamp() <= since {
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

func buildTerminalRunEvent(status RunStatus, errMsg string) *ypb.AIOutputEvent {
	if status == RunStatusFailed {
		if errMsg == "" {
			return newFailedOutputEvent(nil)
		}
		return newFailedOutputEvent(&terminalError{message: errMsg})
	}
	return newResultOutputEvent(string(status))
}

type terminalError struct {
	message string
}

func (e *terminalError) Error() string {
	return e.message
}
