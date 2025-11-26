package yakgrpc

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	mcpclient "github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 测试前设置
func init() {
	mcp.RegisterNewLocalClient(func(locals ...bool) (mcp.YakClientInterface, error) {
		client, err := NewLocalClient(locals...)
		if err != nil {
			return nil, err
		}
		v, ok := client.(mcp.YakClientInterface)
		if !ok {
			return nil, utils.Error("failed to cast client to yakgrpc.Client")
		}
		return v, nil
	})
}

func TestGRPC_StartMcpServer_BasicFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建启动请求
	req := &ypb.StartMcpServerRequest{
		Host:      "127.0.0.1",
		Port:      0, // 使用随机端口
		EnableAll: true,
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var responses []*ypb.StartMcpServerResponse
	var serverUrl string

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NotNil(t, resp)
		responses = append(responses, resp)

		if resp.ServerUrl != "" {
			serverUrl = resp.ServerUrl
		}

		log.Infof("Received MCP response: Status=%s, Message=%s, ServerUrl=%s",
			resp.Status, resp.Message, resp.ServerUrl)

		if resp.Status == "running" {
			break
		}
	}

	// 验证收到的响应
	require.GreaterOrEqual(t, len(responses), 2)

	// 第一个响应应该是 starting 状态
	require.Equal(t, "starting", responses[0].Status)
	require.Contains(t, responses[0].Message, "Initializing MCP server")

	// 应该有一个 configured 状态
	configuredFound := false
	runningFound := false
	for _, resp := range responses {
		if resp.Status == "configured" {
			configuredFound = true
		}
		if resp.Status == "running" {
			runningFound = true
			require.NotEmpty(t, resp.ServerUrl, "ServerUrl should be set when status is running")
		}
	}
	require.True(t, configuredFound, "Should receive configured status")
	require.True(t, runningFound, "Should receive running status")
	require.NotEmpty(t, serverUrl, "ServerUrl should be provided")

	// 验证 URL 格式
	require.Contains(t, serverUrl, "http://127.0.0.1:", "ServerUrl should contain correct host")

	// 创建 SSE MCP 客户端
	mcpClient, err := mcpclient.NewSSEMCPClient(serverUrl)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer mcpClient.Close()

	// 设置上下文和超时
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动客户端连接
	err = mcpClient.Start(ctx)
	if err != nil {
		t.Fatalf("启动 MCP 客户端连接失败: %v", err)
	}

	// 初始化客户端
	t.Log("初始化 MCP 客户端...")
	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
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

	toolsRequest := rawmcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.True(t, len(toolsResult.Tools) > 0)
	log.Infof("获取到 %d 个工具", len(toolsResult.Tools))
}

func TestGRPC_StartMcpServer_DefaultPort(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 不指定端口，应该使用随机端口
	req := &ypb.StartMcpServerRequest{
		// 不设置 Host 和 Port，使用默认值
		Tool: []string{"codec"},
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if resp.Status == "running" {
			require.NotEmpty(t, resp.ServerUrl)
			require.Contains(t, resp.ServerUrl, "http://127.0.0.1:", "Should use default host")
			break
		}
	}
}

func TestGRPC_StartMcpServer_WithAITool(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 第一步：创建一个 AI Tool
	toolName := "test_mcp_tool_" + utils.RandStringBytes(8)
	toolDescription := "Test tool for MCP server validation"
	toolContent := `
cli.String("url", cli.setRequired(true))
`

	saveReq := &ypb.SaveAIToolRequest{
		Name:        toolName,
		Description: toolDescription,
		Content:     toolContent,
		Keywords:    []string{"test", "mcp"},
	}

	saveResp, err := client.SaveAIToolV2(context.Background(), saveReq)
	require.NoError(t, err)
	require.NotNil(t, saveResp)
	require.True(t, saveResp.IsSuccess, "Tool should be created successfully")

	// 清理：测试结束后删除创建的工具
	defer func() {
		deleteReq := &ypb.DeleteAIToolRequest{
			ToolNames: []string{toolName},
		}
		_, _ = client.DeleteAITool(context.Background(), deleteReq)
	}()

	// 第二步：启动 MCP Server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	startReq := &ypb.StartMcpServerRequest{
		Host:      "127.0.0.1",
		Port:      0, // 使用随机端口
		EnableAll: true,
	}

	stream, err := client.StartMcpServer(ctx, startReq)
	require.NoError(t, err)

	var serverUrl string

	// 接收状态消息，直到服务器启动
	for i := 0; i < 5; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NotNil(t, resp)

		if resp.ServerUrl != "" {
			serverUrl = resp.ServerUrl
		}

		log.Infof("MCP Server Status: %s, Message: %s", resp.Status, resp.Message)

		if resp.Status == "running" {
			break
		}
	}

	require.NotEmpty(t, serverUrl, "Server URL should be provided")

	// 第三步：连接 MCP 客户端并验证工具是否存在
	mcpClient, err := mcpclient.NewSSEMCPClient(serverUrl)
	require.NoError(t, err)
	defer mcpClient.Close()

	// 启动客户端连接
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer clientCancel()

	err = mcpClient.Start(clientCtx)
	require.NoError(t, err)

	// 初始化客户端
	initRequest := rawmcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = rawmcp.Implementation{
		Name:    "test-mcp-aitool-client",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(clientCtx, initRequest)
	require.NoError(t, err)
	require.NotNil(t, initResult)
	log.Infof("MCP Client initialized with server: %s %s",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)

	// 第四步：列出所有工具并验证我们创建的工具是否存在
	toolsRequest := rawmcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(clientCtx, toolsRequest)
	require.NoError(t, err)
	require.NotNil(t, toolsResult)
	require.Greater(t, len(toolsResult.Tools), 0, "Should have at least one tool")

	log.Infof("Total tools available: %d", len(toolsResult.Tools))

	// 查找我们创建的工具
	toolFound := false
	for _, tool := range toolsResult.Tools {
		log.Infof("Available tool: %s - %s", tool.Name, tool.Description)
		if tool.Name == toolName {
			toolFound = true
			assert.Equal(t, toolDescription, tool.Description, "Tool description should match")
			log.Infof("Found our test tool: %s", toolName)
			break
		}
	}

	// 验证工具必须存在
	require.True(t, toolFound, "Created AI tool '%s' should be available in MCP server", toolName)
}
