package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// defaultMCPClientInfo is the implementation identity yaklang advertises during
// the MCP initialize handshake (akin to a User-Agent). The version tracks the
// running yaklang build so MCP servers can tell client versions apart.
func defaultMCPClientInfo() mcp.Implementation {
	return mcp.Implementation{
		Name:    "yaklang-aitool-loader",
		Version: consts.GetYakVersion(),
	}
}

// mcpToolParamInfo is a lightweight representation of a tool parameter used for
// JSON serialization into MCPServerToolConfig.ParamsJSON.
type mcpToolParamInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
	Required    bool   `json:"required"`
}

func mapStringAnyToStringMap(input schema.MapStringAny) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = fmt.Sprintf("%v", value)
	}
	return result
}

func LoadAllEnabledAIToolsFromMCPServers(db *gorm.DB, ctx context.Context) ([]*Tool, error) {
	return LoadAllEnabledAIToolsFromMCPServersWithCallback(db, ctx, nil, nil, nil, nil)
}

func LoadAllEnabledAIToolsFromMCPServersWithCallback(
	db *gorm.DB, ctx context.Context,
	onStart func(mcpServer *schema.MCPServer),
	onDone func(mcpServer *schema.MCPServer, tools []*Tool, err error),
	onAllDone func(tools []*Tool, err error),
	onToolsListChanged MCPToolsListChangedHandler,
) ([]*Tool, error) {
	swg := utils.NewSizedWaitGroup(10)
	var results []*Tool
	var m sync.Mutex
	var finalError error
	defer func() {
		if onAllDone != nil {
			onAllDone(results, finalError)
		}
	}()

	for server := range yakit.YieldEnabledMCPServers(ctx, db) {
		if err := swg.AddWithContext(ctx, 1); err != nil {
			finalError = utils.Errorf("load mcp servers cancelled: %v", err)
			return results, finalError
		}
		mcpServer := server
		if onStart != nil {
			onStart(mcpServer)
		}
		go func() {
			defer swg.Done()
			done := utils.NewOnce()
			defer func() {
				done.Do(func() {
					if onDone != nil {
						onDone(mcpServer, results, nil)
					}
				})
			}()
			tools, err := LoadAIToolFromMCPServers(db, ctx, mcpServer.Name, onToolsListChanged)
			done.Do(func() {
				if onDone != nil {
					onDone(mcpServer, tools, err)
				}
			})
			if err != nil {
				log.Errorf("load aitools from mcp server %s failed: %v", mcpServer.Name, err)
				return
			}
			m.Lock()
			results = append(results, tools...)
			m.Unlock()
		}()
	}
	swg.Wait()
	return results, nil
}

