package aivizhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
)

// TestVizServer_Health 验证 health 端点正常返回
func TestVizServer_Health(t *testing.T) {
	// 尝试用 profile DB; 如果环境没有则跳过
	db := consts.GetGormProjectDatabase()
	if db == nil {
		t.Skip("project database not available in test environment")
	}

	server, err := NewVizHTTPServer(WithPort(0), WithServeFrontend(false))
	if err != nil {
		t.Fatalf("NewVizHTTPServer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/viz/health", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal health response failed: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

// TestVizServer_Frontend 验证前端页面正常返回
func TestVizServer_Frontend(t *testing.T) {
	server, err := NewVizHTTPServer(WithPort(0), WithServeFrontend(true))
	if err != nil {
		t.Skipf("NewVizHTTPServer failed (no DB in test env): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Yaklang Agent Viz") {
		t.Error("frontend HTML does not contain expected title")
	}
}
