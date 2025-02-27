package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var filterYakScriptToolOptions = []mcp.ToolOption{
	mcp.WithPaging("pagination",
		[]string{"id", "created_at", "updated_at", "deleted_at", "script_name", "type", "content", "level", "params", "help", "author", "tags", "ignored", "from_local", "local_path", "is_history", "force_interactive", "from_store", "is_general_module", "general_module_verbose", "general_module_key", "from_git", "is_batch_script", "is_external", "enable_plugin_selector", "plugin_selector_types", "online_id", "online_script_name", "online_contributors", "online_is_private", "user_id", "uuid", "head_img", "online_base_url", "base_online_id", "online_official", "online_group", "is_core_plugin", "risk_type", "risk_detail", "risk_annotation", "collaborator_info", "plugin_env_key"},
		mcp.Description(`Pagination settings for the query`)),
	mcp.WithString("type",
		mcp.Description("Script type"),
		mcp.Enum("yak", "codec", "mitm", "nuclei", "port-scan"),
	),
	mcp.WithString("keyword",
		mcp.Description("Keyword search in script content/name"),
	),
	mcp.WithStringArray("excludeScriptNames",
		mcp.Description("Exclude scripts with these names"),
	),
	mcp.WithStringArray("includedScriptNames",
		mcp.Description("Specifically include these script names"),
	),
	mcp.WithStringArray("tag",
		mcp.Description("Filter by script tags"),
	),
	mcp.WithStruct("group",
		[]mcp.PropertyOption{mcp.Description("Filter scripts by group settings")},
		mcp.WithBool("UnSetGroup",
			mcp.Description("if true, filter scripts without these groups")),
		mcp.WithStringArray("Group",
			mcp.Description("Group names to filter")),
	),
	mcp.WithString("userName",
		mcp.Description("Filter scripts by author username"),
	),
	mcp.WithStringArray("excludeTypes",
		mcp.Description("Exclude these script types"),
	),
}

var filterOnlinePluginToolOptions = []mcp.ToolOption{
	mcp.WithString("keywords",
		mcp.Description("Keywords to search for in plugins, search in name, description, tags and content"),
	),
	mcp.WithStringArray("scriptName"),
	mcp.WithStringArray("pluginType",
		mcp.Description("Script type"),
		mcp.Enum("yak", "codec", "mitm", "nuclei", "port-scan"),
	),
	mcp.WithStringArray("tags",
		mcp.Description("Tags associated with the scripts"),
	),
	mcp.WithString("userName",
		mcp.Description("Username of the script creator"),
	),
	mcp.WithStringArray("excludeTypes",
		mcp.Description("Script types to exclude from the query"),
		mcp.Enum("yak", "codec", "mitm", "nuclei", "port-scan"),
	),
	mcp.WithString("group",
		mcp.Description("Script Group"),
	),
	mcp.WithString("uuid",
		mcp.Description("UUID of the script"),
	),
}

