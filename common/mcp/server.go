package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
)

type MCPServer struct {
	server               *server.MCPServer
	sseServer            *server.SSEServer
	streamableHTTPServer *server.StreamableHTTPServer
	httpServer           *http.Server
	grpcClient           YakClientInterface
	profileDB            *gorm.DB
	projectDB            *gorm.DB
	profileDBProvider    func() *gorm.DB
	projectDBProvider    func() *gorm.DB

	sseMu sync.Mutex

	bridgeClientClosers []io.Closer
}

func NewMCPServer(opts ...McpServerOption) (*MCPServer, error) {
	s := &MCPServer{
		server: server.NewMCPServer(
			"Yaklang MCP Server",
			"0.0.2",
			server.WithResourceCapabilities(true, true),
			server.WithPromptCapabilities(true),
		),
	}
	// tools and resources
	cfg := NewMCPServerConfig()
	for _, opt := range opts {
		err := opt(cfg)
		if err != nil {
			return nil, err
		}
	}
	s.bridgeClientClosers = cfg.bridgeClientClosers
	s.profileDB = cfg.profileDB
	s.projectDB = cfg.projectDB
	s.profileDBProvider = cfg.profileDBProvider
	s.projectDBProvider = cfg.projectDBProvider
	s.server.SetToolCallObserver(s.recordToolCall)
	cfg.ApplyConfig(s)
	if cfg.grpcClient != nil {
		s.grpcClient = cfg.grpcClient
	}

	s.server.AddNotificationHandler("notification", s.handleNotification)
	return s, nil
}

func (s *MCPServer) getProfileDatabase() *gorm.DB {
	if s.profileDBProvider != nil {
		return s.profileDBProvider()
	}
	return s.profileDB
}

// getProjectDatabase resolves the project database at call time. Yakit can
// switch projects while an MCP server is still running, so tool handlers must
// not retain the handle that was current when the MCP server started.
func (s *MCPServer) getProjectDatabase() *gorm.DB {
	if s == nil {
		return nil
	}
	if s.projectDBProvider != nil {
		return s.projectDBProvider()
	}
	return s.projectDB
}

func marshalMCPHistoryValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "null", err
	}
	return string(data), nil
}

func mcpToolResultErrorMessage(result *mcp.CallToolResult) string {
	if result == nil {
		return "MCP tool returned an empty error result"
	}
	for _, content := range result.Content {
		if textContent, ok := mcp.AsTextContent(content); ok && textContent.Text != "" {
			return textContent.Text
		}
		if value, ok := content.(map[string]any); ok {
			if text, ok := value["text"].(string); ok && text != "" {
				return text
			}
		}
	}
	return "MCP tool returned an error result"
}

func (s *MCPServer) recordToolCall(
	ctx context.Context,
	request mcp.CallToolRequest,
	result *mcp.CallToolResult,
	callErr error,
	duration time.Duration,
) {
	db := s.getProfileDatabase()
	if db == nil {
		log.Warn("skip mcp tool call history: profile database is unavailable")
		return
	}

	arguments, argumentsErr := marshalMCPHistoryValue(request.Params.Arguments)
	if argumentsErr != nil {
		log.Errorf("serialize mcp tool call arguments failed: %v", argumentsErr)
	}
	resultJSON, resultErr := marshalMCPHistoryValue(result)
	if resultErr != nil {
		log.Errorf("serialize mcp tool call result failed: %v", resultErr)
	}

	clientContext := server.ServerFromContext(ctx)
	history := &schema.MCPToolCallHistory{
		ToolName:       request.Params.Name,
		Arguments:      arguments,
		Result:         resultJSON,
		Success:        callErr == nil && result != nil && !result.IsError,
		DurationMillis: duration.Milliseconds(),
	}
	if callErr != nil {
		history.ErrorMessage = callErr.Error()
	} else if result != nil && result.IsError {
		history.ErrorMessage = mcpToolResultErrorMessage(result)
	} else if result == nil {
		history.ErrorMessage = "MCP tool returned an empty result"
	}
	if clientContext != nil {
		notificationContext := clientContext.CurrentClientContext()
		history.ClientID = notificationContext.ClientID
		history.SessionID = notificationContext.SessionID
		history.ClientName = notificationContext.ClientName
		history.ClientVersion = notificationContext.ClientVersion
	}
	if err := db.Create(history).Error; err != nil {
		log.Errorf("record mcp tool call history failed: %v", err)
	}
}

func (s *MCPServer) ServeSSE(addr, baseURL string) (err error) {
	s.sseMu.Lock()
	sseServer := server.NewSSEServer(s.server, baseURL)
	s.sseServer = sseServer
	s.sseMu.Unlock()

	if err = s.ensureLocalClient(); err != nil {
		return err
	}
	return sseServer.Start(addr)
}

func (s *MCPServer) ServeStreamableHTTP(addr, baseURL string) (err error) {
	s.sseMu.Lock()
	streamableHTTPServer := server.NewStreamableHTTPServer(s.server, baseURL)
	s.streamableHTTPServer = streamableHTTPServer
	s.sseMu.Unlock()

	if err = s.ensureLocalClient(); err != nil {
		return err
	}
	return streamableHTTPServer.Start(addr)
}

func (s *MCPServer) ServeHTTPCompat(addr, baseURL string) (err error) {
	s.sseMu.Lock()
	sseServer := server.NewSSEServer(s.server, baseURL)
	streamableHTTPServer := server.NewStreamableHTTPServer(s.server, baseURL)
	mux := http.NewServeMux()
	sseServer.RegisterHandlers(mux)
	streamableHTTPServer.RegisterHandlers(mux)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	s.sseServer = sseServer
	s.streamableHTTPServer = streamableHTTPServer
	s.httpServer = httpServer
	s.sseMu.Unlock()

	if err = s.ensureLocalClient(); err != nil {
		return err
	}
	return httpServer.ListenAndServe()
}

func (s *MCPServer) ServeStdio() (err error) {
	if err = s.ensureLocalClient(); err != nil {
		return err
	}
	return server.ServeStdio(s.server)
}

func (s *MCPServer) closeBridgeClients() {
	for _, closer := range s.bridgeClientClosers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil {
			log.Warnf("close bridge mcp client failed: %v", err)
		}
	}
	s.bridgeClientClosers = nil
}

func (s *MCPServer) Close(ctxs ...context.Context) {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()

	ctx := context.Background()
	if len(ctxs) > 0 {
		ctx = ctxs[0]
	}
	if s.sseServer != nil {
		s.sseServer.Shutdown(ctx)
		s.sseServer = nil
	}
	if s.streamableHTTPServer != nil {
		s.streamableHTTPServer.Shutdown(ctx)
		s.streamableHTTPServer = nil
	}
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
		s.httpServer = nil
	}
	s.closeBridgeClients()
}

func (s *MCPServer) handleNotification(
	ctx context.Context,
	notification mcp.JSONRPCNotification,
) {
	// TODO
}

func (s *MCPServer) notificationServer(ctx context.Context) *server.MCPServer {
	if scoped := server.ServerFromContext(ctx); scoped != nil {
		return scoped
	}
	return s.server
}

func (s *MCPServer) ensureLocalClient() error {
	if s.grpcClient != nil {
		return nil
	}
	client, err := NewLocalClient(true)
	if err != nil {
		return err
	}
	s.grpcClient = client
	return nil
}

// BindLocalGRPCClient wires the in-process yak gRPC client for legacy tool handlers.
func (s *MCPServer) BindLocalGRPCClient() error {
	return s.ensureLocalClient()
}
