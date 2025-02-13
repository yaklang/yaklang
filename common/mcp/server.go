package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type MCPServer struct {
	server     *server.MCPServer
	grpcClient ypb.YakClient
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

	s.server.AddTool(mcp.NewTool("query_yak_script",
		mcp.WithDescription("Query Yak scripts with flexible filters"),
		mcp.WithPaging("pagination",
			mcp.Description(`Pagination settings for the query, field: id,created_at,updated_at,deleted_at,script_name,type,content,level,params,help,author,tags,ignored,from_local,local_path,is_history,force_interactive,from_store,is_general_module,general_module_verbose,general_module_key,from_git,is_batch_script,is_external,enable_plugin_selector,plugin_selector_types,online_id,online_script_name,online_contributors,online_is_private,user_id,uuid,head_img,online_base_url,base_online_id,online_official,online_group,is_core_plugin,risk_type,risk_detail,risk_annotation,collaborator_info,plugin_env_key`)),
		mcp.WithString("type",
			mcp.Description("Script type filter"),
		),
		mcp.WithString("keyword",
			mcp.Description("Keyword search in script content/name"),
		),
		mcp.WithStringArray("exclude_script_names",
			mcp.Description("Exclude scripts with these names"),
		),
		mcp.WithStringArray("included_script_names",
			mcp.Description("Specifically include these script names"),
		),
		mcp.WithStringArray("tag",
			mcp.Description("Filter by script tags"),
		),
		mcp.WithStringArray("group",
			mcp.Description("Filter by script groups"),
		),
		mcp.WithString("user_name",
			mcp.Description("Filter scripts by author username"),
		),
		mcp.WithStringArray("exclude_types",
			mcp.Description("Exclude these script types"),
		),
	), s.handleQueryYakScriptTool)

	s.server.AddNotificationHandler("notification", s.handleNotification)
	return s
}

func (s *MCPServer) ServeSSE(addr, baseURL string) (err error) {
	sseServer := server.NewSSEServer(s.server, baseURL)
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

func (s *MCPServer) handleNotification(
	ctx context.Context,
	notification mcp.JSONRPCNotification,
) {
	// TODO
}
