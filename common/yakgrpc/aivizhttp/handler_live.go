package aivizhttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
)

// LiveSession 表示一个正在运行的 AI agent session
type LiveSession struct {
	SessionID     string `json:"session_id"`
	CoordinatorID string `json:"coordinator_id,omitempty"`
	IsReAct       bool   `json:"is_react"`
	IsCoordinator bool   `json:"is_coordinator"`
}

// LiveSessionsResponse 活跃 session 列表响应
type LiveSessionsResponse struct {
	Sessions []LiveSession `json:"sessions"`
	Total    int           `json:"total"`
}

// handleListLiveSessions 返回当前正在运行的 agent sessions
// GET /live
// 直接从内存注册表 (aireact EnumerateRunningSessions + aid GetRunningCoordinators) 读取,
// 不依赖 DB, 可以实时反映活跃的 agent.
func (s *VizHTTPServer) handleListLiveSessions(w http.ResponseWriter, r *http.Request) {
	result := make([]LiveSession, 0)
	seen := make(map[string]bool)

	// 1. ReAct sessions
	for _, sid := range aireact.EnumerateRunningSessions() {
		if sid == "" || seen[sid] {
			continue
		}
		result = append(result, LiveSession{
			SessionID: sid,
			IsReAct:   true,
		})
		seen[sid] = true
	}

	// 2. Coordinator sessions
	for _, c := range aid.GetRunningCoordinators() {
		if c == nil || c.Config == nil {
			continue
		}
		sid := c.Config.PersistentSessionId
		if sid == "" {
			sid = c.Config.Id
		}
		if sid != "" && !seen[sid] {
			result = append(result, LiveSession{
				SessionID:     sid,
				CoordinatorID: c.Config.Id,
				IsCoordinator: true,
			})
			seen[sid] = true
		}
	}

	writeJSON(w, http.StatusOK, LiveSessionsResponse{
		Sessions: result,
		Total:    len(result),
	})
}

// isSessionLive 检查 session 是否正在运行 (内存注册表)
func isSessionLive(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if _, ok := aireact.GetRunningSession(sessionID); ok {
		return true
	}
	return false
}