func init() {
	AddGlobalToolSet("yak_script",
		WithTool(mcp.NewTool("static_analyze_yak_script",
			mcp.WithDescription("Static analysis yak script for syntax error and other issues"),
			mcp.WithString("code",
				mcp.Description("The yak script content to be analyzed"),
				mcp.Required(),
			),
			mcp.WithString("pluginType",
				mcp.Description("The type of the yak script"),
				mcp.Required(),
				mcp.Enum("yak", "mitm", "port_scan", "codec", "syntaxflow"),
			),
		), handleStaticAnalyzeYakScript),

		WithTool(mcp.NewTool("query_yak_script",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query Yak scripts with flexible filters"),
			}, filterYakScriptToolOptions...)...,
		), handleQueryYakScript),

		WithTool(mcp.NewTool("exec_yak_script",
			mcp.WithDescription("execute yak script by raw code or yak script name"),
			mcp.WithString("pluginName",
				mcp.Description("Name of the yak script, query_yak_script.result.script_name (if not provided, uses existing code)"),
			),
			mcp.WithString("code",
				mcp.Description("Yak script content"),
			),
			mcp.WithString("pluginType",
				mcp.Description("Type of the yak script, same with query_yak_script.result.type"),
				mcp.Enum("yak", "codec", "mitm", "nuclei", "port-scan"),
				mcp.Required(),
			),
			mcp.WithKVPairs("execParams",
				mcp.Description(`Parameters for the yak script, check script content for the required parameters.Please check the use of all cli libraries, for example: cli.Int("a") means that there is a parameter with key "a" and type int`)),
		), handleExecYakScript),

		WithTool(mcp.NewTool("create_yak_script_group",
			mcp.WithDescription("Create a new Yak script group"),
			mcp.WithString("GroupName",
				mcp.Description("Group name"),
			),
		), handleCreateYakScriptGroup),

		WithTool(mcp.NewTool("list_yak_script_group",
			mcp.WithDescription("List Yak script group information"),
			mcp.WithBool("All",
				mcp.Description("Fetch all groups"),
				mcp.Default(false),
			),
			mcp.WithString("PageId",
				mcp.Description("Page identifier for pagination"),
			),
			mcp.WithBool("IsPocBuiltIn",
				mcp.Description("Filter built-in POC groups"),
				mcp.Default(false),
			),
			mcp.WithStringArray("ExcludeType",
				mcp.Description("Exclude specific script types"),
			),
			mcp.WithNumber("IsMITMParamPlugins",
				mcp.Description(`Filter MITM parameter plugins:
0 - No filter
1 - Only plugins with MITM parameters
2 - Plugins without MITM params OR port-scan type`),
				mcp.Default(0),
				mcp.Enum(0, 1, 2),
			),
		), handleListYakScriptGroup),

		WithTool(mcp.NewTool("query_yak_script_group",
			append([]mcp.ToolOption{
				mcp.WithDescription("Query group names by filtered yak scripts"),
			}, filterYakScriptToolOptions...)...,
		), handleQueryYakScriptGroup),

		WithTool(mcp.NewTool("set_group_for_yak_script",
			mcp.WithDescription("Add/Remove groups for yak script with filtering and group operations"),
			mcp.WithStruct("filter",
				[]mcp.PropertyOption{
					mcp.Description("Filter that same with query_yak_script arguments"),
				},
				filterYakScriptToolOptions...,
			),
			mcp.WithStringArray("saveGroup",
				mcp.Description("Groups to add the filtered scripts to"),
				mcp.MinLength(1),
				mcp.Required(),
			),
			mcp.WithStringArray("removeGroup",
				mcp.Description("Groups to remove the filtered scripts from"),
				mcp.MinLength(1),
			),
		), handleSetGroupForYakScript),

		WithTool(mcp.NewTool("rename_yak_script_group",
			mcp.WithDescription("Rename a Yak script group"),
			mcp.WithString("group",
				mcp.Description("Old group name"),
				mcp.Required(),
			),
			mcp.WithString("newGroup",
				mcp.Description("New group name"),
				mcp.Required(),
			),
		), handleRenameYakScriptGroup),

		WithTool(mcp.NewTool("delete_yak_script_group",
			mcp.WithDescription("Delete a Yak script group, Please let the user confirm again and again"),
			mcp.WithString("group",
				mcp.Description("group name"),
				mcp.Required(),
			),
		), handleDeleteYakScriptGroup),

		// online
		WithTool(mcp.NewTool("query_online_yak_script",
			mcp.WithDescription("Queries online yak scripts based on the provided filters"),
			mcp.WithPaging("pagination",
				[]string{"created_at", "updated_at", "id"},
				mcp.Description("Pagination settings for the query"),
			),
			mcp.WithStruct("data",
				[]mcp.PropertyOption{
					mcp.Description("Filters"),
					mcp.Required(),
				},
				filterOnlinePluginToolOptions...,
			),
		), handleQueryOnlinePlugins),

		WithTool(mcp.NewTool("download_online_yak_script",
			append([]mcp.ToolOption{
				mcp.WithDescription("Download online yak scripts to local based on the provided filters"),
			}, filterOnlinePluginToolOptions...)...,
		), handleDownloadOnlinePlugins),
	)
}

