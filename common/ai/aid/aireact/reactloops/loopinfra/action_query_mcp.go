package loopinfra

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	defaultMCPServerQueryLimit = 20
	defaultMCPToolQueryLimit   = 20
)

var loopAction_QueryMCPServers = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_QUERY_MCP_SERVERS,
	Description: "List enabled MCP servers configured in Yakit. Use this to discover available MCP server names " +
		"before querying tools. Optional keyword filters by server name, type, url, or command. " +
		"Results are paginated via optional `offset` and `limit` (default limit=20). " +
		"When truncated=true in the response, fetch the next page before claiming you listed all servers.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"keyword",
			aitool.WithParam_Description("Optional keyword to filter enabled MCP servers."),
		),
		aitool.WithIntegerParam(
			"offset",
			aitool.WithParam_Description("Optional zero-based index of the first server to return. Default 0."),
		),
		aitool.WithIntegerParam(
			"limit",
			aitool.WithParam_Description(fmt.Sprintf("Optional page size. Default %d.", defaultMCPServerQueryLimit)),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "keyword", AINodeId: "query_mcp_servers"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		if !reactloops.IsMCPServersAllowed(loop.GetInvoker()) {
			return utils.Error("MCP servers are disabled for this runtime")
		}
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		if !reactloops.IsMCPServersAllowed(invoker) {
			operator.Feedback("MCP servers are disabled for this runtime.")
			operator.Continue()
			return
		}

		keyword := strings.TrimSpace(action.GetString("keyword"))
		offset, limit := parseMCPQueryOffsetLimit(action, defaultMCPServerQueryLimit)
		feedback, pageCount, total, _, err := renderMCPServerQueryResult(keyword, offset, limit)
		if err != nil {
			log.Warnf("query_mcp_servers: query failed: %v", err)
			operator.Feedback(fmt.Sprintf("Failed to query MCP servers: %v", err))
			operator.Continue()
			return
		}
		if invoker != nil {
			invoker.AddToTimeline("query_mcp_servers", fmt.Sprintf("已查询 MCP 服务器 %d/%d (offset=%d)", pageCount, total, offset))
		}
		operator.Feedback(feedback)
		operator.Continue()
	},
}

var loopAction_QueryMCPTools = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_QUERY_MCP_TOOLS,
	Description: "List enabled MCP tools for a specific MCP server from cached metadata. " +
		"Use after `query_mcp_servers` when you already know the server name. " +
		"Results are paginated via optional `offset` and `limit` (default limit=20). " +
		"When truncated=true in the response, fetch the next page before claiming you listed all tools. " +
		"For keyword-based discovery across all tools/forges/skills/MCP metadata, use `search_capabilities` instead. " +
		"Call tools via require_tool using the full name mcp_{server}_{tool}.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"server_name",
			aitool.WithParam_Description("Required MCP server name whose enabled tools should be listed."),
			aitool.WithParam_Required(true),
		),
		aitool.WithIntegerParam(
			"offset",
			aitool.WithParam_Description("Optional zero-based index of the first tool to return. Default 0."),
		),
		aitool.WithIntegerParam(
			"limit",
			aitool.WithParam_Description(fmt.Sprintf("Optional page size. Default %d.", defaultMCPToolQueryLimit)),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "server_name", AINodeId: "query_mcp_tools"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		if !reactloops.IsMCPServersAllowed(loop.GetInvoker()) {
			return utils.Error("MCP servers are disabled for this runtime")
		}
		serverName := strings.TrimSpace(action.GetString("server_name"))
		if serverName == "" {
			return utils.Error("query_mcp_tools requires server_name")
		}
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		if !reactloops.IsMCPServersAllowed(invoker) {
			operator.Feedback("MCP servers are disabled for this runtime.")
			operator.Continue()
			return
		}

		serverName := strings.TrimSpace(action.GetString("server_name"))
		offset, limit := parseMCPQueryOffsetLimit(action, defaultMCPToolQueryLimit)
		feedback, pageCount, total, _, err := renderMCPToolQueryResult(serverName, offset, limit)
		if err != nil {
			log.Warnf("query_mcp_tools: query failed: %v", err)
			operator.Feedback(fmt.Sprintf("Failed to query MCP tools: %v", err))
			operator.Continue()
			return
		}
		if invoker != nil {
			invoker.AddToTimeline("query_mcp_tools", fmt.Sprintf("已查询 MCP 工具 %d/%d (offset=%d)", pageCount, total, offset))
		}
		operator.Feedback(feedback)
		operator.Continue()
	},
}

