package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mitchellh/mapstructure"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *MCPServer) registerYakScriptTool() {
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
	), s.handleQueryYakScriptTool)

	s.server.AddTool(mcp.NewTool("exec_yak_script",
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
	), s.handleExecYakScriptTool)

	s.server.AddTool(mcp.NewTool("create_yak_script_group",
		mcp.WithDescription("Create a new Yak script group"),
		mcp.WithString("GroupName",
			mcp.Description("Group name"),
		),
	), s.handleCreateYakScriptGroup)

	s.server.AddTool(mcp.NewTool("query_yak_script_group",
		mcp.WithDescription("Query Yak script group info"),
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
	), s.handleQueryYakScriptGroup)
}

func (s *MCPServer) handleExecYakScriptTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var req ypb.DebugPluginRequest
	err := mapstructure.Decode(request.Params.Arguments, &req)
	if err != nil {
		return nil, utils.Wrap(err, "invalid argument")
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
					Text: fmt.Sprintf("error: %v", err),
				})
			}
			break
		}
		if !exec.IsMessage {
			continue
		}

		content := string(exec.Message)
		// handle complex message
		msgContent := gjson.GetBytes(exec.Message, "content")
		level := msgContent.Get("level").String()
		switch level {
		case "feature-status-card-data":
			data := msgContent.Get("data").String()
			// ignore empty risk message
			if gjson.Get(data, "id").String() == "漏洞/风险/指纹" {
				cardCount := int(gjson.Get(data, "data").Int())
				if cardCount == 0 {
					continue
				}
			}
		case "info", "json":
			// use content directly
			content = msgContent.Get("data").String()
		}
		results = append(results, mcp.TextContent{
			Type: "text",
			Text: content,
		})
	}

	return &mcp.CallToolResult{
		Content: results,
	}, nil
}

func (s *MCPServer) handleQueryYakScriptTool(
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
	return NewCommonCallToolResult(rsp.Data)
}

func (s *MCPServer) handleCreateYakScriptGroup(
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
		return nil, utils.Wrap(err, "failed to query yak script group info")
	}
	return NewCommonCallToolResult("create success")
}

func (s *MCPServer) handleQueryYakScriptGroup(
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
	return NewCommonCallToolResult(rsp.Group)
}