// LoadAIToolFromMCPServers 从数据库中加载指定名称的 MCP 服务器，并将其工具转换为 AITool
// name: MCP 服务器名称
// db: 数据库连接，如果为 nil 则使用默认的 profile 数据库
// onToolsListChanged: optional handler for notifications/tools/list_changed from the remote server
// 返回: 从该 MCP 服务器加载的所有 AITool 列表
func LoadAIToolFromMCPServers(db *gorm.DB, ctx context.Context, name string, onToolsListChanged ...MCPToolsListChangedHandler) ([]*Tool, error) {
	var listChangedHandler MCPToolsListChangedHandler
	if len(onToolsListChanged) > 0 {
		listChangedHandler = onToolsListChanged[0]
	}
	if db == nil {
		// 使用默认的 profile 数据库
		db = consts.GetGormProfileDatabase()
		if db == nil {
			return nil, utils.Errorf("profile database is not initialized")
		}
	}

	mcpServer, err := yakit.GetMCPServerByName(db, name)
	if err != nil {
		return nil, utils.Errorf("get mcp server by name failed: %v", err)
	}
	if !mcpServer.Enable {
		return nil, utils.Errorf("mcp server not found or not enabled: %s", name)
	}

	// 创建 MCP 客户端
	mcpClient, err := createMCPClient(mcpServer)
	if err != nil {
		return nil, utils.Errorf("create mcp client failed: %v", err)
	}
	// 注意：不要在这里关闭 mcpClient，因为返回的 AITool 的 Callback 还需要使用它
	// 客户端的生命周期应该和工具一样长，由调用方负责管理

	// 初始化连接
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = defaultMCPClientInfo()

	var listChangedState *mcpToolsListChangedState
	if listChangedHandler != nil {
		listChangedState = registerMCPToolsListChangedHandler(mcpClient, db, mcpServer, nil, listChangedHandler)
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		return nil, utils.Errorf("initialize mcp client failed: %v", err)
	}

	// 获取工具列表
	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		return nil, utils.Errorf("list tools failed: %v", err)
	}

	// Reconcile the local DB cache with the freshly-fetched tool list first, so
	// that the enable flags read below reflect the current tool set.
	liveEntries := make([]yakit.MCPToolEntry, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		liveEntries = append(liveEntries, yakit.MCPToolEntry{
			ToolName:    t.Name,
			FullName:    fmt.Sprintf("mcp_%s_%s", mcpServer.Name, t.Name),
			Description: t.Description,
			ParamsJSON:  serializeMCPToolParams(&t.InputSchema),
		})
	}
	if syncErr := yakit.SyncAndCacheMCPServerTools(db, mcpServer.Name, liveEntries); syncErr != nil {
		log.Warnf("sync mcp tool cache for server %s failed: %v", mcpServer.Name, syncErr)
	}

	// Batch-load per-tool enable flags after sync so deleted tools are excluded.
	toolConfigs, err := yakit.BatchGetMCPServerToolConfigs(db, mcpServer.Name)
	if err != nil {
		log.Warnf("batch load tool configs for server %s failed: %v, falling back to defaults", mcpServer.Name, err)
		toolConfigs = map[string]*schema.MCPServerToolConfig{}
	}

	// 转换为 AITool
	var aiTools []*Tool
	for _, mcpTool := range toolsResult.Tools {
		cfg, ok := toolConfigs[mcpTool.Name]
		if ok && !cfg.Enable {
			log.Debugf("mcp tool %s/%s is disabled, skipping", mcpServer.Name, mcpTool.Name)
			continue
		}
		aiTool, err := convertMCPToolToAITool(mcpTool, mcpServer, mcpClient)
		if err != nil {
			log.Errorf("convert mcp tool to aitool failed: %v", err)
			continue
		}
		aiTools = append(aiTools, aiTool)
	}

	if len(aiTools) == 0 {
		return nil, utils.Errorf("no tools found in mcp server: %s", name)
	}

	if listChangedState != nil {
		SyncMCPToolsListChangedState(listChangedState, aiTools)
	}

	return aiTools, nil
}