func parseMCPQueryOffsetLimit(action *aicommon.Action, defaultLimit int) (offset, limit int) {
	if action == nil {
		return 0, defaultLimit
	}
	offset = action.GetInt("offset")
	if offset < 0 {
		offset = 0
	}
	limit = action.GetInt("limit")
	if limit <= 0 {
		limit = defaultLimit
	}
	return offset, limit
}

func renderMCPServerQueryResult(keyword string, offset, limit int) (feedback string, pageCount int, total int, truncated bool, err error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "Database not available; cannot query MCP servers.", 0, 0, false, nil
	}

	servers, err := listAllEnabledMCPServers(db, keyword)
	if err != nil {
		return "", 0, 0, false, err
	}
	return buildMCPServerQueryFeedback(keyword, servers, offset, limit)
}

func listAllEnabledMCPServers(db *gorm.DB, keyword string) ([]*schema.MCPServer, error) {
	const pageSize = 100
	var all []*schema.MCPServer
	for page := 1; ; page++ {
		_, batch, err := yakit.QueryMCPServers(db, &ypb.GetAllMCPServersRequest{
			IsEnable: true,
			Keyword:  keyword,
			Pagination: &ypb.Paging{
				Page:    int64(page),
				Limit:   pageSize,
				OrderBy: "name",
				Order:   "asc",
			},
		})
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < pageSize {
			break
		}
	}
	return all, nil
}

func buildMCPServerQueryFeedback(keyword string, servers []*schema.MCPServer, offset, limit int) (feedback string, pageCount int, total int, truncated bool, err error) {
	if limit <= 0 {
		limit = defaultMCPServerQueryLimit
	}
	if offset < 0 {
		offset = 0
	}

	total = len(servers)
	page, truncated := paginateMCPServers(servers, offset, limit)
	pageCount = len(page)

	var sb strings.Builder
	sb.WriteString("### Enabled MCP Servers\n\n")
	if keyword != "" {
		sb.WriteString(fmt.Sprintf("Keyword filter: %q\n\n", keyword))
	}
	if total == 0 {
		sb.WriteString("No enabled MCP servers matched.\n")
		sb.WriteString("Configure or enable MCP servers first, then use `query_mcp_tools` with a known `server_name`.\n")
		sb.WriteString("truncated=false\n")
		sb.WriteString("complete=true\n")
		return sb.String(), 0, 0, false, nil
	}
	if pageCount == 0 {
		sb.WriteString(fmt.Sprintf("No servers in this page. Total matched servers: %d.\n", total))
		sb.WriteString(fmt.Sprintf("Try a smaller offset (< %d).\n", total))
		sb.WriteString(fmt.Sprintf("truncated=%t\n", offset < total))
		sb.WriteString(fmt.Sprintf("complete=%t\n", offset >= total))
		return sb.String(), 0, total, offset < total, nil
	}

	start := offset + 1
	end := offset + pageCount
	sb.WriteString(fmt.Sprintf("Showing servers %d-%d of %d (offset=%d, limit=%d).\n", start, end, total, offset, limit))
	sb.WriteString(fmt.Sprintf("truncated=%t\n", truncated))
	sb.WriteString(fmt.Sprintf("complete=%t\n\n", !truncated))

	for _, server := range page {
		if server == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("- **%s** [%s]", server.Name, server.Type))
		if server.URL != "" {
			sb.WriteString(fmt.Sprintf(" url=%s", utils.ShrinkString(server.URL, 120)))
		} else if server.Command != "" {
			sb.WriteString(fmt.Sprintf(" command=%s", utils.ShrinkString(server.Command, 120)))
		}
		sb.WriteString("\n")
	}

	if truncated {
		nextOffset := offset + limit
		remaining := total - end
		sb.WriteString("\n---\n")
		sb.WriteString(fmt.Sprintf("**TRUNCATED**: %d more server(s) not shown on this page.\n", remaining))
		sb.WriteString("Do NOT claim you have listed all servers until `complete=true`.\n")
		sb.WriteString("Fetch the next page with:\n")
		nextAction := fmt.Sprintf(
			"`{\"@action\":\"query_mcp_servers\",\"offset\":%d,\"limit\":%d}`",
			nextOffset, limit,
		)
		if keyword != "" {
			nextAction = fmt.Sprintf(
				"`{\"@action\":\"query_mcp_servers\",\"keyword\":%q,\"offset\":%d,\"limit\":%d}`",
				keyword, nextOffset, limit,
			)
		}
		sb.WriteString(nextAction + "\n")
	} else {
		sb.WriteString("\n---\n")
		sb.WriteString("**COMPLETE**: All matched enabled servers are listed above.\n")
	}

	sb.WriteString("\nNext: use `{\"@action\":\"query_mcp_tools\",\"server_name\":\"<name>\"}` to list enabled tools for a server.\n")
	sb.WriteString("Tool lists are also paginated via `offset` and `limit` on `query_mcp_tools`.\n")
	sb.WriteString("For keyword discovery across all capabilities (including MCP tools), use `search_capabilities` instead.\n")

	return sb.String(), pageCount, total, truncated, nil
}