func handleStaticAnalyzeYakScript(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		req := ypb.StaticAnalyzeErrorRequest{
			Code:       []byte(utils.MapGetString(args, "code")),
			PluginType: utils.MapGetString(args, "pluginType"),
		}

		rsp, err := s.grpcClient.StaticAnalyzeError(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to static analyze yak script")
		}
		return NewCommonCallToolResult((rsp.Result))
	}

}

func handleExecYakScript(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.DebugPluginRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}

		stream, err := s.grpcClient.DebugPlugin(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query yak script")
		}
		results := make([]any, 0, 4)
		for {
			exec, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}
			if !exec.IsMessage {
				continue
			}

			content := string(exec.Message)
			content = handleExecMessage(content)
			if content == "" {
				continue
			}
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: content,
			})
			s.server.SendNotificationToClient("exec_yak_script/info", map[string]any{
				"content":       content,
				"progressToken": progressToken,
			})
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] Script execution completed with no output",
			})
		}

		return NewCommonCallToolResult(results)
	}
}

func handleQueryYakScript(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryYakScriptRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.QueryYakScript(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query yak script")
		}
		return NewCommonCallToolResult((rsp.Data))
	}
}

func handleCreateYakScriptGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.SetGroupRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.SetGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to create yak script group")
		}
		return NewCommonCallToolResult(("create success"))
	}
}

func handleListYakScriptGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryYakScriptGroupRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.QueryYakScriptGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query yak script group info")
		}
		return NewCommonCallToolResult((rsp.Group))
	}
}

func handleQueryYakScriptGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryYakScriptRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.GetYakScriptGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query yak script group info")
		}
		return NewCommonCallToolResult((rsp))
	}
}

func handleSetGroupForYakScript(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.SaveYakScriptGroupRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.SaveYakScriptGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to add/remove group for yak script")
		}
		return NewCommonCallToolResult(("set group success"))
	}
}

func handleRenameYakScriptGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.RenameYakScriptGroupRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.RenameYakScriptGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to rename yak script group")
		}
		return NewCommonCallToolResult(("rename group success"))
	}
}

func handleDeleteYakScriptGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.DeleteYakScriptGroupRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		_, err = s.grpcClient.DeleteYakScriptGroup(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to delete yak script group")
		}
		return NewCommonCallToolResult(("delete group success"))
	}
}

func handleQueryOnlinePlugins(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.QueryOnlinePluginsRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		rsp, err := s.grpcClient.QueryOnlinePlugins(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to query online plugins")
		}
		return NewCommonCallToolResult((rsp.Data))
	}
}

func handleDownloadOnlinePlugins(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.DownloadOnlinePluginsRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		var progressToken mcp.ProgressToken
		meta := request.Params.Meta
		if meta != nil {
			progressToken = meta.ProgressToken
		}

		stream, err := s.grpcClient.DownloadOnlinePlugins(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to download online plugins")
		}
		results := make([]any, 0, 4)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					results = append(results, mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("[Error] %v", err),
					})
				}
				break
			}
			if msg.Log != "" {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: msg.Log,
				})
				s.server.SendNotificationToClient("download_online_plugins/info", map[string]any{
					"content":       msg.Log,
					"progressToken": progressToken,
					"progress":      msg.Progress,
				})
			} else if msg.Progress > 0 {
				s.server.SendNotificationToClient("download_online_plugins/progress", map[string]any{
					"progressToken": progressToken,
					"progress":      msg.Progress,
				})
			}
		}
		if len(results) == 0 {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: "[System] Download online plugins completed with no output",
			})
		}

		return NewCommonCallToolResult((results))
	}
}
