package aihttp

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handleListAllSessions(w http.ResponseWriter, r *http.Request) {
	items := make(map[string]SessionItem)
	for _, s := range gw.runManager.ListAll() {
		items[s.RunID] = s
	}

	db := gw.getDB()
	if db != nil {
		if metas, err := yakit.QueryAllAISessionMetaOrderByUpdated(db); err == nil {
			for _, meta := range metas {
				if meta == nil {
					continue
				}
				if meta.SessionID == "" {
					continue
				}
				if item, ok := items[meta.SessionID]; ok {
					if item.Title == "" {
						item.Title = meta.Title
					}
					items[meta.SessionID] = item
					continue
				}
				createdAt := meta.UpdatedAt
				if createdAt.IsZero() {
					createdAt = meta.CreatedAt
				}
				items[meta.SessionID] = SessionItem{
					RunID:     meta.SessionID,
					Title:     meta.Title,
					Status:    RunStatusCompleted,
					CreatedAt: createdAt,
					IsAlive:   false,
				}
			}
		}
	}

	result := make([]SessionItem, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	writeJSON(w, http.StatusOK, SessionListResponse{Sessions: result})
}

func (gw *AIAgentHTTPGateway) handleUpdateSessionTitle(w http.ResponseWriter, r *http.Request) {
	runID := mux.Vars(r)["run_id"]
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}
	var req UpdateSessionTitleRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	db := gw.getDB()
	if db == nil {
		writeError(w, http.StatusServiceUnavailable, "project database is unavailable")
		return
	}
	affected, err := yakit.UpdateAISessionMetaTitle(db, runID, req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update session title failed: "+err.Error())
		return
	}
	if affected == 0 {
		if _, err := yakit.CreateOrUpdateAISessionMeta(db, runID, req.Title); err != nil {
			writeError(w, http.StatusInternalServerError, "create session title failed: "+err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"title":  req.Title,
		"status": "updated",
	})
}

func (gw *AIAgentHTTPGateway) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if gw.yakClient == nil {
		writeError(w, http.StatusServiceUnavailable, "grpc client is unavailable")
		return
	}

	req, err := readOptionalDeleteAISessionRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusBadRequest, "request body is empty")
		return
	}
	if !req.GetDeleteAll() && !hasDeleteAISessionFilterCondition(req.GetFilter()) {
		writeError(w, http.StatusBadRequest, "at least one delete filter condition is required")
		return
	}

	gw.cancelAndRemoveDeletedSessions(req)

	resp, err := gw.yakClient.DeleteAISession(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "delete session failed: "+err.Error())
		return
	}

	writeProtoJSON(w, http.StatusOK, resp)
}

func readOptionalDeleteAISessionRequest(r *http.Request) (*ypb.DeleteAISessionRequest, error) {
	body, err := readOptionalRawBody(r)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}

	var req ypb.DeleteAISessionRequest
	if err := readProtoJSONBytes(body, &req); err == nil {
		return &req, nil
	} else {
		var filter ypb.DeleteAISessionFilter
		if filterErr := readProtoJSONBytes(body, &filter); filterErr == nil {
			return &ypb.DeleteAISessionRequest{Filter: &filter}, nil
		} else {
			return nil, fmt.Errorf("invalid request body: request=%v; filter=%v", err, filterErr)
		}
	}
}

func hasDeleteAISessionFilterCondition(filter *ypb.DeleteAISessionFilter) bool {
	if filter == nil {
		return false
	}
	return len(filter.GetSessionID()) > 0 || filter.GetAfterTimestamp() > 0 || filter.GetBeforeTimestamp() > 0
}

func (gw *AIAgentHTTPGateway) cancelAndRemoveDeletedSessions(req *ypb.DeleteAISessionRequest) {
	if req == nil {
		return
	}

	targets := make([]string, 0)
	switch {
	case req.GetDeleteAll():
		for _, item := range gw.runManager.ListAll() {
			targets = append(targets, item.RunID)
		}
	case req.GetFilter() != nil && (req.GetFilter().GetAfterTimestamp() > 0 || req.GetFilter().GetBeforeTimestamp() > 0):
		db := consts.GetGormProjectDatabase()
		if db != nil {
			if sessionIDs, err := yakit.QueryAISessionIDsForDelete(db, req.GetFilter(), false); err == nil {
				targets = append(targets, sessionIDs...)
			}
		}
	default:
		targets = append(targets, req.GetFilter().GetSessionID()...)
	}

	for _, sessionID := range targets {
		if session, ok := gw.runManager.Get(sessionID); ok {
			session.Cancel()
			gw.runManager.Remove(sessionID)
		}
	}
}
