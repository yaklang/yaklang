package aitool

import (
	"context"
	"fmt"
	"io"
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

func LoadAllEnabledAIToolsFromMCPServers(db *gorm.DB, ctx context.Context) ([]*Tool, error) {
	swg := utils.NewSizedWaitGroup(10)
	var results []*Tool
	var m sync.Mutex
	for server := range yakit.YieldEnabledMCPServers(ctx, db) {
		if err := swg.AddWithContext(ctx, 1); err != nil {
			return results, utils.Errorf("load mcp servers cancelled: %v", err)
		}
		mcpServer := server
		go func() {
			defer swg.Done()
			tools, err := LoadAIToolFromMCPServers(db, ctx, mcpServer.Name)
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
// 返回: 从该 MCP 服务器加载的所有 AITool 列表
func LoadAIToolFromMCPServers(db *gorm.DB, ctx context.Context, name string) ([]*Tool, error) {
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
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "yaklang-aitool-loader",
		Version: "1.0.0",
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

	// 转换为 AITool
	var aiTools []*Tool
	for _, mcpTool := range toolsResult.Tools {
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
		sseMcpClient, err := client.NewSSEMCPClient(server.URL)
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

	default:
		return nil, utils.Errorf("unsupported server type: %s", server.Type)
	}
}

// convertMCPToolToAITool 将 MCP 工具转换为 AITool
func convertMCPToolToAITool(mcpTool *mcp.Tool, server *schema.MCPServer, mcpClient client.MCPClient) (*Tool, error) {
	// 生成工具名称: mcp_{server_name}_{tool_name}
	toolName := fmt.Sprintf("mcp_%s_%s", server.Name, mcpTool.Name)

	// 创建工具描述
	description := mcpTool.Description
	if description == "" {
		description = fmt.Sprintf("Tool from MCP server: %s", server.Name)
	} else {
		description = fmt.Sprintf("[MCP:%s] %s", server.Name, description)
	}

	// 创建 AITool，使用 NewFromMCPTool
	aiTool, err := NewFromMCPTool(
		mcpTool,
		WithDescription(description),
		WithCallback(createToolCallback(mcpClient, mcpTool.Name, server.Name)),
	)
	if err != nil {
		return nil, utils.Errorf("create aitool from mcp tool failed: %v", err)
	}

	// 更新工具名称
	aiTool.Name = toolName

	return aiTool, nil
}

// createToolCallback 创建工具调用回调函数
func createToolCallback(mcpClient client.MCPClient, toolName string, serverName string) InvokeCallback {
	return func(ctx context.Context, params InvokeParams, runtimeConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
		// 记录工具调用
		log.Infof("calling mcp tool: %s from server: %s", toolName, serverName)

		// 转换参数为 map[string]interface{}
		mcpParams := make(map[string]interface{})
		for k, v := range params {
			mcpParams[k] = v
		}

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
			errMsg := "tool execution failed"
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(mcp.TextContent); ok {
					errMsg = textContent.Text
				}
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
				// 其他类型的内容，尝试转换为字符串
				resultContent += fmt.Sprintf("%v", c)
			}
		}

		// 输出到 stdout
		if resultContent != "" {
			stdout.Write([]byte(resultContent))
		}

		return resultContent, nil
	}
}
