package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

const DefaultStreamableHTTPPath = "/mcp"

type StreamableHTTPServer struct {
	server         *MCPServer
	baseURL        string
	endpointPath   string
	sessions       sync.Map
	srv            *http.Server
	dispatchOnce   sync.Once
	dispatchDone   chan struct{}
	dispatchCancel func()
}

type streamableHTTPSession struct {
	id              string
	protocolVersion string
	streams         sync.Map
	closeOnce       sync.Once
	done            chan struct{}
}

func (s *streamableHTTPSession) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.streams.Range(func(key, value interface{}) bool {
			if stream, ok := value.(*streamableHTTPEventStream); ok {
				stream.Close()
			}
			s.streams.Delete(key)
			return true
		})
	})
}

func (s *streamableHTTPSession) send(message interface{}) error {
	var sendErr error

	s.streams.Range(func(_, value interface{}) bool {
		stream, ok := value.(*streamableHTTPEventStream)
		if !ok {
			return true
		}
		sendErr = stream.Send(message)
		return sendErr != nil
	})

	return sendErr
}

type streamableHTTPEventStream struct {
	writer    http.ResponseWriter
	flusher   http.Flusher
	closeOnce sync.Once
	done      chan struct{}
	writeMu   sync.Mutex
}

func (s *streamableHTTPEventStream) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

func (s *streamableHTTPEventStream) Send(message interface{}) error {
	eventData, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case <-s.done:
		return fmt.Errorf("stream closed")
	default:
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := fmt.Fprintf(s.writer, "data: %s\n\n", eventData); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func NewStreamableHTTPServer(
	server *MCPServer,
	baseURL string,
) *StreamableHTTPServer {
	return &StreamableHTTPServer{
		server:       server,
		baseURL:      baseURL,
		endpointPath: DefaultStreamableHTTPPath,
		dispatchDone: make(chan struct{}),
	}
}

func NewStreamableHTTPTestServer(server *MCPServer) *httptest.Server {
	httpServer := NewStreamableHTTPServer(server, "")
	httpServer.startNotificationDispatcher()

	mux := http.NewServeMux()
	httpServer.RegisterHandlers(mux)
	testServer := httptest.NewServer(mux)
	httpServer.baseURL = testServer.URL
	return testServer
}

func (s *StreamableHTTPServer) RegisterHandlers(mux *http.ServeMux) {
	s.startNotificationDispatcher()
	mux.HandleFunc(s.endpointPath, s.handleTransport)
}

func (s *StreamableHTTPServer) Start(addr string) error {
	mux := http.NewServeMux()
	s.RegisterHandlers(mux)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s.srv.ListenAndServe()
}

func (s *StreamableHTTPServer) Shutdown(ctx context.Context) error {
	if s.dispatchDone != nil {
		select {
		case <-s.dispatchDone:
		default:
			close(s.dispatchDone)
		}
	}
	if s.dispatchCancel != nil {
		s.dispatchCancel()
		s.dispatchCancel = nil
	}

	s.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*streamableHTTPSession); ok {
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

func (s *StreamableHTTPServer) startNotificationDispatcher() {
	s.dispatchOnce.Do(func() {
		notificationCh, unsubscribe := s.server.SubscribeNotifications(100)
		s.dispatchCancel = unsubscribe

		go func() {
			defer unsubscribe()

			for {
				select {
				case <-s.dispatchDone:
					return
				case notification, ok := <-notificationCh:
					if !ok {
						return
					}
					if notification.Context.SessionID == "" {
						continue
					}

					session, ok := s.getSession(notification.Context.SessionID)
					if !ok {
						continue
					}

					_ = session.send(notification.Notification)
				}
			}
		}()
	})
}

func (s *StreamableHTTPServer) handleTransport(
	w http.ResponseWriter,
	r *http.Request,
) {
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r)
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodDelete:
		s.handleDelete(w, r)
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeJSONRPCError(w, nil, mcp.INVALID_REQUEST, "Method not allowed")
	}
}

func (s *StreamableHTTPServer) handleGet(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !acceptsSSE(r.Header.Get("Accept")) {
		http.Error(w, "Client must accept text/event-stream", http.StatusNotAcceptable)
		return
	}

	sessionID := sessionIDFromHeader(r.Header)
	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	session, ok := s.getSession(sessionID)
	if !ok {
		http.Error(w, "Invalid session ID", http.StatusNotFound)
		return
	}

	if err := validateProtocolVersionHeader(r.Header, session.protocolVersion); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set(mcp.HeaderSessionID, sessionID)
	w.Header().Set(mcp.HeaderProtocolVersion, session.protocolVersion)

	streamID := uuid.NewString()
	stream := &streamableHTTPEventStream{
		writer:  w,
		flusher: flusher,
		done:    make(chan struct{}),
	}

	session.streams.Store(streamID, stream)
	defer func() {
		session.streams.Delete(streamID)
		stream.Close()
	}()

	stream.writeMu.Lock()
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	stream.writeMu.Unlock()

	select {
	case <-r.Context().Done():
	case <-session.done:
	}
}

