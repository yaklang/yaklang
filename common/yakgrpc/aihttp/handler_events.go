package aihttp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

// handleSSEEvents handles GET /agent/run/{run_id}/events
// This is a Server-Sent Events (SSE) endpoint for real-time event streaming
func (gw *AIAgentHTTPGateway) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	runID := getRunID(r)
	if runID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "run_id is required")
		return
	}

	session, ok := gw.runManager.GetSession(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	// Parse optional 'since' parameter for resumption
	var since int64 = 0
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if parsed, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = parsed
		}
	}

	// Setup SSE
	flusher := setupSSE(w)
	if flusher == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming not supported")
		return
	}

	// Send connection established event
	writeSSE(w, &SSEEvent{
		Event: "connected",
		Data: map[string]interface{}{
			"run_id":    runID,
			"task_id":   session.TaskID,
			"status":    session.Status,
			"timestamp": time.Now().Unix(),
		},
	})

	// Send historical events if 'since' is specified
	if since > 0 {
		historicalEvents := session.GetEventsSince(since)
		for _, event := range historicalEvents {
			writeSSE(w, &SSEEvent{
				ID:    event.EventUUID,
				Event: "message",
				Data:  event,
			})
		}
	}

	// Subscribe to new events
	subscriberID := uuid.New().String()
	eventChan := session.Subscribe(subscriberID)
	defer session.Unsubscribe(subscriberID)

	// Send heartbeat every 30 seconds
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	log.Infof("SSE connection established for run: %s", runID)

	for {
		select {
		case <-r.Context().Done():
			log.Infof("SSE connection closed by client for run: %s", runID)
			return

		case <-session.Context().Done():
			// Session was cancelled
			writeSSE(w, &SSEEvent{
				Event: "done",
				Data: map[string]interface{}{
					"status":  session.Status,
					"message": "session ended",
				},
			})
			return

		case event, ok := <-eventChan:
			if !ok {
				// Channel closed
				return
			}
			writeSSE(w, &SSEEvent{
				ID:    event.EventUUID,
				Event: "message",
				Data:  event,
			})

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			writeSSE(w, &SSEEvent{
				Event: "heartbeat",
				Data: map[string]interface{}{
					"timestamp": time.Now().Unix(),
					"status":    session.Status,
				},
			})
		}

		// Check if session is done
		if session.IsDone() {
			writeSSE(w, &SSEEvent{
				Event: "done",
				Data: map[string]interface{}{
					"status":   session.Status,
					"end_time": session.EndTime,
					"error":    session.Error,
				},
			})
			return
		}
	}
}
