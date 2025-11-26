package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	mcptool "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) StartMcpServer(req *ypb.StartMcpServerRequest, stream ypb.Yak_StartMcpServerServer) error {
	if req.GetEnableAll() {
		toolSetList, err := s.GetToolSetList(stream.Context(), &ypb.Empty{})
		if err != nil {
			return err
		}
		for _, toolSet := range toolSetList.ToolSetList {
			req.Tool = append(req.Tool, toolSet.Name)
		}
		for _, resourceSet := range toolSetList.ResourceSetList {
			req.Resource = append(req.Resource, resourceSet.Name)
		}
	}
	return launchMcpServer(stream.Context(), req, stream.Send)
}

func (s *Server) GetToolSetList(ctx context.Context, req *ypb.Empty) (*ypb.GetToolSetListResponse, error) {
	toolSetList := mcp.GlobalToolSetList()
	resourceSetList := mcp.GlobalResourceSetList()
	response := &ypb.GetToolSetListResponse{
		ToolSetList:     make([]*ypb.ToolSetInfo, 0, len(toolSetList)),
		ResourceSetList: make([]*ypb.ResourceSetInfo, 0, len(resourceSetList)),
	}

	for _, toolSet := range toolSetList {
		response.ToolSetList = append(response.ToolSetList, &ypb.ToolSetInfo{
			Name: toolSet,
		})
	}

	for _, resourceSet := range resourceSetList {
		response.ResourceSetList = append(response.ResourceSetList, &ypb.ResourceSetInfo{
			Name: resourceSet,
		})
	}

	return response, nil
}

// launchMcpServer 启动 MCP 服务器的具体实现
func launchMcpServer(ctx context.Context, req *ypb.StartMcpServerRequest, send func(*ypb.StartMcpServerResponse) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic in launchMcpServer: %v", r)
			err = fmt.Errorf("panic in launchMcpServer: %v", r)
		}
	}()

	// 发送启动状态
	err = send(&ypb.StartMcpServerResponse{
		Status:  "starting",
		Message: "Initializing MCP server...",
	})
	if err != nil {
		return err
	}

	// 构建 MCP 服务器选项
	var opts []mcp.McpServerOption

	// 处理工具集配置
	if len(req.GetTool()) > 0 {
		for _, tool := range req.GetTool() {
			if tool != "" {
				opts = append(opts, mcp.WithEnableToolSet(tool))
			}
		}
	}

	if len(req.GetDisableTool()) > 0 {
		for _, tool := range req.GetDisableTool() {
			if tool != "" {
				opts = append(opts, mcp.WithDisableToolSet(tool))
			}
		}
	}

	// 处理资源集配置
	if len(req.GetResource()) > 0 {
		for _, resource := range req.GetResource() {
			if resource != "" {
				opts = append(opts, mcp.WithEnableResourceSet(resource))
			}
		}
	}

	if len(req.GetDisableResource()) > 0 {
		for _, resource := range req.GetDisableResource() {
			if resource != "" {
				opts = append(opts, mcp.WithDisableResourceSet(resource))
			}
		}
	}

	// 处理动态脚本
	if len(req.GetScript()) > 0 {
		opts = append(opts, mcp.WithDynamicScript(req.GetScript()))
	}

	if req.GetEnableYakAITool() {
		_, yakitTools, err := yakit.SearchAIYakToolWithPagination(consts.GetGormProfileDatabase(), "", false, &ypb.Paging{
			OrderBy: "updated_at",
			Order:   "desc",
			Limit:   200,
		})
		if err != nil {
			log.Errorf("failed to search yakit tools: %s", err)
		}

		tools := make([]*mcptool.Tool, 0, len(yakitTools))
		for _, aiTool := range yakitTools {
			tool := mcptool.NewTool(aiTool.Name)
			tool.Description = aiTool.Description
			dataMap := map[string]any{}
			err := json.Unmarshal([]byte(aiTool.Params), &dataMap)
			if err != nil {
				log.Errorf("unmarshal aiTool.Params failed: %v", err)
				continue
			}
			tool.InputSchema.FromMap(dataMap)
			tool.YakScript = aiTool.Content
			tools = append(tools, tool)
		}
		opts = append(opts, mcp.WithYakScriptTools(tools...))
	}

	// 创建 MCP 服务器
	mcpServer, err := mcp.NewMCPServer(opts...)
	if err != nil {
		send(&ypb.StartMcpServerResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to create MCP server: %v", err),
		})
		return err
	}

	// 发送配置完成状态
	err = send(&ypb.StartMcpServerResponse{
		Status:  "configured",
		Message: "MCP server configured successfully",
	})
	if err != nil {
		return err
	}

	// 只支持 SSE 传输协议
	host := req.GetHost()
	if host == "" {
		host = "127.0.0.1"
	}

	port := req.GetPort()
	if port == 0 {
		port = int32(utils.GetRandomAvailableTCPPort())
	}

	hostPort := utils.HostPort(host, int(port))
	urlStr := fmt.Sprintf("http://%s", hostPort)

	// 启动心跳机制
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	// 设置心跳间隔为10秒
	heartbeatInterval := 10 * time.Second

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-heartbeatCtx.Done():
				log.Debugf("Stopping MCP server heartbeat")
				return
			case t := <-ticker.C:
				// 发送心跳消息
				log.Debugf("Sending MCP server heartbeat at %v", t)
				err := send(&ypb.StartMcpServerResponse{
					Status:  "heartbeat",
					Message: fmt.Sprintf("MCP server heartbeat at %v", t.Format(time.RFC3339)),
				})
				if err != nil {
					log.Errorf("Failed to send heartbeat: %v", err)
					// 心跳发送失败，但我们不中断服务器
				}
			}
		}
	}()

	// 在 goroutine 中监听 context 取消
	go func() {
		<-ctx.Done()
		log.Infof("Context cancelled, closing MCP server")
		mcpServer.Close(ctx)
	}()

	// 阻塞运行服务器
	log.Infof("Starting MCP SSE server on: %s", urlStr)
	go func() {
		err := utils.WaitConnect(hostPort, 3)
		if err != nil {
			log.Errorf("Failed to wait for MCP server to start: %v", err)
			return
		}
		// 发送启动状态
		err = send(&ypb.StartMcpServerResponse{
			Status:    "running",
			Message:   fmt.Sprintf("MCP server started with SSE transport on %s", urlStr),
			ServerUrl: urlStr + "/sse",
		})
		if err != nil {
			log.Errorf("Failed to send running status: %v", err)
		}
	}()
	if err := mcpServer.ServeSSE(hostPort, urlStr); err != nil {
		log.Errorf("MCP SSE server error: %v", err)
		send(&ypb.StartMcpServerResponse{
			Status:  "error",
			Message: fmt.Sprintf("MCP SSE server error: %v", err),
		})
		return err
	}

	// 服务器正常停止
	send(&ypb.StartMcpServerResponse{
		Status:  "stopped",
		Message: "MCP SSE server stopped",
	})

	return nil
}
