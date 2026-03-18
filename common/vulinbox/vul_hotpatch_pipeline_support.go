package vulinbox

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	hotPatchPipelineDocsPath       = "/api/pipeline/docs"
	hotPatchPipelineConsolePath    = "/api/pipeline/console"
	hotPatchPipelineBootstrapPath  = "/api/pipeline/bootstrap"
	hotPatchPipelineOrdersPath     = "/api/pipeline/orders/search"
	hotPatchPipelineSessionTTL     = 2 * time.Minute
	hotPatchPipelineClockSkew      = 90 * time.Second
	hotPatchPipelineDefaultPerPage = 10
	hotPatchPipelineMaxPerPage     = 20
	hotPatchPipelineDefaultStatus  = "已发货"
)

var hotPatchPipelineBootstrapKey = []byte("YakitPipeBootKey")

type hotPatchPipelineSession struct {
	ID        string
	Key       []byte
	ExpiresAt time.Time
}

type hotPatchPipelineSessionStore struct {
	mu       sync.Mutex
	sessions map[string]hotPatchPipelineSession
}

func newHotPatchPipelineSessionStore() *hotPatchPipelineSessionStore {
	return &hotPatchPipelineSessionStore{
		sessions: make(map[string]hotPatchPipelineSession),
	}
}

func (s *hotPatchPipelineSessionStore) issueTicket() (map[string]any, error) {
	sessionID, err := hotPatchPipelineRandomHex(8)
	if err != nil {
		return nil, err
	}
	sessionKey := make([]byte, 16)
	if _, err = rand.Read(sessionKey); err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(hotPatchPipelineSessionTTL)

	s.mu.Lock()
	s.sessions[sessionID] = hotPatchPipelineSession{
		ID:        sessionID,
		Key:       append([]byte(nil), sessionKey...),
		ExpiresAt: expiresAt,
	}
	s.mu.Unlock()

	plaintext, err := json.Marshal(map[string]any{
		"session_id":  sessionID,
		"session_key": base64.StdEncoding.EncodeToString(sessionKey),
		"expires_at":  expiresAt.Unix(),
	})
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err = rand.Read(iv); err != nil {
		return nil, err
	}
	ciphertext, err := codec.AESCBCEncrypt(hotPatchPipelineBootstrapKey, plaintext, iv)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"ticket":     base64.StdEncoding.EncodeToString(ciphertext),
		"iv":         base64.StdEncoding.EncodeToString(iv),
		"expires_in": int(hotPatchPipelineSessionTTL.Seconds()),
		"request":    "plain-json-with-hmac",
		"response":   "aes-cbc-envelope",
	}, nil
}

func (s *hotPatchPipelineSessionStore) get(sessionID string) (hotPatchPipelineSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return hotPatchPipelineSession{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		return hotPatchPipelineSession{}, false
	}
	return session, true
}

func hotPatchPipelineWritePlainJSON(writer http.ResponseWriter, status int, payload map[string]any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func hotPatchPipelineWriteEncryptedJSON(writer http.ResponseWriter, status int, session hotPatchPipelineSession, payload map[string]any) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("marshal pipeline payload failed: %v", err)
		hotPatchPipelineWritePlainJSON(writer, http.StatusInternalServerError, map[string]any{
			"message": "marshal response failed",
		})
		return
	}

	iv := make([]byte, aes.BlockSize)
	if _, err = rand.Read(iv); err != nil {
		log.Errorf("generate pipeline iv failed: %v", err)
		hotPatchPipelineWritePlainJSON(writer, http.StatusInternalServerError, map[string]any{
			"message": "generate response iv failed",
		})
		return
	}

	ciphertext, err := codec.AESCBCEncrypt(session.Key, plaintext, iv)
	if err != nil {
		log.Errorf("encrypt pipeline payload failed: %v", err)
		hotPatchPipelineWritePlainJSON(writer, http.StatusInternalServerError, map[string]any{
			"message": "encrypt response failed",
		})
		return
	}

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Pipeline-Encrypted", "1")
	writer.Header().Set("X-Pipeline-Session", session.ID)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"session_id": session.ID,
		"iv":         base64.StdEncoding.EncodeToString(iv),
		"data":       base64.StdEncoding.EncodeToString(ciphertext),
	})
}

func hotPatchPipelineSignature(method, path, timestamp string, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(path))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(timestamp))
	return mac.Sum(nil)
}

func hotPatchPipelinePaging(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = hotPatchPipelineDefaultPerPage
	}
	if size > hotPatchPipelineMaxPerPage {
		size = hotPatchPipelineMaxPerPage
	}
	return page, size
}

func hotPatchPipelineNormalizeRows(rows []map[string]interface{}) []map[string]any {
	normalized := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]any, len(row))
		for key, value := range row {
			item[key] = hotPatchPipelineNormalizeValue(value)
		}
		normalized = append(normalized, item)
	}
	return normalized
}

func hotPatchPipelineNormalizeValue(value any) any {
	if value == nil {
		return nil
	}
	switch ret := value.(type) {
	case *interface{}:
		return hotPatchPipelineNormalizeValue(*ret)
	case []byte:
		return string(ret)
	default:
		return ret
	}
}

func hotPatchPipelineRandomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func hotPatchPipelineTimeDelta(a, b time.Time) time.Duration {
	if a.After(b) {
		return a.Sub(b)
	}
	return b.Sub(a)
}
