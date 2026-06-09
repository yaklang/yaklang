package mcp

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

type MCPServer struct {
	server               *server.MCPServer
	sseServer            *server.SSEServer
	streamableHTTPServer *server.StreamableHTTPServer
	httpServer           *http.Server
	grpcClient           YakClientInterface
	profileDB            *gorm.DB
	projectDB            *gorm.DB

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
	cfg.ApplyConfig(s)
	if cfg.grpcClient != nil {
		s.grpcClient = cfg.grpcClient
	}

	s.server.AddNotificationHandler("notification", s.handleNotification)
	return s, nil
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