// LoadAIToolsFromMCPServer 从单个显式 MCP server（不查 DB）加载工具，用于会话级挂载。
// allowedTools 非空时在 client 侧按裸工具名做白名单过滤，server 多暴露的工具一律丢弃，
// 不依赖 server 自觉只暴露。客户端生命周期随返回的 Tool（Callback 持有 client）。
func LoadAIToolsFromMCPServer(ctx context.Context, server *schema.MCPServer, allowedTools []string) ([]*Tool, error) {
	if server == nil {
		return nil, utils.Errorf("mcp server is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	mcpClient, err := createMCPClient(server)
	if err != nil {
		return nil, utils.Errorf("create mcp client failed: %v", err)
	}
	// createMCPClient already opened a live connection (sse) or spawned a
	// subprocess (stdio). On any failure path below we must close it; only when
	// tools are returned does ownership transfer to the caller (the returned
	// tools' callbacks keep using the client), so we must NOT close it then.
	success := false
	defer func() {
		if !success {
			_ = mcpClient.Close()
		}
	}()

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = defaultMCPClientInfo()
	if _, err = mcpClient.Initialize(initCtx, initRequest); err != nil {
		return nil, utils.Errorf("initialize mcp client failed: %v", err)
	}

	toolsResult, err := mcpClient.ListTools(initCtx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, utils.Errorf("list tools failed: %v", err)
	}

	allow := make(map[string]bool, len(allowedTools))
	for _, name := range allowedTools {
		if name != "" {
			allow[name] = true
		}
	}

	var aiTools []*Tool
	for _, mcpTool := range toolsResult.Tools {
		if len(allow) > 0 && !allow[mcpTool.Name] {
			continue
		}
		aiTool, err := convertMCPToolToAITool(mcpTool, server, mcpClient)
		if err != nil {
			log.Errorf("convert mcp tool to aitool failed: %v", err)
			continue
		}
		aiTools = append(aiTools, aiTool)
	}

	if len(aiTools) == 0 {
		return nil, utils.Errorf("no tools loaded from mcp server %s (allowlist=%v)", server.Name, allowedTools)
	}
	success = true
	return aiTools, nil
}

// createMCPClient 根据 MCP 服务器配置创建客户端
func createMCPClient(server *schema.MCPServer) (client.MCPClient, error) {
	switch server.Type {
	case "stdio":
		// 解析命令和参数
		commandParts := utils.PrettifyListFromStringSplited(server.Command, " ")
		if len(commandParts) == 0 {
			return nil, utils.Errorf("invalid command: %s", server.Command)
		}
		command := commandParts[0]
		args := commandParts[1:]
		return client.NewStdioMCPClient(command, []string{}, args...)

	case "sse":
		sseMcpClient, err := client.NewSSEMCPClient(server.URL, mapStringAnyToStringMap(server.Headers))
		if err != nil {
			return nil, utils.Errorf("create sse mcp client failed: %v", err)
		}
		// 使用一个长期存在的 context 来保持 SSE 连接
		// 不能使用带超时的 context，否则连接会断开导致 session 失效
		err = sseMcpClient.Start(context.Background())
		if err != nil {
			return nil, utils.Errorf("start sse mcp client failed: %v", err)
		}
		return sseMcpClient, nil
	case "streamable_http":
		streamableHTTPClient, err := client.NewStreamableHTTPMCPClient(
			server.URL,
			mapStringAnyToStringMap(server.Headers),
		)
		if err != nil {
			return nil, utils.Errorf(
				"create streamable http mcp client failed: %v",
				err,
			)
		}
		return streamableHTTPClient, nil

	default:
		return nil, utils.Errorf("unsupported server type: %s", server.Type)
	}
}

// convertMCPToolToAITool converts an MCP tool descriptor into an AITool.
// MCP tools use the same global AgreePolicy as all other tools; no per-tool
// approval override is applied here.
func convertMCPToolToAITool(mcpTool *mcp.Tool, server *schema.MCPServer, mcpClient client.MCPClient) (*Tool, error) {
	// Tool name convention: mcp_{server_name}_{tool_name}
	toolName := fmt.Sprintf("mcp_%s_%s", server.Name, mcpTool.Name)

	description := mcpTool.Description
	if description == "" {
		description = fmt.Sprintf("[MCP:%s] Tool from MCP server: %s", server.Name, server.Name)
	} else {
		description = fmt.Sprintf("[MCP:%s] %s", server.Name, description)
	}

	aiTool, err := NewFromMCPTool(
		mcpTool,
		WithDescription(description),
		WithKeywords([]string{"mcp", server.Name, mcpTool.Name, "external", "remote"}),
		WithVerboseName(fmt.Sprintf("%s (MCP:%s)", mcpTool.Name, server.Name)),
		WithCallback(createToolCallback(mcpClient, mcpTool.Name, server.Name)),
	)
	if err != nil {
		return nil, utils.Errorf("create aitool from mcp tool failed: %v", err)
	}

	aiTool.Name = toolName
	aiTool.BridgeMCPClient = mcpClient
	return aiTool, nil
}

// isMCPInternalInvokeParam reports keys injected by Yakit/AI runtime that must not
// be forwarded to external MCP servers (they validate against their own JSON schema).
func isMCPInternalInvokeParam(key string) bool {
	switch key {
	case "runtime_id", "@action", "__DEFAULT__", "__FALLBACK__", "__[yaklang-raw]__":
		return true
	default:
		return false
	}
}

// filterParamsForMCPCall strips Yakit-internal invoke params before forwarding to MCP.
func filterParamsForMCPCall(params InvokeParams) map[string]interface{} {
	mcpParams := make(map[string]interface{})
	for k, v := range params {
		if isMCPInternalInvokeParam(k) {
			continue
		}
		mcpParams[k] = v
	}
	return mcpParams
}

// firstMCPTextFromContent extracts the first text block from MCP tool result content.
// JSON unmarshaling yields map[string]interface{} rather than mcp.TextContent structs.
func firstMCPTextFromContent(content []any) string {
	for _, item := range content {
		if textContent, ok := item.(mcp.TextContent); ok && textContent.Text != "" {
			return textContent.Text
		}
		if m, ok := item.(map[string]interface{}); ok {
			if text, _ := m["text"].(string); text != "" {
				return text
			}
		}
	}
	return ""
}

// createToolCallback 创建工具调用回调函数
func createToolCallback(mcpClient client.MCPClient, toolName string, serverName string) InvokeCallback {
	return func(ctx context.Context, params InvokeParams, runtimeConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
		// 记录工具调用
		log.Infof("calling mcp tool: %s from server: %s", toolName, serverName)

		mcpParams := filterParamsForMCPCall(params)

		// 设置超时
		callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		// 调用 MCP 工具
		callRequest := mcp.CallToolRequest{}
		callRequest.Params.Name = toolName
		callRequest.Params.Arguments = mcpParams

		result, err := mcpClient.CallTool(callCtx, callRequest)
		if err != nil {
			errMsg := fmt.Sprintf("call mcp tool failed: %v", err)
			stderr.Write([]byte(errMsg))
			return nil, utils.Errorf(errMsg)
		}

		// 处理结果
		if result.IsError {
			errMsg := firstMCPTextFromContent(result.Content)
			if errMsg == "" {
				errMsg = "tool execution failed"
			}
			log.Errorf("mcp tool %s execution error: %s, content: %#v", toolName, errMsg, result.Content)
			stderr.Write([]byte(errMsg))
			return nil, utils.Errorf(errMsg)
		}

		// 提取结果内容
		var resultContent string
		for _, content := range result.Content {
			switch c := content.(type) {
			case mcp.TextContent:
				resultContent += c.Text
			case mcp.ImageContent:
				resultContent += fmt.Sprintf("[Image: %s]", c.MIMEType)
			default:
				if text := firstMCPTextFromContent([]any{c}); text != "" {
					resultContent += text
				} else {
					resultContent += fmt.Sprintf("%v", c)
				}
			}
		}

		// 输出到 stdout
		if resultContent != "" {
			stdout.Write([]byte(resultContent))
		}

		return resultContent, nil
	}
}

// LoadAIToolsFromMCPCapability loads MCP tools for an enabled capability entry.
// If name looks like a full AI tool name (mcp_{server}_{tool}), it resolves that tool;
// otherwise name is treated as an MCP server name and all tools from that server are loaded.
func LoadAIToolsFromMCPCapability(db *gorm.DB, ctx context.Context, name string) ([]*Tool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, utils.Error("mcp capability name is empty")
	}
	if strings.HasPrefix(name, "mcp_") {
		tool, err := loadAIToolFromMCPServersByAIToolName(db, ctx, name)
		if err != nil {
			return nil, err
		}
		return []*Tool{tool}, nil
	}
	return LoadAIToolFromMCPServers(db, ctx, name, nil)
}

func loadAIToolFromMCPServersByAIToolName(db *gorm.DB, ctx context.Context, aiToolName string) (*Tool, error) {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}
	if db == nil {
		return nil, utils.Error("profile database is nil")
	}
	for server := range yakit.YieldEnabledMCPServers(ctx, db) {
		tools, err := LoadAIToolFromMCPServers(db, ctx, server.Name, nil)
		if err != nil {
			log.Warnf("load mcp server %q while resolving tool %q failed: %v", server.Name, aiToolName, err)
			continue
		}
		for _, tool := range tools {
			if tool != nil && tool.Name == aiToolName {
				return tool, nil
			}
		}
	}
	return nil, utils.Errorf("mcp tool %q not found in enabled mcp servers", aiToolName)
}

// serializeMCPToolParams converts a ToolInputSchema into a compact JSON string
// suitable for storing in MCPServerToolConfig.ParamsJSON.
func serializeMCPToolParams(schema *mcp.ToolInputSchema) string {
	if schema == nil || schema.Properties == nil || schema.Properties.Len() == 0 {
		return "[]"
	}

	requiredSet := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	var params []mcpToolParamInfo
	schema.Properties.ForEach(func(name string, val any) bool {
		p := mcpToolParamInfo{Name: name, Required: requiredSet[name]}
		if m, ok := val.(map[string]interface{}); ok {
			if t, ok := m["type"].(string); ok {
				p.Type = t
			}
			if d, ok := m["description"].(string); ok {
				p.Description = d
			}
			if def, ok := m["default"]; ok {
				p.Default = fmt.Sprintf("%v", def)
			}
		}
		if p.Type == "" {
			p.Type = "string"
		}
		params = append(params, p)
		return true
	})

	b, err := json.Marshal(params)
	if err != nil {
		return "[]"
	}
	return string(b)
}
