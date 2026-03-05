package aihttp

import (
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
