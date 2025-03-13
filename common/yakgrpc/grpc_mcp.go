package yakgrpc

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	mcp.SetNewLocalClient(NewLocalClient)
}

func (s *Server) StartSSEMCP(req *ypb.StartMCPRequest, stream ypb.Yak_StartSSEMCPServer) error {
	// 创建一个新的gRPC服务来启动MCP，而不是直接调用mcp包
	// 这样可以避免导入循环
	hostPort := fmt.Sprintf("localhost:%d", utils.GetRandomAvailableTCPPort())
	url := fmt.Sprintf("http://%s", hostPort)
	opts := make([]mcp.McpServerOption, 0, len(req.GetTools())+len(req.GetResources())+len(req.GetDisableTools())+len(req.GetDisableResources()))
	for _, toolSet := range req.GetTools() {
		opts = append(opts, mcp.WithEnableToolSet(toolSet))
	}
	for _, toolSet := range req.GetDisableTools() {
		opts = append(opts, mcp.WithDisableToolSet(toolSet))
	}
	for _, resourceSet := range req.GetResources() {
		opts = append(opts, mcp.WithEnableResourceSet(resourceSet))
	}
	for _, resourceSet := range req.GetDisableResources() {
		opts = append(opts, mcp.WithDisableResourceSet(resourceSet))
	}

	mcpServer, err := mcp.NewMCPServer(opts...)
	if err != nil {
		return err
	}
	go func() {
		err = mcpServer.ServeSSE(hostPort, url)
		if err != nil {
			log.Errorf("start sse mcp server failed: %v", err)
		}
	}()

	defer mcpServer.Close()

	stream.Send(&ypb.StartMCPResponse{
		URL: url,
	})

	for {
		select {
		case <-stream.Context().Done():
			return nil
		}
	}
}
