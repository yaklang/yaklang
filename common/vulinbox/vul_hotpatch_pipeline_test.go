package vulinbox

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestHotPatchPipelineSearchFlow(t *testing.T) {
	server := newHotPatchPipelineTestServer(t)
	sessionID, sessionKey := hotPatchPipelineBootstrap(t, server)

	body := []byte(`{"keyword":"商品4","status":"已发货","page":1,"size":10}`)
	resp := hotPatchPipelineSignedSearch(t, server, sessionID, sessionKey, body)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}
	if resp.Header().Get("X-Pipeline-Encrypted") != "1" {
		t.Fatalf("expected encrypted response header, got %q", resp.Header().Get("X-Pipeline-Encrypted"))
	}

	plain := hotPatchPipelineDecryptResponse(t, sessionKey, resp.Body.Bytes())
	var result struct {
		RowCount int              `json:"row_count"`
		Rows     []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		t.Fatalf("unmarshal decrypted response failed: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", result.RowCount)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row entry, got %d", len(result.Rows))
	}
	if result.Rows[0]["username"] != "user1" {
		t.Fatalf("unexpected username: %#v", result.Rows[0]["username"])
	}
}

func TestHotPatchPipelineSearchSQLi(t *testing.T) {
	server := newHotPatchPipelineTestServer(t)
	sessionID, sessionKey := hotPatchPipelineBootstrap(t, server)

	body := []byte(`{"keyword":"' OR 1=1 -- ","status":"已发货","page":1,"size":10}`)
	resp := hotPatchPipelineSignedSearch(t, server, sessionID, sessionKey, body)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.Code, resp.Body.String())
	}

	plain := hotPatchPipelineDecryptResponse(t, sessionKey, resp.Body.Bytes())
	var result struct {
		RowCount int              `json:"row_count"`
		Rows     []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		t.Fatalf("unmarshal decrypted response failed: %v", err)
	}
	if result.RowCount < 5 {
		t.Fatalf("expected injection to expose all orders, got %d rows", result.RowCount)
	}
	if !hotPatchPipelineHasUsername(result.Rows, "admin") {
		t.Fatalf("expected injected result to include admin row: %#v", result.Rows)
	}
}

func TestHotPatchPipelineConsoleEntry(t *testing.T) {
	server := newHotPatchPipelineTestServer(t)

	req := httptest.NewRequest(http.MethodGet, hotPatchPipelineConsolePath, nil)
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	if resp.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	if resp.Header().Get("Location") != hotPatchPipelineDocsPath+"#console" {
		t.Fatalf("unexpected redirect target: %q", resp.Header().Get("Location"))
	}
}

func TestHotPatchPipelineDocsPage(t *testing.T) {
	server := newHotPatchPipelineTestServer(t)

	req := httptest.NewRequest(http.MethodGet, hotPatchPipelineDocsPath, nil)
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "前端实操台") {
		t.Fatalf("docs page missing console section: %s", body)
	}
	if !strings.Contains(body, hotPatchPipelineBootstrapPath) {
		t.Fatalf("docs page missing bootstrap path: %s", body)
	}
}

func newHotPatchPipelineTestServer(t *testing.T) *VulinServer {
	t.Helper()

	db, err := newDBM()
	if err != nil {
		t.Fatalf("newDBM failed: %v", err)
	}
	server := &VulinServer{
		database: db,
		router:   mux.NewRouter(),
	}
	server.registerHotPatchPipelineRoute()
	return server
}

func hotPatchPipelineBootstrap(t *testing.T, server *VulinServer) (string, []byte) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, hotPatchPipelineBootstrapPath, nil)
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("bootstrap failed: %d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Ticket string `json:"ticket"`
		IV     string `json:"iv"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal bootstrap response failed: %v", err)
	}

	ticketBytes, err := base64.StdEncoding.DecodeString(payload.Ticket)
	if err != nil {
		t.Fatalf("decode ticket failed: %v", err)
	}
	ivBytes, err := base64.StdEncoding.DecodeString(payload.IV)
	if err != nil {
		t.Fatalf("decode iv failed: %v", err)
	}
	plainTicket, err := codec.AESCBCDecrypt(hotPatchPipelineBootstrapKey, ticketBytes, ivBytes)
	if err != nil {
		t.Fatalf("decrypt ticket failed: %v", err)
	}

	var ticket struct {
		SessionID  string `json:"session_id"`
		SessionKey string `json:"session_key"`
		ExpiresAt  int64  `json:"expires_at"`
	}
	if err = json.Unmarshal(plainTicket, &ticket); err != nil {
		t.Fatalf("unmarshal ticket failed: %v", err)
	}
	if ticket.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("ticket already expired: %d", ticket.ExpiresAt)
	}

	sessionKey, err := base64.StdEncoding.DecodeString(ticket.SessionKey)
	if err != nil {
		t.Fatalf("decode session key failed: %v", err)
	}
	return ticket.SessionID, sessionKey
}

func hotPatchPipelineSignedSearch(
	t *testing.T,
	server *VulinServer,
	sessionID string,
	sessionKey []byte,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()

	timestamp := time.Now().Unix()
	timestampText := strconv.FormatInt(timestamp, 10)
	signature := hex.EncodeToString(
		hotPatchPipelineSignature(
			http.MethodPost,
			hotPatchPipelineOrdersPath,
			timestampText,
			sessionKey,
		),
	)

	req := httptest.NewRequest(http.MethodPost, hotPatchPipelineOrdersPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pipeline-Session", sessionID)
	req.Header.Set("X-Pipeline-Timestamp", timestampText)
	req.Header.Set("X-Pipeline-Signature", signature)

	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)
	return resp
}

func hotPatchPipelineDecryptResponse(t *testing.T, sessionKey, body []byte) []byte {
	t.Helper()

	var envelope struct {
		IV   string `json:"iv"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("unmarshal response envelope failed: %v", err)
	}

	ivBytes, err := base64.StdEncoding.DecodeString(envelope.IV)
	if err != nil {
		t.Fatalf("decode response iv failed: %v", err)
	}
	dataBytes, err := base64.StdEncoding.DecodeString(envelope.Data)
	if err != nil {
		t.Fatalf("decode response data failed: %v", err)
	}
	plain, err := codec.AESCBCDecrypt(sessionKey, dataBytes, ivBytes)
	if err != nil {
		t.Fatalf("decrypt response failed: %v", err)
	}
	return plain
}

func hotPatchPipelineHasUsername(rows []map[string]any, expected string) bool {
	for _, row := range rows {
		if row["username"] == expected {
			return true
		}
	}
	return false
}
