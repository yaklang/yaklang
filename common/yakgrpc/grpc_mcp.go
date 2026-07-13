package yakgrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) StartMcpServer(req *ypb.StartMcpServerRequest, stream ypb.Yak_StartMcpServerServer) error {
	db := s.GetProfileDatabase()
	globalCfg, err := yakit.GetMCPGlobalConfig(db)
	if err != nil {
		return err
	}

	explicitToolSets := false
	useGlobalStartupDefaults := false
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
		log.Infof("StartMcpServer: EnableAll=true, exposing %d tool sets", len(req.GetTool()))
	} else if len(req.GetTool()) == 0 {
		useGlobalStartupDefaults = true
		req.Tool = append(req.Tool, globalCfg.GetDefaultToolSets()...)
		if len(req.GetResource()) == 0 {
			req.Resource = append(req.Resource, globalCfg.GetDefaultResourceSets()...)
		}
		log.Infof("StartMcpServer: using configured default tool sets (%d sets, ~%d tools)", len(req.GetTool()), mcp.MCPToolCountForSets(req.GetTool()))
	} else {
		explicitToolSets = true
		log.Infof("StartMcpServer: explicit tool sets: %v", req.GetTool())
	}

	if useGlobalStartupDefaults {
		if globalCfg.GetEnableAIToolFramework() {
			req.EnableAIToolFramework = true
		}
		if globalCfg.GetEnableBridgeExternalMCP() {
			req.EnableBridgeExternalMCP = true
		}
	}

	return launchMcpServer(stream.Context(), req, stream.Send, explicitToolSets)
}

func (s *Server) GetToolSetList(ctx context.Context, req *ypb.Empty) (*ypb.GetToolSetListResponse, error) {
	defaultSets, err := yakit.EffectiveDefaultMCPToolSetMap(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	defaultResources, err := yakit.EffectiveDefaultMCPResourceSets(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	defaultResourceSet := make(map[string]struct{}, len(defaultResources))
	for _, name := range defaultResources {
		defaultResourceSet[name] = struct{}{}
	}

	response := &ypb.GetToolSetListResponse{
		ToolSetList:     make([]*ypb.ToolSetInfo, 0, len(mcp.AllMCPToolSetNames())),
		ResourceSetList: make([]*ypb.ResourceSetInfo, 0, len(mcp.GlobalResourceSetList())),
	}

	for _, toolSet := range mcp.AllMCPToolSetNames() {
		entry, ok := mcp.CatalogEntryByName(toolSet)
		info := &ypb.ToolSetInfo{
			Name:      toolSet,
			ToolCount: int32(len(mcp.ToolNamesInSet(toolSet))),
		}
		if ok {
			info.Summary = entry.Summary
			info.Tier = mcp.TierName(entry.Tier)
		}
		_, info.EnabledByDefault = defaultSets[toolSet]
		response.ToolSetList = append(response.ToolSetList, info)
	}

	for _, resourceSet := range mcp.GlobalResourceSetList() {
		_, enabled := defaultResourceSet[resourceSet]
		response.ResourceSetList = append(response.ResourceSetList, &ypb.ResourceSetInfo{
			Name:              resourceSet,
			EnabledByDefault: enabled,
		})
	}

	return response, nil
}

// launchMcpServer 启动 MCP 服务器的具体实现
func launchMcpServer(ctx context.Context, req *ypb.StartMcpServerRequest, send func(*ypb.StartMcpServerResponse) error, explicitToolSets bool) (err error) {
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

	if req.GetEnableAIToolFramework() {
		db := consts.GetGormProfileDatabase()

		// Built-in framework tools: fs, ssa, yakscript, etc.
		builtinTools := buildinaitools.GetAllToolsDynamically(db)
		if len(builtinTools) > 0 {
			opts = append(opts, mcp.WithAITools(builtinTools...))
			log.Infof("launchMcpServer: loaded %d built-in aitool-framework tools", len(builtinTools))
		}
	}

	if req.GetEnableBridgeExternalMCP() {
		db := consts.GetGormProfileDatabase()
		externalTools, mcpErr := aitool.LoadAllEnabledAIToolsFromMCPServers(db, ctx)
		if mcpErr != nil {
			log.Warnf("launchMcpServer: load external mcp tools via bridge failed: %v", mcpErr)
		} else if len(externalTools) > 0 {
			opts = append(opts, mcp.WithAITools(externalTools...))
			log.Infof("launchMcpServer: loaded %d external mcp tools via bridge", len(externalTools))
		}
	}

	// Apply per-tool enable/disable from the profile DB.
	// Tools that were explicitly disabled by the user are filtered out here.
	disabledTools, dbErr := GetDisabledMCPToolNamesFromDB()
	if dbErr != nil {
		log.Warnf("launchMcpServer: failed to load disabled tool list: %v", dbErr)
	}
	// Explicit -t/Tool requests intentionally enable optional sets; do not apply
	// tier-default disable flags for tools in those sets.
	if explicitToolSets {
		for _, setName := range req.GetTool() {
			for _, toolName := range mcp.ToolNamesInSet(setName) {
				delete(disabledTools, toolName)
			}
		}
	}
	if len(disabledTools) > 0 {
		opts = append(opts, mcp.WithDisabledToolNames(disabledTools))
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

	// 默认同时提供 legacy SSE 与 streamable HTTP 传输协议
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
	log.Infof("Starting MCP HTTP server on: %s", urlStr)
	go func() {
		err := utils.WaitConnect(hostPort, 3)
		if err != nil {
			log.Errorf("Failed to wait for MCP server to start: %v", err)
			return
		}
		// 发送启动状态
		err = send(&ypb.StartMcpServerResponse{
			Status:            "running",
			Message:           fmt.Sprintf("MCP server started with SSE transport on %s/sse and Streamable HTTP transport on %s/mcp", urlStr, urlStr),
			ServerUrl:         urlStr + "/sse",
			StreamableHttpUrl: urlStr + "/mcp",
		})
		if err != nil {
			log.Errorf("Failed to send running status: %v", err)
		}
	}()
	if err := mcpServer.ServeHTTPCompat(hostPort, urlStr); err != nil {
		log.Errorf("MCP HTTP server error: %v", err)
		send(&ypb.StartMcpServerResponse{
			Status:  "error",
			Message: fmt.Sprintf("MCP HTTP server error: %v", err),
		})
		return err
	}

	// 服务器正常停止
	send(&ypb.StartMcpServerResponse{
		Status:  "stopped",
		Message: "MCP HTTP server stopped",
	})

	return nil
}
