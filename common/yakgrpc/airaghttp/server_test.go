package airaghttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer 构造一个不经过 DB 校验的测试服务 (注入固定可用集合)
func newTestServer(authToken string, concurrent int) *RAGHTTPServer {
	cfg := NewDefaultConfig()
	cfg.AuthToken = authToken
	if concurrent > 0 {
		cfg.Concurrent = concurrent
	}
	s := &RAGHTTPServer{
		config:           cfg,
		db:               nil, // 测试中不依赖真实 DB
		readyCollections: []string{"test-kb-1", "test-kb-2"},
		ctx:              context.Background(),
	}
	s.registerRoutes()
	return s
}

func TestRAGHTTPServer_Health(t *testing.T) {
	s := newTestServer("", 0)
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body not json: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("health ok expected true, got %v", body["ok"])
	}
	if body["collectionCount"].(float64) != 2 {
		t.Fatalf("collectionCount expected 2, got %v", body["collectionCount"])
	}
}

func TestRAGHTTPServer_Collections(t *testing.T) {
	s := newTestServer("", 0)
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/collections", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("collections expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("collections body not json: %v", err)
	}
	if body["total"].(float64) != 2 {
		t.Fatalf("collections total expected 2, got %v", body["total"])
	}
}

func TestRAGHTTPServer_CORS(t *testing.T) {
	s := newTestServer("", 0)

	// 普通请求带放开的 CORS 头
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Allow-Origin *, got %q", got)
	}

	// OPTIONS 预检返回 204
	optReq := httptest.NewRequest(http.MethodOptions, "/api/rag-server/chat", nil)
	optRec := httptest.NewRecorder()
	s.ServeHTTP(optRec, optReq)
	if optRec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS expected 204, got %d", optRec.Code)
	}
}

func TestRAGHTTPServer_AuthRequired(t *testing.T) {
	s := newTestServer("secret-token", 0)

	// 无 token -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token expected 401, got %d", rec.Code)
	}

	// 错误 token -> 401
	badReq := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	badReq.Header.Set("Authorization", "Bearer wrong")
	badRec := httptest.NewRecorder()
	s.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token expected 401, got %d", badRec.Code)
	}

	// 正确 token -> 200
	okReq := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	okReq.Header.Set("Authorization", "Bearer secret-token")
	okRec := httptest.NewRecorder()
	s.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("correct token expected 200, got %d", okRec.Code)
	}
}

func TestRAGHTTPServer_AuthDisabled(t *testing.T) {
	s := newTestServer("", 0)
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/health", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth disabled expected 200, got %d", rec.Code)
	}
}

func TestRAGHTTPServer_SearchMissingQuery(t *testing.T) {
	s := newTestServer("", 0)
	req := httptest.NewRequest(http.MethodPost, "/api/rag-server/search", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("search missing query expected 400, got %d", rec.Code)
	}
}

func TestRAGHTTPServer_ChatMissingQuestion(t *testing.T) {
	s := newTestServer("", 0)
	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/chat", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("chat missing question expected 400, got %d", rec.Code)
	}
}

func TestRAGHTTPServer_ChatBusy429(t *testing.T) {
	s := newTestServer("", 1)
	// 手动占满唯一并发槽位
	if !s.acquireSlot() {
		t.Fatal("failed to acquire the only slot")
	}
	defer s.releaseSlot()

	req := httptest.NewRequest(http.MethodGet, "/api/rag-server/chat?q=hello", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("chat busy expected 429, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "server is busy") {
		t.Fatalf("chat busy body expected busy message, got %q", rec.Body.String())
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rag-server.yaml")
	content := `host: "127.0.0.1"
port: 18080
route_prefix: "/api/rag-server"
auth_token: "abc"
concurrent: 5
timeout: 60
max_iteration: 2
language: "en"
collections:
  - kb-a
  - kb-b
ai:
  type: "openai"
  model: "gpt-4o"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 18080 {
		t.Fatalf("unexpected host/port: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.AuthToken != "abc" || cfg.Concurrent != 5 || cfg.MaxIteration != 2 {
		t.Fatalf("unexpected config values: %+v", cfg)
	}
	if cfg.Language != "en" || cfg.AI.Type != "openai" || cfg.AI.Model != "gpt-4o" {
		t.Fatalf("unexpected ai/language: %+v", cfg)
	}
	if len(cfg.Collections) != 2 || cfg.Collections[0] != "kb-a" {
		t.Fatalf("unexpected collections: %v", cfg.Collections)
	}
	if !cfg.UseCustomAIConfig() {
		t.Fatal("expected UseCustomAIConfig true (model set)")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg.Port != 9093 || cfg.RoutePrefix != "/api/rag-server" {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
	if cfg.UseCustomAIConfig() {
		t.Fatal("default config should use tiered ai (no custom)")
	}
}
