package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// SSEServer implements a Server-Sent Events (SSE) based MCP server.
// It provides real-time communication capabilities over HTTP using the SSE protocol.
type SSEServer struct {
	server         *MCPServer
	baseURL        string
	sessions       sync.Map
	srv            *http.Server
	dispatchOnce   sync.Once
	dispatchDone   chan struct{}
	dispatchCancel func()
}

// sseSession represents an active SSE connection.
type sseSession struct {
	mu            sync.Mutex
	metadataMu    sync.RWMutex
	writer        http.ResponseWriter
	flusher       http.Flusher
	closeOnce     sync.Once
	done          chan struct{}
	clientName    string
	clientVersion string
}

func (s *sseSession) notificationContext(sessionID string) NotificationContext {
	s.metadataMu.RLock()
	defer s.metadataMu.RUnlock()
	return NotificationContext{
		ClientID:      sessionID,
		SessionID:     sessionID,
		ClientName:    s.clientName,
		ClientVersion: s.clientVersion,
	}
}

func (s *sseSession) setClientMetadata(clientContext NotificationContext) {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	s.clientName = clientContext.ClientName
	s.clientVersion = clientContext.ClientVersion
}

var allowedMessageOriginExtensionSchemes = map[string]struct{}{
	"chrome-extension":     {},
	"moz-extension":        {},
	"safari-web-extension": {},
}

var allowedMessageOriginLocalHosts = map[string]struct{}{
	"127.0.0.1": {},
	"::1":       {},
	"localhost": {},
}

func (s *sseSession) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		close(s.done)
	})
}

func (sess *sseSession) writeMessageEvent(eventData []byte) (err error) {
	// Assemble the complete event before taking the session lock. All endpoint,
	// message, and keep-alive frames ultimately pass through writeFrameLocked,
	// which prevents concurrent ResponseWriter use and frame interleaving. TCP is
	// still free to split a frame across packets, as required by stream semantics.
	frame := make([]byte, 0, len("event: message\ndata: ")+len(eventData)+2)
	frame = append(frame, "event: message\ndata: "...)
	frame = append(frame, eventData...)
	frame = append(frame, '\n', '\n')
	return sess.writeFrame(frame)
}

func (sess *sseSession) writeKeepAlive() error {
	return sess.writeFrame([]byte(":keepalive\n\n"))
}

func (sess *sseSession) writeFrame(frame []byte) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.writeFrameLocked(frame)
}

// writeFrameLocked is the only code path that writes to the SSE
// ResponseWriter. The caller must hold sess.mu.
func (sess *sseSession) writeFrameLocked(frame []byte) (err error) {
	select {
	case <-sess.done:
		return fmt.Errorf("session closed")
	default:
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sse write: %v", r)
		}
	}()

	if _, werr := sess.writer.Write(frame); werr != nil {
		return werr
	}
	sess.flusher.Flush()
	return nil
}

// NewSSEServer creates a new SSE server instance with the given MCP server and base URL.
func NewSSEServer(server *MCPServer, baseURL string) *SSEServer {
	return &SSEServer{
		server:       server,
		baseURL:      baseURL,
		dispatchDone: make(chan struct{}),
	}
}

func (s *SSEServer) startNotificationDispatcher() {
	s.dispatchOnce.Do(func() {
		notificationCh, unsubscribe := s.server.SubscribeNotifications(100)
		s.dispatchCancel = unsubscribe

		go func() {
			defer unsubscribe()

			for {
				select {
				case <-s.dispatchDone:
					return
				case serverNotification, ok := <-notificationCh:
					if !ok {
						return
					}
					if serverNotification.Context.SessionID == "" {
						// Broadcast server-initiated notifications (e.g. tools/list_changed
						// from AddTool) to every active SSE session.
						s.sessions.Range(func(key, _ any) bool {
							if sessionID, ok := key.(string); ok {
								_ = s.SendEventToSession(sessionID, serverNotification.Notification)
							}
							return true
						})
						continue
					}
					_ = s.SendEventToSession(
						serverNotification.Context.SessionID,
						serverNotification.Notification,
					)
				}
			}
		}()
	})
}

func (s *SSEServer) RegisterHandlers(mux *http.ServeMux) {
	s.startNotificationDispatcher()
	mux.HandleFunc("/sse", s.handleSSE)
	mux.HandleFunc("/message", s.handleMessage)
}

// NewTestServer creates a test server for testing purposes
func NewTestServer(server *MCPServer) *httptest.Server {
	sseServer := &SSEServer{
		server:       server,
		dispatchDone: make(chan struct{}),
	}
	sseServer.startNotificationDispatcher()

	testServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/sse":
				sseServer.handleSSE(w, r)
			case "/message":
				sseServer.handleMessage(w, r)
			default:
				http.NotFound(w, r)
			}
		}),
	)

	sseServer.baseURL = testServer.URL
	return testServer
}

// Start begins serving SSE connections on the specified address.
// It sets up HTTP handlers for SSE and message endpoints.
func (s *SSEServer) Start(addr string) error {
	mux := http.NewServeMux()
	s.RegisterHandlers(mux)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the SSE server, closing all active sessions
// and shutting down the HTTP server.
func (s *SSEServer) Shutdown(ctx context.Context) error {
	select {
	case <-s.dispatchDone:
	default:
		close(s.dispatchDone)
	}
	if s.dispatchCancel != nil {
		s.dispatchCancel()
		s.dispatchCancel = nil
	}
	s.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*sseSession); ok {
			session.Close()
		}
		s.sessions.Delete(key)
		return true
	})

	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// handleSSE handles incoming SSE connection requests.