func (s *StreamableHTTPServer) handlePost(
	w http.ResponseWriter,
	r *http.Request,
) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeJSONRPCError(w, nil, mcp.PARSE_ERROR, "Parse error")
		return
	}

	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		s.writeJSONRPCError(w, nil, mcp.INVALID_REQUEST, "Request body is empty")
		return
	}

	var base struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id,omitempty"`
		Method  string          `json:"method,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   json.RawMessage `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &base); err != nil {
		s.writeJSONRPCError(w, nil, mcp.PARSE_ERROR, "Parse error")
		return
	}

	if base.Method == "" && (len(base.Result) > 0 || len(base.Error) > 0) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	sessionID := sessionIDFromHeader(r.Header)
	var session *streamableHTTPSession
	var hasSession bool

	if base.Method == "initialize" {
		if sessionID != "" {
			s.writeJSONRPCError(w, base.ID, mcp.INVALID_PARAMS, "Initialize must not include session ID")
			return
		}
	} else {
		if sessionID == "" {
			s.writeJSONRPCError(w, base.ID, mcp.INVALID_PARAMS, "Missing session ID")
			return
		}
		session, hasSession = s.getSession(sessionID)
		if !hasSession {
			s.writeJSONRPCError(w, base.ID, mcp.INVALID_PARAMS, "Invalid session ID")
			return
		}
		if err := validateProtocolVersionHeader(r.Header, session.protocolVersion); err != nil {
			s.writeJSONRPCError(w, base.ID, mcp.INVALID_PARAMS, err.Error())
			return
		}
	}

	ctx := r.Context()
	if hasSession {
		ctx = s.server.WithContext(ctx, NotificationContext{
			ClientID:  sessionID,
			SessionID: sessionID,
		})
	}

	response := s.server.HandleMessage(ctx, json.RawMessage(body))
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if base.Method == "initialize" {
		if _, ok := response.(mcp.JSONRPCError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		session = &streamableHTTPSession{
			id:              uuid.NewString(),
			protocolVersion: protocolVersionFromResponse(response),
			done:            make(chan struct{}),
		}
		s.sessions.Store(session.id, session)
		w.Header().Set(mcp.HeaderSessionID, session.id)
	}

	if session != nil && session.protocolVersion != "" {
		w.Header().Set(mcp.HeaderProtocolVersion, session.protocolVersion)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *StreamableHTTPServer) handleDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	sessionID := sessionIDFromHeader(r.Header)
	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	session, ok := s.getSession(sessionID)
	if !ok {
		http.Error(w, "Invalid session ID", http.StatusNotFound)
		return
	}

	if err := validateProtocolVersionHeader(r.Header, session.protocolVersion); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session.Close()
	s.sessions.Delete(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *StreamableHTTPServer) getSession(
	sessionID string,
) (*streamableHTTPSession, bool) {
	sessionI, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	session, ok := sessionI.(*streamableHTTPSession)
	return session, ok
}

func (s *StreamableHTTPServer) writeJSONRPCError(
	w http.ResponseWriter,
	id interface{},
	code int,
	message string,
) {
	response := createErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(response)
}

func acceptsSSE(accept string) bool {
	return strings.Contains(accept, "text/event-stream")
}

func sessionIDFromHeader(header http.Header) string {
	sessionID := header.Get(mcp.HeaderSessionID)
	if sessionID != "" {
		return sessionID
	}
	return header.Get(mcp.LegacyHeaderSessionID)
}

func validateProtocolVersionHeader(
	header http.Header,
	sessionProtocolVersion string,
) error {
	version := header.Get(mcp.HeaderProtocolVersion)
	if version == "" {
		return nil
	}

	if !mcp.IsSupportedProtocolVersion(version) {
		return fmt.Errorf("Unsupported protocol version header: %s", version)
	}

	if sessionProtocolVersion != "" && sessionProtocolVersion != version {
		return fmt.Errorf(
			"Protocol version mismatch: expected %s, got %s",
			sessionProtocolVersion,
			version,
		)
	}

	return nil
}

func protocolVersionFromResponse(message mcp.JSONRPCMessage) string {
	response, ok := message.(mcp.JSONRPCResponse)
	if !ok {
		return mcp.LATEST_PROTOCOL_VERSION
	}

	result, ok := response.Result.(mcp.InitializeResult)
	if !ok || result.ProtocolVersion == "" {
		return mcp.LATEST_PROTOCOL_VERSION
	}

	return result.ProtocolVersion
}
