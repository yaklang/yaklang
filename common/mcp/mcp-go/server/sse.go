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
	writer    http.ResponseWriter
	flusher   http.Flusher
	closeOnce sync.Once
	done      chan struct{}
}

var allowedMessageOriginExtensionSchemes = map[string]struct{}{
	"chrome-extension":      {},
	"moz-extension":         {},
	"safari-web-extension":  {},
}

var allowedMessageOriginLocalHosts = map[string]struct{}{
	"127.0.0.1": {},
	"::1":       {},
	"localhost": {},
}

func (s *sseSession) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
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

	s.sessions.Store(sessionID, session)
	defer s.sessions.Delete(sessionID)

	messageEndpoint := fmt.Sprintf(
		"%s/message?sessionId=%s",
		s.baseURL,
		sessionID,
	)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\r\n\r\n", messageEndpoint)
	flusher.Flush()

	<-r.Context().Done()
	session.Close()
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

	// Set the client context in the server before handling the message
	ctx := withTransportContext(r.Context(), legacySSETransport)
	ctx = s.server.WithContext(ctx, NotificationContext{
		ClientID:  sessionID,
		SessionID: sessionID,
	})

	sessionI, ok := s.sessions.Load(sessionID)
	if !ok {
		s.writeJSONRPCError(w, nil, mcp.INVALID_PARAMS, "Invalid session ID")
		return
	}
	session := sessionI.(*sseSession)

	// Parse message as raw JSON
	var rawMessage json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawMessage); err != nil {
		s.writeJSONRPCError(w, nil, mcp.PARSE_ERROR, "Parse error")
		return
	}

	// Process message through MCPServer
	response := s.server.HandleMessage(ctx, rawMessage)

	// Only send response if there is one (not for notifications)
	if response != nil {
		eventData, _ := json.Marshal(response)
		fmt.Fprintf(session.writer, "event: message\ndata: %s\n\n", eventData)
		session.flusher.Flush()

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

	select {
	case <-session.done:
		return fmt.Errorf("session closed")
	default:
		fmt.Fprintf(session.writer, "event: message\ndata: %s\n\n", eventData)
		session.flusher.Flush()
		return nil
	}
}
