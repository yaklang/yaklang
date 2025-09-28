package sfweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
)

var (
	SfWebLogger      = log.GetLogger("sfweb")
	CHAT_GLM_API_KEY = "CHATGLM_API_KEY"
)

type WebSocketRateLimiter struct {
	interval time.Duration
	lastSent time.Time
	mutex    sync.Mutex
	enabled  bool
}

func NewWebSocketRateLimiter(interval time.Duration) *WebSocketRateLimiter {
	return &WebSocketRateLimiter{
		interval: interval,
		enabled:  interval > 0,
	}
}

func (r *WebSocketRateLimiter) TrySend(conn *websocket.Conn, data interface{}) error {
	if !r.enabled {
		return r.DirectSend(conn, data)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastSent)

	if elapsed < r.interval {
		return nil
	}

	r.lastSent = now
	return r.DirectSend(conn, data)
}

func (r *WebSocketRateLimiter) DirectSend(conn *websocket.Conn, data interface{}) error {
	if err := conn.WriteJSON(data); err != nil {
		return err
	}
	if SfWebLogger.Level == log.DebugLevel {
		bytes, _ := json.Marshal(data)
		SfWebLogger.Debugf("->client (rate limited): %s", bytes)
	}
	return nil
}

type RateLimitedWebSocketWriter struct {
	conn        *websocket.Conn
	rateLimiter *WebSocketRateLimiter
}

func NewRateLimitedWebSocketWriter(conn *websocket.Conn, rateLimiter *WebSocketRateLimiter) *RateLimitedWebSocketWriter {
	return &RateLimitedWebSocketWriter{
		conn:        conn,
		rateLimiter: rateLimiter,
	}
}

func (w *RateLimitedWebSocketWriter) TryWriteJSON(data interface{}) error {
	return w.rateLimiter.TrySend(w.conn, data)
}

func (w *RateLimitedWebSocketWriter) WriteJSON(data interface{}) error {
	return w.rateLimiter.DirectSend(w.conn, data)
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func writeErrorJson(w http.ResponseWriter, err error) {
	errBody, _ := json.Marshal(&ErrorResponse{err.Error()})
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(errBody)
}

func writeJson(w http.ResponseWriter, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		writeErrorJson(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write(body)
}

type StatusCodeResponseWriter struct {
	http.ResponseWriter
	http.Hijacker
	StatusCode int
}

func NewStatusCodeResponseWriter(w http.ResponseWriter) *StatusCodeResponseWriter {
	nw := &StatusCodeResponseWriter{ResponseWriter: w, StatusCode: http.StatusOK}
	if hijacker, ok := w.(http.Hijacker); ok {
		nw.Hijacker = hijacker
	}
	return nw
}

func (w *StatusCodeResponseWriter) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

type LogHTTPResponseWriter struct {
	http.ResponseWriter
	http.Hijacker
	wroteHeader bool
	raw         *bytes.Buffer
	StatusCode  int
}

func NewLogHTTPResponseWriter(w http.ResponseWriter) *LogHTTPResponseWriter {
	nw := &LogHTTPResponseWriter{ResponseWriter: w, raw: &bytes.Buffer{}}
	if hijacker, ok := w.(http.Hijacker); ok {
		nw.Hijacker = hijacker
	}
	return nw
}

func (w *LogHTTPResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.raw.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *LogHTTPResponseWriter) Raw() []byte {
	return w.raw.Bytes()
}

func (w *LogHTTPResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.StatusCode = statusCode
		w.wroteHeader = true
		statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))
		w.raw.WriteString(statusLine)

		w.ResponseWriter.Header().Write(w.raw)
		w.raw.WriteString("\r\n")
	}
	w.ResponseWriter.WriteHeader(statusCode)
}