// It sets up appropriate headers and creates a new session for the client.
func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID := uuid.New().String()
	session := &sseSession{
		writer:    w,
		flusher:   flusher,
		done:      make(chan struct{}),
		closeOnce: sync.Once{},
	}

	// Derive the message endpoint base from the incoming request so that the
	// returned URL always matches the origin the client used to connect.
	// This avoids "Endpoint origin does not match connection origin" errors
	// thrown by strict MCP SDK validators when the server binds to 0.0.0.0
	// but the client connects via a real IP or hostname.
	endpointBase := s.baseURL
	if host := r.Host; host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		endpointBase = scheme + "://" + host
	}
	messageEndpoint := fmt.Sprintf(
		"%s/message?sessionId=%s",
		endpointBase,
		sessionID,
	)
	endpointFrame := []byte(fmt.Sprintf("event: endpoint\ndata: %s\r\n\r\n", messageEndpoint))

	// Register while holding the session write lock, then write and flush the
	// endpoint event before unlocking. A client may POST immediately after
	// receiving the endpoint: it will find the session, but its response write
	// cannot overtake or race the endpoint frame.
	session.mu.Lock()
	s.sessions.Store(sessionID, session)
	endpointErr := session.writeFrameLocked(endpointFrame)
	session.mu.Unlock()
	if endpointErr != nil {
		s.sessions.Delete(sessionID)
		session.Close()
		return
	}
	defer s.sessions.Delete(sessionID)

	// Send periodic SSE keep-alive comments to prevent client body timeouts.
	// Many HTTP clients (Node.js fetch/undici, etc.) close idle SSE streams
	// after ~5 min without data, resulting in "Body Timeout Error".
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			session.Close()
			return
		case <-ticker.C:
			if err := session.writeKeepAlive(); err != nil {
				session.Close()
				return
			}
		}
	}
}

// handleMessage processes incoming JSON-RPC messages from clients and sends responses
// back through both the SSE connection and HTTP response.
func (s *SSEServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if r.Method == http.MethodOptions {
		s.handleMessagePreflight(w, r, origin)
		return
	}

	if r.Method != http.MethodPost {
		s.writeJSONRPCError(w, nil, mcp.INVALID_REQUEST, "Method not allowed")
		return
	}

	if !isAllowedMessageOrigin(origin) {
		s.writeJSONRPCErrorWithStatus(w, nil, mcp.INVALID_REQUEST, "Forbidden origin", http.StatusForbidden)
		return
	}
	setAllowedMessageOriginHeaders(w, origin)

	if err := validateJSONContentType(r.Header.Get("Content-Type")); err != nil {
		s.writeJSONRPCError(w, nil, mcp.INVALID_REQUEST, err.Error())
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		s.writeJSONRPCError(w, nil, mcp.INVALID_PARAMS, "Missing sessionId")
		return
	}

	sessionI, ok := s.sessions.Load(sessionID)
	if !ok {
		s.writeJSONRPCError(w, nil, mcp.INVALID_PARAMS, "Invalid session ID")
		return
	}
	session := sessionI.(*sseSession)

	// Set the client context in the server before handling the message.
	ctx := withTransportContext(r.Context(), legacySSETransport)
	ctx = s.server.WithContext(ctx, session.notificationContext(sessionID))

	// Parse message as raw JSON
	var rawMessage json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawMessage); err != nil {
		s.writeJSONRPCError(w, nil, mcp.PARSE_ERROR, "Parse error")
		return
	}

	// Process message through MCPServer
	response := s.server.HandleMessage(ctx, rawMessage)
	if scoped := ServerFromContext(ctx); scoped != nil {
		session.setClientMetadata(scoped.CurrentClientContext())
	}

	// Only send response if there is one (not for notifications)
	if response != nil {
		eventData, _ := json.Marshal(response)
		_ = session.writeMessageEvent(eventData)

		// Send HTTP response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)
	} else {
		// For notifications, just send 202 Accepted with no body
		w.WriteHeader(http.StatusAccepted)
	}
}

// writeJSONRPCError writes a JSON-RPC error response with the given error details.
func (s *SSEServer) writeJSONRPCError(
	w http.ResponseWriter,
	id interface{},
	code int,
	message string,
) {
	s.writeJSONRPCErrorWithStatus(w, id, code, message, http.StatusBadRequest)
}

func (s *SSEServer) writeJSONRPCErrorWithStatus(
	w http.ResponseWriter,
	id interface{},
	code int,
	message string,
	status int,
) {
	response := createErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func (s *SSEServer) handleMessagePreflight(w http.ResponseWriter, r *http.Request, origin string) {
	if !isAllowedMessageOrigin(origin) {
		http.Error(w, "Forbidden origin", http.StatusForbidden)
		return
	}

	setAllowedMessageOriginHeaders(w, origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

func setAllowedMessageOriginHeaders(w http.ResponseWriter, origin string) {
	if origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
}

func isAllowedMessageOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	if origin == "null" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if _, ok := allowedMessageOriginExtensionSchemes[parsed.Scheme]; ok {
		return parsed.Host != ""
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	_, ok := allowedMessageOriginLocalHosts[parsed.Hostname()]
	return ok
}

// SendEventToSession sends an event to a specific SSE session identified by sessionID.
// Returns an error if the session is not found or closed.
func (s *SSEServer) SendEventToSession(
	sessionID string,
	event interface{},
) error {
	sessionI, ok := s.sessions.Load(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	session := sessionI.(*sseSession)

	eventData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return session.writeMessageEvent(eventData)
}
