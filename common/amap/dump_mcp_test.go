package amap

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestDumpMCPToolsInfo(t *testing.T) {
	// 设置 MCP 服务器地址
	mcpServerURL := "https://mcp.amap.com/sse?key="

	// 创建 SSE MCP 客户端
	mcpClient, err := client.NewSSEMCPClient(mcpServerURL)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpClient.Close()

	// 设置上下文和超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动客户端连接
	err = mcpClient.Start(ctx)
	if err != nil {
		t.Fatalf("启动 MCP 客户端连接失败: %v", err)
	}

	// 初始化客户端
	t.Log("初始化 MCP 客户端...")
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "dump-mcp-tools-client",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	t.Logf("初始化成功，服务器名称: %s, 版本: %s",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)

	// 获取工具列表
	t.Log("获取工具列表...")
	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		t.Fatalf("获取工具列表失败: %v", err)
	}

	// 输出工具数量
	t.Logf("获取到 %d 个工具", len(toolsResult.Tools))

	// 保存工具信息到一个漂亮格式的 JSON 文件
	dumpTools(t, toolsResult.Tools)

	promptsResult, _ := mcpClient.ListPrompts(ctx, mcp.ListPromptsRequest{})
	spew.Dump(promptsResult)
	tempsResult, _ := mcpClient.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
	spew.Dump(tempsResult)
	resourcesResult, _ := mcpClient.ListResources(ctx, mcp.ListResourcesRequest{})
	spew.Dump(resourcesResult)
}

// dumpTools 将工具信息保存为漂亮格式的 JSON
func dumpTools(t *testing.T, tools []*mcp.Tool) {
	// 构建工具描述的映射
	toolsMap := make(map[string]map[string]interface{})

	for _, tool := range tools {
		toolInfo := make(map[string]interface{})
		toolInfo["description"] = tool.Description

		if len(tool.InputSchema.Properties) > 0 {
			params := make(map[string]interface{})

			for paramName, paramSchema := range tool.InputSchema.Properties {
				paramInfo := make(map[string]interface{})

				// 尝试从 paramSchema 中提取类型、描述等信息
				if schemaMap, ok := paramSchema.(map[string]interface{}); ok {
					if typ, ok := schemaMap["type"]; ok {
						paramInfo["type"] = typ
					}

					if desc, ok := schemaMap["description"]; ok {
						paramInfo["description"] = desc
					}

					if def, ok := schemaMap["default"]; ok {
						paramInfo["default"] = def
					}

					if required, ok := schemaMap["required"]; ok {
						paramInfo["required"] = required
					}

					if enum, ok := schemaMap["enum"]; ok {
						paramInfo["enum"] = enum
					}
				}

				params[paramName] = paramInfo
			}

			toolInfo["parameters"] = params
		}

		toolsMap[tool.Name] = toolInfo
	}

	// 转换为漂亮格式的 JSON
	jsonBytes, err := json.MarshalIndent(toolsMap, "", "  ")
	if err != nil {
		t.Errorf("转换工具信息为 JSON 失败: %v", err)
		return
	}

	// 输出工具信息
	log.Infof("工具详细信息: \n%s", string(jsonBytes))

	log.Infof("总共获取到 %d 个工具信息", len(tools))

	// 打印每个工具的名称和简短描述
	for i, tool := range tools {
		fmt.Printf("%3d. %s: %s\n", i+1, tool.Name, tool.Description)
	}
}

// TestDumpMCPToolsInfoWithFilter 获取特定类别的工具
func TestDumpMCPToolsInfoWithFilter(t *testing.T) {
	// 设置 MCP 服务器地址
	mcpServerURL := "http://localhost:11432/sse"

	// 创建 SSE MCP 客户端
	mcpClient, err := client.NewSSEMCPClient(mcpServerURL)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpClient.Close()

	// 设置上下文和超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动客户端连接
	err = mcpClient.Start(ctx)
	if err != nil {
		t.Fatalf("启动 MCP 客户端连接失败: %v", err)
	}

	// 初始化客户端
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "dump-mcp-tools-filtered-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 获取工具列表
	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		t.Fatalf("获取工具列表失败: %v", err)
	}

	// 过滤特定类别的工具（例如: port_scan）
	portScanTools := make([]*mcp.Tool, 0)
	for _, tool := range toolsResult.Tools {
		// 这里可以根据需要自定义过滤条件
		if tool.Name == "port_scan" || tool.Name == "query_ports" || tool.Name == "delete_ports" {
			portScanTools = append(portScanTools, tool)
		}
	}

	// 输出端口扫描相关工具
	t.Logf("端口扫描相关工具: %d 个", len(portScanTools))
	dumpTools(t, portScanTools)
}
