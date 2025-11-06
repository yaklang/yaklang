package vulinbox

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed html/group_demo_inventory.html
var groupFuzzerDemoPage []byte

type inventorySession struct {
	Item       string
	Remaining  int
	Successful []string

	mu sync.RWMutex
}

type inventorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*inventorySession
}

func newInventorySessionStore() *inventorySessionStore {
	return &inventorySessionStore{
		sessions: make(map[string]*inventorySession),
	}
}

func (s *inventorySessionStore) set(id string, session *inventorySession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = session
}

func (s *inventorySessionStore) get(id string) (*inventorySession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

var groupFuzzerSessions = newInventorySessionStore()

func (s *VulinServer) registerGroupFuzzerDemo() {
	group := s.router.Name("Group HTTP Fuzzer Demo").Subrouter()

	addRouteWithVulInfo(group, &VulInfo{
		Path:    "/group-demo/inventory",
		Title:   "库存竞态演示（Group HTTP Fuzzer）",
		Handler: s.groupFuzzerDemoPageHandler,
	})
	group.HandleFunc("/group-demo/inventory/", s.groupFuzzerDemoPageHandler).Methods(http.MethodGet)

	api := group.PathPrefix("/group-demo/inventory").Subrouter()
	api.HandleFunc("/init", s.groupFuzzerInventoryInit).Methods(http.MethodPost)
	api.HandleFunc("/buy", s.groupFuzzerInventoryBuy).Methods(http.MethodPost)
	api.HandleFunc("/status", s.groupFuzzerInventoryStatus).Methods(http.MethodGet)
}

func (s *VulinServer) groupFuzzerDemoPageHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write(groupFuzzerDemoPage); err != nil {
		log.Errorf("write group demo page failed: %v", err)
	}
}

func (s *VulinServer) groupFuzzerInventoryInit(writer http.ResponseWriter, request *http.Request) {
	type initReq struct {
		Item  string `json:"item"`
		Stock int    `json:"stock"`
	}
	var req initReq
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.Stock <= 0 {
		req.Stock = 5
	}
	if req.Item == "" {
		req.Item = "yaklang 限量周边"
	}

	sessionID := uuid.NewString()
	groupFuzzerSessions.set(sessionID, &inventorySession{
		Item:      req.Item,
		Remaining: req.Stock,
	})

	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"session_id": sessionID,
		"item":       req.Item,
		"stock":      req.Stock,
	})
}

func (s *VulinServer) groupFuzzerInventoryBuy(writer http.ResponseWriter, request *http.Request) {
	type buyReq struct {
		SessionID string `json:"session_id"`
		User      string `json:"user"`
	}
	var req buyReq
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(writer, "session_id required", http.StatusBadRequest)
		return
	}
	if req.User == "" {
		req.User = "anonymous"
	}

	session, ok := groupFuzzerSessions.get(req.SessionID)
	if !ok || session == nil {
		http.Error(writer, "session not found", http.StatusNotFound)
		return
	}

	session.mu.RLock()
	if session.Remaining <= 0 {
		remaining := session.Remaining
		session.mu.RUnlock()
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ok":        false,
			"message":   "库存不足",
			"remaining": remaining,
		})
		return
	}
	session.mu.RUnlock()

	time.Sleep(20 * time.Millisecond)

	session.mu.Lock()
	session.Remaining--
	session.Successful = append(session.Successful, req.User)
	remaining := session.Remaining
	session.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"ok":        true,
		"message":   req.User + " 抢到了 " + session.Item,
		"remaining": remaining,
		"item":      session.Item,
	})
}

func (s *VulinServer) groupFuzzerInventoryStatus(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(writer, "session_id required", http.StatusBadRequest)
		return
	}

	session, ok := groupFuzzerSessions.get(sessionID)
	if !ok || session == nil {
		http.Error(writer, "session not found", http.StatusNotFound)
		return
	}

	session.mu.RLock()
	resp := map[string]any{
		"item":      session.Item,
		"remaining": session.Remaining,
		"success":   append([]string(nil), session.Successful...),
	}
	session.mu.RUnlock()

	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(resp)
}
