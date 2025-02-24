package mcp

import (
	"context"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type MCPServer struct {
	server     *server.MCPServer
	sseServer  *server.SSEServer
	grpcClient ypb.YakClient
	profileDB  *gorm.DB
	projectDB  *gorm.DB

	sseMu sync.Mutex
}

func NewMCPServer() *MCPServer {
	s := &MCPServer{
		server: server.NewMCPServer(
			"Yaklang MCP Server",
			"0.0.1",
			server.WithResourceCapabilities(true, true),
			server.WithPromptCapabilities(true),
		),
	}

	s.registerYakScriptTool()
	s.registerHTTPFlowTool()
	s.registerCodecTool()
	s.registerYakDocumentTool()
	s.registerPayloadTool()
	s.registerPortScanTool()

	s.server.AddNotificationHandler("notification", s.handleNotification)
	return s
}

func (s *MCPServer) ServeSSE(addr, baseURL string) (err error) {
	s.sseMu.Lock()
	sseServer := server.NewSSEServer(s.server, baseURL)
	s.sseServer = sseServer
	s.sseMu.Unlock()

	s.grpcClient, err = yakgrpc.NewLocalClient(true)
	if err != nil {
		return err
	}
	return sseServer.Start(addr)
}

func (s *MCPServer) ServeStdio() (err error) {
	s.grpcClient, err = yakgrpc.NewLocalClient(true)
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