func paginateMCPServers(servers []*schema.MCPServer, offset, limit int) ([]*schema.MCPServer, bool) {
	if len(servers) == 0 {
		return nil, false
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultMCPServerQueryLimit
	}
	if offset >= len(servers) {
		return nil, offset < len(servers)
	}
	end := offset + limit
	if end > len(servers) {
		end = len(servers)
	}
	return servers[offset:end], end < len(servers)
}

func renderMCPToolQueryResult(serverName string, offset, limit int) (feedback string, pageCount int, total int, truncated bool, err error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "Database not available; cannot query MCP tools.", 0, 0, false, nil
	}

	tools, err := yakit.GetEnabledMCPServerToolConfigsByServer(db, strings.TrimSpace(serverName))
	if err != nil {
		return "", 0, 0, false, err
	}
	return buildMCPToolQueryFeedback(serverName, tools, offset, limit)
}

func buildMCPToolQueryFeedback(serverName string, tools []*schema.MCPServerToolConfig, offset, limit int) (feedback string, pageCount int, total int, truncated bool, err error) {
	if limit <= 0 {
		limit = defaultMCPToolQueryLimit
	}
	if offset < 0 {
		offset = 0
	}

	total = len(tools)
	page, truncated := paginateMCPToolConfigs(tools, offset, limit)
	pageCount = len(page)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Enabled MCP Tools for Server: %s\n\n", serverName))
	if total == 0 {
		sb.WriteString("No enabled MCP tools found for this server.\n")
		sb.WriteString("Ensure the server is enabled and has connected at least once to cache tool metadata.\n")
		sb.WriteString("truncated=false\n")
		sb.WriteString("complete=true\n")
		return sb.String(), 0, 0, false, nil
	}
	if pageCount == 0 {
		sb.WriteString(fmt.Sprintf("No tools in this page. Total enabled tools: %d.\n", total))
		sb.WriteString(fmt.Sprintf("Try a smaller offset (< %d).\n", total))
		sb.WriteString(fmt.Sprintf("truncated=%t\n", offset < total))
		sb.WriteString(fmt.Sprintf("complete=%t\n", offset >= total))
		return sb.String(), 0, total, offset < total, nil
	}

	start := offset + 1
	end := offset + pageCount
	sb.WriteString(fmt.Sprintf("Showing tools %d-%d of %d (offset=%d, limit=%d).\n", start, end, total, offset, limit))
	sb.WriteString(fmt.Sprintf("truncated=%t\n", truncated))
	sb.WriteString(fmt.Sprintf("complete=%t\n\n", !truncated))
	sb.WriteString("Call via `require_tool` using the full tool name shown below.\n\n")
	for _, tool := range page {
		if tool == nil {
			continue
		}
		fullName := fmt.Sprintf("mcp_%s_%s", tool.ServerName, tool.ToolName)
		desc := utils.ShrinkString(tool.Description, 200)
		if desc == "" {
			desc = "(no cached description yet)"
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", fullName, desc))
	}

	if truncated {
		nextOffset := offset + limit
		remaining := total - end
		sb.WriteString("\n---\n")
		sb.WriteString(fmt.Sprintf("**TRUNCATED**: %d more tool(s) not shown on this page.\n", remaining))
		sb.WriteString("Do NOT claim you have listed all tools until `complete=true`.\n")
		sb.WriteString("Fetch the next page with:\n")
		sb.WriteString(fmt.Sprintf(
			"`{\"@action\":\"query_mcp_tools\",\"server_name\":%q,\"offset\":%d,\"limit\":%d}`\n",
			serverName, nextOffset, limit,
		))
	} else {
		sb.WriteString("\n---\n")
		sb.WriteString("**COMPLETE**: All enabled tools for this server are listed above.\n")
	}

	return sb.String(), pageCount, total, truncated, nil
}

func paginateMCPToolConfigs(tools []*schema.MCPServerToolConfig, offset, limit int) ([]*schema.MCPServerToolConfig, bool) {
	if len(tools) == 0 {
		return nil, false
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultMCPToolQueryLimit
	}
	if offset >= len(tools) {
		return nil, offset < len(tools)
	}
	end := offset + limit
	if end > len(tools) {
		end = len(tools)
	}
	return tools[offset:end], end < len(tools)
}
