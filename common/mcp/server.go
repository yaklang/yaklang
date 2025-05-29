package mcp

import (
	"context"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

type MCPServer struct {
	server     *server.MCPServer
	sseServer  *server.SSEServer
	grpcClient YakClientInterface
	profileDB  *gorm.DB
	projectDB  *gorm.DB

	sseMu sync.Mutex
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
	cfg.ApplyConfig(s)

	s.server.AddNotificationHandler("notification", s.handleNotification)
	return s, nil
}

func (s *MCPServer) ServeSSE(addr, baseURL string) (err error) {
	s.sseMu.Lock()
	sseServer := server.NewSSEServer(s.server, baseURL)
	s.sseServer = sseServer
	s.sseMu.Unlock()

	s.grpcClient, err = NewLocalClient(true)
	if err != nil {
		return err
	}
	return sseServer.Start(addr)
}

func (s *MCPServer) ServeStdio() (err error) {
	s.grpcClient, err = NewLocalClient(true)
	if err != nil {
		return err
	}
	return server.ServeStdio(s.server)
}

func (s *MCPServer) Close(ctxs ...context.Context) {
	if s.sseServer == nil {
		return
	}

	s.sseMu.Lock()
	defer s.sseMu.Unlock()

	ctx := context.Background()
	if len(ctxs) > 0 {
		ctx = ctxs[0]
	}
	s.sseServer.Shutdown(ctx)
	s.sseServer = nil
}

func (s *MCPServer) handleNotification(
	ctx context.Context,
	notification mcp.JSONRPCNotification,
) {
	// TODO
}
